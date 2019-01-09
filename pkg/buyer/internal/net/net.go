package net

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
)

// FindListeningTCPPorts finds possible tcp ports that are currently opened
// for listening in the current machine.
func FindListeningTCPPorts() ([]int, error) {
	if netstatCmd == "" {
		return nil, errors.New("dynamic searching for wallet not supported in " +
			"this arch; specify wallet port")
	}

	cmd := exec.Command(netstatCmd, netstatCmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	exp := regexp.MustCompile(listeningTCPPortsRegext)

	lines := strings.Split(string(output), "\n")
	ports := make([]int, 0)
	for _, line := range lines {
		matches := exp.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}

		port, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		ports = append(ports, port)
	}

	return ports, nil
}

// FindListeningWallets tries to find running wallets on localhost that are
// using the specified certFile (rpc.cert), have enabled grpc and are running
// on the specified network.
func FindListeningWallets(certFile string, params *chaincfg.Params) ([]string, error) {
	ports, err := FindListeningTCPPorts()
	if err != nil {
		return nil, errors.Wrapf(err, "error finding listening tcp ports")
	}

	creds, err := credentials.NewClientTLSFromFile(certFile, "localhost")
	if err != nil {
		return nil, errors.Wrapf(err, "error reading certificate file")
	}

	hosts := make([]string, 0)
	for _, port := range ports {
		host := fmt.Sprintf("127.0.0.1:%d", port)

		ctx, cancel := context.WithTimeout(context.Background(),
			300*time.Millisecond)
		defer cancel()

		conn, err := grpc.DialContext(ctx, host, grpc.WithTransportCredentials(creds))
		if err != nil {
			continue
		}

		defer conn.Close()
		wsvc := pb.NewWalletServiceClient(conn)

		ctx, cancel = context.WithTimeout(context.Background(),
			300*time.Millisecond)
		defer cancel()
		resp, err := wsvc.Network(ctx, &pb.NetworkRequest{})
		if err != nil {
			continue
		}

		if resp.ActiveNetwork != uint32(params.Net) {
			continue
		}

		hosts = append(hosts, host)
	}

	return hosts, nil

}

// RemoveHostPort removes the last :.* from the host string.
func RemoveHostPort(host string) string {
	idx := strings.LastIndex(host, ":")
	if idx == -1 {
		return host
	}

	return host[:idx]
}

// IsSubDomain returns true if dst is a subdomain of src. It also returns true
// if src == dst. Note that this is a very simple string check, without regard
// for full DNS validation rules.
func IsSubDomain(root, subdomain string) bool {
	idx := strings.LastIndex(subdomain, root)
	if idx == -1 {
		return false
	}

	return idx == len(subdomain)-len(root)
}

// DetermineMatcherHost tries to find the target server to connect to perform
// the matching session, when the configured host is specified as `host`.
// This uses DNS requests to consult applicable SRV records to redirect the
// connection to a specific server.
// Returns the target host, whether the host was changed due to the presence of
// an SRV record and any errors that happened.
//
// Note that in case of errors, the original matcherHost is returned.
func DetermineMatcherHost(matcherHost string) (string, bool, error) {
	host := RemoveHostPort(matcherHost)

	_, addrs, err := net.LookupSRV(splitticket.SplitTicketSrvService,
		splitticket.SplitTicketSrvProto, host)

	if (err == nil) && (len(addrs) > 0) && (len(addrs[0].Target) > 0) {
		target := addrs[0].Target
		if target[len(target)-1:] == "." && (host[len(host)-1:] != ".") {
			target = target[:len(target)-1]
		}
		matcherHost = fmt.Sprintf("%s:%d", target, addrs[0].Port)

		if !IsSubDomain(host, target) {
			return matcherHost, false, errors.Errorf("SRV target host %s is "+
				"not a subdomain of %s", target, host)
		}

		return matcherHost, true, nil
	}

	if err != nil {
		// These are not a great ways to check for the "no such error" host
		// (target host not found) - ie the stakepool did not configure an SRV
		// record and we should continue with process.
		errstr := err.Error()

		if strings.LastIndex(errstr, "no such host") == len(errstr)-12 {
			// Matches on linux platforms.
			return matcherHost, false, nil
		}

		if strings.LastIndex(errstr, "name does not exist.") == len(errstr)-20 {
			// Matches on windows platforms.
			return matcherHost, false, nil
		}
	}

	return matcherHost, false, err
}

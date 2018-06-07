package net

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
)

// FindListeningTCPPorts finds possible tcp ports that are currently opened
// for listening in the current machine.
func FindListeningTCPPorts() ([]int, error) {
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
			100*time.Millisecond)
		defer cancel()

		conn, err := grpc.DialContext(ctx, host, grpc.WithTransportCredentials(creds))
		if err != nil {
			continue
		}

		defer conn.Close()
		wsvc := pb.NewWalletServiceClient(conn)

		ctx, cancel = context.WithTimeout(context.Background(),
			100*time.Millisecond)
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

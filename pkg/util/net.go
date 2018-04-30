package util

import (
	"context"
	"crypto/elliptic"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/certgen"
	"github.com/decred/dcrd/chaincfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
)

func GenerateRPCKeyPair(keyFile, certFile string) error {

	curve := elliptic.P521()

	// Create directories for cert and key files if they do not yet exist.
	certDir, _ := filepath.Split(certFile)
	keyDir, _ := filepath.Split(keyFile)
	err := os.MkdirAll(certDir, 0700)
	if err != nil {
		return err
	}
	err = os.MkdirAll(keyDir, 0700)
	if err != nil {
		return err
	}

	// Generate cert pair.
	org := "Split Ticket Buyer Org"
	validUntil := time.Now().Add(time.Hour * 24 * 365 * 10)
	cert, key, err := certgen.NewTLSCertPair(curve, org,
		validUntil, nil)
	if err != nil {
		return err
	}
	_, err = tls.X509KeyPair(cert, key)
	if err != nil {
		return err
	}

	// Write cert and (potentially) the key files.
	err = ioutil.WriteFile(certFile, cert, 0600)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(keyFile, key, 0600)
	if err != nil {
		os.Remove(certFile)
		return err
	}

	return nil
}

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
		conn, err := grpc.Dial(host, grpc.WithTransportCredentials(creds),
			grpc.WithTimeout(time.Second))

		if err != nil {
			continue
		}

		defer conn.Close()

		wsvc := pb.NewWalletServiceClient(conn)
		resp, err := wsvc.Network(context.Background(), &pb.NetworkRequest{})
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

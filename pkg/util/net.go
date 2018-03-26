package util

import (
	"crypto/elliptic"
	"crypto/tls"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/certgen"
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

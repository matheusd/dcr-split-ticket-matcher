package util

import (
	"crypto/elliptic"
	"crypto/tls"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/certgen"
	"github.com/pkg/errors"
)

var (
	// ErrKeyPairCreated is returned by LoadRPCKeyPair if a new keypair has just
	// been created
	ErrKeyPairCreated = errors.New("created rpc keypair")
)

// GenerateRPCKeyPair generates a keypair for use with grpc
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
	org := "Split Ticket Inc"
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

// LoadRPCKeyPair loads or creates the given keypair if it does not exist
func LoadRPCKeyPair(keyFile, certFile string) (*tls.Certificate, error) {

	var cert tls.Certificate
	var err error
	var created bool

	if _, err = os.Stat(keyFile); os.IsNotExist(err) {
		err = GenerateRPCKeyPair(keyFile, certFile)
		if err != nil {
			return nil, errors.Wrap(err, "error generating new rpc keypair")
		}

		created = true
	}

	cert, err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "error loading keypair files")
	}

	if created {
		err = ErrKeyPairCreated
	}

	return &cert, err
}

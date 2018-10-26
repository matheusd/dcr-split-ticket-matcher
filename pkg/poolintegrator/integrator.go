package poolintegrator

import (
	"crypto/tls"
	"fmt"
	"github.com/decred/slog"
	"net"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/rpcclient"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/integratorrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Daemon is the structure that defines a pool integrator daemon.
type Daemon struct {
	cfg               *Config
	log               slog.Logger
	rpcKeys           *tls.Certificate
	chainParams       *chaincfg.Params
	poolAddrValidator *util.MasterPubPoolAddrValidator
	wallet            *rpcclient.Client
}

// NewDaemon initializes a new pool integrator daemon
func NewDaemon(cfg *Config) (*Daemon, error) {
	logBackend := util.StandardLogBackend(true, cfg.LogDir, "stmpoolintegrator-{date}-{time}.log")
	log := logBackend.Logger("INTG")
	log.SetLevel(cfg.LogLevel)

	log.Criticalf("Split Ticket Matcher / Voting Pool integrator v%s", version.String())

	chainParams := &chaincfg.MainNetParams
	if cfg.TestNet {
		chainParams = &chaincfg.TestNet3Params
	}

	cert, err := util.LoadRPCKeyPair(cfg.KeyFile, cfg.CertFile)
	if err == util.ErrKeyPairCreated {
		log.Infof("Created RPC keypair with cert '%s' and key '%s'",
			cfg.CertFile, cfg.KeyFile)
	} else if err != nil {
		return nil, errors.Wrap(err, "error loading rpc key pair")
	}

	poolAddrValidator, err := util.NewMasterPubPoolAddrValidator(cfg.PoolSubsidyWalletMasterPub, chainParams)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing pool fee address table")
	}

	wallet, err := connectToDcrWallet(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to wallet")
	}

	d := &Daemon{
		cfg:               cfg,
		log:               log,
		rpcKeys:           cert,
		chainParams:       chainParams,
		poolAddrValidator: poolAddrValidator,
		wallet:            wallet,
	}

	return d, nil
}

// ListenAndServe blocks execution by opening the appropriate listening sockets
// and responding to integration requests
func (d *Daemon) ListenAndServe() error {
	intf := fmt.Sprintf(":%d", d.cfg.Port)

	lis, err := net.Listen("tcp", intf)
	if err != nil {
		d.log.Errorf("Error listening: %v", err)
		return err
	}

	var server *grpc.Server
	creds := credentials.NewServerTLSFromCert(d.rpcKeys)
	server = grpc.NewServer(grpc.Creds(creds))

	pb.RegisterVotePoolIntegratorServiceServer(server, d)

	d.log.Criticalf("Listening on %s", intf)
	return server.Serve(lis)
}

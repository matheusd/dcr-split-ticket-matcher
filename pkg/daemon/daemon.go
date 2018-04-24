package daemon

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Daemon is the main instance of a running dcr split ticket matcher daemon
type Daemon struct {
	cfg     *Config
	log     *logging.Logger
	matcher *matcher.Matcher
	wallet  *WalletClient
	rpcKeys *tls.Certificate
	dcrd    *decredNetwork
}

// NewDaemon returns a new daemon instance and prepares it to listen to
// requests.
func NewDaemon(cfg *Config) (*Daemon, error) {
	net := &chaincfg.TestNet2Params

	d := &Daemon{
		cfg: cfg,
		log: logging.MustGetLogger("dcr-split-ticket-matcher"),
	}

	logBackend := util.StandardLogBackend(true, cfg.LogDir, "dcrstmd-{date}-{time}.log", cfg.LogLevel)
	d.log.SetBackend(logBackend)

	d.log.Noticef("Starting dcrstmd version %s", pkg.Version)

	dcfg := &decredNetworkConfig{
		Host:        cfg.DcrdHost,
		Pass:        cfg.DcrdPass,
		CertFile:    cfg.DcrdCert,
		User:        cfg.DcrdUser,
		logBackend:  logBackend,
		chainParams: net,
	}
	dcrd, err := ConnectToDecredNode(dcfg)
	if err != nil {
		panic(err)
	}
	d.dcrd = dcrd

	dcrwcfg := &WalletConfig{
		Host:       cfg.DcrwHost,
		User:       cfg.DcrwUser,
		Pass:       cfg.DcrwPass,
		CertFile:   cfg.DcrwCert,
		logBackend: logBackend,
	}
	dcrw, err := ConnectToDcrWallet(dcrwcfg)
	if err != nil {
		panic(err)
	}
	d.wallet = dcrw

	if cfg.KeyFile != "" {
		if _, err = os.Stat(cfg.KeyFile); os.IsNotExist(err) {
			err = util.GenerateRPCKeyPair(cfg.KeyFile, cfg.CertFile)
			if err != nil {
				panic(err)
			}
			d.log.Noticef("Generated key (%s) and cert (%s) files",
				cfg.KeyFile, cfg.CertFile)
		}

		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			panic(err)
		}

		d.log.Noticef("Loaded key file at %s", cfg.KeyFile)

		d.rpcKeys = &cert
	}

	minAmount, err := dcrutil.NewAmount(cfg.MinAmount)
	if err != nil {
		panic(err)
	}
	d.log.Infof("Minimum participation amount %s", minAmount)

	mcfg := &matcher.Config{
		LogLevel:                  cfg.LogLevel,
		MinAmount:                 uint64(minAmount),
		PriceProvider:             d.dcrd,
		SignPoolSplitOutProvider:  d.wallet,
		ChainParams:               net,
		PoolFee:                   7.5,
		MaxSessionDuration:        30 * time.Second,
		LogBackend:                logBackend,
		StakeDiffChangeStopWindow: 2,
	}
	d.matcher = matcher.NewMatcher(mcfg)

	return d, nil
}

// ListenAndServe connections for the daemon. Returns an error when done.
func (daemon *Daemon) ListenAndServe() error {
	intf := fmt.Sprintf(":%d", daemon.cfg.Port)

	lis, err := net.Listen("tcp", intf)
	if err != nil {
		daemon.log.Errorf("Error listening: %v", err)
		return err
	}

	daemon.log.Noticef("Running matching engine")
	go daemon.matcher.Run()

	var server *grpc.Server
	if daemon.rpcKeys != nil {
		creds := credentials.NewServerTLSFromCert(daemon.rpcKeys)
		server = grpc.NewServer(grpc.Creds(creds))
	} else {
		server = grpc.NewServer()
	}

	svc := NewSplitTicketMatcherService(daemon.matcher, daemon.dcrd)
	pb.RegisterSplitTicketMatcherServiceServer(server, svc)

	daemon.log.Noticef("Listening on %s", intf)
	server.Serve(lis)

	return nil
}

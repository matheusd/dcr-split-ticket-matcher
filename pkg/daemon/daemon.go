package daemon

import (
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/pkg/errors"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/poolintegrator"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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

	net := &chaincfg.MainNetParams
	if cfg.TestNet {
		net = &chaincfg.TestNet2Params
	}

	if cfg.PoolFee < 0.1 {
		// A pool fee of 0 would create a dust output (and potentially be unable
		// to be reduced on a revocation). So let's just ignore that case for the
		// moment.
		return nil, errors.New("Cannot use pool fee less than 0.1%")
	}

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
	dcrd, err := connectToDecredNode(dcfg)
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
		var cert *tls.Certificate
		cert, err = util.LoadRPCKeyPair(cfg.KeyFile, cfg.CertFile)
		if err == util.ErrKeyPairCreated {
			d.log.Noticef("Generated key (%s) and cert (%s) files",
				cfg.KeyFile, cfg.CertFile)
		} else if err != nil {
			panic(err)
		}

		d.rpcKeys = cert
	}

	minAmount, err := dcrutil.NewAmount(cfg.MinAmount)
	if err != nil {
		panic(err)
	}
	d.log.Infof("Minimum participation amount %s", minAmount)
	d.log.Infof("Stopping when sdiff change is closer than %d blocks", cfg.StakeDiffChangeStopWindow)
	d.log.Infof("Maximum session time: %d seconds", cfg.MaxSessionDuration)
	if cfg.TestNet {
		d.log.Infof("Running on testnet")
	} else {
		d.log.Infof("Running on mainnet")
	}
	d.log.Infof("Publishing transactions: %v", cfg.PublishTransactions)
	d.log.Infof("Using pool fee of %.2f%%", cfg.PoolFee)

	var stakepooldIntegrator *poolintegrator.Client
	if (cfg.StakepooldIntegratorHost != "") && (cfg.StakepooldIntegratorCert != "") {
		stakepooldIntegrator, err = poolintegrator.NewClient(cfg.StakepooldIntegratorHost,
			cfg.StakepooldIntegratorCert)
		if err != nil {
			return nil, errors.Wrap(err, "error initializing sakepoold integrator client")
		}
	}

	var voteAddrValidator matcher.VoteAddressValidationProvider
	if cfg.ValidateVoteAddressOnWallet {
		voteAddrValidator = d.wallet
		d.log.Infof("Validating voter addresses on wallet")
	} else if stakepooldIntegrator != nil {
		voteAddrValidator = stakepooldIntegrator
		d.log.Infof("Using stakepoold integrator to validate vote addresses")
	} else {
		voteAddrValidator = matcher.InsecurePoolAddressesValidator{}
		d.log.Infof("Not validating voter addresses")
	}

	var poolAddrValidator matcher.PoolAddressValidationProvider
	if cfg.PoolSubsidyWalletMasterPub != "" {
		poolAddrValidator, err = util.NewMasterPubPoolAddrValidator(cfg.PoolSubsidyWalletMasterPub, net)
		if err != nil {
			return nil, errors.Wrapf(err, "error deriving pool subsidy "+
				"addresses from masterpubkey")
		}
		d.log.Infof("Validating pool subsidy addresses with masterPubKey")
	} else if stakepooldIntegrator != nil {
		poolAddrValidator = stakepooldIntegrator
		d.log.Info("Using stakepoold integrator to validate pool subsidy addresses")
	} else {
		poolAddrValidator = matcher.InsecurePoolAddressesValidator{}
		d.log.Infof("Not validating pool subsidy addresses")
	}

	poolSigner, err := newPrivateKeySplitPoolSigner(cfg.SplitPoolSignKey, net)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding private key for split "+
			"pool signing")
	}
	d.log.Infof("Using address %s to move pool fee funds from split to ticket",
		poolSigner.address.EncodeAddress())

	d.log.Infof("Using keepalive timeout of %s / %s", cfg.KeepAliveTime,
		cfg.KeepAliveTimeout)

	mcfg := &matcher.Config{
		LogLevel:                  cfg.LogLevel,
		MinAmount:                 uint64(minAmount),
		NetworkProvider:           d.dcrd,
		SignPoolSplitOutProvider:  poolSigner,
		VoteAddrValidator:         voteAddrValidator,
		PoolAddrValidator:         poolAddrValidator,
		ChainParams:               net,
		PoolFee:                   cfg.PoolFee,
		MaxSessionDuration:        cfg.MaxSessionDuration * time.Second,
		LogBackend:                logBackend,
		StakeDiffChangeStopWindow: cfg.StakeDiffChangeStopWindow,
		PublishTransactions:       cfg.PublishTransactions,
		SessionDataDir:            filepath.Join(cfg.DataDir, "sessions"),
	}
	d.matcher = matcher.NewMatcher(mcfg)

	return d, nil
}

// ListenAndServe connections for the daemon. Returns an error when done.
func (daemon *Daemon) ListenAndServe() error {
	if daemon.rpcKeys == nil {
		return fmt.Errorf("RPC TLS keys not specified")
	}

	intf := fmt.Sprintf(":%d", daemon.cfg.Port)

	lis, err := net.Listen("tcp", intf)
	if err != nil {
		daemon.log.Errorf("Error listening: %v", err)
		return err
	}

	daemon.log.Noticef("Running matching engine")
	go daemon.matcher.Run()

	keepAlive := keepalive.ServerParameters{
		Time:    daemon.cfg.KeepAliveTime,
		Timeout: daemon.cfg.KeepAliveTimeout,
	}
	keepAlivePolice := keepalive.EnforcementPolicy{
		MinTime: 1 * time.Minute,
		PermitWithoutStream: true,
	}

	creds := credentials.NewServerTLSFromCert(daemon.rpcKeys)
	server := grpc.NewServer(grpc.Creds(creds), grpc.KeepaliveParams(keepAlive))

	svc := NewSplitTicketMatcherService(daemon.matcher, daemon.dcrd,
		daemon.cfg.AllowPublicSession)
	pb.RegisterSplitTicketMatcherServiceServer(server, svc)

	daemon.log.Noticef("Listening on %s", intf)
	return server.Serve(lis)
}

package daemon

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/slog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/poolintegrator"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Daemon is the main instance of a running dcr split ticket matcher daemon
type Daemon struct {
	cfg          *Config
	log          slog.Logger
	matcher      *matcher.Matcher
	wallet       *WalletClient
	rpcKeys      *tls.Certificate
	dcrd         *decredNetwork
	grpcListener net.Listener
	waitlistSvc  *waitlistWebsocketService
}

// NewDaemon returns a new daemon instance and prepares it to listen to
// requests.
func NewDaemon(cfg *Config) (*Daemon, error) {

	chainParams := &chaincfg.MainNetParams
	if cfg.TestNet {
		chainParams = &chaincfg.TestNet3Params
	} else if cfg.SimNet {
		chainParams = &chaincfg.SimNetParams
	}

	if cfg.PoolFee < 0.1 {
		// A pool fee of 0 would create a dust output (and potentially be unable
		// to be reduced on a revocation). So let's just ignore that case for the
		// moment.
		return nil, errors.New("Cannot use pool fee less than 0.1%")
	}

	d := &Daemon{
		cfg: cfg,
		log: cfg.logger("DAEM"),
	}

	d.log.Criticalf("Starting dcrstmd version %s", version.String())

	dcfg := &decredNetworkConfig{
		Host:        cfg.DcrdHost,
		Pass:        cfg.DcrdPass,
		CertFile:    cfg.DcrdCert,
		User:        cfg.DcrdUser,
		Log:         cfg.logger("DCRD"),
		chainParams: chainParams,
	}
	dcrd, err := connectToDecredNode(dcfg)
	if err != nil {
		panic(err)
	}
	d.dcrd = dcrd

	dcrwcfg := &WalletConfig{
		Host:     cfg.DcrwHost,
		User:     cfg.DcrwUser,
		Pass:     cfg.DcrwPass,
		CertFile: cfg.DcrwCert,
		Log:      cfg.logger("WLLT"),
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
			d.log.Criticalf("Generated key (%s) and cert (%s) files",
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
	d.log.Infof("Maximum session time: %s", cfg.MaxSessionDuration.String())
	if cfg.TestNet {
		d.log.Infof("Running on testnet")
	} else if cfg.SimNet {
		d.log.Infof("Running on simnet")
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
		poolAddrValidator, err = util.NewMasterPubPoolAddrValidator(
			cfg.PoolSubsidyWalletMasterPub, chainParams)
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

	poolSigner, err := newPrivateKeySplitPoolSigner(cfg.SplitPoolSignKey,
		chainParams)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding private key for split "+
			"pool signing")
	}
	d.log.Infof("Using address %s to move pool fee funds from split to ticket",
		poolSigner.address.EncodeAddress())

	d.log.Infof("Using keepalive timeout of %s / %s", cfg.KeepAliveTime,
		cfg.KeepAliveTimeout)

	mcfg := &matcher.Config{
		MinAmount:                 uint64(minAmount),
		NetworkProvider:           d.dcrd,
		SignPoolSplitOutProvider:  poolSigner,
		VoteAddrValidator:         voteAddrValidator,
		PoolAddrValidator:         poolAddrValidator,
		ChainParams:               chainParams,
		PoolFee:                   cfg.PoolFee,
		MaxSessionDuration:        cfg.MaxSessionDuration,
		Log:                       cfg.logger("MTCH"),
		StakeDiffChangeStopWindow: cfg.StakeDiffChangeStopWindow,
		PublishTransactions:       cfg.PublishTransactions,
		SessionDataDir:            filepath.Join(cfg.DataDir, "sessions"),
	}
	d.matcher = matcher.NewMatcher(mcfg)

	if cfg.WaitingListWSBindAddr != "" {
		var wssvc *waitlistWebsocketService
		wssvc, err = newWaitlistWebsocketService(cfg.WaitingListWSBindAddr,
			d.matcher, cfg.logger("WLWS"))
		if err != nil {
			return nil, errors.Wrapf(err, "error starting waitlist "+
				"websocket service")
		}
		d.waitlistSvc = wssvc
		d.log.Criticalf("Waitlist WS service listening on %s",
			cfg.WaitingListWSBindAddr)
	} else {
		d.log.Info("Skipping start of websocket waiting list service")
	}

	intf := fmt.Sprintf(":%d", cfg.Port)
	lis, err := net.Listen("tcp", intf)
	if err != nil {
		return nil, errors.Wrapf(err, "error listening on interface %s", intf)
	}
	d.grpcListener = lis
	d.log.Criticalf("GRPC service listening on %s", intf)

	return d, nil
}

// Run the grpc server matcher and all associated connections as goroutines.
// Returns immediately with a nil error in case of success.
func (daemon *Daemon) Run(serverCtx context.Context) error {
	if daemon.rpcKeys == nil {
		return fmt.Errorf("RPC TLS keys not specified")
	}

	daemon.log.Criticalf("Running matching engine")
	go daemon.wallet.Run(serverCtx)
	go daemon.matcher.Run(serverCtx)
	go daemon.dcrd.run(serverCtx)

	if daemon.waitlistSvc != nil {
		go daemon.waitlistSvc.run(serverCtx, daemon.cfg.CertFile,
			daemon.cfg.KeyFile)
	}

	keepAlive := keepalive.ServerParameters{
		Time:    daemon.cfg.KeepAliveTime,
		Timeout: daemon.cfg.KeepAliveTimeout,
	}
	keepAlivePolicy := keepalive.EnforcementPolicy{
		MinTime:             1 * time.Minute,
		PermitWithoutStream: true,
	}

	creds := credentials.NewServerTLSFromCert(daemon.rpcKeys)
	server := grpc.NewServer(grpc.Creds(creds), grpc.KeepaliveParams(keepAlive),
		grpc.KeepaliveEnforcementPolicy(keepAlivePolicy))

	svc := NewSplitTicketMatcherService(daemon.matcher, daemon.dcrd,
		daemon.cfg.AllowPublicSession)
	pb.RegisterSplitTicketMatcherServiceServer(server, svc)

	daemon.log.Criticalf("Running daemon on pid %d", os.Getpid())

	go func() {
		<-serverCtx.Done()
		daemon.log.Infof("Server context done in daemon")
		server.Stop()
	}()

	go func() { server.Serve(daemon.grpcListener) }()

	return nil
}

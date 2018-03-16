package daemon

import (
	"fmt"
	"net"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

// Daemon is the main instance of a running dcr split ticket matcher daemon
type Daemon struct {
	cfg     *Config
	log     *logging.Logger
	matcher *matcher.Matcher
}

// NewDaemon returns a new daemon instance and prepares it to listen to
// requests.
func NewDaemon(cfg *Config) (*Daemon, error) {
	d := &Daemon{
		cfg: cfg,
		log: logging.MustGetLogger("dcr-split-ticket-matcher"),
	}

	util.SetLoggerBackend(true, "", "", cfg.LogLevel, d.log)

	net := &chaincfg.TestNet2Params

	//voteAddr, err := dcrutil.DecodeAddress("TsbDGCuLMuVpZeP2HwgUKFm8ucGTCxmbatA") // online voting wallet
	voteAddr, err := dcrutil.DecodeAddress("Tse7wS9P6V5JCyy4pNEZxW3D39935MB83jX") // non-online wallet
	if err != nil {
		panic(err)
	}

	// voteProvider := &util.FixedVoteAddressProvider{Address: voteAddr}
	voteProvider, err := util.NewScriptVoteAddressProvider(voteAddr, 1440, net)
	if err != nil {
		panic(err)
	}

	d.log.Infof("Using voting address %s", voteProvider.VotingAddress().String())

	mcfg := &matcher.Config{
		LogLevel:                 cfg.LogLevel,
		MinAmount:                2,
		MaxOnlineParticipants:    10,
		PriceProvider:            &util.FixedTicketPriceProvider{TicketPrice: 51.938 * 1e8, BlockHeight: 260000},
		VoteAddrProvider:         voteProvider,
		SignPoolSplitOutProvider: util.NewBrokenInsecureSplitOutSigner(net),
		ChainParams:              net,
		PoolFee:                  7.5,
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

	server := grpc.NewServer()
	pb.RegisterSplitTicketMatcherServiceServer(server, NewSplitTicketMatcherService(daemon.matcher))

	daemon.log.Noticef("Listening on %s", intf)
	server.Serve(lis)

	return nil
}

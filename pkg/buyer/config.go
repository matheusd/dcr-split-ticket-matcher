package buyer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ansel1/merry"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultDataDir     = dcrutil.AppDataDir("splitticketbuyer", false)
	defaultCfgFilePath = filepath.Join(defaultDataDir, "splitticketbuyer.conf")

	ErrMisingConfigParameter = merry.New("Missing config parameter")
	ErrEmptyPassword         = merry.New("Empty Password Provided")
	ErrHelpRequested         = merry.New("Help Requested")
	ErrArgParsingError       = merry.New("Argument parsing error")
)

type BuyerConfig struct {
	ConfigFile     string  `short:"C" long:"configfile" description:"Path to config file"`
	WalletCertFile string  `long:"wallet.certfile" description:"Path Wallet rpc.cert file"`
	WalletHost     string  `long:"wallet.host" description:"Address of the wallet"`
	Pass           string  `short:"P" long:"pass" description:"Passphrase to unlock the wallet"`
	MatcherHost    string  `long:"matcher.host" description:"Address of the matcher host"`
	MaxAmount      float64 `long:"maxamount" description:"Maximum participation amount"`
	SourceAccount  uint32  `long:"sourceaccount" description:"Source account of funds for purchase"`
	SStxFeeLimits  uint16  `long:"sstxfeelimits" description:"Fee limit allowance for sstx purchases"`
	VoteAddress    string  `long:"voteaddress" description:"Voting address of the stakepool"`
	PoolAddress    string  `long:"pooladdress" description:"Pool fee address of the stakepool"`
	TestNet        bool    `long:"testnet" description:"Whether this is connecting to a testnet wallet/matcher service"`

	Passphrase  []byte
	ChainParams *chaincfg.Params
}

func LoadConfig() (*BuyerConfig, error) {
	var err error

	preCfg := &BuyerConfig{
		ConfigFile: defaultCfgFilePath,
	}
	preParser := flags.NewParser(preCfg, flags.Default)
	_, err = preParser.Parse()
	if err != nil {
		e, ok := err.(*flags.Error)
		if ok && e.Type == flags.ErrHelp {
			return nil, ErrHelpRequested
		}
		preParser.WriteHelp(os.Stderr)
		return nil, ErrArgParsingError.Appendf(": %v", err)
	}

	configFilePath := preCfg.ConfigFile

	cfg := &BuyerConfig{
		MatcherHost:    "localhost:8475",
		SStxFeeLimits:  uint16(0x5800),
		ChainParams:    &chaincfg.MainNetParams,
		WalletCertFile: filepath.Join(dcrutil.AppDataDir("dcrwallet", false), "rpc.cert"),
		SourceAccount:  0,
	}

	parser := flags.NewParser(cfg, flags.Default)
	err = flags.NewIniParser(parser).ParseFile(configFilePath)
	if err != nil {
		return nil, err
	}

	_, err = parser.Parse()
	if err != nil {
		return nil, err
	}

	if cfg.TestNet {
		cfg.ChainParams = &chaincfg.TestNet2Params
	}

	if cfg.VoteAddress == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "VoteAddress")
	}

	if cfg.PoolAddress == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "PoolAddress")
	}

	if cfg.MaxAmount <= 0 {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "MaxAmount")
	}

	if cfg.WalletHost == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "WalletHost")
	}

	if cfg.Pass == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "WalletPass")
	} else if cfg.Pass == "-" {
		pass, err := passFromStdin()
		if err != nil {
			return nil, err
		}
		cfg.Pass = pass
	}

	cfg.Passphrase = []byte(cfg.Pass)
	cfg.Pass = ""

	return cfg, nil

}

func passFromStdin() (string, error) {

	fmt.Printf("Please enter your wallet's private passphrase: ")
	bio := bufio.NewReader(os.Stdin)
	param, err := bio.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && len(param) == 0 {
		return "", ErrEmptyPassword
	}
	return strings.TrimRight(param, "\r\n"), nil
}

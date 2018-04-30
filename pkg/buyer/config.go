package buyer

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ansel1/merry"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrwallet/netparams"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultDataDir     = dcrutil.AppDataDir("splitticketbuyer", false)
	defaultCfgFilePath = filepath.Join(defaultDataDir, "splitticketbuyer.conf")

	DefaultConfig = &BuyerConfig{
		ConfigFile:    defaultCfgFilePath,
		MatcherHost:   "localhost:8475",
		SStxFeeLimits: uint16(0x5800),
		ChainParams:   &chaincfg.MainNetParams,
		// WalletCertFile:  filepath.Join(dcrutil.AppDataDir("dcrwallet", false), "rpc.cert"),
		SourceAccount:   0,
		MaxTime:         30,
		MaxWaitTime:     120,
		DataDir:         defaultDataDir,
		MatcherCertFile: filepath.Join(defaultDataDir, "matcher.cert"),
		Pass:            "-",
	}

	ErrMisingConfigParameter = merry.New("Missing config parameter")
	ErrEmptyPassword         = merry.New("Empty Password Provided")
	ErrHelpRequested         = merry.New("Help Requested")
	ErrArgParsingError       = merry.New("Argument parsing error")
)

type BuyerConfig struct {
	ConfigFile      string  `short:"C" long:"configfile" description:"Path to config file"`
	WalletCertFile  string  `long:"wallet.certfile" description:"Path Wallet rpc.cert file"`
	WalletHost      string  `long:"wallet.host" description:"Address of the wallet. Use 127.0.0.1:0 to try and automatically locate the running wallet on localhost."`
	Pass            string  `short:"P" long:"pass" description:"Passphrase to unlock the wallet"`
	MatcherHost     string  `long:"matcher.host" description:"Address of the matcher host"`
	MaxAmount       float64 `long:"maxamount" description:"Maximum participation amount"`
	SourceAccount   uint32  `long:"sourceaccount" description:"Source account of funds for purchase"`
	SStxFeeLimits   uint16  `long:"sstxfeelimits" description:"Fee limit allowance for sstx purchases"`
	VoteAddress     string  `long:"voteaddress" description:"Voting address of the stakepool"`
	PoolAddress     string  `long:"pooladdress" description:"Pool fee address of the stakepool"`
	TestNet         bool    `long:"testnet" description:"Whether this is connecting to a testnet wallet/matcher service"`
	MaxTime         int     `long:"maxtime" description:"Maximum amount of time (in seconds) to wait for the completion of the split buy"`
	MaxWaitTime     int     `long:"maxwaittime" description:"Maximum amount of time (in seconds) to wait until a new split ticket session is initiated"`
	DataDir         string  `long:"datadir" description:"Directory where session data files are stored"`
	MatcherCertFile string  `long:"matchercertfile" description:"Location of the certificate file for connecting to the grpc matcher service"`
	SessionName     string  `long:"sessionname" description:"Name of the session to connect to. Leave blank to connect to the public matching session."`
	DcrdHost        string  `long:"dcrdhost" description:"Host of the dcrd daemon"`
	DcrdUser        string  `long:"dcrduser" description:"Username of the dcrd daemon"`
	DcrdPass        string  `long:"dcrpass" description:"Password of the dcrd daemon"`
	DcrdCert        string  `long:"dcrdcert" description:"Location of the certificate for the dcrd daemon"`

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
		MatcherHost:     "localhost:8475",
		SStxFeeLimits:   uint16(0x5800),
		ChainParams:     &chaincfg.MainNetParams,
		WalletCertFile:  filepath.Join(dcrutil.AppDataDir("dcrwallet", false), "rpc.cert"),
		SourceAccount:   0,
		MaxTime:         30,
		MaxWaitTime:     120,
		DataDir:         defaultDataDir,
		MatcherCertFile: filepath.Join(defaultDataDir, "matcher.cert"),
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

	if cfg.DcrdHost == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "DcrdHost")
	}

	if cfg.DcrdUser == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "DcrdUser")
	}

	if cfg.DcrdPass == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "DcrdPass")
	}

	if cfg.DcrdCert == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "DcrdCertfile")
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

func (cfg *BuyerConfig) networkCfg() *decredNetworkConfig {
	return &decredNetworkConfig{
		Host:     cfg.DcrdHost,
		User:     cfg.DcrdUser,
		Pass:     cfg.DcrdPass,
		CertFile: cfg.DcrdCert,
	}
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

// DefaultConfigFileExists checks whether the default config file for the buyer
// exists for the current user
func DefaultConfigFileExists() bool {
	_, err := os.Stat(DefaultConfig.ConfigFile)
	return err == nil
}

// InitConfigFromDcrwallet inits a config based on whatever is stored
// in the dcrwallet.conf file. This replaces any existing configuration.
func InitConfigFromDcrwallet() error {
	dcrwalletDir := dcrutil.AppDataDir("dcrwallet", false)
	dcrwalletCfgFile := filepath.Join(dcrwalletDir, "dcrwallet.conf")

	dcrdDir := dcrutil.AppDataDir("dcrd", false)

	ioutil.WriteFile(defaultCfgFilePath, []byte(defaultConfigFileContents), 0644)

	file, err := ini.InsensitiveLoad(dcrwalletCfgFile)
	if err != nil {
		return errors.Wrapf(err, "error opening dcrwallet.conf file")
	}

	section, err := file.GetSection("Application Options")
	if err != nil {
		return errors.Wrapf(err, "error getting main section of dcrwallet conf")
	}

	dst, err := ini.Load(defaultCfgFilePath)
	if err != nil {
		return errors.Wrapf(err, "error initializing default config data")
	}

	dstSection, err := dst.GetSection("Application Options")
	if err != nil {
		return errors.Wrapf(err, "error getting dst section")
	}

	update := func(srcKey, dstKey, def string) {
		srcVal := def
		if section.Haskey(srcKey) {
			srcVal = section.Key(srcKey).Value()
		}
		if srcVal != "" {
			dstSection.Key(dstKey).SetValue(srcVal)
		}
	}

	testnetRpcCert := filepath.Join(defaultDataDir, "testnet-rpc.cert")
	mainnetRpcCert := filepath.Join(defaultDataDir, "mainnet-rpc.cert")
	ioutil.WriteFile(testnetRpcCert, []byte(testnetMatcherRpcCert), 0644)
	ioutil.WriteFile(mainnetRpcCert, []byte(mainnetMatcherRpcCert), 0644)

	activeNet := netparams.MainNetParams
	isTestNet := section.Key("testnet").Value() == "1"
	matcherHost := "mainnet-split-tickets.matheusd.com:8475"
	matcherCert := mainnetRpcCert
	if isTestNet {
		activeNet = netparams.TestNet2Params
		matcherHost = "testnet-split-tickets.matheusd.com:18475"
		matcherCert = testnetRpcCert
	}

	dstSection.Key("MatcherHost").SetValue(matcherHost)
	dstSection.Key("MatcherCertFile").SetValue(matcherCert)

	update("rpcconnect", "DcrdHost", "localhost:"+activeNet.JSONRPCClientPort)
	update("cafile", "DcrdCert", filepath.Join(dcrdDir, "rpc.cert"))
	update("username", "DcrdUser", "")
	update("dcrdusername", "DcrdUser", "")
	update("password", "DcrdPass", "")
	update("dcrdpassword", "DcrdPass", "")
	update("pass", "Pass", "")
	update("testnet", "TestNet", "0")
	update("rpccert", "WalletCertFile", filepath.Join(dcrwalletDir, "rpc.cert"))

	err = dst.SaveTo(defaultCfgFilePath)
	if err != nil {
		return errors.Wrapf(err, "error saving initialized cfg file")
	}

	return nil
}

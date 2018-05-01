package buyer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ansel1/merry"
	"github.com/go-ini/ini"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
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
		SStxFeeLimits: uint16(0x5800),
		ChainParams:   &chaincfg.MainNetParams,
		SourceAccount: 0,
		MaxTime:       30,
		MaxWaitTime:   120,
		DataDir:       defaultDataDir,
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

	if cfg.MaxAmount < 0 {
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
	} else {
		cfg.DcrdCert = util.CleanAndExpandPath(cfg.DcrdCert)
	}

	if cfg.WalletCertFile == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "WalletCertFile")
	} else {
		cfg.WalletCertFile = util.CleanAndExpandPath(cfg.WalletCertFile)
	}

	if cfg.MatcherCertFile == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "MatcherCertFile")
	} else {
		cfg.MatcherCertFile = util.CleanAndExpandPath(cfg.MatcherCertFile)
	}

	if cfg.MatcherHost == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "MatcherHost")
	}

	if cfg.DataDir == "" {
		return nil, merry.WithMessagef(ErrMisingConfigParameter, "Missing config parameter: %s", "DataDir")
	} else {
		cfg.DataDir = util.CleanAndExpandPath(cfg.DataDir)
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

// InitDefaultConfig replaces the current config (if it exists) with the
// factory default config.
func InitDefaultConfig() error {
	err := os.MkdirAll(defaultDataDir, 0755)
	if err != nil {
		return errors.Wrapf(err, "error creating data dir %s", defaultDataDir)
	}
	ioutil.WriteFile(defaultCfgFilePath, []byte(defaultConfigFileContents), 0644)

	return nil
}

// InitConfigFromDcrwallet inits a config based on whatever is stored
// in the dcrwallet.conf file. This replaces any existing configuration.
func InitConfigFromDcrwallet() error {
	dcrwalletDir := dcrutil.AppDataDir("dcrwallet", false)
	dcrwalletCfgFile := filepath.Join(dcrwalletDir, "dcrwallet.conf")
	dcrdDir := dcrutil.AppDataDir("dcrd", false)

	err := InitDefaultConfig()
	if err != nil {
		return err
	}

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

func decreditonConfigDir() string {
	var homeDir string

	usr, err := user.Current()
	if err == nil {
		homeDir = usr.HomeDir
	}
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}

	switch runtime.GOOS {
	case "windows":
		// Windows XP and before didn't have a LOCALAPPDATA, so fallback
		// to regular APPDATA when LOCALAPPDATA is not set.
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			appData = os.Getenv("APPDATA")
		}

		if appData != "" {
			return filepath.Join(appData, "Decrediton")
		}

	case "darwin":
		if homeDir != "" {
			return filepath.Join(homeDir, "Library",
				"Application Support", "decrediton")
		}

	default:
		if homeDir != "" {
			return filepath.Join(homeDir, ".config/decrediton")
		}
	}

	return ""
}

type decreditonRemoteCredentials struct {
	RPCUser     string `json:"rpc_user"`
	RPCPassword string `json:"rpc_password"`
	RPCCert     string `json:"rpc_cert"`
	RPCHost     string `json:"rpc_host"`
	RPCPort     string `json:"rpc_port"`
}

type decreditonStakepool struct {
	Host          string `json:"Host"`
	Network       string `json:"Network"`
	PoolAddress   string `json:"PoolAddress"`
	TicketAddress string `json:"TicketAddress"`
}

type decreditonWalletConfig struct {
	StakePools        []*decreditonStakepool       `json:"stakepools"`
	RemoteCredentials *decreditonRemoteCredentials `json:"remote_credentials"`
}

// InitConfigFromDecrediton replaces the config with the default one plus
// all available entries read from an installed decrediton. Requires a
// decrediton version >= 1.2.0.
//
// The data for the given walletName (of the currently selected network)
// will be used.
func InitConfigFromDecrediton(walletName string) error {
	decreditonDir := decreditonConfigDir()
	decreditonGlobalCfg := filepath.Join(decreditonDir, "config.json")
	dcrdDir := dcrutil.AppDataDir("dcrd", false)

	globalCfgJson, err := ioutil.ReadFile(decreditonGlobalCfg)
	if err != nil {
		return errors.Wrapf(err, "error reading global config.json from "+
			"decrediton at %s", decreditonGlobalCfg)
	}

	globalCfg := &struct {
		Network             string `json:"network"`
		DaemonStartAdvanced bool   `json:"daemon_start_advanced"`
	}{}

	err = json.Unmarshal(globalCfgJson, globalCfg)
	if err != nil {
		return errors.Wrapf(err, "error unmarshaling decrediton config.json")
	}

	if (globalCfg.Network != "testnet") && (globalCfg.Network != "mainnet") {
		return errors.Errorf("unrecognized network in decrediton "+
			"config.json (%s)", globalCfg.Network)
	}

	walletsDir := filepath.Join(decreditonDir, "wallets", globalCfg.Network)
	walletsDirContent, err := ioutil.ReadDir(walletsDir)
	if err != nil {
		return errors.Wrapf(err, "error listing content of wallets dir %s",
			walletsDir)
	}

	hasWanted := false
	for _, f := range walletsDirContent {
		if !f.IsDir() {
			continue
		}
		hasWanted = hasWanted || f.Name() == walletName
	}

	if !hasWanted {
		return errors.Errorf("desired wallet (%s) not found in decrediton "+
			"wallets dir %s", walletName, walletsDir)
	}

	walletDir := filepath.Join(walletsDir, walletName)

	walletCfgJsonFname := filepath.Join(walletDir, "config.json")
	walletCfgJson, err := ioutil.ReadFile(walletCfgJsonFname)
	if err != nil {
		return errors.Wrapf(err, "error reading wallet config.json from "+
			"decrediton at %s", walletCfgJsonFname)
	}

	walletCfg := &decreditonWalletConfig{}

	err = json.Unmarshal(walletCfgJson, walletCfg)
	if err != nil {
		return errors.Wrapf(err, "error unmarshaling decrediton config.json")
	}

	err = InitDefaultConfig()
	if err != nil {
		return err
	}

	dst, err := ini.Load(defaultCfgFilePath)
	if err != nil {
		return errors.Wrapf(err, "error initializing default config data")
	}

	dstSection, err := dst.GetSection("Application Options")
	if err != nil {
		return errors.Wrapf(err, "error getting dst section")
	}

	testnetRpcCert := filepath.Join(defaultDataDir, "testnet-rpc.cert")
	mainnetRpcCert := filepath.Join(defaultDataDir, "mainnet-rpc.cert")
	ioutil.WriteFile(testnetRpcCert, []byte(testnetMatcherRpcCert), 0644)
	ioutil.WriteFile(mainnetRpcCert, []byte(mainnetMatcherRpcCert), 0644)

	activeNet := netparams.MainNetParams
	isTestNet := globalCfg.Network == "testnet"
	matcherHost := "mainnet-split-tickets.matheusd.com:8475"
	matcherCert := mainnetRpcCert
	testnetVal := "0"
	if isTestNet {
		activeNet = netparams.TestNet2Params
		matcherHost = "testnet-split-tickets.matheusd.com:18475"
		matcherCert = testnetRpcCert
		testnetVal = "1"
	}

	dstSection.Key("MatcherHost").SetValue(matcherHost)
	dstSection.Key("MatcherCertFile").SetValue(matcherCert)
	dstSection.Key("TestNet").SetValue(testnetVal)
	dstSection.Key("WalletCertFile").SetValue(filepath.Join(walletDir, "rpc.cert"))

	if globalCfg.DaemonStartAdvanced {
		creds := walletCfg.RemoteCredentials
		dstSection.Key("DcrdHost").SetValue(creds.RPCHost + ":" + creds.RPCPort)
		dstSection.Key("DcrdUser").SetValue(creds.RPCUser)
		dstSection.Key("DcrdPass").SetValue(creds.RPCPassword)
		dstSection.Key("DcrdCert").SetValue(creds.RPCCert)
	} else {
		dstSection.Key("DcrdHost").SetValue("127.0.0.1:" + activeNet.JSONRPCClientPort)
		dstSection.Key("DcrdUser").SetValue("USER")
		dstSection.Key("DcrdPass").SetValue("PASSWORD")
		dstSection.Key("DcrdCert").SetValue(filepath.Join(dcrdDir, "rpc.cert"))
	}

	for _, pool := range walletCfg.StakePools {
		if (pool.PoolAddress != "") &&
			(pool.TicketAddress != "") &&
			(pool.Network == globalCfg.Network) {

			dstSection.Key("VoteAddress").SetValue(pool.TicketAddress)
			dstSection.Key("PoolAddress").SetValue(pool.PoolAddress)
			break
		}
	}

	err = dst.SaveTo(defaultCfgFilePath)
	if err != nil {
		return errors.Wrapf(err, "error saving initialized cfg file")
	}

	return nil
}

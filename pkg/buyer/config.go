package buyer

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/pkg/errors"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrwallet/netparams"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultDataDir     = dcrutil.AppDataDir("splitticketbuyer", false)
	defaultCfgFilePath = filepath.Join(defaultDataDir, "splitticketbuyer.conf")

	// ErrHelpRequested is the error returned when the help command line option
	// was requested
	ErrHelpRequested = fmt.Errorf("help requested")

	// ErrVersionRequested is the error returned when the version command line
	// option was requested
	ErrVersionRequested = fmt.Errorf("version requested")

	// ErrEmptyPassword is the error returned when an empty password has been
	// provided in the config
	ErrEmptyPassword = fmt.Errorf("empty password")
)

// Config stores the configuration needed to perform a single ticket split
// session as a buyer.
type Config struct {
	ConfigFile           string  `short:"C" long:"configfile" description:"Path to config file"`
	WalletCertFile       string  `long:"wallet.certfile" description:"Path Wallet rpc.cert file"`
	WalletHost           string  `long:"wallet.host" description:"Address of the wallet. Use 127.0.0.1:0 to try and automatically locate the running wallet on localhost."`
	Pass                 string  `short:"P" long:"pass" description:"Passphrase to unlock the wallet"`
	MatcherHost          string  `long:"matcher.host" description:"Address of the matcher host"`
	MaxAmount            float64 `long:"maxamount" description:"Maximum participation amount"`
	SourceAccount        uint32  `long:"sourceaccount" description:"Source account of funds for purchase"`
	SStxFeeLimits        uint16  `long:"sstxfeelimits" description:"Fee limit allowance for sstx purchases"`
	VoteAddress          string  `long:"voteaddress" description:"Voting address of the stakepool"`
	PoolAddress          string  `long:"pooladdress" description:"Pool fee address of the stakepool"`
	PoolFeeRate          float64 `long:"poolfeerate" description:"Pool fee rate (percentage) that the given pool has advertised as using"`
	TestNet              bool    `long:"testnet" description:"Whether this is connecting to a testnet wallet/matcher service"`
	MaxTime              int     `long:"maxtime" description:"Maximum amount of time (in seconds) to wait for the completion of the split buy"`
	MaxWaitTime          int     `long:"maxwaittime" description:"Maximum amount of time (in seconds) to wait until a new split ticket session is initiated"`
	DataDir              string  `long:"datadir" description:"Directory where session data files are stored"`
	MatcherCertFile      string  `long:"matchercertfile" description:"Location of the certificate file for connecting to the grpc matcher service"`
	SessionName          string  `long:"sessionname" description:"Name of the session to connect to. Leave blank to connect to the public matching session."`
	DcrdHost             string  `long:"dcrdhost" description:"Host of the dcrd daemon"`
	DcrdUser             string  `long:"dcrduser" description:"Username of the dcrd daemon"`
	DcrdPass             string  `long:"dcrpass" description:"Password of the dcrd daemon"`
	DcrdCert             string  `long:"dcrdcert" description:"Location of the certificate for the dcrd daemon"`
	SkipWaitPublishedTxs bool    `long:"skipwaitpublishedtxs" description:"If specified, the session ends immediately after the last step, without waiting for the matcher to publish the transactions."`
	ShowVersion          bool    `long:"version" description:"Show version and quit"`

	Passphrase  []byte
	ChainParams *chaincfg.Params
}

// ReadPassphrase reads the passphrase from stdin (if needed), fills the
// PassPhrase field and clears the Pass field
func (cfg *Config) ReadPassphrase() error {
	if cfg.Pass == "" {
		return ErrEmptyPassword
	} else if cfg.Pass == "-" {
		pass, err := passFromStdin()
		if err != nil {
			return errors.Wrapf(err, "error reading passphrase from stdin")
		}
		cfg.Passphrase = []byte(pass)
	} else {
		cfg.Passphrase = []byte(cfg.Pass)
		cfg.Pass = ""
	}

	return nil
}

// Validate checks whether the current config has enough information for
// participation into a split ticket buying session
func (cfg *Config) Validate() error {

	// just a syntatic sugar to reduce typed chars
	missing := func(pmt string) missingConfigParameterError {
		return missingConfigParameterError(pmt)
	}

	if cfg.VoteAddress == "" {
		return missing("VoteAddress")
	}

	if cfg.PoolAddress == "" {
		return missing("PoolAddress")
	}

	if cfg.MaxAmount < 0 {
		return missing("MaxAmount")
	}

	if cfg.WalletHost == "" {
		return missing("WalletHost")
	}

	if cfg.DcrdHost == "" {
		return missing("DcrdHost")
	}

	if cfg.DcrdUser == "" {
		return missing("DcrdUser")
	}

	if cfg.DcrdPass == "" {
		return missing("DcrdPass")
	}

	if cfg.DcrdCert == "" {
		return missing("DcrdCertfile")
	}

	if cfg.WalletCertFile == "" {
		return missing("WalletCertFile")
	}

	if cfg.MatcherHost == "" {
		return missing("MatcherHost")
	}

	if cfg.DataDir == "" {
		return missing("DataDir")
	}

	return nil
}

type missingConfigParameterError string

func (pmt missingConfigParameterError) Error() string {
	return fmt.Sprintf("missing config parameter %s", string(pmt))
}

// LoadConfig parses command line arguments, the config file (either the
// default one or the one specificed with the -C option), merges the settings
// and returns a new BuyerConfig object for use in a split ticket buying session
func LoadConfig() (*Config, error) {
	var err error

	preCfg := &Config{
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
		return nil, errors.Wrapf(err, "error parsing cmdline arg")
	}

	if preCfg.ShowVersion {
		return nil, ErrVersionRequested
	}

	configFilePath := preCfg.ConfigFile

	cfg := &Config{
		ConfigFile:           configFilePath,
		SStxFeeLimits:        uint16(0x5800),
		ChainParams:          &chaincfg.MainNetParams,
		SourceAccount:        0,
		MaxTime:              30,
		MaxWaitTime:          0,
		DataDir:              defaultDataDir,
		SkipWaitPublishedTxs: false,
		PoolFeeRate:          5.0,
	}

	parser := flags.NewParser(cfg, flags.Default)
	err = flags.NewIniParser(parser).ParseFile(configFilePath)
	if err != nil {
		return nil, err
	}

	_, err = parser.Parse()
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing arguments")
	}

	if cfg.TestNet {
		cfg.ChainParams = &chaincfg.TestNet2Params
		if cfg.PoolFeeRate == 5.0 {
			cfg.PoolFeeRate = 7.5
		}
	}

	if cfg.DcrdCert != "" {
		cfg.DcrdCert = util.CleanAndExpandPath(cfg.DcrdCert)
	}

	if cfg.WalletCertFile != "" {
		cfg.WalletCertFile = util.CleanAndExpandPath(cfg.WalletCertFile)
	}

	if cfg.MatcherCertFile != "" {
		cfg.MatcherCertFile = util.CleanAndExpandPath(cfg.MatcherCertFile)
	}

	if cfg.DataDir != "" {
		cfg.DataDir = util.CleanAndExpandPath(cfg.DataDir)
	}

	return cfg, nil
}

func (cfg *Config) networkCfg() *decredNetworkConfig {
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
	_, err := os.Stat(defaultCfgFilePath)
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

	activeNet := netparams.MainNetParams
	isTestNet := section.Key("testnet").Value() == "1"
	matcherHost := "mainnet-split-tickets.matheusd.com:8475"
	if isTestNet {
		activeNet = netparams.TestNet2Params
		matcherHost = "testnet-split-tickets.matheusd.com:18475"
	}

	dstSection.Key("MatcherHost").SetValue(matcherHost)

	update("rpcconnect", "DcrdHost", "localhost:"+activeNet.JSONRPCClientPort)
	update("cafile", "DcrdCert", filepath.Join(dcrdDir, "rpc.cert"))
	update("username", "DcrdUser", "")
	update("dcrdusername", "DcrdUser", "")
	update("password", "DcrdPass", "")
	update("dcrdpassword", "DcrdPass", "")
	update("pass", "Pass", "")
	update("testnet", "TestNet", "0")
	update("rpccert", "WalletCertFile", filepath.Join(dcrwalletDir, "rpc.cert"))
	update("pooladdress", "PoolAddress", "")
	update("ticketaddress", "VoteAddress", "")

	err = dst.SaveTo(defaultCfgFilePath)
	if err != nil {
		return errors.Wrapf(err, "error saving initialized cfg file")
	}

	return nil
}

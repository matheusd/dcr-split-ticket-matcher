package poolintegrator

import (
	"os"
	"path/filepath"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/slog"
	flags "github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
)

// Config holds information on how to configure the voting pool integrator daemon.
type Config struct {
	Port         int `long:"port" description:"Port to run the service on"`
	LogLevel     slog.Level
	LogLevelName string `long:"loglevel" description:"Log Level (CRITICAL, ERROR, WARNING, NOTICE, INFO, DEBUG)"`
	LogDir       string `long:"logdir" description:"Log directory. Specify to save log messages to a file"`
	KeyFile      string `long:"keyfile" description:"Location of the rpc.key file (private key for the TLS certificate)."`
	CertFile     string `long:"certfile" description:"Location of the rpc.cert file (TLS certificate)."`
	ShowVersion  bool   `long:"version" description:"Show version and quit"`

	TestNet bool `long:"testnet" description:"Whether to run on testnet"`

	StakepooldConfigFile string `short:"C" long:"StakepooldConfigFile" description:"Path to stakepoold config file. Use an empty file path to run the integrator manually."`

	DcrwHost string `long:"dcrwhost" description:"Address of the dcrwallet daemon"`
	DcrwUser string `long:"dcrwuser" description:"Username of the rpc connection to dcrwallet"`
	DcrwPass string `long:"dcrwpass" description:"Password of the rpc connection to dcrwallet"`
	DcrwCert string `long:"dcrwcert" descript}ion:"Location of the rpc.cert file of dcrwallet"`

	PoolSubsidyWalletMasterPub string `long:"poolsubsidywalletmasterpub" description:"MasterPubKey for deriving addresses where the pool fee is payed to. If empty, pool fee addresses are not validated. Append a :[index] to generate addresses up to the provided index (default: 10000)."`
}

// stakepooldConfig is the information read from stakepoold.conf file
type stakepooldConfig struct {
	WalletHost       string `long:"wallethost"`
	WalletCert       string `long:"walletcert"`
	WalletUser       string `long:"walletuser"`
	WalletPassword   string `long:"walletpassword"`
	ColdWalletExtPub string `long:"coldwalletextpub"`
	TestNet          bool   `long:"testnet"`
}

// LoadConfig loads configuration for a pool integrator daemon from the
// filesystem and passed command line args
func LoadConfig() (*Config, error) {

	var err error

	cfg := &Config{
		Port:         DefaultPort,
		LogLevelName: "INFO",

		KeyFile:  filepath.Join(defaultDataDir, "rpc.key"),
		CertFile: filepath.Join(defaultDataDir, "rpc.cert"),

		StakepooldConfigFile: filepath.Join(dcrutil.AppDataDir("stakepoold", false), "stakepoold.conf"),
	}

	parser := flags.NewParser(cfg, flags.Default)
	_, err = parser.Parse()
	if err != nil {
		e, ok := err.(*flags.Error)
		if ok && e.Type == flags.ErrHelp {
			return nil, ErrHelpRequested
		}
		parser.WriteHelp(os.Stderr)
		return nil, errors.Wrapf(err, "error parsing arguments")
	}

	if cfg.ShowVersion {
		return nil, ErrVersionRequested
	}

	if cfg.StakepooldConfigFile != "" {
		if _, err = os.Stat(cfg.StakepooldConfigFile); os.IsNotExist(err) {
			return nil, errors.Errorf("stakepoold config file does not exist: %s",
				cfg.StakepooldConfigFile)
		}

		err = loadConfigFromStakepoold(cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing config from "+
				"stakepoold.conf")
		}

		if err = hasFullConfig(cfg); err != nil {
			return nil, errors.Wrap(err, "missing config arguments after reading "+
				"config from stakepoold")
		}
	} else if err = hasFullConfig(cfg); err != nil {
		return nil, errors.Wrap(err, "missing config arguments when not reading "+
			"config from stakepoold")
	}

	logLvl, ok := slog.LevelFromString(cfg.LogLevelName)
	if !ok {
		return nil, errors.Errorf("Invalid log level name %s", cfg.LogLevelName)
	}
	cfg.LogLevel = logLvl

	return cfg, nil
}

func loadConfigFromStakepoold(cfg *Config) error {
	spCfg := new(stakepooldConfig)
	parser := flags.NewParser(spCfg, flags.IgnoreUnknown)
	err := flags.NewIniParser(parser).ParseFile(cfg.StakepooldConfigFile)
	if err != nil {
		return errors.Wrapf(err, "error parsing stakepoold ini file")
	}

	cfg.DcrwHost = spCfg.WalletHost
	cfg.DcrwCert = spCfg.WalletCert
	cfg.DcrwUser = spCfg.WalletUser
	cfg.DcrwPass = spCfg.WalletPassword
	cfg.PoolSubsidyWalletMasterPub = spCfg.ColdWalletExtPub
	cfg.TestNet = spCfg.TestNet

	return nil
}

func hasFullConfig(cfg *Config) error {
	if cfg.DcrwHost == "" {
		return errors.New("missing dcrwhost config")
	}
	if cfg.DcrwUser == "" {
		return errors.New("missing dcrwuser config")
	}
	if cfg.DcrwPass == "" {
		return errors.New("missing dcrwpass config")
	}
	if cfg.DcrwCert == "" {
		return errors.New("missing dcrwcert config")
	}
	if cfg.PoolSubsidyWalletMasterPub == "" {
		return errors.New("missing poolsubsidywalletmasterpub config")
	}

	return nil
}

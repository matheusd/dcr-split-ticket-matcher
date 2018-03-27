package daemon

import (
	"os"
	"path/filepath"

	flags "github.com/btcsuite/go-flags"
	"github.com/decred/dcrd/dcrutil"
	"github.com/op/go-logging"
)

// Config stores the config needed to run an instance of the dcr split ticket
// matcher daemon
type Config struct {
	ConfigFile string `short:"C" long:"configfile" description:"Path to config file"`

	Port     int `long:"port" description:"Port to run the service on"`
	LogLevel logging.Level
	LogDir   string
	KeyFile  string
	CertFile string

	DcrdHost string `long:"dcrdhost" description:"Address of the dcrd daemon"`
	DcrdUser string `long:"dcrduser" description:"Username of the rpc connection to dcrd"`
	DcrdPass string `long:"dcrdpass" description:"Password of the rpc connection to dcrd"`
	DcrdCert string `long:"dcrdcert" description:"Location of the rpc.cert file of dcrd"`

	DcrwHost string `long:"dcrwhost" description:"Address of the dcrwallet daemon"`
	DcrwUser string `long:"dcrwuser" description:"Username of the rpc connection to dcrwallet"`
	DcrwPass string `long:"dcrwpass" description:"Password of the rpc connection to dcrwallet"`
	DcrwCert string `long:"dcrwcert" description:"Location of the rpc.cert file of dcrwallet"`
}

var (
	defaultDataDir     = dcrutil.AppDataDir("dcrstmd", false)
	defaultCfgFilePath = filepath.Join(defaultDataDir, "dcrstmd.conf")
)

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
		return nil, ErrArgParsingError.Appendf(": %v", err)
	}

	configFilePath := preCfg.ConfigFile

	cfg := &Config{
		Port:     8475,
		LogLevel: logging.INFO,
		LogDir:   filepath.Join(defaultDataDir, "logs"),

		KeyFile:  filepath.Join(defaultDataDir, "rpc.key"),
		CertFile: filepath.Join(defaultDataDir, "rpc.cert"),

		DcrdHost: "localhost:19109",
		DcrdUser: "USER",
		DcrdPass: "PASSWORD",
		DcrdCert: filepath.Join(dcrutil.AppDataDir("dcrd", false), "rpc.cert"),

		DcrwHost: "localhost:19110",
		DcrwUser: "USER",
		DcrwPass: "PASSWORD",
		DcrwCert: filepath.Join(dcrutil.AppDataDir("dcrwallet", false), "rpc.cert"),
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

	return cfg, nil
}

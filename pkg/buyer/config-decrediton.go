package buyer

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrwallet/netparams"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

const (
	decreditonTestnet = "testnet"
	decreditonMainnet = "mainnet"
)

type decreditonGlobalConfig struct {
	Network             string                       `json:"network"`
	DaemonStartAdvanced bool                         `json:"daemon_start_advanced"`
	RemoteCredentials   *decreditonRemoteCredentials `json:"remote_credentials"`
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

func getDecreditonGlobalConfig() (*decreditonGlobalConfig, error) {
	decreditonDir := decreditonConfigDir()
	decreditonGlobalCfgFile := filepath.Join(decreditonDir, "config.json")

	globalCfgJSON, err := ioutil.ReadFile(decreditonGlobalCfgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading global config.json from "+
			"decrediton at %s", decreditonGlobalCfgFile)
	}

	globalCfg := &decreditonGlobalConfig{}

	err = json.Unmarshal(globalCfgJSON, globalCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling decrediton config.json")
	}

	if (globalCfg.Network != decreditonTestnet) && (globalCfg.Network != decreditonMainnet) {
		return nil, errors.Errorf("unrecognized network in decrediton "+
			"config.json (%s)", globalCfg.Network)
	}

	return globalCfg, nil

}

// ListDecreditonWallets returns the names of the available decrediton wallets.
// Returns nil if decrediton isn't installed or not initialized.
func ListDecreditonWallets() []string {
	decreditonDir := decreditonConfigDir()
	globalCfg, err := getDecreditonGlobalConfig()
	if err != nil {
		return nil
	}

	walletsDir := filepath.Join(decreditonDir, "wallets", globalCfg.Network)
	walletsDirContent, err := ioutil.ReadDir(walletsDir)
	if err != nil {
		return nil
	}

	walletDbDir := "mainnet"
	if globalCfg.Network == decreditonTestnet {
		walletDbDir = "testnet2"
	}

	var res []string

	for _, f := range walletsDirContent {
		if !f.IsDir() {
			continue
		}

		walletDbFname := filepath.Join(walletsDir, f.Name(), walletDbDir, "wallet.db")
		_, err := os.Stat(walletDbFname)
		if err == nil {
			res = append(res, f.Name())
		}
	}

	return res

}

// InitConfigFromDecrediton replaces the config with the default one plus
// all available entries read from an installed decrediton. Requires a
// decrediton version >= 1.2.0.
//
// The data for the given walletName (of the currently selected network)
// will be used.
func InitConfigFromDecrediton(walletName string) error {
	decreditonDir := decreditonConfigDir()
	dcrdDir := dcrutil.AppDataDir("dcrd", false)

	globalCfg, err := getDecreditonGlobalConfig()
	if err != nil {
		return err
	}

	walletsDir := filepath.Join(decreditonDir, "wallets", globalCfg.Network)
	availableWallets := ListDecreditonWallets()
	hasWanted := false
	for _, w := range availableWallets {
		if w == walletName {
			hasWanted = true
			break
		}
	}
	if !hasWanted {
		return errors.Errorf("desired wallet (%s) not found in decrediton "+
			"wallets dir %s", walletName, walletsDir)
	}

	walletDir := filepath.Join(walletsDir, walletName)

	walletCfgJSONFname := filepath.Join(walletDir, "config.json")
	walletCfgJSON, err := ioutil.ReadFile(walletCfgJSONFname)
	if err != nil {
		return errors.Wrapf(err, "error reading wallet config.json from "+
			"decrediton at %s", walletCfgJSONFname)
	}

	walletCfg := &decreditonWalletConfig{}

	err = json.Unmarshal(walletCfgJSON, walletCfg)
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

	testnetRPCCert := filepath.Join(defaultDataDir, "testnet-rpc.cert")
	mainnetRPCCert := filepath.Join(defaultDataDir, "mainnet-rpc.cert")
	ioutil.WriteFile(testnetRPCCert, []byte(testnetMatcherRPCCert), 0644)
	ioutil.WriteFile(mainnetRPCCert, []byte(mainnetMatcherRPCCert), 0644)

	activeNet := netparams.MainNetParams
	isTestNet := globalCfg.Network == decreditonTestnet
	matcherHost := "mainnet-split-tickets.matheusd.com:8475"
	matcherCert := mainnetRPCCert
	testnetVal := "0"
	if isTestNet {
		activeNet = netparams.TestNet2Params
		matcherHost = "testnet-split-tickets.matheusd.com:18475"
		matcherCert = testnetRPCCert
		testnetVal = "1"
	}

	dstSection.Key("MatcherHost").SetValue(matcherHost)
	dstSection.Key("MatcherCertFile").SetValue(matcherCert)
	dstSection.Key("TestNet").SetValue(testnetVal)
	dstSection.Key("WalletCertFile").SetValue(filepath.Join(walletDir, "rpc.cert"))

	var creds *decreditonRemoteCredentials
	if globalCfg.RemoteCredentials != nil {
		creds = globalCfg.RemoteCredentials
	} else if walletCfg.RemoteCredentials != nil {
		creds = walletCfg.RemoteCredentials
	}

	if globalCfg.DaemonStartAdvanced && creds != nil {
		dstSection.Key("DcrdHost").SetValue(creds.RPCHost + ":" + creds.RPCPort)
		dstSection.Key("DcrdUser").SetValue(creds.RPCUser)
		dstSection.Key("DcrdPass").SetValue(creds.RPCPassword)
		dstSection.Key("DcrdCert").SetValue(creds.RPCCert)
	} else {
		// decrediton first tries to load from ~/.dcrd/dcrd.conf
		dcrdConfFname := filepath.Join(dcrutil.AppDataDir("dcrd", false), "dcrd.conf")
		hasDcrdConf := false
		_, err = os.Stat(dcrdConfFname)
		if err == nil {
			hasDcrdConf = true
		} else {
			// if that doesn't exist, it tries to load from ~/.config/decrediton/dcrd.conf
			dcrdConfFname = filepath.Join(decreditonConfigDir(), "dcrd.conf")
			if _, err = os.Stat(dcrdConfFname); err == nil {
				hasDcrdConf = true
			}
		}

		if hasDcrdConf {
			var dcrdIni *ini.File
			var dcrdSection *ini.Section

			dcrdIni, err = ini.Load(dcrdConfFname)
			if err != nil {
				return errors.Wrapf(err, "error reading dcrd ini at %s", dcrdConfFname)
			}

			dcrdSection, err = dcrdIni.GetSection("Application Options")
			if err != nil {
				return errors.Wrapf(err, "error reading Application Options "+
					"section from dcrd")
			}

			dstSection.Key("DcrdUser").SetValue(dcrdSection.Key("rpcuser").String())
			dstSection.Key("DcrdPass").SetValue(dcrdSection.Key("rpcpass").String())
		}

		dstSection.Key("DcrdHost").SetValue("127.0.0.1:" + activeNet.JSONRPCClientPort)
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

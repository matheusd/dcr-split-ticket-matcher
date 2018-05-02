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

type decreditonGlobalConfig struct {
	Network             string `json:"network"`
	DaemonStartAdvanced bool   `json:"daemon_start_advanced"`
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

	globalCfgJson, err := ioutil.ReadFile(decreditonGlobalCfgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading global config.json from "+
			"decrediton at %s", decreditonGlobalCfgFile)
	}

	globalCfg := &decreditonGlobalConfig{}

	err = json.Unmarshal(globalCfgJson, globalCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling decrediton config.json")
	}

	if (globalCfg.Network != "testnet") && (globalCfg.Network != "mainnet") {
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
	if globalCfg.Network == "testnet" {
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

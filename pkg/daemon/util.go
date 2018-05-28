package daemon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/hdkeychain"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/udb"
	"github.com/pkg/errors"
)

// privateKeySplitPoolSigner fulfills the SignPoolSplitOutputProvider by storing
// a single secp256k1 private key and signing the pool fee always using it.
type privateKeySplitPoolSigner struct {
	privateKey chainec.PrivateKey
	address    dcrutil.Address
	net        *chaincfg.Params
}

// newPrivateKeySplitPoolSigner creates a new privateKeySplitPoolSigner given
// a secp256k1 private key wif.
func newPrivateKeySplitPoolSigner(privateKeyWif string, net *chaincfg.Params) (*privateKeySplitPoolSigner, error) {
	if privateKeyWif == "" {
		return nil, errors.Errorf("private key wif is empty")
	}

	wif, err := dcrutil.DecodeWIF(privateKeyWif)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding private key wif")
	}

	if wif.DSA() != chainec.ECTypeSecp256k1 {
		return nil, errors.Errorf("only a secp256k1 private key is acceptable")
	}

	privKey := wif.PrivKey
	x, y := privKey.Public()

	pubKey := secp256k1.NewPublicKey(x, y)
	pubKeyAddr, err := dcrutil.NewAddressSecpPubKey(pubKey.Serialize(), net)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating secpPubKeyAddr from pubKey")
	}

	pkhAddr := pubKeyAddr.AddressPubKeyHash()

	return &privateKeySplitPoolSigner{
		address:    pkhAddr,
		privateKey: privKey,
		net:        net,
	}, nil
}

// PoolFeeAddress fulfills SignPoolSplitOutputProvider.PoolFeeAddress
func (signer *privateKeySplitPoolSigner) PoolFeeAddress() dcrutil.Address {
	return signer.address
}

// SignPoolSplitOutput fulfills SignPoolSplitOutputProvider.SignPoolSplitOutput.
// It uses the stored private key to sign the pool split output.
// Assumes the pool output is the second TxOut of the split tx.
func (signer *privateKeySplitPoolSigner) SignPoolSplitOutput(split, ticket *wire.MsgTx) ([]byte, error) {
	if len(split.TxOut) < 2 {
		return nil, errors.Errorf("split has less than 2 outputs")
	}

	splitOut := split.TxOut[1]

	// Extract and print details from the script.
	scriptClass, addresses, reqSigs, err := txscript.ExtractPkScriptAddrs(
		splitOut.Version, splitOut.PkScript, signer.net)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding split.TxOut[1].PkScript")
	}

	if scriptClass != txscript.PubKeyHashTy {
		return nil, errors.Errorf("script class is not PubKeyHashTy")
	}

	if reqSigs != 1 {
		return nil, errors.Errorf("reqSigs different than 1")
	}

	if len(addresses) != 1 {
		return nil, errors.Errorf("decoded a different number of addresses "+
			"(%d) than expected (1)", len(addresses))
	}

	if addresses[0].EncodeAddress() != signer.address.EncodeAddress() {
		return nil, errors.Errorf("decoded a different address (%s) than "+
			"expected (%s)", addresses[0].EncodeAddress(),
			signer.address.EncodeAddress())
	}

	lookupKey := func(a dcrutil.Address) (chainec.PrivateKey, bool, error) {
		return signer.privateKey, true, nil
	}

	sigScript, err := txscript.SignTxOutput(signer.net,
		ticket, 0, splitOut.PkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil,
		chainec.ECTypeSecp256k1)
	if err != nil {
		return nil, errors.Wrapf(err, "error signing ticket.TxOut[0]")
	}

	return sigScript, nil
}

type masterPubPoolAddrValidator struct {
	addresses map[string]struct{}
}

func newMasterPubPoolAddrValidator(masterPubKey string, net *chaincfg.Params) (
	*masterPubPoolAddrValidator, error) {

	end := uint32(10000)

	if strings.Index(masterPubKey, ":") > -1 {
		idxStart := strings.Index(masterPubKey, ":") + 1
		newEnd, err := strconv.Atoi(masterPubKey[idxStart:])
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding end index of ",
				"masterPubKey")
		}
		end = uint32(newEnd)
		masterPubKey = masterPubKey[:idxStart-1]
	}

	key, err := hdkeychain.NewKeyFromString(masterPubKey)
	if err != nil {
		return nil, err
	}
	if !key.IsForNet(net) {
		return nil, fmt.Errorf("extended public key is for wrong network")
	}

	// Derive from external branch
	branchKey, err := key.Child(udb.ExternalBranch)
	if err != nil {
		return nil, err
	}

	// Derive the addresses from [0, end) for this extended public key.
	addrMap := make(map[string]struct{}, end)
	for i := uint32(0); i < end; i++ {
		child, err := branchKey.Child(i)
		if err == hdkeychain.ErrInvalidChild {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "error deriving %d'nth child key", i)
		}
		addr, err := child.Address(net)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating address for %d'nth ",
				"key", i)
		}

		addrMap[addr.EncodeAddress()] = struct{}{}
	}

	return &masterPubPoolAddrValidator{
		addresses: addrMap,
	}, nil
}

func (v *masterPubPoolAddrValidator) ValidatePoolSubsidyAddress(poolAddr dcrutil.Address) error {
	addr := poolAddr.EncodeAddress()
	_, has := v.addresses[addr]
	if !has {
		return errors.Errorf("pool address %s not found in addresses map",
			addr)
	}

	return nil
}
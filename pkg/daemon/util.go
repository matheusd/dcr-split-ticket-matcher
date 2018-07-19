package daemon

import (
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
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

	if wif.DSA() != dcrec.STEcdsaSecp256k1 {
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
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		return nil, errors.Wrapf(err, "error signing ticket.TxOut[0]")
	}

	return sigScript, nil
}

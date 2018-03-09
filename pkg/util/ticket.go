package util

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

type FixedTicketPriceProvider struct {
	TicketPrice uint64
}

func (p *FixedTicketPriceProvider) CurrentTicketPrice() uint64 {
	return p.TicketPrice
}

type FixedVoteAddressProvider struct {
	Address dcrutil.Address
}

func (p *FixedVoteAddressProvider) VotingAddress() dcrutil.Address {
	return p.Address
}

type ScriptVoteAddressProvider struct {
	PoolAddress   dcrutil.Address
	Script        []byte
	ScriptAddress dcrutil.Address

	net                   *chaincfg.Params
	revocationDelay       uint32
	revocationRelLockTime uint32
}

func NewScriptVoteAddressProvider(poolAddress dcrutil.Address, revocationDelay uint32,
	net *chaincfg.Params) (*ScriptVoteAddressProvider, error) {

	//revocationRelLockTime := (net.TicketExpiry + revocationDelay) & wire.SequenceLockTimeMask
	revocationRelLockTime := uint32(20)
	if revocationRelLockTime&wire.SequenceLockTimeIsSeconds != 0 {
		return nil, fmt.Errorf("RevocationDelay is too big and triggers SequenceLockTimeInSeconds")
	}

	b := txscript.NewScriptBuilder()
	b.
		AddOp(txscript.OP_IF).
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(poolAddress.ScriptAddress()).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG)
	b.
		AddOp(txscript.OP_ELSE).
		AddInt64(int64(revocationRelLockTime)).
		AddOp(txscript.OP_CHECKSEQUENCEVERIFY)
	b.AddOp(txscript.OP_ENDIF)

	script, err := b.Script()
	if err != nil {
		return nil, err
	}

	scriptAddr, err := dcrutil.NewAddressScriptHash(script, net)
	if err != nil {
		return nil, err
	}

	p := &ScriptVoteAddressProvider{
		PoolAddress:           poolAddress,
		Script:                script,
		ScriptAddress:         scriptAddr,
		net:                   net,
		revocationDelay:       revocationDelay,
		revocationRelLockTime: revocationRelLockTime,
	}
	return p, nil
}

func (p *ScriptVoteAddressProvider) VotingAddress() dcrutil.Address {
	return p.ScriptAddress
}

func (p *ScriptVoteAddressProvider) SignRevocation(ticket, revocation *wire.MsgTx) (*wire.MsgTx, error) {
	// TODO: right now this is simulating being able to revoke only after
	// the checksequenceverify time has elapsed. This needs to actually
	// sign the revocation transaction and return the revocation using the
	// signed pubkey conditional branch of the script.
	revoke := revocation.Copy()

	b := txscript.NewScriptBuilder()
	b.
		AddInt64(0).
		AddData(p.Script)
	sigScript, err := b.Script()
	if err != nil {
		return nil, err
	}

	revoke.TxIn[0].SignatureScript = sigScript
	revoke.TxIn[0].Sequence = p.revocationRelLockTime
	revoke.Version = 2 // needed to process OP_CHECKSEQUENCEVERIFY
	return revoke, nil
}

package splitticket

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

func TestCreateUnsignedRevocation(t *testing.T) {
	t.Parallel()

	net := &chaincfg.TestNet3Params
	poolFee, _ := dcrutil.NewAmount(0.1)
	maxParts := stake.MaxInputsPerSStx - 1

	zeroed := [20]byte{}
	addrP2PKH, _ := dcrutil.NewAddressPubKeyHash(zeroed[:], net, 0)
	addrP2SH, _ := dcrutil.NewAddressScriptHash(zeroed[:], net)
	voteScript, _ := txscript.PayToSStx(addrP2SH)
	poolCommitScriptP2PKH, _ := txscript.GenerateSStxAddrPush(addrP2PKH, poolFee,
		CommitmentLimits)
	poolCommitScriptP2SH, _ := txscript.GenerateSStxAddrPush(addrP2SH, poolFee,
		CommitmentLimits)
	changeScript, _ := txscript.PayToSStxChange(addrP2PKH)

	addresses := []dcrutil.Address{addrP2PKH, addrP2SH}
	poolCommitScripts := [][]byte{poolCommitScriptP2PKH, poolCommitScriptP2SH}

	revocationSigScript := make([]byte, 148)

	contribScripts := make([][]byte, 63)
	for p := 0; p < maxParts; p++ {
		addr := addresses[p%len(addresses)]
		contribAmount, _ := dcrutil.NewAmount(float64(1 + p))
		contribScripts[p], _ = txscript.GenerateSStxAddrPush(addr, contribAmount,
			CommitmentLimits)
	}

	ticket := wire.NewMsgTx()
	ticket.AddTxOut(wire.NewTxOut(0, voteScript))
	ticket.AddTxOut(wire.NewTxOut(0, nil))
	ticket.AddTxOut(wire.NewTxOut(0, changeScript))

	// Test various combinations of addresses to see if the size estimates
	// are correct
	for p := 0; p < maxParts; p++ {
		ticket.TxOut[1].PkScript = poolCommitScripts[p%len(poolCommitScripts)]
		ticket.AddTxOut(wire.NewTxOut(0, contribScripts[p]))
		ticket.AddTxOut(wire.NewTxOut(0, changeScript))
		ticket.TxOut[0].Value = 1e7
		for i := 0; i <= p; i++ {
			ticket.TxOut[0].Value += int64(1 + i*1e8)
		}

		ticketHash := ticket.TxHash() // the hash is actually irrelevant

		revocation, err := CreateUnsignedRevocation(&ticketHash, ticket,
			RevocationFeeRate(_testNetwork))
		if err != nil {
			t.Errorf("CreateUnsignedRevocation returned error on "+
				"part %d: %v", p, err)
			continue
		}

		revocation.TxIn[0].SignatureScript = revocationSigScript
		size := revocation.SerializeSize()
		minFee := dcrutil.Amount((size * int(TxFeeRate)) / 1000)

		fee, err := FindRevocationTxFee(ticket, revocation)
		if err != nil {
			t.Errorf("Error finding revocation fee at part %d: %v", p, err)
			continue
		}

		if fee < minFee {
			t.Errorf("Fee for revocation with %d participants (%s) less than "+
				"minimum required (%s)", p, fee, minFee)
		}

		// TODO: check using CheckRevocation (requires actually signing the
		// script)

	}

}

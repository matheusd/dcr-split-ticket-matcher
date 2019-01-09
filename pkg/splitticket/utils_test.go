package splitticket

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/hdkeychain"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

var (
	_testNetwork  = &chaincfg.TestNet3Params
	_testNullSeed = [32]byte{}
	_testHDKey, _ = hdkeychain.NewMaster(_testNullSeed[:], _testNetwork)
)

type testSessionData struct {
	nbParts                       int
	ticketPrice                   dcrutil.Amount
	partTicketFee                 dcrutil.Amount
	totalPoolFee                  dcrutil.Amount
	partsAmounts                  []dcrutil.Amount
	currentBlockHeight            uint32
	secretNbs                     []SecretNumber
	secretHashes                  []SecretNumberHash
	voteAddresses                 []dcrutil.Address
	poolAddresses                 []dcrutil.Address
	commitAddresses               []dcrutil.Address
	changeAddresses               []dcrutil.Address
	changeAmounts                 []dcrutil.Amount
	splitSrcAmounts               [][]dcrutil.Amount
	splitSrcKeys                  []chainec.PrivateKey
	splitChangeAmounts            []dcrutil.Amount
	split2TicketAddresses         []dcrutil.Address
	split2TicketPrivateKeys       []chainec.PrivateKey
	splitChangeAddresses          []dcrutil.Address
	split2TicketPoolFeeAddress    dcrutil.Address
	split2TicketPoolFeePrivateKey chainec.PrivateKey
	split2TicketIndices           []uint32
	splitPartExtraInputAmouts     []dcrutil.Amount
	splitPartFees                 []dcrutil.Amount
	ticketPartFees                []dcrutil.Amount
	poolPartFees                  []dcrutil.Amount
	limits                        uint16
	voterIndex                    int
	mainchainHash                 *chainhash.Hash
	splitSrcHash                  *chainhash.Hash
	splitUtxoMap                  UtxoMap
}

// checkSplit checks the given split against the given session data
func (d *testSessionData) checkSplit(split *wire.MsgTx) error {
	return CheckSplit(split, d.splitUtxoMap, d.secretHashes, d.mainchainHash,
		d.currentBlockHeight, _testNetwork)
}

// checkTicket checks the given ticket agains the given session data
func (d *testSessionData) checkTicket(split, ticket *wire.MsgTx) error {
	return CheckTicket(split, ticket, d.ticketPrice,
		d.partTicketFee, d.partsAmounts, d.currentBlockHeight,
		_testNetwork)
}

// checkSignedTicket checks the given signed ticket against the given session data
func (d *testSessionData) checkSignedTicket(split, ticket *wire.MsgTx) error {
	return CheckSignedTicket(split, ticket, _testNetwork)
}

// checkSignedSplit checks the given signed ticket against the given session data
func (d *testSessionData) checkSignedSplit(split *wire.MsgTx) error {
	return CheckSignedSplit(split, d.splitUtxoMap, _testNetwork)
}

// signTicket signs the ticket transaction with the keys recorded in sessionData.
func (d *testSessionData) signTicket(split, ticket *wire.MsgTx) {
	lookupKey := func(a dcrutil.Address) (chainec.PrivateKey, bool, error) {
		return d.split2TicketPoolFeePrivateKey, true, nil
	}

	sigs := make([][]byte, 0, len(d.partsAmounts)+1)

	sigScript, err := txscript.SignTxOutput(_testNetwork,
		ticket, 0, split.TxOut[1].PkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil, dcrec.STEcdsaSecp256k1)
	if err != nil {
		panic(err)
	}
	sigs = append(sigs, sigScript)

	for i := 1; i < len(ticket.TxIn); i++ {
		lookupKey := func(a dcrutil.Address) (chainec.PrivateKey, bool, error) {
			return d.split2TicketPrivateKeys[i-1], true, nil
		}

		pkScript := split.TxOut[d.split2TicketIndices[i-1]].PkScript
		sigScript, err := txscript.SignTxOutput(_testNetwork,
			ticket, i, pkScript, txscript.SigHashAll,
			txscript.KeyClosure(lookupKey), nil, nil, dcrec.STEcdsaSecp256k1)
		if err != nil {
			panic(err)
		}
		sigs = append(sigs, sigScript)
	}

	for i, sig := range sigs {
		ticket.TxIn[i].SignatureScript = sig
	}
}

// signTicket signs the ticket transaction with the keys recorded in sessionData.
func (d *testSessionData) signSplit(split *wire.MsgTx) {
	sigs := make([][]byte, 0, len(d.partsAmounts)+1)

	for i := 0; i < len(split.TxIn); i++ {
		lookupKey := func(a dcrutil.Address) (chainec.PrivateKey, bool, error) {
			return d.splitSrcKeys[i], true, nil
		}

		outp := wire.OutPoint{Hash: *d.splitSrcHash, Index: uint32(i),
			Tree: wire.TxTreeRegular}

		pkScript := d.splitUtxoMap[outp].PkScript
		sigScript, err := txscript.SignTxOutput(_testNetwork,
			split, i, pkScript, txscript.SigHashAll,
			txscript.KeyClosure(lookupKey), nil, nil, dcrec.STEcdsaSecp256k1)
		if err != nil {
			panic(err)
		}
		sigs = append(sigs, sigScript)
	}

	for i, sig := range sigs {
		split.TxIn[i].SignatureScript = sig
	}
}

// createTestTransactions creates a split and ticket transactions, given all
// data needed to create them.
func (d *testSessionData) createTestTransactions() (split, ticket *wire.MsgTx) {

	net := &chaincfg.TestNet3Params
	zeroed := [20]byte{}
	addrZeroed, _ := dcrutil.NewAddressPubKeyHash(zeroed[:], net, 0)
	zeroChangeScript, _ := txscript.PayToSStxChange(addrZeroed)

	split = wire.NewMsgTx()
	ticket = wire.NewMsgTx()
	nbParts := len(d.partsAmounts)

	ticket.Expiry = TargetTicketExpirationBlock(d.currentBlockHeight, 16,
		_testNetwork)
	ticket.LockTime = 0

	lotteryHash := CalcLotteryCommitmentHash(d.secretHashes, d.partsAmounts,
		d.voteAddresses, d.mainchainHash)

	lotteryScript := make([]byte, 0, 1+1+32)
	lotteryScript = append(lotteryScript, txscript.OP_RETURN)
	lotteryScript = append(lotteryScript, txscript.OP_DATA_32)
	lotteryScript = append(lotteryScript, lotteryHash[:]...)

	split2ticketoolScript, _ := txscript.PayToAddrScript(d.split2TicketPoolFeeAddress)

	split.AddTxOut(wire.NewTxOut(0, lotteryScript))
	split.AddTxOut(wire.NewTxOut(int64(d.totalPoolFee), split2ticketoolScript))

	voteScript, _ := txscript.PayToSStx(d.voteAddresses[d.voterIndex])
	poolCommitScript, _ := txscript.GenerateSStxAddrPush(d.poolAddresses[d.voterIndex],
		d.totalPoolFee, d.limits)

	ticket.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: 1,
		Tree: wire.TxTreeRegular}, wire.NullValueIn, nil))
	ticket.AddTxOut(wire.NewTxOut(int64(d.ticketPrice), voteScript))
	ticket.AddTxOut(wire.NewTxOut(0, poolCommitScript))
	ticket.AddTxOut(wire.NewTxOut(0, zeroChangeScript))

	splitInIdx := uint32(0)
	for i := 0; i < nbParts; i++ {
		for range d.splitSrcAmounts[i] {
			split.AddTxIn(wire.NewTxIn(wire.NewOutPoint(d.splitSrcHash, splitInIdx,
				wire.TxTreeRegular), wire.NullValueIn, nil))
			splitInIdx++
		}
		script, _ := txscript.PayToAddrScript(d.split2TicketAddresses[i])
		split.AddTxOut(wire.NewTxOut(int64(d.partsAmounts[i]+d.partTicketFee), script))

		script, _ = txscript.PayToAddrScript(d.splitChangeAddresses[i])
		split.AddTxOut(wire.NewTxOut(int64(d.splitChangeAmounts[i]), script))

		ticket.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: d.split2TicketIndices[i],
			Tree: wire.TxTreeRegular}, wire.NullValueIn, nil))

		commitScript, _ := txscript.GenerateSStxAddrPush(d.commitAddresses[i],
			d.partsAmounts[i]+d.partTicketFee, d.limits)
		changeScript, _ := txscript.PayToSStxChange(d.changeAddresses[i])

		ticket.AddTxOut(wire.NewTxOut(int64(d.partsAmounts[i]), commitScript))
		ticket.AddTxOut(wire.NewTxOut(int64(d.changeAmounts[i]), changeScript))
	}

	splitHash := split.TxHash()
	for _, in := range ticket.TxIn {
		in.PreviousOutPoint.Hash = splitHash
	}

	return
}

func (d *testSessionData) fillParticipationData(ticketPrice,
	partTicketFee dcrutil.Amount, poolFeeRate float64) {

	// number to secret number
	nb2sn := func(nb uint64) SecretNumber {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], nb)
		return SecretNumber(b[:])
	}

	rnd := rand.New(rand.NewSource(0x543210001701d))

	splitPartFee := dcrutil.Amount(1 * 1e6)

	d.partTicketFee = partTicketFee
	d.totalPoolFee = SessionPoolFee(d.nbParts, ticketPrice,
		int(d.currentBlockHeight), poolFeeRate, _testNetwork)
	d.ticketPrice = ticketPrice

	maxAmounts := make([]dcrutil.Amount, d.nbParts)
	for i := 0; i < d.nbParts; i++ {
		maxAmounts[i] = d.ticketPrice + d.totalPoolFee + d.partTicketFee +
			dcrutil.Amount(10*dcrutil.AtomsPerCoin)
	}

	amounts, poolFees, _ := SelectContributionAmounts(maxAmounts, d.ticketPrice,
		d.partTicketFee, d.totalPoolFee)
	d.partsAmounts = amounts

	splitOutIdx := uint32(2) // 0 = voter lottery commitment, 1 = pool fee output

	for i := 0; i < d.nbParts; i++ {
		secretNb := SecretNumber(nb2sn(rnd.Uint64()))
		extraInputAmount := dcrutil.Amount((1 + rnd.Intn(10)) * dcrutil.AtomsPerCoin)
		totalInputAmount := extraInputAmount + amounts[i] + poolFees[i] + splitPartFee

		d.splitPartFees = append(d.splitPartFees, splitPartFee)
		d.poolPartFees = append(d.poolPartFees, poolFees[i])
		d.ticketPartFees = append(d.ticketPartFees, d.partTicketFee)
		d.secretNbs = append(d.secretNbs, secretNb)
		d.secretHashes = append(d.secretHashes, secretNb.Hash(d.mainchainHash))
		d.splitPartExtraInputAmouts = append(d.splitPartExtraInputAmouts, extraInputAmount)
		d.splitSrcAmounts = append(d.splitSrcAmounts, []dcrutil.Amount{totalInputAmount})
		d.splitChangeAmounts = append(d.splitChangeAmounts, extraInputAmount)
		d.split2TicketIndices = append(d.split2TicketIndices, splitOutIdx)

		splitOutIdx += 2
	}

	_, voterIndex := CalcLotteryResult(d.secretNbs, d.partsAmounts,
		d.mainchainHash)
	d.voterIndex = voterIndex

}

func (d *testSessionData) fillAddressesAndKeys() {
	pubAddr := func(key *hdkeychain.ExtendedKey) dcrutil.Address {
		addr, _ := key.Address(_testNetwork)
		return addr
	}

	privKey := func(key *hdkeychain.ExtendedKey) chainec.PrivateKey {
		pk, _ := key.ECPrivKey()
		return pk
	}

	zeroed := [20]byte{}
	addrZeroed, _ := dcrutil.NewAddressPubKeyHash(zeroed[:], _testNetwork, 0)

	split2ticketPoolKey, _ := _testHDKey.Child(0x1701d000)
	d.split2TicketPoolFeeAddress = pubAddr(split2ticketPoolKey)
	d.split2TicketPoolFeePrivateKey = privKey(split2ticketPoolKey)

	for i := 0; i < d.nbParts; i++ {
		// to simplify the testing procedure, we use the following hierarchy
		// from the root test key: participant-index / purpose, where purpose is
		// the vote key, pool subsidy key, commit address, etc.

		partKey, _ := _testHDKey.Child(uint32(i))
		voteKey, _ := partKey.Child(0)
		poolKey, _ := partKey.Child(1)
		commitKey, _ := partKey.Child(2)
		splitChangeKey, _ := partKey.Child(3)
		split2TicketKey, _ := partKey.Child(4)
		splitSrcKey, _ := partKey.Child(5)

		d.voteAddresses = append(d.voteAddresses, pubAddr(voteKey))
		d.poolAddresses = append(d.poolAddresses, pubAddr(poolKey))
		d.commitAddresses = append(d.commitAddresses, pubAddr(commitKey))
		d.changeAddresses = append(d.changeAddresses, addrZeroed)
		d.changeAmounts = append(d.changeAmounts, 0)
		d.split2TicketAddresses = append(d.split2TicketAddresses, pubAddr(split2TicketKey))
		d.split2TicketPrivateKeys = append(d.split2TicketPrivateKeys, privKey(split2TicketKey))
		d.splitChangeAddresses = append(d.splitChangeAddresses, pubAddr(splitChangeKey))
		d.splitSrcKeys = append(d.splitSrcKeys, privKey(splitSrcKey))

		splitSrcOutp := wire.OutPoint{Hash: *d.splitSrcHash, Index: uint32(i),
			Tree: wire.TxTreeRegular}
		splitSrcScript, _ := txscript.PayToAddrScript(pubAddr(splitSrcKey))
		d.splitUtxoMap[splitSrcOutp] = UtxoEntry{Value: d.splitSrcAmounts[i][0],
			Version: txscript.DefaultScriptVersion, PkScript: splitSrcScript}
	}
}

func createBaseTestData(nbParts int) *testSessionData {
	mainchainHash, _ := chainhash.NewHashFromStr("000000000028687ab733813a5881ecc5fca938aae9dfeeb8370ed1f90d383800")
	splitSrcHash, _ := chainhash.NewHashFromStr("bbccdd071af3fff70ccd282beeaa292f38edac0fc7f8981f5acc0f5525017a04")

	return &testSessionData{
		nbParts:            nbParts,
		limits:             uint16(0x5800),
		mainchainHash:      mainchainHash,
		splitSrcHash:       splitSrcHash,
		currentBlockHeight: 170100,
		splitUtxoMap:       make(UtxoMap),
	}
}

// createStdTestData creates test data based on standard keys and procedures.
// This data can then be manipulated to test specific layouts during a test.
func createStdTestData(nbParts int) *testSessionData {

	ticketPrice := dcrutil.Amount(200 * dcrutil.AtomsPerCoin)
	partTicketFee := SessionParticipantFee(nbParts)

	data := createBaseTestData(nbParts)
	data.fillParticipationData(ticketPrice, partTicketFee, 5.0)
	data.fillAddressesAndKeys()

	return data
}

// TestStdTestDataCreation tests whether the data created by the standard
// testing function validates against the check functions. This is a preface to
// the actual testing functions, which will break the split, ticket and
// revocation transactions in various ways to see if the mistakes are caught.
// Given how the standard test data is created, this is also indirectly testing
// that many of the fee determination functions are correct.
func TestStdTestDataCreation(t *testing.T) {
	t.Parallel()
	var err error

	maxParts := stake.MaxInputsPerSStx - 1

	for i := 1; i < maxParts; i++ {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Parallel()
			data := createStdTestData(i)
			split, ticket := data.createTestTransactions()

			if err = data.checkSplit(split); err != nil {
				t.Errorf("Error checking split with %d participants: %+v", i,
					err)
				return
			}

			if err = data.checkTicket(split, ticket); err != nil {
				t.Errorf("Error checking ticket with %d participants: %+v", i,
					err)
				return
			}

			data.signTicket(split, ticket)

			if err = data.checkSignedTicket(split, ticket); err != nil {
				t.Errorf("Error checking signed ticket with %d participants: %+v", i,
					err)
				return
			}

			data.signSplit(split)

			if err = data.checkSignedSplit(split); err != nil {
				t.Errorf("Error checking signed split with %d participants: %+v", i,
					err)
				return
			}
		})
	}
}

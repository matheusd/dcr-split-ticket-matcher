package matcher

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/dchest/blake256"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/txrules"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher/internal/txsizes"
)

// NewRandUint64 returns a new uint64 or an error
func NewRandUint64() (uint64, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(b[:]), nil
}

// MustRandUint64 returns a new random uint64 or panics
func MustRandUint64() uint64 {
	r, err := NewRandUint64()
	if err != nil {
		panic(err)
	}
	return r
}

func NewRandInt32() (int32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24), nil
}

func MustRandInt32() int32 {
	r, err := NewRandInt32()
	if err != nil {
		panic(err)
	}
	return r
}

func NewRandInt16() (int16, error) {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int16(uint32(b[0]) | uint32(b[1])<<8), nil
}

func MustRandInt16() int16 {
	r, err := NewRandInt16()
	if err != nil {
		panic(err)
	}
	return r
}

func NewRandUInt16() (uint16, error) {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return uint16(uint32(b[0]) | uint32(b[1])<<8), nil
}

func MustRandUInt16() uint16 {
	r, err := NewRandUInt16()
	if err != nil {
		panic(err)
	}
	return r
}

// createUnsignedRevocation creates an unsigned revocation transaction that
// revokes a missed or expired ticket.  Revocations must carry a relay fee and
// this function can error if the revocation contains no suitable output to
// decrease the estimated relay fee from.
func CreateUnsignedRevocation(ticketHash *chainhash.Hash, ticketPurchase *wire.MsgTx, feePerKB dcrutil.Amount) (*wire.MsgTx, error) {
	// Parse the ticket purchase transaction to determine the required output
	// destinations for vote rewards or revocations.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticketPurchase)

	// Calculate the output values for the revocation.  Revocations do not
	// contain any subsidy.
	revocationValues := stake.CalculateRewards(ticketValues,
		ticketPurchase.TxOut[0].Value, 0)

	// Begin constructing the revocation transaction.
	revocation := wire.NewMsgTx()

	// Revocations reference the ticket purchase with the first (and only)
	// input.
	ticketOutPoint := wire.NewOutPoint(ticketHash, 0, wire.TxTreeStake)
	revocation.AddTxIn(wire.NewTxIn(ticketOutPoint, nil))
	scriptSizers := []txsizes.ScriptSizer{txsizes.P2SHScriptSize}

	// All remaining outputs pay to the output destinations and amounts tagged
	// by the ticket purchase.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSRtxPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSRtxSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		revocation.AddTxOut(wire.NewTxOut(revocationValues[i], script))
	}

	// Revocations must pay a fee but do so by decreasing one of the output
	// values instead of increasing the input value and using a change output.
	// Calculate the estimated signed serialize size.
	sizeEstimate := txsizes.EstimateSerializeSize(scriptSizers, revocation.TxOut, false)
	feeEstimate := txrules.FeeForSerializeSize(feePerKB, sizeEstimate)

	// Reduce the output value of one of the outputs to accomodate for the relay
	// fee.  To avoid creating dust outputs, a suitable output value is reduced
	// by the fee estimate only if it is large enough to not create dust.  This
	// code does not currently handle reducing the output values of multiple
	// commitment outputs to accomodate for the fee.
	for _, output := range revocation.TxOut {
		if dcrutil.Amount(output.Value) > feeEstimate {
			amount := dcrutil.Amount(output.Value) - feeEstimate
			if !txrules.IsDustAmount(amount, len(output.PkScript), feePerKB) {
				output.Value = int64(amount)
				return revocation, nil
			}
		}
	}
	return nil, errors.New("no suitable revocation outputs to pay relay fee")
}

// ChooseVoter chooses who should vote on a ticket purchase, given an array of
// contribution amounts that sum to the ticket price. Chance to be selected is
// proportional to amount of contribution
func ChooseVoter(contributions []dcrutil.Amount) int {
	var total dcrutil.Amount
	for _, v := range contributions {
		total += v
	}

	selected, err := rand.Int(rand.Reader, big.NewInt(int64(total)))
	if err != nil {
		// entropy problems... just quit
		panic(err)
	}
	sel := dcrutil.Amount(selected.Int64())

	total = 0
	for i, v := range contributions {
		total += v
		if sel < total {
			return i
		}
	}

	// we shouldn't really get here, as rand.Int() returns [0, total)
	return len(contributions) - 1
}

// SelectContributionAmounts decides how to split the ticket priced at ticketPrice
// such that each ith participant contributes at most maxAmount[i], pays
// a participation fee partFee and contributes poolPartFee into the common
// pool fee input.
//
// In order to call this function sum(maxAmounts) must be > (ticketPrice + sum(partFee))
// and the maxAmounts **MUST** be ordered in ascending order of amount.
//
// The algorithm tries to split the purchasing amounts between the participants
// in such a way as to average the contribution % of the participants, while
// ensuring that the ticket can be bought.
//
// This function returns the corresponding commitment amounts for each participant
// (ie, what the sstxcommitment output will be for each participant).
func SelectContributionAmounts(maxAmounts []dcrutil.Amount, ticketPrice, partFee, poolPartFee dcrutil.Amount) ([]dcrutil.Amount, error) {

	nparts := len(maxAmounts)
	totalAvailable := dcrutil.Amount(0)
	prev := dcrutil.Amount(0)
	remainingMax := make([]dcrutil.Amount, nparts)
	for i, a := range maxAmounts {
		if a < prev {
			return nil, fmt.Errorf("Invalid Argument: maxAmounts must be in ascending order")
		}
		totalAvailable += a
		prev = a

		// remainingMax already considers the partFee and poolPartFee contributions
		// as payed by the participant
		remainingMax[i] = a - partFee - poolPartFee
	}

	// notice that while the partFee is an "additional" amount needed by each participant,
	// the pool fee is simply a part of the ticket price placed into the first sstx
	// output, therefore it is already accounted as needed in the ticketPrice amount
	neededAmount := ticketPrice + partFee*dcrutil.Amount(nparts)
	if totalAvailable < neededAmount {
		return nil, fmt.Errorf("Invalid Argument: total available %s less than needed %s",
			totalAvailable, neededAmount)
	}

	// totalLeft is the remaining part of the ticket that needs to be distributed
	// among the participants. It considers the pool fee included in output 1 as
	// accounted for.
	totalLeft := ticketPrice - poolPartFee*dcrutil.Amount(nparts)

	// Algorithm sketch: loop starting from lowest amount to highest
	// if all participants starting at the current one can fill the ticket, then
	// average the contribution of the remaining participants.
	// Otherwise, all participants contribute at least the amount of the current
	// one and we continue trying to split the remaining ticket amount left
	contribs := make([]dcrutil.Amount, nparts)
	for i, max := range remainingMax {
		remainingParts := dcrutil.Amount(nparts - i)
		if remainingParts*max > totalLeft {
			// average the remaining total among the remaining participants
			perPart := dcrutil.Amount(math.Ceil(float64(totalLeft) / float64(remainingParts)))
			for j := i; (j < nparts) && (totalLeft > 0); j++ {
				if totalLeft < perPart {
					// due to the division, some participants may contribute slightly less
					contribs[j] += totalLeft
					totalLeft = 0
				} else {
					contribs[j] += perPart
					totalLeft -= perPart
				}
			}
			break
		} else {
			// not enough to split the participation equally, so everyone
			// contributes as much as this participant
			for j := i; j < nparts; j++ {
				contribs[j] += max
				totalLeft -= max
				remainingMax[j] -= max
			}
		}
	}

	return contribs, nil
}

const SecretNbHashSize = 32

type SecretNumberHash [SecretNbHashSize]byte

func (h SecretNumberHash) Equals(other SecretNumberHash) bool {
	return bytes.Compare(h[:], other[:]) == 0
}

type SecretNumber uint64

func (nb SecretNumber) Hash(mainchainHash chainhash.Hash) SecretNumberHash {
	var res SecretNumberHash
	var data [8]byte
	var calculated []byte

	binary.LittleEndian.PutUint64(data[:], uint64(nb))

	// note that block hashes are reversed, so the first bytes are the actual
	// random bytes (ending bytes should be a string of 0000s)
	h := blake256.NewSalt(mainchainHash[:16])
	calculated = h.Sum(data[:])
	copy(res[:], calculated)

	return res
}

const SecretNumbersHashSize = 32

type SecretNumbersHash [SecretNumbersHashSize]byte

func (h SecretNumbersHash) SelectedCoin(max uint64) uint64 {
	res := big.NewInt(0)
	n := big.NewInt(0)
	n.SetBytes(h[:])
	m := big.NewInt(int64(max))
	res.Mod(n, m)
	return res.Uint64()
}

type SecretNumbers []SecretNumber

func (nbs SecretNumbers) Hash(mainchainHash chainhash.Hash) SecretNumbersHash {
	var res SecretNumbersHash
	var data [8]byte
	var calculated []byte

	// note that block hashes are reversed, so the first bytes are the actual
	// random bytes (ending bytes should be a string of 0000s)
	h := blake256.NewSalt(mainchainHash[:16])

	for _, nb := range nbs {
		binary.LittleEndian.PutUint64(data[:], uint64(nb))
		calculated = h.Sum(data[:])
	}

	copy(res[:], calculated)

	return res
}

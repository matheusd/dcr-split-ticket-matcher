package splitticket

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"

	"github.com/dchest/blake256"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
)

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

func (h SecretNumberHash) String() string {
	return hex.EncodeToString(h[:])
}

type SecretNumber uint64

func (nb SecretNumber) Hash(mainchainHash *chainhash.Hash) SecretNumberHash {
	var res SecretNumberHash
	var data [8]byte
	var calculated []byte

	binary.LittleEndian.PutUint64(data[:], uint64(nb))

	// note that block hashes are reversed, so the first bytes are the actual
	// random bytes (ending bytes should be a string of 0000s)
	h := blake256.NewSalt(mainchainHash[:16])
	h.Write(data[:])
	calculated = h.Sum(nil)
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

func (h SecretNumbersHash) String() string {
	return hex.EncodeToString(h[:])
}

type SecretNumbers []SecretNumber

func (nbs SecretNumbers) Hash(mainchainHash *chainhash.Hash) SecretNumbersHash {
	var res SecretNumbersHash
	var data [8]byte
	var calculated []byte

	// note that block hashes are reversed, so the first bytes are the actual
	// random bytes (ending bytes should be a string of 0000s)
	h := blake256.NewSalt(mainchainHash[:16])

	for _, nb := range nbs {
		binary.LittleEndian.PutUint64(data[:], uint64(nb))
		h.Write(data[:])
	}
	calculated = h.Sum(nil)
	copy(res[:], calculated)

	return res
}

// SecretNumberHashesHashSize is the size of the resulting hash of all secret
// number hashes.
const SecretNumberHashesHashSize = 32

func SecretNumberHashesHash(hashes []SecretNumberHash, mainchainHash *chainhash.Hash) []byte {
	var calculated []byte

	// note that block hashes are reversed, so the first bytes are the actual
	// random bytes (ending bytes should be a string of 0000s)
	h := blake256.NewSalt(mainchainHash[:16])

	for _, hs := range hashes {
		h.Write(hs[:])
	}
	calculated = h.Sum(nil)

	return calculated
}

// StakeDiffChangeDistance returns the distance (in blocks) to the closest
// stake diff change block (either in the past or the future, whichever is
// closest).
func StakeDiffChangeDistance(blockHeight uint32, params *chaincfg.Params) int32 {

	winSize := int32(params.WorkDiffWindowSize)

	// the remainder decreases as the block height approaches a change block,
	// so the lower baseDist is, the closer it is to the next change.
	baseDist := int32(blockHeight) % winSize
	if baseDist > winSize/2 {
		// the previous change block is closer
		return winSize - baseDist
	} else {
		// the next change block is closer
		return baseDist
	}
}

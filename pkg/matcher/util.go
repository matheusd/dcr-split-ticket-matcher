package matcher

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
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

// NewRandInt32 generates a new random int32 number or an error in case of
// entropy exhaustion
func NewRandInt32() (int32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24), nil
}

// MustRandInt32 generates a new int32 number or panics.
func MustRandInt32() int32 {
	r, err := NewRandInt32()
	if err != nil {
		panic(err)
	}
	return r
}

// NewRandInt16 generates a new int16 or returns an error in case of entropy
// exhaustion
func NewRandInt16() (int16, error) {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int16(uint32(b[0]) | uint32(b[1])<<8), nil
}

// MustRandInt16 generates a new int16 or panics.
func MustRandInt16() int16 {
	r, err := NewRandInt16()
	if err != nil {
		panic(err)
	}
	return r
}

// NewRandUInt16 generates a new uint16 or returns an error in case of entropy
// exhaustion
func NewRandUInt16() (uint16, error) {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return uint16(uint32(b[0]) | uint32(b[1])<<8), nil
}

// MustRandUInt16 generates a new uint16 or panics.
func MustRandUInt16() uint16 {
	r, err := NewRandUInt16()
	if err != nil {
		panic(err)
	}
	return r
}

// TargetTicketExpirationBlock calculates the expected expiration block for a
// ticket, given the current block height and a maximum expiry value.
//
// The calculated value is guaranteed to be < maxExpiry, but may be
// significantly less if the current block height is close to a change in stake
// difficulty.
func TargetTicketExpirationBlock(curBlockHeight, maxExpiry uint32,
	params *chaincfg.Params) uint32 {

	dist := curBlockHeight % uint32(params.WorkDiffWindowSize)
	if dist < maxExpiry {
		return curBlockHeight + dist
	}

	return curBlockHeight + maxExpiry
}

// InsecurePoolAddressesValidator is a validator for vote/pool addresses that
// always accepts the addresses, so it is insecure for production use.
type InsecurePoolAddressesValidator struct{}

// ValidateVoteAddress fulfills the VoteAddressValidationProvider interface
func (v InsecurePoolAddressesValidator) ValidateVoteAddress(voteAddr dcrutil.Address) error {
	return nil
}

// ValidatePoolSubsidyAddress fulfills the PoolAddressValidationProvider
func (v InsecurePoolAddressesValidator) ValidatePoolSubsidyAddress(poolAddr dcrutil.Address) error {
	return nil
}

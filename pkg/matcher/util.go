package matcher

import (
	"context"
	"crypto/rand"
	"encoding/binary"

	"github.com/decred/dcrd/dcrutil"
)

// NewRandInt64 returns a new int64 or an error
func NewRandInt64() (int64, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 |
			int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56,
		nil
}

// MustRandInt64 returns a new random int64 or panics
func MustRandInt64() int64 {
	r, err := NewRandInt64()
	if err != nil {
		panic(err)
	}
	return r
}

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

func encodedAddresses(addrs []dcrutil.Address) []string {
	res := make([]string, len(addrs))
	for i, addr := range addrs {
		res[i] = addr.EncodeAddress()
	}
	return res
}

// WithOriginalSrc returns a new context usable within the matcher package that
// indicates the original source for a given matcher operation
func WithOriginalSrc(parent context.Context, src string) context.Context {
	return context.WithValue(parent, originalSrcCtxKey, src)
}

// OriginalSrcFromCtx extracts the original source from a context variable
func OriginalSrcFromCtx(ctx context.Context) string {
	val, ok := ctx.Value(originalSrcCtxKey).(string)
	if ok {
		return val
	}
	return "[original src not provided]"
}

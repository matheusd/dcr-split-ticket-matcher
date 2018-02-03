package matcher

import (
	"crypto/rand"
	"encoding/binary"
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

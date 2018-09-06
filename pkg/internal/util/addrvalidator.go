package util

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/hdkeychain"
	"github.com/decred/dcrwallet/wallet/udb"
	"github.com/pkg/errors"
)

// MasterPubPoolAddrValidator validates addresses given a masterpubkey
type MasterPubPoolAddrValidator struct {
	addresses map[string]struct{}
}

// NewMasterPubPoolAddrValidator creates a new validator given an extended
// master pubkey for an account.
func NewMasterPubPoolAddrValidator(masterPubKey string, net *chaincfg.Params) (
	*MasterPubPoolAddrValidator, error) {

	end := uint32(10000)

	if strings.Contains(masterPubKey, ":") {
		idxStart := strings.Index(masterPubKey, ":") + 1
		newEnd, err := strconv.Atoi(masterPubKey[idxStart:])
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding end index of "+
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
			return nil, errors.Wrapf(err, "error creating address for %d'nth "+
				"key", i)
		}

		addrMap[addr.EncodeAddress()] = struct{}{}
	}

	return &MasterPubPoolAddrValidator{
		addresses: addrMap,
	}, nil
}

// ValidatePoolSubsidyAddress fulfills matcher.PoolAddressValidationProvider
func (v *MasterPubPoolAddrValidator) ValidatePoolSubsidyAddress(poolAddr dcrutil.Address) error {
	return v.ValidateByEncodedAddr(poolAddr.EncodeAddress())
}

// ValidateByEncodedAddr validate directly by a string value. Assumes addr is
// the result of a call to EncodeAddress()
func (v *MasterPubPoolAddrValidator) ValidateByEncodedAddr(addr string) error {
	if _, has := v.addresses[addr]; !has {
		return errors.Errorf("pool address %s not found in addresses map",
			addr)
	}

	return nil
}

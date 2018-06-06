package splitticket

import (
	"bytes"
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"
)

// VoterSelectionValidationError are errors generated inside the voter
// validation check function. These are considered critical errors of the
// ticket matching process.
type VoterSelectionValidationError struct {
	msg string
}

func (e VoterSelectionValidationError) Error() string {
	return e.msg
}

// CheckSelectedVoter checks whether all the provided information validates
// into the correct voter in the script.
//
// The purpose of this function is to ensure that the provided ticket is
// the one that is supposed to be published into the network, given what is
// known about a split ticket matching session. The parameters are the
// following:
//
// - secretNbs is the list of secret numbers provided by each individual
// participant
// - secretNbHashes is the list of secret number hashes provided by each
// individual participant *before* the split transaction was funded
// - amounts is the list of individual participation amounts
// - voteScripts is the list of voting scripts for each individual participant
// - ticket is the ticket transaction
// - mainchainHash is the hash of the block at the start of the matching session
//
// Note that the lists must be in the correct order, otherwise the lottery
// choice will not be consistent.
func CheckSelectedVoter(secretNbs []SecretNumber,
	secretNbHashes []SecretNumberHash,
	amounts []dcrutil.Amount, voteScripts [][]byte,
	ticket *wire.MsgTx, mainchainHash *chainhash.Hash) error {

	// just a syntactic sugar to make the return a bit less verbose
	newerr := func(msg string, args ...interface{}) error {
		return VoterSelectionValidationError{fmt.Sprintf(msg, args...)}
	}

	nbParts := len(amounts)

	if len(secretNbs) != nbParts {
		return errors.WithStack(newerr("len(secretNbs) != number of participants"))
	}

	if len(secretNbHashes) != nbParts {
		return errors.WithStack(newerr("len(secretNbs) != number of participants"))
	}

	if len(voteScripts) != nbParts {
		return errors.WithStack(newerr("len(voteScripts) != number of participants"))
	}

	for i, snb := range secretNbs {
		hash := snb.Hash(mainchainHash)
		if !hash.Equals(secretNbHashes[i]) {
			return errors.WithStack(newerr("secret number at index %d does " +
				"not hash to the expected value"))
		}
	}

	_, voterIdx := CalcLotteryResult(secretNbs, amounts, mainchainHash)

	expectedVoterPk := voteScripts[voterIdx]
	voterPk := ticket.TxOut[0].PkScript

	if !bytes.Equal(voterPk, expectedVoterPk) {
		return errors.WithStack(newerr("pkscript of ticket is not the " +
			"same as the expected pkscript derived from the secret numbers"))
	}

	return nil
}

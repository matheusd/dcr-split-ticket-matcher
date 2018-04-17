package daemon

import (
	"github.com/ansel1/merry"
)

var (
	ErrPoolFeeInputNotSigned = merry.New("Pool fee input not signed")
	ErrHelpRequested         = merry.New("Help requested")
	ErrArgParsingError       = merry.New("Error parsing command line argument")
	ErrDcrdPingTimeout       = merry.New("Timeout pinging dcrd node")
	ErrSecretNbSizeError     = merry.New("Hash of the secret number has incorrect size")
)

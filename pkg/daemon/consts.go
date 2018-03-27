package daemon

import (
	"github.com/ansel1/merry"
)

var (
	ErrPoolFeeInputNotSigned = merry.New("Pool fee input not signed")
	ErrHelpRequested         = merry.New("Help requested")
	ErrArgParsingError       = merry.New("Error parsing command line argument")
)

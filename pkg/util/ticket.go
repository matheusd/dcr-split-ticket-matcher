package util

import (
	"github.com/decred/dcrd/dcrutil"
)

type FixedTicketPriceProvider struct {
	TicketPrice uint64
}

func (p *FixedTicketPriceProvider) CurrentTicketPrice() uint64 {
	return p.TicketPrice
}

type FixedVoteAddressProvider struct {
	Address dcrutil.Address
}

func (p *FixedVoteAddressProvider) VotingAddress() dcrutil.Address {
	return p.Address
}

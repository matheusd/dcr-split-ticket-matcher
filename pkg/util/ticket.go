package util

type FixedTicketPriceProvider struct {
	TicketPrice uint64
}

func (p *FixedTicketPriceProvider) CurrentTicketPrice() uint64 {
	return p.TicketPrice
}

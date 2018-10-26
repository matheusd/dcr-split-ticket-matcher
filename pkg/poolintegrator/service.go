package poolintegrator

import (
	"context"

	"github.com/decred/dcrd/dcrutil"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/integratorrpc"
	"github.com/pkg/errors"
)

// ValidateVoteAddress fullfill grpc service requirements
func (d *Daemon) ValidateVoteAddress(ctx context.Context, req *pb.ValidateVoteAddressRequest) (
	*pb.ValidateVoteAddressResponse, error) {

	resp := new(pb.ValidateVoteAddressResponse)

	d.log.Infof("Received vote validate request %s", req.Address)

	addr, err := dcrutil.DecodeAddress(req.Address)
	if err != nil {
		resp.Error = errors.Wrap(err, "error decoding address").Error()
		d.log.Warnf("Received vote validate request with undecodable address (%s): %s",
			req.Address, err)
		return resp, nil
	}

	wresp, err := d.wallet.ValidateAddress(addr)
	if err != nil {
		resp.Error = errors.Wrap(err, "error validating vote address").Error()
		d.log.Warnf("Received error trying to validate address %s with wallet: %s",
			req.Address, err)
	} else if !wresp.IsValid {
		resp.Error = "provided address is not valid"
		d.log.Warnf("Received vote validate request with invalid address (%s)",
			req.Address)
	} else if !wresp.IsMine {
		resp.Error = "address is not from this voting pool"
		d.log.Infof("Received vote validate request with address not owned (%s)",
			req.Address)
	}

	return resp, nil
}

// ValidatePoolSubsidyAddress fullfill grpc service requirements
func (d *Daemon) ValidatePoolSubsidyAddress(ctx context.Context,
	req *pb.ValidatePoolSubsidyAddressRequest) (*pb.ValidatePoolSubsidyAddressResponse, error) {

	resp := new(pb.ValidatePoolSubsidyAddressResponse)

	d.log.Infof("Received pool validate request %s", req.Address)

	err := d.poolAddrValidator.ValidateByEncodedAddr(req.Address)
	if err != nil {
		resp.Error = err.Error()
		d.log.Warnf("Received pool validate request by wrong address (%s): %s",
			req.Address, err)
	}

	return resp, nil
}

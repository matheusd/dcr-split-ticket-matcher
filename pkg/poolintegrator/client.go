package poolintegrator

import (
	"context"
	"time"

	"github.com/decred/dcrd/dcrutil"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/integratorrpc"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client represents a client to a poolintegrator daemon (ie, the part that runs
// on the matcher service)
type Client struct {
	client pb.VotePoolIntegratorServiceClient
}

// NewClient creates a new client connection
func NewClient(host, cert string) (*Client, error) {
	creds, err := credentials.NewClientTLSFromFile(cert, "localhost")
	if err != nil {
		return nil, errors.Wrap(err, "error creating client credentials")
	}

	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}

	svcClient := pb.NewVotePoolIntegratorServiceClient(conn)

	client := &Client{
		client: svcClient,
	}

	return client, nil
}

// ValidateVoteAddress fulfills matcher.VoteAddressValidationProvider
func (c *Client) ValidateVoteAddress(voteAddr dcrutil.Address) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req := &pb.ValidateVoteAddressRequest{
		Address: voteAddr.EncodeAddress(),
	}

	resp, err := c.client.ValidateVoteAddress(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "error contacting stakepoold integrator to "+
			"validate vote address %s", req.Address)
	}

	if resp.Error != "" {
		return errors.Errorf("stakepoold integrator replied with error: %s",
			resp.Error)
	}

	return nil
}

// ValidatePoolSubsidyAddress fulfills matcher.PoolAddressValidationProvider
func (c *Client) ValidatePoolSubsidyAddress(poolAddr dcrutil.Address) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req := &pb.ValidatePoolSubsidyAddressRequest{
		Address: poolAddr.EncodeAddress(),
	}

	resp, err := c.client.ValidatePoolSubsidyAddress(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "error contacting stakepoold integrator to "+
			"validate pool subsidy address")
	}

	if resp.Error != "" {
		return errors.Errorf("stakepoold integrator replied with error: %s",
			resp.Error)
	}

	return nil
}

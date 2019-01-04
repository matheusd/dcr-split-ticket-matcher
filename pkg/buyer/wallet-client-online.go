package buyer

import (
	"context"
	"math/rand"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type onlineWalletClient struct {
	conn            *grpc.ClientConn
	wsvc            pb.WalletServiceClient
	publishedSplit  bool
	publishedTicket *chainhash.Hash
}

// connectToWallet connects to the given wallet address
func connectToWallet(walletHost string, walletCert string) (WalletClientConn, error) {
	rand.Seed(time.Now().Unix())
	creds, err := credentials.NewClientTLSFromFile(walletCert, "localhost")
	if err != nil {
		return nil, err
	}

	optCreds := grpc.WithTransportCredentials(creds)

	connCtx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	// wallet doesn't allow config of keepalive for the moment, so disable it
	// and perform a manual ping.
	// optKeepAlive := grpc.WithKeepaliveParams(keepalive.ClientParameters{
	// 	Time:                30 * time.Second,
	// 	Timeout:             5 * time.Second,
	// 	PermitWithoutStream: true,
	// })

	conn, err := grpc.DialContext(connCtx, walletHost, optCreds)
	if err != nil {
		return nil, err
	}

	wsvc := pb.NewWalletServiceClient(conn)

	wc := &onlineWalletClient{
		conn: conn,
		wsvc: wsvc,
	}

	return wc, nil
}

func (c *onlineWalletClient) Ping(ctx context.Context, in *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	return c.wsvc.Ping(ctx, in, opts...)
}

func (c *onlineWalletClient) Network(ctx context.Context, in *pb.NetworkRequest, opts ...grpc.CallOption) (*pb.NetworkResponse, error) {
	return c.wsvc.Network(ctx, in, opts...)
}

func (c *onlineWalletClient) NextAddress(ctx context.Context, in *pb.NextAddressRequest, opts ...grpc.CallOption) (*pb.NextAddressResponse, error) {
	return c.wsvc.NextAddress(ctx, in, opts...)
}

func (c *onlineWalletClient) ConstructTransaction(ctx context.Context, in *pb.ConstructTransactionRequest, opts ...grpc.CallOption) (*pb.ConstructTransactionResponse, error) {
	return c.wsvc.ConstructTransaction(ctx, in, opts...)
}

func (c *onlineWalletClient) SignTransactions(ctx context.Context, in *pb.SignTransactionsRequest, opts ...grpc.CallOption) (*pb.SignTransactionsResponse, error) {
	return c.wsvc.SignTransactions(ctx, in, opts...)
}

func (c *onlineWalletClient) ValidateAddress(ctx context.Context, in *pb.ValidateAddressRequest, opts ...grpc.CallOption) (*pb.ValidateAddressResponse, error) {
	return c.wsvc.ValidateAddress(ctx, in, opts...)
}

func (c *onlineWalletClient) SignMessage(ctx context.Context, in *pb.SignMessageRequest, opts ...grpc.CallOption) (*pb.SignMessageResponse, error) {
	return c.wsvc.SignMessage(ctx, in, opts...)
}

func (c *onlineWalletClient) BestBlock(ctx context.Context, in *pb.BestBlockRequest, opts ...grpc.CallOption) (*pb.BestBlockResponse, error) {
	return c.wsvc.BestBlock(ctx, in, opts...)
}

func (c *onlineWalletClient) TicketPrice(ctx context.Context, in *pb.TicketPriceRequest, opts ...grpc.CallOption) (*pb.TicketPriceResponse, error) {
	return c.wsvc.TicketPrice(ctx, in, opts...)
}

func (c *onlineWalletClient) MonitorForSessionTransactions(ctx context.Context,
	splitHash *chainhash.Hash, ticketsHashes []*chainhash.Hash) error {

	ntfs, err := c.wsvc.TransactionNotifications(ctx, &pb.TransactionNotificationsRequest{})
	if err != nil {
		return errors.Wrap(err, "error attaching to transaction notifications")
	}

	ticketsMap := make(map[chainhash.Hash]struct{}, len(ticketsHashes))
	for _, t := range ticketsHashes {
		ticketsMap[*t] = struct{}{}
	}

	go func() {
		for {
			resp, err := ntfs.Recv()
			if err != nil {
				// probably is due to the context closing
				return
			}

			for _, tx := range resp.UnminedTransactions {
				txh, err := chainhash.NewHash(tx.Hash)
				if err != nil {
					// maybe log?
					continue
				}

				if !c.publishedSplit && splitHash.IsEqual(txh) {
					c.publishedSplit = true
					continue
				}

				_, has := ticketsMap[*txh]
				if has {
					c.publishedTicket = txh
				}
			}

			// unlikley to appear directly as a mined tx, but we should be thorough
			for _, block := range resp.GetAttachedBlocks() {
				for _, tx := range block.Transactions {
					txh, err := chainhash.NewHash(tx.Hash)
					if err != nil {
						// maybe log?
						continue
					}

					if !c.publishedSplit && splitHash.IsEqual(txh) {
						c.publishedSplit = true
						continue
					}

					_, has := ticketsMap[*txh]
					if has {
						c.publishedTicket = txh
					}
				}
			}

			if c.publishedSplit && (c.publishedTicket != nil) {
				// got both transactions. can exit.
				return
			}
		}
	}()

	return nil
}

func (c *onlineWalletClient) PublishedSplitTx() bool {
	return c.publishedSplit
}

func (c *onlineWalletClient) PublishedTicketTx() *chainhash.Hash {
	return c.publishedTicket
}

func (c *onlineWalletClient) TransactionNotifications(ctx context.Context, in *pb.TransactionNotificationsRequest, opts ...grpc.CallOption) (pb.WalletService_TransactionNotificationsClient, error) {
	return c.wsvc.TransactionNotifications(ctx, in, opts...)
}

func (c *onlineWalletClient) Close() error {
	return c.conn.Close()
}

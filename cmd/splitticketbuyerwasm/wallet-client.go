// +build js,wasm

package main

import (
	"context"
	"syscall/js"

	"github.com/decred/dcrd/chaincfg/chainhash"
	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"google.golang.org/grpc"
)

type jsWalletClient struct{}

func (c *jsWalletClient) Ping(ctx context.Context, in *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	out := new(pb.PingResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "ping", in, out)
}

func (c *jsWalletClient) Network(ctx context.Context, in *pb.NetworkRequest, opts ...grpc.CallOption) (*pb.NetworkResponse, error) {
	out := new(pb.NetworkResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "network", in, out)
}

func (c *jsWalletClient) NextAddress(ctx context.Context, in *pb.NextAddressRequest, opts ...grpc.CallOption) (*pb.NextAddressResponse, error) {
	out := new(pb.NextAddressResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "nextAddress", in, out)
}

func (c *jsWalletClient) ConstructTransaction(ctx context.Context, in *pb.ConstructTransactionRequest, opts ...grpc.CallOption) (*pb.ConstructTransactionResponse, error) {
	out := new(pb.ConstructTransactionResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "constructTransaction", in, out)
}

func (c *jsWalletClient) SignTransactions(ctx context.Context, in *pb.SignTransactionsRequest, opts ...grpc.CallOption) (*pb.SignTransactionsResponse, error) {
	out := new(pb.SignTransactionsResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "signTransactions", in, out)
}

func (c *jsWalletClient) ValidateAddress(ctx context.Context, in *pb.ValidateAddressRequest, opts ...grpc.CallOption) (*pb.ValidateAddressResponse, error) {
	out := new(pb.ValidateAddressResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "validateAddress", in, out)
}

func (c *jsWalletClient) SignMessage(ctx context.Context, in *pb.SignMessageRequest, opts ...grpc.CallOption) (*pb.SignMessageResponse, error) {
	out := new(pb.SignMessageResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "signMessage", in, out)
}

func (c *jsWalletClient) BestBlock(ctx context.Context, in *pb.BestBlockRequest, opts ...grpc.CallOption) (*pb.BestBlockResponse, error) {
	out := new(pb.BestBlockResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "bestBlock", in, out)
}

func (c *jsWalletClient) TicketPrice(ctx context.Context, in *pb.TicketPriceRequest, opts ...grpc.CallOption) (*pb.TicketPriceResponse, error) {
	out := new(pb.TicketPriceResponse)
	return out, jsGrpcCall(jsObject.Get("wallet"), "ticketPrice", in, out)
}

func (c *jsWalletClient) MonitorForSessionTransactions(ctx context.Context,
	splitHash *chainhash.Hash, ticketsHashes []*chainhash.Hash) error {

	jsSplitHash := js.ValueOf(splitHash.String())
	jsTicketsHashes := make([]interface{}, len(ticketsHashes))
	for i, h := range ticketsHashes {
		jsTicketsHashes[i] = js.ValueOf(h.String())
	}

	jsObject.Get("network").Call("monitorForSessionTransactions", jsSplitHash,
		jsTicketsHashes)
	return nil
}

func (c *jsWalletClient) PublishedSplitTx() bool {
	return jsObject.Get("publishedSplitTx").Truthy()
}

func (c *jsWalletClient) PublishedTicketTx() *chainhash.Hash {
	pt := jsObject.Get("publishedTicketTx")
	if pt.Type() != js.TypeString {
		return nil
	}

	h := new(chainhash.Hash)
	err := chainhash.Decode(h, pt.String())
	if err != nil {
		// Ignore errors here as these can only be triggered due to wrong usage
		// of the lib.
		return nil
	}

	return h
}

func (c *jsWalletClient) Close() error {
	// nothing to do
	return nil
}

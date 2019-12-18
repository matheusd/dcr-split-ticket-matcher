// +build js,wasm

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"syscall/js"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type jsMatcherClient struct{}

func (c *jsMatcherClient) WatchWaitingList(ctx context.Context, in *pb.WatchWaitingListRequest, opts ...grpc.CallOption) (pb.SplitTicketMatcherService_WatchWaitingListClient, error) {
	return nil, fmt.Errorf("not yet")
}

func (c *jsMatcherClient) FindMatches(ctx context.Context, in *pb.FindMatchesRequest, opts ...grpc.CallOption) (*pb.FindMatchesResponse, error) {
	out := new(pb.FindMatchesResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "findMatches", in, out)
}

func (c *jsMatcherClient) GenerateTicket(ctx context.Context, in *pb.GenerateTicketRequest, opts ...grpc.CallOption) (*pb.GenerateTicketResponse, error) {
	out := new(pb.GenerateTicketResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "generateTicket", in, out)
}

func (c *jsMatcherClient) FundTicket(ctx context.Context, in *pb.FundTicketRequest, opts ...grpc.CallOption) (*pb.FundTicketResponse, error) {
	out := new(pb.FundTicketResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "fundTicket", in, out)
}

func (c *jsMatcherClient) FundSplitTx(ctx context.Context, in *pb.FundSplitTxRequest, opts ...grpc.CallOption) (*pb.FundSplitTxResponse, error) {
	out := new(pb.FundSplitTxResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "fundSplitTx", in, out)
}

func (c *jsMatcherClient) Status(ctx context.Context, in *pb.StatusRequest, opts ...grpc.CallOption) (*pb.StatusResponse, error) {
	out := new(pb.StatusResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "status", in, out)
}

func (c *jsMatcherClient) BuyerError(ctx context.Context, in *pb.BuyerErrorRequest, opts ...grpc.CallOption) (*pb.BuyerErrorResponse, error) {
	out := new(pb.BuyerErrorResponse)
	return out, jsGrpcCall(jsObject.Get("matcher"), "buyerError", in, out)
}

func (c *jsMatcherClient) FetchSpentUtxos(tx *wire.MsgTx) (splitticket.UtxoMap, error) {

	reqOutpoints := make([]interface{}, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outp := make(map[string]interface{}, 3)
		outp["hash"] = in.PreviousOutPoint.Hash.String()
		outp["index"] = in.PreviousOutPoint.Index
		outp["tree"] = in.PreviousOutPoint.Tree
		reqOutpoints[i] = outp
	}

	reply, err := jsAsyncCall(jsObject.Get("network"), "fetchSpentUtxos",
		js.ValueOf(reqOutpoints))
	if err != nil {
		return nil, errors.Wrap(err, "error calling async js fetchSpentUtxos")
	}

	utxoMap := make(splitticket.UtxoMap)

	for _, in := range tx.TxIn {
		key := fmt.Sprintf("%s:%d:%d", in.PreviousOutPoint.Hash.String(),
			in.PreviousOutPoint.Index, in.PreviousOutPoint.Tree)
		utxoVal := reply.Get(key)
		if !utxoVal.Truthy() {
			return nil, errors.Errorf("js did not return utxo data for %s", key)
		}

		if utxoVal.Type() != js.TypeObject {
			return nil, errors.Errorf("return utxo val for %s not an object", key)
		}

		pkScript, err := hex.DecodeString(utxoVal.Get("pkscript").String())
		if err != nil {
			return nil, errors.Wrap(err, "error decoding pkscript from utxo")
		}
		value, err := dcrutil.NewAmount(utxoVal.Get("value").Float())
		if err != nil {
			return nil, errors.Wrap(err, "error decoding value from utxo")
		}
		version := uint16(utxoVal.Get("version").Int())

		utxoMap[in.PreviousOutPoint] = splitticket.UtxoEntry{
			PkScript: pkScript,
			Value:    value,
			Version:  version,
		}
	}

	return utxoMap, nil
}

func (c *jsMatcherClient) Close() {
	// nothing to do
}

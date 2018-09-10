package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"syscall/js"

	"github.com/pkg/errors"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"google.golang.org/grpc"
)

const replyObjectName = "responses"

type wasmMatcherClientAdapter struct {
	globalPrefix string
	callbacks    map[int32]chan struct{}
	lastID       int32
}

func newWasmMatcherClientAdapter() *wasmMatcherClientAdapter {
	res := &wasmMatcherClientAdapter{
		globalPrefix: "_splitTicketBuyer_matcherClient_",
		callbacks:    make(map[int32]chan struct{}),
	}
	return res
}

func (mc *wasmMatcherClientAdapter) callback(args []js.Value) {
	if len(args) < 1 {
		return
	}

	id := int32(args[0].Int())
	if replyChan, exists := mc.callbacks[id]; exists {
		replyChan <- struct{}{}
	}
}

func (mc *wasmMatcherClientAdapter) call(fname string, req, resp interface{}) error {
	id := atomic.AddInt32(&(mc.lastID), 1)

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return errors.Wrapf(err, "error marshaling json request")
	}

	callName := mc.globalPrefix + fname
	replyName := mc.globalPrefix + replyObjectName
	replyChan := make(chan struct{})
	mc.callbacks[id] = replyChan

	js.Global().Call(mc.globalPrefix+fname, id, string(reqBytes))
	<-replyChan
	delete(mc.callbacks, id)

	replyIDStr := fmt.Sprintf("%d", id)
	res := js.Global().Get(replyName).Get(replyIDStr)
	js.Global().Get(replyName).Set(replyIDStr, nil)

	println("got back", replyName, id, res.String())

	if res.Type() != js.TypeString {
		return fmt.Errorf("wasm convertion error: call %s did not return "+
			"a string", callName)
	}
	return json.Unmarshal([]byte(res.String()), resp)
}

func (mc *wasmMatcherClientAdapter) WatchWaitingList(ctx context.Context, in *pb.WatchWaitingListRequest, opts ...grpc.CallOption) (pb.SplitTicketMatcherService_WatchWaitingListClient, error) {
	return nil, errors.New("not yet")
}

func (mc *wasmMatcherClientAdapter) FindMatches(ctx context.Context, in *pb.FindMatchesRequest, opts ...grpc.CallOption) (*pb.FindMatchesResponse, error) {
	return nil, errors.New("not yet")
}

func (mc *wasmMatcherClientAdapter) GenerateTicket(ctx context.Context, in *pb.GenerateTicketRequest, opts ...grpc.CallOption) (*pb.GenerateTicketResponse, error) {
	return nil, errors.New("not yet")
}

func (mc *wasmMatcherClientAdapter) FundTicket(ctx context.Context, in *pb.FundTicketRequest, opts ...grpc.CallOption) (*pb.FundTicketResponse, error) {
	return nil, errors.New("not yet")
}

func (mc *wasmMatcherClientAdapter) FundSplitTx(ctx context.Context, in *pb.FundSplitTxRequest, opts ...grpc.CallOption) (*pb.FundSplitTxResponse, error) {
	return nil, errors.New("not yet")
}

func (mc *wasmMatcherClientAdapter) Status(ctx context.Context, in *pb.StatusRequest, opts ...grpc.CallOption) (*pb.StatusResponse, error) {
	out := &pb.StatusResponse{}
	err := mc.call("status", in, &out)
	println("status returned", out.ProtocolVersion, err)
	if err != nil {
		return nil, err
	}
	return out, nil
}

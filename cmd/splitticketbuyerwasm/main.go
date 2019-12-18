// +build js,wasm

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"syscall/js"

	"github.com/decred/dcrd/chaincfg"
	"github.com/golang/protobuf/proto"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// main object for interaction with js
	jsObject    js.Value = js.Undefined()
	jsObjectFns []js.Func

	startChan     chan struct{}
	interruptChan chan struct{}
)

func getSplitTicketBuyerObject(thisArg js.Value, args []js.Value) interface{} {
	if jsObject != js.Undefined() {
		return jsObject
	}

	configObj := js.Global().Get("Object").New()
	configObj.Set("walletHost", js.Null())
	configObj.Set("voteAddress", js.Null())

	newFn := func(f func(thisArg js.Value, args []js.Value) interface{}) js.Func {
		fn := js.FuncOf(f)
		jsObjectFns = append(jsObjectFns, fn)
		return fn
	}

	obj := js.Global().Get("Object").New()
	obj.Set("config", configObj)
	obj.Set("startBuyer", newFn(startBuyer))
	obj.Set("cancel", newFn(cancel))

	jsObject = obj
	return jsObject
}

func startBuyer(thisArg js.Value, args []js.Value) interface{} {
	close(startChan)
	return nil
}

func cancel(thisArg js.Value, args []js.Value) interface{} {
	close(interruptChan)
	return nil
}

func jsAsyncCall(obj js.Value, fname string, args ...js.Value) (js.Value, error) {
	var funcErr error
	result := js.Undefined()

	replyChan := make(chan struct{})
	resolveFunc := js.FuncOf(func(thisArg js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			result = args[0]
		}
		close(replyChan)
		return nil
	})

	rejectFunc := js.FuncOf(func(thisArg js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			panic("no reject data")
		}
		funcErr = errors.New(args[0].String())
		close(replyChan)
		return nil
	})

	callArgs := make([]interface{}, len(args)+2)
	callArgs[0] = resolveFunc
	callArgs[1] = rejectFunc
	for i, v := range args {
		callArgs[i+2] = v
	}

	obj.Call(fname, callArgs...)
	select {
	case <-replyChan:
	case <-interruptChan:
		funcErr = fmt.Errorf("interrupted")
	}
	resolveFunc.Release()
	rejectFunc.Release()

	return result, funcErr
}

// jsGrpcCall performs a grpc call wrapped in object obj and function fname. It
// encodes the in (request) message and decodes into the out (response) object.
func jsGrpcCall(obj js.Value, fname string, in, out proto.Message) error {
	var funcErr error

	replyChan := make(chan struct{})
	resolveFunc := js.FuncOf(func(thisArg js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			panic("no resolve data")
		}
		bts, err := hex.DecodeString(args[0].String())
		if err != nil {
			panic(err)
		}

		proto.Unmarshal(bts, out)
		close(replyChan)
		return nil
	})

	rejectFunc := js.FuncOf(func(thisArg js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			panic("no reject data")
		}
		funcErr = errors.New(args[0].String())

		if (len(args) > 1) && (args[1].Type() == js.TypeNumber) {
			// might be a grpc error
			errCode := args[1].Int()
			if errCode >= 0 && errCode <= 16 { // 16 is the last known grpc error code
				funcErr = status.Error(codes.Code(errCode), funcErr.Error())
			}
		}
		close(replyChan)
		return nil
	})

	reqData, err := proto.Marshal(in)
	if err != nil {
		return errors.Wrap(err, "error marshalling protobuf request")
	}

	obj.Call(fname, hex.EncodeToString(reqData), resolveFunc, rejectFunc)
	select {
	case <-replyChan:
	case <-interruptChan:
		funcErr = fmt.Errorf("interrupted")
	}
	resolveFunc.Release()
	rejectFunc.Release()

	return funcErr
}

func clearJSObject() {
	if jsObject == js.Undefined() {
		return
	}

	jsObject.Call("clearObj")

	for _, fn := range jsObjectFns {
		fn.Release()
	}
	jsObjectFns = nil
	jsObject = js.Undefined()
}

func buyerLoop() {

	cfgObj := jsObject.Get("config")

	isTestNet := cfgObj.Get("testnet").Truthy()
	chainParams := &chaincfg.MainNetParams
	if isTestNet {
		chainParams = &chaincfg.TestNet3Params
	}

	srcAccount := cfgObj.Get("sourceAccount").Int()
	if srcAccount < 0 {
		panic(errors.New("Cannot use negative source account"))
	}

	cfg := buyer.Config{
		WalletConn:        &jsWalletClient{},
		MatcherConn:       &jsMatcherClient{},
		SaveSessionWriter: &jsSessionWriter{},

		VoteAddress:   cfgObj.Get("voteAddress").String(),
		PoolAddress:   cfgObj.Get("poolAddress").String(),
		MaxAmount:     cfgObj.Get("maxAmount").Float(),
		Passphrase:    []byte(cfgObj.Get("passphrase").String()),
		MatcherHost:   cfgObj.Get("matcherHost").String(),
		SStxFeeLimits: uint16(0x5800),

		TestNet:              isTestNet,
		ChainParams:          chainParams,
		PoolFeeRate:          cfgObj.Get("poolFeeRate").Float(),
		MaxTime:              cfgObj.Get("maxTime").Int(),
		MaxWaitTime:          cfgObj.Get("maxWaitTime").Int(),
		SourceAccount:        uint32(srcAccount),
		SkipWaitPublishedTxs: cfgObj.Get("skipWaitPublishedTxs").Truthy(),
	}

	// FIXME: validate cfg

	reporter := buyer.NewWriterReporter(jsReporter{}, cfg.SessionName)
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)

	err := buyer.BuySplitTicket(ctx, &cfg)
	if err != nil {
		errorFn := jsObject.Get("error")
		if errorFn.Type() == js.TypeFunction {
			errorFn.Call(err.Error())
		} else {
			fmt.Println(err.Error())
		}
	}
}

func main() {
	// basic init
	startChan = make(chan struct{})
	interruptChan = make(chan struct{})

	fn := js.FuncOf(getSplitTicketBuyerObject)
	js.Global().Set("__getSplitTicketBuyerObject", fn)

	// wait for js config object
	select {
	case <-startChan:
		buyerLoop()
	case <-interruptChan:
	}

	// finalization
	js.Global().Set("__getSplitTicketBuyerObject", js.Undefined())
	fn.Release()
	clearJSObject()
}

// +build js,wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"
)

// TODO: actually implement
type jsSessionWriter struct {
	ticketHash string
	data       strings.Builder
}

func (w *jsSessionWriter) StartWritingSession(ticketHash string) {
	w.ticketHash = ticketHash
}

func (w *jsSessionWriter) SessionWritingFinished() {
	obj := js.Global().Get("Object").New()
	obj.Set("name", w.ticketHash)
	obj.Set("data", w.data.String())
	jsAsyncCall(jsObject, "saveSession", obj)
}

func (w *jsSessionWriter) Write(b []byte) (int, error) {
	return w.data.Write(b)
}

type jsReporter struct{}

func (w jsReporter) Write(b []byte) (int, error) {
	fmt.Print(string(b))
	return len(b), nil
}

# Building

Build using go 1.11 and go modules.

Before building the GUI client, you'll probably need some gtk headers installed:

```
# fedora
$ sudo dnf install gtk2-devel

# debian/ubuntu
$ sudo apt-get install libgtk2.0-dev libglib2.0-dev libgtksourceview2.0-dev
```

For everything else, follow the standard instructions as other decred projects. This project is using go modules, so you should preferably build *outside* of your $GOPATH.

```
$ git clone https://github.com/matheusd/dcr-split-ticket-matcher
$ cd dcr-split-ticket-matcher
$ go build ./cmd/...
```

# Building the Wasm Buyer

```
$ GOARCH=wasm GOOS=js go build -o cmd/splitticketbuyerwasm/splitticketbuyerwasm.wasm ./cmd/splitticketbuyer.wasm
```

You can test the wasm buyer with the sample index.html by serving the contents of the `splitticketbuyerwasm

# Running the Client

See instructions about [command line client](client-cli.md) or the [GUI client](client-gui.md).

# Running the Service

Copy the configuration file to the respective dir and configure it:

```
$ mkdir -p ~/.dcrstmd
$ cp samples/dcrstmd.conf ~/.dcrstmd
```

Run the executable. During development, it is easier to do the following:

```
$ go run cmd/dcrstmd/main.go
```

Run with `--publishtransactions` to automatically publish the split and ticket transactions once a session is successfully finished.

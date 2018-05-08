# Building

Standard instructions as other decred projects.

```
$ mkdir -p $GOPATH/src/github.com/matheusd/dcr-split-ticket-matcher
$ cd $GOPATH/src/github.com/matheusd/dcr-split-ticket-matcher
$ git clone https://github.com/matheusd/dcr-split-ticket-matcher .
$ dep ensure
$ go build ./cmd/...
```

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

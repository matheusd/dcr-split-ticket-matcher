# DCR Split Ticket Matcher Service

Alpha vesion of a split ticket matcher service. [Design](/docs/design.md).

See it in action:

[video - multiple decrediton instances in split ticket](https://streamable.com/qnfsm)

[Published Testnet tx](https://testnet.dcrdata.org/tx/134c53c84bdf914e21b9fb04dadcbf178e4de4e2b7d225f9c2e91ec5c60787d7)

## Current Limitations

- Only testnet
- No spam protection
- Does not publish the transactions automatically
- Only for stakepool voters (not solo voters)

## Building

Standard instructions as other decred projects.

```
$ mkdir -p $GOPATH/src/github.com/matheusd/dcr-split-ticket-matcher
$ cd $GOPATH/src/github.com/matheusd/dcr-split-ticket-matcher
$ git clone https://github.com/matheusd/dcr-split-ticket-matcher .
$ dep ensure
$ go build ./cmd/dcrstmd
$ go build ./cmd/splitticketbuyer
$ go build ./cmd/watcher
```

## Running the Service

Just run the executable. Easiest to do the following during development:

```
$ go run cmd/dcrstmd/main.go
```

This is still in alpha, so the service doesn't have config files yet. It will create a `./data` subdir with the tls key and cert files, plus log dir.

## Running the Buyer

First, you need to copy the config file into the corresponding dir:

```
$ mkdir -p ~/.splitticketbuyer
$ cp samples/splitticketbuyer.conf ~/.splitticketbuyer.conf
```

Edit the config file according to your needs. At the very least, you'll need to fill `VoteAddress` with the voting address of your stake pool ("ticket address" on Decrediton) and `PoolAddress` with the subsidy address of your stakepool.

You will also need to grab the `rpc.cert` file from the service you want to connect to and put it on that same dir. If you're running the service locally:

```
$ cp ./data/rpc.cert ~/.splitticketbuyer/rpc.cert
```

Run the buyer normally. If testing purchasing with multiple wallets, you can create a different a different config file and run:

```
$ splitticketbuyer -C ./other-wallet.conf
```

You can also pass various arguments to control the options:

```
$ splitticketbuyer --maxamount 10 --maxwaittime 120
```

The buyer asks for the wallet's private passphrase in order to sign the ticket purchase. You can provide it at the command line (with the `--pass` argument - not recommended), at the config file (**definitely** not recommended), via stdin (currently echoes the characters - not recommended) or via promptsecret (recommended):

```
$ promptsecret | splitticketbuyer
```

Once you run the buyer, it will attempt to join a split ticket purchase. If a session isn't started in `MaxWaitTime` seconds (default: 120) it will automatically stop.

If a session is started, the buyer will go through the steps required to purchase the split ticket. If the session isn't completed in `MaxTime` seconds (default: 30) it is assumed as failed and closed. You'll need to run the buyer to try again.

Once a session is completed, the buyer will dump the raw transaction data for the split transaction, the ticket purchase and the future revocation transaction.

The current alpha version of the service does not automatically publish the transactions to the decred network. This is being done for debugging reasons, in order to allow manual transaction conference before submitting to the network. The beta version should automatically publish the transactions.


## Next Steps

- Writeup of the general challenges of ticket splitting
- Writeup on the different alternatives for moving on with the development
- When Roadmap
- ???
- :rocket::moon:

# StakePool Setup

This doc gives some instructions for setting up the matcher service for stakepool operators. It is current as of the beta (0.4.x/0.5.x) line of releases.

Most config options are self-explanatory. The ones listed below are the critical ones.

## Vote Address Validation

To validate that ticket (voting) addresses are usable by the pool, the service needs to connect to a wallet that has access to the scripts and addresses of the pool.

This is achievable on stakepools by having the split ticket service connect to a dcrwallet connected to a stakepoold instance. This wallet does **not** need to be unlocked and enabled for voting, but **must** receive the scripts for new users, so that their voting addresses are correctly validated.

## Pool Subsidy Address Validation

To validate that the address of the pool subsidy/pool fee is redeemable by the stakepool operator, the split ticket service should be configured with its master pub key in the `PoolSubsidyWalletMasterPub` setting.

When this option is filled, a list with the first 10k addresses of the wallet is generated and the pool fee address of each participant is checked during session setup.

Stakepools should use the same value as the `coldwalletextpub`.

## Pool Fee Intermediate Output Signing

Due to the layout of stake transactions, the pool fee needs to exist as a separate output of the split transaction, with a corresponding input on the ticket.

This movement of funds needs to be signed, but is only temporary, with the utxo existing only during the time between the split and ticket transactions are mined.

For simplicity, a single private key is used by the matcher service to sign the movement of these funds. This needs to be configured in the `SplitPoolSignKey` config setting.

Stakepools should use a private key that they have access to, in case of an error that requires redeeming these funds. For example, use a private key derived from the cold pool fee wallet.

To grab the private key for a given wallet, use the following:

```
$ dcrctl --wallet getnewaddress
( write down the address )
$ dcrctl --wallet dumpprivkey [previous-address]
( write down the private key - starts with Pt on testnet and Pd on mainnet)
```

## TLS Encryption

The buyer uses mandatory TLS encryption and the matcher service won't run without certificates.

On the first run, the daemon will create self signed certificates that can be used in clients by specifying the `MatcherCertFile` in the config file.

For production use in general clients, you need to get actual certificates signed by a CA. The easiest setup is to use Let's Encrypt and specify the `CertFile` and `KeyFile` config entries of the service to the corresponding files.

If you install the matcher daemon as a service (using systemd, upstart, etc), you'll need to restart the service after updating the key and certificate files (usually with a `--post-hook` on `certbot`).

## Service Discovery

To ease deployment, the buyer currently looks for a predefined SRV record on the stakepool domain. If that record is found, the buyer tries to connect to that address using grpc with mandatory tls host validation.

If your stakepool host is `foobar-stakepool.example.com`, then the SRV record you need to add to your DNS zone is `_split-tickets-grpc._tcp.foobar-stakepool.example.com`. Specify the port the service is running (usually 8475 on mainnet and 18745 on testnet) and default priority, weight and TTL values.

An example record:

```
_split-tickets-grpc._tcp.foobar.example.com. 43200 IN SRV 10 100 8475 mainnet-split-tickets.foobar.example.com.
```

This means a buyer can be configured with just the stakepool domain and the service should magically work, not requiring any changes in either stakepool or decred apis or on the virtual servers of the stakepool. It also trivially allows to run the split ticket service in a different server.

**NOTE**: the TLS certificate that the service runs must be for the **target** domain (eg: `mainnet-split-tickets.foobar.example.com`) **not** for the apex or main stakepool domain.

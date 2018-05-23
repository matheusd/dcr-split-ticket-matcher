# StakePool Setup

This doc gives some instructions for setting up the matcher service for stakepool operators. It is current as of the beta (0.4.x/0.5.x) line of releases.

## Address Validation

To validate that ticket and pool subsidy addresses belong to the pool, the wallet the matcher connects to must belong to the pool and must be a copy of the wallet that has voting rights.

**TODO** check if this is a good thing. Maybe we should have 2 wallets: one just for verifying addresses and one for signing the pool fee (for security reasons).

## TLS Encryption

The buyer uses mandatory TLS encryption and the matcher service won't run without certificates.

On the first run, the daemon will create self signed certificates that can be used in clients by specifying the `MatcherCertFile` in the config file.

For production use in general clients, you need to get actual certificates signed by a CA. The easiest setup is to use Let's Encrypt and specify the `CertFile` and `KeyFile` config entries of the service to the corresponding files.

If you install the matcher daemon as a service (using systemd, upstart, etc), you'll need to restart the service after updating the key and certificate files (usually with a `--post-hook` on `certbot`).

## Service Discovery

To ease deployment, the buyer currently looks for a predefined SRV record on the stakepool domain. If it finds, it tries to connect to that address using grpc with mandatory tls host validation.

If your stakepool host is `foobar-stakepool.example.com`, then the SRV record you need to add to your DNS zone is `_split-tickets-grpc._tcp.foobar-stakepool.example.com`. Specify the port the service is running (usually 8475 on mainnet and 18745 on testnet) and default priority, weight and TTL values.

An example record:

```
_split-tickets-grpc._tcp.foobar.example.com. 43200 IN SRV 10 100 8475 mainnet-split-tickets.foobar.example.com.
```

This means a buyer can be configured with just the stakepool domain and the service should magically work, not requiring any changes in either stakepool or decred apis or on the virtual servers of the stakepool. It also trivially allows to run the split ticket service in a different server.

**NOTE**: the TLS certificate that the service runs must be for the **target** domain (eg: `mainnet-split-tickets.foobar.example.com`) **not** for the apex or main stakepool domain.

# Third Party Hosting of Matcher Service

While the split ticket service is in a [beta phase](beta.md), I ([matheusd](https://www.matheusd.com)) can optionally
run the matcher service for a few select voting pools. If you're a voting pool
operator, read this document and then [contact me](https://www.matheusd.com/about).


## Why?

While the service is in beta, I can keep a closer eye at any problems while
running it. Plus, it's much easier to understand the required setup steps for
the voting pool integrator vs the full matching service.

## Why Not?

I'd rather voting pool operators ran their own services. I don't plan on keeping
up with this (running my own service) after this is effectively live in
production, so if you're a voting pool operator, it's better to get used to
running it if you want to provide the option of split tickets to your users.

## How?

In order to validate pool subsidy and vote addresses against your pool, you need
to run a stakepoold integrator service in one (only one) of your stakepoold
services.

I'll run the matcher service and configure it to connect to this integrator
service. During address validation stage of the matching process, I'll make
requests to this integrator to ensure that all participants are from your own
pool.

The following sections list the configuration steps required to do this.

### Run the Integrator

Download one of binary releases of the integrator, or better yet, build it from
source. Then just run it on on one of your stakepoold nodes.

It should work out of the box, and there's not much to it: it reads
`~/.stakepoold/stakepoold.conf` to get the extended master public key for the
pool subsidy wallet and the credentials to connect to the locally running
wallet. It then opens up a grpc service listening on port `9872`.

### Grab the RPC and Contact Me

Grab the file `~/.stmvotepoolintegrator/rpc.cert`. You'll need to send me this
file over a secure channel and we'll need to work out a few details, so contact
me before proceeding (keep reading though, to know all the steps required after
this one).

### Configure the DNS Records

You'll receive a DNS address of where your matcher service will run. You'll need
to configure two DNS records on your voting pool's zone: one SRV record for
[service discovery](stakepool-setup.md/#service_discovery) and one CNAME from
the target of the SRV record to the domain I'll provide you.

### Configure the firewall

You'll need to allow traffic on port `9872` on your firewall **ONLY** from the
IP address your matcher service will run on (I'll provide you the IP). Then
send me either the IP address or a domain for the stakepoold running server.


## And then?

That's it. I'll run the matcher service, connect to the intergrator on your end
and start validating the addresses.

Please make a plan to run the matcher service yourself though.

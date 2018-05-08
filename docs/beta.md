# First Beta Phase

The matcher service and split ticket buyer are currently on their first beta phase. This means the following:

- The code mostly works and does what it's supposed to do
- There are binaries available for command line and GUI buyers
- The code has **not** been extensively vetted by automated tests and more experienced community members
- There haven't been many transactions/votes done by it *yet*

This is a "first phase" beta because the software isn't yet integrated to the stakepool software to close some of the possible tricks a malicious participant or matcher service may use in order to fee drain or lock other participants out of voting.

The "second phase" beta will happen after stakepool integration is completed.

## Risks in joining the beta

There are many risks in using the software and performing the split ticket purchase, so in order to join the beta you should be aware of and understand those risks. As a start:

> This software creates and signs transactions by connecting to your wallet, so bear in mind that **any** software that does that is a possible vector for draining your funds. **USE AT YOUR OWN RISK**.

The following list shows the **known** possible ways that participating in a split ticket buying session in the current version of the software can cause problems for your wallet/funds:

- A malicious participant may use a vote address not assigned to any pool, thus locking your funds for up to 4 months, costing you the transaction fees used;
- A malicious participant may use a pool subsidy address not controlled by the respective pool, with the same consequences as above;
- A well connected (in network terms) malicious participant may cause fee drain by publishing transactions in a very specific way;
- Pool fee is fixed at 5% (on mainnet) and 7.5% (on testnet), so if you use a pool with lower fees you'll spend more than what was strictly needed to vote with that pool. You cannot use pools with higher fees.

Lastly, while there are many checks to ensure that the split and ticket transactions are correct, will be successfully published and don't spend more funds than what is required, this is still beta software and there might be bugs or checks missing that allow a malicious matcher or participant to drain funds from you.

## Managing current risks

In order to participate in the beta, you should take steps to manage the previously disclosed and the unknown possible risks. Some of those steps are listed below:

> Use a different wallet than your main one for participating in the sessions.

This is by far the most useful action you can take to limit any possible damage and reduce future regret. This means you'll need to create a new wallet and register to a new pool account with this wallet, but at the very worst only the funds in this second wallet are compromised.

> Only buy split tickets with trusted friends.

Due to not yet being connected to stakepools, the matcher service is vulnerable to an attack where a malicious participant in a ticket buying session might provide a false voting or pool subsidy address. In that case, the funds will be locked until the vote is missed or the ticket expires, plus the pool fee and transaction fees will be deducted from your wallet anyway.

Therefore, you should only join sessions where you know and trust the other participants not to perform this attack. This is accomplished by using a *session name* when connecting to the service.

Currently, public sessions (i.e. using an empty session name) are **disabled** in the mainnet service.

> Run binaries in a VM or container.

While I've taken steps to protect my building servers, I don't consider they are at the same high levels of security as the ones provided by the original decred devs (C0 et al).

So if you are concerned about my binaries doing anything funny, you should run them on a VM/container/separate machine.

> Build and run your own binaries.

If you're **really** concerned about my binaries, you should [compile and build your own](building.md). If you do so (or even better, review some of the code), please drop me a line privately ([contact info available on my website](https://matheusd.com)) or at the [decred slack](https://slack.decred.org) (username: `matheusd`).

# Joining the Beta

So, have **really** read about the risks of joining the beta and how to manage them? Here's the last warning:

> This software creates and signs transactions by connecting to your wallet and is of beta quality, not yet thoroughly vetted by the larger decred community. **USE AT YOUR OWN RISK**.

[Running the GUI client](client-gui.md)

[Running the CLI client](client-cli.md)

# Second Beta Phase

The matcher service and split ticket buyer are currently on their second beta phase. This means the following:

- The code mostly works and does what it's supposed to do
- The service does not take ownership of your funds
- There are binaries available for command line and GUI buyers
- There are some known issues with integration to the wallet
- The code has **not** been extensively vetted by automated tests and more experienced community members

This is a "second phase" beta because the software has been integrated to a few select voting pools and therefore users using one of these pools can be assured that addresses are validated by them.

## Available Pools

This is a list of pools that are running the split ticket service. Please note that all participants must use the same pool with the same session name in order to complete the matching session.

- https://stake.decredbrasil.com
- https://decredvoting.com

You can see currently running sessions at [mainnet-split-tickets.matheusd.com](https://mainnet-split-tickets.matheusd.com).


## Risks in joining the beta

There are many risks in using the software and performing the split ticket purchase, so in order to join the beta you should be aware of and understand those risks. As a start:

> This software creates and signs transactions by connecting to your wallet, so bear in mind that **any** software that does that is a possible vector for draining your funds. **USE AT YOUR OWN RISK**.

The following list shows the **known** possible ways that participating in a split ticket buying session in the current version of the software can cause problems for your wallet/funds:

- A well connected (in network terms) malicious participant may cause fee drain by publishing transactions in a very specific way (see the doc about [racing fee draing](race-fee-drain.md));

While there are many checks to ensure that the split and ticket transactions are correct, will be successfully published and don't spend more funds than what is required, this is still beta software and there might be bugs or checks missing that allow a malicious matcher or participant to drain funds from you.

## Managing current risks

In order to participate in the beta, you should take steps to manage the previously disclosed and the unknown possible risks. Some of those steps are listed below:

> Use a different wallet than your main one for participating in the sessions.

This is by far the most useful action you can take to limit any possible damage and reduce future regret. This means you'll need to create a new wallet and register to a new pool account with this wallet, but at the very worst only the funds in this second wallet are compromised.

> Only buy split tickets with trusted friends.

While buying with a service integrated to a voting pool should be reasonably safe, as all ticket and pool addresses are validated before the session starts, you might still prefer buying along with trusted friends to ensure no one is cheating in any unforseen way.

> Run binaries in a VM or container.

While I've taken steps to protect my building servers, I don't consider they are at the same high levels of security as the ones provided by the original decred devs (C0 et al).

So if you are concerned about my binaries doing anything funny, you should run them on a VM/container/separate machine.

> Build and run your own binaries.

If you're **really** concerned about my binaries, you should [compile and build your own](building.md). If you do so (or even better, review some of the code), please drop me a line privately ([contact info available on my website](https://matheusd.com)) or at the [decred slack](https://slack.decred.org) (username: `matheusd`).

# Joining the Beta

So, have you **really** read about the risks of joining the beta and how to manage them? Here's the last warning:

> This software creates and signs transactions by connecting to your wallet and is of beta quality, not yet thoroughly vetted by the larger decred community. **USE AT YOUR OWN RISK**.
>
> This software is provided on an "as is" and "as available" basis. The developers do not give any warranties, whether express or implied, as to the suitability or usability of the the software or any of its content.
>
> The developers will not be liable for any loss, whether such loss is direct, indirect, special or consequential, suffered by any party as a result of their use of the Split Ticket software or content. All uses are done at the user’s own risk and the user will be solely responsible for any damage to any computer system or loss of data or loss of funds that results from such activities.

[Running the GUI client](client-gui.md)

[Running the CLI client](client-cli.md)

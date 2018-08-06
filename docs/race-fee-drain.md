# Race Fee Drain Vulnerability

The current version of the split ticket service has a race vulnerability that allows a malicious participant to fee drain honest participants. While it's unlikely to be explored in practice and has a response action that further discourages its use, participants must be aware of it.

## Triggering the Vulnerability

The inputs for the split ticket come from a single transaction constructed with funds from all participants (a so called _split transaction_).

This transaction must be published and mined in the blockchain before the ticket itself is mined.

The current protocol for building a split ticket ensure that the **ticket** is fully funded and valid before the split tx is, such that both transactions can be published at the same time by the matcher.

**However**, the current API for decred full nodes only accept a single transaction for publishing. While both transactions can be published in sequence and remain in the mempool (eventually being mined in the correct order) there is potentially a small delay arising from the fact that they are published in sequence vs concurrently.

This propagation delay means that a malicious participant with a better connectivity to the decred network (than the matcher service) could race the service by waiting for the split transaction to be published and then publishing a transaction spending from the output intended to go to the ticket **before** the ticket is actually sent, causing it (the ticket) to become a double spend and not be accepted.

The end result for this would be that the honest participants get their transaction and pool fee drained by a malicious participant.

## Response Action

In case this attack is detected, split ticket service operators, voting pools and participants can respond on the following way:

- Identify the malicious participant (if the matching is done among known people) by going over the logs (this is particularly possible for voting pools by identifying the user account associated with a given voting address)
- Refund the pool fee to the original participants in their corresponding proportions

## Fixing the Vulnerability

The simplest way to reduce the chances of this vulnerability being exploited is to have the matcher service have a good connectivity to other decred nodes.

Another improvement would be to support concurrent transaction publishing in the network, which would allow the split and ticket transactions to be relayed at the same time, removing the possibility of forcing the ticket to become a double spend.

Finally, the best possible fix would be to make the inputs of the ticket transaction spend from a multisig utxo, such that all participants must agree to the spending by contributing to the signature. However this fix requires a number of changes to the split ticket protocol and doing a traditional n-of-n multisig for a large split (eg: a 10-of-10) would greatly increase the size of the ticket transaction. Working Schnorr signatures are required to implement this.

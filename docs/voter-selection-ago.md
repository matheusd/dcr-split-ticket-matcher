# Voter Selection Algorithm

This document details the voter selection algorithm of a split ticket session.

## Motivation

Allowing the purchase of split tickets in the decred network may introduce a systemic vulnerability that allows a big decred holder to increase its influence beyond what is possible by buying single tickets (*influence amplification*).

This might happen if the the voting criteria used by the split ticket matcher service allows a well-funded attacker to strategically buy portions of a ticket that grant them the voting rights of the full ticket while denying other participants their voting power.

The only viable strategy for granting voting rights to a participant of a split ticket that does not introduce this risk is to randomly select one of the participants in a uniform fashion, weighed by their percentage contribution to it. In other words, someone purchasing 60% of a ticket should have only 60% chance of being selected as the voter of that ticket.

That leaves open the question of **how** to select the voting participant, among those contributing to a split ticket, in a manner that does not introduce influence amplification concerns.

In particular, a naive implementation of voter selection (matcher service provider randomly choosing a participant) is vulnerable to a malicious matcher always choosing a specific (possibly confederate) participant.

Therefore we need to design a voter selection algorithm that randomly chooses the voter in a determinisc way, while preventing any one participant (or matcher) to unilaterally influence the choice.

## The Algorithm

The voter selection algorithm uses a round of communication injected at the start of the split ticket matching workflow. The general algorithm is the following:

  1. Participants generate a random number (secret) and hash this secret number;
  2. Participants send the hash of the secret to the matcher service; the service replies with the hashes of all participants;
  3. The participants reveal their secret numbers to the matcher; the matcher replies with the secret numbers of all participants;
  4. The matcher and each participant can now discover the voting participant by the following subroutine:
    a. The secret numbers are concatenated into a single bit string;
    b. The hash of this bit string is calculated;
    c. The coin selected to vote is calculated as the previous hash modulo the ticket price;
    d. The voting participant is the owner of the coin with the index indicated by the previous step

There are many fine points to address in the algorithm implementation, but the high level overview is that participants pre-commit to a given voter before knowing who exactly is the voter.

They only reveal their secret numbers and discover the chosen voter after all participants have committed to the split. While a malicious matcher or subset of participants could craft any combination of secret numbers, a single honest participant contributing their random secret into the final hash will ensure that the selected coin is random (assuming that the hashing function is cryptographically secure).

## Implementation Notes

### Secret number selection

The secret number should be a random number selected in a cryptographically secure way and replaced at every session. This must be done to prevent a malicious matcher or participant observing the same repeated number from crafting the other numbers to ensure a vote choice.

If the secret number is small (in bit size), the hash should also be salted with a previously known and agreed upon bit string, to prevent use of rainbow tables. One possible alternative for salt is the hash of the mainchain tip.

### Sorting the participants

Before starting the algorithm, the participants must be pre-sorted into a list, such that the following hash calculations of the concatenated data follow the same sequence for all participants.

This can be a random shuffle of the participants. The only requirement is that all participants must be aware of the order at the start of the algorithm (at step 2) so that any attempt to change the voter is detected.

### Indexing the participant's coins

After sorting the participants, their coins can be considered as ordered as slices of the full ticket. The contributions sum up to the ticket price, and each participant owns the coins in the range `[n, n + contribution amount)`.

Example of ordering of contributions (for an 87 DCR ticket):

| Participant Index | Contribution Amount | Cumulative Sum |
| ----------------- | ------------------- | -------------- |
| 0                 | 10 DCR              | 10 DCR         |
| 1                 | 23 DCR              | 33 DCR         |
| 2                 | 39 DCR              | 72 DCR         |
| 3                 | 15 DCR              | 87 DCR         |

If the calculated hash at step 4 is `0x7bac17c36bfef596`, then the selected coin (mod 8700000000) is 7757301654 (~77.57 DCR), which belongs to the participant with index 3.


### Voter Identification

During step 2, along with the hash of the secret number, the matcher should provide a hash of the voting address for each participant and the exact contribution amount.

Then, before funding the ticket transaction all participants can check that the address actually is of the selected voter by comparing the hash of the ticket address (output 0 of the sstx) to that of the selected voter as calculated by step 4.

### Technical Parameters

The suggested technical parameters for calculating the hashes are:

  - 64 bit random secret
  - blake2b hashing function
  - Hash at step 1 as `blake2b(secret, salt=[mainchain tip hash], size=256)`
  - Hash at step 4 as `blake2b([concatenated secrets], salt=[mainchain tip hash], size=64`)

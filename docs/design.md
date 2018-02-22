# Ticket Matcher Service Design

## Glossary

- **Participant**: A wallet contributing some amount of DCR to the ticket
- **Session**: A group of participants in the process of creating a shared ticket
- **Matcher**: The backend service that collects outputs and generates the ticket

## General Flow


![Session Execution](matcher-flow.png)

- `Participate()` informs the matcher service that a wallet can contribute at most X DCR
  - The matcher waits for as many participants as needed, until the ticket price is met
  - Replies with the ticket amount + fees that the participant will enter session
- `AddOutputs()` sends the sstxsubmission (amount from previous step) and sstxchange (usually 0) outputs for the participant
  - The matcher waits for all participants to send their outputs and checks the validity of them
  - After all outputs are received, the ticket is generated (vote address is randomly chosen among participants)
- `Publish()` sends the split tx that contains the utxo contribution of the participant (ticket amount + fees) and the scriptSig for the ticket input
  - The matcher waits for all participants to send their inputs
  - The matcher publishes the splitTxs and the ticket

## Notes

- The scriptSig for each participant must be `SigHashAll | SigHashAnyoneCanPay` so that the the inputs can be created concurrently
- Sending the SplitTx is necessary to confirm that the ticket can be funded
- A malicious matcher service can cause the fees for the split tx to be spent, but cannot redirect the outputs to somewhere else (as long as the participant confirms its output is present on the ticket)

## Issues

- Since the voting address must be decided during ticket creation, there is opportunity for spamming the service and for locking away a participant's funds
  - How to prevent that? Maybe run on this service on existing stakepools
- If the voting address is from a stakepool, how to to pay its fees?
  - Maybe create a single splittx that creates an input to pay the stakepool?
- How to prevent spam?
  - Maybe on `Participate()` the users send and sign the UTXOs that sum up to max participation amount?
- Any way of preventing fee drain?
  - Maybe with a single splitTx?

# Split Ticket Buyer GUI

Before running the software, please read the [**important information about the beta**](beta.md).

[Download a binary release](https://github.com/matheusd/dcr-split-ticket-matcher/releases/latest) or [build your own](building.md).

Run the `splitticketbuyergui` executable. You also need to keep your wallet  (either dcrwallet or decrediton) running while you join a session.

## Configuring

You need to configure the options before joining a split ticket service. You may do this manually or by trying to load a config from dcrwallet/decrediton.

### Loading Options from Decrediton

You need Decrediton version *1.2.1* or later to run the buyer with a decrediton wallet. Make sure the stakepool api has been imported and that the rescan has been completed.

On the main menu, click on Config -> Load from Decrediton. You'll see the list of available wallets. Select the one you want to buy tickets from, and the config will be filled.

Verify if the vote address and stakepool address are correct.

**IMPORTANT**: This process will load the addresses for the first configured stakepool it finds. If you want to use a different stakepool, you'll need to [manually change the config file](config-client.md).

### Loading Options from dcrwallet

Any version > 1.0 of dcrwallet should work.

On the main menu, click on Config -> Load from dcrwallet. The app will try to load the config from `~/.dcrwallet/dcrwallet.conf`.

If the voting or pool address aren't detected, you'll nedd to [manually change it](config-client.md).

### Manual Configuation

If the automated processes fail to pick up one of the settings, you won't be able to participate in the split ticket buying session. If that happens or if you need to fix a particular setting, see the options for [manually configuring the buyer](config-client.md).


## Joining a Session

To participate in a split ticket buying session, first you'll need to find other people and agree on a private session name.

Every participant should fill the session name exactly the same (the name is case sensitive). Agree on a reasonably unguessable session name (such as "My-Group-[random-string-of-digits]") to prevent someone from accidentally joining.

Every participant needs to be online and with their wallets running for the matching to occur. Also, you need to type the wallet's private passphrase into the box, so that the buyer can sign the appropriate transactions.

Select the maximum amount of decred you wish to contribute to the split transaction. If you set an amount higher than what is spendable by the wallet, the session will fail.

After all participants are online, click on "participate". There is a maximum waiting time (default: 120 seconds) and if not enough participants join in that time window, the session is canceled.

**IMPORTANT**: the session automatically starts after participants with enough funds to buy a single ticket connect to it, so the group might need to agree on participation amounts beforehand to allow everyone to participate.

## Session Success

If all participants join and a session starts, the individual buyers will go through the algorithm for creating a split ticket purchase. If no errors are found, the split transaction and ticket purchase transaction will be published and the ticket will go through its regular lifecycle.

# Split Ticket Buyer CLI

Before running the software, please read the [**important information about the beta**](beta.md).

[Download a binary release](https://github.com/matheusd/dcr-split-ticket-matcher/releases/latest) or [build your own](building.md).

Run the `splitticketbuyer` executable. You also need to keep your wallet  (either dcrwallet or decrediton) running while you join a session.

## Configuring

Run the buyer once without any options (`splitticketbuyer`) to create a default config file (and try to load options from an existing dcrwallet install).

You'll then need to [manually configure](config-client.md) any empty options of the buyer in order to join a session. You can also specify options as command line arguments when you execute the buyer.

When using the CLI client, it's usually helpful to configure only the most common options on the config file and leave the session dependent options for the command line arguments.

You'll probably want to leave the following options blank on the config file, and specify them at runtime:

- `sessionname`: Name of the session to join
- `maxamount`: Maximum participation amount

As usual, you can use `-h` to see available arguments.

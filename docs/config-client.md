# Manual Buyer Configuration

The default buyer config file is stored in a file named `splitticketbuyer.conf`. The location of this file is OS-dependent:

| OS      | Data directory                                            |
| -------:|:---------------------------------------------------------:|
| Windows | `C:\Users\<your-username>\AppData\Local\splitticketbuyer` |
| Linux   | `~/.splitticketbuyer`                                     |

**WINDOWS USERS**: Please use [Notepad++](https://notepad-plus-plus.org/) or similar software (**NOT** the default windows notepad application) to edit the file, as otherwise the line endings may be incorrect.

Running the command line buyer for the first time will automatically create the file. Running the GUI client, you have the option of creating the file based on either the dcrwallet or decrediton configs (if they are installed on the machine).

The default config file is created based on the contents of [this file](/pkg/buyer/config-defaults.go) and should be quite self explanatory. The most important config settings are:

- **VoteAddress**: Also sometimes called "ticket address". This is the address for voting with the stakepool.
- **PoolAddress**: Also called "pool subsidy address" or "pool fee address". This is the address where the pool receives its fees.
- **MaxAmount**: Maximum amount to contribute on a split ticket session (including all fees).
- **Pass**: Password for the wallet. When using the command line client, use "-" to read the password from stdin and use the `promptsecret` app to securily provide the password:

```
$ promptsecret | splitticketbuyer
```

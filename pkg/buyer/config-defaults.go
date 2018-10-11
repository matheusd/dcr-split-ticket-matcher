package buyer

const defaultConfigFileContents = `
# Complete the blank config options in order to perform the purchase.

[Application Options]

# Vote address used in the stakepool (Tc... on testnet or Dc... on mainnet)
VoteAddress =

# Pool fee destination address  (usually Ts... on testnet and Ds... on mainnet)
PoolAddress =

# Account number to get funds for the split ticket transaction (defaults to
# account 0)
# SourceAccount = 0

# Maximum amount (in DCR) to participate in the split purchase
MaxAmount = 0.0

# Pool subsidy fee rate (as a percentage). The buyer stops the session if the
# the service attempt to use a rate higher than this.
PoolFeeRate = 5.0

# Address of the matcher daemon.
# Voting Pools that provide the split ticket matching service can usually be
# used just by specifying its host (eg: stake.myexamplepool.com).
# MatcherHost = testnet-split-tickets.matheusd.com:8475
MatcherHost =

# Location of the matcher rpc.cert file when connecting to a custom matcher.
# MatcherCertFile = ~/.splitticketbuyer/matcher.cert

# 1 = TestNet, 0 = MainNet
TestNet = 0

# Network address of the wallet (dcrwallet) instance that will purchase tickets.
# Use the grpc port for this.
# If set to "127.0.0.1:0", then the buyer will attempt to find the wallet
# running in the localhost by enumerating all open tcp ports, connecting to
# it and checking if there is a wallet runnning on the correct network.
WalletHost = 127.0.0.1:0

# Full path to the rpc.cert file of the wallet
WalletCertFile =

# Default dir to save session data. Defaults to $HOME/.splitticketbuyer/data
# DataDir = ~/.splitticketbuyer/data

# Private passphrase of the wallet (use "-" to read from stdin)
# Use a command such as "promptsecret | splitticketbuyer" for better security
#
# *** DO NOT STORE THE PASSPHRASE OF PRODUCTION WALLETS HERE ***
Pass = -

# Dcrd connection options. Complete as needed.
DcrdHost =
DcrdUser =
DcrdPass =
DcrdCert = /home/user/.dcrd/rpc.cert
`

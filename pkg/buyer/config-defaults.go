package buyer

const defaultConfigFileContents = `
# Complete the blank config options in order to perform the purchase.

[Application Options]

# Vote address used in the stakepool (Tc... on testnet or Dc... on mainnet)
VoteAddress =

# Pool fee destination address  (usually Ts... on testnet and Ds... on mainnet)
PoolAddress =

# Maximum amount (in DCR) to participate in the split purchase
MaxAmount = 0.0

# Address of the matcher daemon.
# Online testnet matcher service. You'll need to use the testnet-matcher-rpc.cert
# file to connect to this service.
# MatcherHost = testnet-split-tickets.matheusd.com:8475
MatcherHost =

# Location of the matcher rpc.cert file. Defaults to $HOME/.splitticketbuyer/matcher.cert
MatcherCertFile = ~/.splitticketbuyer/matcher.cert

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
DataDir = ~/.splitticketbuyer/data

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

var testnetMatcherRPCCert = `
-----BEGIN CERTIFICATE-----
MIICfjCCAd+gAwIBAgIRAKopofGharfvO70tNQYERewwCgYIKoZIzj0EAwQwOzEf
MB0GA1UEChMWU3BsaXQgVGlja2V0IEJ1eWVyIE9yZzEYMBYGA1UEAxMPZGVjcmVk
LXRlc3RuZXQyMB4XDTE4MDMyNjIwNDgxNFoXDTI4MDMyNDIwNDgxNFowOzEfMB0G
A1UEChMWU3BsaXQgVGlja2V0IEJ1eWVyIE9yZzEYMBYGA1UEAxMPZGVjcmVkLXRl
c3RuZXQyMIGbMBAGByqGSM49AgEGBSuBBAAjA4GGAAQA3zm4TdN64uAzuGOvDzof
HBrtPFDkgfL8fJCcLbIP9s+jF+m86FdOctErPC+I/j+yxoSgUQh6B8psH2BN2VJo
bZkBmykgzNZpFAt+anLrk9wXefsyLpGLwJFTRq4Tu8YByCunBnGt1Vgt1MpKkFE4
PgtIx7w03dkAmGq6uCXW+k4jvzWjgYAwfjAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0T
AQH/BAUwAwEB/zBbBgNVHREEVDBSgg9kZWNyZWQtdGVzdG5ldDKCCWxvY2FsaG9z
dIcEfwAAAYcQAAAAAAAAAAAAAAAAAAAAAYcEn0HmoIcECgoABocQ/oAAAAAAAAAs
Kqb//oVPXTAKBggqhkjOPQQDBAOBjAAwgYgCQgEx+zdzr4igUB+puo+E1qslBR2r
6f1X52CWDj2VU4NYMmgqcplv07jyga/T6VgMdMPth6CLL7z6U0d+P+tli6ALwwJC
AXKvfuHJOR7K+A1whpQiXBz+4qaTItEw3qQ3336s3XkCXzAYwkIKOGHKQvqM8jdN
q5DhDo1z1XTUMqfSkTPgZtj+
-----END CERTIFICATE-----
`

var mainnetMatcherRPCCert = `
-----BEGIN CERTIFICATE-----
MIICjzCCAfGgAwIBAgIQBe/9iEwxzSduEurpBJ8WAzAKBggqhkjOPQQDBDBBMR8w
HQYDVQQKExZTcGxpdCBUaWNrZXQgQnV5ZXIgT3JnMR4wHAYDVQQDExVtYWlubmV0
LXNwbGl0LXRpY2tldHMwHhcNMTgwNTA3MjIzOTEyWhcNMjgwNTA1MjIzOTEyWjBB
MR8wHQYDVQQKExZTcGxpdCBUaWNrZXQgQnV5ZXIgT3JnMR4wHAYDVQQDExVtYWlu
bmV0LXNwbGl0LXRpY2tldHMwgZswEAYHKoZIzj0CAQYFK4EEACMDgYYABADbvj73
TTpTWLAqBPxpOjKNy+I73Ctki0LUsyVeh26R5PxL92llMP9LFNGgdIxBwlg7urfE
TWndxbsjOzYq1BY9fgCkNhDdoVBxqe9QY9AJleE9Z15luPBO6G9bIomhBCrUlVSY
rQB4x67bj6CAbKSIf1sErDrlF7VxJdlm6Pp2sGaMoKOBhzCBhDAOBgNVHQ8BAf8E
BAMCAqQwDwYDVR0TAQH/BAUwAwEB/zBhBgNVHREEWjBYghVtYWlubmV0LXNwbGl0
LXRpY2tldHOCCWxvY2FsaG9zdIcEfwAAAYcQAAAAAAAAAAAAAAAAAAAAAYcELTeB
cYcEChEAB4cQ/oAAAAAAAABQCmL//vf+1jAKBggqhkjOPQQDBAOBiwAwgYcCQUTN
/AGB1Fgngmq1xq9fWD8dc1BK3J0X2k4aCoX/Ed8dUm5ct+ftXYLrXtZLaE2Doch7
L/POBWkjj2rcBzdfWs9TAkIB/v6QcOmBvXxZtuOZgGddDRQSBAH3Ue77l45Y9f5z
j/EjxixY5WMXY8F795pOSqHHL6nU8TmNofXAzWQiLbOnFmg=
-----END CERTIFICATE-----
`

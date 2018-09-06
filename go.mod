module github.com/matheusd/dcr-split-ticket-matcher

require (
	github.com/dchest/blake256 v1.0.0
	github.com/decred/dcrd/blockchain v1.0.2
	github.com/decred/dcrd/blockchain/stake v1.0.2
	github.com/decred/dcrd/certgen v1.0.1
	github.com/decred/dcrd/chaincfg v1.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/database v1.0.2 // indirect
	github.com/decred/dcrd/dcrec v0.0.0-20180816212643-20eda7ec9229
	github.com/decred/dcrd/dcrec/edwards v0.0.0-20180816212643-20eda7ec9229 // indirect
	github.com/decred/dcrd/dcrec/secp256k1 v1.0.0
	github.com/decred/dcrd/dcrjson v1.0.0
	github.com/decred/dcrd/dcrutil v1.1.1
	github.com/decred/dcrd/gcs v1.0.2 // indirect
	github.com/decred/dcrd/hdkeychain v1.1.0
	github.com/decred/dcrd/rpcclient v1.0.1
	github.com/decred/dcrd/txscript v1.0.1
	github.com/decred/dcrd/wire v1.1.0
	github.com/decred/dcrwallet/rpc/walletrpc v0.1.0
	github.com/decred/dcrwallet/wallet v1.0.0
	github.com/go-ini/ini v1.30.0
	github.com/golang/protobuf v1.1.0
	github.com/gopherjs/gopherjs v0.0.0-20180825215210-0210a2f0f73c // indirect
	github.com/gorilla/websocket v1.2.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jtolds/gls v4.2.1+incompatible // indirect
	github.com/mattn/go-gtk v0.0.0-20180626024644-7d65db4a7c69
	github.com/mattn/go-pointer v0.0.0-20171114154726-1d30dc4b6f28 // indirect
	github.com/op/go-logging v0.0.0-20160211212156-b2cb9fa56473
	github.com/pkg/errors v0.8.0
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/smartystreets/assertions v0.0.0-20180820201707-7c9eb446e3cf // indirect
	github.com/smartystreets/goconvey v0.0.0-20180222194500-ef6db91d284a // indirect
	github.com/stretchr/testify v1.2.2 // indirect
	golang.org/x/net v0.0.0-20180808004115-f9ce57c11b24
	google.golang.org/grpc v1.14.0
)

replace github.com/jessevdk/go-flags => ../../../testes/vendor/go-flags

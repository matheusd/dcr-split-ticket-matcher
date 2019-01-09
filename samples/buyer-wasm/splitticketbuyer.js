
const getServiceClient = (clientClass, address, cert) =>
    new Promise((resolve, reject) => {
        if (cert == "") {
            return reject(new Error("Unable to load dcrwallet certificate.  dcrwallet not running?"));
        }
        var grpc = require("grpc");
        var creds = grpc.credentials.createSsl(cert);
        var client = new clientClass(address, creds);

        var deadline = new Date();
        var deadlineInSeconds = 5;
        deadline.setSeconds(deadline.getSeconds()+deadlineInSeconds);
        grpc.waitForClientReady(client, deadline, function(err) {
            if (err) {
                reject(err);
            } else {
                resolve(client);
            }
        });
    });

function reverseHash(s) {
    s = s.replace(/^(.(..)*)$/, "0$1"); // add a leading zero if needed
    var a = s.match(/../g);             // split number in groups of two
    a.reverse();                        // reverse the groups
    var s2 = a.join("");
    return s2;
}

class SplitTicketBuyer {

    async setup(config) {
        if (!window["__getSplitTicketBuyerObject"]) {
            const sleep = () => new Promise(resolve => setTimeout(resolve, 1000));
            while (!window["__getSplitTicketBuyerObject"]) {
                await sleep(1000);
            }
        }

        const asyncCall = f => async (resolve, reject, ...args) => {
            try {
                const resp = await f.call(that, ...args);
                resolve(resp);
            } catch (error) {
                reject(error);
            }
        }

        this.buyerIntf = __getSplitTicketBuyerObject();
        this.buyerIntf.config = { ...this.buyerIntf.config, ...config };
        this.buyerIntf.publishedSplitTx = false;
        this.buyerIntf.publishedTicketTx = null;
        this.buyerIntf.clearObj = this.clearObj.bind(this);
        this.buyerIntf.saveSession = asyncCall(this.saveSession).bind(this);

        this.monitorSplitHash = null;
        this.monitorTicketHashes = [];

        // setup of wallet calls

        var pb = require("./walletrpc/api_pb");
        var services = require("./walletrpc/api_grpc_pb.js");
        const wss = await getServiceClient(services.WalletServiceClient,
            config.walletHost + ":" + config.walletPort, config.walletCert);
        this.buyerIntf.walletService = wss;
        let bound = f => f.bind(wss);
        const promisify = f => req => new Promise((resolve, reject) =>
            f(req, (error, resp) => error ? reject(error) : resolve(resp)));

        var that = this;
        const grpcCall = (f, reqClass) => async (encodedReq, resolve, reject) => {
            try {
                const req = encodedReq === ""
                    ? new reqClass()
                    : reqClass.deserializeBinary(Buffer.from(encodedReq, "hex"));
                const resp = await f.call(that, req);
                const hexResp = Buffer.from(resp.serializeBinary()).toString("hex");
                resolve(hexResp);
            } catch (error) {
                reject(error, error.code);
            }
        }

        this.buyerIntf.wallet = {
            ping: grpcCall(promisify(bound(wss.ping)), pb.PingRequest),
            network: grpcCall(promisify(bound(wss.network)), pb.NetworkRequest),
            nextAddress: grpcCall(promisify(bound(wss.nextAddress)), pb.NextAddressRequest),
            constructTransaction: grpcCall(promisify(bound(wss.constructTransaction)), pb.ConstructTransactionRequest),
            signTransactions: grpcCall(promisify(bound(wss.signTransactions)), pb.SignTransactionsRequest),
            validateAddress: grpcCall(promisify(bound(wss.validateAddress)), pb.ValidateAddressRequest),
            signMessage: grpcCall(promisify(bound(wss.signMessage)), pb.SignMessageRequest),
            bestBlock: grpcCall(promisify(bound(wss.bestBlock)), pb.BestBlockRequest),
            ticketPrice: grpcCall(promisify(bound(wss.ticketPrice)), pb.TicketPriceRequest),
        };

        // setup of matcher calls

        let matcherHost = config.matcherHost;
        let matcherCert = config.matcherCert;
        try {
            const srvHost = await this.findMatcherHost(config.matcherHost);
            console.log("Changing to use host " + srvHost);
            matcherHost = srvHost;
            matcherCert = undefined;
        } catch (error) {
            console.warn("Error trying to find SRV record for split ticket service", error);
        }

        var mpb = require("./matcherrpc/api_pb");
        var mservices = require("./matcherrpc/api_grpc_pb.js");
        const ms = await getServiceClient(mservices.SplitTicketMatcherServiceClient,
            matcherHost, matcherCert);
        bound = f => f.bind(ms);

        this.buyerIntf.matcher = {
            findMatches: grpcCall(promisify(bound(ms.findMatches)), mpb.FindMatchesRequest),
            generateTicket: grpcCall(promisify(bound(ms.generateTicket)), mpb.GenerateTicketRequest),
            fundTicket: grpcCall(promisify(bound(ms.fundTicket)), mpb.FundTicketRequest),
            fundSplitTx: grpcCall(promisify(bound(ms.fundSplitTx)), mpb.FundSplitTxRequest),
            status: grpcCall(promisify(bound(ms.status)), mpb.StatusRequest),
            buyerError: grpcCall(promisify(bound(ms.buyerError)), mpb.BuyerErrorRequest),
        }

        this.buyerIntf.network = {
            fetchSpentUtxos: asyncCall(this.fetchSpentUtxos),
            monitorForSessionTransactions: this.monitorForSessionTransactions.bind(this),
        }

        const ntfnsRequest = new pb.TransactionNotificationsRequest();
        this.txNtfns = wss.transactionNotifications(ntfnsRequest);
        this.txNtfns.on("data", this.transactionNotification.bind(this));
        this.txNtfns.on("end", () => {
          console.log("Transaction notifications done");
        });
        this.txNtfns.on("error", error => {
          if (!String(error).includes("Cancelled")) {
              console.error("Transactions ntfns error received:", error);
          }
        });
    }

    run() {
        if (!this.buyerIntf) {
            throw new Error("Buyer interface uninitialized");
        }

        this.buyerIntf.startBuyer();
    }

    clearObj() {
        this.txNtfns.cancel();
    }

    async saveSession(data) {
        console.log("session data to save", data);
    }

    async fetchSpentUtxos(outpoints) {
        // fetches exclusively through dcrdata for the moment.

        var axios = require("axios");

        const dcrdataURL = "https://testnet.dcrdata.org"; // TODO: parametrize
        const fetchOutpURL = outp => dcrdataURL + "/api/tx/" + outp.hash + "/out/" + outp.index;
        const keyOutpoint = outp => outp.hash + ":" + outp.index + ":" + outp.tree;
        const resultToEntry = (outp, res) => ({ [keyOutpoint(outp)]: res.data });
        const fetchOutp = async outp => resultToEntry(outp, await axios.get(fetchOutpURL(outp)));
        const results = await Promise.all(outpoints.map(fetchOutp));
        const utxoMap = results.reduce((m, r) => { return { ...m, ...r }; }, {});
        return utxoMap;
    }

    monitorForSessionTransactions(splitHash, ticketHashes) {
        this.monitorSplitHash = splitHash;
        this.monitorTicketHashes = ticketHashes;
    }

    transactionNotification(resp) {

        const checkTx = (tx => {
            const txHash = reverseHash(Buffer.from(tx.getHash()).toString("hex"))

            if (txHash === this.monitorSplitHash) {
                this.buyerIntf.publishedSplitTx = true;
            }

            if (this.monitorTicketHashes.indexOf(txHash) > -1) {
                // found a published ticket
                this.buyerIntf.publishedTicketTx = txHash;
            }
        });

        resp.getUnminedTransactionsList().forEach(checkTx.bind(this));

        resp.getAttachedBlocksList().forEach(block => {
            block.getTransactionsList().forEach(checkTx.bind(this));
        });
    }

    // findMatcherHost tries to use a dns query to find the actual server for
    // the split ticket service. It assumes this is running on electron
    // with a srv-query ipc call available
    findMatcherHost(host, cert) {
        return new Promise((resolve, reject) => {
            var electron = require("electron");
            var found = false;
            const queryHost = "_split-tickets-grpc._tcp." + host;

            electron.ipcRenderer.once("srv-query-result", ((event, err, addrs) => {
                if (found) return;
                found = true;

                if (err) {
                    reject(err);
                    return;
                }

                if (!addrs || !addrs.length) {
                    reject(new Error("No SRV addresses returned for " + queryHost));
                    return;
                }

                const hostAddr = addrs[0].name + ":" + addrs[0].port;
                resolve(hostAddr);
            }).bind(this));

            electron.ipcRenderer.send("srv-query", queryHost);

            setTimeout(() => {
                if (!found) {
                    found = true;
                    reject(new Error("SRV record for " + queryHost + " not found"));
                }
            }, 10000);
        });
    }
}

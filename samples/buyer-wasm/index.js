const { app, BrowserWindow, ipcMain } = require('electron');
const fs = require("fs");
const dns = require("dns");

function createWindow () {
    win = new BrowserWindow({width: 800, height: 600});
    win.webContents.openDevTools();
    win.loadFile('index.html');
}

console.log(`Chrome v${process.versions.chrome}`);
app.on('ready', createWindow);

ipcMain.on("srv-query", (event, host) => {
    dns.resolveSrv(host, (err, addrs) => {
        event.sender.send("srv-query-result", err, addrs);
    })
});

ipcMain.on("get-config", event => {
    const buyerConfig = require("./config.json");
    buyerConfig.walletCert = fs.readFileSync(buyerConfig.walletCertFile);
    if (fs.existsSync(buyerConfig.matcherCertFile)) {
        buyerConfig.matcherCert = fs.readFileSync(buyerConfig.matcherCertFile);
    }
    event.returnValue = buyerConfig;
});
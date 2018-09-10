const { app, BrowserWindow } = require('electron');

function createWindow () {
    win = new BrowserWindow({width: 800, height: 600});
    win.webContents.openDevTools();
    win.loadFile('index.html');
}

console.log(`Chrome v${process.versions.chrome}`);
app.on('ready', createWindow);

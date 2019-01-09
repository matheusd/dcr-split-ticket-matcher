# Split Ticket Wasm Buyer Sample

This is a demo/example for how to run the split ticket buyer wasm module in an electron app.

It requires a recent version of node installed. It currently (as of 2019-01-09) requires a development version of go installed and on your $PATH.

## Compiling

```
cd samples/buyer-wasm
yarn
yarn rebuild-electron-modules
yarn rebuild-module
cp config.sample.json config.json
```

Edit `config.json` as appropriate, so that you can connect to your running wallet and a matcher service.

## Running

```
yarn start
```

After the wasm module is compiled (it takes about 30 seconds), you can click the "run" button and perform a split ticket session.

If everything goes according to plan, you'll join the given split session.

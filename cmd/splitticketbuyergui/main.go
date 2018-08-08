package main

import (
	"fmt"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

func main() {

	if !buyer.DefaultConfigFileExists() {
		wallets := buyer.ListDecreditonWallets()
		if len(wallets) == 1 {
			walletPools := buyer.ListDecreditonWalletStakepools(wallets[0])
			if len(walletPools) == 1 {
				fmt.Println("Initializing config from existing decrediton wallet")
				err := buyer.InitConfigFromDecrediton(wallets[0], walletPools[0])
				if err != nil {
					fmt.Printf("Error initializing config: %v\n", err)
				}
			}
		}
	}

	gtk.Init(nil)
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetPosition(gtk.WIN_POS_CENTER)
	window.SetTitle(fmt.Sprintf("Split Ticket Buyer (v%s)", pkg.Version))
	window.SetIconName("gtk-dialog-info")
	window.Connect("destroy", func(ctx *glib.CallbackContext) {
		gtk.MainQuit()
	}, "foo")

	ui := buildUI()

	window.Add(ui)
	window.SetSizeRequest(600, 600)
	window.Show()

	gtk.Main()
}

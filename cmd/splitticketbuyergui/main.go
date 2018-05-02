package main

import (
	"fmt"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

func main() {

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
	window.ShowAll()

	gtk.Main()
}

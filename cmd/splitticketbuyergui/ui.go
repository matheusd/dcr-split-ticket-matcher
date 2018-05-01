package main

import (
	"fmt"

	"github.com/mattn/go-gtk/gdk"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

func getDecreditonWalletName() {
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetResizable(false)
	window.SetPosition(gtk.WIN_POS_CENTER)
	window.SetTypeHint(gdk.WINDOW_TYPE_HINT_DIALOG)
	window.SetTitle("Select Decrediton Wallet")
	window.SetIconName("gtk-dialog-info")
	window.Connect("destroy", func(ctx *glib.CallbackContext) {
		fmt.Println("destroying decrediton wallet windows")
	}, "foo")

	vbox := gtk.NewVBox(false, 4)

	combo := gtk.NewComboBoxEntryNewText()
	combo.AppendText("Wallet 1")
	combo.AppendText("Wallet 2")
	combo.AppendText("Wallet 3")
	combo.Connect("changed", func() {
		fmt.Println("value:", combo.GetActiveText())
	})
	vbox.PackStart(combo, false, false, 2)

	button := gtk.NewButtonWithLabel("select")
	button.Clicked(func() {
		window.Destroy()
	})
	vbox.PackStart(button, false, false, 2)

	window.Add(vbox)
	window.SetSizeRequest(400, 200)
	window.ShowAll()
}

func buildMainMenu(menubar *gtk.MenuBar) {

	cascademenu := gtk.NewMenuItemWithMnemonic("_File")
	menubar.Append(cascademenu)
	submenu := gtk.NewMenu()
	cascademenu.SetSubmenu(submenu)

	menuitem := gtk.NewMenuItemWithMnemonic("E_xit")
	menuitem.Connect("activate", func() {
		gtk.MainQuit()
	})
	submenu.Append(menuitem)

	cascademenu = gtk.NewMenuItemWithMnemonic("_Config")
	menubar.Append(cascademenu)
	submenu = gtk.NewMenu()
	cascademenu.SetSubmenu(submenu)

	menuitem = gtk.NewMenuItemWithMnemonic("Reset to Default")
	menuitem.Connect("activate", func() {
		fmt.Println("reset to default menu")
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from dcrwallet")
	menuitem.Connect("activate", func() {
		fmt.Println("load from dcrwallet")
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from decrediton")
	menuitem.Connect("activate", func() {
		fmt.Println("load from decrediton")
		getDecreditonWalletName()
	})
	submenu.Append(menuitem)

}

func buildUI() gtk.IWidget {
	vbox := gtk.NewVBox(false, 4)

	menubar := gtk.NewMenuBar()
	buildMainMenu(menubar)

	vpaned := gtk.NewVPaned()
	vbox.PackStart(vpaned, false, false, 2)

	// title

	label := gtk.NewLabel("Split Ticket Buyer")
	label.ModifyFontEasy("DejaVu Serif 15")
	vpaned.Pack1(label, false, false)

	// password entry

	label = gtk.NewLabel("Wallet Password")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	entry := gtk.NewEntry()
	entry.SetText("")
	entry.SetVisibility(false)
	vbox.PackStart(entry, false, false, 2)

	// participation amount

	label = gtk.NewLabel("Max. Amount")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	scale := gtk.NewHScaleWithRange(1, 100, 1)
	scale.Connect("value-changed", func() {
		fmt.Println("scale:", int(scale.GetValue()))
	})
	vbox.PackStart(scale, false, false, 2)

	// session name
	label = gtk.NewLabel("Session Name")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	entry = gtk.NewEntry()
	entry.SetText("")
	vbox.PackStart(entry, false, false, 2)

	// participate button

	button := gtk.NewButtonWithLabel("Participate")
	button.Clicked(func() {
		fmt.Println("button clicked")
	})
	vbox.PackStart(button, false, false, 2)

	// log area

	swin := gtk.NewScrolledWindow(nil, nil)
	swin.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	swin.SetShadowType(gtk.SHADOW_IN)
	textview := gtk.NewTextView()
	var start, end gtk.TextIter
	buffer := textview.GetBuffer()
	buffer.GetStartIter(&start)
	buffer.Insert(&start, "Waiting to participate in session")
	buffer.GetEndIter(&end)
	buffer.InsertAtCursor("\n")
	tag := buffer.CreateTag("bold", map[string]string{"weight": "1700"})
	buffer.GetStartIter(&start)
	buffer.GetEndIter(&end)
	buffer.ApplyTag(tag, &start, &end)
	swin.Add(textview)
	vbox.Add(swin)

	// frame1 := gtk.NewFrame("Wallet Password")
	// framebox1 := gtk.NewVBox(false, 1)
	// frame1.Add(framebox1)
	// framebox1.Add(entry)

	topbox := gtk.NewVBox(false, 0)
	topbox.PackStart(menubar, false, false, 0)

	align := gtk.NewAlignment(0, 0, 1, 1)
	align.SetPadding(10, 10, 10, 10)
	align.Add(vbox)
	topbox.Add(align)

	return topbox
}

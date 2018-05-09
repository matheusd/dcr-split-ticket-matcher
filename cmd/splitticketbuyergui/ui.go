package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/mattn/go-gtk/gdk"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

var partRunning bool

type logFunc func(string, ...interface{})

func (f logFunc) Write(p []byte) (int, error) {
	f(strings.TrimSpace(string(p)))
	return len(p), nil
}

type logToLogChan chan logMsg

func (l logToLogChan) Write(p []byte) (int, error) {
	l <- logMsg{strings.TrimSpace(string(p)), nil}
	return len(p), nil
}

type logMsg struct {
	format string
	args   []interface{}
}

func getDecreditonWalletName(log logFunc) {
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

	wallets := buyer.ListDecreditonWallets()

	combo := gtk.NewComboBoxEntryNewText()
	for _, w := range wallets {
		combo.AppendText(w)
	}
	//combo.Connect("changed", func() {})
	vbox.PackStart(combo, false, false, 2)

	button := gtk.NewButtonWithLabel("select")
	button.Clicked(func() {
		w := combo.GetActiveText()
		log("Resetting config to decrediton wallet \"%s\"", w)
		err := buyer.InitConfigFromDecrediton(w)
		if err != nil {
			log(err.Error())
		} else {
			log("Successfully reset config to decrediton values")
			reportConfig(log)
		}

		window.Destroy()
	})
	vbox.PackStart(button, false, false, 2)

	window.Add(vbox)
	window.SetSizeRequest(400, 200)
	window.ShowAll()
}

func reportConfig(log logFunc) {
	cfg, err := buyer.LoadConfig()
	if err != nil {
		log("Error reading config: %v", err)
		return
	}

	networks := map[bool]string{false: "** MainNet **", true: "TestNet"}

	log("")
	log("Current Config")
	log("Vote Address: %s", cfg.VoteAddress)
	log("Pool Subsidy Address: %s", cfg.PoolAddress)
	log("Network: %s", networks[cfg.TestNet])
	log("Matcher Host: %s", cfg.MatcherHost)

	err = cfg.Validate()
	if err != nil {
		log("")
		log("** INVALID CONFIG **")
		log(err.Error())
		log("Please edit the config file at %s", cfg.ConfigFile)
	}
}

func buildMainMenu(menubar *gtk.MenuBar, log logFunc) {

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

	menuitem = gtk.NewMenuItemWithMnemonic("Show Config")
	menuitem.Connect("activate", func() {
		reportConfig(log)
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Reset to Default")
	menuitem.Connect("activate", func() {
		err := buyer.InitDefaultConfig()
		if err != nil {
			log(err.Error())
		} else {
			log("Successfully reset config to factory defaults")
			reportConfig(log)
		}
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from dcrwallet")
	menuitem.Connect("activate", func() {
		err := buyer.InitConfigFromDcrwallet()
		if err != nil {
			log(err.Error())
		} else {
			log("Successfully reset config to dcrwallet values")
			reportConfig(log)
		}
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from decrediton")
	menuitem.Connect("activate", func() {
		fmt.Println("load from decrediton")
		getDecreditonWalletName(log)
	})
	submenu.Append(menuitem)

}

func participate(log logFunc, passphrase, sessionName string,
	maxAmount float64) {
	cfg, err := buyer.LoadConfig()
	if err != nil {
		log(fmt.Sprintf("Error reading config: %v", err))
		return
	}

	if passphrase == "" {
		log("Empty Passphrase")
		return
	}

	cfg.SessionName = sessionName
	cfg.Passphrase = []byte(passphrase)
	cfg.MaxAmount = maxAmount

	err = cfg.Validate()
	if err != nil {
		reportConfig(log)
		return
	}

	//logger := logToLogChan(logChan)
	logChan := make(logToLogChan)
	splitResultChan := make(chan error)

	go func() {
		reporter := buyer.NewWriterReporter(logChan)
		ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)
		ctx, cancel := context.WithCancel(ctx)
		go buyer.WatchMatcherWaitingList(ctx, cfg.MatcherHost,
			cfg.MatcherCertFile, reporter)
		splitResultChan <- buyer.BuySplitTicket(ctx, cfg)
		cancel()
	}()

	gotResult := false
	timer := time.NewTicker(100 * time.Millisecond)
	for !gotResult {
		select {
		case err = <-splitResultChan:
			if err != nil {
				log("Error trying to purchase split ticket: %v", err)
			}
			timer.Stop()
			gotResult = true
		case logMsg := <-logChan:
			log(logMsg.format, logMsg.args...)
		case <-timer.C:
			for gtk.EventsPending() {
				if gtk.MainIterationDo(false) {
					// early exit requested
					gotResult = true
				}
			}
		}
	}
}

func buildUI() gtk.IWidget {
	vbox := gtk.NewVBox(false, 4)

	menubar := gtk.NewMenuBar()

	vpaned := gtk.NewVPaned()
	vbox.PackStart(vpaned, false, false, 2)

	// title

	label := gtk.NewLabel("Split Ticket Buyer")
	label.ModifyFontEasy("DejaVu Serif 15")
	vpaned.Pack1(label, false, false)

	// participation amount

	label = gtk.NewLabel("Max. Amount (in DCR)")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	amountScale := gtk.NewHScaleWithRange(1, 50, 1)
	vbox.PackStart(amountScale, false, false, 2)

	// session name
	label = gtk.NewLabel("Session Name")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	sessEntry := gtk.NewEntry()
	sessEntry.SetText("")
	vbox.PackStart(sessEntry, false, false, 2)

	// password entry

	label = gtk.NewLabel("Wallet Passphrase")
	label.SetAlignment(0, 0)
	vbox.PackStart(label, false, false, 2)

	pwdEntry := gtk.NewEntry()
	pwdEntry.SetText("")
	pwdEntry.SetVisibility(false)
	vbox.PackStart(pwdEntry, false, false, 2)

	// participate button

	button := gtk.NewButtonWithLabel("Participate")
	vbox.PackStart(button, false, false, 2)

	// log area

	swin := gtk.NewScrolledWindow(nil, nil)
	swin.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	swin.SetShadowType(gtk.SHADOW_IN)
	textview := gtk.NewTextView()
	textview.SetEditable(false)
	textview.SetWrapMode(gtk.WRAP_WORD)
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
	buffer.CreateMark("end", &end, false)
	swin.Add(textview)
	vbox.Add(swin)

	log := logFunc(func(format string, args ...interface{}) {
		var end gtk.TextIter
		msg := fmt.Sprintf(format, args...)
		buffer := textview.GetBuffer()
		endMark := buffer.GetMark("end")
		buffer.GetEndIter(&end)
		buffer.Insert(&end, msg+"\n")
		textview.ScrollToMark(endMark, 0, true, 0, 1)
		for gtk.EventsPending() {
			gtk.MainIterationDo(false)
		}
	})

	// this doesn't need a mutex because button.clicked() only gets called on
	// the main (gtk) thread (albeit with a longer stack frame if the button
	// is clicked while a session is already running).
	partRunning = false

	button.Clicked(func() {
		if partRunning {
			return
		}
		partRunning = true

		sessionName := sessEntry.GetText()
		pass := pwdEntry.GetText()
		maxAmount := amountScale.GetValue()
		participate(log, pass, sessionName, maxAmount)

		partRunning = false
	})

	buildMainMenu(menubar, log)

	topbox := gtk.NewVBox(false, 0)
	topbox.PackStart(menubar, false, false, 0)

	align := gtk.NewAlignment(0, 0, 1, 1)
	align.SetPadding(10, 10, 10, 10)
	align.Add(vbox)
	topbox.Add(align)

	reportConfig(log)

	return topbox
}

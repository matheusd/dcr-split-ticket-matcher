package main

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/mattn/go-gtk/gdk"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

var partRunning bool
var cancelParticipateChan = make(chan struct{})

const disclaimerTxt = `The split ticket buyer is considered BETA software and is subject to several risks which might cause you to LOSE YOUR FUNDS.

By continuing past this point you agree that you are aware of the risks and and is running the sofware AT YOUR OWN RISK.

For more information, read:

https://github.com/matheusd/dcr-split-ticket-matcher/blob/master/docs/beta.md
`

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

func getDecreditonWalletName(logf logFunc) {
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetResizable(false)
	window.SetPosition(gtk.WIN_POS_CENTER)
	window.SetTypeHint(gdk.WINDOW_TYPE_HINT_DIALOG)
	window.SetTitle("Select Decrediton Wallet")
	window.SetIconName("gtk-dialog-info")
	window.Connect("destroy", func(ctx *glib.CallbackContext) {
	}, "foo")

	vbox := gtk.NewVBox(false, 4)

	wallets := buyer.ListDecreditonWallets()

	label := gtk.NewLabel("Wallet")
	label.ModifyFontEasy("DejaVu Serif 15")
	vbox.PackStart(label, false, false, 2)

	combo := gtk.NewComboBoxText()
	for _, w := range wallets {
		combo.AppendText(w)
	}
	vbox.PackStart(combo, false, false, 2)

	label = gtk.NewLabel("Voting Pool")
	label.ModifyFontEasy("DejaVu Serif 15")
	vbox.PackStart(label, false, false, 2)

	comboPool := gtk.NewComboBoxText()
	vbox.PackStart(comboPool, false, false, 2)

	combo.Connect("changed", func() {
		w := combo.GetActiveText()
		walletPools := buyer.ListDecreditonWalletStakepools(w)
		for i := 0; i < 100; i++ {
			comboPool.Remove(0)
		}
		for _, p := range walletPools {
			comboPool.AppendText(p)
		}
	})

	button := gtk.NewButtonWithLabel("select")
	button.Clicked(func() {
		w := combo.GetActiveText()
		p := comboPool.GetActiveText()
		logf("Resetting config to decrediton wallet '%s' pool '%s'", w, p)
		err := buyer.InitConfigFromDecrediton(w, p)
		if err != nil {
			logf(err.Error())
		} else {
			logf("Successfully reset config to decrediton values")
			reportConfig(logf)
		}

		window.Destroy()
	})
	vbox.PackStart(button, false, false, 2)

	window.Add(vbox)
	window.SetSizeRequest(400, 200)
	window.ShowAll()
}

func showParticipationDisclaimer(logf logFunc, resChannel chan bool) {
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetResizable(false)
	window.SetPosition(gtk.WIN_POS_CENTER)
	window.SetTypeHint(gdk.WINDOW_TYPE_HINT_DIALOG)
	window.SetTitle("Participation Disclaimer")
	window.SetIconName("gtk-dialog-info")
	window.SetDeletable(false)

	vbox := gtk.NewVBox(false, 4)

	label := gtk.NewLabel("Participation Disclaimer")
	label.ModifyFontEasy("DejaVu Serif 15")
	vbox.PackStart(label, false, false, 2)

	swin := gtk.NewScrolledWindow(nil, nil)
	swin.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	swin.SetShadowType(gtk.SHADOW_IN)
	textview := gtk.NewTextView()
	textview.SetEditable(false)
	textview.SetWrapMode(gtk.WRAP_WORD)
	var start, end gtk.TextIter
	buffer := textview.GetBuffer()
	buffer.GetStartIter(&start)
	buffer.Insert(&start, disclaimerTxt)
	buffer.GetEndIter(&end)
	buffer.InsertAtCursor("\n")
	swin.Add(textview)
	vbox.Add(swin)

	chk := gtk.NewCheckButtonWithLabel("I Accept the Risks")
	vbox.PackStart(chk, false, false, 2)

	button := gtk.NewButtonWithLabel("Continue")
	button.Clicked(func() {
		window.Destroy()
	})
	vbox.PackStart(button, false, false, 2)

	window.Connect("destroy", func(ctx *glib.CallbackContext) {
		isActive := chk.GetActive()
		resChannel <- isActive
	}, "foo")

	window.Add(vbox)
	window.SetSizeRequest(400, 300)
	window.ShowAll()
}

func reportConfig(logf logFunc) {
	cfg, err := buyer.LoadConfig()
	if err != nil {
		logf("Error reading config: %v", err)
		return
	}

	networks := map[bool]string{false: "** MainNet **", true: "TestNet"}

	logf("")
	logf("Current Config")
	logf("Vote Address: %s", cfg.VoteAddress)
	logf("Pool Subsidy Address: %s", cfg.PoolAddress)
	logf("Network: %s", networks[cfg.TestNet])
	logf("Source Account: %d", cfg.SourceAccount)
	logf("Matcher Host: %s", cfg.MatcherHost)

	err = cfg.Validate()
	if err != nil {
		logf("")
		logf("** INVALID CONFIG **")
		logf(err.Error())
		logf("Please edit the config file at %s", cfg.ConfigFile)
	}
}

func buildMainMenu(menubar *gtk.MenuBar, logf logFunc) {

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
		reportConfig(logf)
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Reset to Default")
	menuitem.Connect("activate", func() {
		err := buyer.InitDefaultConfig()
		if err != nil {
			logf(err.Error())
		} else {
			logf("Successfully reset config to factory defaults")
			reportConfig(logf)
		}
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from dcrwallet")
	menuitem.Connect("activate", func() {
		err := buyer.InitConfigFromDcrwallet()
		if err != nil {
			logf(err.Error())
		} else {
			logf("Successfully reset config to dcrwallet values")
			reportConfig(logf)
		}
	})
	submenu.Append(menuitem)

	menuitem = gtk.NewMenuItemWithMnemonic("Load from decrediton")
	menuitem.Connect("activate", func() {
		getDecreditonWalletName(logf)
	})
	submenu.Append(menuitem)

}

func participate(logf logFunc, passphrase, sessionName string,
	maxAmount float64) {
	cfg, err := buyer.LoadConfig()
	if err != nil {
		logf(fmt.Sprintf("Error reading config: %v", err))
		return
	}

	if passphrase == "" {
		logf("Empty Passphrase")
		return
	}

	cfg.SessionName = sessionName
	cfg.Passphrase = []byte(passphrase)
	cfg.MaxAmount = maxAmount

	err = cfg.Validate()
	if err != nil {
		reportConfig(logf)
		return
	}

	//logger := logToLogChan(logChan)
	logChan := make(logToLogChan)
	splitResultChan := make(chan error)
	logDir := path.Join(cfg.DataDir, "logs")
	reporter := buyer.NewWriterReporter(buyer.NewLoggerMiddleware(logChan, logDir),
		cfg.SessionName)
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		go buyer.WatchMatcherWaitingList(ctx, cfg.MatcherHost,
			cfg.MatcherCertFile, reporter)
		splitResultChan <- buyer.BuySplitTicket(ctx, cfg)
	}()

	gotResult := false
	timer := time.NewTicker(100 * time.Millisecond)
	for !gotResult {
		select {
		case err = <-splitResultChan:
			if err != nil {
				logf("Error trying to purchase split ticket: %v", err)
			}
			timer.Stop()
			gotResult = true
		case logMsg := <-logChan:
			logf(logMsg.format, logMsg.args...)
		case <-cancelParticipateChan:
			logf("User requested to cancel participation")
			gotResult = true
		case <-timer.C:
			for gtk.EventsPending() {
				if gtk.MainIterationDo(false) {
					// early exit requested
					gotResult = true
				}
			}
		}
	}

	cancel()
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

	amountScale := gtk.NewHScaleWithRange(1, 150, 1)
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

	buttonStop := gtk.NewButtonWithLabel("Stop Participation")
	vbox.PackStart(buttonStop, false, false, 2)

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
		buffer.GetEndIter(&end)
		buffer.Insert(&end, msg+"\n")
		endMark := buffer.GetMark("end")
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

		participateChan := make(chan bool, 1)
		showParticipationDisclaimer(log, participateChan)

		ticker := time.NewTicker(20 * time.Millisecond)
		doParticipate := false
		keepWaiting := true
		for keepWaiting {
			select {
			case doParticipate = <-participateChan:
				keepWaiting = false
			case <-ticker.C:
				for gtk.EventsPending() {
					gtk.MainIterationDo(false)
				}
			}
		}
		ticker.Stop()

		close(participateChan)
		if !doParticipate {
			partRunning = false
			log("Cancelling due to not agreeing to beta rules")
			return
		}

		button.SetVisible(false)
		buttonStop.SetVisible(true)

		sessionName := sessEntry.GetText()
		pass := pwdEntry.GetText()
		maxAmount := amountScale.GetValue()
		participate(log, pass, sessionName, maxAmount)
		partRunning = false

		button.SetVisible(true)
		buttonStop.SetVisible(false)
	})

	buttonStop.Clicked(func() {
		go func() {
			cancelParticipateChan <- struct{}{}
		}()
	})

	buildMainMenu(menubar, log)

	topbox := gtk.NewVBox(false, 0)
	topbox.PackStart(menubar, false, false, 0)

	align := gtk.NewAlignment(0, 0, 1, 1)
	align.SetPadding(10, 10, 10, 10)
	align.Add(vbox)
	topbox.Add(align)

	topbox.ShowAll()
	buttonStop.Hide()

	reportConfig(log)

	return topbox
}

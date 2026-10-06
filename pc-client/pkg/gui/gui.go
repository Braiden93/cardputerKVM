package gui

import (
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/evdev"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
)

var (
	ColorCyan   = color.NRGBA{R: 0x00, G: 0xF5, B: 0xD4, A: 0xFF} // Neon Cyan (Connected)
	ColorPurple = color.NRGBA{R: 0x9D, G: 0x4E, B: 0xDD, A: 0xFF} // Purple (Disconnected)
	ColorWhite  = color.NRGBA{R: 0xF0, G: 0xF6, B: 0xFC, A: 0xFF}
)

type GUIApp struct {
	fyneApp fyne.App
	win     fyne.Window

	trans   transport.Transport
	grabber *evdev.Grabber

	// Controls
	connCard        *widget.Card
	titleDotText    *canvas.Text
	portEntry       *widget.Entry
	devSummaryLabel *widget.Label

	isUpdatingUI bool
	mu           sync.Mutex
}

func NewGUIApp(trans transport.Transport, grabber *evdev.Grabber) *GUIApp {
	a := app.NewWithID("com.cardputer.kvm")
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("Cardputer-KVM ○ PASS-THROUGH")
	w.Resize(fyne.NewSize(480, 160))

	g := &GUIApp{
		fyneApp: a,
		win:     w,
		trans:   trans,
		grabber: grabber,
	}

	g.buildUI()
	g.startConnectionMonitor()
	return g
}

func (g *GUIApp) buildUI() {
	// --- Section 1: Transmitter Dongle Connection ---
	g.portEntry = widget.NewEntry()
	ep := ""
	if g.trans != nil {
		ep = g.trans.Endpoint()
		parts := strings.Split(ep, " ")
		if len(parts) > 0 {
			ep = parts[0]
		}
	}
	g.portEntry.SetText(ep)

	btnAutoDetect := widget.NewButtonWithIcon("Detect", theme.SearchIcon(), func() {
		if detected, err := transport.AutoDetectSerialPort(); err == nil {
			g.portEntry.SetText(detected)
			g.reconnectSerial()
		}
	})

	connForm := widget.NewForm(
		widget.NewFormItem("Port:", container.NewBorder(nil, nil, nil, btnAutoDetect, g.portEntry)),
	)

	// Card Header with Cyan (Connected & Verified) vs Purple (Disconnected / Unverified) Dot
	dotColor := ColorPurple
	if g.trans != nil && g.trans.IsConnected() {
		if verified, _ := g.trans.VerifyHandshake(); verified {
			dotColor = ColorCyan
		}
	}

	g.titleDotText = canvas.NewText("● ", dotColor)
	g.titleDotText.TextSize = 14
	g.titleDotText.TextStyle = fyne.TextStyle{Bold: true}

	titleLabel := canvas.NewText("Transmitter Dongle", ColorWhite)
	titleLabel.TextSize = 14
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	titleHeader := container.NewHBox(g.titleDotText, titleLabel)

	cardContent := container.NewVBox(
		titleHeader,
		widget.NewSeparator(),
		connForm,
	)

	g.connCard = widget.NewCard("", "", cardContent)

	// --- Section 2: Bottom Summary & Action Bar ---
	devCountStr := "[Scroll Lock] Toggle"
	if g.grabber != nil {
		devs := g.grabber.Devices()
		devCountStr = fmt.Sprintf("%d devs │ [Scroll Lock]", len(devs))
	}
	g.devSummaryLabel = widget.NewLabel(devCountStr)
	g.devSummaryLabel.Importance = widget.LowImportance

	btnReconnect := widget.NewButtonWithIcon("Reconnect", theme.ViewRefreshIcon(), func() {
		g.reconnectSerial()
	})

	bottomBar := container.NewHBox(
		g.devSummaryLabel,
		layout.NewSpacer(),
		btnReconnect,
	)

	// Main Layout
	mainLayout := container.NewBorder(
		nil,
		bottomBar,
		nil,
		nil,
		g.connCard,
	)

	g.win.SetContent(mainLayout)

	if g.grabber != nil {
		g.updateStateUI(g.grabber.IsGrabbed())
	}
}

func (g *GUIApp) updateStateUI(isGrabbed bool) {
	g.mu.Lock()
	if g.isUpdatingUI {
		g.mu.Unlock()
		return
	}
	g.isUpdatingUI = true
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.isUpdatingUI = false
		g.mu.Unlock()
	}()

	if isGrabbed {
		g.win.SetTitle("Cardputer-KVM ● TARGET ENGAGED")
	} else {
		g.win.SetTitle("Cardputer-KVM ○ PASS-THROUGH")
	}
}

func (g *GUIApp) HandleGrabEvent(evt evdev.GrabEvent) {
	if evt.Type == evdev.EvtGrabToggled {
		fyne.Do(func() {
			g.updateStateUI(evt.Grabbed)
		})
	}
}

func (g *GUIApp) reconnectSerial() {
	g.mu.Lock()
	defer g.mu.Unlock()

	port := strings.TrimSpace(g.portEntry.Text)
	if port == "" {
		return
	}

	baudRate := 921600

	if g.trans != nil {
		_ = g.trans.Close()
		g.trans = nil
	}

	t, err := transport.NewSerialTransport(port, baudRate)
	if err != nil {
		g.titleDotText.Color = ColorPurple
		g.titleDotText.Refresh()
		if g.connCard != nil {
			g.connCard.Refresh()
		}
		return
	}

	g.trans = t
	if g.grabber != nil {
		g.grabber.SetTransport(t)
	}

	if verified, _ := t.VerifyHandshake(); verified {
		g.titleDotText.Color = ColorCyan
	} else {
		g.titleDotText.Color = ColorPurple
	}
	g.titleDotText.Refresh()
	if g.connCard != nil {
		g.connCard.Refresh()
	}
}

func (g *GUIApp) startConnectionMonitor() {
	go func() {
		ticker := time.NewTicker(1000 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			g.checkConnectionStatus()
		}
	}()
}

func (g *GUIApp) checkConnectionStatus() {
	g.mu.Lock()
	trans := g.trans
	g.mu.Unlock()

	isOK := false
	if trans != nil && trans.IsConnected() {
		verified, _ := trans.VerifyHandshake()
		isOK = verified
		if !verified {
			_ = trans.Close()
			g.mu.Lock()
			g.trans = nil
			if g.grabber != nil {
				g.grabber.SetTransport(nil)
			}
			g.mu.Unlock()
		}
	}

	// If currently disconnected, try auto-detecting a re-plugged dongle
	if !isOK {
		if detectedPort, err := transport.AutoDetectSerialPort(); err == nil && detectedPort != "" {
			if verified, _, _ := transport.VerifyTransmitterHandshake(detectedPort, 921600); verified {
				if t, err := transport.NewSerialTransport(detectedPort, 921600); err == nil {
					g.mu.Lock()
					g.trans = t
					if g.grabber != nil {
						g.grabber.SetTransport(t)
					}
					g.mu.Unlock()
					isOK = true

					fyne.Do(func() {
						g.portEntry.SetText(detectedPort)
					})
				}
			}
		}
	}

	targetColor := ColorPurple
	if isOK {
		targetColor = ColorCyan
	}

	fyne.Do(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		if g.titleDotText != nil {
			g.titleDotText.Color = targetColor
			g.titleDotText.Refresh()
			if g.connCard != nil {
				g.connCard.Refresh()
			}
		}
	})
}

func (g *GUIApp) Run() {
	g.win.ShowAndRun()
}

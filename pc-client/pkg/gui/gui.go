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
	btnToggleGrab   *widget.Button

	missedHandshakes int
	isUpdatingUI     bool
	mu               sync.Mutex
}

func NewGUIApp(trans transport.Transport, grabber *evdev.Grabber) *GUIApp {
	a := app.NewWithID("com.cardputer.kvm")
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("Cardputer-KVM ○ PASS-THROUGH")
	w.Resize(fyne.NewSize(520, 160))

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
	isInitOK := false
	if g.trans != nil && g.trans.IsConnected() {
		if verified, _ := g.trans.VerifyHandshake(); verified {
			dotColor = ColorCyan
			isInitOK = true
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

	g.btnToggleGrab = widget.NewButtonWithIcon("Engage KVM", theme.MediaPlayIcon(), func() {
		if g.grabber != nil {
			g.grabber.ToggleGrab()
		}
	})
	if !isInitOK {
		g.btnToggleGrab.Disable()
	}

	btnReconnect := widget.NewButtonWithIcon("Reconnect", theme.ViewRefreshIcon(), func() {
		g.reconnectSerial()
	})

	bottomBar := container.NewHBox(
		g.devSummaryLabel,
		layout.NewSpacer(),
		g.btnToggleGrab,
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
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.SetText("Release KVM")
			g.btnToggleGrab.SetIcon(theme.MediaStopIcon())
			g.btnToggleGrab.Importance = widget.HighImportance
			g.btnToggleGrab.Refresh()
		}
	} else {
		g.win.SetTitle("Cardputer-KVM ○ PASS-THROUGH")
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.SetText("Engage KVM")
			g.btnToggleGrab.SetIcon(theme.MediaPlayIcon())
			g.btnToggleGrab.Importance = widget.MediumImportance
			g.btnToggleGrab.Refresh()
		}
	}
}

func (g *GUIApp) HandleGrabEvent(evt evdev.GrabEvent) {
	switch evt.Type {
	case evdev.EvtGrabToggled:
		fyne.Do(func() {
			g.updateStateUI(evt.Grabbed)
		})
	case evdev.EvtError:
		fyne.Do(func() {
			if g.devSummaryLabel != nil {
				g.devSummaryLabel.SetText(evt.Message)
			}
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
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.Disable()
		}
		return
	}

	g.trans = t
	g.missedHandshakes = 0
	if g.grabber != nil {
		g.grabber.SetTransport(t)
	}

	if verified, _ := t.VerifyHandshake(); verified {
		g.titleDotText.Color = ColorCyan
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.Enable()
		}
	} else {
		g.titleDotText.Color = ColorPurple
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.Disable()
		}
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
		if verified {
			g.mu.Lock()
			g.missedHandshakes = 0
			g.mu.Unlock()
			isOK = true
		} else {
			g.mu.Lock()
			g.missedHandshakes++
			if g.missedHandshakes < 3 {
				// Retain connection state during momentary latency spikes
				isOK = true
			} else {
				// Device truly disconnected or unplugged
				_ = trans.Close()
				g.trans = nil
				if g.grabber != nil {
					g.grabber.SetTransport(nil)
				}
				isOK = false
			}
			g.mu.Unlock()
		}
	}

	// If currently disconnected, auto-detect a connected or re-plugged dongle
	if !isOK {
		if detectedPort, err := transport.AutoDetectSerialPort(); err == nil && detectedPort != "" {
			if t, err := transport.NewSerialTransport(detectedPort, 921600); err == nil {
				if verified, _ := t.VerifyHandshake(); verified {
					g.mu.Lock()
					g.trans = t
					g.missedHandshakes = 0
					if g.grabber != nil {
						g.grabber.SetTransport(t)
					}
					g.mu.Unlock()
					isOK = true

					fyne.Do(func() {
						g.portEntry.SetText(detectedPort)
					})
				} else {
					_ = t.Close()
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

		if g.btnToggleGrab != nil {
			if isOK {
				g.btnToggleGrab.Enable()
			} else {
				g.btnToggleGrab.Disable()
			}
		}

		if g.devSummaryLabel != nil && g.grabber != nil {
			if !isOK {
				g.devSummaryLabel.SetText("Transmitter offline │ Grab safe")
			} else {
				devs := g.grabber.Devices()
				g.devSummaryLabel.SetText(fmt.Sprintf("%d devs │ [Scroll Lock]", len(devs)))
			}
		}
	})
}

func (g *GUIApp) Run() {
	g.win.ShowAndRun()
}

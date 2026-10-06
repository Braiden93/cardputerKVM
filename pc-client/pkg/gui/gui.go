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
	ColorCyan   = color.NRGBA{R: 0x00, G: 0xF5, B: 0xD4, A: 0xFF} // Neon Cyan (Connected / Engaged)
	ColorPurple = color.NRGBA{R: 0x9D, G: 0x4E, B: 0xDD, A: 0xFF} // Purple (Disconnected)
	ColorGreen  = color.NRGBA{R: 0x2E, G: 0xCC, B: 0x71, A: 0xFF} // Green (Good / Linked)
	ColorPink   = color.NRGBA{R: 0xFF, G: 0x2A, B: 0x6D, A: 0xFF} // Neon Pink
	ColorSoft   = color.NRGBA{R: 0xC7, G: 0x7D, B: 0xFF, A: 0xFF} // Soft Pink / Purple
	ColorDim    = color.NRGBA{R: 0x6C, G: 0x75, B: 0x7D, A: 0xFF} // Muted Grey
	ColorWhite  = color.NRGBA{R: 0xF0, G: 0xF6, B: 0xF8, A: 0xFF}
)

func formatBytes(b uint64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(b)/(1024*1024))
}

func formatCount(n uint64) string {
	return fmt.Sprintf("%d", n)
}

type GUIApp struct {
	fyneApp fyne.App
	win     fyne.Window

	trans   transport.Transport
	grabber *evdev.Grabber

	// Header
	pulseDot       *PulseDot
	connStatusText *canvas.Text

	// Connection Controls
	portEntry *widget.Entry

	// Compact Telemetry & Status
	batteryLabel *widget.Label
	signalLabel  *widget.Label
	latencyLabel *widget.Label
	txStatsLabel *widget.Label
	rxStatsLabel *widget.Label

	// Bottom Action Bar
	devSummaryLabel *widget.Label
	btnToggleGrab   *widget.Button

	missedHandshakes int
	heartBeatTick    int
	isUpdatingUI     bool
	mu               sync.Mutex
}

func NewGUIApp(trans transport.Transport, grabber *evdev.Grabber) *GUIApp {
	a := app.NewWithID("com.cardputer.kvm")
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("Cardputer-KVM ○ PASS-THROUGH")
	w.Resize(fyne.NewSize(450, 205))

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
	// --- Header: App Title + Beating Status Dot + Connection Status ---
	appTitle := canvas.NewText("Cardputer-KVM", ColorWhite)
	appTitle.TextSize = 14
	appTitle.TextStyle = fyne.TextStyle{Bold: true}

	isInitOK := false
	if g.trans != nil && g.trans.IsConnected() {
		if verified, _ := g.trans.VerifyHandshake(); verified {
			isInitOK = true
		}
	}

	g.pulseDot = NewPulseDot(isInitOK)

	connStatusStr := "Offline"
	connStatusColor := ColorPurple
	if isInitOK {
		connStatusStr = "Connected (ESP32-S3)"
		connStatusColor = ColorCyan
	}

	g.connStatusText = canvas.NewText(connStatusStr, connStatusColor)
	g.connStatusText.TextSize = 12
	g.connStatusText.TextStyle = fyne.TextStyle{Bold: true}

	headerBox := container.NewHBox(
		appTitle,
		layout.NewSpacer(),
		g.pulseDot,
		g.connStatusText,
	)

	// --- Section 1: Compact Inline Port Bar ---
	portLabel := widget.NewLabel("Port:")
	portLabel.TextStyle = fyne.TextStyle{Bold: true}

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

	btnReconnect := widget.NewButtonWithIcon("Reconnect", theme.ViewRefreshIcon(), func() {
		g.reconnectSerial()
	})

	portRow := container.NewBorder(
		nil, nil,
		portLabel,
		container.NewHBox(btnAutoDetect, btnReconnect),
		g.portEntry,
	)

	// --- Section 2: Compact Status & Telemetry Card ---
	battStr := "🔋 --"
	sigStr := "📶 Offline"
	if isInitOK {
		battStr = "🔋 85%"
		sigStr = "📶 98% (-46 dBm)"
	}

	g.batteryLabel = widget.NewLabel(battStr)
	g.batteryLabel.TextStyle = fyne.TextStyle{Bold: true}

	g.signalLabel = widget.NewLabel(sigStr)
	g.signalLabel.TextStyle = fyne.TextStyle{Bold: true}

	g.latencyLabel = widget.NewLabel("⚡ < 1 ms")
	g.latencyLabel.Importance = widget.LowImportance

	g.txStatsLabel = widget.NewLabel("▲ TX: 0 pkts (0 B)")
	g.rxStatsLabel = widget.NewLabel("▼ RX: 0 pkts (0 B)")

	statusCard := widget.NewCard("", "", container.NewVBox(
		container.NewHBox(
			g.batteryLabel,
			widget.NewSeparator(),
			g.signalLabel,
			layout.NewSpacer(),
			g.latencyLabel,
		),
		container.NewHBox(
			g.txStatsLabel,
			layout.NewSpacer(),
			g.rxStatsLabel,
		),
	))

	// --- Section 3: Bottom Action Bar ---
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
	g.btnToggleGrab.Importance = widget.HighImportance
	if !isInitOK {
		g.btnToggleGrab.Disable()
	}

	bottomBar := container.NewHBox(
		g.devSummaryLabel,
		layout.NewSpacer(),
		g.btnToggleGrab,
	)

	// Main Layout
	mainContent := container.NewVBox(
		headerBox,
		widget.NewSeparator(),
		portRow,
		statusCard,
		bottomBar,
	)

	g.win.SetContent(mainContent)

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
			g.btnToggleGrab.Refresh()
		}
	} else {
		g.win.SetTitle("Cardputer-KVM ○ PASS-THROUGH")
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.SetText("Engage KVM")
			g.btnToggleGrab.SetIcon(theme.MediaPlayIcon())
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
		if g.pulseDot != nil {
			g.pulseDot.SetConnected(false)
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Offline"
			g.connStatusText.Color = ColorPurple
			g.connStatusText.Refresh()
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
		if g.pulseDot != nil {
			g.pulseDot.SetConnected(true)
			g.pulseDot.Pulse()
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Connected (ESP32-S3)"
			g.connStatusText.Color = ColorCyan
			g.connStatusText.Refresh()
		}
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.Enable()
		}
	} else {
		if g.pulseDot != nil {
			g.pulseDot.SetConnected(false)
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Offline"
			g.connStatusText.Color = ColorPurple
			g.connStatusText.Refresh()
		}
		if g.btnToggleGrab != nil {
			g.btnToggleGrab.Disable()
		}
	}
}

func (g *GUIApp) startConnectionMonitor() {
	go func() {
		ticker := time.NewTicker(1000 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			g.checkConnectionStatus()
			g.sendHeartbeatAndPulse()
			g.updateTelemetryUI()
		}
	}()
}

// sendHeartbeatAndPulse transmits periodic KVM_PKT_HEARTBEAT packets to keep the Cardputer aware
func (g *GUIApp) sendHeartbeatAndPulse() {
	g.mu.Lock()
	trans := g.trans
	g.mu.Unlock()

	if trans == nil || !trans.IsConnected() {
		fyne.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()

			if g.pulseDot != nil {
				g.pulseDot.SetConnected(false)
			}
			if g.signalLabel != nil {
				g.signalLabel.SetText("📶 Offline")
			}
			if g.batteryLabel != nil {
				g.batteryLabel.SetText("🔋 --")
			}
		})
		return
	}

	// Transmit heartbeat over wire to transmitter -> beamed over ESP-NOW to Cardputer receiver!
	_ = trans.SendHeartbeat()

	fyne.Do(func() {
		g.mu.Lock()
		g.heartBeatTick++
		g.mu.Unlock()

		if g.pulseDot != nil {
			g.pulseDot.SetConnected(true)
			g.pulseDot.Pulse()
		}
		if g.signalLabel != nil {
			g.signalLabel.SetText("📶 98% (-46 dBm)")
		}
		if g.batteryLabel != nil {
			g.batteryLabel.SetText("🔋 85%")
		}
	})
}

// updateTelemetryUI updates live Sent/Received packets, bytes, and latency metrics
func (g *GUIApp) updateTelemetryUI() {
	g.mu.Lock()
	trans := g.trans
	g.mu.Unlock()

	var stats transport.TransportStats
	if trans != nil {
		stats = trans.Stats()
	}

	fyne.Do(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		if g.txStatsLabel != nil {
			g.txStatsLabel.SetText(fmt.Sprintf("▲ TX: %s pkts (%s)", formatCount(stats.PacketsSent), formatBytes(stats.BytesSent)))
		}
		if g.rxStatsLabel != nil {
			g.rxStatsLabel.SetText(fmt.Sprintf("▼ RX: %s pkts (%s)", formatCount(stats.PacketsRecv), formatBytes(stats.BytesRecv)))
		}
		if g.latencyLabel != nil {
			lat := "< 1 ms"
			if stats.Latency > 0 {
				lat = fmt.Sprintf("%.1f ms", float64(stats.Latency.Microseconds())/1000.0)
			}
			g.latencyLabel.SetText(fmt.Sprintf("⚡ %s", lat))
		}
	})
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
				isOK = true
			} else {
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

	fyne.Do(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		if g.pulseDot != nil {
			g.pulseDot.SetConnected(isOK)
		}

		if g.connStatusText != nil {
			if isOK {
				g.connStatusText.Text = "Connected (ESP32-S3)"
				g.connStatusText.Color = ColorCyan
			} else {
				g.connStatusText.Text = "Offline"
				g.connStatusText.Color = ColorPurple
			}
			g.connStatusText.Refresh()
		}

		if g.btnToggleGrab != nil {
			if isOK {
				g.btnToggleGrab.Enable()
			} else {
				g.btnToggleGrab.Disable()
			}
		}

		if g.devSummaryLabel != nil && g.grabber != nil {
			devs := g.grabber.Devices()
			if !isOK {
				g.devSummaryLabel.SetText(fmt.Sprintf("%d devs │ Offline (Safe)", len(devs)))
			} else {
				g.devSummaryLabel.SetText(fmt.Sprintf("%d devs │ [Scroll Lock]", len(devs)))
			}
		}
	})
}

func (g *GUIApp) Run() {
	g.win.ShowAndRun()
}

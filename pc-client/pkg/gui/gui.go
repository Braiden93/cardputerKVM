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
	ColorCyan   = color.NRGBA{R: 0x00, G: 0xF5, B: 0xD4, A: 0xFF} // Neon Cyan (Dark Mode)
	ColorPurple = color.NRGBA{R: 0x9D, G: 0x4E, B: 0xDD, A: 0xFF} // Purple (Dark Mode)
	ColorGreen  = color.NRGBA{R: 0x2E, G: 0xCC, B: 0x71, A: 0xFF} // Green
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
	btnThemeToggle *widget.Button
	isDarkMode     bool

	// Connection Controls
	portEntry *widget.Entry

	// Compact Telemetry & Status with Cardputer Battery Donut
	batteryDonut    *BatteryDonut
	batterySubLabel *widget.Label
	signalLabel     *widget.Label
	latencyLabel    *widget.Label
	txStatsLabel    *widget.Label
	rxStatsLabel    *widget.Label

	// Slim Compact Footer
	devSummaryText *canvas.Text

	missedHandshakes int
	heartBeatTick    int
	isUpdatingUI     bool
	mu               sync.Mutex
}

func NewGUIApp(trans transport.Transport, grabber *evdev.Grabber) *GUIApp {
	a := app.NewWithID("com.cardputer.kvm")
	// Set default to Light Mode
	a.Settings().SetTheme(theme.LightTheme())

	w := a.NewWindow("Cardputer-KVM ○ PASS-THROUGH")
	w.Resize(fyne.NewSize(480, 140))

	g := &GUIApp{
		fyneApp:    a,
		win:        w,
		trans:      trans,
		grabber:    grabber,
		isDarkMode: false, // Default to light mode
	}

	g.buildUI()
	g.startConnectionMonitor()
	return g
}

func (g *GUIApp) getStatusColor(online bool) color.NRGBA {
	if online {
		if g.isDarkMode {
			return ColorCyan
		}
		return color.NRGBA{R: 0x00, G: 0x7A, B: 0x6E, A: 0xFF} // High contrast teal for light mode
	}
	if g.isDarkMode {
		return ColorPurple
	}
	return color.NRGBA{R: 0x6D, G: 0x28, B: 0xD9, A: 0xFF} // Purple for light mode
}

func (g *GUIApp) toggleTheme() {
	g.mu.Lock()
	g.isDarkMode = !g.isDarkMode
	isDark := g.isDarkMode
	trans := g.trans
	online := trans != nil && trans.IsConnected()
	g.mu.Unlock()

	fyne.Do(func() {
		if isDark {
			g.fyneApp.Settings().SetTheme(theme.DarkTheme())
			if g.btnThemeToggle != nil {
				g.btnThemeToggle.SetText("☀️")
			}
		} else {
			g.fyneApp.Settings().SetTheme(theme.LightTheme())
			if g.btnThemeToggle != nil {
				g.btnThemeToggle.SetText("🌙")
			}
		}

		if g.pulseDot != nil {
			g.pulseDot.SetTheme(isDark)
		}
		if g.batteryDonut != nil {
			g.batteryDonut.SetTheme(isDark)
		}
		if g.connStatusText != nil {
			g.connStatusText.Color = g.getStatusColor(online)
			g.connStatusText.Refresh()
		}
		if g.devSummaryText != nil {
			if isDark {
				g.devSummaryText.Color = color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF}
			} else {
				g.devSummaryText.Color = color.NRGBA{R: 0x64, G: 0x74, B: 0x8B, A: 0xFF}
			}
			g.devSummaryText.Refresh()
		}
	})
}

func (g *GUIApp) buildUI() {
	// --- Header: App Title + Centered Status Dot & Text + Theme Toggle ---
	appTitle := widget.NewLabelWithStyle("Cardputer-KVM", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	isInitOK := false
	if g.trans != nil && g.trans.IsConnected() {
		if verified, _ := g.trans.VerifyHandshake(); verified {
			isInitOK = true
		}
	}

	g.pulseDot = NewPulseDot(isInitOK, g.isDarkMode)

	connStatusStr := "Offline"
	if isInitOK {
		connStatusStr = "Connected (ESP32-S3)"
	}

	g.connStatusText = canvas.NewText(connStatusStr, g.getStatusColor(isInitOK))
	g.connStatusText.TextSize = 12
	g.connStatusText.TextStyle = fyne.TextStyle{Bold: true}

	// Lock the vertical centers of pulseDot and connStatusText together
	statusGroup := container.NewHBox(
		container.NewCenter(g.pulseDot),
		container.NewCenter(g.connStatusText),
	)

	themeBtnText := "🌙"
	if g.isDarkMode {
		themeBtnText = "☀️"
	}
	g.btnThemeToggle = widget.NewButton(themeBtnText, func() {
		g.toggleTheme()
	})

	headerBox := container.NewHBox(
		appTitle,
		layout.NewSpacer(),
		statusGroup,
		g.btnThemeToggle,
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

	// --- Section 2: Compact Status & Telemetry Card with Cardputer Battery Donut ---
	g.batteryDonut = NewBatteryDonut(36, "X", isInitOK, g.isDarkMode)

	battTitle := widget.NewLabelWithStyle("Cardputer Battery", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	battSub := "Firmware pending"
	if !isInitOK {
		battSub = "Cardputer offline"
	}
	g.batterySubLabel = widget.NewLabel(battSub)

	battBox := container.NewHBox(
		container.NewCenter(g.batteryDonut),
		container.NewVBox(battTitle, g.batterySubLabel),
	)

	sigStr := "📶 Offline"
	if isInitOK {
		sigStr = "📶 98% (-46 dBm)"
	}
	g.signalLabel = widget.NewLabel(sigStr)
	g.signalLabel.TextStyle = fyne.TextStyle{Bold: true}

	g.latencyLabel = widget.NewLabel("⚡ < 1 ms")

	g.txStatsLabel = widget.NewLabel("▲ TX: 0 pkts (0 B)")
	g.rxStatsLabel = widget.NewLabel("▼ RX: 0 pkts (0 B)")

	infoCol := container.NewVBox(
		container.NewHBox(g.signalLabel, layout.NewSpacer(), g.latencyLabel),
		container.NewHBox(g.txStatsLabel, layout.NewSpacer(), g.rxStatsLabel),
	)

	statusContent := container.NewBorder(
		nil, nil,
		container.NewHBox(battBox, widget.NewSeparator()),
		nil,
		container.NewPadded(infoCol),
	)

	statusCard := widget.NewCard("", "", statusContent)

	// --- Section 3: Shrunk, Ultra-Slim Footer ---
	devCountStr := "[Scroll Lock] to toggle KVM"
	if g.grabber != nil {
		devs := g.grabber.Devices()
		devCountStr = fmt.Sprintf("%d devices linked │ [Scroll Lock] to engage", len(devs))
	}
	footerColor := color.NRGBA{R: 0x64, G: 0x74, B: 0x8B, A: 0xFF}
	if g.isDarkMode {
		footerColor = color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF}
	}
	g.devSummaryText = canvas.NewText(devCountStr, footerColor)
	g.devSummaryText.TextSize = 11

	footerBox := container.NewHBox(
		g.devSummaryText,
	)

	// Main Layout
	mainContent := container.NewVBox(
		headerBox,
		widget.NewSeparator(),
		portRow,
		statusCard,
		footerBox,
	)

	g.win.SetContent(mainContent)
	// Shrink-wrap the window tightly around the content so there is zero empty space
	g.win.Resize(mainContent.MinSize())

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
		if g.devSummaryText != nil {
			g.devSummaryText.Text = "● KVM Engaged (Input routed to target) │ [Scroll Lock] to return"
			g.devSummaryText.Refresh()
		}
	} else {
		g.win.SetTitle("Cardputer-KVM ○ PASS-THROUGH")
		if g.devSummaryText != nil && g.grabber != nil {
			devs := g.grabber.Devices()
			g.devSummaryText.Text = fmt.Sprintf("%d devices linked │ [Scroll Lock] to engage", len(devs))
			g.devSummaryText.Refresh()
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
			if g.devSummaryText != nil {
				g.devSummaryText.Text = evt.Message
				g.devSummaryText.Refresh()
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
		if g.batteryDonut != nil {
			g.batteryDonut.SetOnline(false)
		}
		if g.batterySubLabel != nil {
			g.batterySubLabel.SetText("Cardputer offline")
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Offline"
			g.connStatusText.Color = g.getStatusColor(false)
			g.connStatusText.Refresh()
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
		if g.batteryDonut != nil {
			g.batteryDonut.SetOnline(true)
		}
		if g.batterySubLabel != nil {
			g.batterySubLabel.SetText("Firmware pending")
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Connected (ESP32-S3)"
			g.connStatusText.Color = g.getStatusColor(true)
			g.connStatusText.Refresh()
		}
	} else {
		if g.pulseDot != nil {
			g.pulseDot.SetConnected(false)
		}
		if g.batteryDonut != nil {
			g.batteryDonut.SetOnline(false)
		}
		if g.batterySubLabel != nil {
			g.batterySubLabel.SetText("Cardputer offline")
		}
		if g.connStatusText != nil {
			g.connStatusText.Text = "Offline"
			g.connStatusText.Color = g.getStatusColor(false)
			g.connStatusText.Refresh()
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
			if g.batteryDonut != nil {
				g.batteryDonut.SetOnline(false)
			}
			if g.batterySubLabel != nil {
				g.batterySubLabel.SetText("Cardputer offline")
			}
			if g.signalLabel != nil {
				g.signalLabel.SetText("📶 Offline")
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
		if g.batteryDonut != nil {
			g.batteryDonut.SetOnline(true)
			g.batteryDonut.SetValue("X", 0.0)
		}
		if g.batterySubLabel != nil {
			g.batterySubLabel.SetText("Firmware pending")
		}
		if g.signalLabel != nil {
			g.signalLabel.SetText("📶 98% (-46 dBm)")
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

		if g.batteryDonut != nil {
			g.batteryDonut.SetOnline(isOK)
		}

		if g.batterySubLabel != nil {
			if isOK {
				g.batterySubLabel.SetText("Firmware pending")
			} else {
				g.batterySubLabel.SetText("Cardputer offline")
			}
		}

		if g.connStatusText != nil {
			if isOK {
				g.connStatusText.Text = "Connected (ESP32-S3)"
			} else {
				g.connStatusText.Text = "Offline"
			}
			g.connStatusText.Color = g.getStatusColor(isOK)
			g.connStatusText.Refresh()
		}

		if g.devSummaryText != nil && g.grabber != nil {
			devs := g.grabber.Devices()
			if !isOK {
				g.devSummaryText.Text = fmt.Sprintf("%d devices detected │ Offline (Safe)", len(devs))
			} else if g.grabber.IsGrabbed() {
				g.devSummaryText.Text = "● KVM Engaged (Input routed to target) │ [Scroll Lock] to return"
			} else {
				g.devSummaryText.Text = fmt.Sprintf("%d devices linked │ [Scroll Lock] to engage", len(devs))
			}
			g.devSummaryText.Refresh()
		}
	})
}

func (g *GUIApp) Run() {
	g.win.ShowAndRun()
}

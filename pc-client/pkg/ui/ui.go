package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/evdev"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
)

type HeartbeatTickMsg time.Time
type GrabMsg evdev.GrabEvent

type Model struct {
	trans   transport.Transport
	grabber *evdev.Grabber

	width  int
	height int

	// KVM State
	isGrabbed        bool
	lastKeyPressed   string
	lastKeyHex       string
	lastModifiers    string
	lastMouseDX      int8
	lastMouseDY      int8
	lastMouseWheel   int8
	lastMouseButtons byte
	lastActionStatus string

	// Hooked Devices
	devices []*evdev.Device

	// Event Log
	activityLog []string
}

func NewModel(trans transport.Transport, grabber *evdev.Grabber) Model {
	devs := grabber.Devices()
	m := Model{
		trans:            trans,
		grabber:          grabber,
		isGrabbed:        grabber.IsGrabbed(),
		lastKeyPressed:   "None",
		lastKeyHex:       "Mod: 0x00 | Keys: 0",
		lastModifiers:    "None",
		lastActionStatus: "Ready — Press [Scroll Lock] to toggle Hardware KVM",
		devices:          devs,
		activityLog: []string{
			fmt.Sprintf("Initialized with %d physical input devices", len(devs)),
			fmt.Sprintf("Connected to %s (%s)", trans.Type(), trans.Endpoint()),
			"RF Protocol: ESP-NOW Channel 1 (Pure Wireless)",
		},
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tickHeartbeat()
}

func tickHeartbeat() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return HeartbeatTickMsg(t)
	})
}

func (m *Model) addLog(entry string) {
	timestamp := time.Now().Format("15:04:05")
	m.activityLog = append(m.activityLog, fmt.Sprintf("[%s] %s", timestamp, entry))
	if len(m.activityLog) > 8 {
		m.activityLog = m.activityLog[len(m.activityLog)-8:]
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case HeartbeatTickMsg:
		if m.trans != nil && m.trans.IsConnected() {
			_ = m.trans.SendHeartbeat()
		}
		return m, tickHeartbeat()

	case GrabMsg:
		evt := evdev.GrabEvent(msg)
		switch evt.Type {
		case evdev.EvtGrabToggled:
			m.isGrabbed = evt.Grabbed
			if evt.Grabbed {
				m.lastActionStatus = "● KVM ENGAGED: Physical Input Exclusively Locked to Cardputer"
				m.addLog("⚡ [Scroll Lock] KVM ENGAGED — Physical keyboard & mouse grabbed")
			} else {
				m.lastActionStatus = "○ PASS-THROUGH: Input returned to local Linux desktop"
				m.addLog("⚡ [Scroll Lock] KVM RELEASED — Local Linux desktop active")
			}

		case evdev.EvtKeyAction:
			m.lastKeyPressed = evt.KeyName
			m.lastKeyHex = evt.KeyHex
			m.lastModifiers = evt.Modifiers
			if evt.Modifiers != "" {
				m.addLog(fmt.Sprintf("Key: %s+%s (%s)", evt.Modifiers, evt.KeyName, evt.Message))
			} else {
				m.addLog(fmt.Sprintf("Key: %s", evt.Message))
			}

		case evdev.EvtMouseAction:
			m.lastMouseDX = evt.MouseDX
			m.lastMouseDY = evt.MouseDY
			m.lastMouseWheel = evt.MouseWheel
			m.lastMouseButtons = evt.MouseButtons
		}
		return m, nil

	case tea.KeyMsg:
		// Terminal stdin keys (only active when ungrabbed, or from secondary terminal)
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s", " ":
			// Software toggle shortcut from terminal
			m.grabber.ToggleGrab()
			return m, nil
		case "p":
			if m.trans != nil {
				_ = m.trans.SendHeartbeat()
				m.addLog("Manual heartbeat ping dispatched.")
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	var s strings.Builder

	// 1. Top Header Banner
	title := HeaderTitle.Render("CARDPUTER HARDWARE KVM SWITCH")
	badge := HeaderBadge.Render("SERIAL DONGLE")
	profile := HeaderProfile.Render("TARGET: Dell KB216 + Mouse (0x413C:0x2113)")

	var statusBadge string
	if m.trans != nil && m.trans.IsConnected() {
		statusBadge = StatusConnected.Render("● " + m.trans.Endpoint())
	} else {
		statusBadge = StatusDisconnected.Render("○ DISCONNECTED")
	}

	topBar := lipgloss.JoinHorizontal(lipgloss.Center, title, badge, profile, "  ", statusBadge)
	s.WriteString(topBar + "\n\n")

	// 2. PRIMARY KVM SWITCH HERO DISPLAY
	if m.isGrabbed {
		s.WriteString(m.renderGrabbedBanner() + "\n\n")
	} else {
		s.WriteString(m.renderPassThroughBanner() + "\n\n")
	}

	// 3. Middle Section: Hooked Devices & Live Telemetry
	devicesBox := m.renderDevicesBox()
	telemetryBox := m.renderTelemetryBox()

	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, devicesBox, "   ", telemetryBox)
	s.WriteString(middleRow + "\n\n")

	// 4. Activity Log Feed
	s.WriteString(m.renderActivityLog() + "\n\n")

	// 5. Footer Shortcuts
	s.WriteString(m.renderFooter())

	return AppStyle.Render(s.String())
}

func (m Model) renderGrabbedBanner() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSlateBg).
		Background(ColorGreen).
		Padding(0, 3).
		Render("● [ TARGET HOST ENGAGED ] — EXCLUSIVE PHYSICAL INPUT GRAB ACTIVE")

	desc := lipgloss.JoinVertical(lipgloss.Center,
		"\n"+lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render("All physical keyboard keystrokes, shortcuts, and mouse movements are routed to Cardputer."),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("Local Linux desktop input is BLOCKED to prevent accidental typing or cursor drift."),
		"\n"+lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Render(">>> PRESS [SCROLL LOCK] TO RELEASE CONTROL BACK TO LOCAL DESKTOP <<<"),
		lipgloss.NewStyle().Foreground(ColorPrimary).Render("Keyboard Scroll Lock LED is illuminated"),
	)

	content := lipgloss.JoinVertical(lipgloss.Center, title, desc)
	return CardBoxActive.Width(82).Align(lipgloss.Center).Render(content)
}

func (m Model) renderPassThroughBanner() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSlateBg).
		Background(ColorYellow).
		Padding(0, 3).
		Render("○ [ LOCAL DESKTOP PASS-THROUGH ] — NORMAL LOCAL OPERATION")

	desc := lipgloss.JoinVertical(lipgloss.Center,
		"\n"+lipgloss.NewStyle().Foreground(ColorText).Render("Your keyboard and mouse are operating locally on this Linux workstation."),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("Cardputer USB HID is standing by on ESP-NOW Channel 1."),
		"\n"+lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(">>> PRESS [SCROLL LOCK] TO LOCK AND CONTROL TARGET PC <<<"),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("(Or press [Space] in this terminal to toggle)"),
	)

	content := lipgloss.JoinVertical(lipgloss.Center, title, desc)
	return CardBox.Width(82).Align(lipgloss.Center).Render(content)
}

func (m Model) renderDevicesBox() string {
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("HOOKED INPUT HARDWARE (/dev/input/):"), "")

	for _, dev := range m.devices {
		icon := "⌨"
		kind := "KBD"
		if dev.IsMouse {
			icon = "🖱"
			kind = "PTR"
		}

		name := dev.Name
		if len(name) > 28 {
			name = name[:25] + "..."
		}

		stateTag := lipgloss.NewStyle().Foreground(ColorGreen).Render("[GRABBED]")
		if !m.isGrabbed {
			stateTag = lipgloss.NewStyle().Foreground(ColorTextDim).Render("[LOCAL]")
		}

		line := fmt.Sprintf("%s [%s] %-18s %-28s %s", icon, kind, dev.Path, name, stateTag)
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorText).Render(line))
	}

	return CardBox.Width(46).Render(strings.Join(rows, "\n"))
}

func (m Model) renderTelemetryBox() string {
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("LIVE INPUT & RF TELEMETRY:"), "")

	stats := m.trans.Stats()

	// Active Key Display
	keyDisp := m.lastKeyPressed
	if m.lastModifiers != "" && m.lastModifiers != "None" {
		keyDisp = m.lastModifiers + "+" + keyDisp
	}

	// Active Mouse Buttons
	var btnStr []string
	if m.lastMouseButtons&protocol.MouseBtnLeft != 0 {
		btnStr = append(btnStr, "LEFT")
	}
	if m.lastMouseButtons&protocol.MouseBtnRight != 0 {
		btnStr = append(btnStr, "RIGHT")
	}
	if m.lastMouseButtons&protocol.MouseBtnMiddle != 0 {
		btnStr = append(btnStr, "MID")
	}
	btnDisplay := "None"
	if len(btnStr) > 0 {
		btnDisplay = strings.Join(btnStr, "+")
	}

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Active Key:"), StatValue.Render(keyDisp)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Key Report:"), lipgloss.NewStyle().Foreground(ColorTextDim).Render(m.lastKeyHex)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Mouse Delta:"), StatValue.Render(fmt.Sprintf("dX:%+4d  dY:%+4d", m.lastMouseDX, m.lastMouseDY))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Mouse Clicks:"), lipgloss.NewStyle().Foreground(ColorYellow).Render(btnDisplay)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Packets Sent:"), StatValue.Render(fmt.Sprintf("%d", stats.PacketsSent))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Wire Speed:"), lipgloss.NewStyle().Foreground(ColorTextDim).Render(m.trans.Endpoint())),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Radio Link:"), lipgloss.NewStyle().Foreground(ColorGreen).Render("ESP-NOW CH 1 Broadcast")),
	)

	return CardBox.Width(33).Render(strings.Join(rows, "\n"))
}

func (m Model) renderActivityLog() string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorTextDim).Render("HARDWARE EVENT STREAM:"))

	for _, log := range m.activityLog {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextDim).Render(log))
	}

	return CardBox.Width(82).Render(strings.Join(lines, "\n"))
}

func (m Model) renderFooter() string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		FooterKey.Render("[Scroll Lock]"), FooterDesc.Render(" Toggle Target/Local"),
		FooterKey.Render("[Space]"), FooterDesc.Render(" Quick Toggle"),
		FooterKey.Render("[p]"), FooterDesc.Render(" Ping Heartbeat"),
		FooterKey.Render("[q / Ctrl+C]"), FooterDesc.Render(" Exit Client"),
	)
}

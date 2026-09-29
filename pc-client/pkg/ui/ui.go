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
	var devs []*evdev.Device
	isGrabbed := false
	if grabber != nil {
		devs = grabber.Devices()
		isGrabbed = grabber.IsGrabbed()
	}
	m := Model{
		trans:            trans,
		grabber:          grabber,
		isGrabbed:        isGrabbed,
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
			if m.grabber != nil {
				m.grabber.ToggleGrab()
			}
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

	// 1. Top Header Bar
	s.WriteString(m.renderTopBar() + "\n\n")

	// 2. Compact KVM Status Strip (Shrunk Hero Bar)
	s.WriteString(m.renderStatusStrip() + "\n\n")

	// 3. Middle Section: Dual Telemetry Cards (Input Monitor + Link & Dongle)
	inputCard := m.renderInputTelemetryCard()
	linkCard := m.renderLinkTelemetryCard()
	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, inputCard, "  ", linkCard)
	s.WriteString(middleRow + "\n\n")

	// 4. Hardware Activity Log Stream
	s.WriteString(m.renderActivityLog() + "\n\n")

	// 5. Footer Shortcuts
	s.WriteString(m.renderFooter())

	return AppStyle.Render(s.String())
}

func (m Model) renderTopBar() string {
	boxWidth := 84
	title := HeaderTitle.Render("CARDPUTER HARDWARE KVM")
	badgeRate := HeaderRate.Render("1000Hz HID")
	badgeDongle := HeaderBadge.Render("SERIAL DONGLE")

	var statusBadge string
	if m.trans != nil && m.trans.IsConnected() {
		statusBadge = StatusConnected.Render("● " + m.trans.Endpoint())
	} else {
		statusBadge = StatusDisconnected.Render("○ DISCONNECTED")
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, title, " ", badgeRate, " ", badgeDongle)
	gap := boxWidth - lipgloss.Width(left) - lipgloss.Width(statusBadge)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + statusBadge
}

func (m Model) renderStatusStrip() string {
	boxWidth := 84
	innerWidth := boxWidth - 4 // minus borders and padding

	if m.isGrabbed {
		tag := PillGrabbed.Render("● TARGET ENGAGED")
		statusText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(" KVM LOCKED TO HOST")
		hotkey := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[Scroll Lock] to Return to Local")

		leftPart := tag + " " + statusText
		gap := innerWidth - lipgloss.Width(leftPart) - lipgloss.Width(hotkey)
		if gap < 2 {
			gap = 2
		}
		line1 := leftPart + strings.Repeat(" ", gap) + hotkey

		subtext := lipgloss.NewStyle().Foreground(ColorTextDim).Render(
			"Keystrokes & mouse stream to Cardputer HID (1000Hz) • Local desktop blocked",
		)

		content := line1 + "\n" + subtext
		return CardBoxSlimActive.Width(boxWidth).Render(content)
	}

	// Pass-through (Local)
	tag := PillPassThrough.Render("○ PASS-THROUGH")
	statusText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(" WORKSTATION DESKTOP ACTIVE")
	hotkey := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("[Scroll Lock] to Lock Target")

	leftPart := tag + " " + statusText
	gap := innerWidth - lipgloss.Width(leftPart) - lipgloss.Width(hotkey)
	if gap < 2 {
		gap = 2
	}
	line1 := leftPart + strings.Repeat(" ", gap) + hotkey

	subtext := lipgloss.NewStyle().Foreground(ColorTextDim).Render(
		"Workstation desktop active locally • Cardputer HID standby on ESP-NOW Ch 1",
	)

	content := line1 + "\n" + subtext
	return CardBoxSlim.Width(boxWidth).Render(content)
}

func (m Model) renderInputTelemetryCard() string {
	cardWidth := 40
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("⌨  LIVE INPUT MONITOR"), "")

	// Active Key Display
	keyDisp := m.lastKeyPressed
	if m.lastModifiers != "" && m.lastModifiers != "None" {
		keyDisp = m.lastModifiers + "+" + keyDisp
	}
	if len(keyDisp) > 18 {
		keyDisp = keyDisp[:15] + "..."
	}

	// Mouse Clicks HUD Badges
	btnL := BtnInactive.Render("[L]")
	if m.lastMouseButtons&protocol.MouseBtnLeft != 0 {
		btnL = BtnActive.Render("[L]")
	}
	btnM := BtnInactive.Render("[M]")
	if m.lastMouseButtons&protocol.MouseBtnMiddle != 0 {
		btnM = BtnActive.Render("[M]")
	}
	btnR := BtnInactive.Render("[R]")
	if m.lastMouseButtons&protocol.MouseBtnRight != 0 {
		btnR = BtnActive.Render("[R]")
	}
	btnsHUD := lipgloss.JoinHorizontal(lipgloss.Left, btnL, " ", btnM, " ", btnR)

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Active Key:"), StatValue.Render(keyDisp)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("HID Report:"), StatValueDim.Render(m.lastKeyHex)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Mouse Delta:"), StatValue.Render(fmt.Sprintf("dX:%+4d  dY:%+4d", m.lastMouseDX, m.lastMouseDY))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Mouse Wheel:"), StatValueDim.Render(fmt.Sprintf("%+2d", m.lastMouseWheel))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Mouse Clicks:"), btnsHUD),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Engine Rate:"), lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("1000 Hz (1ms)")),
	)

	return CardBoxSlim.Width(cardWidth).Render(strings.Join(rows, "\n"))
}

func (m Model) renderLinkTelemetryCard() string {
	cardWidth := 42
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("⚡ LINK & TELEMETRY"), "")

	stats := m.trans.Stats()

	// Short endpoint if needed
	ep := m.trans.Endpoint()
	if len(ep) > 23 {
		ep = ep[:20] + "..."
	}

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Wire Endpoint:"), StatValueDim.Render(ep)),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Serial Baud:"), StatValue.Render("921,600 (97µs/pkt)")),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("RF Protocol:"), lipgloss.NewStyle().Foreground(ColorGreen).Render("ESP-NOW Ch 1 (24M PHY)")),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Target Spoof:"), lipgloss.NewStyle().Foreground(ColorYellow).Render("Dell Dongle (413C:2501)")),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Packets Sent:"), StatValue.Render(fmt.Sprintf("%d pkts", stats.PacketsSent))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Link State:"), lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("● Synchronized")),
	)

	return CardBoxSlim.Width(cardWidth).Render(strings.Join(rows, "\n"))
}

func (m Model) renderActivityLog() string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorTextDim).Render("◆ HARDWARE EVENT STREAM:"), "")

	count := len(m.activityLog)
	start := 0
	if count > 5 {
		start = count - 5
	}
	for _, log := range m.activityLog[start:] {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextDim).Render(log))
	}

	return CardBoxSlim.Width(84).Render(strings.Join(lines, "\n"))
}

func (m Model) renderFooter() string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		FooterKey.Render("[Scroll Lock]"), FooterDesc.Render(" Toggle Target/Local  "),
		FooterKey.Render("[Space]"), FooterDesc.Render(" Quick Toggle  "),
		FooterKey.Render("[p]"), FooterDesc.Render(" Ping  "),
		FooterKey.Render("[q / Ctrl+C]"), FooterDesc.Render(" Exit Client"),
	)
}

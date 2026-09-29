package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
)

type TabMode int

const (
	TabLiveCapture TabMode = 0
	TabMouse       TabMode = 1
	TabMacros      TabMode = 2
	TabInjector    TabMode = 3
	TabTelemetry   TabMode = 4
)

type HeartbeatTickMsg time.Time

type MacroItem struct {
	Key         string
	Name        string
	Description string
	Modifiers   byte
	Keycode     byte
}

type Model struct {
	trans            transport.Transport
	activeTab        TabMode
	captureActive    bool
	width            int
	height           int
	lastKeyPressed   string
	lastKeyHex       string
	lastActionStatus string
	actionErr        string
	lastPacketTime   time.Time

	// Mouse State
	mouseButtons byte
	lastMouseDX  int8
	lastMouseDY  int8

	// Macro Selection
	macroIndex int
	macros     []MacroItem

	// Text Injector
	textInput     textinput.Model
	injectDelayMs int
	isInjecting   bool

	// Log Buffer
	activityLog []string
}

func NewModel(trans transport.Transport) Model {
	ti := textinput.New()
	ti.Placeholder = "Type or paste commands/text to inject into host PC..."
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 56

	macros := []MacroItem{
		{Key: "1", Name: "Ctrl+Alt+Del", Description: "Security / Task Manager / Sign out", Modifiers: protocol.ModLeftCtrl | protocol.ModLeftAlt, Keycode: protocol.KeyDelete},
		{Key: "2", Name: "Alt+Tab", Description: "Switch active window", Modifiers: protocol.ModLeftAlt, Keycode: protocol.KeyTab},
		{Key: "3", Name: "Win+R", Description: "Open Run Dialog", Modifiers: protocol.ModLeftGUI, Keycode: protocol.KeyR},
		{Key: "4", Name: "Win+X", Description: "Power User / Quick Link Menu", Modifiers: protocol.ModLeftGUI, Keycode: protocol.KeyX},
		{Key: "5", Name: "Ctrl+Shift+Esc", Description: "Direct Task Manager launch", Modifiers: protocol.ModLeftCtrl | protocol.ModLeftShift, Keycode: protocol.KeyEscape},
		{Key: "6", Name: "Alt+F4", Description: "Close active application", Modifiers: protocol.ModLeftAlt, Keycode: protocol.KeyF4},
		{Key: "7", Name: "Win+L", Description: "Lock target workstation", Modifiers: protocol.ModLeftGUI, Keycode: protocol.KeyL},
		{Key: "8", Name: "PrtScn", Description: "Capture full screen screenshot", Modifiers: protocol.ModNone, Keycode: protocol.KeyPrintScreen},
		{Key: "9", Name: "Win+D", Description: "Show / minimize all to desktop", Modifiers: protocol.ModLeftGUI, Keycode: protocol.KeyD},
		{Key: "0", Name: "Win+E", Description: "Launch File Explorer", Modifiers: protocol.ModLeftGUI, Keycode: protocol.KeyE},
		{Key: "c", Name: "Ctrl+C", Description: "Interrupt process / copy", Modifiers: protocol.ModLeftCtrl, Keycode: protocol.KeyC},
		{Key: "v", Name: "Ctrl+V", Description: "Paste clipboard content", Modifiers: protocol.ModLeftCtrl, Keycode: protocol.KeyV},
	}

	m := Model{
		trans:            trans,
		activeTab:        TabLiveCapture,
		captureActive:    true,
		lastKeyPressed:   "None",
		lastKeyHex:       "Mod: 0x00 | Code: 0x00",
		lastActionStatus: "Ready — Link: " + trans.Endpoint(),
		macros:           macros,
		textInput:        ti,
		injectDelayMs:    15,
		activityLog:      []string{"System initialized. Channel: " + trans.Type() + " -> ESP-NOW -> Cardputer"},
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickHeartbeat(),
	)
}

func tickHeartbeat() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return HeartbeatTickMsg(t)
	})
}

func (m *Model) addLog(entry string) {
	timestamp := time.Now().Format("15:04:05")
	m.activityLog = append(m.activityLog, fmt.Sprintf("[%s] %s", timestamp, entry))
	if len(m.activityLog) > 10 {
		m.activityLog = m.activityLog[len(m.activityLog)-10:]
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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

	case tea.KeyMsg:
		// Universal Quit handler when not in capture mode
		if !m.captureActive && (msg.String() == "ctrl+c" || msg.String() == "q") {
			return m, tea.Quit
		}

		// When in Live Capture Mode (Keyboard):
		if m.captureActive && m.activeTab == TabLiveCapture {
			// Escape hatch from capture mode: double Esc, or Ctrl+Q, or F12
			if msg.Type == tea.KeyEsc || msg.String() == "ctrl+q" || msg.Type == tea.KeyF12 {
				m.captureActive = false
				m.addLog("Exited Live Capture Mode.")
				m.lastActionStatus = "Live Capture PAUSED. Press 'Space' to resume."
				return m, nil
			}

			// Forward key to Transmitter!
			mod, code, desc := parseKeyToHID(msg)
			if code != protocol.KeyNone || mod != protocol.ModNone {
				err := m.trans.SendKey(mod, code)
				m.lastKeyPressed = desc
				m.lastKeyHex = fmt.Sprintf("Mod: 0x%02X | Code: 0x%02X", mod, code)
				m.lastPacketTime = time.Now()
				if err != nil {
					m.actionErr = err.Error()
					m.addLog(fmt.Sprintf("ERR sending %s: %v", desc, err))
				} else {
					m.actionErr = ""
					m.lastActionStatus = fmt.Sprintf("Forwarded %s -> ESP32-S3 -> Cardputer -> Host PC", desc)
					m.addLog(fmt.Sprintf("Key sent: %s (0x%02X)", desc, code))
				}
			}
			return m, nil
		}

		// Navigation when outside Capture Mode:
		switch msg.String() {
		case "tab":
			m.activeTab = (m.activeTab + 1) % 5
			return m, nil
		case "shift+tab":
			if m.activeTab == 0 {
				m.activeTab = TabTelemetry
			} else {
				m.activeTab--
			}
			return m, nil
		case "1":
			if m.activeTab != TabMacros && m.activeTab != TabInjector {
				m.activeTab = TabLiveCapture
				return m, nil
			}
		case "2":
			if m.activeTab != TabMacros && m.activeTab != TabInjector {
				m.activeTab = TabMouse
				return m, nil
			}
		case "3":
			if m.activeTab != TabMacros && m.activeTab != TabInjector {
				m.activeTab = TabMacros
				return m, nil
			}
		case "4":
			if m.activeTab != TabMacros && m.activeTab != TabInjector {
				m.activeTab = TabInjector
				return m, nil
			}
		case "5":
			if m.activeTab != TabMacros && m.activeTab != TabInjector {
				m.activeTab = TabTelemetry
				return m, nil
			}
		case " ":
			// Space enters Live Capture mode if on tab 0
			if m.activeTab == TabLiveCapture {
				m.captureActive = true
				m.addLog("Entered Live Capture Mode. All keys routed to host.")
				m.lastActionStatus = "LIVE CAPTURE ACTIVE! Routing keys to target host..."
				return m, nil
			}
		}

		// Tab-specific handlers
		switch m.activeTab {
		case TabMouse:
			// Virtual Mouse Pad Controls
			step := int8(12)
			if msg.Alt {
				step = int8(32) // Turbo mode
			}
			switch msg.String() {
			case "up", "k":
				_ = m.trans.SendMouse(0, 0, -step, 0)
				m.lastMouseDY = -step
				m.lastMouseDX = 0
				m.lastActionStatus = fmt.Sprintf("Mouse Move: dY=%d", -step)
				return m, nil
			case "down", "j":
				_ = m.trans.SendMouse(0, 0, step, 0)
				m.lastMouseDY = step
				m.lastMouseDX = 0
				m.lastActionStatus = fmt.Sprintf("Mouse Move: dY=%d", step)
				return m, nil
			case "left", "h":
				_ = m.trans.SendMouse(0, -step, 0, 0)
				m.lastMouseDX = -step
				m.lastMouseDY = 0
				m.lastActionStatus = fmt.Sprintf("Mouse Move: dX=%d", -step)
				return m, nil
			case "right", "l":
				_ = m.trans.SendMouse(0, step, 0, 0)
				m.lastMouseDX = step
				m.lastMouseDY = 0
				m.lastActionStatus = fmt.Sprintf("Mouse Move: dX=%d", step)
				return m, nil
			case "enter", " ":
				// Left Click (press and release)
				_ = m.trans.SendMouse(protocol.MouseBtnLeft, 0, 0, 0)
				time.Sleep(30 * time.Millisecond)
				_ = m.trans.SendMouse(0, 0, 0, 0)
				m.lastActionStatus = "Mouse: Left Click"
				m.addLog("Mouse Left Click")
				return m, nil
			case "r":
				// Right Click
				_ = m.trans.SendMouse(protocol.MouseBtnRight, 0, 0, 0)
				time.Sleep(30 * time.Millisecond)
				_ = m.trans.SendMouse(0, 0, 0, 0)
				m.lastActionStatus = "Mouse: Right Click"
				m.addLog("Mouse Right Click")
				return m, nil
			case "m":
				// Middle Click
				_ = m.trans.SendMouse(protocol.MouseBtnMiddle, 0, 0, 0)
				time.Sleep(30 * time.Millisecond)
				_ = m.trans.SendMouse(0, 0, 0, 0)
				m.lastActionStatus = "Mouse: Middle Click"
				m.addLog("Mouse Middle Click")
				return m, nil
			case "w":
				// Scroll Up
				_ = m.trans.SendMouse(0, 0, 0, 1)
				m.lastActionStatus = "Mouse: Wheel Up"
				return m, nil
			case "s":
				// Scroll Down
				_ = m.trans.SendMouse(0, 0, 0, -1)
				m.lastActionStatus = "Mouse: Wheel Down"
				return m, nil
			}

		case TabMacros:
			// Execute macro by key
			keyStr := msg.String()
			for i, macro := range m.macros {
				if macro.Key == keyStr {
					m.macroIndex = i
					err := m.trans.SendKey(macro.Modifiers, macro.Keycode)
					if err != nil {
						m.actionErr = err.Error()
					} else {
						m.actionErr = ""
						m.lastKeyPressed = macro.Name
						m.lastKeyHex = fmt.Sprintf("Mod: 0x%02X | Code: 0x%02X", macro.Modifiers, macro.Keycode)
						m.lastActionStatus = fmt.Sprintf("Triggered Macro [%s] -> Host PC", macro.Name)
						m.addLog(fmt.Sprintf("Macro executed: %s", macro.Name))
					}
					return m, nil
				}
			}

		case TabInjector:
			if msg.Type == tea.KeyEnter {
				textToInject := m.textInput.Value()
				if textToInject != "" {
					m.isInjecting = true
					m.addLog(fmt.Sprintf("Streaming text (%d chars)...", len(textToInject)))
					delay := time.Duration(m.injectDelayMs) * time.Millisecond
					err := m.trans.SendString(textToInject, delay)
					m.isInjecting = false
					if err != nil {
						m.actionErr = err.Error()
						m.addLog(fmt.Sprintf("Injection failed: %v", err))
					} else {
						m.lastActionStatus = fmt.Sprintf("Successfully injected %d chars to Host PC!", len(textToInject))
						m.addLog(fmt.Sprintf("Injected %d chars via Cardputer", len(textToInject)))
						m.textInput.Reset()
					}
				}
				return m, nil
			}
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd

		case TabTelemetry:
			if msg.String() == "p" {
				err := m.trans.SendHeartbeat()
				if err != nil {
					m.actionErr = err.Error()
				} else {
					m.addLog("Manual heartbeat ping dispatched.")
					m.lastActionStatus = "Heartbeat ping sent to Transmitter"
				}
				return m, nil
			}
		}
	}

	return m, nil
}

// Convert Bubbletea KeyMsg to USB HID codes
func parseKeyToHID(msg tea.KeyMsg) (byte, byte, string) {
	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			r := msg.Runes[0]
			mod, code := protocol.CharToHID(r)
			if msg.Alt {
				mod |= protocol.ModLeftAlt
			}
			name := string(r)
			if mod&protocol.ModLeftShift != 0 {
				name = "Shift+" + name
			}
			if msg.Alt {
				name = "Alt+" + name
			}
			return mod, code, name
		}

	case tea.KeyEnter:
		return protocol.ModNone, protocol.KeyEnter, "Enter"
	case tea.KeyBackspace:
		return protocol.ModNone, protocol.KeyBackspace, "Backspace"
	case tea.KeyTab:
		return protocol.ModNone, protocol.KeyTab, "Tab"
	case tea.KeySpace:
		return protocol.ModNone, protocol.KeySpace, "Space"
	case tea.KeyEsc:
		return protocol.ModNone, protocol.KeyEscape, "Esc"
	case tea.KeyUp:
		return protocol.ModNone, protocol.KeyUpArrow, "Up Arrow"
	case tea.KeyDown:
		return protocol.ModNone, protocol.KeyDownArrow, "Down Arrow"
	case tea.KeyLeft:
		return protocol.ModNone, protocol.KeyLeftArrow, "Left Arrow"
	case tea.KeyRight:
		return protocol.ModNone, protocol.KeyRightArrow, "Right Arrow"
	case tea.KeyDelete:
		return protocol.ModNone, protocol.KeyDelete, "Delete"
	case tea.KeyHome:
		return protocol.ModNone, protocol.KeyHome, "Home"
	case tea.KeyEnd:
		return protocol.ModNone, protocol.KeyEnd, "End"
	case tea.KeyPgUp:
		return protocol.ModNone, protocol.KeyPageUp, "Page Up"
	case tea.KeyPgDown:
		return protocol.ModNone, protocol.KeyPageDown, "Page Down"

	case tea.KeyF1:
		return protocol.ModNone, protocol.KeyF1, "F1"
	case tea.KeyF2:
		return protocol.ModNone, protocol.KeyF2, "F2"
	case tea.KeyF3:
		return protocol.ModNone, protocol.KeyF3, "F3"
	case tea.KeyF4:
		return protocol.ModNone, protocol.KeyF4, "F4"
	case tea.KeyF5:
		return protocol.ModNone, protocol.KeyF5, "F5"
	case tea.KeyF6:
		return protocol.ModNone, protocol.KeyF6, "F6"
	case tea.KeyF7:
		return protocol.ModNone, protocol.KeyF7, "F7"
	case tea.KeyF8:
		return protocol.ModNone, protocol.KeyF8, "F8"
	case tea.KeyF9:
		return protocol.ModNone, protocol.KeyF9, "F9"
	case tea.KeyF10:
		return protocol.ModNone, protocol.KeyF10, "F10"
	case tea.KeyF11:
		return protocol.ModNone, protocol.KeyF11, "F11"
	case tea.KeyF12:
		return protocol.ModNone, protocol.KeyF12, "F12"
	}

	// Ctrl combinations
	str := msg.String()
	if strings.HasPrefix(str, "ctrl+") && len(str) == 6 {
		char := str[5]
		if char >= 'a' && char <= 'z' {
			code := protocol.KeyA + (char - 'a')
			return protocol.ModLeftCtrl, code, fmt.Sprintf("Ctrl+%c", char-32)
		}
	}

	return protocol.ModNone, protocol.KeyNone, msg.String()
}

func (m Model) View() string {
	var s strings.Builder

	// 1. Top Header Banner
	title := HeaderTitle.Render("CARDPUTER-KVM")
	badge := HeaderBadge.Render(m.trans.Type() + " LINK")
	profile := HeaderProfile.Render("HOST: Dell KB216 (0x413C:0x2113)")

	var statusBadge string
	if m.trans != nil && m.trans.IsConnected() {
		statusBadge = StatusConnected.Render("● LINKED: " + m.trans.Endpoint())
	} else {
		statusBadge = StatusDisconnected.Render("○ DISCONNECTED")
	}

	topBar := lipgloss.JoinHorizontal(lipgloss.Center, title, badge, profile, "  ", statusBadge)
	s.WriteString(topBar + "\n\n")

	// 2. Navigation Tabs
	tabNames := []string{"[1] KEYBOARD CAPTURE", "[2] MOUSE TRACKPAD", "[3] MACRO DECK", "[4] TEXT INJECTOR", "[5] TELEMETRY"}
	var tabs []string
	for i, name := range tabNames {
		if TabMode(i) == m.activeTab {
			tabs = append(tabs, TabActive.Render(name))
		} else {
			tabs = append(tabs, TabInactive.Render(name))
		}
	}
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n\n")

	// 3. Tab Content View
	switch m.activeTab {
	case TabLiveCapture:
		s.WriteString(m.renderLiveCaptureView())
	case TabMouse:
		s.WriteString(m.renderMouseView())
	case TabMacros:
		s.WriteString(m.renderMacrosView())
	case TabInjector:
		s.WriteString(m.renderInjectorView())
	case TabTelemetry:
		s.WriteString(m.renderTelemetryView())
	}

	// 4. Activity Log Feed
	s.WriteString("\n" + m.renderActivityLog() + "\n")

	// 5. Bottom Help / Shortcut Line
	s.WriteString(m.renderFooter())

	return AppStyle.Render(s.String())
}

func (m Model) renderLiveCaptureView() string {
	var content strings.Builder

	if m.captureActive {
		banner := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorGreen).
			Padding(0, 2).
			Render("● LIVE KEYBOARD CAPTURE ENGAGED — BEAMING VIA " + m.trans.Type() + " -> ESP-NOW -> HOST")
		content.WriteString(banner + "\n\n")

		hudTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorTextDim).Render("ACTIVE TRANSMITTED KEY:")
		keyDisplay := KeyPill.Render(m.lastKeyPressed)
		hexDisplay := lipgloss.NewStyle().Foreground(ColorPrimary).Render(m.lastKeyHex)

		hudContent := lipgloss.JoinVertical(lipgloss.Center,
			hudTitle,
			"\n"+keyDisplay+"\n",
			hexDisplay,
			"\n"+lipgloss.NewStyle().Foreground(ColorYellow).Render("Press [Esc] or [Ctrl+Q] to pause capture mode"),
		)

		box := CardBoxActive.Width(76).Render(hudContent)
		content.WriteString(box)
	} else {
		banner := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorYellow).
			Padding(0, 2).
			Render("⏸ CAPTURE PAUSED — TERMINAL INTERACTION ACTIVE")
		content.WriteString(banner + "\n\n")

		pausedMsg := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render("Keyboard capture paused. Terminal is local."),
			"\n"+lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Press [SPACEBAR] to resume live typing to remote host!"),
			"\n"+lipgloss.NewStyle().Foreground(ColorTextDim).Render("Or press [Tab] / [2-5] to browse Mouse, Macros, and Injector."),
		)

		box := CardBox.Width(76).Render(pausedMsg)
		content.WriteString(box)
	}

	return content.String()
}

func (m Model) renderMouseView() string {
	var rows []string

	header := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("VIRTUAL MOUSE & TRACKPAD CONTROLLER")
	sub := lipgloss.NewStyle().Foreground(ColorTextDim).Render("Control remote host cursor directly via ESP32-S3 -> ESP-NOW -> Cardputer")
	rows = append(rows, header, sub, "")

	controls := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("Movement:  "), lipgloss.NewStyle().Foreground(ColorText).Render("[Arrow Keys] or [H, J, K, L] (Hold Alt for Turbo 3x)")),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("Clicks:    "), lipgloss.NewStyle().Foreground(ColorText).Render("[Space / Enter] Left Click  |  [r] Right Click  |  [m] Middle Click")),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Scroll:    "), lipgloss.NewStyle().Foreground(ColorText).Render("[w] Scroll Up  |  [s] Scroll Down")),
	)
	rows = append(rows, controls, "")

	lastMove := fmt.Sprintf("Last Motion: dX=%d, dY=%d | Status: %s", m.lastMouseDX, m.lastMouseDY, m.lastActionStatus)
	rows = append(rows, lipgloss.NewStyle().Foreground(ColorPrimary).Render(lastMove))

	return CardBox.Width(76).Render(strings.Join(rows, "\n"))
}

func (m Model) renderMacrosView() string {
	var rows []string

	header := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("ONE-TOUCH SYSTEM MACROS (Press corresponding key 1-9, 0, c, v):")
	rows = append(rows, header, "")

	for i := 0; i < len(m.macros); i += 2 {
		btn1 := m.renderMacroButton(m.macros[i])
		var btn2 string
		if i+1 < len(m.macros) {
			btn2 = m.renderMacroButton(m.macros[i+1])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, btn1, "   ", btn2))
	}

	return CardBox.Width(76).Render(strings.Join(rows, "\n"))
}

func (m Model) renderMacroButton(item MacroItem) string {
	tag := MacroKeyTag.Render("[" + item.Key + "]")
	name := MacroTitle.Render(item.Name)
	desc := lipgloss.NewStyle().Foreground(ColorTextDim).Render(item.Description)

	content := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Left, tag, " ", name),
		desc,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Background(ColorCardAlt).
		Padding(0, 1).
		Width(35).
		Render(content)
}

func (m Model) renderInjectorView() string {
	var rows []string

	header := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("FAST STRING & SCRIPT INJECTOR")
	sub := lipgloss.NewStyle().Foreground(ColorTextDim).Render("Streams typed or pasted text directly to the host PC via Transmitter Serial dongle.")
	rows = append(rows, header, sub, "")

	rows = append(rows, m.textInput.View(), "")

	speedInfo := fmt.Sprintf("Character Delay: %d ms | Press [Enter] to inject to Host", m.injectDelayMs)
	rows = append(rows, lipgloss.NewStyle().Foreground(ColorYellow).Render(speedInfo))

	if m.isInjecting {
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("⚡ Injecting keystrokes..."))
	}

	return CardBox.Width(76).Render(strings.Join(rows, "\n"))
}

func (m Model) renderTelemetryView() string {
	var rows []string

	header := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("LINK TELEMETRY & SYSTEM HEALTH")
	rows = append(rows, header, "")

	stats := m.trans.Stats()

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Transport Type:"), StatValue.Render(m.trans.Type())),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Target Port:"), StatValue.Render(m.trans.Endpoint())),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Wireless Link:"), StatValue.Render("ESP-NOW Channel 1 (Broadcast)")),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Packets Sent:"), StatValue.Render(fmt.Sprintf("%d", stats.PacketsSent))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Payload Volume:"), StatValue.Render(fmt.Sprintf("%d bytes", stats.BytesSent))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Transport Latency:"), StatValue.Render(stats.Latency.String())),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Socket/IO Errors:"), StatValue.Render(fmt.Sprintf("%d", stats.Errors))),
		lipgloss.JoinHorizontal(lipgloss.Left, StatLabel.Render("Last Packet:"), StatValue.Render(stats.LastSent.Format("15:04:05.000"))),
		"",
		lipgloss.NewStyle().Foreground(ColorYellow).Render("Press [p] to send manual Heartbeat Ping"),
	)

	return CardBox.Width(76).Render(strings.Join(rows, "\n"))
}

func (m Model) renderActivityLog() string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorTextDim).Render("TRANSMISSION EVENT LOG:"))

	for _, log := range m.activityLog {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextDim).Render(log))
	}

	return CardBox.Width(76).Render(strings.Join(lines, "\n"))
}

func (m Model) renderFooter() string {
	if m.captureActive && m.activeTab == TabLiveCapture {
		return lipgloss.JoinHorizontal(lipgloss.Left,
			FooterKey.Render("[Esc] / [Ctrl+Q]"), FooterDesc.Render(" Pause Capture"),
			FooterKey.Render("[All Keys]"), FooterDesc.Render(" Transmitted to Remote Host"),
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Left,
		FooterKey.Render("[Tab]"), FooterDesc.Render(" Switch Mode"),
		FooterKey.Render("[Space]"), FooterDesc.Render(" Resume Capture"),
		FooterKey.Render("[1-5]"), FooterDesc.Render(" Quick Tab"),
		FooterKey.Render("[q]"), FooterDesc.Render(" Quit"),
	)
}

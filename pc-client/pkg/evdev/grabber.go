package evdev

import (
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
)

// Linux ioctl constants for evdev
const (
	// EVIOCGRAB = _IOW('E', 0x90, int) -> 0x40044590
	EVIOCGRAB = uintptr(0x40044590)

	// Linux input event types
	EvSyn = 0x00
	EvKey = 0x01
	EvRel = 0x02
	EvLed = 0x11

	// LED codes
	LedScrollL = 0x02 // Scroll Lock LED
)

// InputEvent matches Linux 64-bit struct input_event (24 bytes)
type InputEvent struct {
	TimeSec  uint64 // 8 bytes
	TimeUsec uint64 // 8 bytes
	Type     uint16 // 2 bytes
	Code     uint16 // 2 bytes
	Value    int32  // 4 bytes
}

// Device represents an opened /dev/input/event* device
type Device struct {
	Path       string
	Name       string
	IsKeyboard bool
	IsMouse    bool
	file       *os.File
	fd         int
	canWrite   bool
	grabbed    bool
}

// EventType categorizes Grabber events for UI display
type EventType int

const (
	EvtGrabToggled EventType = iota
	EvtKeyAction
	EvtMouseAction
	EvtError
	EvtLog
)

// GrabEvent carries live telemetry and state changes to the UI
type GrabEvent struct {
	Type         EventType
	Grabbed      bool
	KeyName      string
	KeyHex       string
	Modifiers    string
	MouseDX      int8
	MouseDY      int8
	MouseWheel   int8
	MouseButtons byte
	Message      string
	Time         time.Time
}

// Grabber manages physical evdev input interception and hotkey toggling
type Grabber struct {
	trans      transport.Transport
	devices    []*Device
	grabbed    bool
	mu         sync.RWMutex
	onEvent    func(GrabEvent)
	done       chan struct{}
	closeOnce  sync.Once

	// Active HID Keyboard State (6KRO)
	activeModifiers byte
	activeKeys      []byte // up to 6 keys

	// Active Mouse State
	activeMouseButtons byte
	mouseDX            int32
	mouseDY            int32
	mouseWheel         int32
	lastMouseFlush     time.Time
	lastUIMouseEvent   time.Time
	mouseMu            sync.Mutex
}

// DiscoverDevices scans /proc/bus/input/devices for physical keyboards and mice
func DiscoverDevices() ([]*Device, error) {
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/bus/input/devices: %w", err)
	}

	blocks := strings.Split(string(data), "\n\n")
	var devices []*Device

	nameRe := regexp.MustCompile(`N: Name="([^"]+)"`)
	handlersRe := regexp.MustCompile(`H: Handlers=(.+)`)

	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		nameMatch := nameRe.FindStringSubmatch(block)
		if len(nameMatch) < 2 {
			continue
		}
		name := nameMatch[1]
		nameLower := strings.ToLower(name)

		// Filter out virtual/system buttons and audio devices
		if strings.Contains(nameLower, "power button") ||
			strings.Contains(nameLower, "lid switch") ||
			strings.Contains(nameLower, "sleep button") ||
			strings.Contains(nameLower, "video bus") ||
			strings.Contains(nameLower, "wireless radio") ||
			strings.Contains(nameLower, "pc speaker") ||
			strings.Contains(nameLower, "headphone") ||
			strings.Contains(nameLower, "hdmi") ||
			strings.Contains(nameLower, "avrcp") ||
			strings.Contains(nameLower, "wmi hotkeys") {
			continue
		}

		// CRITICAL: Filter out Cardputer's own spoofed USB HID interface (Dell KB216 / 413c:2113)
		// to prevent catastrophic recursive input loopback when receiver is plugged into the same machine!
		if strings.Contains(strings.ToLower(block), "413c") ||
			strings.Contains(nameLower, "kb216") ||
			strings.Contains(nameLower, "cardputer") {
			continue
		}

		handlersMatch := handlersRe.FindStringSubmatch(block)
		if len(handlersMatch) < 2 {
			continue
		}
		handlers := strings.Fields(handlersMatch[1])

		var eventNode string
		var hasKbdHandler bool
		var hasMouseHandler bool

		for _, h := range handlers {
			if strings.HasPrefix(h, "event") {
				eventNode = h
			}
			if h == "kbd" {
				hasKbdHandler = true
			}
			if h == "mouse" || strings.HasPrefix(h, "mouse") {
				hasMouseHandler = true
			}
		}

		if eventNode == "" {
			continue
		}

		devPath := "/dev/input/" + eventNode

		isMouse := hasMouseHandler || (strings.Contains(nameLower, "touchpad") || (strings.Contains(nameLower, "mouse") && !strings.Contains(nameLower, "keyboard")))
		isKeyboard := hasKbdHandler || strings.Contains(nameLower, "keyboard")

		if !isMouse && !isKeyboard {
			continue
		}

		// Try opening device (read/write preferred for LED control, read-only fallback)
		file, err := os.OpenFile(devPath, os.O_RDWR, 0)
		canWrite := true
		if err != nil {
			file, err = os.OpenFile(devPath, os.O_RDONLY, 0)
			canWrite = false
			if err != nil {
				// Skip device if inaccessible
				continue
			}
		}

		devices = append(devices, &Device{
			Path:       devPath,
			Name:       name,
			IsKeyboard: isKeyboard,
			IsMouse:    isMouse,
			file:       file,
			fd:         int(file.Fd()),
			canWrite:   canWrite,
			grabbed:    false,
		})
	}

	return devices, nil
}

// NewGrabber creates a Grabber instance hooked to all discovered input devices
func NewGrabber(trans transport.Transport, onEvent func(GrabEvent)) (*Grabber, error) {
	devices, err := DiscoverDevices()
	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no usable keyboard or mouse input devices found in /dev/input/")
	}

	g := &Grabber{
		trans:          trans,
		devices:        devices,
		grabbed:        false,
		onEvent:        onEvent,
		done:           make(chan struct{}),
		lastMouseFlush: time.Now(),
	}

	return g, nil
}

// Start begins event listening loops on all devices
func (g *Grabber) Start() {
	for _, dev := range g.devices {
		go g.deviceReadLoop(dev)
	}
}

// ToggleGrab switches between Remote Hardware Grab and Local Pass-Through
func (g *Grabber) ToggleGrab() bool {
	g.mu.Lock()
	newGrab := !g.grabbed
	g.mu.Unlock()

	_ = g.SetGrab(newGrab)
	return newGrab
}

// SetGrab explicitly engages or releases the hardware grab
func (g *Grabber) SetGrab(grab bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.grabbed == grab {
		return nil
	}

	val := 0
	if grab {
		val = 1
	}

	for _, dev := range g.devices {
		err := unix.IoctlSetInt(dev.fd, uint(EVIOCGRAB), val)
		if err == nil {
			dev.grabbed = grab
		}

		// Toggle Scroll Lock LED on keyboards that support write
		if dev.IsKeyboard && dev.canWrite {
			g.setKeyboardLED(dev, LedScrollL, val)
		}
	}

	g.grabbed = grab

	if !grab {
		// When releasing grab back to local host:
		// Send release packets to remote host so no keys remain stuck
		_ = g.trans.SendRelease()
		_ = g.trans.SendMouse(0, 0, 0, 0)
		g.activeModifiers = 0
		g.activeKeys = nil
		g.activeMouseButtons = 0

		if g.onEvent != nil {
			g.onEvent(GrabEvent{
				Type:    EvtGrabToggled,
				Grabbed: false,
				Message: "KVM RELEASED: Keyboard & Mouse returned to Local Linux Desktop",
				Time:    time.Now(),
			})
		}
	} else {
		if g.onEvent != nil {
			g.onEvent(GrabEvent{
				Type:    EvtGrabToggled,
				Grabbed: true,
				Message: "KVM ENGAGED: Physical Input Exclusively Locked to Cardputer Target Host",
				Time:    time.Now(),
			})
		}
	}

	return nil
}

// IsGrabbed returns current hardware grab state
func (g *Grabber) IsGrabbed() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.grabbed
}

// Devices returns list of hooked devices
func (g *Grabber) Devices() []*Device {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.devices
}

// deviceReadLoop reads continuous 24-byte input_event structs from a device
func (g *Grabber) deviceReadLoop(dev *Device) {
	buf := make([]byte, 24)

	for {
		select {
		case <-g.done:
			return
		default:
		}

		n, err := dev.file.Read(buf)
		if err != nil {
			return
		}
		if n != 24 {
			continue
		}

		// Decode Linux struct input_event
		evt := InputEvent{
			TimeSec:  binary.LittleEndian.Uint64(buf[0:8]),
			TimeUsec: binary.LittleEndian.Uint64(buf[8:16]),
			Type:     binary.LittleEndian.Uint16(buf[16:18]),
			Code:     binary.LittleEndian.Uint16(buf[18:20]),
			Value:    int32(binary.LittleEndian.Uint32(buf[20:24])),
		}

		g.handleInputEvent(dev, evt)
	}
}

// handleInputEvent processes raw kernel input events
func (g *Grabber) handleInputEvent(dev *Device, evt InputEvent) {
	// 1. Check for Scroll Lock toggle key (code 70)
	if evt.Type == EvKey && evt.Code == EvKeyScrollLock {
		if evt.Value == 1 { // Key Press
			g.ToggleGrab()
		}
		// Scroll Lock is consumed by the KVM switch, not passed to target
		return
	}

	g.mu.RLock()
	isGrabbed := g.grabbed
	g.mu.RUnlock()

	// If not grabbed, events pass through locally; do not transmit
	if !isGrabbed {
		return
	}

	// 2. Grabbed Mode: Route events to Cardputer Transmitter
	switch evt.Type {
	case EvKey:
		g.handleKeyEvent(evt)

	case EvRel:
		g.handleRelEvent(evt)

	case EvSyn:
		g.handleSynEvent()
	}
}

// handleKeyEvent handles key presses, releases, and mouse button clicks
func (g *Grabber) handleKeyEvent(evt InputEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// A. Mouse Buttons
	switch evt.Code {
	case EvBtnLeft:
		if evt.Value == 1 {
			g.activeMouseButtons |= protocol.MouseBtnLeft
		} else if evt.Value == 0 {
			g.activeMouseButtons &= ^protocol.MouseBtnLeft
		}
		_ = g.trans.SendMouse(g.activeMouseButtons, 0, 0, 0)
		g.emitMouseEvent(0, 0, 0)
		return

	case EvBtnRight:
		if evt.Value == 1 {
			g.activeMouseButtons |= protocol.MouseBtnRight
		} else if evt.Value == 0 {
			g.activeMouseButtons &= ^protocol.MouseBtnRight
		}
		_ = g.trans.SendMouse(g.activeMouseButtons, 0, 0, 0)
		g.emitMouseEvent(0, 0, 0)
		return

	case EvBtnMiddle:
		if evt.Value == 1 {
			g.activeMouseButtons |= protocol.MouseBtnMiddle
		} else if evt.Value == 0 {
			g.activeMouseButtons &= ^protocol.MouseBtnMiddle
		}
		_ = g.trans.SendMouse(g.activeMouseButtons, 0, 0, 0)
		g.emitMouseEvent(0, 0, 0)
		return
	}

	// B. Keyboard Modifiers (Ctrl, Shift, Alt, Meta/Super)
	if modBit, isMod := LinuxToModifierMap[evt.Code]; isMod {
		if evt.Value == 1 {
			g.activeModifiers |= modBit
		} else if evt.Value == 0 {
			g.activeModifiers &= ^modBit
		}
		g.sendReportLocked()
		g.emitKeyEvent(LinuxEvdevKeyName(evt.Code), evt.Value == 1)
		return
	}

	// C. Standard Keyboard Keys (6KRO)
	hidCode, isKey := LinuxToHIDMap[evt.Code]
	if !isKey || hidCode == protocol.KeyNone {
		return
	}

	if evt.Value == 1 { // Key Down
		// Check if already active
		exists := false
		for _, k := range g.activeKeys {
			if k == hidCode {
				exists = true
				break
			}
		}
		if !exists {
			if len(g.activeKeys) < 6 {
				g.activeKeys = append(g.activeKeys, hidCode)
			} else {
				g.activeKeys[5] = hidCode
			}
		}
		g.sendReportLocked()
		g.emitKeyEvent(protocol.KeyName(hidCode), true)

	} else if evt.Value == 0 { // Key Up
		var updated []byte
		for _, k := range g.activeKeys {
			if k != hidCode {
				updated = append(updated, k)
			}
		}
		g.activeKeys = updated
		g.sendReportLocked()
		g.emitKeyEvent(protocol.KeyName(hidCode), false)
	}
	// Note: evt.Value == 2 (autorepeat) is handled natively by the target host USB HID driver
}

// sendReportLocked transmits the current 6KRO report to the transmitter
func (g *Grabber) sendReportLocked() {
	var keys [6]byte
	copy(keys[:], g.activeKeys)
	_ = g.trans.SendReport(g.activeModifiers, keys)
}

// emitKeyEvent dispatches key telemetry to the UI callback
func (g *Grabber) emitKeyEvent(name string, isDown bool) {
	if g.onEvent == nil {
		return
	}
	action := "Press"
	if !isDown {
		action = "Release"
	}
	g.onEvent(GrabEvent{
		Type:      EvtKeyAction,
		Grabbed:   true,
		KeyName:   name,
		KeyHex:    fmt.Sprintf("Mod: 0x%02X | Keys: %d", g.activeModifiers, len(g.activeKeys)),
		Modifiers: protocol.ModifiersName(g.activeModifiers),
		Message:   fmt.Sprintf("Key %s (%s)", name, action),
		Time:      time.Now(),
	})
}

// handleRelEvent buffers relative mouse displacement
func (g *Grabber) handleRelEvent(evt InputEvent) {
	g.mouseMu.Lock()
	defer g.mouseMu.Unlock()

	switch evt.Code {
	case EvRelX:
		g.mouseDX += evt.Value
	case EvRelY:
		g.mouseDY += evt.Value
	case EvRelWheel:
		g.mouseWheel += evt.Value
	}
}

// handleSynEvent is called on kernel EV_SYN; dispatches movement immediately with zero accumulator windup
func (g *Grabber) handleSynEvent() {
	g.mouseMu.Lock()
	if g.mouseDX == 0 && g.mouseDY == 0 && g.mouseWheel == 0 {
		g.mouseMu.Unlock()
		return
	}

	g.mu.RLock()
	buttons := g.activeMouseButtons
	isGrabbed := g.grabbed
	g.mu.RUnlock()

	if !isGrabbed {
		g.mouseDX = 0
		g.mouseDY = 0
		g.mouseWheel = 0
		g.mouseMu.Unlock()
		return
	}

	var reports [8][3]int8
	var count int

	for iter := 0; (g.mouseDX != 0 || g.mouseDY != 0 || g.mouseWheel != 0) && iter < 8; iter++ {
		dx := clampInt8(g.mouseDX)
		dy := clampInt8(g.mouseDY)
		wheel := clampInt8(g.mouseWheel)

		g.mouseDX -= int32(dx)
		g.mouseDY -= int32(dy)
		g.mouseWheel -= int32(wheel)

		reports[count] = [3]int8{dx, dy, wheel}
		count++
	}

	// Reset any remainder to guarantee zero drift or float
	g.mouseDX = 0
	g.mouseDY = 0
	g.mouseWheel = 0
	g.mouseMu.Unlock()

	for i := 0; i < count; i++ {
		_ = g.trans.SendMouse(buttons, reports[i][0], reports[i][1], reports[i][2])
		g.emitMouseEvent(reports[i][0], reports[i][1], reports[i][2])
	}
}

// emitMouseEvent dispatches mouse motion telemetry to the UI callback (rate-limited to 25Hz to prevent TUI stutter)
func (g *Grabber) emitMouseEvent(dx, dy, wheel int8) {
	if g.onEvent == nil {
		return
	}
	now := time.Now()
	if wheel == 0 && now.Sub(g.lastUIMouseEvent) < 40*time.Millisecond {
		return
	}
	g.lastUIMouseEvent = now
	g.onEvent(GrabEvent{
		Type:         EvtMouseAction,
		Grabbed:      true,
		MouseDX:      dx,
		MouseDY:      dy,
		MouseWheel:   wheel,
		MouseButtons: g.activeMouseButtons,
		Message:      fmt.Sprintf("Mouse: dX=%d dY=%d Wheel=%d Btn=0x%02X", dx, dy, wheel, g.activeMouseButtons),
		Time:         now,
	})
}

// setKeyboardLED writes an EV_LED event to illuminate the Scroll Lock LED
func (g *Grabber) setKeyboardLED(dev *Device, ledCode uint16, value int) {
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint16(buf[16:18], EvLed)
	binary.LittleEndian.PutUint16(buf[18:20], ledCode)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(value))
	_, _ = dev.file.Write(buf)
}

// Close releases all hardware grabs, resets LEDs, and closes all device files
func (g *Grabber) Close() error {
	g.closeOnce.Do(func() {
		close(g.done)

		// Release all grabs and turn off LEDs
		for _, dev := range g.devices {
			_ = unix.IoctlSetInt(dev.fd, uint(EVIOCGRAB), 0)
			if dev.IsKeyboard && dev.canWrite {
				g.setKeyboardLED(dev, LedScrollL, 0)
			}
			_ = dev.file.Close()
		}

		_ = g.trans.SendRelease()
		_ = g.trans.SendMouse(0, 0, 0, 0)
	})
	return nil
}

func clampInt8(v int32) int8 {
	if v > 127 {
		return 127
	}
	if v < -127 {
		return -127
	}
	return int8(v)
}

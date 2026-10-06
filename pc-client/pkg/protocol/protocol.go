package protocol

import (
	"encoding/binary"
	"fmt"
)

// Protocol constants matching kvm_protocol.h
const (
	Magic = uint16(0x4B56) // "KV"

	PktTypeKeyReport = uint8(0x01)
	PktTypeMouseMove = uint8(0x02)
	PktTypeHeartbeat = uint8(0x03)
	PktTypePingReq   = uint8(0x04)
	PktTypePingResp  = uint8(0x05)
)

// Keyboard Modifiers (Standard USB HID)
const (
	ModNone       = byte(0x00)
	ModLeftCtrl   = byte(0x01)
	ModLeftShift  = byte(0x02)
	ModLeftAlt    = byte(0x04)
	ModLeftGUI    = byte(0x08) // Windows / Super / Command key
	ModRightCtrl  = byte(0x10)
	ModRightShift = byte(0x20)
	ModRightAlt   = byte(0x40)
	ModRightGUI   = byte(0x80)
)

// HID Key Codes (Standard USB HID Keyboard Page 0x07)
const (
	KeyNone        = byte(0x00)
	KeyA           = byte(0x04)
	KeyB           = byte(0x05)
	KeyC           = byte(0x06)
	KeyD           = byte(0x07)
	KeyE           = byte(0x08)
	KeyF           = byte(0x09)
	KeyG           = byte(0x0A)
	KeyH           = byte(0x0B)
	KeyI           = byte(0x0C)
	KeyJ           = byte(0x0D)
	KeyK           = byte(0x0E)
	KeyL           = byte(0x0F)
	KeyM           = byte(0x10)
	KeyN           = byte(0x11)
	KeyO           = byte(0x12)
	KeyP           = byte(0x13)
	KeyQ           = byte(0x14)
	KeyR           = byte(0x15)
	KeyS           = byte(0x16)
	KeyT           = byte(0x17)
	KeyU           = byte(0x18)
	KeyV           = byte(0x19)
	KeyW           = byte(0x1A)
	KeyX           = byte(0x1B)
	KeyY           = byte(0x1C)
	KeyZ           = byte(0x1D)
	Key1           = byte(0x1E)
	Key2           = byte(0x1F)
	Key3           = byte(0x20)
	Key4           = byte(0x21)
	Key5           = byte(0x22)
	Key6           = byte(0x23)
	Key7           = byte(0x24)
	Key8           = byte(0x25)
	Key9           = byte(0x26)
	Key0           = byte(0x27)
	KeyEnter       = byte(0x28)
	KeyEscape      = byte(0x29)
	KeyBackspace   = byte(0x2A)
	KeyTab         = byte(0x2B)
	KeySpace       = byte(0x2C)
	KeyMinus       = byte(0x2D) // - and _
	KeyEqual       = byte(0x2E) // = and +
	KeyLeftBracket = byte(0x2F) // [ and {
	KeyRightBracket= byte(0x30) // ] and }
	KeyBackslash   = byte(0x31) // \ and |
	KeySemicolon   = byte(0x33) // ; and :
	KeyApostrophe  = byte(0x34) // ' and "
	KeyGrave       = byte(0x35) // ` and ~
	KeyComma       = byte(0x36) // , and <
	KeyPeriod      = byte(0x37) // . and >
	KeySlash       = byte(0x38) // / and ?
	KeyCapsLock    = byte(0x39)
	KeyF1          = byte(0x3A)
	KeyF2          = byte(0x3B)
	KeyF3          = byte(0x3C)
	KeyF4          = byte(0x3D)
	KeyF5          = byte(0x3E)
	KeyF6          = byte(0x3F)
	KeyF7          = byte(0x40)
	KeyF8          = byte(0x41)
	KeyF9          = byte(0x42)
	KeyF10         = byte(0x43)
	KeyF11         = byte(0x44)
	KeyF12         = byte(0x45)
	KeyPrintScreen = byte(0x46)
	KeyScrollLock  = byte(0x47)
	KeyPause       = byte(0x48)
	KeyInsert      = byte(0x49)
	KeyHome        = byte(0x4A)
	KeyPageUp      = byte(0x4B)
	KeyDelete      = byte(0x4C)
	KeyEnd         = byte(0x4D)
	KeyPageDown    = byte(0x4E)
	KeyRightArrow  = byte(0x4F)
	KeyLeftArrow   = byte(0x50)
	KeyDownArrow   = byte(0x51)
	KeyUpArrow     = byte(0x52)
	KeyNumLock     = byte(0x53)
	KeyKpSlash     = byte(0x54)
	KeyKpAsterisk  = byte(0x55)
	KeyKpMinus     = byte(0x56)
	KeyKpPlus      = byte(0x57)
	KeyKpEnter     = byte(0x58)
	KeyKp1         = byte(0x59)
	KeyKp2         = byte(0x5A)
	KeyKp3         = byte(0x5B)
	KeyKp4         = byte(0x5C)
	KeyKp5         = byte(0x5D)
	KeyKp6         = byte(0x5E)
	KeyKp7         = byte(0x5F)
	KeyKp8         = byte(0x60)
	KeyKp9         = byte(0x61)
	KeyKp0         = byte(0x62)
	KeyKpDot       = byte(0x63)
)

// KeyPacket represents the 12-byte wire format matching kvm_key_packet_t
type KeyPacket struct {
	Magic     uint16   // 0x4B56
	PktType   uint8    // 0x01
	Modifiers uint8    // Modifier bitmask
	Keycodes  [6]uint8 // Active HID keycodes
	SeqNum    uint16   // Sequence counter
}

// MousePacket represents the 9-byte wire format matching kvm_mouse_packet_t
type MousePacket struct {
	Magic   uint16 // 0x4B56
	PktType uint8  // 0x02
	Buttons uint8  // Bitmask: Bit 0=Left, Bit 1=Right, Bit 2=Middle
	DX      int8   // X displacement (-127 to 127)
	DY      int8   // Y displacement (-127 to 127)
	Wheel   int8   // Vertical scroll (-127 to 127)
	SeqNum  uint16
}

// Mouse Buttons
const (
	MouseBtnNone   = byte(0x00)
	MouseBtnLeft   = byte(0x01)
	MouseBtnRight  = byte(0x02)
	MouseBtnMiddle = byte(0x04)
)

// HeartbeatPacket represents the 9-byte wire format matching kvm_heartbeat_packet_t
type HeartbeatPacket struct {
	Magic    uint16 // 0x4B56
	PktType  uint8  // 0x03
	UptimeMs uint32 // Client uptime or timestamp
	SeqNum   uint16
}

// EncodeMousePacket serializes a relative mouse motion/click report into 9 bytes
func EncodeMousePacket(buttons byte, dx, dy, wheel int8, seq uint16) []byte {
	buf := make([]byte, 9)
	binary.LittleEndian.PutUint16(buf[0:2], Magic)
	buf[2] = PktTypeMouseMove
	buf[3] = buttons
	buf[4] = byte(dx)
	buf[5] = byte(dy)
	buf[6] = byte(wheel)
	binary.LittleEndian.PutUint16(buf[7:9], seq)
	return buf
}

// EncodeKeyPacket serializes a 6KRO keyboard report into 12 bytes
func EncodeKeyPacket(modifiers byte, keycodes [6]byte, seq uint16) []byte {
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint16(buf[0:2], Magic)
	buf[2] = PktTypeKeyReport
	buf[3] = modifiers
	copy(buf[4:10], keycodes[:])
	binary.LittleEndian.PutUint16(buf[10:12], seq)
	return buf
}

// EncodeSingleKey serializes a single key press report
func EncodeSingleKey(modifier byte, keycode byte, seq uint16) []byte {
	var keys [6]byte
	keys[0] = keycode
	return EncodeKeyPacket(modifier, keys, seq)
}

// EncodeRelease serializes an all-keys-up release report
func EncodeRelease(seq uint16) []byte {
	var keys [6]byte
	return EncodeKeyPacket(0, keys, seq)
}

// EncodeHeartbeat serializes a heartbeat ping packet
func EncodeHeartbeat(uptimeMs uint32, seq uint16) []byte {
	buf := make([]byte, 9)
	binary.LittleEndian.PutUint16(buf[0:2], Magic)
	buf[2] = PktTypeHeartbeat
	binary.LittleEndian.PutUint32(buf[3:7], uptimeMs)
	binary.LittleEndian.PutUint16(buf[7:9], seq)
	return buf
}

// PingReqPacket represents the 5-byte ping request packet (PC Client -> Transmitter)
type PingReqPacket struct {
	Magic   uint16 // 0x4B56
	PktType uint8  // 0x04
	SeqNum  uint16
}

// PingRespPacket represents the 27-byte identity response packet (Transmitter -> PC Client)
type PingRespPacket struct {
	Magic    uint16
	PktType  uint8
	MAC      [6]byte
	DeviceID string
	SeqNum   uint16
}

// EncodePingReq serializes a 5-byte ping request packet
func EncodePingReq(seq uint16) []byte {
	buf := make([]byte, 5)
	binary.LittleEndian.PutUint16(buf[0:2], Magic)
	buf[2] = PktTypePingReq
	binary.LittleEndian.PutUint16(buf[3:5], seq)
	return buf
}

// ParsePingResp attempts to decode a 27-byte ping response packet from raw bytes
func ParsePingResp(buf []byte) (*PingRespPacket, error) {
	if len(buf) < 27 {
		return nil, fmt.Errorf("buffer too short for ping response: got %d bytes, want 27", len(buf))
	}
	magic := binary.LittleEndian.Uint16(buf[0:2])
	if magic != Magic {
		return nil, fmt.Errorf("invalid magic: 0x%04X", magic)
	}
	if buf[2] != PktTypePingResp {
		return nil, fmt.Errorf("invalid packet type: 0x%02X", buf[2])
	}
	var resp PingRespPacket
	resp.Magic = magic
	resp.PktType = buf[2]
	copy(resp.MAC[:], buf[3:9])

	devIDBytes := buf[9:25]
	nullIdx := 0
	for nullIdx < len(devIDBytes) && devIDBytes[nullIdx] != 0 {
		nullIdx++
	}
	resp.DeviceID = string(devIDBytes[:nullIdx])
	resp.SeqNum = binary.LittleEndian.Uint16(buf[25:27])
	return &resp, nil
}

// CharToHID converts an ASCII character to a HID keycode and shift modifier
func CharToHID(c rune) (modifier byte, keycode byte) {
	if c >= 'a' && c <= 'z' {
		return ModNone, KeyA + byte(c-'a')
	}
	if c >= 'A' && c <= 'Z' {
		return ModLeftShift, KeyA + byte(c-'A')
	}
	if c >= '1' && c <= '9' {
		return ModNone, Key1 + byte(c-'1')
	}

	switch c {
	case '0':
		return ModNone, Key0
	case ' ':
		return ModNone, KeySpace
	case '\n', '\r':
		return ModNone, KeyEnter
	case '\t':
		return ModNone, KeyTab
	case '\b':
		return ModNone, KeyBackspace

	// Shifted Number Row
	case '!':
		return ModLeftShift, Key1
	case '@':
		return ModLeftShift, Key2
	case '#':
		return ModLeftShift, Key3
	case '$':
		return ModLeftShift, Key4
	case '%':
		return ModLeftShift, Key5
	case '^':
		return ModLeftShift, Key6
	case '&':
		return ModLeftShift, Key7
	case '*':
		return ModLeftShift, Key8
	case '(':
		return ModLeftShift, Key9
	case ')':
		return ModLeftShift, Key0

	// Symbols
	case '-':
		return ModNone, KeyMinus
	case '_':
		return ModLeftShift, KeyMinus
	case '=':
		return ModNone, KeyEqual
	case '+':
		return ModLeftShift, KeyEqual
	case '[':
		return ModNone, KeyLeftBracket
	case '{':
		return ModLeftShift, KeyLeftBracket
	case ']':
		return ModNone, KeyRightBracket
	case '}':
		return ModLeftShift, KeyRightBracket
	case '\\':
		return ModNone, KeyBackslash
	case '|':
		return ModLeftShift, KeyBackslash
	case ';':
		return ModNone, KeySemicolon
	case ':':
		return ModLeftShift, KeySemicolon
	case '\'':
		return ModNone, KeyApostrophe
	case '"':
		return ModLeftShift, KeyApostrophe
	case '`':
		return ModNone, KeyGrave
	case '~':
		return ModLeftShift, KeyGrave
	case ',':
		return ModNone, KeyComma
	case '<':
		return ModLeftShift, KeyComma
	case '.':
		return ModNone, KeyPeriod
	case '>':
		return ModLeftShift, KeyPeriod
	case '/':
		return ModNone, KeySlash
	case '?':
		return ModLeftShift, KeySlash
	}

	return ModNone, KeyNone
}

// KeyName returns a human-readable representation of a keycode
func KeyName(keycode byte) string {
	if keycode >= KeyA && keycode <= KeyZ {
		return string('A' + (keycode - KeyA))
	}
	if keycode >= Key1 && keycode <= Key9 {
		return string('1' + (keycode - Key1))
	}
	switch keycode {
	case Key0:
		return "0"
	case KeyEnter:
		return "Enter"
	case KeyEscape:
		return "Esc"
	case KeyBackspace:
		return "Backspace"
	case KeyTab:
		return "Tab"
	case KeySpace:
		return "Space"
	case KeyMinus:
		return "-"
	case KeyEqual:
		return "="
	case KeyLeftBracket:
		return "["
	case KeyRightBracket:
		return "]"
	case KeyBackslash:
		return "\\"
	case KeySemicolon:
		return ";"
	case KeyApostrophe:
		return "'"
	case KeyGrave:
		return "`"
	case KeyComma:
		return ","
	case KeyPeriod:
		return "."
	case KeySlash:
		return "/"
	case KeyCapsLock:
		return "Caps Lock"
	case KeyF1:
		return "F1"
	case KeyF2:
		return "F2"
	case KeyF3:
		return "F3"
	case KeyF4:
		return "F4"
	case KeyF5:
		return "F5"
	case KeyF6:
		return "F6"
	case KeyF7:
		return "F7"
	case KeyF8:
		return "F8"
	case KeyF9:
		return "F9"
	case KeyF10:
		return "F10"
	case KeyF11:
		return "F11"
	case KeyF12:
		return "F12"
	case KeyPrintScreen:
		return "PrtScn"
	case KeyScrollLock:
		return "ScrLk"
	case KeyPause:
		return "Pause"
	case KeyInsert:
		return "Insert"
	case KeyHome:
		return "Home"
	case KeyPageUp:
		return "PgUp"
	case KeyDelete:
		return "Delete"
	case KeyEnd:
		return "End"
	case KeyPageDown:
		return "PgDn"
	case KeyRightArrow:
		return "Right"
	case KeyLeftArrow:
		return "Left"
	case KeyDownArrow:
		return "Down"
	case KeyUpArrow:
		return "Up"
	case KeyNumLock:
		return "NumLock"
	case KeyKpSlash:
		return "Kp/"
	case KeyKpAsterisk:
		return "Kp*"
	case KeyKpMinus:
		return "Kp-"
	case KeyKpPlus:
		return "Kp+"
	case KeyKpEnter:
		return "KpEnter"
	case KeyKp1:
		return "Kp1"
	case KeyKp2:
		return "Kp2"
	case KeyKp3:
		return "Kp3"
	case KeyKp4:
		return "Kp4"
	case KeyKp5:
		return "Kp5"
	case KeyKp6:
		return "Kp6"
	case KeyKp7:
		return "Kp7"
	case KeyKp8:
		return "Kp8"
	case KeyKp9:
		return "Kp9"
	case KeyKp0:
		return "Kp0"
	case KeyKpDot:
		return "Kp."
	default:
		return "None"
	}
}

// ModifiersName returns a human-readable string for active modifiers
func ModifiersName(mods byte) string {
	var parts []string
	if mods&ModLeftCtrl != 0 || mods&ModRightCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ModLeftAlt != 0 || mods&ModRightAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ModLeftShift != 0 || mods&ModRightShift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ModLeftGUI != 0 || mods&ModRightGUI != 0 {
		parts = append(parts, "Super")
	}
	if len(parts) == 0 {
		return ""
	}
	res := parts[0]
	for _, p := range parts[1:] {
		res += "+" + p
	}
	return res
}

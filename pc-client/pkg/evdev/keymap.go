package evdev

import (
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
)

// Linux evdev Key Codes (from linux/input-event-codes.h)
const (
	EvKeyEsc        = 1
	EvKey1          = 2
	EvKey2          = 3
	EvKey3          = 4
	EvKey4          = 5
	EvKey5          = 6
	EvKey6          = 7
	EvKey7          = 8
	EvKey8          = 9
	EvKey9          = 10
	EvKey0          = 11
	EvKeyMinus      = 12
	EvKeyEqual      = 13
	EvKeyBackspace  = 14
	EvKeyTab        = 15
	EvKeyQ          = 16
	EvKeyW          = 17
	EvKeyE          = 18
	EvKeyR          = 19
	EvKeyT          = 20
	EvKeyY          = 21
	EvKeyU          = 22
	EvKeyI          = 23
	EvKeyO          = 24
	EvKeyP          = 25
	EvKeyLeftBrace  = 26
	EvKeyRightBrace = 27
	EvKeyEnter      = 28
	EvKeyLeftCtrl   = 29
	EvKeyA          = 30
	EvKeyS          = 31
	EvKeyD          = 32
	EvKeyF          = 33
	EvKeyG          = 34
	EvKeyH          = 35
	EvKeyJ          = 36
	EvKeyK          = 37
	EvKeyL          = 38
	EvKeySemicolon  = 39
	EvKeyApostrophe = 40
	EvKeyGrave      = 41
	EvKeyLeftShift  = 42
	EvKeyBackslash  = 43
	EvKeyZ          = 44
	EvKeyX          = 45
	EvKeyC          = 46
	EvKeyV          = 47
	EvKeyB          = 48
	EvKeyN          = 49
	EvKeyM          = 50
	EvKeyComma      = 51
	EvKeyDot        = 52
	EvKeySlash      = 53
	EvKeyRightShift = 54
	EvKeyKpAsterisk = 55
	EvKeyLeftAlt    = 56
	EvKeySpace      = 57
	EvKeyCapsLock   = 58
	EvKeyF1         = 59
	EvKeyF2         = 60
	EvKeyF3         = 61
	EvKeyF4         = 62
	EvKeyF5         = 63
	EvKeyF6         = 64
	EvKeyF7         = 65
	EvKeyF8         = 66
	EvKeyF9         = 67
	EvKeyF10        = 68
	EvKeyNumLock    = 69
	EvKeyScrollLock = 70 // Hardware KVM Toggle!
	EvKeyKp7        = 71
	EvKeyKp8        = 72
	EvKeyKp9        = 73
	EvKeyKpMinus    = 74
	EvKeyKp4        = 75
	EvKeyKp5        = 76
	EvKeyKp6        = 77
	EvKeyKpPlus     = 78
	EvKeyKp1        = 79
	EvKeyKp2        = 80
	EvKeyKp3        = 81
	EvKeyKp0        = 82
	EvKeyKpDot      = 83
	EvKeyF11        = 87
	EvKeyF12        = 88
	EvKeyKpEnter    = 96
	EvKeyRightCtrl  = 97
	EvKeyKpSlash    = 98
	EvKeySysRq      = 99
	EvKeyRightAlt   = 100
	EvKeyHome       = 102
	EvKeyUp         = 103
	EvKeyPageUp     = 104
	EvKeyLeft       = 105
	EvKeyRight      = 106
	EvKeyEnd        = 107
	EvKeyDown       = 108
	EvKeyPageDown   = 109
	EvKeyInsert     = 110
	EvKeyDelete     = 111
	EvKeyPause      = 119
	EvKeyLeftMeta   = 125 // Super / Windows
	EvKeyRightMeta  = 126
	EvKeyCompose    = 127

	// Mouse Buttons
	EvBtnLeft   = 0x110
	EvBtnRight  = 0x111
	EvBtnMiddle = 0x112
	EvBtnSide   = 0x113
	EvBtnExtra  = 0x114

	// Relative Axes
	EvRelX     = 0x00
	EvRelY     = 0x01
	EvRelWheel = 0x08
)

// LinuxToHIDMap maps Linux event code -> USB HID Key Code
var LinuxToHIDMap = map[uint16]byte{
	EvKeyEsc:        protocol.KeyEscape,
	EvKey1:          protocol.Key1,
	EvKey2:          protocol.Key2,
	EvKey3:          protocol.Key3,
	EvKey4:          protocol.Key4,
	EvKey5:          protocol.Key5,
	EvKey6:          protocol.Key6,
	EvKey7:          protocol.Key7,
	EvKey8:          protocol.Key8,
	EvKey9:          protocol.Key9,
	EvKey0:          protocol.Key0,
	EvKeyMinus:      protocol.KeyMinus,
	EvKeyEqual:      protocol.KeyEqual,
	EvKeyBackspace:  protocol.KeyBackspace,
	EvKeyTab:        protocol.KeyTab,
	EvKeyQ:          protocol.KeyQ,
	EvKeyW:          protocol.KeyW,
	EvKeyE:          protocol.KeyE,
	EvKeyR:          protocol.KeyR,
	EvKeyT:          protocol.KeyT,
	EvKeyY:          protocol.KeyY,
	EvKeyU:          protocol.KeyU,
	EvKeyI:          protocol.KeyI,
	EvKeyO:          protocol.KeyO,
	EvKeyP:          protocol.KeyP,
	EvKeyLeftBrace:  protocol.KeyLeftBracket,
	EvKeyRightBrace: protocol.KeyRightBracket,
	EvKeyEnter:      protocol.KeyEnter,
	EvKeyA:          protocol.KeyA,
	EvKeyS:          protocol.KeyS,
	EvKeyD:          protocol.KeyD,
	EvKeyF:          protocol.KeyF,
	EvKeyG:          protocol.KeyG,
	EvKeyH:          protocol.KeyH,
	EvKeyJ:          protocol.KeyJ,
	EvKeyK:          protocol.KeyK,
	EvKeyL:          protocol.KeyL,
	EvKeySemicolon:  protocol.KeySemicolon,
	EvKeyApostrophe: protocol.KeyApostrophe,
	EvKeyGrave:      protocol.KeyGrave,
	EvKeyBackslash:  protocol.KeyBackslash,
	EvKeyZ:          protocol.KeyZ,
	EvKeyX:          protocol.KeyX,
	EvKeyC:          protocol.KeyC,
	EvKeyV:          protocol.KeyV,
	EvKeyB:          protocol.KeyB,
	EvKeyN:          protocol.KeyN,
	EvKeyM:          protocol.KeyM,
	EvKeyComma:      protocol.KeyComma,
	EvKeyDot:        protocol.KeyPeriod,
	EvKeySlash:      protocol.KeySlash,
	EvKeySpace:      protocol.KeySpace,
	EvKeyCapsLock:   protocol.KeyCapsLock,
	EvKeyF1:         protocol.KeyF1,
	EvKeyF2:         protocol.KeyF2,
	EvKeyF3:         protocol.KeyF3,
	EvKeyF4:         protocol.KeyF4,
	EvKeyF5:         protocol.KeyF5,
	EvKeyF6:         protocol.KeyF6,
	EvKeyF7:         protocol.KeyF7,
	EvKeyF8:         protocol.KeyF8,
	EvKeyF9:         protocol.KeyF9,
	EvKeyF10:        protocol.KeyF10,
	EvKeyF11:        protocol.KeyF11,
	EvKeyF12:        protocol.KeyF12,
	EvKeySysRq:      protocol.KeyPrintScreen,
	EvKeyHome:       protocol.KeyHome,
	EvKeyUp:         protocol.KeyUpArrow,
	EvKeyPageUp:     protocol.KeyPageUp,
	EvKeyLeft:       protocol.KeyLeftArrow,
	EvKeyRight:      protocol.KeyRightArrow,
	EvKeyEnd:        protocol.KeyEnd,
	EvKeyDown:       protocol.KeyDownArrow,
	EvKeyPageDown:   protocol.KeyPageDown,
	EvKeyInsert:     protocol.KeyInsert,
	EvKeyDelete:     protocol.KeyDelete,
	EvKeyPause:      protocol.KeyPause,
	EvKeyNumLock:    protocol.KeyNumLock,
	EvKeyKpSlash:    protocol.KeyKpSlash,
	EvKeyKpAsterisk: protocol.KeyKpAsterisk,
	EvKeyKpMinus:    protocol.KeyKpMinus,
	EvKeyKpPlus:     protocol.KeyKpPlus,
	EvKeyKpEnter:    protocol.KeyKpEnter,
	EvKeyKp1:        protocol.KeyKp1,
	EvKeyKp2:        protocol.KeyKp2,
	EvKeyKp3:        protocol.KeyKp3,
	EvKeyKp4:        protocol.KeyKp4,
	EvKeyKp5:        protocol.KeyKp5,
	EvKeyKp6:        protocol.KeyKp6,
	EvKeyKp7:        protocol.KeyKp7,
	EvKeyKp8:        protocol.KeyKp8,
	EvKeyKp9:        protocol.KeyKp9,
	EvKeyKp0:        protocol.KeyKp0,
	EvKeyKpDot:      protocol.KeyKpDot,
}

// LinuxToModifierMap checks if an evdev code is a modifier
var LinuxToModifierMap = map[uint16]byte{
	EvKeyLeftCtrl:   protocol.ModLeftCtrl,
	EvKeyLeftShift:  protocol.ModLeftShift,
	EvKeyLeftAlt:    protocol.ModLeftAlt,
	EvKeyLeftMeta:   protocol.ModLeftGUI,
	EvKeyRightCtrl:  protocol.ModRightCtrl,
	EvKeyRightShift: protocol.ModRightShift,
	EvKeyRightAlt:   protocol.ModRightAlt,
	EvKeyRightMeta:  protocol.ModRightGUI,
}

// LinuxEvdevKeyName returns human readable key description
func LinuxEvdevKeyName(code uint16) string {
	switch code {
	case EvKeyScrollLock:
		return "ScrollLock (TOGGLE)"
	case EvKeyLeftCtrl:
		return "LeftCtrl"
	case EvKeyRightCtrl:
		return "RightCtrl"
	case EvKeyLeftShift:
		return "LeftShift"
	case EvKeyRightShift:
		return "RightShift"
	case EvKeyLeftAlt:
		return "LeftAlt"
	case EvKeyRightAlt:
		return "RightAlt"
	case EvKeyLeftMeta:
		return "Super/Win"
	case EvKeyRightMeta:
		return "RightSuper"
	case EvKeyEnter:
		return "Enter"
	case EvKeySpace:
		return "Space"
	case EvKeyBackspace:
		return "Backspace"
	case EvKeyTab:
		return "Tab"
	case EvKeyEsc:
		return "Esc"
	case EvBtnLeft:
		return "Mouse LeftBtn"
	case EvBtnRight:
		return "Mouse RightBtn"
	case EvBtnMiddle:
		return "Mouse MidBtn"
	default:
		if hid, ok := LinuxToHIDMap[code]; ok {
			return protocol.KeyName(hid)
		}
		return "Key"
	}
}

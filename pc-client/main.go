package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/ui"
)

func main() {
	// Flags
	serialPort := flag.String("serial", "", "Serial port for ESP32-S3 transmitter dongle (e.g. /dev/ttyUSB0, auto-detected if empty)")
	baudRate := flag.Int("baud", 115200, "Serial baud rate")

	injectText := flag.String("text", "", "CLI one-shot: inject text string to target PC without TUI")
	injectMacro := flag.String("macro", "", "CLI one-shot: execute macro ('cad' for Ctrl+Alt+Del, 'lock' for Win+L, etc.)")
	charDelayMs := flag.Int("delay", 15, "Delay between keystrokes in ms (for text injection)")

	flag.Parse()

	// Connect to ESP32-S3 transmitter dongle via USB cable
	trans, err := transport.NewSerialTransport(*serialPort, *baudRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to ESP32-S3 transmitter dongle: %v\n", err)
		fmt.Fprintf(os.Stderr, "Tip: Ensure your ESP32-S3 is plugged into USB. Use -serial <device>\n")
		os.Exit(1)
	}
	defer trans.Close()

	// 1. One-shot text injection CLI mode
	if *injectText != "" {
		fmt.Printf("Injecting string via %s (%s)...\n", trans.Type(), trans.Endpoint())
		delay := time.Duration(*charDelayMs) * time.Millisecond
		if err := trans.SendString(*injectText, delay); err != nil {
			fmt.Fprintf(os.Stderr, "Injection failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Success! All keystrokes transmitted.")
		return
	}

	// 2. One-shot macro CLI mode
	if *injectMacro != "" {
		switch *injectMacro {
		case "cad", "ctrl-alt-del":
			fmt.Printf("Injecting Ctrl+Alt+Del via %s...\n", trans.Type())
			_ = trans.SendKey(protocol.ModLeftCtrl|protocol.ModLeftAlt, protocol.KeyDelete)
		case "lock", "win-l":
			fmt.Printf("Injecting Win+L via %s...\n", trans.Type())
			_ = trans.SendKey(protocol.ModLeftGUI, protocol.KeyL)
		case "alttab", "alt-tab":
			fmt.Printf("Injecting Alt+Tab via %s...\n", trans.Type())
			_ = trans.SendKey(protocol.ModLeftAlt, protocol.KeyTab)
		default:
			fmt.Fprintf(os.Stderr, "Unknown macro '%s'. Available: 'cad', 'lock', 'alttab'\n", *injectMacro)
			os.Exit(1)
		}
		return
	}

	// 3. Full Interactive Bubbletea TUI Mode
	m := ui.NewModel(trans)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

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
	targetIP := flag.String("target", "192.168.4.1", "Cardputer receiver IP address")
	targetPort := flag.Int("port", 8888, "Cardputer receiver UDP port")
	injectText := flag.String("text", "", "CLI one-shot: inject text string to target PC without TUI")
	injectMacro := flag.String("macro", "", "CLI one-shot: execute macro ('cad' for Ctrl+Alt+Del, 'lock' for Win+L, etc.)")
	charDelayMs := flag.Int("delay", 15, "Delay between keystrokes in ms (for text injection)")

	flag.Parse()

	endpoint := fmt.Sprintf("%s:%d", *targetIP, *targetPort)

	// Initialize UDP Transport
	trans, err := transport.NewUDPTransport(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing UDP transport to %s: %v\n", endpoint, err)
		os.Exit(1)
	}
	defer trans.Close()

	// 1. One-shot text injection CLI mode
	if *injectText != "" {
		fmt.Printf("Injecting string into host via %s (%d chars)...\n", endpoint, len(*injectText))
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
			fmt.Printf("Injecting Ctrl+Alt+Del via %s...\n", endpoint)
			_ = trans.SendKey(protocol.ModLeftCtrl|protocol.ModLeftAlt, protocol.KeyDelete)
		case "lock", "win-l":
			fmt.Printf("Injecting Win+L via %s...\n", endpoint)
			_ = trans.SendKey(protocol.ModLeftGUI, protocol.KeyL)
		case "alttab", "alt-tab":
			fmt.Printf("Injecting Alt+Tab via %s...\n", endpoint)
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

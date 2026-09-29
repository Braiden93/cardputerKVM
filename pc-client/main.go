package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/evdev"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/ui"
)

func main() {
	// Flags
	serialPort := flag.String("serial", "", "Serial port for ESP32-S3 transmitter dongle (e.g. /dev/ttyUSB0, auto-detected if empty)")
	baudRate := flag.Int("baud", 921600, "Serial baud rate (default 921600 for 1000Hz polling)")
	startGrabbed := flag.Bool("grab", false, "Start immediately in hardware grab mode (locks physical input to target PC)")

	injectText := flag.String("text", "", "CLI one-shot: inject text string to target PC without TUI")
	injectMacro := flag.String("macro", "", "CLI one-shot: execute macro ('cad' for Ctrl+Alt+Del, 'lock' for Win+L, etc.)")
	pasteClipboard := flag.Bool("paste", false, "CLI one-shot: paste host clipboard to target PC at high speed (~125 chars/sec)")
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

	// 0. One-shot clipboard paste CLI mode
	if *pasteClipboard {
		text, err := evdev.ReadHostClipboard()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read host clipboard: %v\n", err)
			os.Exit(1)
		}
		runes := []rune(text)
		fmt.Printf("Blasting %d characters from host clipboard via %s (~125 chars/sec)...\n", len(runes), trans.Type())
		if err := trans.SendStringFast(text, nil, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Paste failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Success! Pasted %d characters.\n", len(runes))
		return
	}

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

	// 3. Hardware KVM Grabber Mode
	var (
		p      *tea.Program
		progMu sync.Mutex
	)

	grabber, err := evdev.NewGrabber(trans, func(evt evdev.GrabEvent) {
		progMu.Lock()
		defer progMu.Unlock()
		if p != nil {
			p.Send(ui.GrabMsg(evt))
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing physical input grabber: %v\n", err)
		fmt.Fprintf(os.Stderr, "Tip: Run with 'sudo ./cardputer-kvm' to grant access to /dev/input/event* devices.\n")
		os.Exit(1)
	}
	defer grabber.Close()

	// Start continuous background reader loops on physical devices
	grabber.Start()

	if *startGrabbed {
		_ = grabber.SetGrab(true)
	}

	// Trap OS termination signals to ensure input is always ungrabbed on exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		_ = grabber.Close()
		progMu.Lock()
		if p != nil {
			p.Quit()
		}
		progMu.Unlock()
		os.Exit(0)
	}()

	// 4. Run Interactive Bubbletea Hardware KVM HUD
	m := ui.NewModel(trans, grabber)
	progMu.Lock()
	p = tea.NewProgram(m, tea.WithAltScreen())
	progMu.Unlock()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

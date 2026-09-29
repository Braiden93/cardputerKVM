package transport

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/protocol"
)

// TransportStats tracks operational telemetry
type TransportStats struct {
	PacketsSent uint64
	BytesSent   uint64
	Errors      uint64
	LastSent    time.Time
	Latency     time.Duration
}

// Transport abstracts physical wire and wireless network communication
type Transport interface {
	Send(data []byte) error
	SendKey(modifier byte, keycode byte) error
	SendReport(modifier byte, keycodes [6]byte) error
	SendRelease() error
	SendMouse(buttons byte, dx, dy, wheel int8) error
	SendHeartbeat() error
	SendString(s string, charDelay time.Duration) error
	SendStringFast(text string, cancel <-chan struct{}, onProgress func(done, total int)) error
	Close() error
	IsConnected() bool
	Stats() TransportStats
	Endpoint() string
	Type() string
}

// =====================================================================
// Serial Dongle Transport (Primary Mode: PC -> ESP32-S3 via USB Cable)
// =====================================================================

type SerialTransport struct {
	portName  string
	baudRate  int
	port      serial.Port
	seq       uint16
	stats     TransportStats
	mu        sync.Mutex
	connected bool
}

// AutoDetectSerialPort scans for connected USB-to-UART or USB CDC devices
func AutoDetectSerialPort() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("failed to enumerate serial ports: %w", err)
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no serial devices found. Plug in the ESP32-S3 transmitter via USB")
	}

	// 1. Prefer /dev/ttyUSB* (CH340 / CP2102)
	for _, p := range ports {
		if strings.Contains(p, "ttyUSB") {
			return p, nil
		}
	}

	// 2. Prefer /dev/ttyACM* (Native ESP32-S3 USB CDC)
	for _, p := range ports {
		if strings.Contains(p, "ttyACM") {
			return p, nil
		}
	}

	// 3. Fallback to first port
	return ports[0], nil
}

// NewSerialTransport connects to the ESP32-S3 transmitter dongle via USB cable
func NewSerialTransport(portName string, baudRate int) (*SerialTransport, error) {
	if portName == "" {
		detected, err := AutoDetectSerialPort()
		if err != nil {
			return nil, err
		}
		portName = detected
	}

	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	return &SerialTransport{
		portName:  portName,
		baudRate:  baudRate,
		port:      p,
		connected: true,
	}, nil
}

func (s *SerialTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.port == nil || !s.connected {
		return fmt.Errorf("serial port %s not connected", s.portName)
	}

	start := time.Now()
	n, err := s.port.Write(data)
	if err != nil {
		s.stats.Errors++
		return err
	}

	s.stats.PacketsSent++
	s.stats.BytesSent += uint64(n)
	s.stats.LastSent = time.Now()
	s.stats.Latency = time.Since(start)
	return nil
}

func (s *SerialTransport) SendKey(modifier byte, keycode byte) error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	pkt := protocol.EncodeSingleKey(modifier, keycode, seq)
	if err := s.Send(pkt); err != nil {
		return err
	}

	time.Sleep(35 * time.Millisecond)
	return s.SendRelease()
}

func (s *SerialTransport) SendReport(modifier byte, keycodes [6]byte) error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	pkt := protocol.EncodeKeyPacket(modifier, keycodes, seq)
	return s.Send(pkt)
}

func (s *SerialTransport) SendRelease() error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	pkt := protocol.EncodeRelease(seq)
	return s.Send(pkt)
}

func (s *SerialTransport) SendMouse(buttons byte, dx, dy, wheel int8) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.port == nil || !s.connected {
		return fmt.Errorf("serial port %s not connected", s.portName)
	}

	s.seq++
	pkt := protocol.EncodeMousePacket(buttons, dx, dy, wheel, s.seq)
	n, err := s.port.Write(pkt)
	if err != nil {
		s.stats.Errors++
		return err
	}
	s.stats.PacketsSent++
	s.stats.BytesSent += uint64(n)
	return nil
}

func (s *SerialTransport) SendHeartbeat() error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	uptime := uint32(time.Now().UnixMilli())
	s.mu.Unlock()

	pkt := protocol.EncodeHeartbeat(uptime, seq)
	return s.Send(pkt)
}

func (s *SerialTransport) SendString(text string, charDelay time.Duration) error {
	for _, ch := range text {
		mod, code := protocol.CharToHID(ch)
		if code != protocol.KeyNone {
			if err := s.SendKey(mod, code); err != nil {
				return err
			}
			time.Sleep(charDelay)
		}
	}
	return nil
}

// SendStringFast injects text at maximum safe speed (~125 chars/sec, 1,500 WPM) without dropping keystrokes.
func (s *SerialTransport) SendStringFast(text string, cancel <-chan struct{}, onProgress func(done, total int)) error {
	runes := []rune(text)
	total := len(runes)
	done := 0

	// 1. Clear any held keys/modifiers before beginning
	_ = s.SendRelease()
	time.Sleep(10 * time.Millisecond)

	for _, ch := range runes {
		if cancel != nil {
			select {
			case <-cancel:
				_ = s.SendRelease()
				return fmt.Errorf("paste cancelled by user")
			default:
			}
		}

		mod, code := protocol.CharToHID(ch)
		if code != protocol.KeyNone {
			// Key Down (10ms hold)
			s.mu.Lock()
			s.seq++
			pkt := protocol.EncodeSingleKey(mod, code, s.seq)
			s.mu.Unlock()

			if err := s.Send(pkt); err != nil {
				return err
			}
			time.Sleep(10 * time.Millisecond)

			// Key Up / Release (10ms release)
			s.mu.Lock()
			s.seq++
			relPkt := protocol.EncodeRelease(s.seq)
			s.mu.Unlock()

			if err := s.Send(relPkt); err != nil {
				return err
			}

			// Allow terminal time to process newlines without dropping subsequent lines
			if code == protocol.KeyEnter {
				time.Sleep(25 * time.Millisecond)
			} else {
				time.Sleep(10 * time.Millisecond)
			}
		}

		done++
		if onProgress != nil {
			onProgress(done, total)
		}
	}

	_ = s.SendRelease()
	return nil
}

func (s *SerialTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	if s.port != nil {
		return s.port.Close()
	}
	return nil
}

func (s *SerialTransport) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

func (s *SerialTransport) Stats() TransportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *SerialTransport) Endpoint() string {
	return fmt.Sprintf("%s @ %d baud", s.portName, s.baudRate)
}

func (s *SerialTransport) Type() string {
	return "Serial Dongle"
}

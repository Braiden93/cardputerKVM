package transport

import (
	"encoding/binary"
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
	VerifyHandshake() (bool, string)
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

// VerifyTransmitterHandshake opens a serial port, sends KVM_PKT_PING_REQ, and checks for "KVM_TRANSMITTER" response
func VerifyTransmitterHandshake(portName string, baudRate int) (bool, string, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(portName, mode)
	if err != nil {
		return false, "", err
	}
	defer p.Close()

	_ = p.SetReadTimeout(30 * time.Millisecond)

	req := protocol.EncodePingReq(1)
	if _, err := p.Write(req); err != nil {
		return false, "", err
	}

	buf := make([]byte, 64)
	readBytes := 0
	deadline := time.Now().Add(60 * time.Millisecond)

	for readBytes < 27 && time.Now().Before(deadline) {
		n, err := p.Read(buf[readBytes:])
		if err != nil {
			break
		}
		readBytes += n
	}

	if readBytes < 27 {
		return false, "", fmt.Errorf("no response from %s", portName)
	}

	for i := 0; i <= readBytes-27; i++ {
		magic := binary.LittleEndian.Uint16(buf[i : i+2])
		if magic == protocol.Magic && buf[i+2] == protocol.PktTypePingResp {
			parsed, err := protocol.ParsePingResp(buf[i : i+27])
			if err == nil && parsed.DeviceID == "KVM_TRANSMITTER" {
				return true, parsed.DeviceID, nil
			}
		}
	}

	return false, "", fmt.Errorf("device on %s is not a Cardputer-KVM transmitter", portName)
}

// AutoDetectSerialPort scans for connected USB-to-UART or USB CDC devices and verifies hardware identity
func AutoDetectSerialPort() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("failed to enumerate serial ports: %w", err)
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no serial devices found. Plug in the ESP32-S3 transmitter via USB")
	}

	// 1. Scan for a verified Cardputer-KVM transmitter dongle with response
	for _, p := range ports {
		if verified, _, _ := VerifyTransmitterHandshake(p, 921600); verified {
			return p, nil
		}
	}

	// 2. Fallback: Prefer /dev/ttyUSB* (CH340 / CP2102)
	for _, p := range ports {
		if strings.Contains(p, "ttyUSB") {
			return p, nil
		}
	}

	// 3. Fallback: Prefer /dev/ttyACM* (Native ESP32-S3 USB CDC)
	for _, p := range ports {
		if strings.Contains(p, "ttyACM") {
			return p, nil
		}
	}

	// 4. Fallback to first port
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
		s.connected = false
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
		s.connected = false
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
	// Normalize line endings to avoid double-Enter on Windows CRLF text
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

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
		err := s.port.Close()
		s.port = nil
		return err
	}
	return nil
}

func (s *SerialTransport) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// VerifyHandshake sends a ping request over the open serial transport to verify transmitter identity
func (s *SerialTransport) VerifyHandshake() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.port == nil || !s.connected {
		return false, ""
	}

	_ = s.port.SetReadTimeout(30 * time.Millisecond)

	s.seq++
	req := protocol.EncodePingReq(s.seq)
	if _, err := s.port.Write(req); err != nil {
		s.connected = false
		return false, ""
	}

	buf := make([]byte, 64)
	readBytes := 0
	deadline := time.Now().Add(60 * time.Millisecond)

	for readBytes < 27 && time.Now().Before(deadline) {
		n, err := s.port.Read(buf[readBytes:])
		if err != nil {
			s.connected = false
			return false, ""
		}
		readBytes += n
	}

	if readBytes < 27 {
		s.connected = false
		return false, ""
	}

	for i := 0; i <= readBytes-27; i++ {
		magic := binary.LittleEndian.Uint16(buf[i : i+2])
		if magic == protocol.Magic && buf[i+2] == protocol.PktTypePingResp {
			parsed, err := protocol.ParsePingResp(buf[i : i+27])
			if err == nil && parsed.DeviceID == "KVM_TRANSMITTER" {
				return true, parsed.DeviceID
			}
		}
	}

	s.connected = false
	return false, ""
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

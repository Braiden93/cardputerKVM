package transport

import (
	"fmt"
	"net"
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
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	pkt := protocol.EncodeMousePacket(buttons, dx, dy, wheel, seq)
	return s.Send(pkt)
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

// =====================================================================
// UDP Transport (Secondary Mode: Direct Wi-Fi)
// =====================================================================

type UDPTransport struct {
	targetAddr string
	conn       *net.UDPConn
	raddr      *net.UDPAddr
	seq        uint16
	stats      TransportStats
	mu         sync.Mutex
	connected  bool
}

func NewUDPTransport(targetAddr string) (*UDPTransport, error) {
	raddr, err := net.ResolveUDPAddr("udp4", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP addr '%s': %w", targetAddr, err)
	}

	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("dial UDP: %w", err)
	}

	return &UDPTransport{
		targetAddr: targetAddr,
		conn:       conn,
		raddr:      raddr,
		connected:  true,
	}, nil
}

func (t *UDPTransport) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return fmt.Errorf("transport not connected")
	}

	start := time.Now()
	n, err := t.conn.Write(data)
	if err != nil {
		t.stats.Errors++
		return err
	}

	t.stats.PacketsSent++
	t.stats.BytesSent += uint64(n)
	t.stats.LastSent = time.Now()
	t.stats.Latency = time.Since(start)
	return nil
}

func (t *UDPTransport) SendKey(modifier byte, keycode byte) error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeSingleKey(modifier, keycode, seq)
	if err := t.Send(pkt); err != nil {
		return err
	}

	time.Sleep(35 * time.Millisecond)
	return t.SendRelease()
}

func (t *UDPTransport) SendReport(modifier byte, keycodes [6]byte) error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeKeyPacket(modifier, keycodes, seq)
	return t.Send(pkt)
}

func (t *UDPTransport) SendRelease() error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeRelease(seq)
	return t.Send(pkt)
}

func (t *UDPTransport) SendMouse(buttons byte, dx, dy, wheel int8) error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeMousePacket(buttons, dx, dy, wheel, seq)
	return t.Send(pkt)
}

func (t *UDPTransport) SendHeartbeat() error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	uptime := uint32(time.Now().UnixMilli())
	t.mu.Unlock()

	pkt := protocol.EncodeHeartbeat(uptime, seq)
	return t.Send(pkt)
}

func (t *UDPTransport) SendString(s string, charDelay time.Duration) error {
	for _, ch := range s {
		mod, code := protocol.CharToHID(ch)
		if code != protocol.KeyNone {
			if err := t.SendKey(mod, code); err != nil {
				return err
			}
			time.Sleep(charDelay)
		}
	}
	return nil
}

func (t *UDPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}

func (t *UDPTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

func (t *UDPTransport) Stats() TransportStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats
}

func (t *UDPTransport) Endpoint() string {
	return t.targetAddr
}

func (t *UDPTransport) Type() string {
	return "Wi-Fi (UDP)"
}

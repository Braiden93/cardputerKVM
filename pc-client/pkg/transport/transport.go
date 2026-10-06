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
	PacketsRecv uint64
	BytesRecv   uint64
	Errors      uint64
	LastSent    time.Time
	LastRecv    time.Time
	Latency     time.Duration
}

// Transport abstracts physical wire and wireless network communication
type Transport interface {
	Send(data []byte) error
	SendKey(modifier byte, keycode byte) error
	SendReport(modifier byte, keycodes [6]byte) error
	SendRelease() error
	SendMouse(buttons byte, dx, dy, wheel int8) error
	SendHeartbeat(isEngaged bool) error
	SetOnTelemetry(cb func(telem *protocol.TelemetryPacket, rtt time.Duration))
	LatestTelemetry() (*protocol.TelemetryPacket, time.Time)
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
	portName        string
	baudRate        int
	port            serial.Port
	seq             uint16
	stats           TransportStats
	mu              sync.Mutex
	connected       bool
	closeChan       chan struct{}

	heartbeatMu     sync.Mutex
	heartbeatTimes  map[uint16]time.Time

	pingRespChan    chan *protocol.PingRespPacket

	latestTelem     *protocol.TelemetryPacket
	latestTelemTime time.Time

	cbMu            sync.Mutex
	onTelemetry     func(telem *protocol.TelemetryPacket, rtt time.Duration)
}

// drainSerialPort reads and discards pending bytes in the serial RX buffer until idle
func drainSerialPort(p serial.Port, timeout time.Duration) {
	_ = p.SetReadTimeout(timeout)
	buf := make([]byte, 256)
	for {
		n, err := p.Read(buf)
		if err != nil || n == 0 {
			break
		}
	}
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

	// Drain any boot logs or residual noise before pinging
	drainSerialPort(p, 20*time.Millisecond)

	_ = p.SetReadTimeout(30 * time.Millisecond)

	req := protocol.EncodePingReq(1)
	if _, err := p.Write(req); err != nil {
		return false, "", err
	}

	buf := make([]byte, 512)
	readBytes := 0
	deadline := time.Now().Add(300 * time.Millisecond)

	for time.Now().Before(deadline) && readBytes < len(buf)-27 {
		n, err := p.Read(buf[readBytes:])
		if err != nil {
			break
		}
		if n > 0 {
			readBytes += n
			for i := 0; i <= readBytes-27; i++ {
				magic := binary.LittleEndian.Uint16(buf[i : i+2])
				if magic == protocol.Magic && buf[i+2] == protocol.PktTypePingResp {
					parsed, err := protocol.ParsePingResp(buf[i : i+27])
					if err == nil && parsed.DeviceID == "KVM_TRANSMITTER" {
						drainSerialPort(p, 10*time.Millisecond)
						return true, parsed.DeviceID, nil
					}
				}
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

	// 1. Sort candidate USB-to-serial ports first (ttyUSB*, ttyACM*, usbserial*, COM*)
	var usbPorts []string
	var otherPorts []string
	for _, p := range ports {
		pLower := strings.ToLower(p)
		if strings.Contains(pLower, "ttyusb") || strings.Contains(pLower, "ttyacm") ||
			strings.Contains(pLower, "usbserial") || strings.Contains(pLower, "com") {
			usbPorts = append(usbPorts, p)
		} else {
			otherPorts = append(otherPorts, p)
		}
	}

	candidatePorts := append(usbPorts, otherPorts...)

	// 2. Scan candidate ports for verified Cardputer-KVM transmitter handshake
	for _, p := range candidatePorts {
		// Skip motherboard legacy platform UARTs like /dev/ttyS0../dev/ttyS31
		if strings.Contains(p, "/dev/ttyS") && !strings.Contains(p, "/dev/ttyUSB") {
			continue
		}
		if verified, _, _ := VerifyTransmitterHandshake(p, 921600); verified {
			return p, nil
		}
	}

	// 3. Fallback: Prefer /dev/ttyUSB* (CH340 / CP2102)
	for _, p := range ports {
		if strings.Contains(p, "ttyUSB") {
			return p, nil
		}
	}

	// 4. Fallback: Prefer /dev/ttyACM* (Native ESP32-S3 USB CDC)
	for _, p := range ports {
		if strings.Contains(p, "ttyACM") {
			return p, nil
		}
	}

	// 5. Fallback to first USB port or first port
	if len(usbPorts) > 0 {
		return usbPorts[0], nil
	}
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

	s := &SerialTransport{
		portName:       portName,
		baudRate:       baudRate,
		port:           p,
		connected:      true,
		closeChan:      make(chan struct{}),
		heartbeatTimes: make(map[uint16]time.Time),
		pingRespChan:   make(chan *protocol.PingRespPacket, 4),
	}

	go s.readLoop()
	return s, nil
}

// readLoop continuously reads and demuxes incoming serial packets (ping responses and telemetry)
func (s *SerialTransport) readLoop() {
	buf := make([]byte, 1024)
	rxBuf := make([]byte, 0, 2048)

	for {
		s.mu.Lock()
		p := s.port
		conn := s.connected
		s.mu.Unlock()

		if p == nil || !conn {
			return
		}

		_ = p.SetReadTimeout(100 * time.Millisecond)
		n, err := p.Read(buf)
		if err != nil {
			s.mu.Lock()
			if s.connected {
				s.connected = false
				s.stats.Errors++
			}
			s.mu.Unlock()
			return
		}

		select {
		case <-s.closeChan:
			return
		default:
		}

		if n > 0 {
			s.mu.Lock()
			s.stats.BytesRecv += uint64(n)
			s.stats.LastRecv = time.Now()
			s.mu.Unlock()

			rxBuf = append(rxBuf, buf[:n]...)

			for len(rxBuf) >= 2 {
				magic := binary.LittleEndian.Uint16(rxBuf[0:2])
				if magic != protocol.Magic {
					drop := 1
					for drop+1 < len(rxBuf) {
						if binary.LittleEndian.Uint16(rxBuf[drop:drop+2]) == protocol.Magic {
							break
						}
						drop++
					}
					rxBuf = rxBuf[drop:]
					continue
				}

				if len(rxBuf) < 3 {
					break
				}

				pktType := rxBuf[2]
				var pktSize int
				switch pktType {
				case protocol.PktTypePingResp:
					pktSize = 27
				case protocol.PktTypeTelemetry:
					pktSize = 12
				default:
					rxBuf = rxBuf[1:]
					continue
				}

				if len(rxBuf) < pktSize {
					break
				}

				pktData := rxBuf[:pktSize]

				switch pktType {
				case protocol.PktTypePingResp:
					if resp, err := protocol.ParsePingResp(pktData); err == nil {
						s.mu.Lock()
						s.stats.PacketsRecv++
						s.mu.Unlock()
						select {
						case s.pingRespChan <- resp:
						default:
						}
					}
				case protocol.PktTypeTelemetry:
					if telem, err := protocol.ParseTelemetry(pktData); err == nil {
						now := time.Now()
						s.mu.Lock()
						s.stats.PacketsRecv++
						s.latestTelem = telem
						s.latestTelemTime = now
						s.mu.Unlock()

						var rtt time.Duration
						s.heartbeatMu.Lock()
						if sentTime, ok := s.heartbeatTimes[telem.EchoSeq]; ok {
							rtt = now.Sub(sentTime)
							delete(s.heartbeatTimes, telem.EchoSeq)
							for oldSeq, t := range s.heartbeatTimes {
								if now.Sub(t) > 5*time.Second {
									delete(s.heartbeatTimes, oldSeq)
								}
							}
						}
						s.heartbeatMu.Unlock()

						if rtt > 0 {
							s.mu.Lock()
							s.stats.Latency = rtt
							s.mu.Unlock()
						}

						s.cbMu.Lock()
						cb := s.onTelemetry
						s.cbMu.Unlock()
						if cb != nil {
							cb(telem, rtt)
						}
					}
				}

				rxBuf = rxBuf[pktSize:]
			}
		}
	}
}

func (s *SerialTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.port == nil || !s.connected {
		return fmt.Errorf("serial port %s not connected", s.portName)
	}

	n, err := s.port.Write(data)
	if err != nil {
		s.stats.Errors++
		s.connected = false
		return err
	}

	s.stats.PacketsSent++
	s.stats.BytesSent += uint64(n)
	s.stats.LastSent = time.Now()
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

func (s *SerialTransport) SendHeartbeat(isEngaged bool) error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	hostStatus := protocol.HostStatusConnected
	if isEngaged {
		hostStatus = protocol.HostStatusEngaged
	}

	now := time.Now()
	s.heartbeatMu.Lock()
	s.heartbeatTimes[seq] = now
	for oldSeq, t := range s.heartbeatTimes {
		if now.Sub(t) > 5*time.Second {
			delete(s.heartbeatTimes, oldSeq)
		}
	}
	s.heartbeatMu.Unlock()

	uptime := uint32(now.UnixMilli())
	pkt := protocol.EncodeHeartbeat(hostStatus, uptime, seq)
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
	if !s.connected {
		return nil
	}
	s.connected = false
	close(s.closeChan)
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
	if !s.IsConnected() {
		return false, ""
	}

	// Drain any residual ping responses from channel
	for {
		select {
		case <-s.pingRespChan:
		default:
			goto drained
		}
	}
drained:

	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	req := protocol.EncodePingReq(seq)
	if err := s.Send(req); err != nil {
		return false, ""
	}

	select {
	case resp := <-s.pingRespChan:
		if resp != nil && resp.DeviceID == "KVM_TRANSMITTER" {
			return true, resp.DeviceID
		}
	case <-time.After(250 * time.Millisecond):
	}

	return false, ""
}

func (s *SerialTransport) SetOnTelemetry(cb func(telem *protocol.TelemetryPacket, rtt time.Duration)) {
	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	s.onTelemetry = cb
}

func (s *SerialTransport) LatestTelemetry() (*protocol.TelemetryPacket, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestTelem, s.latestTelemTime
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

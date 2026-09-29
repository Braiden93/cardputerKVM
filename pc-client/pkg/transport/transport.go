package transport

import (
	"fmt"
	"net"
	"sync"
	"time"

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

// Transport abstracts network and hardware communication
type Transport interface {
	Send(data []byte) error
	SendKey(modifier byte, keycode byte) error
	SendReport(modifier byte, keycodes [6]byte) error
	SendRelease() error
	SendHeartbeat() error
	SendString(s string, charDelay time.Duration) error
	Close() error
	IsConnected() bool
	Stats() TransportStats
	Endpoint() string
}

// UDPTransport implements Transport over UDP to Cardputer-ADV
type UDPTransport struct {
	targetAddr string
	conn       *net.UDPConn
	raddr      *net.UDPAddr
	seq        uint16
	stats      TransportStats
	mu         sync.Mutex
	connected  bool
}

// NewUDPTransport creates a new UDP sender targeting targetAddr (e.g., "192.168.4.1:8888")
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

// Send transmits raw bytes over UDP
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

// SendKey transmits a single key press and schedules release
func (t *UDPTransport) SendKey(modifier byte, keycode byte) error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeSingleKey(modifier, keycode, seq)
	if err := t.Send(pkt); err != nil {
		return err
	}

	// Immediate or micro-delayed release
	time.Sleep(15 * time.Millisecond)
	return t.SendRelease()
}

// SendReport sends a multi-key 6KRO report (for key combinations or hold states)
func (t *UDPTransport) SendReport(modifier byte, keycodes [6]byte) error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeKeyPacket(modifier, keycodes, seq)
	return t.Send(pkt)
}

// SendRelease sends an all-keys-up report
func (t *UDPTransport) SendRelease() error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	t.mu.Unlock()

	pkt := protocol.EncodeRelease(seq)
	return t.Send(pkt)
}

// SendHeartbeat sends a ping packet to keep link active
func (t *UDPTransport) SendHeartbeat() error {
	t.mu.Lock()
	t.seq++
	seq := t.seq
	uptime := uint32(time.Now().UnixMilli())
	t.mu.Unlock()

	pkt := protocol.EncodeHeartbeat(uptime, seq)
	return t.Send(pkt)
}

// SendString streams a string character by character with configurable delay
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

// Close closes the UDP socket
func (t *UDPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}

// IsConnected checks if transport is active
func (t *UDPTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// Stats returns current statistics
func (t *UDPTransport) Stats() TransportStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats
}

// Endpoint returns the target address
func (t *UDPTransport) Endpoint() string {
	return t.targetAddr
}

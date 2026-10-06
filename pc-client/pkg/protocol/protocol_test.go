package protocol

import (
	"testing"
)

func TestHeartbeatPacketEncoding(t *testing.T) {
	seq := uint16(1234)
	uptime := uint32(56789)
	data := EncodeHeartbeat(HostStatusConnected, uptime, seq)

	if len(data) != 10 {
		t.Fatalf("expected 10 bytes, got %d", len(data))
	}
	if data[0] != 0x56 || data[1] != 0x4B {
		t.Errorf("magic mismatch")
	}
	if data[2] != PktTypeHeartbeat {
		t.Errorf("expected pkt_type 0x03, got 0x%02X", data[2])
	}
	if data[3] != HostStatusConnected {
		t.Errorf("expected host_status 0x01, got 0x%02X", data[3])
	}
}

func TestTelemetryPacketParsing(t *testing.T) {
	raw := []byte{
		0x56, 0x4B,       // Magic 0x4B56
		PktTypeTelemetry, // 0x06
		85,               // BatteryPct = 85%
		0x70, 0x0F,       // BatteryMv = 3952 mV (0x0F70)
		0xD4,             // RSSI = -44 dBm (int8: -44 is 0xD4)
		1,                // USBMounted = 1
		0x34, 0x12,       // EchoSeq = 0x1234 (4660)
		0x78, 0x56,       // SeqNum = 0x5678 (22136)
	}

	telem, err := ParseTelemetry(raw)
	if err != nil {
		t.Fatalf("failed to parse telemetry: %v", err)
	}

	if telem.BatteryPct != 85 {
		t.Errorf("expected BatteryPct 85, got %d", telem.BatteryPct)
	}
	if telem.BatteryMv != 3952 {
		t.Errorf("expected BatteryMv 3952, got %d", telem.BatteryMv)
	}
	if telem.RSSI != -44 {
		t.Errorf("expected RSSI -44, got %d", telem.RSSI)
	}
	if telem.USBMounted != 1 {
		t.Errorf("expected USBMounted 1, got %d", telem.USBMounted)
	}
	if telem.EchoSeq != 0x1234 {
		t.Errorf("expected EchoSeq 0x1234, got 0x%04X", telem.EchoSeq)
	}
	if telem.SeqNum != 0x5678 {
		t.Errorf("expected SeqNum 0x5678, got 0x%04X", telem.SeqNum)
	}
}

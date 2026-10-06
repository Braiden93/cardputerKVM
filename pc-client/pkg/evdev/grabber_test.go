package evdev

import (
	"testing"

	"github.com/ThunderStorm/Cardputer-KVM/pc-client/pkg/transport"
)

func TestDiscoverDevices(t *testing.T) {
	devs, err := DiscoverDevices()
	if err != nil {
		t.Fatalf("DiscoverDevices() returned error: %v", err)
	}

	t.Logf("Discovered %d input devices:", len(devs))
	for _, d := range devs {
		t.Logf("  Path: %-18s | Name: %-32s | Kbd: %-5v | Mouse: %-5v | Writable: %v",
			d.Path, d.Name, d.IsKeyboard, d.IsMouse, d.canWrite)
	}

	if len(devs) == 0 {
		t.Errorf("Expected at least 1 input device, got 0")
	}
}

func TestGrabberSafetyWhenDisconnected(t *testing.T) {
	// Grabber with nil transport
	g := &Grabber{
		trans:   nil,
		grabbed: false,
	}

	// 1. SetGrab(true) must be rejected
	err := g.SetGrab(true)
	if err == nil {
		t.Errorf("Expected SetGrab(true) to fail with nil transport, got nil error")
	}
	if g.IsGrabbed() {
		t.Errorf("Expected IsGrabbed() to be false, got true")
	}

	// 2. ToggleGrab() must return false
	toggled := g.ToggleGrab()
	if toggled {
		t.Errorf("Expected ToggleGrab() to return false when disconnected, got true")
	}
	if g.IsGrabbed() {
		t.Errorf("Expected IsGrabbed() to remain false, got true")
	}
}

func TestGrabberReleaseCycleNoDeadlock(t *testing.T) {
	mockTrans := &mockConnectedTransport{}
	var lastEvent GrabEvent
	g := &Grabber{
		trans:   mockTrans,
		grabbed: false,
		onEvent: func(evt GrabEvent) {
			lastEvent = evt
		},
	}

	// 1. Engage grab
	if err := g.SetGrab(true); err != nil {
		t.Fatalf("SetGrab(true) failed: %v", err)
	}
	if !g.IsGrabbed() {
		t.Errorf("Expected IsGrabbed() to be true")
	}
	if lastEvent.Type != EvtGrabToggled || !lastEvent.Grabbed {
		t.Errorf("Expected EvtGrabToggled with Grabbed=true")
	}

	// 2. Release grab (must not deadlock!)
	if err := g.SetGrab(false); err != nil {
		t.Fatalf("SetGrab(false) failed: %v", err)
	}
	if g.IsGrabbed() {
		t.Errorf("Expected IsGrabbed() to be false")
	}
	if lastEvent.Type != EvtGrabToggled || lastEvent.Grabbed {
		t.Errorf("Expected EvtGrabToggled with Grabbed=false")
	}

	// 3. Toggle cycles
	for i := 0; i < 5; i++ {
		toggled := g.ToggleGrab()
		expected := (i%2 == 0) // true, false, true, false, true
		if toggled != expected {
			t.Errorf("Iter %d: ToggleGrab() = %v, want %v", i, toggled, expected)
		}
	}
}

type mockConnectedTransport struct {
	transport.SerialTransport
}

func (m *mockConnectedTransport) IsConnected() bool                                 { return true }
func (m *mockConnectedTransport) SendRelease() error                                { return nil }
func (m *mockConnectedTransport) SendMouse(buttons byte, dx, dy, wheel int8) error { return nil }
func (m *mockConnectedTransport) SendReport(modifier byte, keycodes [6]byte) error  { return nil }



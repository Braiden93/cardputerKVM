package evdev

import (
	"testing"
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

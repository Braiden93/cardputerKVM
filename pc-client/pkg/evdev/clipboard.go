package evdev

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
)

// ReadHostClipboard reads text from the host Linux desktop clipboard.
// It transparently handles running as root/sudo (dropping privileges to the real user),
// and supports Wayland (wl-paste), X11 (xclip / xsel), and a staging file fallback.
func ReadHostClipboard() (string, error) {
	// Detect if running under sudo
	sudoUser := os.Getenv("SUDO_USER")
	sudoUID := os.Getenv("SUDO_UID")
	if sudoUID == "" {
		sudoUID = "1000"
	}

	// 1. Wayland Desktop (GNOME / KDE / Hyprland / Sway)
	// When running with sudo, root cannot talk to Wayland directly:
	// We execute wl-paste as the real desktop user ($SUDO_USER) with their XDG_RUNTIME_DIR.
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if waylandDisplay == "" {
		waylandDisplay = "wayland-0"
	}
	runtimeDir := fmt.Sprintf("/run/user/%s", sudoUID)

	var wlCmd *exec.Cmd
	if sudoUser != "" && os.Geteuid() == 0 {
		wlCmd = exec.Command("sudo", "-u", sudoUser, "env",
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			"wl-paste", "--no-newline")
	} else {
		wlCmd = exec.Command("wl-paste", "--no-newline")
	}

	var wlOut, wlErr bytes.Buffer
	wlCmd.Stdout = &wlOut
	wlCmd.Stderr = &wlErr
	if err := wlCmd.Run(); err == nil && wlOut.Len() > 0 {
		return wlOut.String(), nil
	}

	// 2. Python GTK4 / GDK Wayland fallback (pre-installed on GNOME & CachyOS)
	pyScript := `
import gi
try:
    gi.require_version('Gtk', '4.0')
    from gi.repository import Gtk, Gdk, GLib
    import sys
    Gtk.init()
    disp = Gdk.Display.get_default()
    if disp:
        cb = disp.get_clipboard()
        loop = GLib.MainLoop()
        def on_done(cb, res):
            try:
                t = cb.read_text_finish(res)
                if t: sys.stdout.write(t)
            except Exception: pass
            loop.quit()
        cb.read_text_async(None, on_done)
        GLib.timeout_add(1000, loop.quit)
        loop.run()
except Exception:
    pass
`
	var pyCmd *exec.Cmd
	if sudoUser != "" && os.Geteuid() == 0 {
		pyCmd = exec.Command("sudo", "-u", sudoUser, "env",
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			"python3", "-c", pyScript)
	} else {
		pyCmd = exec.Command("python3", "-c", pyScript)
	}

	var pyOut bytes.Buffer
	pyCmd.Stdout = &pyOut
	if err := pyCmd.Run(); err == nil && pyOut.Len() > 0 {
		return pyOut.String(), nil
	}

	// 3. X11 Desktop (xclip / xsel)
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	var xCmd *exec.Cmd
	if sudoUser != "" && os.Geteuid() == 0 {
		xCmd = exec.Command("sudo", "-u", sudoUser, "env",
			fmt.Sprintf("DISPLAY=%s", display),
			"xclip", "-selection", "clipboard", "-o")
	} else {
		xCmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	}

	var xOut bytes.Buffer
	xCmd.Stdout = &xOut
	if err := xCmd.Run(); err == nil && xOut.Len() > 0 {
		return xOut.String(), nil
	}

	// 3. Fallback: Go clipboard library
	if text, err := clipboard.ReadAll(); err == nil && len(strings.TrimSpace(text)) > 0 {
		return text, nil
	}

	// 4. Staging file fallback (useful for automated testing or if wl-clipboard is not yet installed)
	stagingPaths := []string{
		"/tmp/cardputer_clipboard.txt",
		fmt.Sprintf("/home/%s/.cardputer_clipboard.txt", sudoUser),
	}
	for _, p := range stagingPaths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return string(data), nil
		}
	}

	// Provide clear actionable guidance
	return "", fmt.Errorf("wl-paste not found or clipboard empty. Please run: 'sudo pacman -S wl-clipboard' (or echo text into /tmp/cardputer_clipboard.txt)")
}

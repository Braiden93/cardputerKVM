# Cardputer-KVM — PC Client (Hardware KVM Switch)

A high-performance, single-binary hardware KVM switch client written in **Go** using [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lipgloss](https://github.com/charmbracelet/lipgloss), and Linux kernel **`evdev`** (`/dev/input/event*`).

The client provides **true hardware-level input grabbing**: when you press **`Scroll Lock`**, it exclusively grabs your physical keyboard and mouse at the Linux kernel level via `EVIOCGRAB`. Your local desktop is locked out (preventing cursor drift or accidental typing), your keyboard's **Scroll Lock LED illuminates**, and all physical keystrokes and mouse movements are beamed via **USB Serial** to the **ESP32-S3 Transmitter Dongle** $\rightarrow$ **ESP-NOW (2.4 GHz RF)** $\rightarrow$ **M5Stack Cardputer-ADV Receiver** $\rightarrow$ **Target Machine USB HID**.

Press **`Scroll Lock`** again, and control instantly returns to your local Linux workstation.

---

## 1. System Architecture

```mermaid
graph LR
    subgraph Control Workstation (Linux)
        K[Physical Keyboard /dev/input] -->|evdev EVIOCGRAB| C[cardputer-kvm client]
        M[Physical Mouse /dev/input] -->|evdev EVIOCGRAB| C
        C -->|USB Serial @ 115200 baud| T[Transmitter: ESP32-S3 Dongle]
    end

    subgraph Wireless Link
        T -->|ESP-NOW 2.4 GHz RF / Channel 1| R[Receiver: M5Stack Cardputer-ADV]
    end

    subgraph Target Host Machine
        R -->|Native USB-C OTG| H[Target Host]
        H -.->|Detects 0x413C:0x2113| D[Dell KB216 Wired Keyboard + Mouse]
    end
```

- **Zero Network Setup:** Pure ESP-NOW Channel 1 RF link. No Wi-Fi routers, SoftAP, or IP addresses.
- **Hardware-Level Grabbing:** Direct Linux input subsystem interception (`/dev/input/event*`). The local window manager cannot steal key combos like `Alt+Tab`, `Super/Win`, or `Ctrl+Alt+Del`.
- **Keyboard LED Sync:** Automatically illuminates the physical `Scroll Lock` LED on your keyboard when KVM control is locked onto the target machine.

---

## 2. Hardware KVM Workflow

1. Plug the **ESP32-S3 Transmitter Dongle** into a USB port on your workstation.
2. Launch the client with `sudo`:
   ```bash
   sudo ./cardputer-kvm
   ```
3. The KVM switch HUD opens in **Local Desktop Pass-Through Mode**.
4. Press **`[Scroll Lock]`**:
   - The status bar turns **NEON GREEN**: `● [ TARGET HOST ENGAGED ]`.
   - Your keyboard's Scroll Lock LED lights up.
   - Physical keyboard and mouse events are exclusively routed to the Cardputer.
   - Local Linux desktop receives no input.
5. Press **`[Scroll Lock]`** again:
   - Control instantly returns to your local desktop.
   - The Scroll Lock LED turns off.

---

## 3. Compilation & Build

### Prerequisites
- **Go 1.21+** (`sudo pacman -S go` on Arch/CachyOS).

### Build Binary
```bash
cd pc-client
make        # produces compact stripped binary 'cardputer-kvm'
```

---

## 4. Usage & Options

### Launch with Root Permissions (Recommended for evdev grab)
```bash
sudo ./cardputer-kvm
```

*(Note: If your user is already in the `input` group, you can also run `./cardputer-kvm` without sudo).*

### Start Directly in Hardware Grab Mode
```bash
sudo ./cardputer-kvm -grab
```

### Specify a Custom Serial Port
```bash
sudo ./cardputer-kvm -serial /dev/ttyUSB0
```

### CLI One-Shot Automation (Without TUI)
Send a string directly:
```bash
./cardputer-kvm -text "sudo apt update && sudo apt upgrade -y\n"
```

Trigger `Ctrl+Alt+Del`:
```bash
./cardputer-kvm -macro cad
```

Trigger `Win+L`:
```bash
./cardputer-kvm -macro lock
```

---

## 5. Hotkeys & Controls

| Key | Function |
| :--- | :--- |
| **`[Scroll Lock]`** | **Toggle Hardware KVM Grab** (Switch between Remote Target and Local Desktop) |
| **`[Space]`** | Quick software toggle from terminal (when ungrabbed) |
| **`[p]`** | Dispatch manual heartbeat ping to verify transmitter link |
| **`[q]`** or **`[Ctrl+C]`** | Quit client (safely ungrabs all hardware before exit) |

---

## 6. Project Structure

```text
pc-client/
├── Makefile                   # Build, run, tidy helpers
├── go.mod                     # Go module definitions
├── go.sum                     # Checksums
├── main.go                    # Entry point, flag parser & signal trap
├── pkg/
│   ├── evdev/
│   │   ├── grabber.go         # Linux kernel EVIOCGRAB & 125Hz mouse coalescer
│   │   ├── grabber_test.go    # Device discovery & test suite
│   │   └── keymap.go          # Linux evdev keycodes -> USB HID scan code table
│   ├── protocol/
│   │   └── protocol.go        # Wire packet encoders (6KRO key report, mouse, heartbeat)
│   ├── transport/
│   │   └── transport.go       # High-speed UART0 serial bridge & port auto-detection
│   └── ui/
│       ├── styles.go          # Lipgloss cyberpunk theme & color tokens
│       └── ui.go              # Bubbletea Hardware KVM Switch HUD
└── README.md                  # This documentation
```

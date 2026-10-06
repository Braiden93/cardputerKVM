# Cardputer-KVM — PC Companion GUI (Hardware KVM Switch)

A modern, high-performance desktop Hardware KVM Switch application written in **Go** using **Fyne** and the Linux kernel **`evdev`** subsystem (`/dev/input/event*`).

The client provides **true hardware-level input grabbing**: when you press **`[Scroll Lock]`**, it exclusively grabs your physical keyboard and mouse at the Linux kernel level via `EVIOCGRAB`. Your local desktop is locked out (preventing cursor drift or accidental typing), your keyboard's **Scroll Lock LED illuminates**, and all physical keystrokes and 125Hz mouse movements are beamed via **High-Speed Serial (921,600 baud)** $\rightarrow$ **ESP32-S3 Transmitter Dongle** $\rightarrow$ **ESP-NOW (2.4 GHz RF)** $\rightarrow$ **M5Stack Cardputer-ADV Receiver** $\rightarrow$ **Target Machine USB HID**.

Press **`[Scroll Lock]`** again, and control instantly returns to your local workstation.

---

## 1. System Architecture

```mermaid
graph LR
    subgraph Control Workstation (Linux)
        K[Physical Keyboard /dev/input] -->|evdev EVIOCGRAB| C[cardputer-kvm GUI]
        M[Physical Mouse /dev/input] -->|evdev EVIOCGRAB| C
        C -->|USB Serial @ 921,600 baud| T[Transmitter: ESP32-S3 Dongle]
    end

    subgraph Wireless Link
        T <-->|ESP-NOW Duplex RF (Channel 1, 1-2ms RTT)| R[Receiver: M5Stack Cardputer-ADV]
    end

    subgraph Target Host Machine
        R -->|Native USB-C OTG| H[Target Host]
        H -.->|Detects 0x413C:0x2501| D[Dell Wireless Keyboard & Mouse Combo]
    end
```

* **Zero Network Setup:** Pure ESP-NOW Channel 1 RF link. No Wi-Fi routers, SoftAP, or IP addresses.
* **Hardware-Level Grabbing:** Direct Linux input subsystem interception (`/dev/input/event*`). The local window manager cannot steal key combos like `Alt+Tab`, `Super/Win`, or `Ctrl+Alt+Del`.
* **Keyboard LED Sync:** Automatically illuminates the physical `Scroll Lock` LED on your keyboard when KVM control is locked onto the target machine.
* **Bi-Directional Telemetry:** The Cardputer continuously streams live battery levels, RF RSSI, and target host USB mount status back to the PC client GUI.

---

## 2. Desktop GUI Features

* **Light & Dark Theme Toggle:**
  * Clean, modern interface supporting both Light Mode (default) and sleek Dark Mode.
  * Instant toggle via the header button (`☀️` / `🌙`) with persistent high-contrast color tokens.
* **Cardputer Battery Donut Gauge:**
  * Vector-drawn circular ring gauge displaying live Cardputer battery percentage (`0–100%`) and battery voltage in volts (e.g., `4.12 V`).
* **Target Host USB Status Badge:**
  * Real-time indicator displaying `🟢 Target USB Active` when mounted, or `⚪ Target USB Unplugged` when the cable is removed or target host is asleep.
* **Animated Heartbeat Pulse Dot:**
  * Beating heart dot synchronized with incoming RF telemetry ACKs from the Cardputer.
* **Live RTT Latency Display:**
  * True end-to-end Round-Trip Time measurement in milliseconds (e.g., `⚡ 1.2 ms`) calculated by echoing monotonic packet sequence numbers.
* **TX & RX Performance Metrics:**
  * Live counters tracking total transmitted and received packets and throughput in real-time.
* **Ultra-Slim Footer:**
  * Displays linked input devices and immediate status: `● KVM Engaged (Input routed to target) │ [Scroll Lock] to return`.
* **Zero-Flicker Locked Grid:**
  * Strict two-column locked grid layout preventing redraw jitter or container shifting during live telemetry updates.

---

## 3. Hardware KVM Workflow

1. Plug the **ESP32-S3 Transmitter Dongle** into a USB port on your workstation.
2. Launch the client:
   ```bash
   sudo ./cardputer-kvm
   ```
3. The KVM app automatically detects the serial port (e.g. `/dev/ttyUSB0` or `/dev/ttyACM0`) at 921,600 baud.
4. Press **`[Scroll Lock]`**:
   * The footer updates to: `● KVM Engaged (Input routed to target) │ [Scroll Lock] to return`.
   * Your keyboard's Scroll Lock LED lights up.
   * Physical keyboard and mouse events are exclusively routed to the Cardputer.
   * Local Linux desktop receives no input.
5. Press **`[Scroll Lock]`** again:
   * Control instantly returns to your local desktop.
   * The Scroll Lock LED turns off.

---

## 4. Compilation & Build

### Prerequisites
* **Go 1.21+** (`sudo pacman -S go` on Arch/CachyOS).
* Fyne dependencies (`libgl1-mesa-dev`, `xorg-dev` on Debian/Ubuntu, or standard X11/OpenGL packages on Arch).

### Build Binary
```bash
cd pc-client
go build -o cardputer-kvm .
```

---

## 5. Usage & Options

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

---

## 6. Project Structure

```text
pc-client/
├── go.mod                     # Go module definitions
├── go.sum                     # Checksums
├── main.go                    # Entry point, CLI flags & lifecycle
├── pkg/
│   ├── evdev/
│   │   ├── grabber.go         # Linux kernel EVIOCGRAB & 125Hz mouse coalescer
│   │   ├── grabber_test.go    # Device discovery & test suite
│   │   └── keymap.go          # Linux evdev keycodes -> USB HID scan code table
│   ├── gui/
│   │   ├── gui.go             # Fyne Desktop GUI, telemetry HUD & event loops
│   │   ├── battery_donut.go   # Vector-drawn circular battery ring gauge
│   │   └── pulse_dot.go       # Animated beating heart status dot
│   ├── protocol/
│   │   ├── protocol.go        # Binary packet encoders (6KRO key, mouse, telemetry, heartbeat)
│   │   └── protocol_test.go   # Protocol unit test suite
│   └── transport/
│       └── transport.go       # 921,600 baud UART0 driver, auto-detection, RTT latency loop
└── README.md                  # This documentation
```

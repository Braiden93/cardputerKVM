# Cardputer-KVM — Wireless Transmitter (ESP32-S3 N16R8)

Wireless KVM Transmitter firmware designed for **ESP32-S3 N16R8** (16MB Flash, 8MB Octal PSRAM) or any ESP32-S3 USB dongle.

Functions as a high-speed hardware RF bridge between your workstation and the **Cardputer-ADV Receiver**:
* Connects to your PC via USB Serial UART0 at **921,600 baud**.
* Translates binary serial packets from the `pc-client` desktop app into low-latency **ESP-NOW 2.4 GHz RF** packets.
* Automatically relays telemetry received from the Cardputer (battery %, voltage, RSSI, target USB status) straight back to your PC.

---

## 1. Features

1. **Hardware Configuration (N16R8):**
   * Target: `ESP32-S3`
   * Flash: 16MB SPI Flash (`DIO`, `80MHz`)
   * PSRAM: 8MB Octal PSRAM (`OPI`, `80MHz`)
   * CPU: Dual-Core 240MHz
2. **High-Speed Serial Interface (921,600 Baud):**
   * Fast non-blocking UART0 binary protocol.
   * Delivers sub-millisecond mouse movements (125Hz) and instant 6KRO keystrokes.
3. **ESP-NOW 2.4 GHz Wireless Link:**
   * Operates on **Channel 1** with 12M PHY rate.
   * Broadcasts to `FF:FF:FF:FF:FF:FF` with automatic peer registration.
4. **Intelligent Heartbeat Arbitration:**
   * **Standby Mode (`0x00`):** When the PC app is not connected or idle, the transmitter broadcasts an idle standby heartbeat beacon once per second. The Cardputer LCD shows `RF:STANDBY` (Amber).
   * **Active / Takeover Mode (`0x01` / `0x02`):** When the PC client connects and streams, the transmitter automatically yields heartbeat control to the PC app. The Cardputer LCD transitions to `RF:READY` (Cyan) or `RF:ENGAGED` (Neon Green).
5. **Bidirectional Telemetry Relay:**
   * Receives `KVM_PKT_TELEMETRY` over ESP-NOW from the Cardputer and forwards it directly onto UART0 for the PC companion app.
6. **Interactive Serial Console Bridge:**
   * When opened in `idf.py monitor` (without the PC client app), typing in the terminal directly beams keystrokes to the target PC!
7. **Onboard BOOT Button (GPIO 0):**
   * Pressing the BOOT button on the S3 board beams a test greeting over the air:
     `[Hello from ESP32-S3 N16R8 over ESP-NOW!]\n`

---

## 2. How to Flash the Transmitter

Connect your ESP32-S3 board to your PC via its USB port (usually `/dev/ttyUSB0` or `/dev/ttyACM0`).

```bash
# 1. Activate the local ESP-IDF environment
. ./esp-sdk/export.sh

# 2. Enter transmitter directory
cd transmitter

# 3. Flash and open serial monitor
idf.py -p /dev/ttyUSB0 flash monitor
```

---

## 3. End-to-End Test Workflow

1. **Cardputer-ADV (Receiver):**
   * Plugged into the target PC via USB-C.
   * Screen displays `USB` (Green) and `RF:STANDBY` (Amber) once transmitter is powered on.
2. **Launch PC Companion App:**
   * On your Linux workstation, run `cd pc-client && go run .`
   * The status changes to `RF:READY` (Cyan) with a beating heart animation!
   * Press **`[Scroll Lock]`** to engage KVM control: status becomes `RF:ENGAGED` (Neon Green), and mouse cursor/typing is routed to the target machine!

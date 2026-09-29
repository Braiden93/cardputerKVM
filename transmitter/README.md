# Cardputer-KVM — Wireless Transmitter (ESP32-S3 N16R8)

Wireless KVM Transmitter firmware designed for **ESP32-S3 N16R8** (16MB Flash, 8MB Octal PSRAM).

Beams keystrokes wirelessly to the **Cardputer-ADV Receiver** via **ESP-NOW 2.4 GHz RF**, which injects them directly into the target PC as an authentic **Dell KB216 Wired Keyboard**.

---

## 1. Features

1. **Hardware Configuration (N16R8):**
   * Target: `ESP32-S3`
   * Flash: 16MB SPI Flash (`DIO`, `80MHz`)
   * PSRAM: 8MB Octal PSRAM (`OPI`, `80MHz`)
   * CPU: Dual-Core 240MHz
2. **ESP-NOW 2.4 GHz Wireless Link:**
   * Operates on **Channel 1** (peer-to-peer RF, ~2–5 ms latency).
   * Broadcasts to `FF:FF:FF:FF:FF:FF` (zero MAC hardcoding needed).
3. **Interactive Serial Console Bridge:**
   * Open `idf.py monitor` on your transmitter: any key you type on your development PC keyboard is read by the transmitter, beamed wirelessly over ESP-NOW to the Cardputer, and typed onto the target host PC!
4. **Onboard BOOT Button (GPIO 0):**
   * Press the BOOT button on the S3 board to beam a test greeting over the air:
     `[Hello from ESP32-S3 N16R8 over ESP-NOW!]\n`
5. **Heartbeat Link Telemetry:**
   * Beams a heartbeat packet every 2 seconds so the Cardputer-ADV screen displays:
     `RF: LINKED [Transmitter MAC]`

---

## 2. How to Flash the Transmitter

Connect your ESP32-S3 N16R8 board to your PC via its USB port (usually `/dev/ttyUSB0` or `/dev/ttyACM0`).

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
   * Screen displays:
     * `USB: CONNECTED (Dell KB216)` (Green)
     * `RF: LINKED [...]` (Cyan) once transmitter boots.
2. **ESP32-S3 N16R8 (Transmitter):**
   * Powered on and running `idf.py monitor`.
   * **Test 1:** Press the **BOOT button** on the transmitter $\rightarrow$ Watch text appear on the target PC!
   * **Test 2:** Type into the transmitter terminal $\rightarrow$ Watch characters appear on the target PC in real-time!

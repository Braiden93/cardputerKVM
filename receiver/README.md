# Cardputer-KVM — Receiver Firmware (M5Stack Cardputer-ADV)

Firmware for the **M5Stack Cardputer-ADV** operating as the primary **Smart KVM Receiver and Host Interface**:
* **USB Stack:** Native USB OTG emulating an authentic **Dell Wireless Keyboard & Mouse Combo** (`0x413C:0x2501` KM636 / KM714 Wireless Receiver) with composite 1000Hz Boot Keyboard and Boot Mouse interfaces.
* **USB State Detection:** Instant cable-unplug detection via TinyUSB Suspend (`tud_suspended()`) within 3ms.
* **Keypad Controller:** Official M5Stack TCA8418 I²C driver with complete Fn/Shift keymapping.
* **Display & Graphics:** M5GFX offscreen sprite canvas rendering at 30+ FPS (tear-free DMA blits).
* **Power & Telemetry:** Dedicated FreeRTOS `telemetry_task` reading ADC1 battery level and voltage, beaming live status back to the transmitter.

---

## 1. Features

1. **Dell Wireless Combo Spoofing:**
   * VID: `0x413C` (Dell Inc.)
   * PID: `0x2501` (Dell KM636 / KM714 Wireless Combo Receiver)
   * Product: `"Wireless Receiver"`
   * Manufacturer: `"Dell"`
   * Dedicated 1000Hz (1ms polling) Boot Keyboard and Mouse endpoints.
2. **Instant USB Disconnect & Suspend Detection:**
   * Automatically detects when the USB-C cable is unplugged from the target PC or when the target host sleeps using USB 3ms bus inactivity detection (`tud_mounted() && !tud_suspended()`).
   * When unplugged, keystrokes and mouse movements fail fast rather than stalling on dead endpoints.
3. **On-Screen Live Dashboard (1.14" ST7789 LCD):**
   * **Top Status Bar:**
     * `USB` (Green) when active host connected, `NO USB` (Red) when unplugged.
     * `RF SCAN` (Gray), `RF:STANDBY` (Amber), `RF:READY` (Cyan), or `RF:ENGAGED` (Neon Green).
     * Animated beating heart icon synchronized with incoming RF packets.
     * Battery gauge icon with live percentage (Green > 40%, Yellow > 20%, Red < 20%).
   * **Left Panel (Keyboard Card):** Live keypress name, hex scancode, active modifiers (`CTRL`, `SHIFT`, `ALT`, `GUI`), and transmission counter.
   * **Right Panel (Mouse Card):** Directional cursor movement indicator, left/middle/right click indicators, and scroll wheel activity.
4. **Local Physical Keyboard Override:**
   * Physical keys typed on the Cardputer-ADV's 56-key keyboard are scanned via the TCA8418 I²C controller and injected directly into the target machine over USB.
5. **Bidirectional Telemetry Feedback:**
   * Transmits live battery %, battery mV, RSSI, and USB mounted status back to the transmitter dongle over ESP-NOW.

---

## 2. Hardware Compatibility Note (USB-C to USB-C)

> [!WARNING]
> **Type-A to Type-C vs Type-C to Type-C:**
> * Connecting via a **USB-A to USB-C cable** (from a PC or laptop) works **100% reliably**.
> * Connecting via a direct **USB-C to USB-C cable** to a smartphone or smart PD charger may fail to enumerate because the Stamp-S3 core has bridged `CC1` and `CC2` pins on the PCB. Smartphones interpret this as an analog audio adapter rather than a USB host.
> * **Solution for Phones:** Use a **USB-C to USB-A OTG adapter** on the phone, connected via a standard USB-A to USB-C cable to the Cardputer. Ensure **OTG Connection** is enabled in your phone's settings.

---

## 3. How to Build & Flash

Connect your Cardputer-ADV via its USB-C port to your computer.

```bash
# 1. Activate the local ESP-IDF environment
. ./esp-sdk/export.sh

# 2. Enter receiver directory
cd receiver

# 3. Configure target (first time)
idf.py set-target esp32s3

# 4. Build, flash and open serial monitor
idf.py -p /dev/ttyACM0 flash monitor
```

> [!TIP]
> **If flashing fails to connect:**
> Hold down the **G0 button** on the Stamp-S3 core while plugging in the USB cable (or flicking the power switch ON), then release. This puts the ESP32-S3 in ROM download mode.

---

## 4. How to Verify

1. **Verify OS Recognition:**
   Run in your terminal on the target computer:
   ```bash
   lsusb
   ```
   You will see:
   ```text
   ID 413c:2501 Dell Computer Corp. Wireless Receiver
   ```

2. **Verify Keystrokes & Mouse:**
   * Open any application on the target computer.
   * Type on the Cardputer's physical keyboard or send remote input from the transmitter/PC client: text and cursor movements respond with sub-millisecond latency.
   * Watch the Cardputer's screen update dynamically with real-time telemetry.

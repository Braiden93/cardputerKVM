# Cardputer-KVM — Receiver Firmware (M5Stack Cardputer-ADV)

Firmware for the **M5Stack Cardputer-ADV** operating as the primary **Smart KVM Receiver and Host Interface**:
* **USB Stack:** Native USB OTG emulating an authentic **Dell KB216 Wired Keyboard** (`0x413C:0x2113`).
* **Keypad Controller:** Official M5Stack TCA8418 I²C driver with complete Fn/Shift keymapping.
* **Display & Power:** M5Unified (ST7789 240x135 LCD, battery monitoring, PMIC, power hold).

---

## 1. Features

1. **Dell Keyboard Spoofing:**
   * VID: `0x413C` (Dell Inc.)
   * PID: `0x2113` (Dell KB216 Wired Keyboard)
   * Product: `"Dell KB216 Wired Keyboard"`
   * Manufacturer: `"Dell"`
2. **Local Physical Keyboard Override:**
   * Physical keys typed on the Cardputer-ADV are read via the TCA8418 driver and injected straight into the connected PC over USB!
3. **On-Screen Dashboard:**
   * The 1.14" ST7789 display displays:
     * Header with live Battery level (%)
     * USB Connection Status (`CONNECTED` in Green vs `WAITING` in Red)
     * Last key pressed and character code
     * Keystroke transmission counter
4. **Stamp-S3 G0 Button:**
   * Pressing the G0 button on the top Stamp-S3 core injects a test greeting string.

---

## 2. How to Build & Flash

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

## 3. How to Verify

1. **Verify OS Recognition:**
   Run in your terminal:
   ```bash
   lsusb
   ```
   You will see:
   ```text
   ID 413c:2113 Dell Computer Corp. Dell KB216 Wired Keyboard
   ```

2. **Verify Keystrokes:**
   * Open any text editor (Notepad, Gedit, terminal) on your computer.
   * Start typing on the **Cardputer's physical keyboard**: your PC will receive the letters in real-time!
   * Watch the Cardputer's screen update dynamically with the key name, keycode, and counter.

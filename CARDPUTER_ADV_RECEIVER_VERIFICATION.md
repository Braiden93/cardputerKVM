# Cardputer-ADV Receiver — Integration & Verification Report

## 1. Executive Summary
During initial hardware testing, the standalone ESP32-S3 Uno board was identified to have its onboard USB-C port hardwired through a **WCH CH340** USB-UART bridge (`1a86:7523`), which prevents native USB OTG / HID device emulation.

The project pivoted to designate the **M5Stack Cardputer-ADV** as the **primary KVM Receiver**. The Cardputer-ADV routes its external USB-C port directly to the ESP32-S3 native USB pins (**GPIO 19: D-**, **GPIO 20: D+**) and incorporates an on-board 1.14" color LCD, making it an ideal "Smart KVM Receiver" with live connection monitoring and local console override.

Using M5Stack's official HAL, we successfully implemented and verified native USB HID emulation spoofing an authentic **Dell KB216 Wired Keyboard**.

---

## 2. Integration with M5Stack Official HAL

Instead of rolling a custom low-level HAL, we extracted and integrated the official M5Stack hardware abstraction layer from the **`CardputerADV` branch** of [`m5stack/M5Cardputer-UserDemo`](https://github.com/m5stack/M5Cardputer-UserDemo/tree/CardputerADV):

| Component / Layer | Source / Implementation | Functionality in Receiver |
| :--- | :--- | :--- |
| **Keypad Controller** | `Adafruit_TCA8418` (`main/hal/utils/adafruit_tca8418/`) | Interfaces the Cardputer-ADV's dedicated **TCA8418 I²C keypad controller** (I²C addr `0x34`, INT on GPIO 11). |
| **Keyboard Mapping** | `Keyboard` class (`main/hal/keyboard/`) | Handles physical 56-key matrix, modifier masks, and Fn/Shift key combinations. |
| **Display & Power** | `M5Unified` & `M5GFX` | Manages the 1.14" ST7789 TFT display (240x135), backlight, AXP PMIC, and battery monitoring. |
| **USB HID Stack** | `tusb_hid_device_helper` (`main/hal/utils/tusb_hid_device/`) | TinyUSB wrapper configured with custom Dell descriptors and keycode dispatching. |

---

## 3. Dell Keyboard Spoofing Profile

The USB stack was configured to present the following USB identity to the target host:

* **Vendor ID (VID):** `0x413C` (Dell Inc.)
* **Product ID (PID):** `0x2113` (Dell KB216 Wired Keyboard)
* **bcdDevice:** `0x0108`
* **Manufacturer String:** `"Dell"`
* **Product String:** `"Dell KB216 Wired Keyboard"`
* **Serial Number:** `"CN0739W9715816780123"`

---

## 4. Verification Results

### A. Host OS Kernel Detection (`dmesg`)
When plugged into the target Linux host via USB-C, the kernel immediately enumerated the device without warnings:

```text
[92931.965647] usb 1-2: new full-speed USB device number 36 using xhci_hcd
[92932.089271] usb 1-2: New USB device found, idVendor=413c, idProduct=2113, bcdDevice= 1.08
[92932.090039] usb 1-2: New USB device strings: Mfr=1, Product=2, SerialNumber=3
[92932.090040] usb 1-2: Product: Dell KB216 Wired Keyboard
[92932.090041] usb 1-2: Manufacturer: Dell
[92932.090042] usb 1-2: SerialNumber: CN0739W9715816780123
[92932.092667] input: Dell Dell KB216 Wired Keyboard as /devices/.../input/input40
[92932.191375] input: Dell Dell KB216 Wired Keyboard Mouse as /devices/.../input/input41
[92932.191590] hid-generic 0003:413C:2113.000B: input,hidraw5: USB HID v1.11 Keyboard [Dell Dell KB216 Wired Keyboard] on usb-0000:00:14.0-2/input0
```

### B. Keystroke Injection Test
1. **Auto-greeting:** 1.5 seconds after USB mounting, the Cardputer automatically transmitted:
   `"Hello from Cardputer-ADV (Dell KB216)!\n"`, successfully injecting text directly into the host terminal.
2. **Local Keyboard Override:** Physical keys typed on the Cardputer-ADV keyboard are decoded by the TCA8418 driver and simultaneously injected into the target PC while updating the on-screen dashboard.

---

## 5. Architectural Role in Cardputer-KVM

With the Cardputer-ADV established as the **Receiver**:
1. **Direct USB Connection:** Plugs directly into the target PC as a hardware KVM dongle.
2. **Visual Feedback:** The 1.14" ST7789 display acts as an on-desk KVM status monitor (connection state, remote client link, battery level, last transmitted keystroke).
3. **Local Console Access:** Provides emergency physical keyboard input at the target PC if the wireless transmitter is disconnected.
4. **Wireless Endpoint:** Listens for incoming input packets via **ESP-NOW / Wi-Fi** from remote transmitters (ESP32-S3 remote, Android app, or PC client) and injects them into the host.

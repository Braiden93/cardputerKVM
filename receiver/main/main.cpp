/*
 * Cardputer-ADV KVM Receiver Test — Official M5Stack HAL + Dell Keyboard Spoofing
 *
 * Hardware: M5Stack Cardputer-ADV (ESP32-S3 + TCA8418 Keypad + ST7789 Display)
 * Features:
 *   - Native USB OTG emulating Dell KB216 Wired Keyboard (VID: 0x413C, PID: 0x2113)
 *   - Official M5Stack TCA8418 Keyboard Driver (reads all physical keys & modifiers)
 *   - Physical keystrokes injected directly into target PC over USB-C
 *   - Visual status & live keypress log on 1.14" ST7789 LCD
 *   - G0 (BtnA) button press sends test string to PC
 */

#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include <M5Unified.hpp>
#include <M5GFX.h>

#include "keyboard/keyboard.h"
#include "utils/tusb_hid_device/tusb_hid_device_helper.h"

static const char* TAG = "CARDPUTER_KVM";

static Keyboard keyboard;
static bool was_usb_mounted = false;
static std::string last_key_name = "None";
static uint8_t last_key_code = 0;
static uint32_t key_count = 0;

static void send_hid_key(uint8_t modifier, uint8_t keycode)
{
    if (!tusb_hid_device_helper_is_mounted()) return;
    uint8_t keys[6] = { keycode, 0, 0, 0, 0, 0 };
    tusb_hid_device_helper_report(modifier, keys);
    vTaskDelay(pdMS_TO_TICKS(20));
    tusb_hid_device_helper_report(0, NULL);
    vTaskDelay(pdMS_TO_TICKS(20));
}

static void send_test_greeting()
{
    const char* str = "Hello from Cardputer-ADV (Dell KB216)!\n";
    ESP_LOGI(TAG, "Sending greeting: %s", str);
    while (*str) {
        char c = *str++;
        uint8_t modifier = 0;
        uint8_t key = 0;
        if (c >= 'a' && c <= 'z') key = 0x04 + (c - 'a');
        else if (c >= 'A' && c <= 'Z') { key = 0x04 + (c - 'A'); modifier = 0x02; }
        else if (c >= '1' && c <= '9') key = 0x1E + (c - '1');
        else if (c == '0') key = 0x27;
        else if (c == ' ') key = 0x2C;
        else if (c == '\n') key = 0x28;
        else if (c == '!') { key = 0x1E; modifier = 0x02; }
        else if (c == '(') { key = 0x26; modifier = 0x02; }
        else if (c == ')') { key = 0x27; modifier = 0x02; }
        else if (c == '-') key = 0x2D;
        if (key) send_hid_key(modifier, key);
    }
}

static void update_display(bool force = false)
{
    static bool prev_mounted = false;
    static std::string prev_key = "";
    static int prev_batt = -1;

    bool current_mounted = tusb_hid_device_helper_is_mounted();
    int current_batt = M5.Power.getBatteryLevel();

    if (!force && prev_mounted == current_mounted && prev_key == last_key_name && prev_batt == current_batt) {
        return;
    }

    prev_mounted = current_mounted;
    prev_key = last_key_name;
    prev_batt = current_batt;

    M5.Display.startWrite();

    // Top Header Bar
    M5.Display.fillRect(0, 0, 240, 22, 0x10A2); // Dark blue header
    M5.Display.setTextColor(TFT_WHITE, 0x10A2);
    M5.Display.setTextSize(1);
    M5.Display.drawString("Cardputer-ADV  |  Dell Spoof", 6, 6);

    char batt_str[16];
    snprintf(batt_str, sizeof(batt_str), "%d%%", current_batt);
    M5.Display.drawRightString(batt_str, 234, 6);

    // USB Connection Status Banner
    if (current_mounted) {
        M5.Display.fillRect(6, 28, 228, 28, 0x03E0); // Green
        M5.Display.setTextColor(TFT_WHITE, 0x03E0);
        M5.Display.drawCenterString("USB: CONNECTED (Dell KB216)", 120, 36);
    } else {
        M5.Display.fillRect(6, 28, 228, 28, 0x7800); // Dark red / orange
        M5.Display.setTextColor(TFT_WHITE, 0x7800);
        M5.Display.drawCenterString("USB: WAITING FOR PC (Plug USB-C)", 120, 36);
    }

    // Keypress Display Box
    M5.Display.fillRect(6, 62, 228, 48, 0x18E3); // Dark slate panel
    M5.Display.drawRect(6, 62, 228, 48, 0x4A69);
    M5.Display.setTextColor(0x07FF, 0x18E3); // Cyan
    M5.Display.drawString("Last Input:", 14, 68);

    M5.Display.setTextColor(TFT_WHITE, 0x18E3);
    char key_info[64];
    snprintf(key_info, sizeof(key_info), "Key: %-8s  Code: 0x%02X", last_key_name.c_str(), last_key_code);
    M5.Display.drawString(key_info, 14, 84);

    char count_info[32];
    snprintf(count_info, sizeof(count_info), "Sent: %lu", (unsigned long)key_count);
    M5.Display.drawRightString(count_info, 226, 84);

    // Bottom Help Line
    M5.Display.fillRect(0, 116, 240, 19, TFT_BLACK);
    M5.Display.setTextColor(TFT_DARKGRAY, TFT_BLACK);
    M5.Display.drawCenterString("Type keys or press G0 to test injection", 120, 120);

    M5.Display.endWrite();
}

extern "C" void app_main(void)
{
    ESP_LOGI(TAG, "=== Cardputer-ADV KVM Dell Keyboard Test ===");

    // 1. Initialize M5Unified (Display, Power Management, Battery, I2C)
    auto cfg = M5.config();
    M5.begin(cfg);
    M5.Display.setRotation(1); // Landscape (240x135)
    M5.Display.setBrightness(120);
    M5.Display.fillScreen(TFT_BLACK);

    ESP_LOGI(TAG, "M5Unified initialized successfully.");

    // 2. Initialize TinyUSB HID with Dell KB216 Descriptors
    ESP_LOGI(TAG, "Initializing TinyUSB HID (Dell KB216)...");
    tusb_hid_device_helper_init();

    // 3. Initialize Cardputer-ADV Keyboard (TCA8418 I2C Matrix)
    ESP_LOGI(TAG, "Initializing Cardputer-ADV Keyboard...");
    if (!keyboard.init()) {
        ESP_LOGE(TAG, "Failed to initialize Cardputer-ADV Keyboard (TCA8418)!");
    } else {
        ESP_LOGI(TAG, "Cardputer-ADV Keyboard initialized successfully.");
    }

    // 4. Hook Keyboard Event Callback
    keyboard.onKeyEvent.connect([](const Keyboard::KeyEvent_t& keyEvent) {
        if (!tusb_hid_device_helper_is_mounted()) {
            return;
        }

        if (keyEvent.state) { // Key Pressed
            uint8_t mod = keyboard.getModifierMask() | keyEvent.extraModifiers;
            if (keyEvent.isModifier) {
                tusb_hid_device_helper_report(mod, NULL);
            } else {
                uint8_t keycode[6] = { keyEvent.keyCode, 0, 0, 0, 0, 0 };
                tusb_hid_device_helper_report(mod, keycode);
            }

            last_key_name = keyEvent.keyName ? keyEvent.keyName : "Special";
            last_key_code = keyEvent.keyCode;
            key_count++;

            ESP_LOGI(TAG, "Key Pressed: %s (0x%02X) Mod: 0x%02X",
                     last_key_name.c_str(), keyEvent.keyCode, mod);
        } else { // Key Released
            tusb_hid_device_helper_report(keyboard.getModifierMask(), NULL);
            ESP_LOGD(TAG, "Key Released");
        }

        update_display();
    });

    update_display(true);

    bool greeting_sent = false;

    // 5. Main Event Loop
    while (1) {
        M5.update();
        keyboard.update();

        bool is_mounted = tusb_hid_device_helper_is_mounted();
        if (is_mounted != was_usb_mounted) {
            was_usb_mounted = is_mounted;
            ESP_LOGI(TAG, "USB Mounted Status Changed: %s", is_mounted ? "CONNECTED" : "DISCONNECTED");
            update_display(true);

            // Auto-type greeting once after host enumeration
            if (is_mounted && !greeting_sent) {
                vTaskDelay(pdMS_TO_TICKS(1500));
                send_test_greeting();
                greeting_sent = true;
                last_key_name = "Greeting!";
                update_display(true);
            }
        }

        // BtnA (G0 button on Stamp-S3)
        if (M5.BtnA.wasPressed()) {
            ESP_LOGI(TAG, "BtnA (G0) Pressed! Injecting test string...");
            send_test_greeting();
            last_key_name = "G0 Clicked";
            key_count++;
            update_display(true);
        }

        // Periodic UI refresh for battery / state
        static uint32_t last_ui_tick = 0;
        uint32_t now = esp_log_timestamp();
        if (now - last_ui_tick > 1000) {
            last_ui_tick = now;
            update_display();
        }

        vTaskDelay(pdMS_TO_TICKS(5));
    }
}

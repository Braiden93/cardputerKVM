/*
 * Cardputer-ADV KVM Receiver — Official M5Stack HAL + Dell Keyboard Spoofing + ESP-NOW
 *
 * Hardware: M5Stack Cardputer-ADV (ESP32-S3 + TCA8418 Keypad + ST7789 Display)
 * Features:
 *   - Native USB OTG emulating Dell KB216 Wired Keyboard (VID: 0x413C, PID: 0x2113)
 *   - ESP-NOW 2.4 GHz wireless receiver (Channel 1, ultra-low latency)
 *   - Injects incoming wireless keystrokes directly into Host PC
 *   - Local Cardputer-ADV keyboard override (TCA8418 I2C driver)
 *   - Visual on-screen dashboard displaying USB status, RF link, battery, and live key log
 */

#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "esp_system.h"
#include "esp_wifi.h"
#include "esp_now.h"
#include "esp_netif.h"
#include "esp_event.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include <M5Unified.hpp>
#include <M5GFX.h>

#include "keyboard/keyboard.h"
#include "utils/tusb_hid_device/tusb_hid_device_helper.h"
#include "kvm_protocol.h"

static const char* TAG = "CARDPUTER_KVM";

static Keyboard keyboard;
static bool was_usb_mounted = false;
static std::string last_key_name = "None";
static uint8_t last_key_code = 0;
static uint32_t key_count = 0;

// ESP-NOW Remote Link State
static char remote_mac_str[18] = "None";
static uint32_t last_remote_packet_time = 0;
static bool remote_linked = false;

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

// Forward declarations
static void process_kvm_packet(const uint8_t *data, int len, const char *source_tag);
static void update_display(bool force = false);

typedef struct {
    uint8_t pkt_type;
    uint8_t modifiers;
    uint8_t keycodes[6];
    uint8_t buttons;
    int8_t  dx;
    int8_t  dy;
    int8_t  wheel;
    char    source_tag[12];
} kvm_event_t;

static QueueHandle_t s_kvm_queue = NULL;

static void kvm_dispatcher_task(void *pvParameters)
{
    kvm_event_t evt;
    ESP_LOGI(TAG, "High-Priority KVM USB Dispatcher Task started.");

    while (1) {
        if (xQueueReceive(s_kvm_queue, &evt, portMAX_DELAY)) {
            if (!tusb_hid_device_helper_is_mounted()) continue;

            if (evt.pkt_type == KVM_PKT_KEY_REPORT) {
                bool has_key = false;
                for (int i = 0; i < 6; i++) {
                    if (evt.keycodes[i] != 0) {
                        has_key = true;
                        break;
                    }
                }
                uint8_t *keys = has_key ? evt.keycodes : NULL;
                tusb_hid_device_helper_report(evt.modifiers, keys);

                last_key_code = evt.keycodes[0];
                last_key_name = has_key ? "Remote Key" : "Released";
                key_count++;
            } else if (evt.pkt_type == KVM_PKT_MOUSE_MOVE) {
                // High-performance queue coalescing: merge any backlog of mouse deltas
                int32_t total_dx = evt.dx;
                int32_t total_dy = evt.dy;
                int32_t total_wheel = evt.wheel;
                uint8_t buttons = evt.buttons;

                kvm_event_t next_evt;
                while (xQueuePeek(s_kvm_queue, &next_evt, 0) == pdTRUE) {
                    if (next_evt.pkt_type == KVM_PKT_MOUSE_MOVE) {
                        xQueueReceive(s_kvm_queue, &next_evt, 0);
                        total_dx += next_evt.dx;
                        total_dy += next_evt.dy;
                        total_wheel += next_evt.wheel;
                        buttons = next_evt.buttons;
                    } else {
                        break; // Next item is a key event, preserve order
                    }
                }

                if (total_dx > 127) total_dx = 127;
                if (total_dx < -127) total_dx = -127;
                if (total_dy > 127) total_dy = 127;
                if (total_dy < -127) total_dy = -127;
                if (total_wheel > 127) total_wheel = 127;
                if (total_wheel < -127) total_wheel = -127;

                tusb_hid_device_helper_mouse_report(buttons, (int8_t)total_dx, (int8_t)total_dy, (int8_t)total_wheel);
                last_key_name = "Mouse";
                key_count++;
            }
        }
    }
}

static void process_kvm_packet(const uint8_t *data, int len, const char *source_tag)
{
    if (len < sizeof(uint16_t)) return;
    uint16_t magic = *(uint16_t*)data;
    if (magic != KVM_ESPNOW_MAGIC) return;

    last_remote_packet_time = esp_log_timestamp();
    remote_linked = true;

    uint8_t pkt_type = data[2];

    if (pkt_type == KVM_PKT_KEY_REPORT && len >= sizeof(kvm_key_packet_t)) {
        const kvm_key_packet_t *pkt = (const kvm_key_packet_t*)data;
        kvm_event_t evt = {};
        evt.pkt_type = pkt_type;
        evt.modifiers = pkt->modifiers;
        memcpy(evt.keycodes, pkt->keycodes, 6);
        strncpy(evt.source_tag, source_tag, sizeof(evt.source_tag)-1);
        if (s_kvm_queue) {
            xQueueSend(s_kvm_queue, &evt, 0);
        }
    }
    else if (pkt_type == KVM_PKT_MOUSE_MOVE && len >= sizeof(kvm_mouse_packet_t)) {
        const kvm_mouse_packet_t *pkt = (const kvm_mouse_packet_t*)data;
        kvm_event_t evt = {};
        evt.pkt_type = pkt_type;
        evt.buttons = pkt->buttons;
        evt.dx = pkt->dx;
        evt.dy = pkt->dy;
        evt.wheel = pkt->wheel;
        strncpy(evt.source_tag, source_tag, sizeof(evt.source_tag)-1);
        if (s_kvm_queue) {
            xQueueSend(s_kvm_queue, &evt, 0);
        }
    }
    else if (pkt_type == KVM_PKT_HEARTBEAT) {
        ESP_LOGD(TAG, "[%s] Heartbeat received", source_tag);
    }
}

// ESP-NOW Receive Callback
static void on_espnow_recv(const esp_now_recv_info_t *recv_info, const uint8_t *data, int len)
{
    // Record remote transmitter MAC
    snprintf(remote_mac_str, sizeof(remote_mac_str), "%02X:%02X:%02X:%02X:%02X:%02X",
             recv_info->src_addr[0], recv_info->src_addr[1], recv_info->src_addr[2],
             recv_info->src_addr[3], recv_info->src_addr[4], recv_info->src_addr[5]);
    process_kvm_packet(data, len, "ESP-NOW");
}

static void init_espnow_receiver()
{
    ESP_LOGI(TAG, "Initializing Pure ESP-NOW Receiver on Channel %d...", KVM_ESPNOW_CHANNEL);

    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));
    ESP_ERROR_CHECK(esp_wifi_set_storage(WIFI_STORAGE_RAM));
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_start());
    ESP_ERROR_CHECK(esp_wifi_set_channel(KVM_ESPNOW_CHANNEL, WIFI_SECOND_CHAN_NONE));

    ESP_ERROR_CHECK(esp_now_init());
    ESP_ERROR_CHECK(esp_now_register_recv_cb(on_espnow_recv));

    ESP_LOGI(TAG, "Pure ESP-NOW Receiver Ready on Channel %d (Station Mode, No AP)", KVM_ESPNOW_CHANNEL);
}

static void update_display(bool force)
{
    static bool prev_mounted = false;
    static bool prev_rf_linked = false;
    static std::string prev_key = "";
    static int prev_batt = -1;

    bool current_mounted = tusb_hid_device_helper_is_mounted();
    int current_batt = M5.Power.getBatteryLevel();

    // Check remote link timeout (5 seconds)
    bool is_rf_alive = (esp_log_timestamp() - last_remote_packet_time < 5000) && remote_linked;

    if (!force && prev_mounted == current_mounted && prev_rf_linked == is_rf_alive &&
        prev_key == last_key_name && prev_batt == current_batt) {
        return;
    }

    prev_mounted = current_mounted;
    prev_rf_linked = is_rf_alive;
    prev_key = last_key_name;
    prev_batt = current_batt;

    M5.Display.startWrite();

    // Top Header Bar
    M5.Display.fillRect(0, 0, 240, 20, 0x10A2); // Dark blue header
    M5.Display.setTextColor(TFT_WHITE, 0x10A2);
    M5.Display.setTextSize(1);
    M5.Display.drawString("Cardputer-ADV KVM", 6, 5);

    char batt_str[16];
    snprintf(batt_str, sizeof(batt_str), "%d%%", current_batt);
    M5.Display.drawRightString(batt_str, 234, 5);

    // USB Connection Status Banner
    if (current_mounted) {
        M5.Display.fillRect(6, 24, 228, 22, 0x03E0); // Green
        M5.Display.setTextColor(TFT_WHITE, 0x03E0);
        M5.Display.drawCenterString("USB: CONNECTED (Dell KB216)", 120, 30);
    } else {
        M5.Display.fillRect(6, 24, 228, 22, 0x7800); // Dark red / orange
        M5.Display.setTextColor(TFT_WHITE, 0x7800);
        M5.Display.drawCenterString("USB: WAITING FOR HOST PC", 120, 30);
    }

    // Wireless RF Link Banner
    if (is_rf_alive) {
        M5.Display.fillRect(6, 50, 228, 22, 0x0013); // Dark teal
        M5.Display.setTextColor(0x07FF, 0x0013); // Cyan
        char rf_str[48];
        snprintf(rf_str, sizeof(rf_str), "RF: LINKED [%.8s...]", remote_mac_str);
        M5.Display.drawCenterString(rf_str, 120, 56);
    } else {
        M5.Display.fillRect(6, 50, 228, 22, 0x2124); // Dark gray
        M5.Display.setTextColor(TFT_YELLOW, 0x2124);
        M5.Display.drawCenterString("RF: ESP-NOW CH 1 (LISTENING)", 120, 56);
    }

    // Keypress Display Box
    M5.Display.fillRect(6, 76, 228, 38, 0x18E3); // Dark slate panel
    M5.Display.drawRect(6, 76, 228, 38, 0x4A69);

    M5.Display.setTextColor(TFT_WHITE, 0x18E3);
    char key_info[64];
    snprintf(key_info, sizeof(key_info), "Input: %-10s (0x%02X)", last_key_name.c_str(), last_key_code);
    M5.Display.drawString(key_info, 12, 88);

    char count_info[32];
    snprintf(count_info, sizeof(count_info), "Sent: %lu", (unsigned long)key_count);
    M5.Display.drawRightString(count_info, 226, 88);

    // Bottom Help Line
    M5.Display.fillRect(0, 118, 240, 17, TFT_BLACK);
    M5.Display.setTextColor(0x07FF, TFT_BLACK); // Cyan
    M5.Display.drawCenterString("ESP-NOW CH 1 | Pure RF Wireless", 120, 121);

    M5.Display.endWrite();
}

extern "C" void app_main(void)
{
    ESP_LOGI(TAG, "=== Cardputer-ADV KVM Receiver (Pure ESP-NOW) ===");

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

    // 4. Create KVM USB Event Queue and Dispatcher Task
    s_kvm_queue = xQueueCreate(64, sizeof(kvm_event_t));
    xTaskCreate(kvm_dispatcher_task, "kvm_disp", 4096, NULL, 15, NULL);

    // 5. Initialize Pure ESP-NOW Receiver
    init_espnow_receiver();

    // 6. Hook Local Keyboard Event Callback
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

            ESP_LOGI(TAG, "Local Key: %s (0x%02X) Mod: 0x%02X",
                     last_key_name.c_str(), keyEvent.keyCode, mod);
        } else { // Key Released
            tusb_hid_device_helper_report(keyboard.getModifierMask(), NULL);
        }

        update_display();
    });

    update_display(true);

    bool greeting_sent = false;

    // 6. Main Event Loop
    while (1) {
        M5.update();
        keyboard.update();

        bool is_mounted = tusb_hid_device_helper_is_mounted();
        if (is_mounted != was_usb_mounted) {
            was_usb_mounted = is_mounted;
            ESP_LOGI(TAG, "USB Status: %s", is_mounted ? "CONNECTED" : "DISCONNECTED");
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

        // Periodic UI refresh (5Hz, decoupled from USB dispatcher)
        static uint32_t last_ui_tick = 0;
        uint32_t now = esp_log_timestamp();
        if (now - last_ui_tick > 200) {
            last_ui_tick = now;
            update_display();
        }

        vTaskDelay(pdMS_TO_TICKS(5));
    }
}

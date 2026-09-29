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

typedef struct {
    int32_t dx;
    int32_t dy;
    int32_t wheel;
    uint8_t buttons;
    bool    dirty;
    portMUX_TYPE mux;
} kvm_mouse_state_t;

static kvm_mouse_state_t s_mouse_state = {
    .dx = 0,
    .dy = 0,
    .wheel = 0,
    .buttons = 0,
    .dirty = false,
    .mux = portMUX_INITIALIZER_UNLOCKED
};

static void kvm_dispatcher_task(void *pvParameters)
{
    ESP_LOGI(TAG, "Phase-Locked 1000Hz KVM USB Dispatcher Task started.");
    uint8_t last_sent_buttons = 0;

    while (1) {
        if (!tusb_hid_device_helper_is_mounted()) {
            vTaskDelay(pdMS_TO_TICKS(10));
            continue;
        }

        // 1. Process discrete keyboard events first
        kvm_event_t evt;
        if (s_kvm_queue && xQueueReceive(s_kvm_queue, &evt, 0) == pdTRUE) {
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
            }
        }

        // 2. Dispatch accumulated mouse displacement phase-locked to USB frame readiness
        portENTER_CRITICAL(&s_mouse_state.mux);
        bool has_motion = s_mouse_state.dirty;
        portEXIT_CRITICAL(&s_mouse_state.mux);

        if (has_motion) {
            if (tusb_hid_device_helper_ready()) {
                portENTER_CRITICAL(&s_mouse_state.mux);
                int32_t m_dx = s_mouse_state.dx;
                int32_t m_dy = s_mouse_state.dy;
                int32_t m_wheel = s_mouse_state.wheel;
                uint8_t m_buttons = s_mouse_state.buttons;

                bool buttons_changed = (m_buttons != last_sent_buttons);

                if (m_dx != 0 || m_dy != 0 || m_wheel != 0 || buttons_changed) {
                    int8_t send_dx = (m_dx > 127) ? 127 : ((m_dx < -127) ? -127 : (int8_t)m_dx);
                    int8_t send_dy = (m_dy > 127) ? 127 : ((m_dy < -127) ? -127 : (int8_t)m_dy);
                    int8_t send_wheel = (m_wheel > 127) ? 127 : ((m_wheel < -127) ? -127 : (int8_t)m_wheel);

                    s_mouse_state.dx -= send_dx;
                    s_mouse_state.dy -= send_dy;
                    s_mouse_state.wheel -= send_wheel;
                    s_mouse_state.dirty = (s_mouse_state.dx != 0 || s_mouse_state.dy != 0 || s_mouse_state.wheel != 0);
                    last_sent_buttons = m_buttons;
                    portEXIT_CRITICAL(&s_mouse_state.mux);

                    tusb_hid_device_helper_mouse_report_nowait(m_buttons, send_dx, send_dy, send_wheel);
                    last_key_name = "Mouse";
                    key_count++;

                    // Sent 1 frame: USB endpoint is now busy for 1ms SOF
                    vTaskDelay(pdMS_TO_TICKS(1));
                } else {
                    s_mouse_state.dirty = false;
                    portEXIT_CRITICAL(&s_mouse_state.mux);
                    vTaskDelay(pdMS_TO_TICKS(1));
                }
            } else {
                // USB endpoint busy transmitting previous 1ms frame:
                // Micro-sleep to phase-lock with host SOF completion without burning CPU
                esp_rom_delay_us(100);
            }
        } else {
            // Idle: sleep 1 tick (1ms)
            vTaskDelay(pdMS_TO_TICKS(1));
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
        portENTER_CRITICAL(&s_mouse_state.mux);
        s_mouse_state.dx += pkt->dx;
        s_mouse_state.dy += pkt->dy;
        s_mouse_state.wheel += pkt->wheel;
        s_mouse_state.buttons = pkt->buttons;
        s_mouse_state.dirty = true;
        portEXIT_CRITICAL(&s_mouse_state.mux);
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
    esp_wifi_config_espnow_rate(WIFI_IF_STA, WIFI_PHY_RATE_24M);

    ESP_LOGI(TAG, "Pure ESP-NOW Receiver Ready on Channel %d (Station Mode, 24M PHY, No AP)", KVM_ESPNOW_CHANNEL);
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
        M5.Display.drawCenterString("USB: CONNECTED (Dell Wireless)", 120, 30);
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

    // 2. Initialize TinyUSB HID with Dell Wireless Combo Receiver Descriptors
    ESP_LOGI(TAG, "Initializing TinyUSB HID (Dell Wireless Combo Receiver 413c:2501)...");
    tusb_hid_device_helper_init();

    // 3. Initialize Cardputer-ADV Keyboard (TCA8418 I2C Matrix)
    ESP_LOGI(TAG, "Initializing Cardputer-ADV Keyboard...");
    if (!keyboard.init()) {
        ESP_LOGE(TAG, "Failed to initialize Cardputer-ADV Keyboard (TCA8418)!");
    } else {
        ESP_LOGI(TAG, "Cardputer-ADV Keyboard initialized successfully.");
    }

    // 4. Create KVM USB Event Queue and Dispatcher Task (pinned to CPU 0, separate from TinyUSB on CPU 1)
    s_kvm_queue = xQueueCreate(256, sizeof(kvm_event_t));
    xTaskCreatePinnedToCore(kvm_dispatcher_task, "kvm_disp", 4096, NULL, 6, NULL, 0);

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

    // 6. Main Event Loop
    while (1) {
        M5.update();
        keyboard.update();

        bool is_mounted = tusb_hid_device_helper_is_mounted();
        if (is_mounted != was_usb_mounted) {
            was_usb_mounted = is_mounted;
            ESP_LOGI(TAG, "USB Status: %s", is_mounted ? "CONNECTED" : "DISCONNECTED");
            update_display(true);
        }

        // BtnA (G0 button on Stamp-S3) - manual LCD refresh
        if (M5.BtnA.wasPressed()) {
            ESP_LOGI(TAG, "BtnA (G0) Pressed! Refreshing display...");
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

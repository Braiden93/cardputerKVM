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

// Live Input Activity & Visual Feedback Tracking
static uint32_t last_kbd_event_time = 0;
static uint8_t  last_kbd_mod = 0;
static bool     last_kbd_is_local = false;

static uint32_t last_mouse_event_time = 0;
static uint32_t last_mouse_move_time = 0;
static uint32_t last_mouse_click_time = 0;
static uint32_t last_mouse_wheel_time = 0;
static int32_t  live_display_mouse_dx = 0;
static int32_t  live_display_mouse_dy = 0;
static int32_t  live_display_mouse_wheel = 0;
static uint8_t  live_display_mouse_buttons = 0;
static uint8_t  last_clicked_buttons = 0;
static uint32_t total_mouse_packets = 0;

// Offscreen Canvas for Tear-Free 25FPS Rendering
static M5Canvas s_canvas(&M5.Display);
static bool s_has_canvas = false;

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

static std::string get_key_display_string(uint8_t mod, uint8_t code)
{
    std::string s = "";
    if (mod & 0x11) s += "Ctrl+";
    if (mod & 0x22) s += "Shift+";
    if (mod & 0x44) s += "Alt+";
    if (mod & 0x88) s += "Win+";

    if (code >= 0x04 && code <= 0x1D) {
        char ch = 'A' + (code - 0x04);
        s += ch;
    } else if (code >= 0x1E && code <= 0x26) {
        char ch = '1' + (code - 0x1E);
        s += ch;
    } else if (code == 0x27) {
        s += "0";
    } else if (code == 0x28) {
        s += "Enter";
    } else if (code == 0x29) {
        s += "Esc";
    } else if (code == 0x2A) {
        s += "Bksp";
    } else if (code == 0x2B) {
        s += "Tab";
    } else if (code == 0x2C) {
        s += "Space";
    } else if (code == 0x2D) {
        s += "-";
    } else if (code == 0x2E) {
        s += "=";
    } else if (code == 0x2F) {
        s += "[";
    } else if (code == 0x30) {
        s += "]";
    } else if (code == 0x31) {
        s += "\\";
    } else if (code == 0x33) {
        s += ";";
    } else if (code == 0x34) {
        s += "'";
    } else if (code == 0x36) {
        s += ",";
    } else if (code == 0x37) {
        s += ".";
    } else if (code == 0x38) {
        s += "/";
    } else if (code >= 0x3A && code <= 0x45) {
        char buf[8];
        snprintf(buf, sizeof(buf), "F%d", code - 0x3A + 1);
        s += buf;
    } else if (code == 0x4F) {
        s += "Right";
    } else if (code == 0x50) {
        s += "Left";
    } else if (code == 0x51) {
        s += "Down";
    } else if (code == 0x52) {
        s += "Up";
    } else if (code != 0) {
        char buf[8];
        snprintf(buf, sizeof(buf), "0x%02X", code);
        s += buf;
    } else if (s.empty()) {
        s = "None";
    } else {
        if (!s.empty() && s.back() == '+') {
            s.pop_back();
        }
    }
    return s;
}

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

                if (has_key || evt.modifiers != 0) {
                    last_kbd_event_time = esp_log_timestamp();
                    last_kbd_is_local = false;
                    last_key_code = evt.keycodes[0];
                    last_kbd_mod = evt.modifiers;
                    last_key_name = get_key_display_string(evt.modifiers, evt.keycodes[0]);
                    key_count++;
                } else {
                    last_key_name = "None";
                    last_key_code = 0;
                    last_kbd_mod = 0;
                }
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

    uint32_t now = esp_log_timestamp();
    last_remote_packet_time = now;
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
        last_mouse_event_time = now;
        total_mouse_packets++;

        if (pkt->dx != 0 || pkt->dy != 0) {
            last_mouse_move_time = now;
            live_display_mouse_dx = pkt->dx;
            live_display_mouse_dy = pkt->dy;
        }
        if (pkt->wheel != 0) {
            last_mouse_wheel_time = now;
            live_display_mouse_wheel = pkt->wheel;
        }
        if (pkt->buttons != 0) {
            last_mouse_click_time = now;
            last_clicked_buttons |= pkt->buttons;
        }
        live_display_mouse_buttons = pkt->buttons;

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

// -------------------------------------------------------------
// Visual Icon & Card Renderers
// -------------------------------------------------------------

static void draw_usb_icon(LovyanGFX &gfx, int x, int y, bool connected)
{
    uint16_t col = connected ? 0x07E0 : 0xF800; // Bright Green : Red

    // Central trunk
    gfx.drawFastVLine(x + 5, y + 2, 9, col);

    // Top arrow (pointing up)
    gfx.drawPixel(x + 5, y + 0, col);
    gfx.drawFastHLine(x + 4, y + 1, 3, col);
    gfx.drawFastHLine(x + 3, y + 2, 5, col);

    // Left branch (square head)
    gfx.drawLine(x + 5, y + 6, x + 2, y + 4, col);
    gfx.drawFastVLine(x + 2, y + 2, 2, col);
    gfx.fillRect(x + 1, y + 1, 3, 3, col);

    // Right branch (circle head)
    gfx.drawLine(x + 5, y + 7, x + 8, y + 5, col);
    gfx.drawFastVLine(x + 8, y + 3, 2, col);
    gfx.fillCircle(x + 8, y + 2, 1, col);

    // Bottom base circle
    gfx.fillCircle(x + 5, y + 11, 2, col);

    // Text status badge
    gfx.setTextColor(col, 0x10A2);
    gfx.drawString(connected ? "USB" : "NO USB", x + 14, y + 3);
}

static void draw_rf_icon(LovyanGFX &gfx, int x, int y, bool linked)
{
    uint16_t col = linked ? 0x07FF : 0x7BEF; // Neon Cyan : Slate Gray

    // Central antenna mast
    gfx.drawFastVLine(x + 6, y + 6, 6, col);
    gfx.fillCircle(x + 6, y + 5, 2, col);

    // Inner radiating waves
    gfx.drawPixel(x + 3, y + 3, col);
    gfx.drawPixel(x + 4, y + 2, col);
    gfx.drawFastHLine(x + 5, y + 1, 3, col);
    gfx.drawPixel(x + 8, y + 2, col);
    gfx.drawPixel(x + 9, y + 3, col);

    // Outer radiating waves
    gfx.drawPixel(x + 0, y + 3, col);
    gfx.drawPixel(x + 1, y + 1, col);
    gfx.drawFastHLine(x + 3, y - 1, 7, col);
    gfx.drawPixel(x + 11, y + 1, col);
    gfx.drawPixel(x + 12, y + 3, col);

    // Text label
    gfx.setTextColor(col, 0x10A2);
    gfx.drawString(linked ? "RF LINK" : "RF SCAN", x + 16, y + 3);
}

static void draw_battery_icon(LovyanGFX &gfx, int x, int y, int batt_level)
{
    if (batt_level < 0) batt_level = 0;
    if (batt_level > 100) batt_level = 100;

    // Body
    gfx.drawRoundRect(x, y, 18, 10, 2, 0x7BEF);
    // Terminal nub
    gfx.fillRect(x + 18, y + 3, 2, 4, 0x7BEF);

    // Color based on level
    uint16_t col = (batt_level > 40) ? 0x07E0 : ((batt_level > 20) ? 0xFFE0 : 0xF800);
    int fill_w = (batt_level * 14) / 100;
    if (fill_w > 0) {
        gfx.fillRect(x + 2, y + 2, fill_w, 6, col);
    }

    char batt_str[12];
    snprintf(batt_str, sizeof(batt_str), "%d%%", batt_level);
    gfx.setTextColor(TFT_WHITE, 0x10A2);
    gfx.drawRightString(batt_str, x - 4, y + 1);
}

static void draw_keyboard_card(LovyanGFX &gfx, int x, int y, int w, int h, bool active,
                               const std::string &key_name, uint8_t key_code, uint8_t mod)
{
    // Background card panel
    gfx.fillRoundRect(x, y, w, h, 4, 0x0842);
    gfx.drawRoundRect(x, y, w, h, 4, active ? 0xFFE0 : 0x2965);

    // Header label & status dot
    gfx.setTextColor(active ? 0xFFE0 : 0x7BEF, 0x0842);
    gfx.drawString("KEYBOARD", x + 8, y + 5);
    gfx.fillCircle(x + w - 10, y + 8, 3, active ? 0xFFE0 : 0x31A6);

    // Keyboard chassis
    int kx = x + 16, ky = y + 17;
    gfx.fillRoundRect(kx, ky, 82, 34, 3, active ? 0x2104 : 0x10A2);
    gfx.drawRoundRect(kx, ky, 82, 34, 3, active ? 0xFFE0 : 0x4208);

    uint16_t key_col = active ? 0xFFE0 : 0x4A69;

    // Row 1 (7 keys)
    for (int i = 0; i < 7; i++) {
        gfx.fillRect(kx + 4 + i * 11, ky + 3, 8, 5, key_col);
    }
    // Row 2 (7 keys)
    for (int i = 0; i < 7; i++) {
        gfx.fillRect(kx + 4 + i * 11, ky + 10, 8, 5, key_col);
    }
    // Row 3 (7 keys)
    for (int i = 0; i < 7; i++) {
        gfx.fillRect(kx + 4 + i * 11, ky + 17, 8, 5, key_col);
    }
    // Row 4: Modifiers + Spacebar
    uint16_t mod_col = (active && mod != 0) ? 0xFFFF : key_col;
    uint16_t space_col = (active && key_code == 0x2C) ? 0xFFFF : key_col;
    gfx.fillRect(kx + 4, ky + 24, 12, 6, mod_col);
    gfx.fillRect(kx + 20, ky + 24, 42, 6, space_col);
    gfx.fillRect(kx + 66, ky + 24, 12, 6, mod_col);

    // Key display pill
    gfx.fillRoundRect(x + 6, y + 56, w - 12, 20, 3, 0x10A2);
    gfx.drawRoundRect(x + 6, y + 56, w - 12, 20, 3, active ? 0xFFE0 : 0x2125);

    gfx.setTextColor(active ? 0xFFFF : 0x07FF, 0x10A2);
    gfx.drawCenterString(key_name.empty() ? "None" : key_name.c_str(), x + w / 2, y + 62);

    // Scancode / info
    if (key_code != 0) {
        char code_str[16];
        snprintf(code_str, sizeof(code_str), "HID: 0x%02X", key_code);
        gfx.setTextColor(0x52AA, 0x0842);
        gfx.drawCenterString(code_str, x + w / 2, y + 79);
    } else {
        gfx.setTextColor(0x52AA, 0x0842);
        gfx.drawCenterString("IDLE", x + w / 2, y + 79);
    }
}

static void draw_mouse_card(LovyanGFX &gfx, int x, int y, int w, int h,
                            bool moving, bool left_click, bool right_click, bool mid_click,
                            int32_t dx, int32_t dy, int32_t wheel)
{
    bool any_active = moving || left_click || right_click || mid_click;

    // Background card panel
    gfx.fillRoundRect(x, y, w, h, 4, 0x0842);
    uint16_t border_col = (left_click || right_click) ? 0xF800 : (moving ? 0x07E0 : (mid_click ? 0x07FF : 0x2965));
    gfx.drawRoundRect(x, y, w, h, 4, border_col);

    // Header label & status dot
    gfx.setTextColor(any_active ? border_col : 0x7BEF, 0x0842);
    gfx.drawString("MOUSE", x + 8, y + 5);
    gfx.fillCircle(x + w - 10, y + 8, 3, any_active ? border_col : 0x31A6);

    // Mouse Graphic
    int mx = x + 39, my = y + 17;

    // Palm Rest (Lower Shell)
    gfx.fillRoundRect(mx, my + 15, 36, 25, 7, moving ? 0x0320 : 0x10A2);
    gfx.drawRoundRect(mx, my + 15, 36, 25, 7, moving ? 0x07E0 : 0x31A6);

    if (moving) {
        gfx.drawCircle(mx + 18, my + 27, 4, 0x07E0);
        gfx.drawCircle(mx + 18, my + 27, 7, 0x04A0);
    } else {
        gfx.drawFastHLine(mx + 12, my + 28, 12, 0x2965);
    }

    // Left Button
    int lb_x = mx, lb_y = my, lb_w = 16, lb_h = 16;
    uint16_t lb_col = left_click ? 0xF800 : 0x18E3;
    gfx.fillRoundRect(lb_x, lb_y, lb_w, lb_h, 3, lb_col);
    gfx.drawRoundRect(lb_x, lb_y, lb_w, lb_h, 3, left_click ? 0xFFFF : 0x4208);
    gfx.setTextColor(left_click ? 0xFFFF : 0x632C, lb_col);
    gfx.drawString("L", lb_x + 5, lb_y + 4);

    // Right Button
    int rb_x = mx + 20, rb_y = my, rb_w = 16, rb_h = 16;
    uint16_t rb_col = right_click ? 0xF800 : 0x18E3;
    gfx.fillRoundRect(rb_x, rb_y, rb_w, rb_h, 3, rb_col);
    gfx.drawRoundRect(rb_x, rb_y, rb_w, rb_h, 3, right_click ? 0xFFFF : 0x4208);
    gfx.setTextColor(right_click ? 0xFFFF : 0x632C, rb_col);
    gfx.drawString("R", rb_x + 6, rb_y + 4);

    // Scroll Wheel
    int mw_x = mx + 15, mw_y = my + 2, mw_w = 6, mw_h = 11;
    uint16_t mw_col = mid_click ? 0x07FF : 0x39E7;
    gfx.fillRoundRect(mw_x, mw_y, mw_w, mw_h, 2, mw_col);
    gfx.drawRoundRect(mw_x, mw_y, mw_w, mw_h, 2, mid_click ? 0xFFFF : 0x632C);

    // Directional Motion Indicator Arrows (▲, ▼, ◄, ►)
    // UP Arrow
    uint16_t up_col = (moving && dy < 0) ? 0x07E0 : 0x18C3;
    gfx.fillTriangle(mx + 18, my - 5, mx + 15, my - 2, mx + 21, my - 2, up_col);
    // DOWN Arrow
    uint16_t down_col = (moving && dy > 0) ? 0x07E0 : 0x18C3;
    gfx.fillTriangle(mx + 18, my + 44, mx + 15, my + 41, mx + 21, my + 41, down_col);
    // LEFT Arrow
    uint16_t left_arrow_col = (moving && dx < 0) ? 0x07E0 : 0x18C3;
    gfx.fillTriangle(mx - 5, my + 24, mx - 2, my + 21, mx - 2, my + 27, left_arrow_col);
    // RIGHT Arrow
    uint16_t right_arrow_col = (moving && dx > 0) ? 0x07E0 : 0x18C3;
    gfx.fillTriangle(mx + 41, my + 24, mx + 38, my + 21, mx + 38, my + 27, right_arrow_col);

    // Readout Pill
    gfx.fillRoundRect(x + 6, y + 56, w - 12, 32, 3, 0x10A2);
    gfx.drawRoundRect(x + 6, y + 56, w - 12, 32, 3, border_col);

    // Vector readout
    char vec_str[32];
    snprintf(vec_str, sizeof(vec_str), "dX:%+3d dY:%+3d", (int)dx, (int)dy);
    gfx.setTextColor(moving ? 0x07E0 : 0x7BEF, 0x10A2);
    gfx.drawCenterString(vec_str, x + w / 2, y + 60);

    // Button states [L] [M] [R]
    gfx.setTextColor(left_click ? 0xF800 : 0x4A69, 0x10A2);
    gfx.drawString("[L]", x + 16, y + 74);

    gfx.setTextColor(mid_click ? 0x07FF : 0x4A69, 0x10A2);
    gfx.drawString("[M]", x + 48, y + 74);

    gfx.setTextColor(right_click ? 0xF800 : 0x4A69, 0x10A2);
    gfx.drawString("[R]", x + 80, y + 74);
}

static void update_display(bool force)
{
    uint32_t now = esp_log_timestamp();

    bool current_mounted = tusb_hid_device_helper_is_mounted();
    int current_batt = M5.Power.getBatteryLevel();
    bool is_rf_alive = (now - last_remote_packet_time < 5000) && remote_linked;

    bool kbd_active = (now - last_kbd_event_time < 220);
    bool mouse_moving = (now - last_mouse_move_time < 220);
    bool left_clicked = (live_display_mouse_buttons & 0x01) ||
                        ((last_clicked_buttons & 0x01) && (now - last_mouse_click_time < 220));
    bool right_clicked = (live_display_mouse_buttons & 0x02) ||
                         ((last_clicked_buttons & 0x02) && (now - last_mouse_click_time < 220));
    bool mid_clicked = (live_display_mouse_buttons & 0x04) ||
                        ((last_clicked_buttons & 0x04) && (now - last_mouse_click_time < 220)) ||
                        (now - last_mouse_wheel_time < 220);

    if (now - last_mouse_click_time > 220) {
        last_clicked_buttons = live_display_mouse_buttons;
    }

    LovyanGFX &gfx = s_has_canvas ? (LovyanGFX&)s_canvas : (LovyanGFX&)M5.Display;

    if (!s_has_canvas) {
        M5.Display.startWrite();
    }

    // Top Header Bar
    gfx.fillRect(0, 0, 240, 20, 0x10A2); // Dark blue header
    gfx.drawFastHLine(0, 20, 240, 0x2125);

    // USB Icon (x: 6, y: 4)
    draw_usb_icon(gfx, 6, 4, current_mounted);

    // RF Icon (x: 76, y: 4)
    draw_rf_icon(gfx, 76, 4, is_rf_alive);

    // Battery Icon (x: 204, y: 5)
    draw_battery_icon(gfx, 204, 5, current_batt);

    // Main Card 1: KEYBOARD (x: 4, y: 23, w: 114, h: 93)
    draw_keyboard_card(gfx, 4, 23, 114, 93, kbd_active, last_key_name, last_key_code, last_kbd_mod);

    // Main Card 2: MOUSE (x: 122, y: 23, w: 114, h: 93)
    int32_t show_dx = mouse_moving ? live_display_mouse_dx : 0;
    int32_t show_dy = mouse_moving ? live_display_mouse_dy : 0;
    int32_t show_wheel = (now - last_mouse_wheel_time < 220) ? live_display_mouse_wheel : 0;
    draw_mouse_card(gfx, 122, 23, 114, 93, mouse_moving, left_clicked, right_clicked, mid_clicked,
                    show_dx, show_dy, show_wheel);

    // Bottom Status Bar (y: 118..134)
    gfx.fillRect(0, 117, 240, 18, TFT_BLACK);
    gfx.drawFastHLine(0, 117, 240, 0x2125);

    char pkt_str[32];
    snprintf(pkt_str, sizeof(pkt_str), "PKTS: %lu", (unsigned long)(key_count + total_mouse_packets));
    gfx.setTextColor(0x07FF, TFT_BLACK);
    gfx.drawString(pkt_str, 6, 122);

    gfx.setTextColor(0x632C, TFT_BLACK);
    gfx.drawRightString("Dell 413C:2501", 234, 122);

    if (s_has_canvas) {
        s_canvas.pushSprite(0, 0);
    } else {
        M5.Display.endWrite();
    }
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

    if (s_canvas.createSprite(240, 135)) {
        s_has_canvas = true;
        ESP_LOGI(TAG, "M5Canvas 240x135 initialized successfully for tear-free rendering.");
    } else {
        ESP_LOGW(TAG, "M5Canvas allocation failed; using direct display rendering.");
    }

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

            last_kbd_event_time = esp_log_timestamp();
            last_kbd_is_local = true;
            last_key_code = keyEvent.keyCode;
            last_kbd_mod = mod;
            last_key_name = keyEvent.keyName ? keyEvent.keyName : get_key_display_string(mod, keyEvent.keyCode);
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

        // Dynamic adaptive UI refresh (25Hz during motion/input, 5Hz when idle)
        static uint32_t last_ui_tick = 0;
        uint32_t now = esp_log_timestamp();
        bool is_animating = (now - last_kbd_event_time < 250) ||
                            (now - last_mouse_move_time < 250) ||
                            (now - last_mouse_click_time < 250) ||
                            (now - last_mouse_wheel_time < 250);
        uint32_t interval = is_animating ? 40 : 200;

        if (now - last_ui_tick >= interval) {
            last_ui_tick = now;
            update_display();
        }

        vTaskDelay(pdMS_TO_TICKS(5));
    }
}


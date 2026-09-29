/*
 * Cardputer-KVM — Wireless Transmitter
 * Target Hardware: ESP32-S3 N16R8 (16MB Flash, 8MB Octal PSRAM)
 *
 * Features:
 *   - ESP-NOW 2.4 GHz wireless transmitter on Channel 1 (ultra-low latency)
 *   - Automatic broadcast pairing (beams to Cardputer-ADV receiver)
 *   - Interactive Serial Console: Type in terminal -> Beams wirelessly to Cardputer -> Injects to PC!
 *   - BOOT Button (GPIO 0): Press to inject a test string
 *   - Heartbeat Ping: Beams uptime every 2s to maintain RF Link indicator on Cardputer screen
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
#include "driver/gpio.h"
#include "driver/uart.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include "kvm_protocol.h"

static const char *TAG = "KVM_TRANSMITTER";

#define BOOT_BUTTON_PIN GPIO_NUM_0

static const uint8_t broadcast_mac[6] = { 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF };
static uint16_t packet_seq = 0;

/***********************************************************************
 * 1. ESP-NOW Send Helpers
 ***********************************************************************/

static void on_espnow_sent(const uint8_t *mac_addr, esp_now_send_status_t status)
{
    ESP_LOGD(TAG, "Packet sent status: %s", status == ESP_NOW_SEND_SUCCESS ? "SUCCESS" : "FAIL");
}

static void send_raw_packet(const void *data, size_t len)
{
    esp_err_t result = esp_now_send(broadcast_mac, (const uint8_t*)data, len);
    if (result != ESP_OK) {
        ESP_LOGE(TAG, "Failed to send ESP-NOW packet: %s", esp_err_to_name(result));
    }
}

static void send_key_report(uint8_t modifiers, uint8_t keycode)
{
    kvm_key_packet_t pkt;
    pkt.magic = KVM_ESPNOW_MAGIC;
    pkt.pkt_type = KVM_PKT_KEY_REPORT;
    pkt.modifiers = modifiers;
    memset(pkt.keycodes, 0, sizeof(pkt.keycodes));
    pkt.keycodes[0] = keycode;
    pkt.seq_num = packet_seq++;

    send_raw_packet(&pkt, sizeof(pkt));
}

// Send press + release
static void send_hid_keystroke(uint8_t modifiers, uint8_t keycode)
{
    send_key_report(modifiers, keycode);
    vTaskDelay(pdMS_TO_TICKS(20));

    // Release
    send_key_report(0, 0);
    vTaskDelay(pdMS_TO_TICKS(20));
}

// Convert ASCII char to HID keycode + modifier
static void send_ascii_char(char c)
{
    uint8_t modifier = 0;
    uint8_t key = 0;

    if (c >= 'a' && c <= 'z') {
        key = 0x04 + (c - 'a');
    } else if (c >= 'A' && c <= 'Z') {
        key = 0x04 + (c - 'A');
        modifier = 0x02; // Left Shift
    } else if (c >= '1' && c <= '9') {
        key = 0x1E + (c - '1');
    } else if (c == '0') {
        key = 0x27;
    } else if (c == ' ') {
        key = 0x2C;
    } else if (c == '\n' || c == '\r') {
        key = 0x28; // Enter
    } else if (c == '\b' || c == 0x7F) {
        key = 0x2A; // Backspace
    } else if (c == '\t') {
        key = 0x2B; // Tab
    } else if (c == '!') {
        key = 0x1E; modifier = 0x02;
    } else if (c == '@') {
        key = 0x1F; modifier = 0x02;
    } else if (c == '#') {
        key = 0x20; modifier = 0x02;
    } else if (c == '$') {
        key = 0x21; modifier = 0x02;
    } else if (c == '%') {
        key = 0x22; modifier = 0x02;
    } else if (c == '^') {
        key = 0x23; modifier = 0x02;
    } else if (c == '&') {
        key = 0x24; modifier = 0x02;
    } else if (c == '*') {
        key = 0x25; modifier = 0x02;
    } else if (c == '(') {
        key = 0x26; modifier = 0x02;
    } else if (c == ')') {
        key = 0x27; modifier = 0x02;
    } else if (c == '-') {
        key = 0x2D;
    } else if (c == '_') {
        key = 0x2D; modifier = 0x02;
    } else if (c == '=') {
        key = 0x2E;
    } else if (c == '+') {
        key = 0x2E; modifier = 0x02;
    } else if (c == '[') {
        key = 0x2F;
    } else if (c == '{') {
        key = 0x2F; modifier = 0x02;
    } else if (c == ']') {
        key = 0x30;
    } else if (c == '}') {
        key = 0x30; modifier = 0x02;
    } else if (c == ';') {
        key = 0x33;
    } else if (c == ':') {
        key = 0x33; modifier = 0x02;
    } else if (c == '\'') {
        key = 0x34;
    } else if (c == '"') {
        key = 0x34; modifier = 0x02;
    } else if (c == ',') {
        key = 0x36;
    } else if (c == '<') {
        key = 0x36; modifier = 0x02;
    } else if (c == '.') {
        key = 0x37;
    } else if (c == '>') {
        key = 0x37; modifier = 0x02;
    } else if (c == '/') {
        key = 0x38;
    } else if (c == '?') {
        key = 0x38; modifier = 0x02;
    }

    if (key != 0) {
        send_hid_keystroke(modifier, key);
    }
}

static void send_string(const char *str)
{
    ESP_LOGI(TAG, "Beaming string over ESP-NOW: \"%s\"", str);
    while (*str) {
        send_ascii_char(*str++);
    }
}

/***********************************************************************
 * 2. Background Tasks
 ***********************************************************************/

// Heartbeat task: keeps link active on Cardputer display
static void heartbeat_task(void *pvParameters)
{
    while (1) {
        kvm_heartbeat_packet_t hb;
        hb.magic = KVM_ESPNOW_MAGIC;
        hb.pkt_type = KVM_PKT_HEARTBEAT;
        hb.uptime_ms = esp_log_timestamp();
        hb.seq_num = packet_seq++;

        send_raw_packet(&hb, sizeof(hb));
        vTaskDelay(pdMS_TO_TICKS(2000));
    }
}

#define UART_PORT UART_NUM_0
#define UART_BUF_SIZE 512

static void init_uart()
{
    uart_config_t uart_config = {
        .baud_rate = 115200,
        .data_bits = UART_DATA_8_BITS,
        .parity    = UART_PARITY_DISABLE,
        .stop_bits = UART_STOP_BITS_1,
        .flow_ctrl = UART_HW_FLOWCTRL_DISABLE,
        .source_clk = UART_SCLK_DEFAULT,
    };
    uart_driver_install(UART_PORT, UART_BUF_SIZE * 2, 0, 0, NULL, 0);
    uart_param_config(UART_PORT, &uart_config);
}

// High-speed Serial Bridge: PC Client (Serial) -> ESP-NOW -> Cardputer Receiver
static void serial_bridge_task(void *pvParameters)
{
    uint8_t rx_buf[256];
    size_t rx_len = 0;

    ESP_LOGI(TAG, "Serial KVM Bridge active on UART0 (115200 baud)");

    while (1) {
        int len = uart_read_bytes(UART_PORT, rx_buf + rx_len, sizeof(rx_buf) - rx_len, pdMS_TO_TICKS(10));
        if (len > 0) {
            rx_len += len;

            while (rx_len >= 2) {
                uint16_t magic = *(uint16_t*)rx_buf;
                if (magic == KVM_ESPNOW_MAGIC) {
                    if (rx_len < 3) break;
                    uint8_t pkt_type = rx_buf[2];
                    size_t pkt_size = 0;

                    if (pkt_type == KVM_PKT_KEY_REPORT) {
                        pkt_size = sizeof(kvm_key_packet_t);
                    } else if (pkt_type == KVM_PKT_MOUSE_MOVE) {
                        pkt_size = sizeof(kvm_mouse_packet_t);
                    } else if (pkt_type == KVM_PKT_HEARTBEAT) {
                        pkt_size = sizeof(kvm_heartbeat_packet_t);
                    } else {
                        // Unknown packet type, discard 1 byte to realign
                        rx_len--;
                        memmove(rx_buf, rx_buf + 1, rx_len);
                        continue;
                    }

                    if (rx_len < pkt_size) {
                        // Wait for complete frame
                        break;
                    }

                    // Beam packet immediately over ESP-NOW!
                    send_raw_packet(rx_buf, pkt_size);

                    rx_len -= pkt_size;
                    if (rx_len > 0) {
                        memmove(rx_buf, rx_buf + pkt_size, rx_len);
                    }
                } else {
                    // Raw ASCII typing fallback (e.g. typing in terminal)
                    char c = (char)rx_buf[0];
                    if (c > 0) {
                        send_ascii_char(c);
                    }
                    rx_len--;
                    if (rx_len > 0) {
                        memmove(rx_buf, rx_buf + 1, rx_len);
                    }
                }
            }
        }
    }
}

/***********************************************************************
 * 3. Initialization & Main
 ***********************************************************************/

static void init_espnow_transmitter()
{
    ESP_LOGI(TAG, "Initializing NVS and Wi-Fi...");
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));
    ESP_ERROR_CHECK(esp_wifi_set_storage(WIFI_STORAGE_RAM));
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_start());
    ESP_ERROR_CHECK(esp_wifi_set_channel(KVM_ESPNOW_CHANNEL, WIFI_SECOND_CHAN_NONE));

    ESP_LOGI(TAG, "Initializing ESP-NOW...");
    ESP_ERROR_CHECK(esp_now_init());
    ESP_ERROR_CHECK(esp_now_register_send_cb(on_espnow_sent));

    // Register broadcast peer
    esp_now_peer_info_t peer_info = {};
    memcpy(peer_info.peer_addr, broadcast_mac, 6);
    peer_info.channel = KVM_ESPNOW_CHANNEL;
    peer_info.ifidx = WIFI_IF_STA;
    peer_info.encrypt = false;
    ESP_ERROR_CHECK(esp_now_add_peer(&peer_info));

    ESP_LOGI(TAG, "ESP-NOW Transmitter ready on Channel %d (Broadcast: FF:FF:FF:FF:FF:FF)", KVM_ESPNOW_CHANNEL);
}

void app_main(void)
{
    ESP_LOGI(TAG, "=== Cardputer-KVM Transmitter (ESP32-S3 N16R8) ===");

    // 1. Initialize UART0 Serial Bridge
    init_uart();

    // 2. Configure BOOT Button (GPIO 0)
    const gpio_config_t btn_cfg = {
        .pin_bit_mask = BIT64(BOOT_BUTTON_PIN),
        .mode = GPIO_MODE_INPUT,
        .pull_up_en = GPIO_PULLUP_ENABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type = GPIO_INTR_DISABLE,
    };
    ESP_ERROR_CHECK(gpio_config(&btn_cfg));

    // 3. Initialize ESP-NOW
    init_espnow_transmitter();

    // 4. Start Heartbeat & Serial Bridge Tasks
    xTaskCreate(heartbeat_task, "kvm_heartbeat", 3072, NULL, 5, NULL);
    xTaskCreate(serial_bridge_task, "kvm_serial_bridge", 4096, NULL, 5, NULL);

    // 5. Main Button Polling Loop
    bool last_btn_state = true;

    while (1) {
        bool current_btn = gpio_get_level(BOOT_BUTTON_PIN);
        if (last_btn_state && !current_btn) {
            // Button pressed!
            ESP_LOGI(TAG, "BOOT button pressed! Beaming greeting packet...");
            send_string("[Hello from ESP32-S3 N16R8 over ESP-NOW!]\n");
            vTaskDelay(pdMS_TO_TICKS(300)); // Debounce
        }
        last_btn_state = current_btn;

        vTaskDelay(pdMS_TO_TICKS(50));
    }
}

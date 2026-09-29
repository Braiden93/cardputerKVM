/*
 * SPDX-FileCopyrightText: 2022-2024 Espressif Systems (Shanghai) CO LTD
 *
 * SPDX-License-Identifier: Unlicense OR CC0-1.0
 */

#include "tusb_hid_device_helper.h"

#include <stdlib.h>
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "tinyusb.h"
#include "class/hid/hid_device.h"
#include "driver/gpio.h"
#include "esp_rom_sys.h"

#define APP_BUTTON (GPIO_NUM_0)  // Use BOOT signal by default
static const char *TAG = "tusb_hid";

/************* TinyUSB descriptors ****************/

enum {
    ITF_NUM_KEYBOARD = 0,
    ITF_NUM_MOUSE    = 1,
    ITF_NUM_TOTAL    = 2
};

#define EPNUM_KEYBOARD 0x81
#define EPNUM_MOUSE    0x82

#define TUSB_DESC_TOTAL_LEN (TUD_CONFIG_DESC_LEN + 2 * TUD_HID_DESC_LEN)

/**
 * @brief Dedicated HID report descriptors
 *
 * Dedicated standard Boot Keyboard (Interface 0)
 * Dedicated standard Boot Mouse (Interface 1)
 */
const uint8_t desc_hid_keyboard_report[] = {
    TUD_HID_REPORT_DESC_KEYBOARD()
};

const uint8_t desc_hid_mouse_report[] = {
    TUD_HID_REPORT_DESC_MOUSE()
};

// Dell Wireless Keyboard & Mouse Combo Dongle Device Descriptor
static const tusb_desc_device_t dell_device_descriptor = {
    .bLength            = sizeof(tusb_desc_device_t),
    .bDescriptorType    = TUSB_DESC_DEVICE,
    .bcdUSB             = 0x0200,
    .bDeviceClass       = 0x00,
    .bDeviceSubClass    = 0x00,
    .bDeviceProtocol    = 0x00,
    .bMaxPacketSize0    = CFG_TUD_ENDPOINT0_SIZE,
    .idVendor           = 0x413C, // Dell Inc.
    .idProduct          = 0x2501, // Dell Keyboard and mouse dongle (KM636 / KM714 Wireless Combo)
    .bcdDevice          = 0x0100, // Hardware revision 1.00
    .iManufacturer      = 0x01,   // "Dell"
    .iProduct           = 0x02,   // "Wireless Receiver"
    .iSerialNumber      = 0x00,   // None (standard for genuine dongles)
    .bNumConfigurations = 0x01
};

/**
 * @brief String descriptors
 */
static const char *hid_string_descriptor[] = {
    (char[]){0x09, 0x04}, // 0: English (0x0409)
    "Dell",               // 1: Manufacturer
    "Wireless Receiver",  // 2: Product
};

/**
 * @brief Configuration descriptor
 *
 * Composite configuration with 2 distinct HID interfaces:
 * - Interface 0: Keyboard (Boot Protocol Keyboard, 1000Hz polling)
 * - Interface 1: Mouse (Boot Protocol Mouse, 1000Hz polling)
 */
static const uint8_t hid_configuration_descriptor[] = {
    // Config number, interface count (2), string index (0), total length, attribute, power in mA (98mA)
    TUD_CONFIG_DESCRIPTOR(1, ITF_NUM_TOTAL, 0, TUSB_DESC_TOTAL_LEN, TUSB_DESC_CONFIG_ATT_REMOTE_WAKEUP, 98),

    // Interface 0: Keyboard (bInterfaceSubClass=1 BOOT, bInterfaceProtocol=1 KEYBOARD, 1ms polling)
    TUD_HID_DESCRIPTOR(ITF_NUM_KEYBOARD, 0, HID_ITF_PROTOCOL_KEYBOARD, sizeof(desc_hid_keyboard_report), EPNUM_KEYBOARD, 16, 1),

    // Interface 1: Mouse (bInterfaceSubClass=1 BOOT, bInterfaceProtocol=2 MOUSE, 1ms polling)
    TUD_HID_DESCRIPTOR(ITF_NUM_MOUSE, 0, HID_ITF_PROTOCOL_MOUSE, sizeof(desc_hid_mouse_report), EPNUM_MOUSE, 16, 1),
};

/********* TinyUSB HID callbacks ***************/

// Invoked when received GET HID REPORT DESCRIPTOR request
uint8_t const *tud_hid_descriptor_report_cb(uint8_t instance)
{
    if (instance == ITF_NUM_KEYBOARD) {
        return desc_hid_keyboard_report;
    } else if (instance == ITF_NUM_MOUSE) {
        return desc_hid_mouse_report;
    }
    return NULL;
}

// Invoked when received GET_REPORT control request
uint16_t tud_hid_get_report_cb(uint8_t instance, uint8_t report_id, hid_report_type_t report_type, uint8_t *buffer,
                               uint16_t reqlen)
{
    (void)instance;
    (void)report_id;
    (void)report_type;
    (void)buffer;
    (void)reqlen;
    return 0;
}

// Invoked when received SET_REPORT control request
void tud_hid_set_report_cb(uint8_t instance, uint8_t report_id, hid_report_type_t report_type, uint8_t const *buffer,
                           uint16_t bufsize)
{
    (void)instance;
    (void)report_id;
    (void)report_type;
    (void)buffer;
    (void)bufsize;
}

/********* Application ***************/

typedef enum {
    MOUSE_DIR_RIGHT,
    MOUSE_DIR_DOWN,
    MOUSE_DIR_LEFT,
    MOUSE_DIR_UP,
    MOUSE_DIR_MAX,
} mouse_dir_t;

#define DISTANCE_MAX 125
#define DELTA_SCALAR 5

#if 0
static void mouse_draw_square_next_delta(int8_t *delta_x_ret, int8_t *delta_y_ret)
{
    static mouse_dir_t cur_dir = MOUSE_DIR_RIGHT;
    static uint32_t distance   = 0;

    // Calculate next delta
    if (cur_dir == MOUSE_DIR_RIGHT) {
        *delta_x_ret = DELTA_SCALAR;
        *delta_y_ret = 0;
    } else if (cur_dir == MOUSE_DIR_DOWN) {
        *delta_x_ret = 0;
        *delta_y_ret = DELTA_SCALAR;
    } else if (cur_dir == MOUSE_DIR_LEFT) {
        *delta_x_ret = -DELTA_SCALAR;
        *delta_y_ret = 0;
    } else if (cur_dir == MOUSE_DIR_UP) {
        *delta_x_ret = 0;
        *delta_y_ret = -DELTA_SCALAR;
    }

    // Update cumulative distance for current direction
    distance += DELTA_SCALAR;
    // Check if we need to change direction
    if (distance >= DISTANCE_MAX) {
        distance = 0;
        cur_dir++;
        if (cur_dir == MOUSE_DIR_MAX) {
            cur_dir = 0;
        }
    }
}

static void app_send_hid_demo(void)
{
    // Keyboard output: Send key 'a/A' pressed and released
    ESP_LOGI(TAG, "Sending Keyboard report");
    uint8_t keycode[6] = {HID_KEY_A};
    tud_hid_keyboard_report(HID_ITF_PROTOCOL_KEYBOARD, 0, keycode);
    vTaskDelay(pdMS_TO_TICKS(50));
    tud_hid_keyboard_report(HID_ITF_PROTOCOL_KEYBOARD, 0, NULL);

    // Mouse output: Move mouse cursor in square trajectory
    ESP_LOGI(TAG, "Sending Mouse report");
    int8_t delta_x;
    int8_t delta_y;
    for (int i = 0; i < (DISTANCE_MAX / DELTA_SCALAR) * 4; i++) {
        // Get the next x and y delta in the draw square pattern
        mouse_draw_square_next_delta(&delta_x, &delta_y);
        tud_hid_mouse_report(HID_ITF_PROTOCOL_MOUSE, 0x00, delta_x, delta_y, 0, 0);
        vTaskDelay(pdMS_TO_TICKS(20));
    }
}
#endif

static void _demo_app_main(void)
{
    // // Initialize button that will trigger HID reports
    // const gpio_config_t boot_button_config = {
    //     .pin_bit_mask = BIT64(APP_BUTTON),
    //     .mode         = GPIO_MODE_INPUT,
    //     .intr_type    = GPIO_INTR_DISABLE,
    //     .pull_up_en   = true,
    //     .pull_down_en = false,
    // };
    // ESP_ERROR_CHECK(gpio_config(&boot_button_config));

    ESP_LOGI(TAG, "USB initialization (Dell Wireless Dongle 413c:2501)");
    const tinyusb_config_t tusb_cfg = {
        .device_descriptor       = &dell_device_descriptor,
        .string_descriptor       = hid_string_descriptor,
        .string_descriptor_count = sizeof(hid_string_descriptor) / sizeof(hid_string_descriptor[0]),
        .external_phy            = false,
#if (TUD_OPT_HIGH_SPEED)
        .fs_configuration_descriptor = hid_configuration_descriptor,
        .hs_configuration_descriptor = hid_configuration_descriptor,
        .qualifier_descriptor        = NULL,
#else
        .configuration_descriptor = hid_configuration_descriptor,
#endif  // TUD_OPT_HIGH_SPEED
    };

    ESP_ERROR_CHECK(tinyusb_driver_install(&tusb_cfg));
    ESP_LOGI(TAG, "USB initialization DONE");
}

void tusb_hid_device_helper_init(void)
{
    _demo_app_main();
}

bool tusb_hid_device_helper_ready(void)
{
    return tud_mounted() && (tud_hid_n_ready(ITF_NUM_KEYBOARD) || tud_hid_n_ready(ITF_NUM_MOUSE));
}

bool tusb_hid_device_helper_report(uint8_t modifier, uint8_t *keycode)
{
    if (!tud_mounted()) return false;
    int timeout_us = 5000;
    while (!tud_hid_n_ready(ITF_NUM_KEYBOARD) && timeout_us > 0) {
        esp_rom_delay_us(10);
        timeout_us -= 10;
    }
    return tud_hid_n_keyboard_report(ITF_NUM_KEYBOARD, 0, modifier, keycode);
}

bool tusb_hid_device_helper_mouse_report(uint8_t buttons, int8_t x, int8_t y, int8_t vertical)
{
    if (!tud_mounted()) return false;
    int timeout_us = 2000;
    while (!tud_hid_n_ready(ITF_NUM_MOUSE) && timeout_us > 0) {
        esp_rom_delay_us(10);
        timeout_us -= 10;
    }
    return tud_hid_n_mouse_report(ITF_NUM_MOUSE, 0, buttons, x, y, vertical, 0);
}

bool tusb_hid_device_helper_mouse_report_nowait(uint8_t buttons, int8_t x, int8_t y, int8_t vertical)
{
    if (!tud_mounted() || !tud_hid_n_ready(ITF_NUM_MOUSE)) return false;
    return tud_hid_n_mouse_report(ITF_NUM_MOUSE, 0, buttons, x, y, vertical, 0);
}

bool tusb_hid_device_helper_is_mounted(void)
{
    return tud_mounted();
}

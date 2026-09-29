/*
 * SPDX-FileCopyrightText: 2025 M5Stack Technology CO LTD
 *
 * SPDX-License-Identifier: MIT
 */
// https://github.com/espressif/esp-idf/blob/v5.4.2/examples/peripherals/usb/device/tusb_hid
#pragma once
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void tusb_hid_device_helper_init(void);
bool tusb_hid_device_helper_ready(void);
bool tusb_hid_device_helper_report(uint8_t modifier, uint8_t* keycode);
bool tusb_hid_device_helper_mouse_report(uint8_t buttons, int8_t x, int8_t y, int8_t vertical);
bool tusb_hid_device_helper_mouse_report_nowait(uint8_t buttons, int8_t x, int8_t y, int8_t vertical);
bool tusb_hid_device_helper_is_mounted(void);

#ifdef __cplusplus
}
#endif

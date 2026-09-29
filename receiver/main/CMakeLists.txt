file(GLOB_RECURSE HAL_SRCS
    "hal/*.c"
    "hal/*.cpp"
)

idf_component_register(
    SRCS "main.cpp" ${HAL_SRCS}
    INCLUDE_DIRS "." "hal" "hal/keyboard" "hal/utils/adafruit_tca8418" "hal/utils/tusb_hid_device"
    REQUIRES M5Unified M5GFX mooncake_log esp_tinyusb esp_wifi nvs_flash
)

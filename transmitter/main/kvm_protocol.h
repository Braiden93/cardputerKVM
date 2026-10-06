#pragma once
#include <stdint.h>
#include <stdbool.h>

#define KVM_ESPNOW_CHANNEL 1
#define KVM_ESPNOW_MAGIC   0x4B56 // "KV"

typedef enum {
    KVM_PKT_KEY_REPORT  = 0x01, // Keyboard press / release report
    KVM_PKT_MOUSE_MOVE  = 0x02, // Mouse move / click report
    KVM_PKT_HEARTBEAT   = 0x03, // Connection check / ping
    KVM_PKT_PING_REQ    = 0x04, // Handshake ping request from PC Client
    KVM_PKT_PING_RESP   = 0x05, // Identity ping response from Transmitter
} kvm_packet_type_t;

// Standard 6KRO USB-compliant Key Packet
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_KEY_REPORT
    uint8_t  modifiers;     // Bitmask: Ctrl, Shift, Alt, GUI
    uint8_t  keycodes[6];   // Active HID keycodes (e.g. 0x04 for 'a')
    uint16_t seq_num;       // Sequence number
} kvm_key_packet_t;

// Mouse Movement & Click Packet
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_MOUSE_MOVE (0x02)
    uint8_t  buttons;       // Bitmask: Bit 0 = Left, Bit 1 = Right, Bit 2 = Middle
    int8_t   dx;            // X displacement (-127 to 127)
    int8_t   dy;            // Y displacement (-127 to 127)
    int8_t   wheel;         // Vertical scroll (-127 to 127)
    uint16_t seq_num;       // Sequence number
} kvm_mouse_packet_t;

// Heartbeat Packet
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC
    uint8_t  pkt_type;      // KVM_PKT_HEARTBEAT
    uint32_t uptime_ms;     // Transmitter uptime in ms
    uint16_t seq_num;
} kvm_heartbeat_packet_t;

// Handshake Ping Request Packet (PC Client -> Transmitter)
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_PING_REQ (0x04)
    uint16_t seq_num;       // Sequence number
} kvm_ping_req_packet_t;

// Identity Response Packet (Transmitter -> PC Client)
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_PING_RESP (0x05)
    uint8_t  mac[6];        // Transmitter Wi-Fi STA MAC
    char     device_id[16]; // "KVM_TRANSMITTER" (null-terminated)
    uint16_t seq_num;
} kvm_ping_resp_packet_t;


# Cardputer-KVM Security & Pairing Plan (`security-patch`)

This document defines the cryptographic architecture, threat model, push-button pairing protocol, and implementation roadmap for securing the RF link between the **Transmitter (ESP32-S3)** and the **Receiver (Cardputer-ADV)**.

---

## 1. Threat Model & Motivations

### Current Architecture Vulnerabilities (Baseline)
Currently, Cardputer-KVM operates in **open broadcast / promiscuous mode**:
1. **Transmitter Broadcast:** The transmitter sends packets addressed to `FF:FF:FF:FF:FF:FF` on Channel 1 without encryption (`peer.encrypt = false`).
2. **Promiscuous Receiver:** The Cardputer receiver accepts any valid KVM packet with matching magic `0x4B56` and automatically registers the sender's MAC as its peer.
3. **Eavesdropping Risk:** Anyone with an ESP32 or Wi-Fi card in monitor mode can capture plain-text keystrokes (passwords, SSH keys, private messages) and mouse coordinates.
4. **Injection / Hijacking Risk:** Any attacker broadcasting forged `KVM_PKT_KEY_REPORT` packets can execute arbitrary keystroke injection (wireless BadUSB) on the host computer without physical access.
5. **Replay Attacks:** Captured legitimate key packets can be retransmitted to repeat sensitive actions.

### Security Goals
* **Confidentiality:** Keystrokes and mouse movements must be encrypted over the air using hardware AES-128.
* **Integrity & Authenticity:** Keystroke injection from rogue devices must be rejected at the PHY/MAC layer.
* **Anti-Replay:** Monotonic sequence counter validation must drop retransmitted frames.
* **Zero-Friction Physical Pairing:** Proximity-based, two-button physical handshake (no manual hex key entry, no Wi-Fi credentials).
* **Non-Volatile Persistence:** Paired keys and MAC addresses persist across reboots in ESP32 Flash NVS.
* **Safe Fallback & Factory Reset:** Dedicated physical gesture to wipe keys and return to pairing mode.

---

## 2. Push-Button Pairing Workflow

```
+-----------------------------------------------------------------------------------+
|                              PHYSICAL USER ACTION                                 |
|                                                                                   |
|   1. Hold Cardputer 'BtnG0' (2s)          2. Press Transmitter 'BOOT' Button     |
|      (or choose 'Pair' in menu)              (or click "Pair" in PC GUI)          |
+-----------------------------------------------------------------------------------+
                                       |
                                       v
+------------------------+                           +------------------------+
|     Cardputer-ADV      |                           |   ESP32-S3 Transmitter |
|       (Receiver)       |                           |        (Dongle)        |
+------------------------+                           +------------------------+
           |                                                      |
           |-- Enters 15s Pairing Window                          |-- Enters Pairing Mode
           |   (ST7789 displays countdown)                        |
           |                                                      |
           |                 KVM_PKT_PAIR_REQ                     |
           |<-----------------------------------------------------|
           |   (Src: TX_MAC, Nonce_Tx, Pairing Token)             |
           |                                                      |
           |-- Verifies window is active                          |
           |-- Generates Nonce_Rx                                 |
           |-- Derives AES-128 LMK (HMAC-SHA256)                  |
           |                                                      |
           |                 KVM_PKT_PAIR_ACK                     |
           |----------------------------------------------------->|
           |   (Src: RX_MAC, Nonce_Rx, Auth Hash)                 |
           |                                                      |
           |                                                      |-- Derives identical LMK
           |                                                      |-- Stores RX_MAC & LMK in NVS
           |                                                      |-- Configures encrypted peer
           |                                                      |
           |               KVM_PKT_PAIR_CONFIRM                   |
           |<-----------------------------------------------------|
           |   (Encrypted test ping using new LMK)                |
           |                                                      |
           |-- Confirms encrypted ping received                   |
           |-- Stores TX_MAC & LMK in NVS                         |
           |-- Disables broadcast promiscuity                     |
           |-- LCD shows "PAIRED TO XX:XX:XX:XX:XX:XX"            |
           |                                                      |
           |======================================================|
           |   NORMAL SECURE OPERATION (Unicast AES-128-CCMP)     |
           |======================================================|
```

### Detailed User Step-by-Step
1. **Initiate on Cardputer (Receiver):**
   - User holds `BtnG0` (`M5.BtnA`) for **2 seconds**.
   - The ST7789 display displays an overlay modal:
     - Header: `[ PAIRING MODE ]` (Yellow accent)
     - Progress: Animated 15-second countdown timer bar.
     - Text: `Press BOOT button on Dongle to pair...`
     - Cancel: Quick press `BtnG0` cancels pairing and returns to current state.
2. **Initiate on Transmitter (Dongle):**
   - User presses the physical **`BOOT` button** (GPIO 0) on the transmitter dongle (or clicks **"Pair Receiver"** in the PC Client GUI).
   - Dongle status LED blinks rapidly (5Hz).
3. **Automatic Cryptographic Exchange:**
   - The transmitter transmits a `KVM_PKT_PAIR_REQ` beacon.
   - The Cardputer validates that the 15-second pairing window is active.
   - Both devices exchange nonces and compute a unique 128-bit **Local Master Key (LMK)**.
   - Both devices save the paired peer's MAC and LMK to NVS storage.
4. **Visual Confirmation:**
   - Cardputer screen flashes **Green**: `PAIRED: [AA:BB:CC:DD:EE:FF]`.
   - Normal KVM HUD returns, with an encrypted lock badge icon `[LOCKED]` displayed next to the RF status badge.
5. **Subsequent Boots:**
   - Both devices boot, read the paired MAC and LMK from NVS, and immediately establish encrypted unicast communication without requiring re-pairing.

---

## 3. Cryptographic Specification

### ESP-NOW Native Hardware Security (AES-128-CCMP)
ESP-NOW includes native support for IEEE 802.11i CCMP encryption using the ESP32-S3's hardware crypto engine:
- **PMK (Primary Master Key):** 16 bytes. Used to encrypt the Local Master Key during over-the-air peer setup (ESP-NOW internal requirement). Configured via `esp_now_set_pmk()`.
- **LMK (Local Master Key):** 16 bytes. Assigned per peer in `esp_now_peer_info_t.lmk`. All subsequent payload frames sent to this peer are encrypted using CCMP (Counter Mode with CBC-MAC).
- **Overhead:** AES-128 encryption and decryption are executed entirely in hardware, adding **<15 microseconds** of latency per frame (virtually zero overhead).

### Key Derivation Protocol
To ensure that keys are unique to each pairing session and not hardcoded across builds:
$$\text{LMK} = \text{Truncate}_{128}\Big(\text{HMAC-SHA256}\big(\text{MasterSalt},\; \text{MAC}_{\text{TX}} \parallel \text{MAC}_{\text{RX}} \parallel \text{Nonce}_{\text{TX}} \parallel \text{Nonce}_{\text{RX}}\big)\Big)$$

- **`MasterSalt`**: 32-byte secret seed shared by firmware builds (can be customized via Kconfig or compile-time define).
- **`Nonce_Tx`**: 16-byte cryptographically secure random number generated by `esp_fill_random()` on the transmitter.
- **`Nonce_Rx`**: 16-byte cryptographically secure random number generated by `esp_fill_random()` on the receiver.
- **Result:** An eavesdropper intercepting the pairing handshake cannot derive the LMK without physical access to the pre-shared salt, and any third party cannot inject packets into the resulting session.

### Anti-Replay Architecture
Every encrypted packet carries a 32-bit monotonically increasing sequence number:
```c
uint32_t seq_num;
```
1. Receiver tracks `last_valid_seq_num` initialized to 0 upon pairing/boot.
2. For every incoming packet:
   - If `pkt->seq_num <= last_valid_seq_num`: The packet is dropped immediately and logged as a replay attempt.
   - If `pkt->seq_num > last_valid_seq_num`: The packet is processed, and `last_valid_seq_num = pkt->seq_num`.
3. Sequence window protection handles minor packet reordering if multi-packet bursts are enabled.

---

## 4. Protocol Extensions (`kvm_protocol.h`)

### New Packet Types
```c
typedef enum {
    KVM_PKT_KEY_REPORT    = 0x01,
    KVM_PKT_MOUSE_MOVE    = 0x02,
    KVM_PKT_HEARTBEAT     = 0x03,
    KVM_PKT_PING_REQ      = 0x04,
    KVM_PKT_PING_RESP     = 0x05,
    KVM_PKT_TELEMETRY     = 0x06,
    // Security & Pairing Extensions
    KVM_PKT_PAIR_REQ      = 0x08, // Transmitter -> Receiver (Pairing Request)
    KVM_PKT_PAIR_ACK      = 0x09, // Receiver -> Transmitter (Pairing Acknowledgment)
    KVM_PKT_PAIR_CONFIRM  = 0x0A, // Transmitter -> Receiver (Encrypted Verification)
    KVM_PKT_UNPAIR        = 0x0B, // Either side -> Peer (Revoke Pairing)
} kvm_packet_type_t;
```

### Packet Structures
```c
// Pairing Request Packet (Transmitter -> Receiver)
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_PAIR_REQ (0x08)
    uint8_t  tx_mac[6];     // Transmitter MAC address
    uint8_t  tx_nonce[16];  // Random 128-bit salt from transmitter
    uint32_t timestamp;     // Transmitter uptime in ms
    uint16_t checksum;      // CRC16 verification
} kvm_pair_req_packet_t;

// Pairing ACK Packet (Receiver -> Transmitter)
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_PAIR_ACK (0x09)
    uint8_t  rx_mac[6];     // Receiver MAC address
    uint8_t  rx_nonce[16];  // Random 128-bit salt from receiver
    uint8_t  auth_tag[16];  // HMAC-SHA256 verification tag
    uint32_t session_id;    // Unique 32-bit session token
    uint16_t checksum;      // CRC16 verification
} kvm_pair_ack_packet_t;

// Pairing Confirmation / Unpair Packet
typedef struct __attribute__((packed)) {
    uint16_t magic;         // KVM_ESPNOW_MAGIC (0x4B56)
    uint8_t  pkt_type;      // KVM_PKT_PAIR_CONFIRM or KVM_PKT_UNPAIR
    uint32_t session_id;    // Must match session token
    uint32_t seq_num;       // Initial encrypted sequence counter
} kvm_pair_confirm_packet_t;
```

---

## 5. Non-Volatile Storage (NVS) Schema

Both devices maintain persistent security configuration in an NVS namespace named `"kvm_sec"`:

| Key Name | Type | Size | Description |
| :--- | :--- | :--- | :--- |
| `paired` | `uint8_t` | 1 byte | `1` = Paired & Encrypted, `0` = Unpaired / Open |
| `peer_mac` | `blob` | 6 bytes | MAC address of the authorized partner |
| `lmk` | `blob` | 16 bytes | 128-bit AES Local Master Key |
| `seq_rx` | `uint32_t` | 4 bytes | Last confirmed sequence counter (checkpointed periodically) |

### Factory Reset / Unpairing Mechanism
If the user wants to unpair or pair with a different transmitter:
- **Cardputer (Receiver):** Hold `BtnG0` for **10 seconds** until the screen turns Red with `[ NVS CLEARED: UNPAIRED ]`.
- **Transmitter (Dongle):** Hold `BOOT` button for **10 seconds** on boot, or send `UNPAIR` command via PC Client.
- NVS keys in `"kvm_sec"` are erased, and devices return to the unpaired state.

---

## 6. Implementation Roadmap for `security-patch` Branch

### Phase 1: Protocol Definitions & NVS Helpers
- [ ] Add `KVM_PKT_PAIR_*` enum entries and structs to [`receiver/main/kvm_protocol.h`](file:///media/thunder/LinWin/Projectos/Cardputer-KVM/receiver/main/kvm_protocol.h).
- [ ] Create `security_manager.h` and `security_manager.c` in both `receiver` and `transmitter`:
  - `sec_init()`
  - `sec_is_paired()`
  - `sec_get_peer_mac(uint8_t out_mac[6])`
  - `sec_get_lmk(uint8_t out_lmk[16])`
  - `sec_save_pairing(const uint8_t mac[6], const uint8_t lmk[16])`
  - `sec_erase_pairing()`

### Phase 2: Cardputer Receiver Implementation
- [ ] In [`receiver/main/main.cpp`](file:///media/thunder/LinWin/Projectos/Cardputer-KVM/receiver/main/main.cpp):
  - Detect `M5.BtnA.wasHold()` (2 seconds threshold).
  - Implement pairing window timer state machine (15 seconds countdown).
  - Render visual `[ PAIRING ]` modal on ST7789 display.
  - In `on_espnow_recv()`:
    - If in pairing window, handle `KVM_PKT_PAIR_REQ`.
    - Derive LMK, respond with `KVM_PKT_PAIR_ACK`.
    - On receiving `KVM_PKT_PAIR_CONFIRM`, register encrypted peer with `peer.encrypt = true` and `memcpy(peer.lmk, lmk, 16)`.
    - Store in NVS and exit pairing window.
  - Reject all input packets from any MAC other than the stored `peer_mac`.
  - Validate monotonic sequence numbers.

### Phase 3: ESP32-S3 Transmitter Implementation
- [ ] In [`transmitter/main/main.c`](file:///media/thunder/LinWin/Projectos/Cardputer-KVM/transmitter/main/main.c):
  - Update `BOOT` button handler: short-press triggers pairing request beacon burst.
  - Add UART command handler for PC client initiated pairing (`CMD_TRIGGER_PAIRING`).
  - Implement handshake state machine (`PAIR_REQ` -> wait for `PAIR_ACK` -> compute LMK -> send `PAIR_CONFIRM`).
  - Configure `esp_now_add_peer()` with `encrypt = true` and target receiver MAC.
  - Save paired receiver MAC and LMK to NVS.

### Phase 4: PC Client GUI Integration
- [ ] In [`pc-client/pkg/gui/gui.go`](file:///media/thunder/LinWin/Projectos/Cardputer-KVM/pc-client/pkg/gui/gui.go):
  - Add a **"Pair Receiver"** button in settings/toolbar.
  - Add visual security badge:
    - 🔒 `Encrypted (AES-128)` when paired.
    - ⚠️ `Unpaired (Open Broadcast)` when running in legacy/fallback mode.

### Phase 5: Verification & Attack Testing
- [ ] **Monitor Mode Capture:** Run Wireshark 802.11 monitor mode on Channel 1 to verify packets are encrypted and contain no readable HID keycodes.
- [ ] **Replay Attack Test:** Inject previously captured valid packets using a test ESP32; confirm receiver discards them due to sequence number rejection.
- [ ] **Rogue Injection Test:** Send spoofed keystrokes from an unpaired ESP32; confirm receiver ignores them.
- [ ] **Reboot Persistence Test:** Power cycle both devices; confirm immediate reconnect without re-pairing.

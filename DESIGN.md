# P2P Local Chat Application - Technical Design Document

## 1. Overview

A cross-platform local network peer-to-peer chat application built in Go. Devices discover each other via mDNS, establish secure encrypted connections, and exchange text and files. All data persists locally; no central server or cloud storage.

**Core Features:**
- Local network device discovery (mDNS)
- Device naming and identification
- Secure peer-to-peer messaging
- File/image sharing
- Local SQLite storage
- Cross-platform GUI (Windows/macOS/Linux)

---

## 2. Tech Stack

| Component | Technology | Justification |
|-----------|-----------|---------------|
| **Language** | Go 1.21+ | Single binary, cross-platform, lightweight |
| **GUI Framework** | Fyne v2 | Pure Go, native look, simple widget set |
| **Local Discovery** | zeroconf (mDNS) | Zero-config service discovery, standard on all platforms |
| **Networking** | Go stdlib net (TCP) | Simple, direct control over protocol |
| **Key Exchange** | X25519 (ECDH) | Modern, fast, small keys, forward secrecy per session |
| **Signing** | Ed25519 | Fast, compact signatures, pairs naturally with X25519 |
| **Encryption (Messages)** | AES-256-GCM | Fast, authenticated encryption for message/file payloads |
| **Hashing** | SHA-256 | Message integrity and fingerprint verification |
| **Storage** | SQLite3 | Lightweight, serverless, cross-platform |
| **Device ID** | UUID v4 | Unique peer identification |

---

## 3. File Structure

```
p2p-chat/
├── main.go                    # Entry point, initializes app
├── config/
│   └── config.go              # App config, paths, constants
├── crypto/
│   ├── keypair.go             # Ed25519 signing key generation/loading
│   ├── ecdh.go                # X25519 key exchange, session key derivation
│   ├── encryption.go          # AES-256-GCM encryption/decryption
│   └── signatures.go          # Message signing/verification (Ed25519)
├── storage/
│   ├── database.go            # SQLite initialization, migrations
│   ├── peers.go               # Peer CRUD operations
│   ├── messages.go            # Message CRUD operations
│   └── files.go               # File metadata storage
├── discovery/
│   ├── mdns.go                # mDNS broadcasting and listening
│   └── peer_scanner.go        # Periodic discovery polling
├── network/
│   ├── protocol.go            # Message structure definitions
│   ├── connection.go          # Peer connection management
│   ├── client.go              # Outbound connection handler
│   └── server.go              # Inbound connection listener
├── ui/
│   ├── main_window.go         # Main window layout
│   ├── peers_list.go          # Peer discovery/add list
│   ├── chat_window.go         # Chat conversation view
│   ├── file_handler.go        # File picker/preview
│   └── settings.go            # Device naming, fingerprint display
├── models/
│   ├── peer.go                # Peer data structure
│   ├── message.go             # Message data structure
│   └── file.go                # File metadata structure
└── go.mod

Data Storage (OS-dependent):
├── ~/.config/p2p-chat/        # Linux
├── ~/Library/Application Support/p2p-chat/  # macOS
└── %APPDATA%\p2p-chat\        # Windows
    ├── app.db                 # SQLite database
    ├── config.json            # Device name, ID, port (single source of truth)
    ├── keys/
    │   ├── ed25519_private.pem  # Ed25519 signing private key
    │   ├── ed25519_public.pem   # Ed25519 signing public key
    │   └── x25519_private.pem   # X25519 static private key (for ECDH)
    └── files/                   # Downloaded files/images
```

---

## 4. Trust Model

This application uses a **TOFU (Trust On First Use)** model, similar to SSH:

```
1. The first time two peers connect, they exchange public keys over
   plaintext TCP. This initial exchange is vulnerable to MITM.
2. Once exchanged, each peer's public key is stored locally and pinned.
3. All subsequent connections verify the peer's identity against the
   stored key. A key mismatch triggers a warning and blocks the connection.
4. To mitigate MITM during first contact, the UI exposes a
   "Key Fingerprint" (truncated SHA-256 of the public key) that
   users can verify out-of-band (e.g., read aloud, compare screens).
5. The Settings screen shows the local device fingerprint.
6. Each peer in the peer list shows their fingerprint on tap/click.
```

**Key Fingerprint Format:**
```
SHA-256 of Ed25519 public key, displayed as:
  AB12 CD34 EF56 7890 1234 5678 9ABC DEF0
(16 bytes / 32 hex chars, space-separated in groups of 4)
```

---

## 5. Message Structures

### 5.1 Discovery/Handshake Messages

**mDNS Service Advertisement**
```
Service Name: _p2pchat._tcp.local.
Instance Name: {device_name}._p2pchat._tcp.local.
Port: {listening_port}
TXT Records:
  - device_id: {uuid}
  - version: 1
  - key_fingerprint: {sha256(ed25519_public_key)[0:16] as hex}
```

**Initial Connection Handshake** (on TCP connect)
```json
{
  "type": "handshake",
  "device_id": "550e8400-e29b-41d4-a716-446655440000",
  "device_name": "Alice's Laptop",
  "ed25519_public_key": "base64_ed25519_public_key",
  "x25519_public_key": "base64_x25519_ephemeral_public_key",
  "protocol_version": 1,
  "timestamp": 1707298800000,
  "signature": "base64_ed25519_signature"
}
```

**Handshake Response**
```json
{
  "type": "handshake_response",
  "device_id": "660e8400-e29b-41d4-a716-446655440001",
  "device_name": "Bob's Phone",
  "ed25519_public_key": "base64_ed25519_public_key",
  "x25519_public_key": "base64_x25519_ephemeral_public_key",
  "protocol_version": 1,
  "timestamp": 1707298800050,
  "signature": "base64_ed25519_signature"
}
```

After both sides exchange `x25519_public_key` values, each side performs X25519 ECDH to derive a **shared session key**. This key is used for all AES-256-GCM encryption for the lifetime of the TCP connection.

**Version Mismatch Handling:**
```json
{
  "type": "error",
  "code": "version_mismatch",
  "message": "Unsupported protocol version. Expected 1, got 2.",
  "supported_versions": [1],
  "timestamp": 1707298800060
}
```
If `protocol_version` does not match a supported version, the peer sends an error and closes the connection. The UI displays "Incompatible version" next to the peer.

### 5.2 Peer Management Messages

**Add Peer Request** (user clicks "Add")
```json
{
  "type": "peer_add_request",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "from_device_name": "Alice's Laptop",
  "timestamp": 1707298800000,
  "signature": "base64_ed25519_signature"
}
```

**Peer Add Response**
```json
{
  "type": "peer_add_response",
  "status": "accepted",
  "device_id": "660e8400-e29b-41d4-a716-446655440001",
  "device_name": "Bob's Phone",
  "timestamp": 1707298800100,
  "signature": "base64_ed25519_signature"
}
```

**Simultaneous Add Resolution:**
If both peers send `peer_add_request` to each other before receiving a response, the conflict is resolved by UUID comparison: the peer with the lexicographically *lower* `device_id` is treated as the initiator, and the other peer auto-accepts.

**Peer Remove**
```json
{
  "type": "peer_remove",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1707298800000,
  "signature": "base64_ed25519_signature"
}
```
On receipt, the remote peer removes the sender from their peer list and closes the connection.

**Peer Disconnect** (graceful)
```json
{
  "type": "peer_disconnect",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1707298800000
}
```
Signals a graceful disconnect (app closing, going offline). The remote peer marks the sender as offline without removing them.

### 5.3 Keep-Alive

```json
{
  "type": "ping",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1707298800000
}
```

```json
{
  "type": "pong",
  "from_device_id": "660e8400-e29b-41d4-a716-446655440001",
  "timestamp": 1707298800010
}
```

Sent every 60 seconds on idle connections. If no `pong` is received within 15 seconds, the connection is considered dead and closed.

### 5.4 Chat Messages

**Encrypted Chat Message** (encrypted payload, sent over TCP)
```json
{
  "type": "message",
  "message_id": "msg-550e8400-001",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "to_device_id": "660e8400-e29b-41d4-a716-446655440001",
  "content_type": "text",
  "sequence": 42,
  "encrypted_content": "base64_aes256gcm_ciphertext",
  "iv": "base64_initialization_vector",
  "timestamp": 1707298800000,
  "signature": "base64_ed25519_signature"
}
```

**`sequence`**: Monotonically increasing integer per TCP session, starting at 1. Used for replay protection (see §10).

**Plaintext Content** (decrypted from `encrypted_content`)
```json
{
  "body": "Hey, how are you?"
}
```

The `content_type` field on the outer message is the single source of truth for how to interpret the decrypted body. It is not duplicated inside the plaintext.

### 5.5 File Transfer Messages

**File Transfer Initiation**
```json
{
  "type": "file_request",
  "file_id": "file-550e8400-001",
  "from_device_id": "550e8400-e29b-41d4-a716-446655440000",
  "to_device_id": "660e8400-e29b-41d4-a716-446655440001",
  "filename": "photo.jpg",
  "filesize": 2048576,
  "filetype": "image/jpeg",
  "checksum": "sha256_hash_of_file",
  "sequence": 43,
  "timestamp": 1707298800000,
  "signature": "base64_ed25519_signature"
}
```

**File Transfer Response** (recipient accepts or rejects)
```json
{
  "type": "file_response",
  "file_id": "file-550e8400-001",
  "status": "accepted",
  "from_device_id": "660e8400-e29b-41d4-a716-446655440001",
  "timestamp": 1707298800050,
  "signature": "base64_ed25519_signature"
}
```

`status` values: `"accepted"` or `"rejected"`. Chunks are **only** sent after an `"accepted"` response. The recipient UI shows a prompt: `"Alice wants to send photo.jpg (2.0 MB). Accept?"`.

**File Transfer Data** (sent in chunks, encrypted)
```json
{
  "type": "file_data",
  "file_id": "file-550e8400-001",
  "chunk_index": 0,
  "total_chunks": 10,
  "chunk_size": 262144,
  "encrypted_data": "base64_aes256gcm_chunk",
  "iv": "base64_iv",
  "timestamp": 1707298800100
}
```

**File Transfer Complete**
```json
{
  "type": "file_complete",
  "file_id": "file-550e8400-001",
  "status": "success",
  "timestamp": 1707298800500
}
```

### 5.6 Acknowledgment/Status Messages

**Message Acknowledgment**
```json
{
  "type": "ack",
  "message_id": "msg-550e8400-001",
  "from_device_id": "660e8400-e29b-41d4-a716-446655440001",
  "status": "received",
  "timestamp": 1707298800050
}
```

### 5.7 Error Messages

```json
{
  "type": "error",
  "code": "decryption_failed | invalid_signature | unknown_type | version_mismatch | queue_full",
  "message": "Human-readable description",
  "related_message_id": "msg-550e8400-001",
  "timestamp": 1707298800060
}
```

---

## 6. Data Models

### 6.1 Peer Model
```
Peer:
  - device_id (UUID, primary key)
  - device_name (string, user-facing name)
  - ed25519_public_key (base64-encoded public key)
  - key_fingerprint (string, truncated SHA-256 hex)
  - added_timestamp (int64 Unix ms)
  - last_seen_timestamp (int64 Unix ms)
  - status (enum: online, offline, pending)
  - local_port (int, the port they're listening on)
  - local_ip (string, last known local IP)
```

### 6.2 Message Model
```
Message:
  - message_id (string, unique per conversation)
  - from_device_id (UUID)
  - to_device_id (UUID)
  - content (string, plaintext after decryption)
  - content_type (enum: text, image, file)
  - timestamp_sent (int64 Unix ms)
  - timestamp_received (int64 Unix ms)
  - is_read (boolean)
  - signature (base64 Ed25519 signature)
```

### 6.3 File Model
```
File:
  - file_id (string, unique identifier)
  - message_id (string, FK to Message)
  - filename (string)
  - filesize (int64, bytes)
  - filetype (string, MIME type)
  - stored_path (string, local disk path)
  - checksum (string, SHA-256 hash)
  - timestamp_received (int64 Unix ms)
  - transfer_status (enum: pending, accepted, rejected, complete, failed)
```

### 6.4 Device Configuration
```
DeviceConfig (stored as config.json — single source of truth):
  - device_id (UUID, generated on first launch)
  - device_name (string, editable by user)
  - listening_port (int, default 9999)
  - ed25519_private_key_path (string)
  - ed25519_public_key_path (string)
  - x25519_private_key_path (string)
```

---

## 7. UI Layout Descriptions

### 7.1 Main Window
```
┌─────────────────────────────────────────────────┐
│  P2P Chat                                   [_][□][X]
├─────────────────────────────────────────────────┤
│ [⚙️ Settings]  [🔍 Refresh Discovery]            │
├──────────────────┬──────────────────────────────┤
│                  │                              │
│  Peers List      │  Chat View                   │
│                  │                              │
│  □ Alice (●)    │  ┌──────────────────────────┐ │
│  □ Bob (●)      │  │ Alice's Laptop     [🔑]  │ │
│  □ Carol (○)    │  ├──────────────────────────┤ │
│                  │  │ Hey there!               │ │
│  [+ Add Peer]   │  │ Hi! How are you?         │ │
│                  │  └──────────────────────────┘ │
│                  │  ┌─ Input Field ───────────┐ │
│                  │  │ Type message...      [📎]│ │
│                  │  │                  [Send]  │ │
│                  │  └──────────────────────────┘ │
└──────────────────┴──────────────────────────────┘
```

The `[🔑]` button shows the peer's key fingerprint for out-of-band verification.

### 7.2 Peer Discovery/Add Dialog
```
┌────────────────────────────────────┐
│  Available Peers                [X] │
├────────────────────────────────────┤
│                                    │
│  Device Name        Status  Action │
│  ─────────────────────────────────  │
│  Alice's Laptop     Online  [Add]   │
│  Bob's Phone        Online  [Add]   │
│  Carol's Desktop    Offline [Add]   │
│                                    │
│                     [Close] [Refresh]
│                                    │
└────────────────────────────────────┘
```

### 7.3 Settings/Device Configuration
```
┌────────────────────────────────────┐
│  Device Settings                [X] │
├────────────────────────────────────┤
│                                    │
│  Device Name: [My Laptop_______]   │
│                                    │
│  Device ID: 550e8400-e29b-41d4...  │
│                                    │
│  Fingerprint:                      │
│  AB12 CD34 EF56 7890 1234 5678... │
│                                    │
│  Listening Port: [9999___]          │
│                                    │
│  [Reset Keys]                       │
│                                    │
│                      [Save]  [Close]│
│                                    │
└────────────────────────────────────┘
```

### 7.4 File Transfer Accept Dialog
```
┌────────────────────────────────────┐
│  Incoming File                  [X] │
├────────────────────────────────────┤
│                                    │
│  Alice wants to send you a file:   │
│                                    │
│  📄 photo.jpg                      │
│  Size: 2.0 MB                      │
│  Type: image/jpeg                  │
│                                    │
│              [Reject]    [Accept]   │
│                                    │
└────────────────────────────────────┘
```

### 7.5 Chat Window (per peer)
```
Message display area:
- Left-aligned: messages from peer (gray background)
- Right-aligned: messages from self (blue background)
- Timestamp below each message
- For files: thumbnail + filename with download option
- Delivery status indicator (✓ sent, ✓✓ delivered, ✗ failed)

Input area:
- Text input field (multiline)
- File attachment button (📎)
- Send button

Example:
  Alice's Laptop                    [🔑]
  ──────────────────

  Hey there!
  12:34 PM

                                    Hi! How are you?
                                    12:35 PM ✓✓

  Pretty good, just sent you a photo
  [photo.jpg] 2.1 MB  ↓
  12:36 PM ✓
```

---

## 8. Discovery & Connection Flow

### 8.1 Startup Sequence
```
1.  App launches
2.  Load config.json (or generate new config if first run)
3.  Initialize SQLite database (run migrations)
4.  Load Ed25519 + X25519 keypairs from disk (or generate if first run)
5.  Start mDNS broadcaster (advertise this device)
6.  Start mDNS listener (scan for other devices)
7.  Populate "Available Peers" list from discovered services
8.  Load added peers from database
9.  Attempt to reconnect to known peers that are online
10. Display main UI
```

### 8.2 Peer Discovery
```
Continuous Process:
- mDNS listener runs in background goroutine
- Listens for _p2pchat._tcp.local. services
- When service discovered:
  - Extract device_id, device_name, port, IP from TXT records
  - Add to temporary "available peers" list (in-memory only)
  - Don't persist unless user clicks "Add"

Manual Refresh:
- User clicks "🔍 Refresh Discovery" button
- Force re-scan of network
- Clear and repopulate available peers list
```

### 8.3 Peer Addition
```
User Flow:
1.  User sees "Bob's Phone" in available peers
2.  Clicks [Add]
3.  App initiates TCP connection to Bob's device
4.  Performs handshake (exchange Ed25519 + ephemeral X25519 keys)
5.  Derives shared session key via X25519 ECDH
6.  Sends "peer_add_request" (encrypted with session key)
7.  Bob's device receives request, shows prompt to user
8.  Bob clicks [Accept]
9.  Bob's device sends "peer_add_response" with status "accepted"
10. Both devices store peer record in SQLite (including Ed25519 public key)
11. Chat window opens

Simultaneous Add:
- If both peers send peer_add_request before receiving a response,
  the peer with the lexicographically lower device_id is treated
  as the initiator. The other peer auto-accepts.

Database Update:
- peer.status = "online"
- peer.ed25519_public_key = exchanged key
- peer.key_fingerprint = computed fingerprint
- peer.added_timestamp = now
```

### 8.4 Connection Establishment & Session Key Derivation
```
1. TCP connection opened
2. Both sides send handshake with:
   - Their Ed25519 identity public key
   - A fresh ephemeral X25519 public key (generated per connection)
3. Both sides perform X25519(local_ephemeral_private, remote_ephemeral_public)
4. Derive session key: HKDF-SHA256(shared_secret, salt="p2pchat-v1", info="session-key")
5. Result: 32-byte AES-256-GCM session key used for all messages on this connection
6. Each side initializes a sequence counter to 0
7. If the peer's Ed25519 key is already stored, verify it matches.
   If mismatch → warn user: "⚠️ Key has changed! Possible MITM."
   Connection is blocked until user explicitly re-trusts.
```

### 8.5 Message Delivery
```
1.  User types message, clicks Send
2.  Generate message_id (UUID)
3.  Increment session sequence counter
4.  Create Message struct with plaintext content
5.  Encrypt content with session AES-256-GCM key + fresh random IV (12 bytes)
6.  Sign the encrypted payload with sender's Ed25519 private key
7.  Serialize to JSON with sequence number
8.  Send over TCP to peer (length-prefixed frame)
9.  Store in local SQLite as sent message (plaintext)
10. Peer receives frame, verifies sequence > last seen sequence
11. Peer verifies Ed25519 signature using sender's stored public key
12. Peer decrypts content using session key + IV
13. Peer stores in SQLite as received message
14. Peer sends ack message
15. Local message marked as delivered
```

---

## 9. SQLite Storage Schema

### Table: peers
```sql
CREATE TABLE peers (
  device_id           TEXT PRIMARY KEY,
  device_name         TEXT NOT NULL,
  ed25519_public_key  TEXT NOT NULL,
  key_fingerprint     TEXT NOT NULL,
  status              TEXT CHECK(status IN ('online','offline','pending','blocked')) DEFAULT 'pending',
  added_timestamp     INTEGER NOT NULL,
  last_seen_timestamp INTEGER,
  last_known_ip       TEXT,
  last_known_port     INTEGER
);
```

### Table: messages
```sql
CREATE TABLE messages (
  message_id         TEXT PRIMARY KEY,
  from_device_id     TEXT REFERENCES peers(device_id),
  to_device_id       TEXT REFERENCES peers(device_id),
  content            TEXT NOT NULL,
  content_type       TEXT CHECK(content_type IN ('text','image','file')) DEFAULT 'text',
  timestamp_sent     INTEGER NOT NULL,
  timestamp_received INTEGER,
  is_read            INTEGER DEFAULT 0,
  delivery_status    TEXT CHECK(delivery_status IN ('pending','sent','delivered','failed')) DEFAULT 'pending',
  signature          TEXT
);
```

### Table: files
```sql
CREATE TABLE files (
  file_id            TEXT PRIMARY KEY,
  message_id         TEXT REFERENCES messages(message_id),
  from_device_id     TEXT,
  to_device_id       TEXT,
  filename           TEXT NOT NULL,
  filesize           INTEGER NOT NULL,
  filetype           TEXT,
  stored_path        TEXT NOT NULL,
  checksum           TEXT NOT NULL,
  timestamp_received INTEGER,
  transfer_status    TEXT CHECK(transfer_status IN ('pending','accepted','rejected','complete','failed')) DEFAULT 'pending'
);
```

### Table: seen_message_ids
```sql
CREATE TABLE seen_message_ids (
  message_id  TEXT PRIMARY KEY,
  received_at INTEGER NOT NULL
);
```

Used for replay protection. Periodically pruned (messages older than 24 hours removed).

---

## 10. Encryption & Security

### 10.1 Key Generation & Storage
```
On First Launch:
1. Generate Ed25519 signing keypair
   - Save to: {app_data_dir}/keys/ed25519_private.pem (0600)
   - Save to: {app_data_dir}/keys/ed25519_public.pem
2. Generate X25519 static private key
   - Save to: {app_data_dir}/keys/x25519_private.pem (0600)
3. Compute key fingerprint: SHA-256(ed25519_public_key)[0:16] as hex
4. Store fingerprint in config.json for display
```

### 10.2 Session Key Derivation (per TCP connection)
```
On Each Connection:
1. Generate ephemeral X25519 keypair (not persisted)
2. Exchange ephemeral public keys in handshake
3. Compute shared secret: X25519(my_ephemeral_private, their_ephemeral_public)
4. Derive session key:
   session_key = HKDF-SHA256(
     ikm    = shared_secret,
     salt   = "p2pchat-session-v1",
     info   = sorted(device_id_a, device_id_b),
     length = 32 bytes
   )
5. This session key is used for all AES-256-GCM encryption on this connection
6. Ephemeral private key is discarded after derivation
7. Forward secrecy: compromising long-term keys cannot decrypt past sessions
```

### 10.3 Message Encryption
```
For each message on an established connection:

Encrypt:
1. Serialize plaintext content as JSON bytes
2. Generate random 12-byte IV (nonce) for AES-256-GCM
3. Encrypt using session key:
   ciphertext, tag = AES-256-GCM(session_key, iv, plaintext)
4. Sign the ciphertext with sender's Ed25519 private key:
   signature = Ed25519.Sign(private_key, ciphertext)
5. Construct wire message:
   {
     "type": "message",
     "message_id": "...",
     "from_device_id": "...",
     "to_device_id": "...",
     "content_type": "text",
     "sequence": 42,
     "encrypted_content": base64(ciphertext + tag),
     "iv": base64(iv),
     "signature": base64(signature),
     "timestamp": ...
   }

Decrypt (on receive):
1. Check sequence > last_seen_sequence for this connection (reject if not)
2. Check message_id not in seen_message_ids table (reject duplicates)
3. Verify Ed25519 signature against sender's stored public key
4. Decrypt: plaintext = AES-256-GCM.Open(session_key, iv, ciphertext)
5. Parse plaintext JSON
6. Store plaintext in database
7. Insert message_id into seen_message_ids
8. Update last_seen_sequence
9. Display to user
```

### 10.4 File Encryption
```
Files sent in chunks (256 KB per chunk):
- Each chunk encrypted independently with AES-256-GCM using the session key
- Each chunk gets a unique random IV
- File checksum (SHA-256) computed on plaintext before encryption
- Checksum sent in file_request, verified after full reassembly
- Failed/corrupted chunks trigger retransmission of that chunk
```

### 10.5 Replay Protection
```
Three layers of replay protection:

1. Sequence Numbers (per TCP session):
   - Each side maintains a monotonic counter starting at 1
   - Every sent message increments the counter
   - Receiver rejects any message with sequence <= last_seen_sequence
   - Counters reset when TCP connection is re-established

2. Message ID Registry (persistent):
   - Every received message_id is stored in seen_message_ids table
   - Duplicate message_ids are rejected
   - Entries older than 24 hours are pruned periodically

3. Timestamp Validation:
   - Messages with timestamps more than 5 minutes in the past or future
     are rejected (guards against gross clock skew attacks)
```

### 10.6 Key Change Detection
```
When connecting to a known peer:
1. Compare their Ed25519 public key to the stored value
2. If match → proceed normally
3. If mismatch → block connection, display warning:
   "⚠️ The identity key for {device_name} has changed.
    This could mean the device was reset, or someone is
    intercepting your connection.
    Old fingerprint: AB12 CD34 ...
    New fingerprint: FF98 BA76 ...
    [Trust New Key]  [Disconnect]"
4. User must explicitly accept the new key to continue
5. If accepted, update stored key and fingerprint
```

---

## 11. Network Protocol Specification

### 11.1 Connection Establishment
```
Client → Server (TCP connect):
1. Open TCP socket to peer's listening port
2. Send handshake message (JSON, length-prefixed):
   {
     "type": "handshake",
     "device_id": "...",
     "device_name": "...",
     "ed25519_public_key": "base64...",
     "x25519_public_key": "base64_ephemeral...",
     "protocol_version": 1,
     "timestamp": ...,
     "signature": "base64_ed25519_signature"
   }

Server → Client (on TCP accept):
1. Receive handshake
2. Verify device_id not blocked
3. Check protocol_version is supported (if not, send error and close)
4. If known peer: verify Ed25519 key matches stored key
5. Send handshake_response (same structure)
6. Both sides derive session key via X25519 ECDH + HKDF
7. Connection state → READY
```

### 11.2 Message Framing
```
Each message sent as:
[4-byte length][JSON payload]

Length: uint32 (big-endian) = size of JSON payload in bytes
Payload: UTF-8 JSON object
Max frame size: 10 MB
Read timeout: 30 seconds per frame

Example (pseudocode):
  payload := map[string]any{"type": "message", ...}
  jsonBytes, _ := json.Marshal(payload)
  length := uint32(len(jsonBytes))
  binary.Write(conn, binary.BigEndian, length)
  conn.Write(jsonBytes)
```

### 11.3 Connection States
```
- CONNECTING:    TCP connected, handshake in progress
- READY:         Handshake complete, session key derived, messages can flow
- IDLE:          Connected but no activity (ping/pong every 60s)
- DISCONNECTING: Peer sent peer_disconnect, closing gracefully
- DISCONNECTED:  Connection closed or failed
```

---

## 12. Error Handling & Resilience

### 12.1 Connection Failures
```
- If TCP connection drops: mark peer as offline, state → DISCONNECTED
- Retry connection with exponential backoff:
  - Attempt 1: immediate
  - Attempt 2: 5 seconds
  - Attempt 3: 15 seconds
  - Attempt 4: 60 seconds
  - Then: every 60 seconds until success or user cancels
- On user interaction (click on offline peer): attempt immediate reconnect
- Display offline indicator (○) next to peer name
```

### 12.2 Message Queue (Offline Messages)
```
When a peer is offline, outbound messages are queued locally:
- Stored in SQLite with delivery_status = "pending"
- On reconnect: send all pending messages in order
- Queue limits per peer:
  - Max 500 queued messages
  - Max 7 days age (older messages marked as "failed")
  - Max 50 MB total queued content
- When limits exceeded: oldest messages are marked "failed"
  and the user is notified
```

### 12.3 Message Failures
```
- If send fails mid-stream: store/keep as "pending" in DB
- Retry on reconnect (respecting queue limits)
- If file transfer interrupted: resume from last acknowledged chunk
- User notified of delivery status:
  ✓  = sent (TCP delivered)
  ✓✓ = delivered (ack received from peer)
  ✗  = failed (send error or queue expired)
```

### 12.4 Decryption & Verification Failures
```
- Invalid Ed25519 signature: reject message, send error response, log
- Sequence number violation: reject silently, log
- Duplicate message_id: reject silently, log
- Decryption error (bad session key, corrupt data): send error response, log
- Corrupted file checksum after reassembly: request retransmission of full file
- Timestamp out of range (>5min skew): reject, send error response
```

---

## 13. Configuration & Constants

### Default Values
```
Listening Port:              9999
Max Frame Size:              10 MB
File Chunk Size:             256 KB (262,144 bytes)
Keep-Alive Interval:         60 seconds
Keep-Alive Timeout:          15 seconds
Connection Timeout:          30 seconds
Discovery Refresh Interval:  10 seconds (background)
mDNS TTL:                   120 seconds
Timestamp Skew Tolerance:    5 minutes
Message Queue Max Per Peer:  500 messages
Message Queue Max Age:       7 days
Message Queue Max Size:      50 MB per peer
Seen Message ID Prune Age:   24 hours
Protocol Version:            1
```

### Paths
```
Linux:   ~/.config/p2p-chat/
macOS:   ~/Library/Application Support/p2p-chat/
Windows: %APPDATA%\p2p-chat\
```

---
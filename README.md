# This document will track what has been implemented. When something has been done, please include it here. Any updates to the app should also update the readme.

# P2P Chat — Implementation Checklist

## Phase 1: Project Scaffold & Configuration
- [x] Initialize Go module with dependencies (fyne, go-sqlite3, zeroconf, uuid)
- [x] Implement OS-aware data directory resolution (Linux/macOS/Windows)
- [x] Create directory structure on first launch (`keys/`, `files/`)
- [x] Generate device UUID on first launch
- [x] Implement `config.json` creation with default values
- [x] Implement `config.json` load on subsequent launches
- [x] Implement Ed25519 keypair generation
- [x] Implement Ed25519 PEM save/load (private key with 0600 permissions)
- [x] Implement X25519 static key generation
- [x] Implement X25519 PEM save/load (private key with 0600 permissions)
- [x] Compute and store key fingerprint (truncated SHA-256 hex)
- [x] Wire up `main.go` startup: load config → generate keys if missing → print identity

### Phase 1 Verification
- `go run .` first run creates config and key files under the OS-specific app data directory.
- `go run .` second run reuses the same identity and key material.

## Phase 2: SQLite Storage Layer
- [ ] Implement database open/create with `app.db`
- [ ] Implement schema migrations (peers, messages, files, seen_message_ids tables)
- [ ] Implement `AddPeer`
- [ ] Implement `GetPeer`
- [ ] Implement `ListPeers`
- [ ] Implement `UpdatePeerStatus`
- [ ] Implement `RemovePeer`
- [ ] Implement `SaveMessage`
- [ ] Implement `GetMessages(peerID, limit, offset)`
- [ ] Implement `MarkDelivered`
- [ ] Implement `GetPendingMessages(peerID)`
- [ ] Implement `PruneExpiredQueue`
- [ ] Implement `SaveFileMetadata`
- [ ] Implement `UpdateTransferStatus`
- [ ] Implement `GetFileByID`
- [ ] Implement `InsertSeenID`
- [ ] Implement `HasSeenID`
- [ ] Implement `PruneOldEntries` (seen_message_ids)
- [ ] Unit tests for all peer CRUD operations
- [ ] Unit tests for all message CRUD operations
- [ ] Unit tests for all file CRUD operations
- [ ] Unit tests for seen_message_ids operations

## Phase 3: Encryption & Signing
- [ ] Implement ephemeral X25519 keypair generation
- [ ] Implement X25519 ECDH shared secret computation
- [ ] Implement HKDF-SHA256 session key derivation
- [ ] Implement `Encrypt(sessionKey, plaintext) → (ciphertext, iv)` (AES-256-GCM)
- [ ] Implement `Decrypt(sessionKey, iv, ciphertext) → plaintext` (AES-256-GCM)
- [ ] Implement `Sign(privateKey, data) → signature` (Ed25519)
- [ ] Implement `Verify(publicKey, data, signature) → bool` (Ed25519)
- [ ] Round-trip encryption test (encrypt → decrypt → compare)
- [ ] Signature validity test (sign → verify)
- [ ] Signature tampering test (sign → modify data → verify rejects)
- [ ] Session key derivation test (both sides derive same key)

## Phase 4: Network Protocol & TCP Server
- [ ] Define model structs (Peer, Message, File) with JSON tags
- [ ] Define all protocol message type structs with JSON serialization
- [ ] Implement length-prefixed framing (`WriteFrame` / `ReadFrame`)
- [ ] Implement max frame size enforcement (10 MB)
- [ ] Implement TCP listener (configurable port)
- [ ] Implement inbound connection accept and handshake read
- [ ] Implement handshake verification and response
- [ ] Implement session key derivation on connection establish
- [ ] Implement `PeerConnection` struct (conn, session key, sequence counters, state)
- [ ] Implement `SendMessage` on `PeerConnection`
- [ ] Implement `ReceiveMessage` on `PeerConnection`
- [ ] Implement connection state machine (CONNECTING → READY → IDLE → DISCONNECTED)
- [ ] Implement outbound dial and handshake send
- [ ] Implement key change detection (compare stored vs received Ed25519 key)
- [ ] Implement key change warning flow (block until user decision)
- [ ] Implement ping/pong keep-alive (60s interval, 15s timeout)
- [ ] Implement connection timeout (30s)
- [ ] Test: two instances handshake and derive matching session keys
- [ ] Test: keep-alive maintains idle connections
- [ ] Test: dead connection detected on ping timeout

## Phase 5: mDNS Discovery
- [ ] Implement mDNS service broadcast with TXT records (device_id, version, key_fingerprint)
- [ ] Implement mDNS listener for `_p2pchat._tcp.local.`
- [ ] Filter self from discovered peers (by device_id)
- [ ] Implement background polling goroutine (10s interval)
- [ ] Maintain in-memory available peers list
- [ ] Expose discovery events (channel or callback) for UI consumption
- [ ] Implement manual refresh trigger
- [ ] Wire discovery into startup sequence
- [ ] Test: two instances on same LAN discover each other within 10s
- [ ] Test: manual refresh finds peers immediately

## Phase 6: Peer Management
- [ ] Implement `peer_add_request` send (initiator side)
- [ ] Implement `peer_add_request` receive and queue for user approval
- [ ] Implement accept flow: both sides persist peer to SQLite
- [ ] Implement reject flow: close connection, nothing persisted
- [ ] Implement `peer_add_response` send/receive
- [ ] Implement simultaneous add resolution (lower UUID = initiator, other auto-accepts)
- [ ] Implement `peer_remove` send/receive (remove from DB, close connection)
- [ ] Implement `peer_disconnect` send/receive (mark offline, keep in DB)
- [ ] Implement reconnection to known online peers on startup
- [ ] Implement exponential backoff retry (immediate → 5s → 15s → 60s → every 60s)
- [ ] Test: A adds B, B accepts, both show online
- [ ] Test: B restarts, A detects offline, reconnects when B returns
- [ ] Test: A removes B, both sides clean up
- [ ] Test: simultaneous add resolves without conflict

## Phase 7: Messaging
- [ ] Implement send flow: encrypt with session key → sign → frame → send → store locally
- [ ] Implement receive flow: validate sequence → check seen_message_ids → verify signature → decrypt → store → display
- [ ] Implement `ack` message send on receive
- [ ] Implement `ack` message handling (update delivery_status to "delivered")
- [ ] Implement replay protection: sequence number validation (reject if ≤ last seen)
- [ ] Implement replay protection: message_id deduplication via seen_message_ids table
- [ ] Implement replay protection: timestamp skew rejection (±5 min)
- [ ] Implement offline message queue (store as "pending" in DB)
- [ ] Implement queue drain on reconnect (send pending messages in order)
- [ ] Implement queue limits: max 500 messages per peer
- [ ] Implement queue limits: max 7 days age
- [ ] Implement queue limits: max 50 MB per peer
- [ ] Implement queue overflow handling (oldest marked "failed", user notified)
- [ ] Implement error responses for decryption/signature failures
- [ ] Test: A sends to B, B receives and decrypts, A sees ✓✓
- [ ] Test: B offline → A queues → B reconnects → messages delivered
- [ ] Test: replayed message rejected (duplicate message_id)
- [ ] Test: out-of-sequence message rejected
- [ ] Test: tampered signature rejected

## Phase 8: File Transfer
- [ ] Implement `file_request` send with metadata and SHA-256 checksum
- [ ] Implement `file_request` receive and present accept/reject prompt
- [ ] Implement `file_response` send (accepted/rejected)
- [ ] Implement `file_response` handling (proceed or abort)
- [ ] Implement chunked file reading (256 KB chunks)
- [ ] Implement per-chunk AES-256-GCM encryption with unique IV
- [ ] Implement `file_data` send with chunk_index and total_chunks
- [ ] Implement chunked file receiving and reassembly
- [ ] Implement per-chunk decryption
- [ ] Implement SHA-256 checksum verification on completed file
- [ ] Implement `file_complete` acknowledgment send/receive
- [ ] Implement chunk retransmission on failure
- [ ] Implement transfer resume (track last acknowledged chunk, resume on reconnect)
- [ ] Save received files to `files/` directory
- [ ] Store file metadata in database
- [ ] Implement file picker dialog (UI)
- [ ] Implement transfer progress tracking
- [ ] Test: A sends 5 MB file to B, B accepts, file transfers, checksum matches
- [ ] Test: B rejects file, no data sent
- [ ] Test: connection drops mid-transfer → reconnect → resumes from last chunk
- [ ] Test: corrupted file detected by checksum mismatch

## Phase 9: GUI
- [ ] Implement main window split layout (peer list left, chat view right, toolbar top)
- [ ] Implement peer list with online/offline indicators (● / ○)
- [ ] Implement peer list click to open chat view
- [ ] Implement "Add Peer" button
- [ ] Implement peer discovery dialog (available peers list, Add/Refresh/Close)
- [ ] Implement chat message list with left/right alignment
- [ ] Implement message timestamps
- [ ] Implement delivery status indicators (✓ sent, ✓✓ delivered, ✗ failed)
- [ ] Implement text input field (multiline)
- [ ] Implement send button
- [ ] Implement file attach button (📎) wired to file picker
- [ ] Implement file message display (filename, size, download)
- [ ] Implement file transfer progress bar in chat
- [ ] Implement file transfer accept/reject dialog
- [ ] Implement settings screen (device name, fingerprint, port, reset keys)
- [ ] Implement per-peer fingerprint display (🔑 button in chat header)
- [ ] Implement key change warning dialog (show old/new fingerprint, Trust/Disconnect)
- [ ] Implement peer add request notification/prompt
- [ ] Wire all UI callbacks to network/storage layers
- [ ] Handle threading: network events → UI updates via Fyne thread-safe methods
- [ ] Full manual walkthrough: launch → discover → add → chat → send file → settings → restart → reconnect → verify fingerprints

## Phase 10: Polish & Hardening
- [ ] Add structured logging (connection events, errors, crypto failures)
- [ ] Implement seen_message_ids pruning timer (hourly, remove >24h)
- [ ] Implement message queue pruning on startup
- [ ] Implement message queue pruning timer (periodic)
- [ ] Handle port-in-use: detect, pick next available, update config
- [ ] Handle corrupt database: detect, offer reset
- [ ] Handle missing/corrupt key files: detect, regenerate, warn user
- [ ] Handle oversized frames: reject at read
- [ ] Implement graceful shutdown: send `peer_disconnect` to all peers
- [ ] Implement graceful shutdown: close TCP listeners
- [ ] Implement graceful shutdown: flush/close database
- [ ] Cross-platform build: Linux
- [ ] Cross-platform build: macOS
- [ ] Cross-platform build: Windows
- [ ] Integration test: 2 instances, full add → chat → file flow
- [ ] Integration test: 3 instances, multiple peer connections simultaneously
- [ ] Integration test: kill signal handled gracefully
- [ ] Integration test: corrupt state recovery

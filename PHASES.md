# P2P Chat — Implementation Phases

Each phase builds on the previous and produces a testable milestone. Reference the Technical Design Document (TDD) for detailed specs.

---

## Phase 1: Project Scaffold & Configuration

**Goal:** App launches, generates identity, persists config.

**Tasks:**
1. Initialize Go module, install dependencies (`fyne`, `go-sqlite3`, `zeroconf`, `google/uuid`)
2. Implement `config/config.go` — OS-aware data directory resolution (TDD §13 Paths)
3. First-launch flow: generate UUID, create directory structure, write default `config.json` (TDD §6.4)
4. Load/save config from disk on subsequent launches
5. Implement `crypto/keypair.go` — Ed25519 keypair generation, PEM save/load with `0600` permissions
6. Implement `crypto/ecdh.go` — X25519 static key generation and PEM save/load
7. Compute and store key fingerprint (TDD §4)
8. `main.go` wires up startup: load config → generate keys if missing → print identity to stdout

**Verify:** Run binary twice. First run generates files. Second run loads them. Config dir contains `config.json`, `keys/ed25519_private.pem`, `keys/ed25519_public.pem`, `keys/x25519_private.pem`.

---

## Phase 2: SQLite Storage Layer

**Goal:** Database initializes, migrations run, CRUD operations work.

**Tasks:**
1. Implement `storage/database.go` — open/create `app.db`, run schema migrations (TDD §9)
2. Implement `storage/peers.go` — `AddPeer`, `GetPeer`, `ListPeers`, `UpdatePeerStatus`, `RemovePeer`
3. Implement `storage/messages.go` — `SaveMessage`, `GetMessages(peerID, limit, offset)`, `MarkDelivered`, `GetPendingMessages(peerID)`, `PruneExpiredQueue`
4. Implement `storage/files.go` — `SaveFileMetadata`, `UpdateTransferStatus`, `GetFileByID`
5. Implement `seen_message_ids` table operations — `InsertSeenID`, `HasSeenID`, `PruneOldEntries`
6. Write unit tests for all CRUD operations

**Verify:** Unit tests pass. Can insert peers, messages, files, query them back, and prune old entries.

---

## Phase 3: Encryption & Signing

**Goal:** All crypto operations work in isolation.

**Tasks:**
1. Implement `crypto/ecdh.go` — ephemeral X25519 keypair generation, ECDH shared secret computation, HKDF session key derivation (TDD §10.2)
2. Implement `crypto/encryption.go` — `Encrypt(sessionKey, plaintext) → (ciphertext, iv)` and `Decrypt(sessionKey, iv, ciphertext) → plaintext` using AES-256-GCM
3. Implement `crypto/signatures.go` — `Sign(privateKey, data) → signature` and `Verify(publicKey, data, signature) → bool` using Ed25519
4. Write round-trip tests: generate session key → encrypt → decrypt → compare
5. Write signature tests: sign → verify (valid), sign → tamper → verify (reject)

**Verify:** All crypto unit tests pass. Round-trip encryption produces identical plaintext. Tampered signatures are rejected.

---

## Phase 4: Network Protocol & TCP Server

**Goal:** Two instances can connect, handshake, and derive a session key.

**Tasks:**
1. Implement `models/` structs — `Peer`, `Message`, `File` with JSON tags (TDD §6)
2. Implement `network/protocol.go` — all message type structs and JSON serialization (TDD §5), length-prefixed framing (TDD §11.2), `WriteFrame` / `ReadFrame` helpers
3. Implement `network/server.go` — TCP listener on configured port, accept connections, read handshake, verify/respond, derive session key, track connection state (TDD §11.1, §11.3)
4. Implement `network/client.go` — dial peer, send handshake, read response, derive session key
5. Implement `network/connection.go` — `PeerConnection` struct holding: `net.Conn`, session key, sequence counters, peer info, connection state, `SendMessage` / `ReceiveMessage` methods
6. Implement key change detection logic (TDD §10.6)
7. Implement ping/pong keep-alive (TDD §5.3)

**Verify:** Run two instances on different ports. They handshake, derive matching session keys, and maintain connection via ping/pong. Kill one — the other detects disconnect.

---

## Phase 5: mDNS Discovery

**Goal:** Devices find each other automatically on the local network.

**Tasks:**
1. Implement `discovery/mdns.go` — broadcast service with TXT records (TDD §5.1), listen for `_p2pchat._tcp.local.` services
2. Implement `discovery/peer_scanner.go` — background goroutine, 10-second polling, maintain in-memory available peers list, expose channel or callback for UI updates
3. Filter out self (by `device_id`) from discovered peers
4. Wire into startup sequence (TDD §8.1 steps 5-7)
5. Implement manual refresh trigger

**Verify:** Run two instances on the same LAN. Each appears in the other's discovered peers list within 10 seconds. Manual refresh works immediately.

---

## Phase 6: Peer Management

**Goal:** Users can add/accept/remove peers. Peer state persists.

**Tasks:**
1. Implement the peer add flow end-to-end (TDD §8.3):
   - Initiator sends `peer_add_request` over encrypted connection
   - Receiver queues request for user approval
   - On accept: both sides persist peer to SQLite
   - On reject: connection closed, nothing persisted
2. Implement simultaneous add resolution (lower UUID wins — TDD §5.2)
3. Implement `peer_remove` — remove from DB, notify remote peer, close connection (TDD §5.2)
4. Implement `peer_disconnect` — graceful offline signal (TDD §5.2)
5. Implement reconnection to known peers on startup (TDD §8.1 step 9)
6. Implement exponential backoff retry (TDD §12.1)

**Verify:** Instance A adds Instance B. B sees prompt, accepts. Both show each other as online. Restart B — A detects offline, then reconnects when B comes back. A removes B — both sides clean up.

---

## Phase 7: Messaging

**Goal:** Peers can exchange encrypted text messages with delivery confirmation.

**Tasks:**
1. Implement send flow (TDD §8.5): encrypt with session key, sign, frame, send, store locally
2. Implement receive flow: validate sequence, check `seen_message_ids`, verify signature, decrypt, store, display
3. Implement `ack` messages (TDD §5.6) — update `delivery_status` on sender side
4. Implement replay protection — all three layers (TDD §10.5)
5. Implement offline message queue — store pending, send on reconnect, enforce limits (TDD §12.2)
6. Implement error responses for decryption/signature failures (TDD §5.7)

**Verify:** A sends message to B → B receives, decrypts, displays. A sees ✓✓. B goes offline → A queues messages → B reconnects → queued messages delivered. Replayed messages are rejected.

---

## Phase 8: File Transfer

**Goal:** Peers can send and receive files with acceptance prompt.

**Tasks:**
1. Implement `file_request` sending with metadata and checksum (TDD §5.5)
2. Implement `file_response` — recipient accept/reject prompt (TDD §5.5, §7.4)
3. Implement chunked sending — 256 KB chunks, each independently encrypted (TDD §10.4)
4. Implement chunked receiving — reassemble, verify SHA-256 checksum on completion
5. Implement `file_complete` acknowledgment
6. Implement chunk retransmission on failure
7. Implement resume — track last acknowledged chunk, resume from there on reconnect
8. Store file metadata in DB, save file to `files/` directory
9. Implement `ui/file_handler.go` — file picker dialog, progress tracking

**Verify:** A sends a 5 MB file to B. B sees accept prompt, accepts. File transfers with progress. Checksum matches. Kill connection mid-transfer → reconnect → resumes from last chunk.

---

## Phase 9: GUI

**Goal:** Full Fyne UI matching the layouts in the TDD.

**Tasks:**
1. Implement `ui/main_window.go` — split layout: peer list (left), chat view (right), toolbar (top) (TDD §7.1)
2. Implement `ui/peers_list.go` — list of added peers with online/offline indicator, click to open chat, "Add Peer" button opening discovery dialog
3. Implement `ui/peers_list.go` discovery dialog — list available (discovered) peers, Add/Refresh/Close buttons (TDD §7.2)
4. Implement `ui/chat_window.go` — message list (left/right aligned), text input, send button, file attach button (📎), delivery status indicators (TDD §7.5)
5. Implement `ui/settings.go` — device name editor, fingerprint display, port config, reset keys (TDD §7.3)
6. Implement fingerprint display per peer — `[🔑]` button in chat header (TDD §7.1)
7. Implement file transfer accept/reject dialog (TDD §7.4)
8. Implement key change warning dialog (TDD §10.6)
9. Wire all UI callbacks to the network/storage layers built in prior phases
10. Handle threading: network events → UI updates via Fyne's thread-safe methods

**Verify:** Full manual walkthrough: launch → discover peer → add → chat → send file → change settings → restart → reconnect → verify fingerprints. All UI states match TDD layouts.



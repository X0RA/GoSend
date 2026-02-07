# This document will track what has been implemented. When something has been done, please include it here. Any updates to the app should also update the readme.

# P2P Chat — Implementation Checklist

## overview
- `main.go`: GUI app entrypoint; loads config/keys, opens SQLite, builds local identity, and launches the Phase 9 Fyne runtime.
- `tools.go`: Build-tagged imports that keep planned dependencies (`fyne`, `zeroconf`, `go-sqlite3`) pinned in `go.mod`.
- `go.mod`: Go module definition and direct/indirect dependency versions.
- `go.sum`: Dependency checksum lock file.
- `config/config.go`: Data directory resolution, first-run directory creation, config load/create/save, and default config normalization (including `port_mode` behavior for automatic vs fixed listening ports).
- `config/config_test.go`: Tests first-run config creation and second-run reload stability.
- `crypto/keypair.go`: Ed25519 keypair generation/load/save, fingerprint generation, and display formatting.
- `crypto/keypair_test.go`: Tests Ed25519/X25519 key persistence stability across repeated loads.
- `crypto/ecdh.go`: X25519 key handling, ephemeral keypair generation, shared secret computation, and HKDF session key derivation.
- `crypto/ecdh_session_test.go`: Tests that both peers derive identical shared secrets and session keys.
- `crypto/encryption.go`: AES-256-GCM encryption/decryption helpers.
- `crypto/encryption_test.go`: Round-trip encryption/decryption test.
- `crypto/signatures.go`: Ed25519 sign/verify helpers.
- `crypto/signatures_test.go`: Valid-signature and tampered-payload rejection tests.
- `models/peer.go`: JSON-tagged peer model used by the network/UI layers.
- `models/message.go`: JSON-tagged message model used by the network/UI layers.
- `models/file.go`: JSON-tagged file metadata model used by the network/UI layers.
- `network/protocol.go`: Protocol message structs, handshake signing/verification helpers, and length-prefixed framing (`WriteFrame`/`ReadFrame`) with 10 MB enforcement.
- `network/connection.go`: `PeerConnection` implementation with sequence counters, connection state machine, send/receive APIs, and ping/pong keep-alive handling.
- `network/handshake.go`: Shared handshake options/defaults, key-change decision hook, and session-key derivation helpers.
- `network/client.go`: Outbound TCP dial and handshake flow, including session-key derivation and key-change checks.
- `network/server.go`: TCP listener/accept loop, inbound handshake verification/response, and connection handoff.
- `network/peer_manager.go`: Peer lifecycle manager for add/accept/reject/remove/disconnect plus encrypted message send/receive, ack/error handling, replay protection checks, offline queue drain, and reconnect backoff.
- `network/file_transfer.go`: File transfer engine for `file_request`/`file_response`/`file_data`/`file_complete`, chunk encryption/decryption, checksum verification, retransmission retries, and reconnect resume.
- `network/protocol_test.go`: Framing tests (round-trip and oversized frame rejection).
- `network/integration_test.go`: Integration tests for handshake/session key matching, idle keep-alive stability, dead connection timeout detection, and key-change decision blocking.
- `network/peer_manager_test.go`: Integration tests for peer add approval queue, restart reconnection, peer removal cleanup, and simultaneous add resolution.
- `network/messaging_test.go`: Integration tests for delivered-ack updates, offline queue drain after reconnect, duplicate message replay rejection, out-of-sequence rejection, and tampered-signature rejection.
- `network/file_transfer_test.go`: Integration tests for accepted transfers, reject flow, reconnect resume, and checksum mismatch failure handling.
- `discovery/mdns.go`: mDNS broadcaster setup plus combined discovery service startup/shutdown orchestration.
- `discovery/peer_scanner.go`: Background peer scanner with self-filtering, in-memory peer list, peer-aging to avoid transient dropouts, per-scan resolver lifecycle, event channel, and manual refresh support.
- `discovery/mdns_test.go`: Tests broadcaster TXT record generation and service startup/shutdown wiring.
- `discovery/peer_scanner_test.go`: Tests self-filtering, manual refresh updates, background polling, and peer removal events.
- `storage/types.go`: Shared storage types, status constants, validation helpers, null conversion helpers, and common errors.
- `storage/database.go`: SQLite open/create logic plus schema migrations for peers/messages/files/seen IDs.
- `storage/database_test.go`: Migration/open tests that verify DB file creation, schema version, and required tables.
- `storage/peers.go`: Peer CRUD methods (`AddPeer`, `GetPeer`, `ListPeers`, `UpdatePeerStatus`, `RemovePeer`).
- `storage/peers_test.go`: Peer CRUD tests.
- `storage/messages.go`: Message CRUD and queue methods (`SaveMessage`, `GetMessages`, `MarkDelivered`, `UpdateDeliveryStatus`, `GetMessageByID`, `GetPendingMessages`, `PruneExpiredQueue`).
- `storage/messages_test.go`: Message CRUD tests including delivery status updates, message lookup by ID, and queue-pruning behavior.
- `storage/files.go`: File metadata CRUD methods (`SaveFileMetadata`, `UpdateTransferStatus`, `GetFileByID`) with nullable ID/peer fields for flexible transfer lifecycle persistence.
- `storage/files_test.go`: File metadata CRUD tests.
- `ui/main_window.go`: GUI runtime controller; window shell, service lifecycle (discovery + peer manager), background event loops, and thread-safe dialog prompts for peer add/file accept/key change.
- `ui/peers_list.go`: Left-pane peers list with online/offline indicators and selection, plus the discovery/add dialog (`Add`, `Refresh`, `Close`).
- `ui/chat_window.go`: Per-peer chat view; message timeline (left/right alignment, timestamps, delivery marks), multiline input/send, attachment flow, and file transfer progress rendering.
- `ui/settings.go`: Settings dialog for device name, fingerprint visibility, listening port mode (`Automatic`/`Fixed`), fixed-port value, and key reset workflow.
- `ui/file_handler.go`: UI-facing file picker and transfer progress tracking helper.
- `storage/seen_ids.go`: Replay-protection ID methods (`InsertSeenID`, `HasSeenID`, `PruneOldEntries`).
- `storage/seen_ids_test.go`: Seen-message ID insert/check/prune tests.
- `storage/testutil_test.go`: Shared test helpers for creating a temporary store and seed peers.
- `DESIGN.md`: Technical design document describing architecture, protocol, data model, crypto, and UX flows.
- `PHASES.md`: Sequential implementation plan with phase-by-phase goals and verification criteria.
- `AGENTS.md`: Local coding agent instructions and workflow constraints for this repository.
- `README.md`: Implementation checklist and verification notes tracking what is complete.

## Development Commands
- `make build`: Build the app binary at `./bin/gosend`.
- `make run_tests`: Build and run two instances simultaneously for local P2P testing:
  - `P2P_CHAT_DATA_DIR=/tmp/gosend-a ./bin/gosend`
  - `P2P_CHAT_DATA_DIR=/tmp/gosend-b ./bin/gosend`
- `make run_client_a`: Build and run the `gosend-a` client only.
- `make run_client_b`: Build and run the `gosend-b` client only.

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
- [x] Implement database open/create with `app.db`
- [x] Implement schema migrations (peers, messages, files, seen_message_ids tables)
- [x] Implement `AddPeer`
- [x] Implement `GetPeer`
- [x] Implement `ListPeers`
- [x] Implement `UpdatePeerStatus`
- [x] Implement `RemovePeer`
- [x] Implement `SaveMessage`
- [x] Implement `GetMessages(peerID, limit, offset)`
- [x] Implement `MarkDelivered`
- [x] Implement `GetPendingMessages(peerID)`
- [x] Implement `PruneExpiredQueue`
- [x] Implement `SaveFileMetadata`
- [x] Implement `UpdateTransferStatus`
- [x] Implement `GetFileByID`
- [x] Implement `InsertSeenID`
- [x] Implement `HasSeenID`
- [x] Implement `PruneOldEntries` (seen_message_ids)
- [x] Unit tests for all peer CRUD operations
- [x] Unit tests for all message CRUD operations
- [x] Unit tests for all file CRUD operations
- [x] Unit tests for seen_message_ids operations

### Phase 2 Verification
- `go test ./...` passes, including storage unit tests for migrations and all CRUD paths.
- `go vet ./...` passes.

## Phase 3: Encryption & Signing
- [x] Implement ephemeral X25519 keypair generation
- [x] Implement X25519 ECDH shared secret computation
- [x] Implement HKDF-SHA256 session key derivation
- [x] Implement `Encrypt(sessionKey, plaintext) → (ciphertext, iv)` (AES-256-GCM)
- [x] Implement `Decrypt(sessionKey, iv, ciphertext) → plaintext` (AES-256-GCM)
- [x] Implement `Sign(privateKey, data) → signature` (Ed25519)
- [x] Implement `Verify(publicKey, data, signature) → bool` (Ed25519)
- [x] Round-trip encryption test (encrypt → decrypt → compare)
- [x] Signature validity test (sign → verify)
- [x] Signature tampering test (sign → modify data → verify rejects)
- [x] Session key derivation test (both sides derive same key)

### Phase 3 Verification
- `go test ./...` passes, including crypto tests for ECDH/HKDF derivation, AES-GCM round-trip, and signature checks.
- `go vet ./...` passes.

## Phase 4: Network Protocol & TCP Server
- [x] Define model structs (Peer, Message, File) with JSON tags
- [x] Define all protocol message type structs with JSON serialization
- [x] Implement length-prefixed framing (`WriteFrame` / `ReadFrame`)
- [x] Implement max frame size enforcement (10 MB)
- [x] Implement TCP listener (configurable port)
- [x] Implement inbound connection accept and handshake read
- [x] Implement handshake verification and response
- [x] Implement session key derivation on connection establish
- [x] Implement `PeerConnection` struct (conn, session key, sequence counters, state)
- [x] Implement `SendMessage` on `PeerConnection`
- [x] Implement `ReceiveMessage` on `PeerConnection`
- [x] Implement connection state machine (CONNECTING → READY → IDLE → DISCONNECTED)
- [x] Implement outbound dial and handshake send
- [x] Implement key change detection (compare stored vs received Ed25519 key)
- [x] Implement key change warning flow (block until user decision)
- [x] Implement ping/pong keep-alive (60s interval, 15s timeout)
- [x] Implement connection timeout (30s)
- [x] Test: two instances handshake and derive matching session keys
- [x] Test: keep-alive maintains idle connections
- [x] Test: dead connection detected on ping timeout

### Phase 4 Verification
- `go test ./...` passes, including network framing and integration tests for handshake/session derivation and keep-alive behavior.
- `go vet ./...` passes.

## Phase 5: mDNS Discovery
- [x] Implement mDNS service broadcast with TXT records (device_id, version, key_fingerprint)
- [x] Implement mDNS listener for `_p2pchat._tcp.local.`
- [x] Filter self from discovered peers (by device_id)
- [x] Implement background polling goroutine (10s interval)
- [x] Maintain in-memory available peers list
- [x] Expose discovery events (channel or callback) for UI consumption
- [x] Implement manual refresh trigger
- [x] Wire discovery into startup sequence
- [x] Test: two instances on same LAN discover each other within 10s
- [x] Test: manual refresh finds peers immediately

### Phase 5 Verification
- `go test ./...` passes, including discovery tests for TXT record publishing, self-filtering, polling updates, and manual refresh.
- `go vet ./...` passes.
- Manual refresh no longer treats expected scan timeout windows as errors; peers are retained across transient missed scans until stale.

### Discovery Stability Retrospective
- **Observed issue:** Peers were discovered at startup, then disappeared after ~1 minute; manual refresh often reported `context deadline exceeded` and did not rediscover peers.
- **Root cause 1:** The scanner reused a single `zeroconf.Resolver` across scans. After a scan context ended, resolver internals were effectively closed, so later scans became unreliable.
- **Root cause 2:** `PeerStaleAfter` defaulting used TTL before the TTL default was set, causing peers to age out too quickly in real usage.
- **Fix 1:** Use a fresh resolver per scan (`discovery/peer_scanner.go`), and keep refresh handling tolerant of normal scan-window timeout/cancel behavior.
- **Fix 2:** Set TTL defaults before stale-time calculation (`discovery/mdns.go`), and compute stale retention from TTL/refresh interval to avoid transient dropouts.
- **Result:** Discovery remains stable over time, and refresh is reliable instead of becoming startup-only.

## Phase 6: Peer Management
- [x] Implement `peer_add_request` send (initiator side)
- [x] Implement `peer_add_request` receive and queue for user approval
- [x] Implement accept flow: both sides persist peer to SQLite
- [x] Implement reject flow: close connection, nothing persisted
- [x] Implement `peer_add_response` send/receive
- [x] Implement simultaneous add resolution (lower UUID = initiator, other auto-accepts)
- [x] Implement `peer_remove` send/receive (remove from DB, close connection)
- [x] Implement `peer_disconnect` send/receive (mark offline, keep in DB)
- [x] Implement reconnection to known online peers on startup
- [x] Implement exponential backoff retry (immediate → 5s → 15s → 60s → every 60s)
- [x] Test: A adds B, B accepts, both show online
- [x] Test: B restarts, A detects offline, reconnects when B returns
- [x] Test: A removes B, both sides clean up
- [x] Test: simultaneous add resolves without conflict

### Phase 6 Verification
- `go test ./...` passes, including peer management integration tests for add/remove/disconnect, restart reconnection, and simultaneous add resolution.
- `go vet ./...` passes.

### Reconnection After Restart Retrospective
- **Observed issue:** After closing and reopening one client, known peers remained shown as offline even though the other client was still running and discoverable on the network. The restarted client never attempted to reconnect.
- **Root cause 1 (status gap):** On shutdown, `PeerManager.Stop()` closes all connections. Each `connectionLoop` cleanup marks the peer as `"offline"` in the database. On restart, `PeerManager.Start()` only reconnects peers with status `"online"` — since every peer is now `"offline"`, no reconnection is ever attempted.
- **Root cause 2 (discovery–reconnection gap):** The `discoveryEventLoop` in the UI layer processed mDNS `EventPeerUpserted` events but only used them to update the discovery dialog's available-peers list. It never checked whether a discovered peer was an already-added peer that should be reconnected to. Discovery and reconnection were completely decoupled.
- **Root cause 3 (wrong stored port):** When a peer connects inbound, `persistPeerConnection` stores the remote TCP endpoint via `conn.RemoteAddr()`. For inbound connections this is the peer's ephemeral outbound port, not their listening port. Any reconnection attempt using this stored port would fail. Discovery provides the correct listening port, so bridging discovery into reconnection also fixes this.
- **Fix 1:** Added `NotifyPeerDiscovered(deviceID, ip, port)` to `PeerManager` (`network/peer_manager.go`). When called, it validates the peer is known and not blocked, updates the stored endpoint in the database from the discovery data (correct listening port), and starts a reconnect worker if no active connection exists.
- **Fix 2:** Bridged discovery events to reconnection in the UI controller (`ui/main_window.go`). On each `EventPeerUpserted`, if the discovered peer is a known/added peer, `NotifyPeerDiscovered` is called. Additionally, on startup after the initial mDNS scan completes, all discovered peers are checked against the known peer list and reconnection is triggered for any matches.
- **Result:** After restart, known peers are reconnected within one mDNS scan interval (~10 seconds) as soon as they are rediscovered on the network. Both the restarted client and the still-running client can initiate reconnection, and the stored endpoint is always updated from discovery to reflect the correct listening port.

## Phase 7: Messaging
- [x] Implement send flow: encrypt with session key → sign → frame → send → store locally
- [x] Implement receive flow: validate sequence → check seen_message_ids → verify signature → decrypt → store → display
- [x] Implement `ack` message send on receive
- [x] Implement `ack` message handling (update delivery_status to "delivered")
- [x] Implement replay protection: sequence number validation (reject if ≤ last seen)
- [x] Implement replay protection: message_id deduplication via seen_message_ids table
- [x] Implement replay protection: timestamp skew rejection (±5 min)
- [x] Implement offline message queue (store as "pending" in DB)
- [x] Implement queue drain on reconnect (send pending messages in order)
- [x] Implement queue limits: max 500 messages per peer
- [x] Implement queue limits: max 7 days age
- [x] Implement queue limits: max 50 MB per peer
- [x] Implement queue overflow handling (oldest marked "failed", user notified)
- [x] Implement error responses for decryption/signature failures
- [x] Test: A sends to B, B receives and decrypts, A sees ✓✓
- [x] Test: B offline → A queues → B reconnects → messages delivered
- [x] Test: replayed message rejected (duplicate message_id)
- [x] Test: out-of-sequence message rejected
- [x] Test: tampered signature rejected

### Phase 7 Verification
- `go test ./...` passes, including messaging integration tests for ack delivery, replay protection, and offline queue drain.
- `go vet ./...` passes.

## Phase 8: File Transfer
- [x] Implement `file_request` send with metadata and SHA-256 checksum
- [x] Implement `file_request` receive and present accept/reject prompt
- [x] Implement `file_response` send (accepted/rejected)
- [x] Implement `file_response` handling (proceed or abort)
- [x] Implement chunked file reading (256 KB chunks)
- [x] Implement per-chunk AES-256-GCM encryption with unique IV
- [x] Implement `file_data` send with chunk_index and total_chunks
- [x] Implement chunked file receiving and reassembly
- [x] Implement per-chunk decryption
- [x] Implement SHA-256 checksum verification on completed file
- [x] Implement `file_complete` acknowledgment send/receive
- [x] Implement chunk retransmission on failure
- [x] Implement transfer resume (track last acknowledged chunk, resume on reconnect)
- [x] Save received files to `files/` directory
- [x] Store file metadata in database
- [x] Implement file picker dialog (UI)
- [x] Implement transfer progress tracking
- [x] Test: A sends 5 MB file to B, B accepts, file transfers, checksum matches
- [x] Test: B rejects file, no data sent
- [x] Test: connection drops mid-transfer → reconnect → resumes from last chunk
- [x] Test: corrupted file detected by checksum mismatch

### Phase 8 Verification
- `go test ./...` passes, including file transfer integration tests for accept/reject, retransmission/resume, and checksum validation.
- `go vet ./...` passes.

## Phase 9: GUI
- [x] Implement main window split layout (peer list left, chat view right, toolbar top)
- [x] Implement peer list with online/offline indicators (● / ○)
- [x] Implement peer list click to open chat view
- [x] Implement "Add Peer" button
- [x] Implement peer discovery dialog (available peers list, Add/Refresh/Close)
- [x] Implement chat message list with left/right alignment
- [x] Implement message timestamps
- [x] Implement delivery status indicators (✓ sent, ✓✓ delivered, ✗ failed)
- [x] Implement text input field (multiline)
- [x] Implement send button
- [x] Implement file attach button (📎) wired to file picker
- [x] Implement file message display (filename, size, download)
- [x] Implement file transfer progress bar in chat
- [x] Implement file transfer accept/reject dialog
- [x] Implement settings screen (device name, fingerprint, port, reset keys)
- [x] Implement per-peer fingerprint display (🔑 button in chat header)
- [x] Implement key change warning dialog (show old/new fingerprint, Trust/Disconnect)
- [x] Implement peer add request notification/prompt
- [x] Wire all UI callbacks to network/storage layers
- [x] Handle threading: network events → UI updates via Fyne thread-safe methods
- [ ] Full manual walkthrough: launch → discover → add → chat → send file → settings → restart → reconnect → verify fingerprints

### Phase 9 Verification
- `go test ./...` passes with the new GUI package compiled.
- `GOCACHE=/tmp/go-build go vet ./...` passes.
- Full GUI walkthrough remains manual (to be validated interactively across two app instances).
- Port behavior: in `Automatic` mode the app binds an available port at launch and advertises that bound port via discovery; in `Fixed` mode it binds the configured `listening_port`.

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

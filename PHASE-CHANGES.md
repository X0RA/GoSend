# PHASE_CHANGES.md — GoSend Enhancement Roadmap

---

## Phase 1 — Protocol Integrity & Cryptographic Hardening

This phase focuses entirely on the wire protocol and cryptographic layer. The changes touch `network/protocol.go`, `network/peer_manager.go`, `network/handshake.go`, `network/connection.go`, `crypto/ecdh.go`, and `storage/database.go`. The goal is to ensure that every message exchanged between peers is signed, verified, and resistant to replay, and that session keys have bounded lifetimes. Nothing in this phase changes the UI or user-facing settings — it purely hardens what's already happening under the hood.

### 1.1 — Sign All Control Messages

Currently `ack`, `file_complete`, `chunk_ack`, and `chunk_nack` are not signature-protected. Every protocol message that triggers a state transition should be cryptographically attributable to the sender, otherwise a compromised transport could forge delivery confirmations or file completion signals.

- [x] Add `Signature` field to `AckMessage` in `protocol.go`
- [x] Add `Signature` field to `FileComplete` in `protocol.go`
- [x] Add `Signature` and `FromDeviceID` fields to chunk ack/nack responses in `FileResponse` (already has signature — verify it's being set and checked on chunk_ack/chunk_nack paths)
- [x] Update `peer_manager.go` send paths to sign these messages with the local Ed25519 key before dispatch
- [x] Update `peer_manager.go` receive paths to verify signatures on `ack`, `file_complete`, and chunk responses before processing state transitions
- [x] Add tests covering forged/missing signatures on each of these message types to confirm rejection

### 1.2 — Handshake Replay Protection via Challenge-Response Nonce

The handshake currently relies on timestamps and transport timing for freshness. An attacker who records a valid handshake could replay it later on the same network. Adding an explicit challenge-response nonce binds the session key derivation to a specific handshake exchange and makes recorded handshakes useless.

- [x] Define a new pre-handshake frame type (e.g., `handshake_challenge`) containing a random 32-byte nonce, sent by the listener immediately upon accepting a TCP connection
- [x] Update the dialer in `client.go` to read the challenge nonce before sending its `handshake` message, and include the nonce in the signed handshake payload
- [x] Update the listener in `server.go` to generate and send the challenge, then verify the nonce is echoed back correctly in the received handshake
- [x] Include the challenge nonce in the HKDF info string during session key derivation in `handshake.go`, so that replayed handshakes derive a different (useless) key
- [x] Add tests that replay a recorded handshake and verify the connection is rejected or derives an incompatible session key

### 1.3 — Periodic Session Rekeying

Session keys are established once per connection and reused for the entire lifetime. For connections that stay alive for hours, a single key protects potentially gigabytes of data. Periodic rekeying limits the blast radius of any single key compromise and is standard practice for long-running encrypted channels.

- [x] Define `rekey_request` and `rekey_response` message types in `protocol.go`, each carrying a fresh ephemeral X25519 public key and an Ed25519 signature
- [x] Implement a rekey trigger in `connection.go` that fires after either 1 hour of elapsed time or 1 GB of data transmitted on the connection, whichever comes first
- [x] Implement the rekey exchange flow: initiator sends `rekey_request`, responder replies `rekey_response`, both sides derive a new session key and atomically switch the encryption context
- [x] Ensure in-flight messages that were encrypted under the old key are still decryptable during the brief transition window (sequence number or epoch tagging)
- [x] Add tests for rekey under load, rekey with concurrent file transfers, and rekey failure handling (connection should be dropped if rekey fails)

### 1.4 — Separate Frame Size Limits for Control vs Data

The current global `MaxFrameSize` of 10 MB applies to all message types. Control messages should never approach that size, and allowing them to do so means an unauthenticated peer could send a 10 MB "handshake" to exhaust memory before the connection is even verified.

- [x] Define `MaxControlFrameSize` (64 KB) alongside the existing `MaxFrameSize` (10 MB) in `protocol.go`
- [x] Update `ReadFrame` / `ReadFrameWithTimeout` to accept a size limit parameter, or add a `ReadControlFrame` variant that enforces the smaller limit
- [x] Apply the control frame limit to all reads that occur before and during handshake in `client.go` and `server.go`
- [x] Apply the control frame limit to all non-`file_data` message reads in the main connection read loop in `connection.go`
- [x] Add tests that send an oversized control frame and verify the connection is terminated cleanly

### 1.5 — Enable SQLite WAL Mode

This is a single-line change with significant stability impact. WAL mode reduces lock contention between the UI's 2-second poll loop and the network manager's write operations, and provides better crash resilience characteristics.

- [x] Add `PRAGMA journal_mode=WAL` execution at database open time in `storage/database.go`
- [x] Add an optional periodic `PRAGMA wal_checkpoint(TRUNCATE)` call (e.g., on startup and every 24 hours) to keep the WAL file bounded
- [x] Verify that existing tests pass under WAL mode and that concurrent read/write patterns behave correctly

---

## Phase 2 — Trust Model & Key Management Cleanup

This phase addresses how the app handles peer identity over time. The changes primarily touch `network/peer_manager.go`, `network/handshake.go`, `storage/peers.go`, `storage/database.go`, and `config/config.go`. The theme is ensuring that key material is stored intentionally and that trust decisions are durable and auditable.

### 2.1 — Persist Key Rotation Decisions

When a user accepts a changed peer key during handshake, the pinned key in the `peers` table is not reliably updated. This causes the same trust warning to reappear on the next connection, which trains users to click "trust" reflexively — exactly the opposite of good security behavior.

- [x] Update the key-change acceptance path in `peer_manager.go` (both the incoming handshake handler and the outbound dial path) so that when the user trusts a new key, the `ed25519_public_key` and `key_fingerprint` columns in the `peers` table are updated immediately via `storage/peers.go`
- [x] Create a `key_rotation_events` table in `storage/database.go` with schema migration, containing `peer_device_id`, `old_key_fingerprint`, `new_key_fingerprint`, `decision` (trusted/rejected), and `timestamp`
- [x] Write a rotation event to this table on every key-change decision (both trust and reject) for audit purposes
- [x] Add a helper in `storage/peers.go` to query recent key rotation events for a given peer, for potential future UI surfacing
- [x] Add tests that simulate a key change, verify the DB is updated, and verify no re-prompt occurs on the next connection

### 2.2 — Remove the Unused Persisted X25519 Private Key

The app generates and stores a persistent X25519 private key file, but the handshake exclusively uses freshly generated ephemeral X25519 keys. This dead key file serves no purpose and could cause confusion during security review. Since the ephemeral-only approach is actually stronger for forward secrecy, the persistent key should be removed rather than incorporated.

- [x] Remove the `X25519PrivateKeyPath` field from `DeviceConfig` in `config/config.go`
- [x] Remove the X25519 key generation and file persistence from first-run setup in `main.go` and `crypto/keypair.go`
- [x] Remove the normalization logic for the X25519 path in `config.go`'s `normalizeDefaults`
- [x] Update key reset in `ui/settings.go` to no longer regenerate the X25519 persistent key
- [x] Add a config migration step that silently ignores the `x25519_private_key_path` field if present in an older `config.json` (backward compatibility for existing users)
- [x] Verify handshake still functions correctly since it already uses ephemeral keys generated in `crypto/ecdh.go`

### 2.3 — Structured Security Event Logging

Security-relevant events (key changes, rejected signatures, replay rejections, handshake failures, rate limit triggers) are currently logged to stdout with no structured format or queryable history. For a security-focused app, these events should be durable and reviewable.

- [x] Create a `security_events` table in `storage/database.go` with columns: `id`, `event_type`, `peer_device_id` (nullable), `details` (JSON text), `severity` (info/warning/critical), `timestamp`
- [x] Define a `LogSecurityEvent` method on the store that writes to this table
- [x] Instrument key points in `peer_manager.go`: handshake failures, signature verification failures, replay rejections, key rotation decisions, and connection rate limit triggers
- [x] Add a `GetSecurityEvents` query method with optional filters (by type, peer, severity, time range) for future UI or export use
- [x] Add a retention policy that prunes security events older than a configurable horizon (default 90 days)

---

## Phase 3 — Per-Peer Settings & File Transfer Configuration

This phase introduces the user-facing trust and file management settings that make the app practical for daily use between trusted devices. The changes touch `config/config.go`, `storage/database.go`, `storage/peers.go`, `network/peer_manager.go`, `network/file_transfer.go`, `ui/settings.go`, `ui/peers_list.go`, and `ui/chat_window.go`.

### 3.1 — Global File Transfer Settings

Users need control over where files are saved and how large incoming files can be before they're automatically rejected. These are app-wide defaults that apply to all peers unless overridden.

- [x] Add `download_directory` field to `DeviceConfig` in `config/config.go`, defaulting to `<data-dir>/files/`
- [x] Add `max_receive_file_size` field to `DeviceConfig` (int64, bytes; 0 means unlimited)
- [x] Update the settings UI in `ui/settings.go` to include a "Download Location" row with a folder picker dialog
- [x] Update the settings UI to include a "Max File Size" entry with common presets (100 MB, 500 MB, 1 GB, 5 GB, Unlimited) selectable via dropdown
- [x] Update `peer_manager.go`'s file request handler to check incoming `file_request.Filesize` against the configured limit and auto-reject with a descriptive message if exceeded
- [x] Update `file_transfer.go`'s receive path to use the configured download directory instead of the hardcoded `FilesDir`
- [x] Ensure existing file transfers in progress are not affected by a settings change mid-transfer

### 3.2 — Per-Peer Settings Table and Trust Profiles

This is the core feature for trusted peer usability. Each peer gets configurable behavior for file acceptance, size limits, and download location, so users can say "auto-accept everything from my phone" while requiring approval from other devices.

- [x] Create a `peer_settings` table in `storage/database.go` with schema migration, containing: `peer_device_id` (FK to peers), `auto_accept_files` (boolean, default false), `max_file_size` (int64, 0 = use global default), `download_directory` (text, empty = use global default), `custom_name` (text, empty = use peer's chosen name), `trust_level` (text: normal/trusted, default normal)
- [x] Add CRUD methods in `storage/peers.go`: `GetPeerSettings`, `UpdatePeerSettings`, `EnsurePeerSettingsExist` (creates default row on peer add)
- [x] Update `peer_manager.go`'s file request handler to check per-peer settings before prompting the user: if `auto_accept_files` is true and filesize is within the peer's `max_file_size` limit (or unlimited), accept automatically without invoking the `OnFileRequest` callback
- [x] Update the file receive path to resolve the download directory from per-peer settings first, falling back to global settings
- [x] Call `EnsurePeerSettingsExist` when a new peer is added (in the add-request acceptance flow)

### 3.3 — Peer Rename Feature

Users should be able to assign custom display names to peers after adding them. The peer's actual chosen name is preserved and shown as secondary context, but the renamed label takes priority everywhere in the UI.

- [x] The `custom_name` column in the `peer_settings` table (created in 3.2) stores the user-assigned name
- [x] Update `ui/peers_list.go` to display the custom name as the primary label when set, with the peer's original device name shown as smaller subtext beneath it
- [x] Update `ui/chat_window.go` to use the custom name in the chat header when set
- [x] Add a "Rename Peer" option to the peer context menu or peer settings dialog, implemented as a simple text entry dialog that writes to `peer_settings.custom_name`
- [x] Ensure that mDNS discovery name updates from the peer (if they change their device name) update the stored `device_name` in the `peers` table but do not overwrite the user's custom rename

### 3.4 — Per-Peer Settings UI

Surface the per-peer settings in an accessible dialog so users can configure trust per device.

- [x] Add a "Peer Settings" button or gear icon to the peer list item or chat header, opening a dialog for the selected peer
- [x] The dialog should display: peer's device name (read-only), custom name entry, device ID (read-only), fingerprint (read-only, formatted as grouped hex for readability), trust level toggle (Normal / Trusted)
- [x] The dialog should include file transfer settings: auto-accept files toggle, max file size override (entry with presets or "Use global default"), download directory override (folder picker or "Use global default")
- [x] Save changes immediately to the `peer_settings` table on dialog confirm
- [x] Show the peer's trust level as a subtle visual badge (e.g., a small shield icon) next to their name in the peer list for trusted peers

---

## Phase 4 — Network Resilience & Rate Limiting

This phase hardens the networking layer against abuse and instability. The changes are concentrated in `network/server.go`, `network/connection.go`, `network/peer_manager.go`, and `discovery/peer_scanner.go`. These changes protect the app on shared or semi-trusted LANs.

### 4.1 — Connection and Request Rate Limiting

Without rate limits, a malicious device on the LAN can flood the listener with connections or spam add requests, degrading performance and responsiveness.

- [ ] Add a per-IP connection rate limiter to the TCP accept loop in `network/server.go` (e.g., max 5 new connections per IP per minute, using `golang.org/x/time/rate` or a simple token bucket)
- [ ] Add a per-connection malformed frame counter in `network/connection.go` that disconnects the peer after 3 consecutive invalid frames
- [ ] Add a per-peer cooldown for add requests in `peer_manager.go` (e.g., one add request per peer device ID per 30 seconds, rejecting duplicates silently)
- [ ] Add a per-peer cooldown for file requests (e.g., max 5 file requests per minute from the same peer)
- [ ] Log rate limit triggers to the security events table (from Phase 2.3)
- [ ] Add tests that simulate rapid connection floods and verify the rate limiter engages correctly

### 4.2 — Jittered Exponential Backoff with Endpoint Health Scoring

The fixed reconnect backoff sequence creates synchronized retry storms when multiple peers recover simultaneously. Adding jitter and preferring recently-successful endpoints improves reconnect reliability.

- [ ] Modify the reconnect backoff logic in `peer_manager.go` to add ±25% random jitter to each interval (e.g., a 60s base becomes 45–75s)
- [ ] Add an `endpoint_health` field to the in-memory peer connection state tracking: increment on successful connect, decrement (with floor of 0) on failure
- [ ] When a peer has been discovered at multiple addresses (via mDNS), sort candidate addresses by health score and attempt the highest-scored first
- [ ] Add a cap on maximum reconnect attempts before giving up (e.g., 50 attempts), after which the peer is left in offline state until the next mDNS discovery event triggers a fresh attempt
- [ ] Add tests that verify jitter is applied and that health scoring affects endpoint selection order

### 4.3 — Harden mDNS Privacy Surface

mDNS currently advertises `device_id` and `key_fingerprint` as stable identifiers in cleartext TXT records. Any device on the LAN can passively track GoSend presence over time.

- [ ] Remove `key_fingerprint` from the mDNS TXT records in `discovery/mdns.go` — this value is exchanged during the handshake and does not need to be broadcast
- [ ] Replace the raw `device_id` in TXT records with a rotating discovery token derived from HMAC(device_id, current_hour) using a key derived from the Ed25519 identity, so passive observers see a changing value
- [ ] Update `discovery/peer_scanner.go` to verify rotating tokens from known peers by trying HMAC verification against stored peer public keys and device IDs
- [ ] Ensure unknown peers (not yet added) can still be discovered — the instance name and service type remain sufficient for the discovery dialog
- [ ] Add tests that verify token rotation works across hour boundaries and that known peers are still correctly identified

### 4.4 — Persist Outbound File Transfer State for Crash Recovery

If the app crashes during a large file transfer, all progress is lost because outbound transfer state lives only in memory. Persisting checkpoints enables resume after restart.

- [ ] Create a `transfer_checkpoints` table in `storage/database.go` with: `file_id`, `direction` (send/receive), `next_chunk`, `bytes_transferred`, `temp_path`, `updated_at`
- [ ] Update `file_transfer.go`'s send loop to write a checkpoint every 50 chunks or 10 MB (whichever comes first)
- [ ] Update `file_transfer.go`'s receive side to write a checkpoint on each successfully received chunk (or batched every N chunks for performance)
- [ ] On startup, scan for incomplete transfer checkpoints in `peer_manager.go` and register them as resumable; when the relevant peer reconnects, re-initiate using the `resume_from_chunk` mechanism in `FileResponse`
- [ ] Clean up checkpoint rows when a transfer completes or fails definitively
- [ ] Add tests for crash-resume scenarios: simulate a partial transfer, restart, and verify the transfer resumes from the checkpoint rather than the beginning

---

## Phase 5 — Multi-File Transfer, Folder Transfer & Drag-and-Drop

This phase extends the file transfer system to support batch operations and folder structure preservation, and adds drag-and-drop as the primary interaction method for sending files and folders. The changes touch `network/protocol.go`, `network/file_transfer.go`, `network/peer_manager.go`, `ui/chat_window.go`, `ui/file_handler.go`, and `storage/files.go`.

### 5.1 — Multi-File Transfer Queue

Currently, file transfer is one file at a time. Users should be able to select multiple files and have them queued for sequential transfer.

- [ ] Add a `transfer_queue` structure in `peer_manager.go` (or a dedicated `transfer_queue.go`) that holds a per-peer ordered list of pending file send operations
- [ ] Update `SendFile` to enqueue rather than immediately start a transfer; a background worker per peer pops from the queue and initiates the next transfer when the current one completes
- [ ] Update `ui/file_handler.go` to support the native file picker returning multiple file selections
- [ ] Update `ui/chat_window.go` to invoke multi-file send when multiple paths are returned from the picker
- [ ] Show queued transfers in the chat window as pending transfer cards (filename + "Waiting..." status) that update to active progress when their turn arrives
- [ ] Add the ability to cancel a queued transfer before it starts

### 5.2 — Folder Transfer with Structure Preservation

Transferring an entire folder should preserve the directory hierarchy on the receiving end. This is modeled as a batch of file transfers wrapped in a folder metadata envelope.

- [ ] Define a `folder_transfer_request` message type in `protocol.go` containing: `folder_id`, `folder_name`, `total_files`, `total_size`, list of relative file paths with individual sizes, `from_device_id`, `to_device_id`, `timestamp`, `signature`
- [ ] Define a `folder_transfer_response` message type (accepted/rejected) with the same signing pattern
- [ ] On the sender side in `file_transfer.go`, recursively enumerate the folder, build the manifest, send the folder request, and upon acceptance, queue each file with its relative path as metadata
- [ ] On the receiver side, create the root folder in the download directory, then accept each file transfer and write to the correct relative path within that folder
- [ ] Handle edge cases: empty subdirectories (send a manifest entry with zero size), symlinks (skip or follow based on a policy), and filename sanitization to prevent path traversal (reject any relative path containing `..` or absolute paths)
- [ ] Update `storage/files.go` to store folder transfer metadata linking individual file records to their parent folder transfer

### 5.3 — Drag-and-Drop for Files and Folders

Drag-and-drop onto the chat window should be the primary way to initiate file and folder transfers. Dropping a file triggers a single file send; dropping a folder triggers the folder transfer flow from 5.2; dropping multiple items queues them all.

- [ ] Implement the Fyne `desktop.DragContainer` or `widget.BaseWidget` `Droppable` interface on the chat window's content area in `ui/chat_window.go`
- [ ] On drop, inspect each dropped URI: classify as file or directory
- [ ] For single files, initiate a standard `SendFile` (or enqueue into the multi-file queue)
- [ ] For directories, initiate the folder transfer flow from 5.2
- [ ] For multiple mixed items (files and folders), enqueue all of them into the transfer queue in the order they were dropped
- [ ] Show a visual drop target indicator (e.g., a highlighted border or overlay text "Drop files here") when a drag enters the chat area
- [ ] Add tests (or at minimum manual test scripts) covering drag of single file, multiple files, single folder, and mixed items

### 5.4 — Transfer Queue Visibility and Management

When multiple transfers are active or queued, users need visibility into what's happening and the ability to manage the queue.

- [ ] Add a transfer queue panel accessible from the status bar or a dedicated icon in the top bar of the UI
- [ ] The panel lists all active and pending transfers (both send and receive) grouped by peer, showing: filename, direction (sending/receiving), progress bar with percentage, transfer speed, and estimated time remaining for active transfers
- [ ] Each queued or active transfer should have a cancel button that either dequeues it (if pending) or aborts it (if in progress) with a confirmation prompt
- [ ] Completed transfers remain in the list briefly (e.g., 30 seconds) with a "Complete" badge before being removed, or can be cleared manually
- [ ] Failed transfers show an error message and a retry button

---

## Phase 6 — UX Polish, Notifications & Data Management

This phase focuses on the user experience layer and operational hygiene features. The changes touch `ui/main_window.go`, `ui/chat_window.go`, `ui/settings.go`, `ui/peers_list.go`, `storage/database.go`, `storage/messages.go`, and `config/config.go`. These are the changes that elevate the app from functional to polished.

### 6.1 — Inline File Transfer Progress in Chat

File transfers should appear as rich inline cards within the chat transcript, not just status text. This gives users immediate visual feedback in the context where they initiated the transfer.

- [ ] Define a new chat row widget in `ui/chat_window.go` for file transfers, displaying: filename, file size, a progress bar, transfer speed, and status text (Waiting / Sending / Receiving / Complete / Failed)
- [ ] For completed received files, render the filename as a clickable link that opens the file's containing folder (using `fyne.CurrentApp().OpenURL` or OS-specific open command)
- [ ] For failed transfers, show a retry button inline that re-initiates the send
- [ ] Wire the `OnFileProgress` callback from `PeerManager` into the chat view to update the progress bar in real time
- [ ] Ensure that transfer cards are persisted in the chat history (they already have entries in the `files` table) and display correctly when scrolling back through history

### 6.2 — Desktop Notifications

When the app is not in focus, incoming messages and file requests should produce system-level desktop notifications so users don't miss them.

- [ ] Implement desktop notifications using `fyne.CurrentApp().SendNotification()` for incoming messages when the app window is not focused or the user is viewing a different peer's chat
- [ ] Implement notifications for incoming file requests (showing filename and sender name)
- [ ] Add a global notification toggle in `ui/settings.go` (on/off, default on)
- [ ] Add a per-peer notification mute option in the peer settings (from Phase 3.4), so users can silence noisy peers while keeping notifications for others
- [ ] Ensure notifications do not fire for auto-accepted file transfers from trusted peers (to avoid notification spam for devices that are configured to auto-accept)

### 6.3 — Fingerprint Verification and Copy

The fingerprint display is functional but minimal. Making it easy to compare fingerprints out-of-band directly improves the security of the trust model.

- [ ] Update fingerprint display throughout the UI to use grouped hex format (e.g., `AB12 CD34 EF56 7890`) instead of a continuous string, for easier verbal comparison
- [ ] Add a "Copy Fingerprint" button next to the fingerprint display in the device settings dialog
- [ ] In the peer settings dialog (from Phase 3.4), show both the local fingerprint and the peer's fingerprint side by side with copy buttons for each, so users can compare them during a phone call or in-person meeting
- [ ] Add a "Verified" toggle in the peer settings that the user can set after manually verifying fingerprints out-of-band; once set, display a small checkmark badge next to the peer's name in the peer list
- [ ] Store the `verified` boolean in the `peer_settings` table; automatically reset it to false if the peer's key ever changes (key rotation from Phase 2.1)

### 6.4 — Connection State Indicators

The peer list currently shows online/offline. Users should see more granular states to understand what the app is doing, especially during reconnection or file transfer.

- [ ] Add "Connecting..." state display in `ui/peers_list.go` for peers that are mid-handshake
- [ ] Add "Reconnecting..." state with an indication of backoff progress (e.g., "Reconnecting in 45s...")
- [ ] Add "Transferring..." state with a small inline progress indicator for peers with active file transfers
- [ ] Map these visual states from the internal connection lifecycle states (`CONNECTING`, `READY`, `IDLE`, `DISCONNECTING`) and from the file transfer manager's active transfer tracking
- [ ] Ensure state transitions update the peer list promptly rather than waiting for the 2-second poll interval (consider event-driven updates for connection state changes)

### 6.5 — Data Retention and Cleanup Policies

Message history, file metadata, and the seen-message-ID table grow indefinitely. Users should be able to configure automatic cleanup, and the app should maintain itself without manual intervention.

- [ ] Add `message_retention_days` field to `DeviceConfig` in `config/config.go` with options: 30, 90, 365, and 0 (forever, the default)
- [ ] Add `cleanup_downloaded_files` boolean to `DeviceConfig` (default false) — when enabled, files older than the retention period are deleted from disk
- [ ] Implement a background cleanup job in `peer_manager.go` or a dedicated `maintenance.go` that runs on startup and then once every 24 hours
- [ ] The cleanup job deletes messages from `storage/messages.go` older than the retention period, prunes `seen_message_ids` entries older than 14 days, removes completed file metadata older than the retention period, and optionally deletes the corresponding files from disk
- [ ] Add the retention settings to the settings UI in `ui/settings.go` as a dropdown (30 days / 90 days / 1 year / Keep forever)
- [ ] Add a manual "Clear Chat History" button per peer in the peer settings dialog, with a confirmation prompt

### 6.6 — Chat History Search

As conversations grow, finding a specific message or file becomes important. A simple search feature makes long-lived peer relationships manageable.

- [ ] Add a search bar to the top of the chat view in `ui/chat_window.go`, toggled by a search icon button
- [ ] Implement search in `storage/messages.go` using SQL `LIKE` matching against the `content` column, filtered by the selected peer's device ID
- [ ] Display search results as a filtered view of the chat transcript, with matched terms highlighted if Fyne's text rendering supports it (otherwise just show the filtered list)
- [ ] Add a filter toggle to show only file transfers in the chat view (queries the `files` table for the selected peer)
- [ ] Ensure search is performant by adding an index on `messages(from_device_id, to_device_id, content)` if not already present

### 6.7 — Graceful Settings Hot-Reload (Device Name)

Currently all settings changes require a restart. At minimum, device name changes should apply immediately since they don't require rebinding any network resources.

- [ ] Update the settings save path in `ui/settings.go` to detect when only the device name changed (no port change)
- [ ] When only the name changed, propagate the new name to the mDNS advertiser in `discovery/mdns.go` by restarting the mDNS registration with the updated instance name
- [ ] Update active connections to use the new name for future protocol messages (update `PeerManager.options.Identity.DeviceName`)
- [ ] Remove the "Restart required" prompt when only the name was changed; keep it only for port changes
- [ ] Add a test that verifies a name change is reflected in mDNS advertisements without a full restart

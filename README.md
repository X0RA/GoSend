# GoSend — P2P Local Network Chat

This README reflects the current repository state (module `gosend`) as of February 8, 2026.

GoSend is a local-network peer-to-peer desktop app built in Go. It uses mDNS for discovery, TCP for transport, a signed handshake with ephemeral X25519 key exchange, encrypted framed traffic, SQLite persistence, and a Fyne GUI. It is designed for same-LAN usage with authenticated peers, encrypted transport, and durable local history.

**Runtime story:** Startup resolves data dir, loads/creates config, ensures Ed25519 identity keys, computes fingerprint, opens SQLite, then starts the GUI. The GUI starts the peer manager (TCP listener + protocol engine) and mDNS discovery (broadcast + scanner). The scanner discovers LAN peers and keeps a live cache. The user explicitly adds a discovered peer (TCP dial + signed handshake + add-request). After mutual acceptance, the peer is persisted and eligible for auto-reconnect. Messages and files are exchanged over encrypted sessions with signatures, replay checks, and persistence. If a peer goes offline, outbound messages/files are queued and resume on reconnect. On shutdown, the UI stops the manager and discovery, waits for loops, then closes the DB.

## Roadmap Completion

All roadmap phases in `PHASE-CHANGES.md` are implemented.

- Phase 1: protocol hardening (signed control frames, handshake challenge nonce, rekey, control/data frame limits, WAL).
- Phase 2: trust/key lifecycle cleanup (durable key-rotation decisions, structured security events, legacy X25519 config cleanup).
- Phase 3: global + per-peer transfer policy and trust/profile settings.
- Phase 4: network resilience and abuse protection (rate limits, jittered reconnect health scoring, mDNS privacy token, crash-resume checkpoints).
- Phase 5: multi-file queueing, folder transfer manifests, drag-and-drop, and transfer queue management.
- Phase 6: UX polish (inline transfer cards, notifications, fingerprint verification workflow, runtime state indicators, retention cleanup, chat search, hot-reload device name).

## What Is Implemented Today

- Local LAN peer discovery via mDNS service `_p2pchat._tcp.local.` with TXT keys: `discovery_token`, `version` (no raw `device_id` or `key_fingerprint` broadcast).
- Challenge-response handshake (`handshake_challenge`, `handshake`, `handshake_response`) with Ed25519 identities and ephemeral X25519 keys.
- Session key derivation via HKDF-SHA256 using salt `p2pchat-session-v1`, sorted device IDs, and the per-connection handshake challenge nonce.
- Transport-level encryption for all post-handshake frames using AES-256-GCM (`secure_frame`).
- Automatic signed rekey exchange (`rekey_request`, `rekey_response`) after 1 hour or 1 GiB sent, with secure-frame epoch tagging and previous-epoch decrypt window during transition.
- Separate frame limits: `64 KB` for control frames and `10 MB` for data frames.
- Peer lifecycle flows: add request/response, remove, disconnect, reconnect workers, and discovery-driven reconnect.
- Network abuse protections: per-IP inbound connection rate limiting, per-peer add-request cooldown, per-peer file-request rate limiting, and disconnect-on-3-consecutive malformed frames.
- Reconnect improvements: jittered exponential backoff (±25%), endpoint health scoring across discovered addresses, and reconnect-attempt cap (50 by default, reset by fresh discovery).
- Peers are only persisted after an accepted `peer_add_request`; a raw connection/handshake alone does not auto-add a peer.
- Encrypted + signed text messages with delivery status tracking (`pending`, `sent`, `delivered`, `failed`).
- Offline outbound queue with limits: 500 pending messages per peer, 7-day age cutoff, 50 MB pending content per peer.
- Replay defenses for chat messages: per-connection sequence checks, persistent seen ID table, and timestamp skew checks.
- Trusted key rotations persist immediately to the pinned peer key plus an auditable `key_rotation_events` trail.
- Structured `security_events` logging captures handshake failures, signature failures, replay rejections, key-rotation decisions, and queue-limit triggers.
- Per-peer outbound file transfer queue (sequential send worker), queued-transfer cancel/retry, and multi-file picker enqueue flow.
- Folder transfer envelope (`folder_transfer_request` / `folder_transfer_response`) with signed manifests, structure-preserving receive paths, and traversal-safe relative-path validation.
- File transfer with request/accept/reject, signed chunk ack/nack + signed file completion, reconnect resume from chunk, end checksum verification, persistent transfer checkpoints for crash recovery, global receive limits/download location, and per-peer auto-accept/limit/directory overrides.
- Chat drag-and-drop (single/multi file + folder) plus transfer queue panel with grouped active/pending visibility, speed/ETA display, cancel/retry controls, and short completed-item retention.
- Inline chat transfer cards are persisted from `files` metadata, show live speed/progress/status, support inline retry, and open the containing folder for completed received files.
- Desktop notifications for incoming messages and file requests when the app is in background or another peer chat is active, with global enable/disable and per-peer mute (auto-accepted inbound transfers do not trigger file-request notifications).
- Per-peer controls include custom names, trust level (`normal`/`trusted`), notification mute, fingerprint verification flag, trusted badge, and verified badge in the peer list.
- Peer list state indicators include `Connecting...`, reconnect countdown (`Reconnecting in Ns...`), and inline `Transferring...` progress.
- Chat search includes message `LIKE` search plus a files-only filter backed by `files` queries.
- Configurable data retention and maintenance cleanup (messages/files/seen IDs) plus per-peer clear-history action.
- Device-name settings hot-reload updates active protocol identity and restarts mDNS advertisement without full app restart (port changes still require restart).
- Runtime state change callbacks refresh peer status indicators immediately while the regular 2-second poll keeps chat/history views synchronized.
- Cross-platform GUI (Fyne) with peer list, chat pane, discovery dialog, device settings dialog, peer settings dialog, key-change prompt, file-transfer prompts/progress, and chat header color that reflects online status.

## Quick Start

Build:

```bash
make build
```

Run one client using default data directory:

```bash
./bin/gosend
```

Run two isolated clients locally:

```bash
make build
P2P_CHAT_DATA_DIR=/tmp/gosend-a ./bin/gosend
P2P_CHAT_DATA_DIR=/tmp/gosend-b ./bin/gosend
```

Or use the Make targets:

```bash
make run_client_a
make run_client_b
```

## Development Commands

Repository-provided:

```bash
make build
make test_non_ui
make run_client_a
make run_client_b
```

Standard Go commands used in this project:

```bash
gofmt -w .
go test ./...
go vet ./...
go mod tidy
```

Manual UI verification for transfer/search/notification UX:

1. Drag one file into an active chat, verify a queued card appears as `Waiting...` then progresses to `complete`.
2. Drag multiple files together, verify they preserve drop order and run sequentially.
3. Drag a folder containing nested files + an empty subdirectory, verify receiver preserves structure.
4. Drag mixed items (files + folders), verify all items enqueue in the original drop order.
5. Open the transfer queue panel and verify cancel/retry actions and 30-second completed-item retention behavior.
6. Toggle chat search, query a known message term, and verify transcript rows are filtered.
7. Enable the files-only search filter and verify only transfer cards are shown for the selected peer.
8. Background the app or switch to another peer chat, send a message/file request from another client, and verify desktop notifications fire (respecting per-peer mute/global toggle).
9. In peer settings, toggle Verified and verify `[Verified]` badge appears in the peer list.
10. Change only device name in settings and verify no restart prompt appears and mDNS-discovered name updates on other clients.

## GitHub Release Automation

This repository includes a release workflow at `.github/workflows/release-build.yml`.

When a GitHub Release is published, the workflow:

- Uses the release tag (`github.event.release.tag_name`).
- Builds a Linux binary (`linux/amd64`) and a macOS Apple Silicon app bundle archive (`darwin/arm64` as `.app.zip`).
- Uploads those artifacts to that same GitHub Release as assets.
- Restores/saves Go dependency/build caches via `actions/setup-go` using `go.sum` as the cache dependency key.
- The macOS AMD64 and Windows matrix entries are currently commented out.

Expected asset names:

- `gosend-<tag>-linux-amd64`
- `gosend-<tag>-darwin-arm64.app.zip`

Tag notes:

- Git tags cannot contain spaces. For prereleases, prefer tags like `v0.0.1-alpha` (not `v0.0.1 alpha`).
- The macOS `.app` bundle is ad-hoc signed (not Developer ID signed/notarized); users may still need to right-click `Open` on first launch.

## Runtime Data and Config

Data directory resolution (`config.ResolveDataDir`):

- `P2P_CHAT_DATA_DIR` override if set.
- Linux: `${XDG_CONFIG_HOME:-~/.config}/p2p-chat`
- macOS: `~/Library/Application Support/p2p-chat`
- Windows: `%APPDATA%\p2p-chat` (fallback `~/AppData/Roaming/p2p-chat`)

Expected layout:

```text
<data-dir>/
  app.db
  config.json
  keys/
    ed25519_private.pem
    ed25519_public.pem
  files/
```

Default first-run config values:

- `device_id`: generated UUID.
- `device_name`: hostname (fallback `P2P Chat Device`).
- `port_mode`: `automatic`.
- `listening_port`: `0` (ephemeral OS-selected port).
- `download_directory`: `<data-dir>/files`.
- `max_receive_file_size`: `0` (unlimited).
- `notifications_enabled`: `true`.
- `message_retention_days`: `0` (`Keep forever`).
- `cleanup_downloaded_files`: `false`.
- Key paths are under `<data-dir>/keys`.
- `key_fingerprint`: computed at startup from Ed25519 public key and persisted.

`config.json` fields:

```json
{
  "device_id": "string",
  "device_name": "string",
  "port_mode": "automatic|fixed",
  "listening_port": 0,
  "download_directory": "string",
  "max_receive_file_size": 0,
  "notifications_enabled": true,
  "message_retention_days": 0,
  "cleanup_downloaded_files": false,
  "ed25519_private_key_path": "string",
  "ed25519_public_key_path": "string",
  "key_fingerprint": "hex"
}
```

Normalization (for older configs):

- Missing `port_mode` with a positive `listening_port` becomes `fixed`.
- Fixed mode with `listening_port=0` is normalized to `9999` (`DefaultListeningPort`).
- Automatic mode never requires a fixed port.
- Missing key path fields are backfilled to the default keys directory.
- Missing `download_directory` is backfilled to `<data-dir>/files`.
- Negative `max_receive_file_size` values are normalized to `0` (unlimited).
- Missing `notifications_enabled` is backfilled to `true`.
- Invalid `message_retention_days` values are normalized to `0` (`Keep forever`).
- Legacy `x25519_private_key_path` fields are silently removed when old configs are rewritten.

Runtime application:

- Config is loaded at startup; fingerprint is recomputed from the Ed25519 public key and persisted if changed.
- Device-name changes from Settings are applied at runtime (peer manager identity update + mDNS re-registration) without restart.
- Port changes from Settings are saved immediately and require restart to rebind the TCP listener.
- Download location and max receive file size changes are saved immediately and used for future incoming file requests; active transfers continue with the paths/limits resolved when that request was accepted.

## Protocol and Security (Current Behavior)

Transport:

- TCP listener binds `0.0.0.0:<port>`; port from config (automatic or fixed).
- Length-prefixed frames: `[4-byte big-endian length][JSON payload]`.
- Control frame payload limit: `64 KB`; data frame payload limit: `10 MB`; frame read timeout default `30s`.

Connection states and keepalive:

- States: `CONNECTING`, `READY`, `IDLE`, `DISCONNECTING`, `DISCONNECTED`.
- Idle keepalive ping every `60s`; pong timeout `15s`. Auto-respond-to-ping is configurable in handshake options.
- Automatic session rekey trigger: every `1h` or `1 GiB` sent on a connection (whichever occurs first).

Handshake flow:

1. Listener accepts TCP and sends `handshake_challenge` with a random 32-byte nonce.
2. Dialer reads the challenge and sends `handshake` (identity + ephemeral X25519 public key + echoed challenge nonce + signature).
3. Listener verifies nonce/version/signature/key-pin policy and replies `handshake_response`.
4. Both derive the same session key and switch to encrypted `secure_frame` transport.
- Protocol version must equal `1`; version mismatch yields typed `error` with supported versions.

Cryptography:

- **Identity**: Ed25519 long-term signing key; private key PEM `0600`, public PEM `0644`. Fingerprint = first 16 bytes of SHA-256(Ed25519 public key), hex.
- **Session key**: Ephemeral X25519 keypairs per connection; shared secret via ECDH; initial session key = HKDF-SHA256 over shared secret with salt `p2pchat-session-v1`, info = sorted device IDs (`minID|maxID`) + handshake challenge nonce. Rekeyed session keys use signed ephemeral exchange context (`rekey|<epoch>`), 32-byte output.
- **Encryption**: After handshake, every payload is in `secure_frame` (epoch + base64 nonce + ciphertext). AES-256-GCM with random nonce per encryption; plaintext is the JSON protocol message. File chunk payloads (`file_data`) are encrypted with the session key and then wrapped in `secure_frame` again.
- **Trust**: TOFU with pinned Ed25519 key in peer record. On key mismatch, handshake is blocked until user trust/reject; trusted decisions immediately update the pinned key/fingerprint and are written to `key_rotation_events`.
- **X25519**: Handshake/rekey key exchange is ephemeral-only per session; no persistent X25519 private key file is stored.

Message/control integrity:

- Signed (Ed25519) and verified: handshake messages, `peer_add_request`, `peer_add_response`, `peer_remove`, `message`, `ack`, `rekey_request`, `rekey_response`, `file_request`, `file_response`, `file_complete`, `folder_transfer_request`, and `folder_transfer_response`.
- `file_response` validation requires signature + `from_device_id` sender binding for chunk ack/nack and request responses.
- Replay/tamper defenses: per-connection sequence numbers, timestamp skew ±5 minutes, persistent `seen_message_ids` dedupe.

## Discovery (mDNS)

LAN discovery uses `github.com/grandcat/zeroconf` (mDNS/DNS-SD).

Advertised:

- Service `_p2pchat._tcp`, domain `local.`; instance name = local `device_name`; port = active TCP listening port.
- TXT: `discovery_token` (hourly rotating HMAC derived from local Ed25519 identity + `device_id`) and `version=1`.

Scanner:

- Background browse plus manual refresh. Refresh interval `10s`; scan timeout per browse `5s`.
- Peers retained across transient misses; removed after stale timeout. Default TTL `120s`; effective stale = max(30s, 2×TTL, 6×refresh) → with defaults ~240s.

Filtering and events:

- Known peers are resolved by verifying the rotating token against stored `(device_id, Ed25519 public key)` pairs (current hour ±1 hour skew).
- Self entries are filtered by token verification.
- Unknown peers are still discoverable and shown with synthetic IDs derived from instance/host/endpoint metadata.
- Device name fallback order: mDNS instance name, hostname, then `Unknown Peer`. IPv4/IPv6 addresses are deduplicated and sorted. Events: `peer_upserted`, `peer_removed`.

Integration with networking:

- Discovery does not auto-add unknown peers. For known peers, discovered endpoint sets are fed to `NotifyPeerDiscoveredEndpoints`, which updates the stored endpoint candidate list and can trigger reconnect. Unknown peers require the explicit user add flow.

## Messaging, Queue, and Reconnect Behavior

Messaging:

- Outbound chat messages are encrypted (payload), signed, and sent as `message`.
- If send succeeds immediately: status `sent` then `delivered` after `ack`.
- If peer is offline/unavailable: message is stored as `pending`.

Queue enforcement:

- Called during send and queue drain.
- Prunes pending messages older than 7 days (marks them `failed`).
- Enforces per-peer max pending count (500) and bytes (50 MB).
- Oldest pending messages are marked `failed` when limits are exceeded.

Reconnect:

- Base backoff sequence: `0s -> 5s -> 15s -> 60s -> 60s...`, each interval jittered by ±25%.
- On disconnect: peer marked offline and reconnect worker starts (unless suppressed by manual remove/disconnect).
- Candidate endpoints are sorted by in-memory `endpoint_health` score (higher first) before each reconnect round; successful dials increment health and failures decrement with floor `0`.
- Reconnect attempts are capped at `50` by default; after cap exhaustion, retries pause until a fresh discovery event triggers a new worker.
- Discovery bridge (`NotifyPeerDiscoveredEndpoints`) updates stored endpoint candidates and triggers reconnect for known non-blocked peers.

## File Transfer Behavior

Outbound:

- `SendFile(peerID, path)` and `SendFiles(peerID, paths)` enqueue outbound files into a per-peer ordered queue; only one file is active per peer at a time.
- `SendFolder(peerID, folderPath)` builds a signed folder manifest, waits for `folder_transfer_response`, then enqueues accepted files with `folder_id` + `relative_path` metadata.
- Sends `file_request`; waits for `accepted` or `rejected`.
- On accept, resumes from `resume_from_chunk` if provided.
- Chunks are read from source file and encrypted as `file_data` with per-chunk nonce.
- Receiver responds with `chunk_ack` / `chunk_nack`.
- Each chunk retries up to `MaxChunkRetries` (default 3).
- Outbound checkpoints are persisted to `transfer_checkpoints` every 50 chunks or 10 MB, whichever comes first.
- Queued or active transfers can be canceled (`CancelTransfer`), and failed/rejected outbound transfers can be re-queued (`RetryTransfer`).
- After all chunks are sent, sender sends signed `file_complete` and waits for the receiver’s signed `file_complete` (after checksum and rename). Wait uses `FileCompleteTimeout` (default 5 minutes); chunk acks use `FileResponseTimeout` (default 10s). Default chunk size 256 KiB. If the receiver’s completion arrives after the sender timed out, the sender still applies completion so the UI updates.

Inbound:

- For each incoming `file_request`, the manager resolves peer settings (`peer_settings`) and computes effective receive policy.
- If `folder_id`/`relative_path` are present, the target file path is resolved under the accepted inbound folder root and validated to remain inside that root.
- Effective size limit is peer `max_file_size` when set (`>0`), otherwise global `max_receive_file_size`; oversized requests are auto-rejected with a descriptive message.
- File requests are rate-limited per peer (default 5/minute); excess requests are rejected and logged as security events.
- If within limit and `auto_accept_files=true`, the request is auto-accepted; otherwise the UI accept/reject callback is invoked.
- Accepted non-folder transfers write to `<resolved-download-dir>/<file_id>_<basename>.part`; folder-scoped transfers write to `<folder-root>/<relative_path>.part`. The resolved base directory is peer `download_directory` override first, then global `download_directory`.
- Inbound checkpoints are persisted in batches (every 10 chunks or 2 MB).
- On `file_complete`, receiver verifies checksum and renames `.part` to final file.

Folder transfer safety/policy:

- Folder manifests include file and directory entries with relative paths and sizes.
- Relative paths are normalized and rejected if absolute or traversal (`..`) is detected.
- Symlinks are skipped during folder enumeration.
- Empty directories are preserved via directory manifest entries.

Completion/failure:

- Success => both sides `transfer_status = complete`.
- Reject => `rejected` on both sides.
- Checksum mismatch/finalize failure => `failed` and sender receives failed completion.
- Resume after reconnect is supported via `resume_from_chunk`.
- Checkpoints are loaded at startup so interrupted transfers can resume automatically when the peer reconnects; checkpoint rows are removed on definitive completion/failure.

## SQLite Schema and Persistence

Persistence uses `github.com/mattn/go-sqlite3` with a single DB (`app.db`) under the app data directory. DSN enables foreign keys (`_foreign_keys=on`); busy timeout `5000ms`. The database is opened in WAL mode (`PRAGMA journal_mode=WAL`) with startup checkpointing (`PRAGMA wal_checkpoint(TRUNCATE)`) and a periodic 24-hour checkpoint loop. Schema migrations are versioned with `PRAGMA user_version`.

Tables store: **peers** — identity, pinned Ed25519 public key, fingerprint, status, timestamps, last known endpoint. **messages** — content, content type, sent/received timestamps, read flag, delivery status, signature. **files** — file IDs, sender/receiver, filename/type/size/path/checksum, transfer status, plus optional `folder_id`/`relative_path` linkage for folder sends. **folder_transfers** — folder envelope metadata (`folder_name`, root path, totals, status). **seen_message_ids** — replay defense. **key_rotation_events** — trusted/rejected key-change decisions with old/new fingerprints. **security_events** — structured security logs with severity and JSON details. **peer_settings** — per-peer transfer policy (`auto_accept_files`, `max_file_size`, `download_directory`) plus presentation/trust metadata (`custom_name`, `trust_level`, `notifications_muted`, `verified`). **transfer_checkpoints** — resumable send/receive progress (`next_chunk`, `bytes_transferred`, `temp_path`, `updated_at`). Status enums: peer `online`/`offline`/`pending`/`blocked`; delivery `pending`/`sent`/`delivered`/`failed`; file `pending`/`accepted`/`rejected`/`complete`/`failed`. Updates in practice: peer add accepted → peer row created/updated to `online` and default `peer_settings` ensured; disconnect → `offline`; message sent → `sent`, after ACK → `delivered`; offline messages stored as `pending` and drained on reconnect; file request/accept/reject/complete update `files.transfer_status`; folder request/response transitions update `folder_transfers.transfer_status`; received message IDs go into `seen_message_ids`; key-change trust/reject decisions write to `key_rotation_events` and reset `peer_settings.verified`; trusted decisions also update `peers.ed25519_public_key` + `peers.key_fingerprint`; checkpoint rows are upserted during transfers and removed on definitive completion/failure. Pending queue prune: messages older than 7 days marked `failed`. Security-event retention pruning defaults to 90 days. Maintenance cleanup (configurable retention) prunes old messages, old completed file metadata, and seen IDs older than 14 days.

Schema:

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

CREATE TABLE files (
  file_id            TEXT PRIMARY KEY,
  message_id         TEXT REFERENCES messages(message_id),
  folder_id          TEXT,
  relative_path      TEXT,
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

CREATE TABLE folder_transfers (
  folder_id        TEXT PRIMARY KEY,
  from_device_id   TEXT,
  to_device_id     TEXT,
  folder_name      TEXT NOT NULL,
  root_path        TEXT NOT NULL,
  total_files      INTEGER NOT NULL,
  total_size       INTEGER NOT NULL,
  transfer_status  TEXT NOT NULL CHECK(transfer_status IN ('pending','accepted','rejected','complete','failed')) DEFAULT 'pending',
  timestamp        INTEGER NOT NULL
);

CREATE TABLE seen_message_ids (
  message_id  TEXT PRIMARY KEY,
  received_at INTEGER NOT NULL
);

CREATE TABLE key_rotation_events (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  peer_device_id      TEXT NOT NULL REFERENCES peers(device_id) ON DELETE CASCADE,
  old_key_fingerprint TEXT NOT NULL,
  new_key_fingerprint TEXT NOT NULL,
  decision            TEXT NOT NULL CHECK(decision IN ('trusted','rejected')),
  timestamp           INTEGER NOT NULL
);

CREATE TABLE security_events (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type     TEXT NOT NULL,
  peer_device_id TEXT,
  details        TEXT NOT NULL,
  severity       TEXT NOT NULL CHECK(severity IN ('info','warning','critical')),
  timestamp      INTEGER NOT NULL
);

CREATE TABLE peer_settings (
  peer_device_id      TEXT PRIMARY KEY REFERENCES peers(device_id) ON DELETE CASCADE,
  auto_accept_files   INTEGER NOT NULL DEFAULT 0,
  max_file_size       INTEGER NOT NULL DEFAULT 0,
  download_directory  TEXT NOT NULL DEFAULT '',
  custom_name         TEXT NOT NULL DEFAULT '',
  trust_level         TEXT NOT NULL CHECK(trust_level IN ('normal','trusted')) DEFAULT 'normal',
  notifications_muted INTEGER NOT NULL DEFAULT 0,
  verified            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE transfer_checkpoints (
  file_id            TEXT NOT NULL,
  direction          TEXT NOT NULL CHECK(direction IN ('send','receive')),
  next_chunk         INTEGER NOT NULL DEFAULT 0,
  bytes_transferred  INTEGER NOT NULL DEFAULT 0,
  temp_path          TEXT NOT NULL DEFAULT '',
  updated_at         INTEGER NOT NULL,
  PRIMARY KEY (file_id, direction)
);
```

Indexes:

- `idx_messages_peer_time` on `(to_device_id, delivery_status, timestamp_sent)`
- `idx_seen_message_received_at` on `(received_at)`
- `idx_key_rotation_events_peer_time` on `(peer_device_id, timestamp DESC, id DESC)`
- `idx_security_events_time` on `(timestamp DESC, id DESC)`
- `idx_security_events_type` on `(event_type, timestamp DESC, id DESC)`
- `idx_security_events_peer` on `(peer_device_id, timestamp DESC, id DESC)`
- `idx_transfer_checkpoints_updated_at` on `(updated_at DESC, file_id, direction)`
- `idx_folder_transfers_peer_time` on `(to_device_id, from_device_id, timestamp DESC, folder_id)`
- `idx_messages_peer_content` on `(from_device_id, to_device_id, content)`
- `idx_files_peer_status_time` on `(to_device_id, from_device_id, transfer_status, timestamp_received DESC, file_id)`

## UI Summary

The GUI (Fyne) is a single-window desktop app that wires user intent to the networking, discovery, and storage layers.

Layout:

- Left pane: known peers list from DB (self filtered out), plus discovery dialog entry point. Rows use custom peer names when set, show the original device name as secondary text, append `[Trusted]` / `[Verified]` badges, and surface runtime states (`Connecting...`, reconnect countdown, transfer progress).
- Right pane: selected peer chat transcript (messages + persisted file transfer rows), compose box, Send and Attach actions.
- Top bar: app title, transfer queue panel, discovery refresh, settings. Bottom bar: global runtime feedback (errors, queue warnings, connect state).
- Text messages show a copy button (clipboard icon); file/image messages do not. Chat composer is hidden until a peer is selected.
- Chat header is clickable to show the selected peer fingerprint; its color reflects online/offline status; a peer-settings gear button appears when a peer is selected.
- Chat header includes a search toggle; search supports content filtering and a files-only mode.
- Composer: message box on the left, `Send` above `Attach` on the right. `Attach` supports multi-file enqueue; folders are supported via drag-and-drop. `Enter` sends; `Shift+Enter` newline.
- Hovering key buttons (peer info, peer settings, app settings, refresh, discover, send, attach) shows a tooltip and status hint.

Runtime loops:

- Discovery event loop: consumes mDNS scanner events and updates discovered peers.
- Pending add-request loop: surfaces incoming add requests to the confirm dialog.
- Manager error loop: streams async errors from the network manager to the status label.
- Poll loop (every 2s): refreshes peer list and current chat transcript from SQLite.

Dialogs: discovery (peers + Add, dark panels), incoming peer add confirmation, incoming file accept/reject, key-change trust/deny, device settings, peer settings.

Key flows: Add peer (discovery → select → dial → `peer_add_request`); incoming add (accept/reject). Message (Enter to send); files (multi-file picker → queued `SendFile`), folders (drag/drop directory → `SendFolder` manifest + queued files), transfer queue management (top-bar queue panel with cancel/retry), key change (trust/reject with fingerprints). Incoming file (auto-accept or accept/reject with name/size/type based on per-peer/global settings). Peer settings (chat-header gear → save custom name/trust/transfer policy).

Settings:

- Device settings include name, port mode (`automatic`/`fixed`), fixed port, global download location, global max receive file size, notifications toggle, message retention dropdown, cleanup policy toggle, fingerprint display/copy, and key reset (regenerates Ed25519 key files and updates fingerprint).
- Device name persists and hot-reloads at runtime (manager identity + mDNS broadcaster restart). Port changes persist immediately and require restart to rebind the listener.
- Download location/max receive file size and retention/notification settings are persisted immediately and affect future behavior without restart.
- Peer settings include read-only identity fields (device name/ID plus local+peer fingerprint copy), custom name, trust level, verified toggle, notifications mute, auto-accept files, max file size override, download directory override, and per-peer clear chat history.

Theme:

- Custom dark-leaning palette (online/offline colors, message bubbles, overlays). Tooltips use overlay management and delayed show.

## Repository Layout

```text
.
├── .github/
│   ├── Info.plist.template
│   └── workflows/
│       └── release-build.yml
├── AGENTS.md
├── Makefile
├── README.md
├── main.go
├── tools.go
├── go.mod
├── go.sum
├── bin/
│   └── gosend
├── config/
│   ├── config.go
│   └── config_test.go
├── crypto/
│   ├── ecdh.go
│   ├── ecdh_session_test.go
│   ├── encryption.go
│   ├── encryption_test.go
│   ├── keypair.go
│   ├── keypair_test.go
│   ├── signatures.go
│   └── signatures_test.go
├── discovery/
│   ├── mdns.go
│   ├── mdns_test.go
│   ├── peer_scanner.go
│   └── peer_scanner_test.go
├── models/
│   ├── file.go
│   ├── message.go
│   └── peer.go
├── network/
│   ├── client.go
│   ├── connection.go
│   ├── connection_test.go
│   ├── file_transfer.go
│   ├── file_transfer_test.go
│   ├── handshake.go
│   ├── integration_test.go
│   ├── messaging_test.go
│   ├── peer_manager.go
│   ├── peer_manager_test.go
│   ├── protocol.go
│   ├── protocol_test.go
│   ├── rekey_test.go
│   ├── server.go
│   └── server_test.go
├── storage/
│   ├── database.go
│   ├── database_test.go
│   ├── files.go
│   ├── files_test.go
│   ├── messages.go
│   ├── messages_test.go
│   ├── peers.go
│   ├── peers_test.go
│   ├── security_events.go
│   ├── security_events_test.go
│   ├── seen_ids.go
│   ├── seen_ids_test.go
│   ├── testutil_test.go
│   └── types.go
└── ui/
    ├── chat_window.go
    ├── clickable_label.go
    ├── file_handler.go
    ├── main_window.go
    ├── peers_list.go
    ├── settings.go
    └── theme.go
```

## File-by-File Reference

### Root Files

- `main.go`: Startup entrypoint. Loads/normalizes config, ensures Ed25519 key material, persists fingerprint updates, opens SQLite store, constructs local identity, and starts UI runtime.
- `Makefile`: Convenience commands for build and non-UI test flows, plus two client run targets with isolated data dirs.
- `.github/workflows/release-build.yml`: On release publish, builds release artifacts and uploads them to the created release as assets (Linux binary + macOS ARM `.app.zip`).
- `tools.go`: Build-tagged dependency anchors for `fyne`, `zeroconf`, and `go-sqlite3`.
- `go.mod`: Module `gosend`, Go version `1.25.6`, direct dependencies (Fyne, UUID, zeroconf, sqlite3, x/crypto).
- `go.sum`: Dependency checksum lockfile.
- `AGENTS.md`: Local coding-agent instructions for this repository.
- `README.md`: Project documentation.
- `bin/gosend`: Built binary artifact (generated by `make build`).

### `config/`

- `config/config.go`: Data dir resolution, directory creation, config load/save, default config generation, and config normalization/migration (`port_mode`, `download_directory`, `max_receive_file_size`, `notifications_enabled`, `message_retention_days`, `cleanup_downloaded_files`, and legacy X25519 field removal).
- `config/config_test.go`: Tests first-run creation, stable reload behavior, legacy normalization paths, missing notifications backfill, retention normalization, and legacy X25519 field migration cleanup.

### `crypto/`

- `crypto/keypair.go`: Ed25519 keypair ensure/load/save, fingerprint generation, and fingerprint formatting.
- `crypto/keypair_test.go`: Verifies Ed25519 key persistence stability across repeated ensure calls.
- `crypto/ecdh.go`: Ephemeral X25519 keypair generation, shared secret computation, and HKDF session key derivation.
- `crypto/ecdh_session_test.go`: Validates both sides derive identical shared secrets and session keys.
- `crypto/encryption.go`: AES-256-GCM encrypt/decrypt helpers with nonce generation and validation.
- `crypto/encryption_test.go`: Round-trip encryption/decryption tests.
- `crypto/signatures.go`: Ed25519 sign/verify helpers with input validation.
- `crypto/signatures_test.go`: Tests valid signature verification and tamper rejection.

### `discovery/`

- `discovery/mdns.go`: mDNS config defaults, rotating discovery-token generation, broadcaster setup, runtime `UpdateDeviceName` re-registration, and combined discovery service startup/shutdown orchestration.
- `discovery/mdns_test.go`: Tests rotating-token TXT composition, token rotation/verification, service start/stop wiring, device-name update re-registration, and stale-time default behavior from TTL.
- `discovery/peer_scanner.go`: Background/manual scanning, known-peer token verification, unknown-peer synthetic IDs, in-memory peer cache, stale-peer aging, and event emission.
- `discovery/peer_scanner_test.go`: Tests self filtering via token verification, known-peer resolution, unknown-peer fallback discovery, manual refresh updates, and polling/removal behavior.

### `models/`

- `models/peer.go`: JSON model struct for peer payload shape.
- `models/message.go`: JSON model struct for message payload shape.
- `models/file.go`: JSON model struct for file metadata payload shape.

Note: runtime logic currently uses `storage` structs directly; `models` package is present but not currently imported by active runtime code.

### `storage/`

- `storage/types.go`: Core storage data types, status/content constants, folder-transfer metadata structs, peer-trust constants, validators, null helpers, and shared errors.
- `storage/database.go`: SQLite open/create helpers, WAL mode + periodic checkpointing, migrations (`peer_settings` extensions, `transfer_checkpoints`, `folder_transfers`, `files` folder-link columns, and search/retention indexes), schema versioning, and connection lifecycle.
- `storage/database_test.go`: Verifies DB creation, migrations, schema version, and expected table presence.
- `storage/peers.go`: Peer CRUD + endpoint/status updates, discovery-driven device-name updates, per-peer settings CRUD/default ensure (including notifications mute + verified), pinned-key updates, verified reset on key change, and key-rotation event history helpers.
- `storage/peers_test.go`: Peer CRUD, endpoint/status update, per-peer settings CRUD/defaults, verified reset behavior, pinned-key update, and key-rotation event tests.
- `storage/messages.go`: Message insert/read/update APIs, pending queue reads, search APIs, retention deletion helpers, per-peer history deletion, and expired queue pruning.
- `storage/messages_test.go`: Message CRUD, ordering, status updates, pending retrieval, search behavior, and retention/peer delete behavior.
- `storage/files.go`: File metadata insert/read (including `folder_id`/`relative_path`), peer file list/search APIs, timestamp updates, retention/peer metadata deletion helpers, folder-transfer metadata upsert/query/status helpers, and transfer-checkpoint CRUD/list APIs.
- `storage/files_test.go`: File metadata CRUD/status update tests plus transfer-checkpoint, folder-transfer linkage, and retention/peer-query coverage.
- `storage/security_events.go`: Structured security-event insert/query/retention-prune helpers.
- `storage/security_events_test.go`: Security-event filter and retention behavior tests.
- `storage/seen_ids.go`: Seen-message ID insert/check/prune helpers.
- `storage/seen_ids_test.go`: Seen ID insert/existence/prune tests.
- `storage/testutil_test.go`: Shared test setup helpers for temp stores and seeded peers.

### `network/`

- `network/protocol.go`: Protocol constants/types (including folder transfer envelopes and file folder metadata fields), JSON helpers, frame read/write helpers (control/data limits), handshake/rekey message helpers.
- `network/protocol_test.go`: Frame round-trip and oversized frame rejection tests.
- `network/connection.go`: Encrypted framed peer connection, connection states, keepalive ping/pong, malformed-frame strike counter (3 consecutive invalid frames), sequence tracking, rekey triggers, and epoch-based key rotation.
- `network/handshake.go`: Handshake options/defaults, key-change decision flow, and session-key derivation helpers.
- `network/client.go`: Outbound dial + handshake + session setup.
- `network/server.go`: Listener accept loop, per-IP inbound connection rate limiting, and inbound handshake/session setup.
- `network/peer_manager.go`: High-level peer lifecycle manager: add/approve/remove/disconnect, message send/receive, add/file request rate limits, reconnect workers, runtime state callbacks, discovery-triggered reconnect, folder-transfer message dispatch, default peer-settings creation on first persisted peer, maintenance cleanup scheduling, and device-name hot-reload identity updates.
- `network/file_transfer.go`: File transfer protocol implementation: per-peer outbound queue, send/cancel/retry APIs, folder manifest request/response flow, chunk send/ack/nack, resume, checksum verification, checkpoint persistence/recovery, completion timestamp persistence, status persistence, progress callbacks (with speed/ETA), and per-peer/global receive-policy resolution.
- `network/integration_test.go`: Handshake/session/keepalive/ping-timeout/key-change decision, replay, and control-frame-limit integration tests.
- `network/server_test.go`: Per-IP inbound connection rate-limit enforcement test.
- `network/connection_test.go`: Epoch transition decrypt-window tests plus malformed-frame disconnect-threshold tests.
- `network/rekey_test.go`: Rekey trigger/rekey-under-load/concurrent-transfer/failure-handling tests.
- `network/peer_manager_test.go`: Add/remove/restart/simultaneous-add/spoof-protection behavior tests, request-rate-limit tests, jitter/backoff and endpoint-health ordering tests, peer-settings row creation for accepted peers, and runtime device-name hot-reload behavior.
- `network/messaging_test.go`: Delivery updates, offline queue drain, replay rejection, sequence rejection, signature tamper rejection, spoofed ack rejection tests.
- `network/file_transfer_test.go`: Accepted/rejected transfer flows, sequential queue behavior, queued-cancel behavior, folder structure preservation + traversal rejection, retry flow, reconnect/crash-resume coverage, checksum mismatch failure tests, max-receive-limit auto-reject, and auto-accept directory override behavior.

### `ui/`

- `ui/main_window.go`: Application controller, service startup/shutdown, foreground/background lifecycle handling, event loops, status updates, dialogs, transfer queue panel, notification dispatch for incoming messages/file requests, runtime peer-state event handling, dynamic file/retention-policy wiring into `PeerManager`, and cross-layer wiring.
- `ui/peers_list.go`: Peer list pane, discovery dialog rendering, discovered-peer add flow, peer selection, custom-name sorting/secondary labels, trusted/verified badges, and runtime state + transfer progress indicators.
- `ui/chat_window.go`: Chat pane rendering, message send flow, multi-file/folder queueing flow, chat search (with files-only filter), transcript composition, persisted+live transfer row rendering/progress/actions (cancel/retry/speed/ETA), completed-received file folder links, custom peer-name header, and peer-settings entry point.
- `ui/settings.go`: Device settings dialog (name, fingerprint copy, port mode/port, download location, max file size, notifications, retention, cleanup), per-peer settings dialog (custom name/trust/verified/mute/transfer overrides + clear history), hot-reload handling for name-only changes, and key reset workflow.
- `ui/file_handler.go`: Picker/progress helper used by UI for multi-path selection and transfer progress state.
- `ui/clickable_label.go`: Clickable label widget used by chat header for peer fingerprint interactions.
- `ui/theme.go`: Theme/palette helpers, rounded UI primitives, and hover/tooltip button behavior.

# GoSend — P2P Local Network Chat

This README reflects the current repository state (module `gosend`) as of February 8, 2026.

GoSend is a local-network peer-to-peer desktop app built in Go. It uses mDNS for discovery, TCP for transport, a signed handshake with ephemeral X25519 key exchange, encrypted framed traffic, SQLite persistence, and a Fyne GUI. It is designed for same-LAN usage with authenticated peers, encrypted transport, and durable local history.

**Runtime story:** Startup resolves data dir, loads/creates config, ensures keys, computes fingerprint, opens SQLite, then starts the GUI. The GUI starts the peer manager (TCP listener + protocol engine) and mDNS discovery (broadcast + scanner). The scanner discovers LAN peers and keeps a live cache. The user explicitly adds a discovered peer (TCP dial + signed handshake + add-request). After mutual acceptance, the peer is persisted and eligible for auto-reconnect. Messages and files are exchanged over encrypted sessions with signatures, replay checks, and persistence. If a peer goes offline, outbound messages/files are queued and resume on reconnect. On shutdown, the UI stops the manager and discovery, waits for loops, then closes the DB.

## What Is Implemented Today

- Local LAN peer discovery via mDNS service `_p2pchat._tcp.local.` with TXT keys: `device_id`, `version`, `key_fingerprint`.
- Signed handshake (`handshake`, `handshake_response`) with Ed25519 identities and ephemeral X25519 keys.
- Session key derivation via HKDF-SHA256 using salt `p2pchat-session-v1` and sorted device IDs.
- Transport-level encryption for all post-handshake frames using AES-256-GCM (`secure_frame`).
- Peer lifecycle flows: add request/response, remove, disconnect, reconnect workers, and discovery-driven reconnect.
- Peers are only persisted after an accepted `peer_add_request`; a raw connection/handshake alone does not auto-add a peer.
- Encrypted + signed text messages with delivery status tracking (`pending`, `sent`, `delivered`, `failed`).
- Offline outbound queue with limits: 500 pending messages per peer, 7-day age cutoff, 50 MB pending content per peer.
- Replay defenses for chat messages: per-connection sequence checks, persistent seen ID table, and timestamp skew checks.
- File transfer with request/accept/reject, chunk ack/nack retries, reconnect resume from chunk, and end checksum verification.
- Cross-platform GUI (Fyne) with peer list, chat pane, discovery dialog, settings dialog, key-change prompt, file-transfer prompts/progress, and chat header color that reflects online status.

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
    x25519_private.pem
  files/
```

Default first-run config values:

- `device_id`: generated UUID.
- `device_name`: hostname (fallback `P2P Chat Device`).
- `port_mode`: `automatic`.
- `listening_port`: `0` (ephemeral OS-selected port).
- Key paths are under `<data-dir>/keys`.
- `key_fingerprint`: computed at startup from Ed25519 public key and persisted.

`config.json` fields:

```json
{
  "device_id": "string",
  "device_name": "string",
  "port_mode": "automatic|fixed",
  "listening_port": 0,
  "ed25519_private_key_path": "string",
  "ed25519_public_key_path": "string",
  "x25519_private_key_path": "string",
  "key_fingerprint": "hex"
}
```

Normalization (for older configs):

- Missing `port_mode` with a positive `listening_port` becomes `fixed`.
- Fixed mode with `listening_port=0` is normalized to `9999` (`DefaultListeningPort`).
- Automatic mode never requires a fixed port.
- Missing key path fields are backfilled to the default keys directory.

Runtime application:

- Config is loaded at startup; fingerprint is recomputed from the Ed25519 public key and persisted if changed.
- Name/port changes from the Settings UI are saved immediately but require restart to apply to the active listener and discovery services.

## Protocol and Security (Current Behavior)

Transport:

- TCP listener binds `0.0.0.0:<port>`; port from config (automatic or fixed).
- Length-prefixed frames: `[4-byte big-endian length][JSON payload]`.
- Max frame payload: `10 MB`; frame read timeout default `30s`.

Connection states and keepalive:

- States: `CONNECTING`, `READY`, `IDLE`, `DISCONNECTING`, `DISCONNECTED`.
- Idle keepalive ping every `60s`; pong timeout `15s`. Auto-respond-to-ping is configurable in handshake options.

Handshake flow:

1. Dialer opens TCP and sends `handshake` (identity + ephemeral X25519 public key + signature).
2. Listener verifies version/signature/key-pin policy and replies `handshake_response`.
3. Both derive the same session key and switch to encrypted `secure_frame` transport.
- Protocol version must equal `1`; version mismatch yields typed `error` with supported versions.

Cryptography:

- **Identity**: Ed25519 long-term signing key; private key PEM `0600`, public PEM `0644`. Fingerprint = first 16 bytes of SHA-256(Ed25519 public key), hex.
- **Session key**: Ephemeral X25519 keypairs per connection; shared secret via ECDH; session key = HKDF-SHA256 over shared secret with salt `p2pchat-session-v1`, info = sorted device IDs (`minID|maxID`), 32-byte output.
- **Encryption**: After handshake, every payload is in `secure_frame` (base64 nonce + ciphertext). AES-256-GCM with random nonce per encryption; plaintext is the JSON protocol message. File chunk payloads (`file_data`) are encrypted with the session key and then wrapped in `secure_frame` again.
- **Trust**: TOFU with pinned Ed25519 key in peer record. On key mismatch, handshake is blocked until user trust/reject; if user trusts, session can proceed. Persistent key pin update when user trusts a new key is not fully modeled yet.
- **X25519**: A persisted X25519 private key file is ensured in config, but the handshake uses only ephemeral X25519 keypairs per session.

Message/control integrity:

- Signed (Ed25519) and verified: handshake messages, `peer_add_request`, `peer_add_response`, `peer_remove`, `message`, `file_request`, `file_response`.
- `ack` and `file_complete` are not signed; `ack` accepted only when sender/route matches stored message ownership.
- Replay/tamper defenses: per-connection sequence numbers, timestamp skew ±5 minutes, persistent `seen_message_ids` dedupe.

## Discovery (mDNS)

LAN discovery uses `github.com/grandcat/zeroconf` (mDNS/DNS-SD).

Advertised:

- Service `_p2pchat._tcp`, domain `local.`; instance name = local `device_name`; port = active TCP listening port.
- TXT: `device_id`, `version=1`, `key_fingerprint` (16-byte-truncated SHA-256 hex).

Scanner:

- Background browse plus manual refresh. Refresh interval `10s`; scan timeout per browse `5s`.
- Peers retained across transient misses; removed after stale timeout. Default TTL `120s`; effective stale = max(30s, 2×TTL, 6×refresh) → with defaults ~240s.

Filtering and events:

- Self filtered by `device_id`. Device name: mDNS instance name, then hostname, then device ID fallback. IPv4/IPv6 deduplicated and sorted. Events: `peer_upserted`, `peer_removed`.

Integration with networking:

- Discovery does not auto-add unknown peers. For known peers, discovered endpoints are fed to `NotifyPeerDiscovered`, which updates stored endpoint and can trigger reconnect. Unknown peers require the explicit user add flow.

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

- Backoff sequence: `0s -> 5s -> 15s -> 60s -> 60s...`.
- On disconnect: peer marked offline and reconnect worker starts (unless suppressed by manual remove/disconnect).
- Discovery bridge (`NotifyPeerDiscovered`) updates stored endpoint and triggers reconnect for known non-blocked peers.

## File Transfer Behavior

Outbound:

- `SendFile(peerID, path)` computes checksum, stores metadata (`pending`), and starts transfer if connected.
- Sends `file_request`; waits for `accepted` or `rejected`.
- On accept, resumes from `resume_from_chunk` if provided.
- Chunks are read from source file and encrypted as `file_data` with per-chunk nonce.
- Receiver responds with `chunk_ack` / `chunk_nack`.
- Each chunk retries up to `MaxChunkRetries` (default 3).
- After all chunks are sent, sender sends `file_complete` and waits for the receiver’s `file_complete` (after checksum and rename). Wait uses `FileCompleteTimeout` (default 5 minutes); chunk acks use `FileResponseTimeout` (default 10s). Default chunk size 256 KiB. If the receiver’s completion arrives after the sender timed out, the sender still applies completion so the UI updates.

Inbound:

- User accept/reject prompt via UI callback.
- Accepted transfers write to `<files>/<file_id>_<basename>.part` with random-access chunk writes.
- On `file_complete`, receiver verifies checksum and renames `.part` to final file.

Completion/failure:

- Success => both sides `transfer_status = complete`.
- Reject => `rejected` on both sides.
- Checksum mismatch/finalize failure => `failed` and sender receives failed completion.
- Resume after reconnect is supported via `resume_from_chunk`.

## SQLite Schema and Persistence

Persistence uses `github.com/mattn/go-sqlite3` with a single DB (`app.db`) under the app data directory. DSN enables foreign keys (`_foreign_keys=on`); busy timeout `5000ms`. Schema migrations are versioned with `PRAGMA user_version`.

Tables store: **peers** — identity, pinned Ed25519 public key, fingerprint, status, timestamps, last known endpoint. **messages** — content, content type, sent/received timestamps, read flag, delivery status, signature. **files** — file IDs, sender/receiver, filename/type/size/path/checksum, transfer status. **seen_message_ids** — replay defense. Status enums: peer `online`/`offline`/`pending`/`blocked`; delivery `pending`/`sent`/`delivered`/`failed`; file `pending`/`accepted`/`rejected`/`complete`/`failed`. Updates in practice: peer add accepted → peer row created/updated to `online`; disconnect → `offline`; message sent → `sent`, after ACK → `delivered`; offline messages stored as `pending` and drained on reconnect; file request/accept/reject/complete update `files.transfer_status`; received message IDs go into `seen_message_ids`. Pending queue prune: messages older than 7 days marked `failed`. Seen-ID table can be pruned by timestamp (API in storage).

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

CREATE TABLE seen_message_ids (
  message_id  TEXT PRIMARY KEY,
  received_at INTEGER NOT NULL
);
```

Indexes:

- `idx_messages_peer_time` on `(to_device_id, delivery_status, timestamp_sent)`
- `idx_seen_message_received_at` on `(received_at)`

## UI Summary

The GUI (Fyne) is a single-window desktop app that wires user intent to the networking, discovery, and storage layers.

Layout:

- Left pane: known peers list from DB (self filtered out), plus discovery dialog entry point.
- Right pane: selected peer chat transcript (messages + file transfer rows), compose box, Send and Attach actions.
- Top bar: app title, discovery refresh, settings. Bottom bar: global runtime feedback (errors, queue warnings, connect state).
- Text messages show a copy button (clipboard icon); file/image messages do not. Chat composer is hidden until a peer is selected.
- Chat header is clickable to show the selected peer fingerprint; its color reflects online/offline status.
- Composer: message box on the left, `Send` above `Attach` on the right. `Enter` sends; `Shift+Enter` newline.
- Hovering key buttons (peer info, settings, refresh, discover, send, attach) shows a tooltip and status hint.

Runtime loops:

- Discovery event loop: consumes mDNS scanner events and updates discovered peers.
- Pending add-request loop: surfaces incoming add requests to the confirm dialog.
- Manager error loop: streams async errors from the network manager to the status label.
- Poll loop (every 2s): refreshes peer list and current chat transcript from SQLite.

Dialogs: discovery (peers + Add, dark panels), incoming peer add confirmation, incoming file accept/reject, key-change trust/deny, settings.

Key flows: Add peer (discovery → select → dial → `peer_add_request`); incoming add (accept/reject). Message (Enter to send); file (picker → `SendFile`, progress). Key change (trust/reject with fingerprints). Incoming file (accept/reject with name/size/type).

Settings:

- Device name, port mode (`automatic`/`fixed`), fixed port, fingerprint display, key reset (regenerates Ed25519 + X25519 key files and updates fingerprint). Name/port are persisted immediately; restart required to apply to active services.

Theme:

- Custom dark-leaning palette (online/offline colors, message bubbles, overlays). Tooltips use overlay management and delayed show.

## Repository Layout

```text
.
├── .github/
│   └── workflows/
│       └── release-build.yml
├── AGENTS.md
├── Makefile
├── README.md
├── STACK.md
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
│   ├── file_transfer.go
│   ├── file_transfer_test.go
│   ├── handshake.go
│   ├── integration_test.go
│   ├── messaging_test.go
│   ├── peer_manager.go
│   ├── peer_manager_test.go
│   ├── protocol.go
│   ├── protocol_test.go
│   └── server.go
├── storage/
│   ├── database.go
│   ├── database_test.go
│   ├── files.go
│   ├── files_test.go
│   ├── messages.go
│   ├── messages_test.go
│   ├── peers.go
│   ├── peers_test.go
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

- `main.go`: Startup entrypoint. Loads/normalizes config, ensures key material, persists fingerprint updates, opens SQLite store, constructs local identity, and starts UI runtime.
- `Makefile`: Convenience commands for build and non-UI test flows, plus two client run targets with isolated data dirs.
- `.github/workflows/release-build.yml`: On release publish, builds release artifacts and uploads them to the created release as assets (Linux binary + macOS ARM `.app.zip`).
- `tools.go`: Build-tagged dependency anchors for `fyne`, `zeroconf`, and `go-sqlite3`.
- `go.mod`: Module `gosend`, Go version `1.25.6`, direct dependencies (Fyne, UUID, zeroconf, sqlite3, x/crypto).
- `go.sum`: Dependency checksum lockfile.
- `AGENTS.md`: Local coding-agent instructions for this repository.
- `README.md`: Project documentation.
- `STACK.md`: System architecture and technology stack narrative.
- `bin/gosend`: Built binary artifact (generated by `make build`).

### `config/`

- `config/config.go`: Data dir resolution, directory creation, config load/save, default config generation, and legacy config normalization (including `port_mode`).
- `config/config_test.go`: Tests first-run creation, stable reload behavior, and legacy port mode normalization.

### `crypto/`

- `crypto/keypair.go`: Ed25519 keypair ensure/load/save, fingerprint generation, and fingerprint formatting.
- `crypto/keypair_test.go`: Verifies Ed25519 and X25519 key persistence stability across repeated ensure calls.
- `crypto/ecdh.go`: X25519 private key ensure/load/save, ephemeral keypair generation, shared secret computation, HKDF session key derivation.
- `crypto/ecdh_session_test.go`: Validates both sides derive identical shared secrets and session keys.
- `crypto/encryption.go`: AES-256-GCM encrypt/decrypt helpers with nonce generation and validation.
- `crypto/encryption_test.go`: Round-trip encryption/decryption tests.
- `crypto/signatures.go`: Ed25519 sign/verify helpers with input validation.
- `crypto/signatures_test.go`: Tests valid signature verification and tamper rejection.

### `discovery/`

- `discovery/mdns.go`: mDNS config defaults, broadcaster setup, and combined discovery service startup/shutdown orchestration.
- `discovery/mdns_test.go`: Tests TXT composition, service start/stop wiring, and stale-time default behavior from TTL.
- `discovery/peer_scanner.go`: Background/manual scanning, in-memory peer cache, stale-peer aging, event emission, and self-filtering.
- `discovery/peer_scanner_test.go`: Tests self filtering, manual refresh updates, polling/removal behavior, and timeout tolerance.

### `models/`

- `models/peer.go`: JSON model struct for peer payload shape.
- `models/message.go`: JSON model struct for message payload shape.
- `models/file.go`: JSON model struct for file metadata payload shape.

Note: runtime logic currently uses `storage` structs directly; `models` package is present but not currently imported by active runtime code.

### `storage/`

- `storage/types.go`: Core storage data types, status/content constants, validators, null helpers, and shared errors.
- `storage/database.go`: SQLite open/create helpers, migrations, schema versioning, and connection lifecycle.
- `storage/database_test.go`: Verifies DB creation, migrations, schema version, and expected table presence.
- `storage/peers.go`: Peer CRUD + endpoint/status update logic.
- `storage/peers_test.go`: Peer CRUD and endpoint/status update tests.
- `storage/messages.go`: Message insert/read/update APIs, pending queue reads, and expired queue pruning.
- `storage/messages_test.go`: Message CRUD, ordering, status updates, pending retrieval, and prune behavior.
- `storage/files.go`: File metadata insert/read and transfer-status updates.
- `storage/files_test.go`: File metadata CRUD/status update tests.
- `storage/seen_ids.go`: Seen-message ID insert/check/prune helpers.
- `storage/seen_ids_test.go`: Seen ID insert/existence/prune tests.
- `storage/testutil_test.go`: Shared test setup helpers for temp stores and seeded peers.

### `network/`

- `network/protocol.go`: Protocol constants/types, JSON helpers, frame read/write helpers, handshake build/sign/verify utilities.
- `network/protocol_test.go`: Frame round-trip and oversized frame rejection tests.
- `network/connection.go`: Encrypted framed peer connection, connection states, keepalive ping/pong, sequence tracking, read/send loops.
- `network/handshake.go`: Handshake options/defaults, key-change decision flow, and session-key derivation helpers.
- `network/client.go`: Outbound dial + handshake + session setup.
- `network/server.go`: Listener accept loop and inbound handshake/session setup.
- `network/peer_manager.go`: High-level peer lifecycle manager: add/approve/remove/disconnect, message send/receive, queue drain/limits, reconnect workers, discovery-triggered reconnect.
- `network/file_transfer.go`: File transfer protocol implementation: request/response, chunk send/ack/nack, resume, checksum verification, status persistence, progress callbacks.
- `network/integration_test.go`: Handshake/session/keepalive/ping-timeout/key-change decision integration tests.
- `network/peer_manager_test.go`: Add/remove/restart/simultaneous-add/spoof-protection/pending-cleanup behavior tests.
- `network/messaging_test.go`: Delivery updates, offline queue drain, replay rejection, sequence rejection, signature tamper rejection, spoofed ack rejection tests.
- `network/file_transfer_test.go`: Accepted/rejected transfer flows, reconnect resume, and checksum mismatch failure tests.

### `ui/`

- `ui/main_window.go`: Application controller, service startup/shutdown, event loops, status updates, dialogs, and cross-layer wiring.
- `ui/peers_list.go`: Peer list pane, discovery dialog rendering, discovered-peer add flow, and peer selection.
- `ui/chat_window.go`: Chat pane rendering, message send flow, file attach flow, transcript composition, transfer row rendering/progress.
- `ui/settings.go`: Settings dialog (device name, fingerprint, port mode/port) and key reset workflow.
- `ui/file_handler.go`: Picker/progress helper used by UI for file selection and transfer progress state.
- `ui/clickable_label.go`: Clickable label widget used by chat header for peer fingerprint interactions.
- `ui/theme.go`: Theme/palette helpers, rounded UI primitives, and hover/tooltip button behavior.


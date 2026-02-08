# GoSend — P2P Local Network Chat

This README reflects the current repository state (module `gosend`) as of February 7, 2026.

GoSend is a local-network peer-to-peer desktop app built in Go. It uses mDNS for discovery, TCP for transport, a signed handshake with ephemeral X25519 key exchange, encrypted framed traffic, SQLite persistence, and a Fyne GUI.

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
- The macOS `.app` bundle is unsigned; users may need to right-click `Open` on first launch.

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

## Protocol and Security (Current Behavior)

Framing:

- Length-prefixed frames: `[4-byte big-endian length][JSON payload]`.
- Max frame payload: `10 MB`.
- Frame read timeout default: `30s`.

Connection-level encryption:

- After handshake, every payload is wrapped in `secure_frame` with:
  - `nonce` (base64)
  - `ciphertext` (base64)
- AES-256-GCM key = negotiated session key.

Handshake:

- `handshake` and `handshake_response` include Ed25519 public key, ephemeral X25519 public key, version, timestamp, signature.
- Protocol version must equal `1`.
- Key-change checks use TOFU-style pinned keys from storage.

Message/control integrity checks:

- `peer_add_request`: signed + timestamp skew checked.
- `peer_add_response`: signed + timestamp skew checked.
- `peer_remove`: signed + timestamp skew checked.
- `message`: signed + timestamp skew checked + sequence checked + seen-ID dedupe.
- `file_request`: signed + timestamp skew checked + sequence checked.
- `file_response`: signed by sender in implementation; receiver verifies signature when present.
- `ack`: not signed; accepted only when sender/route matches stored message ownership.

Timestamp skew tolerance:

- ±5 minutes (`maxTimestampSkew`).

Keep-alive defaults:

- Ping interval: `60s` idle.
- Pong timeout: `15s`.

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

Inbound:

- User accept/reject prompt via UI callback.
- Accepted transfers write to `<files>/<file_id>_<basename>.part` with random-access chunk writes.
- On `file_complete`, receiver verifies checksum and renames `.part` to final file.

Completion/failure:

- Success => both sides `transfer_status = complete`.
- Reject => `rejected` on both sides.
- Checksum mismatch/finalize failure => `failed` and sender receives failed completion.
- Resume after reconnect is supported via `resume_from_chunk`.

## SQLite Schema

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

Main window:

- Left pane: known peers list from DB (self filtered out).
- Right pane: selected peer chat transcript + file transfer rows.
- Text messages show a copy button (clipboard icon) to copy the message text; file/image messages do not show the copy button.
- Chat composer is hidden until a peer is selected.
- Chat header text is clickable to show the selected peer fingerprint, and its color reflects online/offline status.
- Chat composer layout: message box on the left, with `Send` above `Attach` in a vertical action group on the right.
- Chat input keyboard behavior: `Enter` sends the message, `Shift+Enter` inserts a newline.
- Toolbar: Settings + Refresh Discovery.
- Footer: status label for errors/events.
- Hovering key action buttons (peer info/settings/refresh/discover/send/attach) shows a tooltip and a short status hint.

Dialogs/prompts:

- Discovery dialog with available peers and Add actions, styled to match the app dark panels.
- Incoming peer add confirmation.
- Incoming file transfer accept/reject confirmation.
- Key-change trust/deny dialog.
- Settings dialog for device name, fingerprint display, port mode, fixed port, key reset.

## Repository Layout

```text
.
├── .github/
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
    ├── file_handler.go
    ├── main_window.go
    ├── peers_list.go
    └── settings.go
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

## Known Gaps / Current Limitations

- `seen_message_ids` pruning exists (`storage.PruneOldEntries`) but is not scheduled by runtime code.
- Trusting a key change allows the current connection, but pinned peer key replacement is not persisted in storage yet.
- `PeerManager.Stop()` closes connections directly; it does not broadcast `peer_disconnect` on app shutdown.
- `Makefile` declares `.PHONY run_tests`, but no `run_tests` target is currently defined.
- No dedicated UI test suite is present.

# Discrepancies

- Previous README listed `DESIGN.md` and `PHASES.md` as repository files. Current repo does not contain those files.
- Previous README documented `make run_tests`. Current `Makefile` has no `run_tests` target.
- Previous README implied default listening behavior was port `9999`. Current first-run config defaults to `port_mode=automatic` and `listening_port=0`.
- Previous README described simultaneous add conflict resolution using lexicographic `device_id` winner logic. Current code auto-accepts when an outbound add is already pending, which allows both sides to accept.
- Previous README stated file checksum failure triggers full retransmission. Current code marks transfer as `failed` and reports failure; no automatic whole-file retransmit is implemented.
- Previous README treated seen-ID pruning as part of active runtime behavior. Current pruning helper exists but is only called in tests unless manually invoked.
- Previous README described models as active network/UI models. Current runtime paths use `storage` models; `models/` structs are presently unused.
- Previous README used a broad `Go 1.21+` statement. Current `go.mod` explicitly sets `go 1.25.6`.
- Previous README implied post key-change trust is durably re-pinned. Current code accepts trusted key changes for handshake but does not persist replacement key material.
- Previous README under-described file encryption layering. Current code encrypts each `file_data` chunk and also encrypts all post-handshake frames at the connection layer.

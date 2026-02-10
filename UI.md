# GoSend UI Reference

This document describes every page, element, and interaction in the GoSend user interface. It intentionally avoids describing where elements are placed and focuses only on what exists and what it does.

## Main Window

The main window is titled **GoSend** and contains the following areas: a toolbar, a peers panel, a chat panel, and a status bar.

### Toolbar

- **GoSend** title label.
- **Transfer Queue** button — opens the Transfer Queue dialog.
- **Refresh Discovery** button — triggers a manual mDNS network scan for peers.
- **Discover** button — opens the Discover Peers dialog.
- **Settings** button — opens the Device Settings dialog.

### Peers Panel

- **"Peers"** header label.
- **Peers list** — shows all known peers (excluding self), sorted alphabetically by display name. Selecting a peer loads their chat transcript.

Each peer row displays:

- The peer's display name (custom name if set, otherwise device name, otherwise device ID).
- A **[Trusted]** badge if the peer's trust level is set to Trusted.
- A **[Verified]** badge if the peer has been marked as verified out-of-band.
- A secondary line showing connection state: `Online`, `Offline`, `Connecting...`, `Disconnecting...`, or `Reconnecting in Ns`.
- If a custom name is set, the original device name (or device ID) is shown as secondary context alongside the connection state.

### Chat Panel

When no peer is selected, a placeholder message reads **"Select a peer to start chatting"**. When a peer is selected, the following elements become visible:

#### Chat Header

- **Peer name and fingerprint** label — shows the selected peer's display name and their key fingerprint.
- **Search** button — toggles the search bar on and off.
- **Peer Settings** button — opens the Peer Settings dialog for the selected peer.

#### Search Bar

Toggled by the Search button. Contains:

- A **text input** with placeholder "Search messages and files" — filters the transcript as you type.
- A **"Files only"** checkbox — when checked, hides text messages and shows only file transfer rows.
- A **Clear** button — resets the search text and unchecks the files-only filter.

#### Chat Transcript

A scrollable list that shows all messages and file transfers with the selected peer, sorted chronologically. The list auto-scrolls to the most recent entry.

**Message rows** display:

- A sender label: **"You"** for outbound or **"Peer"** for inbound.
- A timestamp (e.g. `3:04 PM`).
- A delivery status mark for outbound messages: `✓✓` (delivered), `✓` (sent), `✗` (failed), `…` (pending).
- The message content.
- Double-clicking a message copies its content to the clipboard.

**File transfer rows** display:

- A direction label: **[Send File]** or **[Receive File]**.
- The filename (or file ID if no name is available).
- A metadata line with timestamp, file size, transfer status, and progress percentage.
- The relative path (for folder transfers).
- The stored file path (for completed transfers).
- Double-clicking a file row opens the containing folder in the system file manager.

Transfer status text for file rows: `Waiting`, `Sending`, `Receiving`, `Sending (N%)`, `Receiving (N%)`, `Complete`, or `Failed`.

#### Transfer Action Buttons

A row of contextual buttons that activate when a file transfer row is selected in the transcript:

- **Cancel** — cancels a pending or active transfer (shows a confirmation prompt).
- **Retry** — re-queues a failed or rejected outbound transfer.
- **Show Path** — opens the containing folder of a completed file in the system file manager.
- **Copy Path** — copies the file's stored path to the clipboard.

All four buttons are disabled when no file transfer row is selected. Each button enables or disables based on the selected transfer's state and direction.

#### Message Composer

Visible only when a peer is selected. Contains:

- A **multiline text input** with placeholder "Type a message..." (plain text only).
- A **Send** button — sends the typed message. The keyboard shortcut **Ctrl+Enter** also sends.
- An **Attach Files** button — opens a multi-file picker dialog to queue files for transfer.
- An **Attach Folder** button — opens a folder picker dialog to queue an entire folder for transfer.

### Status Bar

- A **status label** showing the most recent runtime status message (e.g. "Ready (listening on 12345)", "Settings saved", error messages).
- A **Logs** button — opens the Application Logs dialog.

### System Tray Icon

If the system tray is available, a tray icon appears with the tooltip **"GoSend"**. It delivers desktop notification popups for incoming messages and file requests.

---

## Dialogs

### Discover Peers Dialog

Opened from the toolbar's Discover button.

- A subtitle: **"Peers discovered on your local network"**.
- A **discovered peers list** — each row shows the peer's name (or device ID), an **[added]** tag if already a known peer, their network address and port, and online/offline status.
- **Refresh** button — triggers a new mDNS scan and updates the list.
- **Add Selected** button — initiates a connection and peer-add handshake with the selected discovered peer.
- **Close** button.

The list updates automatically as peers appear and disappear on the network.

### Device Settings Dialog

Opened from the toolbar's Settings button.

Contains a form with the following fields:

- **Device Name** — editable text input for the local device's display name.
- **Device ID** — read-only label showing the local device's UUID.
- **Fingerprint** — read-only label showing the local Ed25519 key fingerprint, with a **Copy** button.
- **Port Mode** — dropdown: `Automatic` or `Fixed`.
- **Port** — text input for the listening port number (only enabled when Port Mode is Fixed).
- **Download Location** — text input showing the download directory path, with a **Browse** button that opens a folder picker.
- **Max File Size** — dropdown with presets (`Unlimited`, `100 MB`, `500 MB`, `1 GB`, `5 GB`, `Custom`) plus a custom byte-value input. Help text explains that 0 means unlimited.
- **Notifications** — checkbox: "Enable desktop notifications".
- **Message Retention** — dropdown: `Keep forever`, `30 days`, `90 days`, `1 year`.
- **File Cleanup** — checkbox: "Delete downloaded files when purging metadata".
- **Reset Keys** button — prompts for confirmation, then regenerates identity keys (requires app restart).
- **Save** / **Cancel** buttons.

Changing the device name takes effect immediately without restart. Changing the port requires a restart (the dialog informs the user of this). All other changes take effect immediately on save.

### Peer Settings Dialog

Opened from the chat panel's Peer Settings button.

Contains a form with the following fields:

- **Device Name** — read-only label showing the peer's reported device name.
- **Device ID** — read-only label showing the peer's UUID.
- **Peer Fingerprint** — read-only label showing the peer's Ed25519 key fingerprint, with a **Copy** button.
- **Local Fingerprint** — read-only label showing the local device's fingerprint, with a **Copy** button. (Useful for out-of-band verification with the peer.)
- **Custom Name** — editable text input to assign a local display name for this peer.
- **Trust Level** — dropdown: `Normal` or `Trusted`. Help text: "Trust level is currently informational metadata shown as a badge."
- **Verified** — checkbox: "Verified out-of-band". Help text: "Verified out-of-band means you manually compared fingerprints over a separate trusted channel."
- **Notifications** — checkbox: "Mute notifications for this peer".
- **Auto-Accept Files** — checkbox: "Auto-accept incoming files".
- **Max File Size** — dropdown with presets (`Use global default`, `100 MB`, `500 MB`, `1 GB`, `5 GB`, `Custom`) plus a custom byte-value input. Help text explains that 0 means use the global default.
- **Download Directory** — checkbox: "Use global download directory". When unchecked, a text input and **Browse** button appear for choosing a peer-specific download location.
- **Clear Chat History** button — prompts for confirmation, then deletes all messages and transfer history with this peer.
- **Save** / **Cancel** buttons.

### Transfer Queue Dialog

Opened from the toolbar's Transfer Queue button.

- A **transfer list** showing all active and recently completed transfers across all peers. Active/pending transfers appear before completed ones. Each row displays: peer name, filename (or file ID), transfer status, progress percentage, speed (bytes/s), and estimated time remaining (ETA).
- **Cancel Selected** button — cancels the selected active transfer.
- **Retry Selected** button — re-queues the selected failed transfer.
- **Clear Completed** button — removes all completed transfers from the list.
- **Close** button.

### Application Logs Dialog

Opened from the status bar's Logs button.

- A **read-only text area** displaying two sections:
  - **Runtime** — timestamped log entries from the application runtime (status changes, errors, connection events). Up to 500 most recent entries are retained.
  - **Security Events** — timestamped security audit entries showing event type, severity (INFO/WARNING/CRITICAL), associated peer, and details. Up to 300 most recent events are shown.
- A **Close** button.

### Confirmation and Prompt Dialogs

These appear as modal message boxes in response to specific events:

- **Incoming Peer Request** — "X wants to add you as a peer. Accept?" with Yes/No.
- **Incoming File Request** — shows the sender's name, filename, file size, and file type. "Accept this file?" with Yes/No. (Skipped if auto-accept is enabled for that peer.)
- **Key Change Warning** — "The identity key for X has changed." Shows old and new fingerprints. "Trust the new key?" with Yes/No.
- **Cancel Transfer** — "Cancel this transfer?" with Yes/No.
- **Clear Chat History** — "Delete all messages and transfer history with this peer?" with Yes/No.
- **Reset Keys** — "Reset identity keys? Existing peers will see a key-change warning. This requires restart." with Yes/No.
- **Restart Required** — informational notice shown after saving port changes.
- **Restart Recommended** — informational notice shown if a device-name runtime update fails.
- **Validation Warnings** — shown when settings form inputs are invalid (e.g. empty device name, invalid port).

---

## Desktop Notifications

When the system tray is available, GoSend sends desktop notification popups for:

- **Incoming messages** — title includes the sender's name; body shows the message content (truncated to 200 characters).
- **Incoming file requests** — title includes the sender's name; body shows the filename and size.

Notifications are suppressed when:

- Notifications are globally disabled in device settings.
- The sending peer has notifications muted in their peer settings.
- The sending peer's chat is currently active (already being viewed).
- A file request is auto-accepted (no file-request notification fires).

---

## Theming

The entire UI uses the **Catppuccin Mocha** dark colour theme. All colours are defined centrally in `ui/qt_theme.go` as named constants with semantic aliases, making it straightforward to swap to a different colour scheme by editing only that file.

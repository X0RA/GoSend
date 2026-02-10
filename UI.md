# GoSend UI Reference

This document describes every page, element, and interaction in the GoSend user interface. It intentionally avoids describing where elements are placed and focuses only on what exists and what it does.

## Main Window

The main window is titled **GoSend** and contains the following areas: a toolbar, a peers panel, a chat panel, and a status bar.

### Toolbar

The toolbar has a Mantle background with a 40px fixed height.

- **GoSend** title label in blue (accent colour), bold.
- A thin **vertical separator** line between the title and action buttons.
- **⇄ Transfer Queue** button — opens the Transfer Queue dialog.
- **↻ Refresh Discovery** button — triggers a manual mDNS network scan for peers.
- **⌕ Discover** button — opens the Discover Peers dialog.
- **⚙ Settings** button (right-aligned) — opens the Device Settings dialog.

Toolbar buttons are transparent with no border, showing text in a muted secondary colour. On hover they gain a subtle Surface0 background.

### Peers Panel

The peers panel has a Mantle background matching the toolbar.

- **"PEERS"** header label (uppercase, Overlay2 colour, bold) with a compact **online count** badge (mono text on Surface0 background, right-aligned).
- **Peers list** — shows all known peers (excluding self), sorted alphabetically by display name. Selecting a peer loads their chat transcript.

Peer selection uses a **2px left border** indicator: blue when selected, transparent otherwise. Selected rows have a Surface0 background. Hover shows the same Surface0 background.

Each peer row is a custom styled widget displaying:

- A small **coloured status dot**: green for Online, gray for Offline, blue for Connecting, yellow for Reconnecting.
- The peer's **display name** (custom name if set, otherwise device name, otherwise device ID).
- Semi-transparent **badges**: "Trusted" in teal on a teal-tinted background (rgba), "Verified" in green on a green-tinted background (rgba).
- A coloured **status text line**: green for Online, yellow for Reconnecting, Overlay0 grey for Offline.
- If a custom name is set, the original device name (or device ID) is shown in parentheses next to the status text.

### Chat Panel

The chat panel has a Base background for the transcript area. When no peer is selected, a placeholder message reads **"Select a peer to start chatting"**. When a peer is selected, the following elements become visible:

#### Chat Header

The chat header has a Mantle background with a bottom border, fixed at 44px height.

- **Peer name** label — shows the selected peer's display name.
- **Fingerprint** label (Overlay1 colour, 11px mono) — shows the selected peer's key fingerprint.
- **⌕** icon button (Overlay2 colour) — toggles the search bar on and off.
- **⚙** icon button (Overlay2 colour) — opens the Peer Settings dialog for the selected peer.

#### Search Bar

Toggled by the Search button. Contains:

- A **text input** with placeholder "Search messages and files" — filters the transcript as you type.
- A **"Files only"** checkbox — when checked, hides text messages and shows only file transfer rows.
- A **Clear** button — resets the search text and unchecks the files-only filter.

#### Chat Transcript

A scrollable list that shows all messages and file transfers with the selected peer, sorted chronologically. The list auto-scrolls to the most recent entry.

**Message rows** display with no card background (transparent), matching the mockup's minimal message style:

- A **coloured sender label** (12px): blue "You" for outbound, or the peer's display name in mauve for inbound.
- A **timestamp** (e.g. `3:04 PM`) in Overlay1 colour, 11px.
- A **delivery status mark** for outbound messages: `✓✓` (delivered, **blue**), `✓` (sent, Overlay2), `✗` (failed, red), `…` (pending, Overlay1).
- The **message content** in Subtext1 colour with word-wrapping.
- Double-clicking a message copies its content to the clipboard.

**File transfer cards** display on a Surface0 background with 4px border-radius:

- A **file icon** (📄) coloured by direction, and a **direction label**: peach `[Send File]` or teal `[Receive File]`.
- The **filename** (or file ID if no name is available).
- An indented metadata line with **timestamp**, **file size** (both Overlay1), and a colour-coded **status label** (green=Complete, yellow=Sending/Receiving, red=Failed).
- An inline **progress bar** for active transfers (4px height, blue accent on Surface2 track).
- The **stored file path** in Overlay0 mono text (for completed transfers).
- **Inline action buttons** on each file card (matching the mockup's per-row controls):
  - **✕ Cancel** — cancels a pending or active transfer (shows a confirmation prompt).
  - **↻ Retry** — re-queues a failed or rejected outbound transfer.
  - **↗ Show Path** — opens the containing folder of a completed file.
  - **⎘ Copy Path** — copies the file's stored path to the clipboard.
  Action buttons are styled as small flat text (11px) in Subtext0 colour, gaining a Mantle background on hover. They are muted when disabled.
- Double-clicking a file row opens the containing folder in the system file manager.

Transfer status text for file rows: `Waiting`, `Sending`, `Receiving`, `Sending (N%)`, `Receiving (N%)`, `Complete`, or `Failed`.

#### Message Composer

Visible only when a peer is selected. Has a Mantle background with a top border.

- A **📎** icon button (Overlay2 colour) — opens a multi-file picker dialog to queue files for transfer.
- A **📁** icon button (Overlay2 colour) — opens a folder picker dialog to queue an entire folder for transfer.
- A **multiline text input** with placeholder "Type a message..." (Surface0 background, Surface2 border).
- A **➤** send icon button (blue accent colour, transparent background) — sends the typed message. The keyboard shortcut **Ctrl+Enter** also sends.

### Status Bar

The status bar has a Crust background (darkest shade), 28px fixed height, with 11px text.

- A **status label** (Overlay1 colour) showing the most recent runtime status message (e.g. "Ready (listening on 12345)", "Settings saved", error messages).
- A **📜 Logs** button (Overlay1 colour, transparent background) — opens the Application Logs dialog.

### System Tray Icon

If the system tray is available, a tray icon appears with the tooltip **"GoSend"**. It delivers desktop notification popups for incoming messages and file requests.

---

## Dialogs

### Discover Peers Dialog

Opened from the toolbar's Discover button. Uses the standard dialog chrome: header bar (Mantle, title + subtitle) and footer bar (Mantle, action buttons).

- **Header**: Title "Discover Peers" with subtitle "Peers discovered on your local network" (Overlay1).
- A **discovered peers list** — each row shows the peer's name (or device ID), an **[added]** tag if already a known peer, their network address and port, and online/offline status.
- **Footer buttons**:
  - **↻ Refresh** (secondary) — triggers a new mDNS scan and updates the list.
  - **+ Add Selected** (primary, blue) — initiates a connection and peer-add handshake with the selected discovered peer.
  - **Close** (secondary).

The list updates automatically as peers appear and disappear on the network.

### Device Settings Dialog

Opened from the toolbar's Settings button. Uses the standard dialog chrome: header bar (Mantle, title "Device Settings") and footer bar (Mantle, Cancel/Save buttons with secondary/primary styling).

Contains a scrollable form with the following fields:

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
- **⚠ Reset Identity Keys** danger button (red-tinted background) — prompts for confirmation, then regenerates identity keys (requires app restart).

Changing the device name takes effect immediately without restart. Changing the port requires a restart (the dialog informs the user of this). All other changes take effect immediately on save.

### Peer Settings Dialog

Opened from the chat panel's Peer Settings button. Uses the standard dialog chrome: header bar (Mantle, title "Peer Settings" with the peer's display name as subtitle) and footer bar (Mantle, Cancel/Save buttons with secondary/primary styling).

Contains a scrollable form with the following fields:

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
- **🗑 Clear Chat History** danger button (red-tinted background) — prompts for confirmation, then deletes all messages and transfer history with this peer.

### Transfer Queue Dialog

Opened from the toolbar's Transfer Queue button.

- A **transfer list** showing all active and recently completed transfers across all peers, using styled card widgets. Active/pending transfers appear before completed ones. Each card displays: peer name (bold, coloured), filename (or file ID), colour-coded status label, progress percentage, transfer speed (bytes/s), estimated time remaining (ETA), and an inline progress bar for active transfers.
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

The entire UI uses the **Catppuccin Mocha** dark colour theme. All colours are defined centrally in `ui/qt_theme.go` as named constants with semantic aliases. Widget-specific styling (toolbar, peer list, chat cards, composer, action buttons, icon buttons) is defined via object-name-scoped QSS rules. Styled widgets are built by helper functions in `ui/qt_styled_widgets.go`. Swapping to a different colour scheme requires editing only the colour constants in `qt_theme.go`.

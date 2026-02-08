# GoSend Issue Analysis (From User Reports)

This document captures the reported issues exactly as described, plus code-level observations and likely root-cause hypotheses.
No fixes are proposed here.

## 1. Rejected File Stays at 0% on Sender

User description:
"When a file is declined, it should show that on the sender's side - it currently sits there at 0 %"

In my words:
The sender-side chat card is not reliably transitioning from pending/progress UI into a rejected/failed state when the receiver declines.

What I found:
- Rejection is persisted in network flow (`network/file_transfer.go:738`, `network/file_transfer.go:833`).
- The reject path does not emit a sender progress/state event equivalent to completion/failure events (`network/file_transfer.go:734`, `network/file_transfer.go:835`).
- UI merge prefers live in-memory status over DB status (`ui/chat_window.go:1114`), so stale `pending` can override persisted `rejected`.
- `upsertFileTransfer` preserves old fields when incoming update has zero/empty values (`ui/chat_window.go:942`, `ui/chat_window.go:952`, `ui/chat_window.go:955`).

Likely issue:
Live transfer state on sender can become stale and override persisted `rejected`, leaving a 0% waiting-looking row.

## 2. Unlimited Max File Size Semantics Are Unclear

User description:
"How is 'unlimited' handled for max file size? Does 0 mean that the max filesize is unlimited? If so, the UI should be updated so that it's clear to the user"

In my words:
The sentinel value behavior is not obvious in the UI, especially because `0` has context-dependent meaning.

What I found:
- Global max receive size: `0` means unlimited (`ui/main_window.go:1467`, `network/peer_manager.go:2064`).
- Enforced limit checks only when `effectiveSizeLimit > 0` (`network/file_transfer.go:1021`).
- Peer-level setting uses `0` as "Use global default" (not directly "unlimited") (`ui/settings.go:536`, `ui/settings.go:552`).
- Global settings uses preset label "Unlimited", but the custom field can be set to `0` which is not self-explanatory (`ui/settings.go:24`, `ui/settings.go:113`, `ui/settings.go:540`).

Likely issue:
The same numeric sentinel (`0`) is used for two meanings depending on scope (global unlimited vs peer inherits global), and the UI does not explain this clearly.

## 3. Missing Logs Button in Bottom Bar

User description:
"A button to view logs of the application - and what it has been doing. Put the button on the bottom bar on the far right."

In my words:
There is no UI affordance for viewing operational/security history from within the app.

What I found:
- Bottom bar currently contains only status text (`ui/main_window.go:421`, `ui/main_window.go:426`).
- Structured security events are stored/queryable in SQLite (`storage/security_events.go:20`, `storage/security_events.go:77`).
- Runtime errors are pushed to status line via manager error loop, not persisted as user-readable timeline (`ui/main_window.go:339`, `ui/main_window.go:355`).

Likely issue:
Logging data exists partially (security events), but no UI integration exists for user-facing log browsing.

## 4. Device Name With Spaces Appears Escaped (`my\ name`)

User description:
"If a client sets their name to something with a space, the other device will see the name with a \ ie 'my name' becomes 'my\ name'"

In my words:
mDNS-discovered names appear escaped and are being treated as literal display names.

What I found:
- Discovery parser uses `entry.Instance` directly as `DeviceName` (`discovery/peer_scanner.go:327`, `discovery/peer_scanner.go:357`).
- Discovered device name is written into peer storage when it changes (`network/peer_manager.go:2492`, `network/peer_manager.go:2493`).
- No unescape/normalization is applied to discovery instance names before persistence/display.

Likely issue:
Escaped mDNS instance strings are being persisted/displayed verbatim rather than normalized for UI.

## 5. Extra "Add Another File" Dialog Is Unwanted

User description:
"When I select a file to transfer, i don't like the screen that comes up that says 'add another file to this transfer queue'... remove that box."

In my words:
The file attach flow currently forces a repeated confirm loop for multi-select behavior instead of a cleaner one-shot picker flow.

What I found:
- Attach uses `FileHandler.PickPaths()` (`ui/chat_window.go:268`).
- Picker is wired to `pickFilePath`, which opens one file picker then loops with confirm dialog (`ui/main_window.go:182`, `ui/main_window.go:1196`, `ui/main_window.go:1260`).
- The dialog text exactly matches reported UX (`ui/main_window.go:1268`).

Likely issue:
A legacy iterative picker flow is still active and conflicts with expected modern multi-select/attach behavior.

## 6. Incoming File Reject Dialog Reappears After Delay

User description:
"When client a sends a file and client b has the incoming file screen - if client b doesn't click any buttons for awhile then clicks reject the incoming file screen just re-appears (I had to click it twice for it to go away)."

In my words:
If the receiver waits too long before deciding, sender retries request and receiver sees a duplicate prompt.

What I found:
- Sender waits for accept/reject with timeout (`network/file_transfer.go:728`, `network/file_transfer.go:1710`).
- Default file response timeout is 10 seconds (`network/peer_manager.go:44`).
- On timeout/error while still pending, transfer is requeued at front for retry (`network/file_transfer.go:399`, `network/file_transfer.go:407`, `network/file_transfer.go:408`).
- Each incoming request invokes the UI confirmation callback (`network/file_transfer.go:1027`, `ui/main_window.go:1102`).

Likely issue:
Request retries due timeout can produce multiple prompts for the same file if user decision is slower than sender timeout window.

## 7. Rejected -> Cancel -> Retry Produces Incorrect Sender UI State

User description:
"If a file was sent by client a then rejected by client b, client a still shows 0%. If client a then presses cancel on the file (that was declined) it shows the retry and show path buttons. If retry is clicked on client a, and then accepted on client b, client a immediately shows that the file was sent 'complete' but client b correctly displays the progress."

In my words:
Sender-side transfer state becomes sticky/incorrect after terminal transitions, and retry does not fully reset UI transfer flags.

What I found:
- Cancel path sets `TransferCompleted: true` in UI state (`ui/chat_window.go:387`, `ui/chat_window.go:391`).
- "Show Path" is shown for any outgoing transfer with `TransferCompleted` and path, regardless success/failure (`ui/chat_window.go:807`).
- Retry UI injects `TransferCompleted: false`, but `upsertFileTransfer` preserves old completed flag if new entry is false (`ui/chat_window.go:420`, `ui/chat_window.go:427`, `ui/chat_window.go:954`, `ui/chat_window.go:955`).
- Merge function also keeps live completed state sticky (`ui/chat_window.go:1117`, `ui/chat_window.go:1118`).
- Status formatter returns "Complete" whenever `TransferCompleted` is true (`ui/chat_window.go:853`, `ui/chat_window.go:854`).

Likely issue:
Terminal transfer flags and status precedence are sticky across retries, so sender UI can show completed state prematurely/inaccurately.

## 8. "Trust Level" and "Verified Out-of-Band" Meanings Are Not Explained

User description:
"Trust level - what does it actually mean?... Also the same with 'verified out-of-band' what does that mean??"

In my words:
The settings expose security/trust labels without user-facing definitions, and current behavior may not match user expectations.

What I found:
- Trust level options exist in peer settings (`ui/settings.go:313`) and badges are shown in peer list (`ui/peers_list.go:68`).
- Code search shows trust level is persisted but not used for transfer/network policy branching in runtime logic (only storage + UI badge paths; no behavioral checks in network flow).
- Verified toggle exists and shows badge (`ui/settings.go:323`, `ui/peers_list.go:71`).
- Verified flag is reset on key change (`network/peer_manager.go:1891`).

Likely issue:
Terms are security-significant but currently function mostly as metadata/badges, so users lack both explanation and clear behavioral impact.

## 9. Transfer Queue Theming/Readability Is Poor

User description:
"The UI for Transfer Queue isn't themed correctly and the text is very hard to readz"

In my words:
The transfer queue mixes dark custom backgrounds with theme-dependent text styles, creating low contrast in some theme variants.

What I found:
- Queue rows use hardcoded dark panel backgrounds (`ui/main_window.go:652`, `ui/theme.go:21`).
- Row text is standard `widget.Label`; metadata is marked low importance (`ui/main_window.go:633`, `ui/main_window.go:635`, `ui/main_window.go:636`).
- App theme wrapper mainly customizes overlay/shadow, not all content colors (`ui/theme.go:157`, `ui/theme.go:159`).

Likely issue:
Foreground text color/importance is not harmonized with hardcoded dark row backgrounds, causing contrast problems.

## 10. Received File Rows Need Path Visibility/Copy Action

User description:
"In the messages, for recieved files (not sent) after it's recieved there should be a copy path button and maybe under the file it can show the path that it's saved to"

In my words:
Inbound completed files can open containing folder, but users cannot easily see/copy the exact saved file path from chat.

What I found:
- Incoming completed rows render filename as hyperlink to open containing folder (`ui/chat_window.go:729`, `ui/chat_window.go:731`).
- "Show Path" button currently appears only for outgoing completed files (`ui/chat_window.go:807`).
- No explicit path text/copy control is rendered for incoming completed file cards.

Likely issue:
UI supports folder-open action but lacks direct path visibility/copy affordance for received items.

## 11. Peer List Progress Bar Should Be Removed

User description:
"When a file transfer is in progress - can we remove the progress bar under the peers list - it looks ugly. Keep the transferring text but remove the big progress bar that's in the peers list."

In my words:
Current peer list shows both textual transfer state and a progress bar; user wants text-only state.

What I found:
- Peer rows instantiate and render an inline `widget.ProgressBar` (`ui/peers_list.go:39`, `ui/peers_list.go:41`).
- During transfer, state text already shows `"Transferring... %.0f%%"` (`ui/peers_list.go:80`).
- Progress bar visibility is toggled separately (`ui/peers_list.go:97`, `ui/peers_list.go:101`).

Likely issue:
Duplicate transfer indicators (text + bar) increase visual clutter in a narrow list row.


Visual design summary and component code:

## Color Palette (from globals.css)

The design uses Catppuccin Mocha (dark theme). Key colors:

- **Base colors:**
  - `--ctp-crust`: #11111b (darkest background)
  - `--ctp-mantle`: #181825 (secondary background)
  - `--ctp-base`: #1e1e2e (main background)
  - `--ctp-surface0`: #313244 (card/panel background)
  - `--ctp-surface1`: #45475a (elevated surface)
  - `--ctp-surface2`: #585b70 (border/divider)

- **Text colors:**
  - `--ctp-text`: #cdd6f4 (primary text)
  - `--ctp-subtext1`: #bac2de (secondary text)
  - `--ctp-subtext0`: #a6adc8 (tertiary text)
  - `--ctp-overlay2`: #9399b2
  - `--ctp-overlay1`: #7f849c
  - `--ctp-overlay0`: #6c7086

- **Accent colors:**
  - `--ctp-blue`: #89b4fa (primary actions)
  - `--ctp-green`: #a6e3a1 (success/online)
  - `--ctp-red`: #f38ba8 (error/destructive)
  - `--ctp-yellow`: #f9e2af (warning/in-progress)
  - `--ctp-teal`: #94e2d5 (trusted badge)
  - `--ctp-peach`: #fab387 (outbound files)
  - `--ctp-mauve`: #cba6f7 (inbound messages)

---

## Component Design Details

### 1. Toolbar (`toolbar.tsx`)

**Layout:**
- Height: 40px (`h-10`)
- Horizontal flex container with items centered
- Padding: 12px horizontal (`px-3`), gap: 8px (`gap-2`)
- Background: `--ctp-mantle` (#181825)

**Elements (left to right):**
1. "GoSend" title:
   - Font: 14px, semibold, tight tracking
   - Color: `--ctp-blue` (#89b4fa)
   - Right margin: 16px (`mr-4`)

2. Vertical divider:
   - Width: 1px, height: 20px (`h-5`)
   - Color: `--ctp-surface2` (#585b70)

3. Buttons (Transfer Queue, Refresh Discovery, Discover):
   - Padding: 10px horizontal, 4px vertical (`px-2.5 py-1`)
   - Font: 12px (`text-xs`)
   - Icon size: 14px
   - Gap between icon and label: 6px (`gap-1.5`)
   - Default color: `--ctp-subtext1` (#bac2de)
   - Hover background: `--ctp-surface0` (#313244)
   - Border radius: 4px (`rounded`)

4. Spacer (`flex-1`)

5. Settings button (same styling as other buttons)

**Complete Code:**
```tsx
"use client"

import React from "react"

import { RefreshCw, Search, Settings, ArrowDownUp } from "lucide-react"

interface ToolbarProps {
  onOpenTransferQueue: () => void
  onOpenDiscover: () => void
  onOpenSettings: () => void
}

function ToolbarButton({
  icon: Icon,
  label,
  onClick,
}: {
  icon: React.ComponentType<{ size?: number }>
  label: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs transition-colors"
      style={{ color: "var(--ctp-subtext1)", backgroundColor: "transparent" }}
      onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
      onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
    >
      <Icon size={14} />
      {label}
    </button>
  )
}

export function Toolbar({ onOpenTransferQueue, onOpenDiscover, onOpenSettings }: ToolbarProps) {
  return (
    <div className="flex items-center h-10 px-3 gap-2" style={{ backgroundColor: "var(--ctp-mantle)" }}>
      <span className="text-sm font-semibold tracking-tight mr-4" style={{ color: "var(--ctp-blue)" }}>
        GoSend
      </span>

      <div className="h-5 w-px" style={{ backgroundColor: "var(--ctp-surface2)" }} />

      <ToolbarButton icon={ArrowDownUp} label="Transfer Queue" onClick={onOpenTransferQueue} />
      <ToolbarButton icon={RefreshCw} label="Refresh Discovery" />
      <ToolbarButton icon={Search} label="Discover" onClick={onOpenDiscover} />

      <div className="flex-1" />

      <ToolbarButton icon={Settings} label="Settings" onClick={onOpenSettings} />
    </div>
  )
}
```

---

### 2. Peers Panel (`peers-panel.tsx`)

**Layout:**
- Width: 220px fixed
- Background: `--ctp-mantle` (#181825)
- Border right: 1px solid `--ctp-surface0` (#313244)
- Flex column layout, full height

**Header:**
- Padding: 12px horizontal, 10px vertical (`px-3 py-2.5`)
- "PEERS" label:
  - Font: 12px (`text-xs`), semibold, uppercase, wide tracking
  - Color: `--ctp-overlay2` (#9399b2)
- Online count badge (right-aligned):
  - Font: 12px monospace (`text-xs font-mono`)
  - Padding: 6px horizontal, 2px vertical (`px-1.5 py-0.5`)
  - Background: `--ctp-surface0` (#313244)
  - Color: `--ctp-overlay1` (#7f849c)
  - Border radius: 4px (`rounded`)

**Peer List Items:**
- Each peer button:
  - Full width, left-aligned text
  - Padding: 12px horizontal, 8px vertical (`px-3 py-2`)
  - Border left: 2px (transparent when not selected, `--ctp-blue` when selected)
  - Selected background: `--ctp-surface0` (#313244)
  - Hover background (when not selected): `--ctp-surface0` (#313244)

**Peer Item Content:**
1. First row:
   - Online indicator dot: 8px × 8px (`w-2 h-2`), rounded
     - Online: `--ctp-green` (#a6e3a1)
     - Connecting/Reconnecting: `--ctp-yellow` (#f9e2af)
     - Offline: `--ctp-overlay0` (#6c7086)
   - Display name: 14px (`text-sm`), color `--ctp-text` (#cdd6f4)
   - Badges (Trusted/Verified):
     - Font: 10px (`text-[10px]`)
     - Padding: 4px horizontal, 1px vertical (`px-1 py-px`)
     - Trusted: teal text with rgba(148, 226, 213, 0.1) background
     - Verified: green text with rgba(166, 227, 161, 0.1) background

2. Second row (indented 14px `ml-3.5`):
   - Status text: 11px (`text-[11px]`)
     - Online: `--ctp-green`
     - Reconnecting: `--ctp-yellow`
     - Offline: `--ctp-overlay0`
   - Device name (if present): 11px, color `--ctp-overlay1` (#7f849c)

**Complete Code:**
```tsx
"use client"

import { cn } from "@/lib/utils"

export interface Peer {
  id: string
  displayName: string
  deviceName?: string
  status: "Online" | "Offline" | "Connecting..." | "Reconnecting in 5s"
  trusted?: boolean
  verified?: boolean
}

const mockPeers: Peer[] = [
  { id: "a1b2c3", displayName: "Alice's MacBook", status: "Online", trusted: true, verified: true },
  { id: "d4e5f6", displayName: "Bob", deviceName: "bob-desktop", status: "Online", trusted: true },
  { id: "g7h8i9", displayName: "Charlie's Phone", status: "Offline" },
  { id: "j0k1l2", displayName: "Dev Server", status: "Online" },
  { id: "m3n4o5", displayName: "Eve", deviceName: "eve-laptop", status: "Reconnecting in 5s" },
  { id: "p6q7r8", displayName: "Frank's Tablet", status: "Offline" },
]

interface PeersPanelProps {
  selectedPeerId: string | null
  onSelectPeer: (peerId: string) => void
}

export function PeersPanel({ selectedPeerId, onSelectPeer }: PeersPanelProps) {
  return (
    <div className="flex flex-col h-full" style={{ backgroundColor: "var(--ctp-mantle)" }}>
      <div className="px-3 py-2.5 flex items-center">
        <span className="text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
          Peers
        </span>
        <span
          className="ml-auto text-xs font-mono px-1.5 py-0.5 rounded"
          style={{ color: "var(--ctp-overlay1)", backgroundColor: "var(--ctp-surface0)" }}
        >
          {mockPeers.filter((p) => p.status === "Online").length}
        </span>
      </div>

      <div className="flex-1 overflow-y-auto">
        {mockPeers.map((peer) => {
          const isSelected = peer.id === selectedPeerId
          const isOnline = peer.status === "Online"
          return (
            <button
              key={peer.id}
              type="button"
              onClick={() => onSelectPeer(peer.id)}
              className={cn(
                "w-full text-left px-3 py-2 transition-colors border-l-2",
              )}
              style={{
                backgroundColor: isSelected ? "var(--ctp-surface0)" : "transparent",
                borderLeftColor: isSelected ? "var(--ctp-blue)" : "transparent",
              }}
              onMouseEnter={(e) => {
                if (!isSelected) e.currentTarget.style.backgroundColor = "var(--ctp-surface0)"
              }}
              onMouseLeave={(e) => {
                if (!isSelected) e.currentTarget.style.backgroundColor = "transparent"
              }}
            >
              <div className="flex items-center gap-1.5">
                {/* Online indicator dot */}
                <span
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{
                    backgroundColor: isOnline ? "var(--ctp-green)" : peer.status === "Reconnecting in 5s" || peer.status === "Connecting..." ? "var(--ctp-yellow)" : "var(--ctp-overlay0)",
                  }}
                />
                <span className="text-sm truncate" style={{ color: "var(--ctp-text)" }}>
                  {peer.displayName}
                </span>
                {peer.trusted && (
                  <span
                    className="text-[10px] px-1 py-px rounded flex-shrink-0"
                    style={{ color: "var(--ctp-teal)", backgroundColor: "rgba(148, 226, 213, 0.1)" }}
                  >
                    Trusted
                  </span>
                )}
                {peer.verified && (
                  <span
                    className="text-[10px] px-1 py-px rounded flex-shrink-0"
                    style={{ color: "var(--ctp-green)", backgroundColor: "rgba(166, 227, 161, 0.1)" }}
                  >
                    Verified
                  </span>
                )}
              </div>
              <div className="flex items-center gap-1.5 mt-0.5 ml-3.5">
                <span className="text-[11px]" style={{ color: isOnline ? "var(--ctp-green)" : peer.status.startsWith("Reconnecting") ? "var(--ctp-yellow)" : "var(--ctp-overlay0)" }}>
                  {peer.status}
                </span>
                {peer.deviceName && (
                  <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>
                    ({peer.deviceName})
                  </span>
                )}
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
```

---

### 3. Chat Panel (`chat-panel.tsx`)

**Layout:**
- Background: `--ctp-base` (#1e1e2e)
- Flex column, full height

**Chat Header:**
- Height: 44px (`h-11`)
- Padding: 16px horizontal (`px-4`)
- Background: `--ctp-mantle` (#181825)
- Border bottom: 1px solid `--ctp-surface0` (#313244)
- Contains:
  - Peer name: 14px (`text-sm`), medium weight, color `--ctp-text`
  - Peer ID: 11px monospace (`text-[11px] font-mono`), color `--ctp-overlay1`
  - Search button: 15px icon, hover background `--ctp-surface0`, active color `--ctp-blue`
  - Settings button: 15px icon, same hover behavior

**Search Bar (when visible):**
- Padding: 16px horizontal, 8px vertical (`px-4 py-2`)
- Background: `--ctp-surface0` (#313244)
- Border bottom: 1px solid `--ctp-surface1` (#45475a)
- Contains:
  - Search icon: 14px, color `--ctp-overlay1`
  - Input: 14px text, placeholder opacity 50%
  - "Files only" checkbox: 14px × 14px (`w-3.5 h-3.5`), accent color `--ctp-blue`
  - Close button: 14px icon

**Chat Transcript:**
- Padding: 16px horizontal, 12px vertical (`px-4 py-3`)
- Vertical spacing between messages: 6px (`space-y-1.5`)

**Message Items:**
- File messages:
  - Background: `--ctp-surface0` (#313244)
  - Hover background: `--ctp-surface1` (#45475a)
  - Padding: 12px horizontal, 8px vertical (`px-3 py-2`)
  - Border radius: 4px (`rounded`)
- Text messages:
  - Background: transparent
  - Hover background: rgba(49, 50, 68, 0.5)

**File Transfer Row:**
- File icon: 14px
  - Outbound: `--ctp-peach` (#fab387)
  - Inbound: `--ctp-teal` (#94e2d5)
- "[Send File]" / "[Receive File]" label: 11px medium, same colors as icon
- File name: 14px (`text-sm`), color `--ctp-text`
- Metadata row (indented 22px `ml-[22px]`):
  - Timestamp, file size: 11px, color `--ctp-overlay1`
  - Status: 11px medium, color varies by status
- Progress bar:
  - Height: 4px (`h-1`)
  - Background: `--ctp-surface2` (#585b70)
  - Fill: `--ctp-blue` (#89b4fa)
  - Border radius: full (`rounded-full`)
- Stored path: 11px monospace, color `--ctp-overlay0`
- Action buttons:
  - Font: 11px (`text-[11px]`)
  - Padding: 8px horizontal, 4px vertical (`px-2 py-1`)
  - Enabled color: `--ctp-subtext0` (#a6adc8)
  - Disabled color: `--ctp-surface2` (#585b70)
  - Hover background: `--ctp-mantle` (#181825)

**Text Message Row:**
- Sender name: 12px (`text-xs`), medium weight
  - Outbound: `--ctp-blue` (#89b4fa)
  - Inbound: `--ctp-mauve` (#cba6f7)
- Timestamp: 11px, color `--ctp-overlay1`
- Delivery status icons:
  - Delivered: ✓✓ (blue)
  - Sent: ✓ (overlay2)
  - Failed: ✗ (red)
  - Pending: … (overlay1)
- Message content: 14px (`text-sm`), color `--ctp-subtext1` (#bac2de)

**Message Composer:**
- Padding: 12px horizontal, 10px vertical (`px-3 py-2.5`)
- Background: `--ctp-mantle` (#181825)
- Border top: 1px solid `--ctp-surface0` (#313244)
- Contains:
  - Attach Files button: 16px icon, color `--ctp-overlay2`
  - Attach Folder button: 16px icon, color `--ctp-overlay2`
  - Textarea container:
    - Background: `--ctp-surface0` (#313244)
    - Border: 1px solid `--ctp-surface2` (#585b70)
    - Padding: 12px horizontal, 6px vertical (`px-3 py-1.5`)
    - Min height: 32px, max height: 120px
    - Border radius: 4px (`rounded`)
  - Send button: 16px icon, color `--ctp-blue` (#89b4fa)

**Complete Code:**
```tsx
"use client"

import { useState } from "react"
import {
  Search,
  Settings,
  Send,
  Paperclip,
  FolderOpen,
  X,
  RotateCcw,
  FolderSearch,
  Copy,
  File,
  MessageSquare,
} from "lucide-react"

interface ChatMessage {
  id: string
  type: "message" | "file"
  direction: "outbound" | "inbound"
  timestamp: string
  // message fields
  content?: string
  deliveryStatus?: "delivered" | "sent" | "failed" | "pending"
  // file fields
  fileName?: string
  fileSize?: string
  transferStatus?: string
  progress?: number
  storedPath?: string
}

const mockMessages: ChatMessage[] = [
  {
    id: "1",
    type: "message",
    direction: "outbound",
    timestamp: "2:58 PM",
    content: "Hey, are you around? I need to send you those project files.",
    deliveryStatus: "delivered",
  },
  {
    id: "2",
    type: "message",
    direction: "inbound",
    timestamp: "2:59 PM",
    content: "Yeah, I'm online. Go ahead and send them over!",
  },
  {
    id: "3",
    type: "file",
    direction: "outbound",
    timestamp: "3:00 PM",
    fileName: "project-assets-v2.zip",
    fileSize: "48.3 MB",
    transferStatus: "Complete",
    progress: 100,
    storedPath: "/home/alice/Downloads/project-assets-v2.zip",
  },
  {
    id: "4",
    type: "message",
    direction: "inbound",
    timestamp: "3:02 PM",
    content: "Got it, thanks! Extracting now.",
  },
  {
    id: "5",
    type: "file",
    direction: "inbound",
    timestamp: "3:04 PM",
    fileName: "feedback-notes.md",
    fileSize: "12 KB",
    transferStatus: "Complete",
    progress: 100,
    storedPath: "/home/user/Downloads/feedback-notes.md",
  },
  {
    id: "6",
    type: "message",
    direction: "outbound",
    timestamp: "3:05 PM",
    content: "Perfect. I also have the updated designs, sending now.",
    deliveryStatus: "delivered",
  },
  {
    id: "7",
    type: "file",
    direction: "outbound",
    timestamp: "3:06 PM",
    fileName: "ui-mockups-final.fig",
    fileSize: "127.8 MB",
    transferStatus: "Sending (67%)",
    progress: 67,
  },
  {
    id: "8",
    type: "message",
    direction: "outbound",
    timestamp: "3:07 PM",
    content: "This one's a bit larger, might take a minute.",
    deliveryStatus: "sent",
  },
]

function DeliveryIcon({ status }: { status?: string }) {
  if (!status) return null
  const icons: Record<string, { symbol: string; color: string }> = {
    delivered: { symbol: "\u2713\u2713", color: "var(--ctp-blue)" },
    sent: { symbol: "\u2713", color: "var(--ctp-overlay2)" },
    failed: { symbol: "\u2717", color: "var(--ctp-red)" },
    pending: { symbol: "\u2026", color: "var(--ctp-overlay1)" },
  }
  const icon = icons[status]
  if (!icon) return null
  return (
    <span className="text-xs font-mono ml-1" style={{ color: icon.color }}>
      {icon.symbol}
    </span>
  )
}

interface ChatPanelProps {
  selectedPeerName: string | null
  onOpenPeerSettings: () => void
}

export function ChatPanel({ selectedPeerName, onOpenPeerSettings }: ChatPanelProps) {
  const [showSearch, setShowSearch] = useState(false)
  const [searchText, setSearchText] = useState("")
  const [filesOnly, setFilesOnly] = useState(false)

  if (!selectedPeerName) {
    return (
      <div className="flex items-center justify-center h-full" style={{ backgroundColor: "var(--ctp-base)" }}>
        <div className="text-center">
          <MessageSquare size={48} className="mx-auto mb-3" style={{ color: "var(--ctp-surface2)" }} />
          <p className="text-sm" style={{ color: "var(--ctp-overlay1)" }}>
            Select a peer to start chatting
          </p>
        </div>
      </div>
    )
  }

  const filteredMessages = mockMessages.filter((msg) => {
    if (filesOnly && msg.type !== "file") return false
    if (searchText) {
      const text = msg.content || msg.fileName || ""
      if (!text.toLowerCase().includes(searchText.toLowerCase())) return false
    }
    return true
  })

  return (
    <div className="flex flex-col h-full" style={{ backgroundColor: "var(--ctp-base)" }}>
      {/* Chat Header */}
      <div
        className="flex items-center px-4 h-11 gap-3 flex-shrink-0"
        style={{ backgroundColor: "var(--ctp-mantle)", borderBottom: "1px solid var(--ctp-surface0)" }}
      >
        <div className="flex-1 min-w-0">
          <span className="text-sm font-medium" style={{ color: "var(--ctp-text)" }}>
            {selectedPeerName}
          </span>
          <span className="text-[11px] font-mono ml-2" style={{ color: "var(--ctp-overlay1)" }}>
            a1b2...c3d4
          </span>
        </div>
        <button
          type="button"
          onClick={() => setShowSearch(!showSearch)}
          className="p-1.5 rounded transition-colors"
          style={{
            color: showSearch ? "var(--ctp-blue)" : "var(--ctp-overlay2)",
            backgroundColor: showSearch ? "var(--ctp-surface0)" : "transparent",
          }}
          onMouseEnter={(e) => { if (!showSearch) e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          onMouseLeave={(e) => { if (!showSearch) e.currentTarget.style.backgroundColor = "transparent" }}
        >
          <Search size={15} />
        </button>
        <button
          type="button"
          onClick={onOpenPeerSettings}
          className="p-1.5 rounded transition-colors"
          style={{ color: "var(--ctp-overlay2)", backgroundColor: "transparent" }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
        >
          <Settings size={15} />
        </button>
      </div>

      {/* Search Bar */}
      {showSearch && (
        <div
          className="flex items-center px-4 py-2 gap-2 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-surface0)", borderBottom: "1px solid var(--ctp-surface1)" }}
        >
          <Search size={14} style={{ color: "var(--ctp-overlay1)" }} />
          <input
            type="text"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="Search messages and files"
            className="flex-1 bg-transparent text-sm outline-none placeholder:opacity-50"
            style={{ color: "var(--ctp-text)", }}
          />
          <label className="flex items-center gap-1.5 cursor-pointer">
            <input
              type="checkbox"
              checked={filesOnly}
              onChange={(e) => setFilesOnly(e.target.checked)}
              className="w-3.5 h-3.5 rounded accent-[var(--ctp-blue)]"
            />
            <span className="text-xs" style={{ color: "var(--ctp-subtext0)" }}>Files only</span>
          </label>
          <button
            type="button"
            onClick={() => { setSearchText(""); setFilesOnly(false) }}
            className="p-1 rounded transition-colors"
            style={{ color: "var(--ctp-overlay1)" }}
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Chat Transcript */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-1.5">
        {filteredMessages.map((msg) => {
          const isFile = msg.type === "file"
          const isOutbound = msg.direction === "outbound"
          const isComplete = msg.transferStatus === "Complete"
          const isFailed = msg.transferStatus?.startsWith("Failed") ?? false
          const canCancel = isFile && !isComplete && !isFailed
          const canRetry = isFile && isFailed
          const canShowPath = isFile && isComplete && Boolean(msg.storedPath)
          const canCopyPath = isFile && Boolean(msg.storedPath)

          return (
            <div
              key={msg.id}
              className="w-full text-left rounded px-3 py-2 transition-colors"
              style={{
                backgroundColor: isFile ? "var(--ctp-surface0)" : "transparent",
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = isFile ? "var(--ctp-surface1)" : "rgba(49, 50, 68, 0.5)"
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = isFile ? "var(--ctp-surface0)" : "transparent"
              }}
            >
              {isFile ? (
                /* File transfer row */
                <div>
                  <div className="flex items-center gap-2">
                    <File size={14} style={{ color: isOutbound ? "var(--ctp-peach)" : "var(--ctp-teal)" }} />
                    <span className="text-[11px] font-medium" style={{ color: isOutbound ? "var(--ctp-peach)" : "var(--ctp-teal)" }}>
                      {isOutbound ? "[Send File]" : "[Receive File]"}
                    </span>
                    <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                      {msg.fileName}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 mt-1 ml-[22px]">
                    <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>{msg.timestamp}</span>
                    <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>{msg.fileSize}</span>
                    <span
                      className="text-[11px] font-medium"
                      style={{
                        color: msg.transferStatus === "Complete"
                          ? "var(--ctp-green)"
                          : msg.transferStatus?.startsWith("Failed")
                            ? "var(--ctp-red)"
                            : "var(--ctp-yellow)",
                      }}
                    >
                      {msg.transferStatus}
                    </span>
                  </div>
                  {/* Progress bar for active transfers */}
                  {msg.progress !== undefined && msg.progress < 100 && (
                    <div className="mt-1.5 ml-[22px] h-1 rounded-full overflow-hidden" style={{ backgroundColor: "var(--ctp-surface2)" }}>
                      <div
                        className="h-full rounded-full transition-all"
                        style={{ width: `${msg.progress}%`, backgroundColor: "var(--ctp-blue)" }}
                      />
                    </div>
                  )}
                  {msg.storedPath && (
                    <div className="mt-1 ml-[22px]">
                      <span className="text-[11px] font-mono" style={{ color: "var(--ctp-overlay0)" }}>
                        {msg.storedPath}
                      </span>
                    </div>
                  )}
                  <div className="mt-2 ml-[22px] flex items-center gap-1.5 flex-wrap">
                    {[
                      { icon: X, label: "Cancel", enabled: canCancel },
                      { icon: RotateCcw, label: "Retry", enabled: canRetry },
                      { icon: FolderSearch, label: "Show Path", enabled: canShowPath },
                      { icon: Copy, label: "Copy Path", enabled: canCopyPath },
                    ].map(({ icon: Icon, label, enabled }) => (
                      <button
                        key={label}
                        type="button"
                        disabled={!enabled}
                        className="flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors"
                        style={{
                          color: enabled ? "var(--ctp-subtext0)" : "var(--ctp-surface2)",
                          backgroundColor: "transparent",
                          cursor: enabled ? "pointer" : "default",
                        }}
                        onMouseEnter={(e) => { if (enabled) e.currentTarget.style.backgroundColor = "var(--ctp-mantle)" }}
                        onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
                      >
                        <Icon size={12} />
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                /* Message row */
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium" style={{ color: isOutbound ? "var(--ctp-blue)" : "var(--ctp-mauve)" }}>
                      {isOutbound ? "You" : selectedPeerName}
                    </span>
                    <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>{msg.timestamp}</span>
                    {isOutbound && <DeliveryIcon status={msg.deliveryStatus} />}
                  </div>
                  <p className="text-sm mt-0.5" style={{ color: "var(--ctp-subtext1)" }}>
                    {msg.content}
                  </p>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* Message Composer */}
      <div
        className="flex items-end px-3 py-2.5 gap-2 flex-shrink-0"
        style={{ backgroundColor: "var(--ctp-mantle)", borderTop: "1px solid var(--ctp-surface0)" }}
      >
        <button
          type="button"
          className="p-1.5 rounded transition-colors flex-shrink-0"
          style={{ color: "var(--ctp-overlay2)" }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          title="Attach Files"
        >
          <Paperclip size={16} />
        </button>
        <button
          type="button"
          className="p-1.5 rounded transition-colors flex-shrink-0"
          style={{ color: "var(--ctp-overlay2)" }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          title="Attach Folder"
        >
          <FolderOpen size={16} />
        </button>
        <div
          className="flex-1 rounded px-3 py-1.5 min-h-[32px] max-h-[120px] overflow-y-auto"
          style={{ backgroundColor: "var(--ctp-surface0)", border: "1px solid var(--ctp-surface2)" }}
        >
          <textarea
            placeholder="Type a message..."
            rows={1}
            className="w-full bg-transparent text-sm outline-none resize-none placeholder:opacity-40"
            style={{ color: "var(--ctp-text)" }}
          />
        </div>
        <button
          type="button"
          className="p-1.5 rounded transition-colors flex-shrink-0"
          style={{ color: "var(--ctp-blue)" }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          title="Send (Ctrl+Enter)"
        >
          <Send size={16} />
        </button>
      </div>
    </div>
  )
}
```

---

### 4. Status Bar (`status-bar.tsx`)

**Layout:**
- Height: 28px (`h-7`)
- Padding: 12px horizontal (`px-3`)
- Background: `--ctp-crust` (#11111b)
- Border top: 1px solid `--ctp-surface0` (#313244)
- Flex row, items centered

**Content:**
- Status text: 11px (`text-[11px]`), color `--ctp-overlay1` (#7f849c)
- "Logs" button (right-aligned):
  - Padding: 6px horizontal, 2px vertical (`px-1.5 py-0.5`)
  - Font: 11px (`text-[11px]`)
  - Icon size: 12px
  - Gap: 4px (`gap-1`)
  - Color: `--ctp-overlay1` (#7f849c)
  - Hover background: `--ctp-surface0` (#313244)
  - Border radius: 4px (`rounded`)

**Complete Code:**
```tsx
"use client"

import { ScrollText } from "lucide-react"

export function StatusBar() {
  return (
    <div
      className="flex items-center h-7 px-3 gap-2 flex-shrink-0"
      style={{ backgroundColor: "var(--ctp-crust)", borderTop: "1px solid var(--ctp-surface0)" }}
    >
      <span className="text-[11px] flex-1 truncate" style={{ color: "var(--ctp-overlay1)" }}>
        Ready (listening on 42069)
      </span>
      <button
        type="button"
        className="flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] transition-colors"
        style={{ color: "var(--ctp-overlay1)", backgroundColor: "transparent" }}
        onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
        onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
      >
        <ScrollText size={12} />
        Logs
      </button>
    </div>
  )
}
```

---

### 5. Settings Dialog (`settings-dialog.tsx`)

**Dialog Container:**
- Modal overlay: rgba(17, 17, 27, 0.7) - semi-transparent crust color
- Dialog width: 540px
- Max height: 85vh
- Background: `--ctp-base` (#1e1e2e)
- Border: 1px solid `--ctp-surface1` (#45475a)
- Border radius: 8px (`rounded-lg`)
- Shadow: 2xl

**Header:**
- Padding: 16px horizontal, 12px vertical (`px-4 py-3`)
- Background: `--ctp-mantle` (#181825)
- Border bottom: 1px solid `--ctp-surface0` (#313244)
- Title: 14px (`text-sm`), semibold, color `--ctp-text`
- Close button: 16px icon, hover background `--ctp-surface0`

**Form Content:**
- Padding: 16px horizontal, 16px vertical (`px-4 py-4`)
- Vertical spacing between sections: 16px (`space-y-4`)

**Section Containers:**
- Background: `--ctp-mantle` (#181825)
- Border: 1px solid `--ctp-surface0` (#313244)
- Padding: 12px (`px-3 py-3`)
- Border radius: 6px (`rounded-md`)
- Section title: 11px (`text-[11px]`), semibold, uppercase, wide tracking, color `--ctp-overlay2`
- Vertical spacing within section: 12px (`space-y-3`)

**Form Elements:**
- FormLabel: 12px (`text-xs`), medium weight, color `--ctp-subtext0`, block, margin bottom 4px
- FormHint: 10px (`text-[10px]`), color `--ctp-overlay0`, margin top 2px
- Text inputs:
  - Padding: 10px horizontal, 6px vertical (`px-2.5 py-1.5`)
  - Font: 14px (`text-sm`)
  - Background: `--ctp-surface0` (#313244)
  - Border: 1px solid `--ctp-surface2` (#585b70)
  - Focus border: `--ctp-blue` (#89b4fa)
  - Disabled background: `--ctp-crust` (#11111b)
  - Disabled color: `--ctp-overlay0` (#6c7086)
- Select dropdowns: Same styling as text inputs
- Read-only fields:
  - Background: `--ctp-crust` (#11111b)
  - Color: `--ctp-overlay2` (#9399b2)
  - Font: 12px monospace (`text-xs font-mono`)
- Copy button:
  - Padding: 8px horizontal, 6px vertical (`px-2 py-1.5`)
  - Font: 11px (`text-[11px]`)
  - Background: `--ctp-surface0` (#313244)
  - Hover background: `--ctp-surface1` (#45475a)
- Checkboxes:
  - Size: 14px × 14px (`w-3.5 h-3.5`)
  - Accent color: `#89b4fa` (blue)
- Browse button: Same as copy button styling

**Danger Zone:**
- Border: 1px solid rgba(243, 139, 168, 0.2) (red with 20% opacity)
- Title color: `--ctp-red` (#f38ba8)
- Button:
  - Background: rgba(243, 139, 168, 0.1) (red with 10% opacity)
  - Hover background: rgba(243, 139, 168, 0.2)
  - Color: `--ctp-red` (#f38ba8)
  - Padding: 12px horizontal, 6px vertical (`px-3 py-1.5`)
  - Font: 12px (`text-xs`)

**Footer:**
- Padding: 16px horizontal, 12px vertical (`px-4 py-3`)
- Background: `--ctp-mantle` (#181825)
- Border top: 1px solid `--ctp-surface0` (#313244)
- Buttons:
  - Cancel: Background `--ctp-surface0`, color `--ctp-subtext0`
  - Save: Background `--ctp-blue`, color `--ctp-base`, hover background `--ctp-lavender`
  - Padding: 12px horizontal, 6px vertical (`px-3 py-1.5` for Cancel, `px-4` for Save)
  - Font: 12px (`text-xs`), Save is medium weight
  - Gap between buttons: 8px (`gap-2`)

**Complete Code:**
```tsx
"use client"

import React from "react"

import { useState } from "react"
import { X, Copy, FolderOpen, AlertTriangle } from "lucide-react"

interface SettingsDialogProps {
  open: boolean
  onClose: () => void
}

function FormLabel({ children }: { children: React.ReactNode }) {
  return (
    <label className="text-xs font-medium block mb-1" style={{ color: "var(--ctp-subtext0)" }}>
      {children}
    </label>
  )
}

function FormHint({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[10px] mt-0.5" style={{ color: "var(--ctp-overlay0)" }}>
      {children}
    </p>
  )
}

function ReadOnlyField({ label, value, copyable }: { label: string; value: string; copyable?: boolean }) {
  return (
    <div>
      <FormLabel>{label}</FormLabel>
      <div className="flex items-center gap-2">
        <div
          className="flex-1 px-2.5 py-1.5 rounded text-xs font-mono truncate"
          style={{ backgroundColor: "var(--ctp-crust)", color: "var(--ctp-overlay2)", border: "1px solid var(--ctp-surface0)" }}
        >
          {value}
        </div>
        {copyable && (
          <button
            type="button"
            className="flex items-center gap-1 px-2 py-1.5 rounded text-[11px] transition-colors flex-shrink-0"
            style={{ color: "var(--ctp-subtext0)", backgroundColor: "var(--ctp-surface0)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          >
            <Copy size={11} />
            Copy
          </button>
        )}
      </div>
    </div>
  )
}

export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const [deviceName, setDeviceName] = useState("My Desktop")
  const [portMode, setPortMode] = useState<"automatic" | "fixed">("automatic")
  const [port, setPort] = useState("42069")
  const [downloadLocation, setDownloadLocation] = useState("/home/user/Downloads")
  const [maxFileSize, setMaxFileSize] = useState("unlimited")
  const [notifications, setNotifications] = useState(true)
  const [messageRetention, setMessageRetention] = useState("forever")
  const [fileCleanup, setFileCleanup] = useState(false)

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" style={{ backgroundColor: "rgba(17, 17, 27, 0.7)" }}>
      <div
        className="flex flex-col rounded-lg overflow-hidden shadow-2xl"
        style={{
          backgroundColor: "var(--ctp-base)",
          border: "1px solid var(--ctp-surface1)",
          width: "540px",
          maxHeight: "85vh",
        }}
      >
        {/* Header */}
        <div
          className="flex items-center px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderBottom: "1px solid var(--ctp-surface0)" }}
        >
          <h2 className="text-sm font-semibold flex-1" style={{ color: "var(--ctp-text)" }}>
            Device Settings
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded transition-colors"
            style={{ color: "var(--ctp-overlay2)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Form Content */}
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4 min-h-0">
          {/* Identity Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Identity
            </h3>

            {/* Device Name */}
            <div>
              <FormLabel>Device Name</FormLabel>
              <input
                type="text"
                value={deviceName}
                onChange={(e) => setDeviceName(e.target.value)}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none transition-colors"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
                onFocus={(e) => { e.currentTarget.style.borderColor = "var(--ctp-blue)" }}
                onBlur={(e) => { e.currentTarget.style.borderColor = "var(--ctp-surface2)" }}
              />
            </div>

            <ReadOnlyField label="Device ID" value="f47ac10b-58cc-4372-a567-0e02b2c3d479" copyable />
            <ReadOnlyField label="Fingerprint" value="SHA256:xK4rQp8...mN2vB7w" copyable />
          </div>

          {/* Network Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Network
            </h3>

            {/* Port Mode */}
            <div>
              <FormLabel>Port Mode</FormLabel>
              <select
                value={portMode}
                onChange={(e) => setPortMode(e.target.value as "automatic" | "fixed")}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none appearance-none cursor-pointer"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
              >
                <option value="automatic">Automatic</option>
                <option value="fixed">Fixed</option>
              </select>
            </div>

            {/* Port */}
            <div>
              <FormLabel>Port</FormLabel>
              <input
                type="text"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                disabled={portMode === "automatic"}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none font-mono transition-colors"
                style={{
                  backgroundColor: portMode === "automatic" ? "var(--ctp-crust)" : "var(--ctp-surface0)",
                  color: portMode === "automatic" ? "var(--ctp-overlay0)" : "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                  cursor: portMode === "automatic" ? "not-allowed" : "text",
                }}
                onFocus={(e) => { if (portMode === "fixed") e.currentTarget.style.borderColor = "var(--ctp-blue)" }}
                onBlur={(e) => { e.currentTarget.style.borderColor = "var(--ctp-surface2)" }}
              />
              <FormHint>Changing the port requires an application restart.</FormHint>
            </div>
          </div>

          {/* Storage Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Storage
            </h3>

            {/* Download Location */}
            <div>
              <FormLabel>Download Location</FormLabel>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={downloadLocation}
                  onChange={(e) => setDownloadLocation(e.target.value)}
                  className="flex-1 px-2.5 py-1.5 rounded text-sm font-mono outline-none transition-colors"
                  style={{
                    backgroundColor: "var(--ctp-surface0)",
                    color: "var(--ctp-text)",
                    border: "1px solid var(--ctp-surface2)",
                  }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = "var(--ctp-blue)" }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = "var(--ctp-surface2)" }}
                />
                <button
                  type="button"
                  className="flex items-center gap-1 px-2.5 py-1.5 rounded text-[11px] transition-colors flex-shrink-0"
                  style={{ color: "var(--ctp-subtext0)", backgroundColor: "var(--ctp-surface0)" }}
                  onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
                  onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
                >
                  <FolderOpen size={12} />
                  Browse
                </button>
              </div>
            </div>

            {/* Max File Size */}
            <div>
              <FormLabel>Max File Size</FormLabel>
              <select
                value={maxFileSize}
                onChange={(e) => setMaxFileSize(e.target.value)}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none appearance-none cursor-pointer"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
              >
                <option value="unlimited">Unlimited</option>
                <option value="100mb">100 MB</option>
                <option value="500mb">500 MB</option>
                <option value="1gb">1 GB</option>
                <option value="5gb">5 GB</option>
                <option value="custom">Custom</option>
              </select>
              <FormHint>Set to 0 or Unlimited for no size limit on incoming files.</FormHint>
            </div>
          </div>

          {/* Preferences Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Preferences
            </h3>

            {/* Notifications */}
            <label className="flex items-center gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={notifications}
                onChange={(e) => setNotifications(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-[#89b4fa]"
              />
              <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                Enable desktop notifications
              </span>
            </label>

            {/* Message Retention */}
            <div>
              <FormLabel>Message Retention</FormLabel>
              <select
                value={messageRetention}
                onChange={(e) => setMessageRetention(e.target.value)}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none appearance-none cursor-pointer"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
              >
                <option value="forever">Keep forever</option>
                <option value="30">30 days</option>
                <option value="90">90 days</option>
                <option value="365">1 year</option>
              </select>
            </div>

            {/* File Cleanup */}
            <label className="flex items-center gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={fileCleanup}
                onChange={(e) => setFileCleanup(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-[#89b4fa]"
              />
              <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                Delete downloaded files when purging metadata
              </span>
            </label>
          </div>

          {/* Danger Zone */}
          <div
            className="rounded-md px-3 py-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid rgba(243, 139, 168, 0.2)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider mb-2" style={{ color: "var(--ctp-red)" }}>
              Danger Zone
            </h3>
            <button
              type="button"
              className="flex items-center gap-1.5 px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: "var(--ctp-red)",
                backgroundColor: "rgba(243, 139, 168, 0.1)",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "rgba(243, 139, 168, 0.2)" }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "rgba(243, 139, 168, 0.1)" }}
            >
              <AlertTriangle size={13} />
              Reset Identity Keys
            </button>
            <FormHint>Regenerates your Ed25519 keys. Existing peers will see a key-change warning. Requires restart.</FormHint>
          </div>
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-end gap-2 px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderTop: "1px solid var(--ctp-surface0)" }}
        >
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded text-xs transition-colors"
            style={{
              color: "var(--ctp-subtext0)",
              backgroundColor: "var(--ctp-surface0)",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-1.5 rounded text-xs font-medium transition-colors"
            style={{
              color: "var(--ctp-base)",
              backgroundColor: "var(--ctp-blue)",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-lavender)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-blue)" }}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
```

---

### 6. Peer Settings Dialog (`peer-settings-dialog.tsx`)

**Layout:**
- Same structure as Settings Dialog
- Width: 520px (slightly narrower)
- Same modal overlay and styling

**Header:**
- Contains peer name subtitle: 11px (`text-[11px]`), color `--ctp-overlay1`, margin top 2px

**Sections:**
- Same styling as Settings Dialog
- Sections: Peer Identity, Display, Trust & Verification, File Transfers, Danger Zone

**Special Elements:**
- Verified checkbox with description:
  - Checkbox has margin top 2px (`mt-0.5`)
  - Description text: 10px (`text-[10px]`), color `--ctp-overlay0`, margin top 2px
- Conditional download directory (shown when "Use global download directory" is unchecked):
  - Indented 24px (`ml-6`)

**Danger Zone:**
- Button uses Trash2 icon instead of AlertTriangle
- Text: "Clear Chat History"

**Complete Code:**
```tsx
"use client"

import React from "react"

import { useState } from "react"
import { X, Copy, FolderOpen, Trash2 } from "lucide-react"

interface PeerSettingsDialogProps {
  open: boolean
  onClose: () => void
  peerName: string
}

function FormLabel({ children }: { children: React.ReactNode }) {
  return (
    <label className="text-xs font-medium block mb-1" style={{ color: "var(--ctp-subtext0)" }}>
      {children}
    </label>
  )
}

function FormHint({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[10px] mt-0.5" style={{ color: "var(--ctp-overlay0)" }}>
      {children}
    </p>
  )
}

function ReadOnlyField({ label, value, copyable }: { label: string; value: string; copyable?: boolean }) {
  return (
    <div>
      <FormLabel>{label}</FormLabel>
      <div className="flex items-center gap-2">
        <div
          className="flex-1 px-2.5 py-1.5 rounded text-xs font-mono truncate"
          style={{ backgroundColor: "var(--ctp-crust)", color: "var(--ctp-overlay2)", border: "1px solid var(--ctp-surface0)" }}
        >
          {value}
        </div>
        {copyable && (
          <button
            type="button"
            className="flex items-center gap-1 px-2 py-1.5 rounded text-[11px] transition-colors flex-shrink-0"
            style={{ color: "var(--ctp-subtext0)", backgroundColor: "var(--ctp-surface0)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          >
            <Copy size={11} />
            Copy
          </button>
        )}
      </div>
    </div>
  )
}

export function PeerSettingsDialog({ open, onClose, peerName }: PeerSettingsDialogProps) {
  const [customName, setCustomName] = useState("")
  const [trustLevel, setTrustLevel] = useState<"normal" | "trusted">("trusted")
  const [verified, setVerified] = useState(true)
  const [muteNotifications, setMuteNotifications] = useState(false)
  const [autoAccept, setAutoAccept] = useState(false)
  const [maxFileSize, setMaxFileSize] = useState("global")
  const [useGlobalDownload, setUseGlobalDownload] = useState(true)
  const [downloadDir, setDownloadDir] = useState("/home/user/Downloads")

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" style={{ backgroundColor: "rgba(17, 17, 27, 0.7)" }}>
      <div
        className="flex flex-col rounded-lg overflow-hidden shadow-2xl"
        style={{
          backgroundColor: "var(--ctp-base)",
          border: "1px solid var(--ctp-surface1)",
          width: "520px",
          maxHeight: "85vh",
        }}
      >
        {/* Header */}
        <div
          className="flex items-center px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderBottom: "1px solid var(--ctp-surface0)" }}
        >
          <div className="flex-1 min-w-0">
            <h2 className="text-sm font-semibold" style={{ color: "var(--ctp-text)" }}>
              Peer Settings
            </h2>
            <p className="text-[11px] mt-0.5 truncate" style={{ color: "var(--ctp-overlay1)" }}>
              {peerName}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded transition-colors"
            style={{ color: "var(--ctp-overlay2)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Form Content */}
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4 min-h-0">
          {/* Identity Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Peer Identity
            </h3>

            <ReadOnlyField label="Device Name" value={peerName} />
            <ReadOnlyField label="Device ID" value="a1b2c3d4-e5f6-7890-abcd-ef1234567890" copyable />
            <ReadOnlyField label="Peer Fingerprint" value="SHA256:pQ9rSt...uV3wX4y" copyable />
            <ReadOnlyField label="Local Fingerprint" value="SHA256:xK4rQp8...mN2vB7w" copyable />
          </div>

          {/* Display Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Display
            </h3>

            {/* Custom Name */}
            <div>
              <FormLabel>Custom Name</FormLabel>
              <input
                type="text"
                value={customName}
                onChange={(e) => setCustomName(e.target.value)}
                placeholder="Leave empty to use device name"
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none transition-colors placeholder:opacity-40"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
                onFocus={(e) => { e.currentTarget.style.borderColor = "var(--ctp-blue)" }}
                onBlur={(e) => { e.currentTarget.style.borderColor = "var(--ctp-surface2)" }}
              />
            </div>
          </div>

          {/* Trust & Verification Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              Trust & Verification
            </h3>

            {/* Trust Level */}
            <div>
              <FormLabel>Trust Level</FormLabel>
              <select
                value={trustLevel}
                onChange={(e) => setTrustLevel(e.target.value as "normal" | "trusted")}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none appearance-none cursor-pointer"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
              >
                <option value="normal">Normal</option>
                <option value="trusted">Trusted</option>
              </select>
              <FormHint>Trust level is currently informational metadata shown as a badge.</FormHint>
            </div>

            {/* Verified */}
            <label className="flex items-start gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={verified}
                onChange={(e) => setVerified(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-[#89b4fa] mt-0.5"
              />
              <div>
                <span className="text-sm block" style={{ color: "var(--ctp-text)" }}>
                  Verified out-of-band
                </span>
                <span className="text-[10px] block mt-0.5" style={{ color: "var(--ctp-overlay0)" }}>
                  You manually compared fingerprints over a separate trusted channel.
                </span>
              </div>
            </label>
          </div>

          {/* File Transfer Section */}
          <div
            className="rounded-md px-3 py-3 space-y-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid var(--ctp-surface0)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--ctp-overlay2)" }}>
              File Transfers
            </h3>

            {/* Notifications */}
            <label className="flex items-center gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={muteNotifications}
                onChange={(e) => setMuteNotifications(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-[#89b4fa]"
              />
              <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                Mute notifications for this peer
              </span>
            </label>

            {/* Auto-Accept */}
            <label className="flex items-center gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={autoAccept}
                onChange={(e) => setAutoAccept(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-[#89b4fa]"
              />
              <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                Auto-accept incoming files
              </span>
            </label>

            {/* Max File Size */}
            <div>
              <FormLabel>Max File Size</FormLabel>
              <select
                value={maxFileSize}
                onChange={(e) => setMaxFileSize(e.target.value)}
                className="w-full px-2.5 py-1.5 rounded text-sm outline-none appearance-none cursor-pointer"
                style={{
                  backgroundColor: "var(--ctp-surface0)",
                  color: "var(--ctp-text)",
                  border: "1px solid var(--ctp-surface2)",
                }}
              >
                <option value="global">Use global default</option>
                <option value="100mb">100 MB</option>
                <option value="500mb">500 MB</option>
                <option value="1gb">1 GB</option>
                <option value="5gb">5 GB</option>
                <option value="custom">Custom</option>
              </select>
              <FormHint>Set to 0 to use the global default from device settings.</FormHint>
            </div>

            {/* Download Directory */}
            <div>
              <label className="flex items-center gap-2.5 cursor-pointer mb-2">
                <input
                  type="checkbox"
                  checked={useGlobalDownload}
                  onChange={(e) => setUseGlobalDownload(e.target.checked)}
                  className="w-3.5 h-3.5 rounded accent-[#89b4fa]"
                />
                <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                  Use global download directory
                </span>
              </label>
              {!useGlobalDownload && (
                <div className="flex items-center gap-2 ml-6">
                  <input
                    type="text"
                    value={downloadDir}
                    onChange={(e) => setDownloadDir(e.target.value)}
                    className="flex-1 px-2.5 py-1.5 rounded text-sm font-mono outline-none transition-colors"
                    style={{
                      backgroundColor: "var(--ctp-surface0)",
                      color: "var(--ctp-text)",
                      border: "1px solid var(--ctp-surface2)",
                    }}
                    onFocus={(e) => { e.currentTarget.style.borderColor = "var(--ctp-blue)" }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = "var(--ctp-surface2)" }}
                  />
                  <button
                    type="button"
                    className="flex items-center gap-1 px-2.5 py-1.5 rounded text-[11px] transition-colors flex-shrink-0"
                    style={{ color: "var(--ctp-subtext0)", backgroundColor: "var(--ctp-surface0)" }}
                    onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
                    onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
                  >
                    <FolderOpen size={12} />
                    Browse
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* Danger Zone */}
          <div
            className="rounded-md px-3 py-3"
            style={{ backgroundColor: "var(--ctp-mantle)", border: "1px solid rgba(243, 139, 168, 0.2)" }}
          >
            <h3 className="text-[11px] font-semibold uppercase tracking-wider mb-2" style={{ color: "var(--ctp-red)" }}>
              Danger Zone
            </h3>
            <button
              type="button"
              className="flex items-center gap-1.5 px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: "var(--ctp-red)",
                backgroundColor: "rgba(243, 139, 168, 0.1)",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "rgba(243, 139, 168, 0.2)" }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "rgba(243, 139, 168, 0.1)" }}
            >
              <Trash2 size={13} />
              Clear Chat History
            </button>
            <FormHint>Deletes all messages and transfer history with this peer.</FormHint>
          </div>
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-end gap-2 px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderTop: "1px solid var(--ctp-surface0)" }}
        >
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded text-xs transition-colors"
            style={{
              color: "var(--ctp-subtext0)",
              backgroundColor: "var(--ctp-surface0)",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-1.5 rounded text-xs font-medium transition-colors"
            style={{
              color: "var(--ctp-base)",
              backgroundColor: "var(--ctp-blue)",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-lavender)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-blue)" }}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
```

---

### 7. Discover Dialog (`discover-dialog.tsx`)

**Layout:**
- Width: 520px
- Max height: 460px
- Same modal overlay and container styling as Settings Dialog

**Header:**
- Title: "Discover Peers"
- Subtitle: 11px (`text-[11px]`), color `--ctp-overlay1`, margin top 2px

**Peer List:**
- Each peer item:
  - Padding: 16px horizontal, 10px vertical (`px-4 py-2.5`)
  - Border bottom: 1px solid `--ctp-surface0` (#313244)
  - Selected background: `--ctp-surface0` (#313244)
  - Hover background (when not selected): rgba(49, 50, 68, 0.5)
- Peer item content:
  - Wifi/WifiOff icon: 14px
    - Online: `--ctp-green` (#a6e3a1)
    - Offline: `--ctp-overlay0` (#6c7086)
  - Peer name: 14px (`text-sm`), color `--ctp-text`
  - "added" badge: 10px (`text-[10px]`), padding 6px horizontal, 2px vertical (`px-1.5 py-px`), color `--ctp-blue`, background rgba(137, 180, 250, 0.1)
  - Address:port (right-aligned): 11px monospace (`text-[11px] font-mono`), color `--ctp-overlay1`
  - Status text (indented 22px `ml-[22px]`): 11px, color varies by status

**Footer:**
- Left: Refresh button
  - Background: `--ctp-surface0` (#313244)
  - Color: `--ctp-subtext1` (#bac2de)
  - Icon spins when scanning (`animate-spin`)
- Right: Add Selected and Close buttons
  - Add Selected: Enabled when peer is selected, online, and not already added
    - Enabled: Background `--ctp-blue`, color `--ctp-base`, hover `--ctp-lavender`
    - Disabled: Background `--ctp-surface0`, color `--ctp-surface2`
  - Close: Same styling as other secondary buttons

**Complete Code:**
```tsx
"use client"

import { useState } from "react"
import { RefreshCw, Plus, X, Wifi, WifiOff } from "lucide-react"

interface DiscoveredPeer {
  id: string
  name: string
  address: string
  port: number
  online: boolean
  alreadyAdded: boolean
}

const mockDiscovered: DiscoveredPeer[] = [
  { id: "a1b2c3", name: "Alice's MacBook", address: "192.168.1.12", port: 42069, online: true, alreadyAdded: true },
  { id: "d4e5f6", name: "Bob", address: "192.168.1.34", port: 42069, online: true, alreadyAdded: true },
  { id: "x1y2z3", name: "Grace's Laptop", address: "192.168.1.55", port: 42069, online: true, alreadyAdded: false },
  { id: "q9w8e7", name: "Heidi's PC", address: "192.168.1.87", port: 42070, online: true, alreadyAdded: false },
  { id: "r6t5y4", name: "ivan-server", address: "192.168.1.100", port: 42069, online: false, alreadyAdded: false },
]

interface DiscoverDialogProps {
  open: boolean
  onClose: () => void
}

export function DiscoverDialog({ open, onClose }: DiscoverDialogProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  if (!open) return null

  const handleRefresh = () => {
    setScanning(true)
    setTimeout(() => setScanning(false), 1500)
  }

  const selectedPeer = mockDiscovered.find((p) => p.id === selectedId)
  const canAdd = selectedPeer && !selectedPeer.alreadyAdded && selectedPeer.online

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" style={{ backgroundColor: "rgba(17, 17, 27, 0.7)" }}>
      <div
        className="flex flex-col rounded-lg overflow-hidden shadow-2xl"
        style={{
          backgroundColor: "var(--ctp-base)",
          border: "1px solid var(--ctp-surface1)",
          width: "520px",
          maxHeight: "460px",
        }}
      >
        {/* Header */}
        <div
          className="flex items-center px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderBottom: "1px solid var(--ctp-surface0)" }}
        >
          <div className="flex-1">
            <h2 className="text-sm font-semibold" style={{ color: "var(--ctp-text)" }}>
              Discover Peers
            </h2>
            <p className="text-[11px] mt-0.5" style={{ color: "var(--ctp-overlay1)" }}>
              Peers discovered on your local network
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded transition-colors"
            style={{ color: "var(--ctp-overlay2)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Peer List */}
        <div className="flex-1 overflow-y-auto min-h-0">
          {mockDiscovered.map((peer) => {
            const isSelected = peer.id === selectedId
            return (
              <button
                key={peer.id}
                type="button"
                onClick={() => setSelectedId(peer.id)}
                className="w-full text-left px-4 py-2.5 transition-colors"
                style={{
                  backgroundColor: isSelected ? "var(--ctp-surface0)" : "transparent",
                  borderBottom: "1px solid var(--ctp-surface0)",
                }}
                onMouseEnter={(e) => {
                  if (!isSelected) e.currentTarget.style.backgroundColor = "rgba(49, 50, 68, 0.5)"
                }}
                onMouseLeave={(e) => {
                  if (!isSelected) e.currentTarget.style.backgroundColor = "transparent"
                }}
              >
                <div className="flex items-center gap-2">
                  {peer.online ? (
                    <Wifi size={14} style={{ color: "var(--ctp-green)" }} />
                  ) : (
                    <WifiOff size={14} style={{ color: "var(--ctp-overlay0)" }} />
                  )}
                  <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                    {peer.name}
                  </span>
                  {peer.alreadyAdded && (
                    <span
                      className="text-[10px] px-1.5 py-px rounded"
                      style={{ color: "var(--ctp-blue)", backgroundColor: "rgba(137, 180, 250, 0.1)" }}
                    >
                      added
                    </span>
                  )}
                  <span className="ml-auto text-[11px] font-mono" style={{ color: "var(--ctp-overlay1)" }}>
                    {peer.address}:{peer.port}
                  </span>
                </div>
                <div className="ml-[22px] mt-0.5">
                  <span className="text-[11px]" style={{ color: peer.online ? "var(--ctp-green)" : "var(--ctp-overlay0)" }}>
                    {peer.online ? "Online" : "Offline"}
                  </span>
                </div>
              </button>
            )
          })}
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-between px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderTop: "1px solid var(--ctp-surface0)" }}
        >
          <button
            type="button"
            onClick={handleRefresh}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded text-xs transition-colors"
            style={{
              color: "var(--ctp-subtext1)",
              backgroundColor: "var(--ctp-surface0)",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
          >
            <RefreshCw size={13} className={scanning ? "animate-spin" : ""} />
            Refresh
          </button>

          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={!canAdd}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: canAdd ? "var(--ctp-base)" : "var(--ctp-surface2)",
                backgroundColor: canAdd ? "var(--ctp-blue)" : "var(--ctp-surface0)",
                cursor: canAdd ? "pointer" : "default",
              }}
              onMouseEnter={(e) => { if (canAdd) e.currentTarget.style.backgroundColor = "var(--ctp-lavender)" }}
              onMouseLeave={(e) => { if (canAdd) e.currentTarget.style.backgroundColor = "var(--ctp-blue)" }}
            >
              <Plus size={13} />
              Add Selected
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: "var(--ctp-subtext0)",
                backgroundColor: "var(--ctp-surface0)",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
```

---

### 8. Transfer Queue Dialog (`transfer-queue-dialog.tsx`)

**Layout:**
- Width: 760px (wider than other dialogs)
- Max height: 520px
- Same modal overlay and container styling

**Header:**
- Title: "Transfer Queue"
- Subtitle shows counts: 11px (`text-[11px]`), color `--ctp-overlay1`

**Filter Tabs:**
- Container: Background `--ctp-surface0` (#313244), border bottom 1px solid `--ctp-surface1` (#45475a)
- Padding: 16px horizontal, 8px vertical (`px-4 py-2`)
- Gap between tabs: 6px (`gap-1.5`)
- Tab buttons:
  - Padding: 10px horizontal, 4px vertical (`px-2.5 py-1`)
  - Font: 12px (`text-xs`)
  - Selected: Background `--ctp-blue`, color `--ctp-base`
  - Unselected: Background transparent, color `--ctp-subtext0`, hover background `--ctp-surface1`

**Transfer List Items:**
- Each transfer:
  - Padding: 16px horizontal, 12px vertical (`px-4 py-3`)
  - Border bottom: 1px solid `--ctp-surface0` (#313244)
- Transfer content:
  - Direction icon: 14px
    - Outbound (Upload): `--ctp-peach` (#fab387)
    - Inbound (Download): `--ctp-teal` (#94e2d5)
  - File name: 14px (`text-sm`), color `--ctp-text`
  - File size (right-aligned): 11px monospace (`text-[11px] font-mono`), color `--ctp-overlay1`
  - Metadata row (indented 22px `ml-[22px]`):
    - Direction text: 11px, color `--ctp-overlay1`
    - Status: 11px medium, capitalized, color varies:
      - Complete: `--ctp-green`
      - Failed/Canceled: `--ctp-red`
      - Paused: `--ctp-peach`
      - Transferring: `--ctp-blue`
      - Queued: `--ctp-yellow`
    - Speed and ETA: 11px, color `--ctp-overlay1`
  - Progress bar:
    - Height: 4px (`h-1`)
    - Background: `--ctp-surface2` (#585b70)
    - Fill color: matches status color
    - Width: percentage based on progress
  - Stored path (if complete): 11px monospace, color `--ctp-overlay0`
  - Action buttons:
    - Same styling as chat panel action buttons
    - Icons: Pause, Play, RotateCcw (Retry), Ban (Cancel), FolderSearch (Show Path)

**Footer:**
- Left: Status indicators
  - Complete: Green icon + count, 11px (`text-[11px]`)
  - Issues: Red icon +
**Footer:**
- Left: Status indicators
  - Complete: Green icon + count, 11px (`text-[11px]`)
  - Issues: Red icon + count, 11px (`text-[11px]`)
- Right: Action buttons
  - "Clear Completed": Secondary button styling
  - "Close": Primary button styling (blue background)

**Complete Code:**
```tsx
"use client"

import { useMemo, useState } from "react"
import {
  X,
  Upload,
  Download,
  Pause,
  Play,
  RotateCcw,
  FolderSearch,
  Ban,
  CheckCircle2,
  AlertCircle,
} from "lucide-react"

type TransferStatus = "queued" | "transferring" | "paused" | "failed" | "complete" | "canceled"

interface QueueTransfer {
  id: string
  fileName: string
  peerName: string
  direction: "outbound" | "inbound"
  fileSize: string
  status: TransferStatus
  progress: number
  speed?: string
  eta?: string
  path?: string
}

const mockQueue: QueueTransfer[] = [
  {
    id: "q1",
    fileName: "release-build-v4.2.tar.gz",
    peerName: "Alice's MacBook",
    direction: "outbound",
    fileSize: "1.9 GB",
    status: "transferring",
    progress: 42,
    speed: "27.4 MB/s",
    eta: "1m 34s",
  },
  {
    id: "q2",
    fileName: "backend-logs-2026-02-09.zip",
    peerName: "Dev Server",
    direction: "inbound",
    fileSize: "318 MB",
    status: "queued",
    progress: 0,
  },
  {
    id: "q3",
    fileName: "design-review-notes.md",
    peerName: "Bob",
    direction: "inbound",
    fileSize: "17 KB",
    status: "complete",
    progress: 100,
    path: "/home/user/Downloads/design-review-notes.md",
  },
  {
    id: "q4",
    fileName: "session-capture.mp4",
    peerName: "Eve",
    direction: "outbound",
    fileSize: "842 MB",
    status: "paused",
    progress: 63,
    speed: "0 MB/s",
  },
  {
    id: "q5",
    fileName: "database-migration.sql",
    peerName: "Frank's Tablet",
    direction: "outbound",
    fileSize: "2.6 MB",
    status: "failed",
    progress: 78,
  },
  {
    id: "q6",
    fileName: "archive-2025.tar",
    peerName: "Charlie's Phone",
    direction: "inbound",
    fileSize: "4.1 GB",
    status: "canceled",
    progress: 11,
  },
]

interface TransferQueueDialogProps {
  open: boolean
  onClose: () => void
}

export function TransferQueueDialog({ open, onClose }: TransferQueueDialogProps) {
  const [filter, setFilter] = useState<"all" | "active" | "done" | "issues">("all")

  const transfers = useMemo(() => {
    return mockQueue.filter((transfer) => {
      if (filter === "all") return true
      if (filter === "active") {
        return transfer.status === "queued" || transfer.status === "transferring" || transfer.status === "paused"
      }
      if (filter === "done") return transfer.status === "complete"
      return transfer.status === "failed" || transfer.status === "canceled"
    })
  }, [filter])

  const activeCount = mockQueue.filter((t) => t.status === "queued" || t.status === "transferring" || t.status === "paused").length
  const completeCount = mockQueue.filter((t) => t.status === "complete").length
  const issueCount = mockQueue.filter((t) => t.status === "failed" || t.status === "canceled").length

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" style={{ backgroundColor: "rgba(17, 17, 27, 0.7)" }}>
      <div
        className="flex flex-col rounded-lg overflow-hidden shadow-2xl"
        style={{
          backgroundColor: "var(--ctp-base)",
          border: "1px solid var(--ctp-surface1)",
          width: "760px",
          maxHeight: "520px",
        }}
      >
        <div
          className="flex items-center px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderBottom: "1px solid var(--ctp-surface0)" }}
        >
          <div className="flex-1">
            <h2 className="text-sm font-semibold" style={{ color: "var(--ctp-text)" }}>
              Transfer Queue
            </h2>
            <p className="text-[11px] mt-0.5" style={{ color: "var(--ctp-overlay1)" }}>
              {activeCount} active, {completeCount} complete, {issueCount} with issues
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded transition-colors"
            style={{ color: "var(--ctp-overlay2)" }}
            onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
          >
            <X size={16} />
          </button>
        </div>

        <div
          className="flex items-center px-4 py-2 gap-1.5 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-surface0)", borderBottom: "1px solid var(--ctp-surface1)" }}
        >
          {[
            { key: "all", label: "All" },
            { key: "active", label: "Active" },
            { key: "done", label: "Completed" },
            { key: "issues", label: "Issues" },
          ].map((item) => {
            const selected = filter === item.key
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => setFilter(item.key as "all" | "active" | "done" | "issues")}
                className="px-2.5 py-1 rounded text-xs transition-colors"
                style={{
                  color: selected ? "var(--ctp-base)" : "var(--ctp-subtext0)",
                  backgroundColor: selected ? "var(--ctp-blue)" : "transparent",
                }}
                onMouseEnter={(e) => { if (!selected) e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
                onMouseLeave={(e) => { if (!selected) e.currentTarget.style.backgroundColor = "transparent" }}
              >
                {item.label}
              </button>
            )
          })}
        </div>

        <div className="flex-1 overflow-y-auto min-h-0">
          {transfers.map((transfer) => {
            const statusColor =
              transfer.status === "complete"
                ? "var(--ctp-green)"
                : transfer.status === "failed" || transfer.status === "canceled"
                  ? "var(--ctp-red)"
                  : transfer.status === "paused"
                    ? "var(--ctp-peach)"
                    : transfer.status === "transferring"
                      ? "var(--ctp-blue)"
                      : "var(--ctp-yellow)"

            const canPause = transfer.status === "transferring"
            const canResume = transfer.status === "paused"
            const canRetry = transfer.status === "failed" || transfer.status === "canceled"
            const canCancel = transfer.status === "queued" || transfer.status === "transferring" || transfer.status === "paused"
            const canShowPath = transfer.status === "complete" && Boolean(transfer.path)

            return (
              <div
                key={transfer.id}
                className="px-4 py-3 border-b"
                style={{ borderColor: "var(--ctp-surface0)" }}
              >
                <div className="flex items-center gap-2">
                  {transfer.direction === "outbound" ? (
                    <Upload size={14} style={{ color: "var(--ctp-peach)" }} />
                  ) : (
                    <Download size={14} style={{ color: "var(--ctp-teal)" }} />
                  )}
                  <span className="text-sm" style={{ color: "var(--ctp-text)" }}>
                    {transfer.fileName}
                  </span>
                  <span className="text-[11px] ml-auto font-mono" style={{ color: "var(--ctp-overlay1)" }}>
                    {transfer.fileSize}
                  </span>
                </div>

                <div className="flex items-center gap-2 mt-1 ml-[22px]">
                  <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>
                    {transfer.direction === "outbound" ? "To" : "From"} {transfer.peerName}
                  </span>
                  <span className="text-[11px] font-medium capitalize" style={{ color: statusColor }}>
                    {transfer.status}
                  </span>
                  {transfer.speed && (
                    <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>
                      {transfer.speed}
                    </span>
                  )}
                  {transfer.eta && (
                    <span className="text-[11px]" style={{ color: "var(--ctp-overlay1)" }}>
                      ETA {transfer.eta}
                    </span>
                  )}
                </div>

                <div className="mt-1.5 ml-[22px] h-1 rounded-full overflow-hidden" style={{ backgroundColor: "var(--ctp-surface2)" }}>
                  <div
                    className="h-full rounded-full transition-all"
                    style={{ width: `${transfer.progress}%`, backgroundColor: statusColor }}
                  />
                </div>

                {transfer.path && (
                  <div className="mt-1 ml-[22px]">
                    <span className="text-[11px] font-mono" style={{ color: "var(--ctp-overlay0)" }}>
                      {transfer.path}
                    </span>
                  </div>
                )}

                <div className="mt-2 ml-[22px] flex items-center gap-1.5">
                  {[
                    { icon: Pause, label: "Pause", enabled: canPause },
                    { icon: Play, label: "Resume", enabled: canResume },
                    { icon: RotateCcw, label: "Retry", enabled: canRetry },
                    { icon: Ban, label: "Cancel", enabled: canCancel },
                    { icon: FolderSearch, label: "Show Path", enabled: canShowPath },
                  ].map(({ icon: Icon, label, enabled }) => (
                    <button
                      key={label}
                      type="button"
                      disabled={!enabled}
                      className="flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors"
                      style={{
                        color: enabled ? "var(--ctp-subtext0)" : "var(--ctp-surface2)",
                        backgroundColor: "transparent",
                        cursor: enabled ? "pointer" : "default",
                      }}
                      onMouseEnter={(e) => { if (enabled) e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
                      onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "transparent" }}
                    >
                      <Icon size={12} />
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            )
          })}
        </div>

        <div
          className="flex items-center justify-between px-4 py-3 flex-shrink-0"
          style={{ backgroundColor: "var(--ctp-mantle)", borderTop: "1px solid var(--ctp-surface0)" }}
        >
          <div className="flex items-center gap-3 text-[11px]">
            <span className="flex items-center gap-1" style={{ color: "var(--ctp-green)" }}>
              <CheckCircle2 size={12} />
              Complete: {completeCount}
            </span>
            <span className="flex items-center gap-1" style={{ color: "var(--ctp-red)" }}>
              <AlertCircle size={12} />
              Issues: {issueCount}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: "var(--ctp-subtext0)",
                backgroundColor: "var(--ctp-surface0)",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface1)" }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-surface0)" }}
            >
              Clear Completed
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded text-xs transition-colors"
              style={{
                color: "var(--ctp-base)",
                backgroundColor: "var(--ctp-blue)",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-lavender)" }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = "var(--ctp-blue)" }}
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
```

---

## Summary: Typography & Spacing System

**Font Sizes:**
- 10px (`text-[10px]`): Hints, small badges
- 11px (`text-[11px]`): Status text, metadata, timestamps, small buttons
- 12px (`text-xs`): Labels, buttons, section headers
- 14px (`text-sm`): Body text, input fields, peer names
- 16px+ (`text-base`): Headers (when used)

**Font Weights:**
- Regular: Default body text
- Medium (`font-medium`): Labels, important text
- Semibold (`font-semibold`): Headers, titles

**Spacing:**
- Padding: 3px (`px-0.5`), 4px (`px-1`), 6px (`px-1.5`), 8px (`px-2`), 10px (`px-2.5`), 12px (`px-3`), 16px (`px-4`)
- Gaps: 4px (`gap-1`), 6px (`gap-1.5`), 8px (`gap-2`), 10px (`gap-2.5`), 12px (`gap-3`)
- Margins: Similar scale

**Border Radius:**
- 4px (`rounded`): Buttons, inputs, small elements
- 6px (`rounded-md`): Section containers
- 8px (`rounded-lg`): Dialog containers
- Full (`rounded-full`): Dots, progress bars

**Icons:**
- All icons from `lucide-react`
- Sizes: 11px, 12px, 13px, 14px, 15px, 16px (varies by context)

This design uses a consistent dark theme with clear hierarchy, subtle hover states, and semantic color usage throughout.
package ui

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/widgets"

	"gosend/network"
	"gosend/storage"
)

func (c *controller) transferByID(fileID string) (chatFileEntry, bool) {
	if strings.TrimSpace(fileID) == "" {
		return chatFileEntry{}, false
	}
	c.chatMu.RLock()
	defer c.chatMu.RUnlock()
	entry, ok := c.fileTransfers[fileID]
	return entry, ok
}

func (c *controller) cancelTransferFromUI(fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" || c.manager == nil {
		return
	}
	answer := widgets.QMessageBox_Question(c.window, "Cancel Transfer", "Cancel this transfer?", widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)
	if answer != widgets.QMessageBox__Yes {
		return
	}
	go func() {
		if err := c.manager.CancelTransfer(fileID); err != nil {
			c.setStatus(fmt.Sprintf("Cancel transfer failed: %v", err))
			return
		}
		if meta, err := c.store.GetFileByID(fileID); err == nil {
			peerID := meta.ToDeviceID
			if meta.FromDeviceID != c.cfg.DeviceID {
				peerID = meta.FromDeviceID
			}
			c.upsertFileTransfer(chatFileEntry{
				FileID:            fileID,
				PeerDeviceID:      peerID,
				Status:            "failed",
				TransferCompleted: true,
				CompletedAt:       time.Now().UnixMilli(),
				UpdatedAt:         time.Now().UnixMilli(),
			})
			c.refreshChatForPeer(peerID)
		} else {
			c.refreshChatView()
		}
		c.setStatus("Transfer canceled")
	}()
}

func (c *controller) retryTransferFromUI(fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" || c.manager == nil {
		return
	}
	go func() {
		if err := c.manager.RetryTransfer(fileID); err != nil {
			c.setStatus(fmt.Sprintf("Retry transfer failed: %v", err))
			return
		}
		if meta, err := c.store.GetFileByID(fileID); err == nil {
			peerID := meta.ToDeviceID
			if meta.FromDeviceID != c.cfg.DeviceID {
				peerID = meta.FromDeviceID
			}
			c.upsertFileTransfer(chatFileEntry{
				FileID:            fileID,
				PeerDeviceID:      peerID,
				Filename:          meta.Filename,
				Filesize:          meta.Filesize,
				StoredPath:        meta.StoredPath,
				Status:            "pending",
				TransferCompleted: false,
				BytesTransferred:  0,
				TotalBytes:        meta.Filesize,
				UpdatedAt:         time.Now().UnixMilli(),
			})
			c.refreshChatForPeer(peerID)
		} else {
			c.refreshChatView()
		}
		c.setStatus("Transfer re-queued")
	}()
}

func (c *controller) upsertFileTransfer(entry chatFileEntry) {
	if entry.FileID == "" {
		return
	}
	now := time.Now().UnixMilli()
	if entry.UpdatedAt == 0 {
		entry.UpdatedAt = now
	}

	c.chatMu.Lock()
	existing, exists := c.fileTransfers[entry.FileID]
	if exists {
		if entry.FolderID == "" {
			entry.FolderID = existing.FolderID
		}
		if entry.RelativePath == "" {
			entry.RelativePath = existing.RelativePath
		}
		if entry.PeerDeviceID == "" {
			entry.PeerDeviceID = existing.PeerDeviceID
		}
		if entry.Direction == "" {
			entry.Direction = existing.Direction
		}
		if entry.Filename == "" {
			entry.Filename = existing.Filename
		}
		if entry.Filesize == 0 {
			entry.Filesize = existing.Filesize
		}
		if entry.Filetype == "" {
			entry.Filetype = existing.Filetype
		}
		if entry.StoredPath == "" {
			entry.StoredPath = existing.StoredPath
		}
		if entry.TotalBytes == 0 {
			entry.TotalBytes = existing.TotalBytes
		}
		if entry.BytesTransferred == 0 && existing.BytesTransferred > 0 && !entry.TransferCompleted {
			entry.BytesTransferred = existing.BytesTransferred
		}
		if entry.SpeedBytesPerSec == 0 {
			entry.SpeedBytesPerSec = existing.SpeedBytesPerSec
		}
		if entry.ETASeconds == 0 {
			entry.ETASeconds = existing.ETASeconds
		}
		if strings.TrimSpace(entry.Status) == "" {
			entry.Status = existing.Status
		}
		if entry.AddedAt == 0 {
			entry.AddedAt = existing.AddedAt
		}
		if entry.CompletedAt == 0 {
			entry.CompletedAt = existing.CompletedAt
		}
		if !entry.TransferCompleted {
			if isTerminalTransferStatus(entry.Status) {
				entry.TransferCompleted = true
			} else {
				entry.TransferCompleted = existing.TransferCompleted && !strings.EqualFold(entry.Status, "pending")
			}
		}
		if entry.UpdatedAt < existing.UpdatedAt {
			entry.UpdatedAt = existing.UpdatedAt
		}
	} else {
		if entry.AddedAt == 0 {
			entry.AddedAt = now
		}
	}
	if entry.TransferCompleted && entry.CompletedAt == 0 {
		entry.CompletedAt = now
	}
	if entry.TotalBytes == 0 {
		entry.TotalBytes = entry.Filesize
	}
	if entry.TransferCompleted && strings.EqualFold(entry.Status, "") {
		entry.Status = "complete"
	}
	c.fileTransfers[entry.FileID] = entry
	c.chatMu.Unlock()
}

func isTerminalTransferStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "complete", "failed", "rejected":
		return true
	default:
		return false
	}
}

func (c *controller) mergeChatFilesForPeer(peerID string, files []storage.FileMetadata) []chatFileEntry {
	if strings.TrimSpace(peerID) == "" {
		return nil
	}

	c.chatMu.RLock()
	live := make(map[string]chatFileEntry, len(c.fileTransfers))
	for fileID, entry := range c.fileTransfers {
		live[fileID] = entry
	}
	c.chatMu.RUnlock()

	merged := make(map[string]chatFileEntry, len(files))
	for _, meta := range files {
		entry := c.chatFileEntryFromMetadata(meta)
		if entry.PeerDeviceID != peerID {
			continue
		}
		if liveEntry, ok := live[entry.FileID]; ok {
			entry = mergeChatFileEntry(entry, liveEntry)
			if isTerminalTransferStatus(meta.TransferStatus) {
				entry.Status = meta.TransferStatus
				entry.TransferCompleted = true
				if strings.EqualFold(meta.TransferStatus, "complete") {
					entry.BytesTransferred = entry.TotalBytes
				}
			}
			delete(live, entry.FileID)
		}
		merged[entry.FileID] = entry
	}

	for fileID, liveEntry := range live {
		if liveEntry.PeerDeviceID != peerID {
			continue
		}
		if _, exists := merged[fileID]; exists {
			continue
		}
		if isTerminalTransferStatus(liveEntry.Status) && liveEntry.TransferCompleted {
			continue
		}
		merged[fileID] = liveEntry
	}

	out := make([]chatFileEntry, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AddedAt == out[j].AddedAt {
			return out[i].FileID < out[j].FileID
		}
		return out[i].AddedAt < out[j].AddedAt
	})
	return out
}

func (c *controller) chatFileEntryFromMetadata(meta storage.FileMetadata) chatFileEntry {
	peerID := meta.FromDeviceID
	direction := "receive"
	if meta.FromDeviceID == c.cfg.DeviceID {
		peerID = meta.ToDeviceID
		direction = "send"
	}

	addedAt := int64(0)
	if meta.TimestampReceived != nil {
		addedAt = *meta.TimestampReceived
	}

	terminal := isTerminalTransferStatus(meta.TransferStatus)
	completed := strings.EqualFold(meta.TransferStatus, "complete")
	completedAt := int64(0)
	if terminal {
		completedAt = addedAt
	}

	bytesTransferred := int64(0)
	if completed {
		bytesTransferred = meta.Filesize
	}

	return chatFileEntry{
		FileID:            meta.FileID,
		FolderID:          meta.FolderID,
		RelativePath:      meta.RelativePath,
		PeerDeviceID:      peerID,
		Direction:         direction,
		Filename:          meta.Filename,
		Filesize:          meta.Filesize,
		Filetype:          meta.Filetype,
		StoredPath:        meta.StoredPath,
		AddedAt:           addedAt,
		BytesTransferred:  bytesTransferred,
		TotalBytes:        meta.Filesize,
		Status:            meta.TransferStatus,
		TransferCompleted: completed,
		CompletedAt:       completedAt,
		UpdatedAt:         time.Now().UnixMilli(),
	}
}

func mergeChatFileEntry(base, live chatFileEntry) chatFileEntry {
	merged := base
	if merged.FolderID == "" {
		merged.FolderID = live.FolderID
	}
	if merged.RelativePath == "" {
		merged.RelativePath = live.RelativePath
	}
	if merged.PeerDeviceID == "" {
		merged.PeerDeviceID = live.PeerDeviceID
	}
	if merged.Direction == "" {
		merged.Direction = live.Direction
	}
	if merged.Filename == "" {
		merged.Filename = live.Filename
	}
	if merged.Filesize == 0 {
		merged.Filesize = live.Filesize
	}
	if merged.Filetype == "" {
		merged.Filetype = live.Filetype
	}
	if merged.StoredPath == "" {
		merged.StoredPath = live.StoredPath
	}
	if merged.TotalBytes == 0 {
		merged.TotalBytes = live.TotalBytes
	}
	if live.BytesTransferred > merged.BytesTransferred {
		merged.BytesTransferred = live.BytesTransferred
	}
	if live.SpeedBytesPerSec > 0 {
		merged.SpeedBytesPerSec = live.SpeedBytesPerSec
	}
	if live.ETASeconds > 0 {
		merged.ETASeconds = live.ETASeconds
	}
	if strings.TrimSpace(live.Status) != "" && live.UpdatedAt >= merged.UpdatedAt {
		merged.Status = live.Status
	}
	if live.TransferCompleted {
		merged.TransferCompleted = true
	}
	if merged.CompletedAt == 0 {
		merged.CompletedAt = live.CompletedAt
	}
	if merged.AddedAt == 0 {
		merged.AddedAt = live.AddedAt
	}
	if live.UpdatedAt > merged.UpdatedAt {
		merged.UpdatedAt = live.UpdatedAt
	}
	return merged
}

func (c *controller) handleFileProgress(progress network.FileProgress) {
	entry := chatFileEntry{
		FileID:            progress.FileID,
		PeerDeviceID:      progress.PeerDeviceID,
		Direction:         progress.Direction,
		BytesTransferred:  progress.BytesTransferred,
		TotalBytes:        progress.TotalBytes,
		SpeedBytesPerSec:  progress.SpeedBytesPerSec,
		ETASeconds:        progress.ETASeconds,
		Status:            strings.TrimSpace(progress.Status),
		TransferCompleted: progress.TransferCompleted,
		UpdatedAt:         time.Now().UnixMilli(),
	}
	if entry.Status == "" {
		entry.Status = "accepted"
	}

	meta, err := c.store.GetFileByID(progress.FileID)
	if err == nil {
		if entry.PeerDeviceID == "" {
			if meta.FromDeviceID == c.cfg.DeviceID {
				entry.PeerDeviceID = meta.ToDeviceID
			} else {
				entry.PeerDeviceID = meta.FromDeviceID
			}
		}
		entry.Filename = meta.Filename
		entry.Filesize = meta.Filesize
		entry.Filetype = meta.Filetype
		entry.StoredPath = meta.StoredPath
		entry.FolderID = meta.FolderID
		entry.RelativePath = meta.RelativePath
		if isTerminalTransferStatus(meta.TransferStatus) {
			entry.Status = meta.TransferStatus
			entry.TransferCompleted = true
		}
	}

	if entry.TransferCompleted {
		if strings.TrimSpace(entry.Status) == "" || strings.EqualFold(entry.Status, "accepted") {
			entry.Status = "complete"
		}
		entry.CompletedAt = time.Now().UnixMilli()
	}
	if entry.TotalBytes == 0 {
		entry.TotalBytes = entry.Filesize
	}
	if strings.EqualFold(entry.Status, "complete") && entry.TotalBytes > 0 {
		entry.BytesTransferred = entry.TotalBytes
		entry.TransferCompleted = true
	}

	c.upsertFileTransfer(entry)
	if entry.PeerDeviceID != "" {
		c.refreshChatForPeer(entry.PeerDeviceID)
	}
	c.enqueueUI(func() {
		c.renderPeerList(-1)
	})
}

func (c *controller) transferPeerName(peerDeviceID string) string {
	if peer := c.peerByID(peerDeviceID); peer != nil {
		name := c.peerDisplayName(peer)
		if strings.TrimSpace(name) != "" {
			return name
		}
		if strings.TrimSpace(peer.DeviceName) != "" {
			return peer.DeviceName
		}
	}
	if strings.TrimSpace(peerDeviceID) != "" {
		return peerDeviceID
	}
	return "Unknown"
}

func (c *controller) showTransferQueuePanel() {
	c.enqueueUI(func() {
		dlg := widgets.NewQDialog(c.window, 0)
		dlg.SetWindowTitle("Transfer Queue")
		dlg.Resize2(760, 520)

		layout := widgets.NewQVBoxLayout()
		layout.SetContentsMargins(0, 0, 0, 0)
		layout.SetSpacing(0)

		// ── Header (mockup: mantle bg, title + subtitle) ────
		header := widgets.NewQWidget(nil, 0)
		header.SetObjectName("dialogHeader")
		headerLayout := widgets.NewQHBoxLayout()
		headerLayout.SetContentsMargins(16, 10, 16, 10)
		headerLayout.SetSpacing(0)
		headerInfo := widgets.NewQVBoxLayout()
		headerInfo.SetContentsMargins(0, 0, 0, 0)
		headerInfo.SetSpacing(2)
		titleLbl := widgets.NewQLabel2("Transfer Queue", nil, 0)
		titleLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; font-weight: bold; background: transparent;", colorText))
		headerInfo.AddWidget(titleLbl, 0, 0)
		headerLayout.AddLayout(headerInfo, 1)
		header.SetLayout(headerLayout)

		// ── List ────────────────────────────────────────────
		list := widgets.NewQListWidget(nil)
		list.SetObjectName("chatList")
		entries := c.transferQueueEntriesSnapshot()
		render := func() {
			entries = c.transferQueueEntriesSnapshot()
			list.Clear()
			for _, entry := range entries {
				peerName := c.transferPeerName(entry.PeerDeviceID)
				w := createTransferQueueItemWidget(entry, peerName)
				item := widgets.NewQListWidgetItem(list, 0)
				h := itemHeightForWidget(w, 60)
				item.SetSizeHint(core.NewQSize2(0, h))
				list.SetItemWidget(item, w)
			}
		}
		render()

		// ── Footer (mockup: mantle bg, action buttons) ──────
		footer := widgets.NewQWidget(nil, 0)
		footer.SetObjectName("dialogFooter")
		btnLayout := widgets.NewQHBoxLayout()
		btnLayout.SetContentsMargins(16, 10, 16, 10)
		btnLayout.SetSpacing(8)

		cancelBtn := widgets.NewQPushButton2("Cancel Selected", nil)
		cancelBtn.SetObjectName("secondaryBtn")
		retryBtn := widgets.NewQPushButton2("Retry Selected", nil)
		retryBtn.SetObjectName("secondaryBtn")
		clearBtn := widgets.NewQPushButton2("Clear Completed", nil)
		clearBtn.SetObjectName("secondaryBtn")
		closeBtn := widgets.NewQPushButton2("Close", nil)
		closeBtn.SetObjectName("primaryBtn")

		cancelBtn.ConnectClicked(func(bool) {
			idx := list.CurrentRow()
			if idx < 0 || idx >= len(entries) {
				return
			}
			c.cancelTransferFromUI(entries[idx].FileID)
			render()
		})
		retryBtn.ConnectClicked(func(bool) {
			idx := list.CurrentRow()
			if idx < 0 || idx >= len(entries) {
				return
			}
			c.retryTransferFromUI(entries[idx].FileID)
			render()
		})
		clearBtn.ConnectClicked(func(bool) {
			c.clearCompletedTransfers()
			render()
		})
		closeBtn.ConnectClicked(func(bool) { dlg.Accept() })

		btnLayout.AddWidget(cancelBtn, 0, 0)
		btnLayout.AddWidget(retryBtn, 0, 0)
		btnLayout.AddWidget(clearBtn, 0, 0)
		btnLayout.AddStretch(1)
		btnLayout.AddWidget(closeBtn, 0, 0)
		footer.SetLayout(btnLayout)

		layout.AddWidget(header, 0, 0)
		layout.AddWidget(list, 1, 0)
		layout.AddWidget(footer, 0, 0)
		dlg.SetLayout(layout)
		dlg.Exec()
		runtime.KeepAlive(cancelBtn)
		runtime.KeepAlive(retryBtn)
		runtime.KeepAlive(clearBtn)
		runtime.KeepAlive(closeBtn)
		runtime.KeepAlive(list)
		runtime.KeepAlive(dlg)
	})
}

func (c *controller) transferQueueEntriesSnapshot() []chatFileEntry {
	c.chatMu.RLock()
	entries := make([]chatFileEntry, 0, len(c.fileTransfers))
	for _, transfer := range c.fileTransfers {
		entries = append(entries, transfer)
	}
	c.chatMu.RUnlock()

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].TransferCompleted != entries[j].TransferCompleted {
			return !entries[i].TransferCompleted
		}
		if entries[i].PeerDeviceID != entries[j].PeerDeviceID {
			return entries[i].PeerDeviceID < entries[j].PeerDeviceID
		}
		if entries[i].AddedAt != entries[j].AddedAt {
			return entries[i].AddedAt < entries[j].AddedAt
		}
		return entries[i].FileID < entries[j].FileID
	})
	return entries
}

func (c *controller) clearCompletedTransfers() {
	c.chatMu.Lock()
	for fileID, transfer := range c.fileTransfers {
		if transfer.TransferCompleted {
			delete(c.fileTransfers, fileID)
		}
	}
	c.chatMu.Unlock()
	c.refreshChatView()
}

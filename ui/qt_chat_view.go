package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"

	appcrypto "gosend/crypto"
	"gosend/storage"
)

func (c *controller) refreshPeersFromStore() {
	if c.store == nil {
		return
	}

	rows, err := c.store.ListPeers()
	if err != nil {
		c.setStatus(fmt.Sprintf("Load peers failed: %v", err))
		return
	}

	filtered := make([]storage.Peer, 0, len(rows))
	for _, peer := range rows {
		if peer.DeviceID == c.cfg.DeviceID {
			continue
		}
		filtered = append(filtered, peer)
	}

	settingsByPeerID := make(map[string]storage.PeerSettings, len(filtered))
	for _, peer := range filtered {
		if err := c.store.EnsurePeerSettingsExist(peer.DeviceID); err != nil {
			c.setStatus(fmt.Sprintf("Ensure peer settings failed: %v", err))
			continue
		}
		settings, err := c.store.GetPeerSettings(peer.DeviceID)
		if err != nil {
			c.setStatus(fmt.Sprintf("Load peer settings failed: %v", err))
			continue
		}
		settingsByPeerID[peer.DeviceID] = *settings
	}

	peerDisplayName := func(peer storage.Peer) string {
		settings, ok := settingsByPeerID[peer.DeviceID]
		if ok && strings.TrimSpace(settings.CustomName) != "" {
			return strings.TrimSpace(settings.CustomName)
		}
		return strings.TrimSpace(peer.DeviceName)
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := peerDisplayName(filtered[i])
		right := peerDisplayName(filtered[j])
		if left == right {
			return filtered[i].DeviceID < filtered[j].DeviceID
		}
		return left < right
	})

	selectedIndex := -1
	c.peersMu.Lock()
	c.peers = filtered
	c.peerSettings = settingsByPeerID
	if c.selectedPeerID != "" {
		for i := range c.peers {
			if c.peers[i].DeviceID == c.selectedPeerID {
				selectedIndex = i
				break
			}
		}
		if selectedIndex < 0 {
			c.selectedPeerID = ""
		}
	}
	c.peersMu.Unlock()

	c.enqueueUI(func() {
		c.renderPeerList(selectedIndex)
		c.updateChatHeader()
	})
	if selectedIndex < 0 {
		c.refreshChatView()
	}
	c.refreshDiscoveryRows()
}

func (c *controller) renderPeerList(selectedIndex int) {
	if c.peerList == nil {
		return
	}
	c.peerList.Clear()
	peers := c.listPeersSnapshot()

	onlineCount := 0
	for _, peer := range peers {
		display := c.peerDisplayName(&peer)
		if strings.TrimSpace(display) == "" {
			display = peer.DeviceID
		}
		settings := c.peerSettingsByID(peer.DeviceID)
		isTrusted := c.peerTrustLevel(peer.DeviceID) == storage.PeerTrustLevelTrusted
		isVerified := settings != nil && settings.Verified

		runtimeState := c.runtimeStateForPeer(peer.DeviceID)
		stateText := c.peerStatePresentation(&peer, runtimeState)

		// Track online peers for the header badge.
		if strings.Contains(strings.ToLower(stateText), "online") {
			onlineCount++
		}

		secondaryInfo := ""
		if settings != nil && strings.TrimSpace(settings.CustomName) != "" {
			secondary := strings.TrimSpace(peer.DeviceName)
			if secondary == "" {
				secondary = peer.DeviceID
			}
			secondaryInfo = secondary
		}

		w := createPeerItemWidget(display, stateText, isTrusted, isVerified, secondaryInfo)
		item := widgets.NewQListWidgetItem(c.peerList, 0)
		h := itemHeightForWidget(w, 48)
		item.SetSizeHint(core.NewQSize2(0, h))
		c.peerList.SetItemWidget(item, w)
	}

	// Update count badge.
	if c.peerCountBadge != nil {
		c.peerCountBadge.SetText(fmt.Sprintf("%d", onlineCount))
	}

	if selectedIndex >= 0 && selectedIndex < len(peers) {
		c.peerList.SetCurrentRow(selectedIndex)
	}
}

func (c *controller) peerByIndex(index int) *storage.Peer {
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	if index < 0 || index >= len(c.peers) {
		return nil
	}
	peer := c.peers[index]
	return &peer
}

func (c *controller) selectPeerByIndex(index int) {
	peer := c.peerByIndex(index)
	if peer == nil {
		return
	}
	c.peersMu.Lock()
	c.selectedPeerID = peer.DeviceID
	c.peersMu.Unlock()
	c.updateChatHeader()
	c.refreshChatView()
}

func (c *controller) updateChatHeader() {
	selectedPeerID := c.currentSelectedPeerID()
	peerName := "Select a peer to start chatting"
	fingerprint := ""
	hasPeer := false
	if selectedPeerID != "" {
		if peer := c.peerByID(selectedPeerID); peer != nil {
			hasPeer = true
			peerName = c.peerDisplayName(peer)
			if strings.TrimSpace(peerName) == "" {
				peerName = peer.DeviceName
			}
			if strings.TrimSpace(peerName) == "" {
				peerName = peer.DeviceID
			}
			fingerprint = appcrypto.FormatFingerprint(peer.KeyFingerprint)
		}
	}

	c.enqueueUI(func() {
		if c.chatHeader != nil {
			c.chatHeader.SetText(peerName)
		}
		if c.chatFingerprint != nil {
			c.chatFingerprint.SetText(fingerprint)
			if hasPeer {
				c.chatFingerprint.Show()
			} else {
				c.chatFingerprint.Hide()
			}
		}
		if c.chatComposer != nil {
			if hasPeer {
				c.chatComposer.Show()
			} else {
				c.chatComposer.Hide()
				if c.messageInput != nil {
					c.messageInput.Clear()
				}
			}
		}
		if c.peerSettingsBtn != nil {
			if hasPeer {
				c.peerSettingsBtn.Show()
			} else {
				c.peerSettingsBtn.Hide()
			}
		}
		if c.searchBtn != nil {
			if hasPeer {
				c.searchBtn.Show()
			} else {
				c.searchBtn.Hide()
			}
		}
		if !hasPeer && c.chatSearchRow != nil {
			c.chatSearchVisible = false
			c.chatSearchQuery = ""
			c.chatFilesOnly = false
			if c.chatSearchInput != nil {
				c.chatSearchInput.SetText("")
			}
			if c.chatFilesOnlyChk != nil {
				c.chatFilesOnlyChk.SetChecked(false)
			}
			c.chatSearchRow.Hide()
		}
	})
}

func (c *controller) toggleChatSearch() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		return
	}
	c.chatMu.Lock()
	c.chatSearchVisible = !c.chatSearchVisible
	visible := c.chatSearchVisible
	c.chatMu.Unlock()

	c.enqueueUI(func() {
		if c.chatSearchRow == nil {
			return
		}
		if visible {
			c.chatSearchRow.Show()
			if c.chatSearchInput != nil {
				c.chatSearchInput.SetFocus2()
			}
		} else {
			c.chatSearchRow.Hide()
			if c.chatSearchInput != nil {
				c.chatSearchInput.SetText("")
			}
			if c.chatFilesOnlyChk != nil {
				c.chatFilesOnlyChk.SetChecked(false)
			}
		}
	})
	if !visible {
		c.chatMu.Lock()
		c.chatSearchQuery = ""
		c.chatFilesOnly = false
		c.chatMu.Unlock()
	}
	c.refreshChatView()
}

func (c *controller) currentSelectedPeerID() string {
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	return c.selectedPeerID
}

func (c *controller) sendCurrentMessage() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer before sending a message")
		return
	}
	if c.messageInput == nil {
		return
	}
	content := strings.TrimSpace(c.messageInput.ToPlainText())
	if content == "" {
		return
	}
	c.messageInput.Clear()

	go func() {
		if _, err := c.manager.SendTextMessage(peerID, content); err != nil {
			c.setStatus(fmt.Sprintf("Send message failed: %v", err))
			return
		}
		c.refreshChatForPeer(peerID)
	}()
}

func (c *controller) attachFileToCurrentPeer() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer before attaching files")
		return
	}
	paths := widgets.QFileDialog_GetOpenFileNames(c.window, "Select Files", c.currentDownloadDirectory(), "All files (*)", "", 0)
	if len(paths) == 0 {
		return
	}
	go c.queuePathsForPeer(peerID, paths)
}

func (c *controller) attachFolderToCurrentPeer() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer before attaching folders")
		return
	}
	path := widgets.QFileDialog_GetExistingDirectory(c.window, "Select Folder", c.currentDownloadDirectory(), 0)
	if strings.TrimSpace(path) == "" {
		return
	}
	go c.queuePathsForPeer(peerID, []string{path})
}

func (c *controller) queuePathsForPeer(peerID string, paths []string) {
	if strings.TrimSpace(peerID) == "" {
		return
	}
	queuedFiles := 0
	queuedFolders := 0
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			c.setStatus(fmt.Sprintf("Read path failed: %v", err))
			continue
		}

		if info.IsDir() {
			folderID, fileIDs, sendErr := c.manager.SendFolder(peerID, path)
			if sendErr != nil {
				c.setStatus(fmt.Sprintf("Send folder failed: %v", sendErr))
				continue
			}
			queuedFolders++
			for _, fileID := range fileIDs {
				meta, metaErr := c.store.GetFileByID(fileID)
				if metaErr != nil {
					continue
				}
				c.upsertFileTransfer(chatFileEntry{
					FileID:           fileID,
					FolderID:         folderID,
					RelativePath:     meta.RelativePath,
					PeerDeviceID:     peerID,
					Direction:        "send",
					Filename:         meta.Filename,
					Filesize:         meta.Filesize,
					StoredPath:       meta.StoredPath,
					AddedAt:          time.Now().UnixMilli(),
					Status:           "pending",
					TotalBytes:       meta.Filesize,
					BytesTransferred: 0,
					UpdatedAt:        time.Now().UnixMilli(),
				})
				queuedFiles++
			}
			continue
		}

		fileID, sendErr := c.manager.SendFile(peerID, path)
		if sendErr != nil {
			c.setStatus(fmt.Sprintf("Send file failed: %v", sendErr))
			continue
		}
		c.upsertFileTransfer(chatFileEntry{
			FileID:           fileID,
			PeerDeviceID:     peerID,
			Direction:        "send",
			Filename:         filepath.Base(path),
			Filesize:         info.Size(),
			StoredPath:       path,
			AddedAt:          time.Now().UnixMilli(),
			Status:           "pending",
			TotalBytes:       info.Size(),
			BytesTransferred: 0,
			UpdatedAt:        time.Now().UnixMilli(),
		})
		queuedFiles++
	}

	c.refreshChatForPeer(peerID)
	if queuedFolders > 0 {
		c.setStatus(fmt.Sprintf("Queued %d file(s) from %d folder(s)", queuedFiles, queuedFolders))
		return
	}
	c.setStatus(fmt.Sprintf("Queued %d file(s)", queuedFiles))
}

func (c *controller) refreshChatForPeer(peerID string) {
	if peerID == "" {
		return
	}
	if selected := c.currentSelectedPeerID(); selected != peerID {
		return
	}
	c.refreshChatView()
}

func (c *controller) refreshChatView() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.chatMu.Lock()
		c.chatMessages = nil
		c.chatFiles = nil
		c.transcriptRows = nil
		c.chatMu.Unlock()
		c.enqueueUI(c.renderTranscript)
		return
	}

	c.chatMu.RLock()
	query := strings.TrimSpace(c.chatSearchQuery)
	filesOnly := c.chatFilesOnly
	c.chatMu.RUnlock()

	messages := make([]storage.Message, 0)
	if !filesOnly {
		var err error
		if query != "" {
			messages, err = c.store.SearchMessages(peerID, query, 2000, 0)
		} else {
			messages, err = c.store.GetMessages(peerID, 2000, 0)
		}
		if err != nil {
			c.setStatus(fmt.Sprintf("Load messages failed: %v", err))
			return
		}
	}

	var (
		files []storage.FileMetadata
		err   error
	)
	if query != "" {
		files, err = c.store.SearchFilesForPeer(peerID, query, 2000, 0)
	} else {
		files, err = c.store.ListFilesForPeer(peerID, 2000, 0)
	}
	if err != nil {
		c.setStatus(fmt.Sprintf("Load files failed: %v", err))
		return
	}

	mergedFiles := c.mergeChatFilesForPeer(peerID, files)
	rows := buildTranscriptRows(messages, mergedFiles, c.cfg.DeviceID)

	c.chatMu.Lock()
	c.chatMessages = messages
	c.chatFiles = mergedFiles
	c.transcriptRows = rows
	c.chatMu.Unlock()
	c.enqueueUI(c.renderTranscript)
}

func buildTranscriptRows(messages []storage.Message, files []chatFileEntry, localDeviceID string) []transcriptRow {
	type rowWrap struct {
		ts      int64
		isMsg   bool
		message storage.Message
		file    chatFileEntry
	}
	rows := make([]rowWrap, 0, len(messages)+len(files))
	for _, message := range messages {
		rows = append(rows, rowWrap{ts: message.TimestampSent, isMsg: true, message: message})
	}
	for _, file := range files {
		ts := file.AddedAt
		if ts <= 0 {
			ts = time.Now().UnixMilli()
		}
		rows = append(rows, rowWrap{ts: ts, isMsg: false, file: file})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts < rows[j].ts
		}
		if rows[i].isMsg != rows[j].isMsg {
			return rows[i].isMsg
		}
		if !rows[i].isMsg {
			return rows[i].file.FileID < rows[j].file.FileID
		}
		return rows[i].message.MessageID < rows[j].message.MessageID
	})

	out := make([]transcriptRow, 0, len(rows))
	for _, row := range rows {
		if row.isMsg {
			out = append(out, transcriptRow{isMessage: true, message: row.message})
		} else {
			file := row.file
			if file.Direction == "" {
				if file.fileDirection(localDeviceID) == "send" {
					file.Direction = "send"
				} else {
					file.Direction = "receive"
				}
			}
			out = append(out, transcriptRow{isMessage: false, file: file})
		}
	}
	return out
}

func (e chatFileEntry) fileDirection(localDeviceID string) string {
	if strings.TrimSpace(localDeviceID) == "" {
		return e.Direction
	}
	return e.Direction
}

func (c *controller) renderTranscript() {
	if c.chatList == nil {
		return
	}
	c.chatMu.RLock()
	rows := append([]transcriptRow(nil), c.transcriptRows...)
	c.chatMu.RUnlock()

	c.chatList.Clear()
	for _, row := range rows {
		if row.isMessage {
			w := c.buildMessageWidget(row.message)
			item := widgets.NewQListWidgetItem(c.chatList, 0)
			h := itemHeightForWidget(w, 52)
			item.SetSizeHint(core.NewQSize2(0, h))
			c.chatList.SetItemWidget(item, w)
			continue
		}
		w := createFileTransferCardWidget(row.file)
		item := widgets.NewQListWidgetItem(c.chatList, 0)
		h := itemHeightForWidget(w, 64)
		item.SetSizeHint(core.NewQSize2(0, h))
		c.chatList.SetItemWidget(item, w)
	}
	c.onTranscriptSelectionChanged(-1)
	if len(rows) > 0 {
		c.chatList.ScrollToBottom()
	}
}

// buildMessageWidget creates a styled card for one text message.
func (c *controller) buildMessageWidget(message storage.Message) *widgets.QWidget {
	sender := c.senderDisplayName(message)
	isOutbound := message.FromDeviceID == c.cfg.DeviceID
	deliveryMark := ""
	if isOutbound {
		deliveryMark = deliveryStatusMark(message.DeliveryStatus)
	}
	return createMessageCardWidget(
		sender,
		formatTimestamp(message.TimestampSent),
		message.Content,
		deliveryMark,
		isOutbound,
	)
}

// senderDisplayName returns a short display name for the message sender.
func (c *controller) senderDisplayName(msg storage.Message) string {
	if msg.FromDeviceID == c.cfg.DeviceID {
		return "You"
	}
	if peer := c.peerByID(msg.FromDeviceID); peer != nil {
		name := c.peerDisplayName(peer)
		if strings.TrimSpace(name) != "" {
			return name
		}
		if strings.TrimSpace(peer.DeviceName) != "" {
			return peer.DeviceName
		}
	}
	return "Peer"
}

func (c *controller) onTranscriptSelectionChanged(row int) {
	if row < 0 {
		c.selectedFileID = ""
		c.updateTransferActionButtons(chatFileEntry{}, false)
		return
	}
	c.chatMu.RLock()
	if row >= len(c.transcriptRows) {
		c.chatMu.RUnlock()
		c.selectedFileID = ""
		c.updateTransferActionButtons(chatFileEntry{}, false)
		return
	}
	tr := c.transcriptRows[row]
	c.chatMu.RUnlock()
	if tr.isMessage {
		c.selectedFileID = ""
		c.updateTransferActionButtons(chatFileEntry{}, false)
		return
	}
	c.selectedFileID = tr.file.FileID
	c.updateTransferActionButtons(tr.file, true)
}

func (c *controller) onTranscriptActivated(row int) {
	if row < 0 {
		return
	}
	c.chatMu.RLock()
	if row >= len(c.transcriptRows) {
		c.chatMu.RUnlock()
		return
	}
	tr := c.transcriptRows[row]
	c.chatMu.RUnlock()

	if tr.isMessage {
		content := strings.TrimSpace(tr.message.Content)
		if content != "" {
			clipboard := gui.QGuiApplication_Clipboard()
			if clipboard != nil {
				clipboard.SetText(content, gui.QClipboard__Clipboard)
				c.setStatus("Message copied")
			}
		}
		return
	}
	if strings.TrimSpace(tr.file.StoredPath) != "" {
		if err := openContainingFolder(tr.file.StoredPath); err != nil {
			c.setStatus(fmt.Sprintf("Open path failed: %v", err))
		}
	}
}

func (c *controller) updateTransferActionButtons(file chatFileEntry, hasFile bool) {
	if c.cancelBtn == nil || c.retryBtn == nil || c.showPathBtn == nil || c.copyPathBtn == nil {
		return
	}
	if !hasFile {
		c.cancelBtn.SetEnabled(false)
		c.retryBtn.SetEnabled(false)
		c.showPathBtn.SetEnabled(false)
		c.copyPathBtn.SetEnabled(false)
		return
	}
	pendingOrActive := !file.TransferCompleted && (strings.EqualFold(file.Status, "pending") || strings.EqualFold(file.Status, "accepted"))
	canRetry := (strings.EqualFold(file.Status, "failed") || strings.EqualFold(file.Status, "rejected")) && strings.EqualFold(file.Direction, "send")
	hasPath := strings.TrimSpace(file.StoredPath) != ""
	c.cancelBtn.SetEnabled(pendingOrActive)
	c.retryBtn.SetEnabled(canRetry)
	c.showPathBtn.SetEnabled(hasPath)
	c.copyPathBtn.SetEnabled(hasPath)
}

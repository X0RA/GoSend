package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gosend/crypto"
	"gosend/network"
	"gosend/storage"
)

type chatFileEntry struct {
	FileID            string
	PeerDeviceID      string
	Direction         string
	Filename          string
	Filesize          int64
	Filetype          string
	StoredPath        string
	AddedAt           int64
	BytesTransferred  int64
	TotalBytes        int64
	Status            string
	TransferCompleted bool
}

func (c *controller) buildChatPane() fyne.CanvasObject {
	c.chatHeader = widget.NewLabel("Select a peer to start chatting")
	c.chatHeader.TextStyle = fyne.TextStyle{Bold: true}
	c.chatKeyButton = widget.NewButtonWithIcon("", theme.InfoIcon(), c.showSelectedPeerFingerprint)
	c.chatKeyButton.Disable()
	header := container.NewBorder(nil, nil, nil, c.chatKeyButton, c.chatHeader)

	emptyLabel := widget.NewLabel("No messages yet")
	emptyLabel.Alignment = fyne.TextAlignCenter
	emptyLabel.Importance = widget.LowImportance
	c.chatMessagesBox = container.NewVBox(emptyLabel)
	c.chatScroll = container.NewVScroll(c.chatMessagesBox)

	c.messageInput = widget.NewMultiLineEntry()
	c.messageInput.SetPlaceHolder("Type a message...")
	c.messageInput.Wrapping = fyne.TextWrapWord
	c.messageInput.SetMinRowsVisible(2)

	attachBtn := widget.NewButtonWithIcon("", theme.MailAttachmentIcon(), c.attachFileToCurrentPeer)
	sendBtn := widget.NewButton("Send", c.sendCurrentMessage)
	sendBtn.Importance = widget.HighImportance
	controls := container.NewHBox(attachBtn, layout.NewSpacer(), sendBtn)
	inputPane := container.NewBorder(nil, controls, nil, nil, c.messageInput)

	return container.NewBorder(
		container.NewVBox(container.NewPadded(header), widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewPadded(inputPane)),
		nil, nil, c.chatScroll,
	)
}

func (c *controller) updateChatHeader() {
	selectedPeerID := c.currentSelectedPeerID()
	peerName := "Select a peer to start chatting"
	hasPeer := false
	if selectedPeerID != "" {
		if peer := c.peerByID(selectedPeerID); peer != nil {
			hasPeer = true
			peerName = fmt.Sprintf("%s (%s)", peer.DeviceName, peerStatusIndicator(peer.Status))
		}
	}

	fyne.Do(func() {
		if c.chatHeader != nil {
			c.chatHeader.SetText(peerName)
		}
		if c.chatKeyButton != nil {
			if hasPeer {
				c.chatKeyButton.Enable()
			} else {
				c.chatKeyButton.Disable()
			}
		}
	})
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

	content := strings.TrimSpace(c.messageInput.Text)
	if content == "" {
		return
	}
	c.messageInput.SetText("")

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
		c.setStatus("Select a peer before attaching a file")
		return
	}

	go func() {
		path, err := c.fileHandler.PickFile()
		if err != nil {
			if err != errFilePickerCancelled {
				c.setStatus(fmt.Sprintf("Pick file failed: %v", err))
			}
			return
		}

		info, err := os.Stat(path)
		if err != nil {
			c.setStatus(fmt.Sprintf("Read file failed: %v", err))
			return
		}
		if info.IsDir() {
			c.setStatus("Cannot send a directory")
			return
		}

		fileID, err := c.manager.SendFile(peerID, path)
		if err != nil {
			c.setStatus(fmt.Sprintf("Send file failed: %v", err))
			return
		}

		c.upsertFileTransfer(chatFileEntry{
			FileID:       fileID,
			PeerDeviceID: peerID,
			Direction:    "send",
			Filename:     filepath.Base(path),
			Filesize:     info.Size(),
			StoredPath:   path,
			AddedAt:      time.Now().UnixMilli(),
			Status:       "pending",
			TotalBytes:   info.Size(),
		})
		c.refreshChatForPeer(peerID)
		c.setStatus(fmt.Sprintf("File queued: %s", filepath.Base(path)))
	}()
}

func (c *controller) showSelectedPeerFingerprint() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		return
	}
	peer := c.peerByID(peerID)
	if peer == nil {
		return
	}

	fingerprint := crypto.FormatFingerprint(peer.KeyFingerprint)
	message := fmt.Sprintf("%s\n\nDevice ID: %s\nFingerprint: %s", peer.DeviceName, peer.DeviceID, fingerprint)
	dialog.ShowInformation("Peer Fingerprint", message, c.window)
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
		c.chatMu.Unlock()
		c.renderChatTranscript()
		return
	}

	messages, err := c.store.GetMessages(peerID, 1000, 0)
	if err != nil {
		c.setStatus(fmt.Sprintf("Load messages failed: %v", err))
		return
	}

	c.chatMu.Lock()
	c.chatMessages = messages
	c.chatMu.Unlock()
	c.renderChatTranscript()
}

func (c *controller) renderChatTranscript() {
	peerID := c.currentSelectedPeerID()

	c.chatMu.RLock()
	messages := make([]storage.Message, len(c.chatMessages))
	copy(messages, c.chatMessages)
	c.chatMu.RUnlock()

	files := c.fileTransfersForPeer(peerID)
	rows := buildConversationRows(messages, files, c.cfg.DeviceID, c.window)

	fyne.Do(func() {
		if c.chatMessagesBox == nil {
			return
		}
		c.chatMessagesBox.RemoveAll()
		if len(rows) == 0 {
			empty := widget.NewLabel("No messages yet")
			empty.Alignment = fyne.TextAlignCenter
			empty.Importance = widget.LowImportance
			c.chatMessagesBox.Add(empty)
		} else {
			for _, row := range rows {
				c.chatMessagesBox.Add(row)
			}
		}
		c.chatMessagesBox.Refresh()
		if c.chatScroll != nil {
			c.chatScroll.ScrollToBottom()
		}
	})
}

func buildConversationRows(messages []storage.Message, files []chatFileEntry, localDeviceID string, parentWindow fyne.Window) []fyne.CanvasObject {
	type conversationRow struct {
		timestamp int64
		kind      int
		message   *storage.Message
		file      *chatFileEntry
	}

	rows := make([]conversationRow, 0, len(messages)+len(files))
	for i := range messages {
		msg := messages[i]
		rows = append(rows, conversationRow{
			timestamp: msg.TimestampSent,
			kind:      0,
			message:   &msg,
		})
	}
	for i := range files {
		file := files[i]
		timestamp := file.AddedAt
		if timestamp == 0 {
			timestamp = time.Now().UnixMilli()
		}
		rows = append(rows, conversationRow{
			timestamp: timestamp,
			kind:      1,
			file:      &file,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].timestamp == rows[j].timestamp {
			return rows[i].kind < rows[j].kind
		}
		return rows[i].timestamp < rows[j].timestamp
	})

	out := make([]fyne.CanvasObject, 0, len(rows))
	for _, row := range rows {
		if row.message != nil {
			out = append(out, renderMessageRow(*row.message, localDeviceID))
			continue
		}
		if row.file != nil {
			out = append(out, renderFileRow(*row.file, parentWindow))
		}
	}
	return out
}

func renderMessageRow(message storage.Message, localDeviceID string) fyne.CanvasObject {
	outgoing := isOutgoingMessage(message, localDeviceID)
	body := widget.NewLabel(message.Content)
	body.Wrapping = fyne.TextWrapWord

	statusStr := ""
	if outgoing {
		statusStr = " " + deliveryStatusMark(message.DeliveryStatus)
	}
	ts := canvas.NewText(formatTimestamp(message.TimestampSent)+statusStr, colorMuted)
	ts.TextSize = 11
	ts.Alignment = fyne.TextAlignTrailing

	bgColor := colorIncomingMsg
	if outgoing {
		bgColor = colorOutgoingMsg
	}

	content := container.NewVBox(body, ts)
	bubble := newRoundedBg(bgColor, 10, content)

	if outgoing {
		return container.NewGridWithColumns(2, layout.NewSpacer(), bubble)
	}
	return container.NewGridWithColumns(2, bubble, layout.NewSpacer())
}

func isOutgoingMessage(message storage.Message, localDeviceID string) bool {
	return strings.TrimSpace(localDeviceID) != "" && message.FromDeviceID == localDeviceID
}

func renderFileRow(file chatFileEntry, parentWindow fyne.Window) fyne.CanvasObject {
	name := valueOrDefault(file.Filename, file.FileID)
	title := widget.NewLabel("📄 " + name)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateEllipsis

	items := []fyne.CanvasObject{title}

	// Image preview for image files with an available path.
	storedPath := strings.TrimSpace(file.StoredPath)
	if storedPath != "" && isImageFile(file.Filename) {
		if _, err := os.Stat(storedPath); err == nil {
			img := canvas.NewImageFromFile(storedPath)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(200, 150))
			items = append(items, img)
		}
	}

	statusText := fileTransferStatusText(file)
	meta := canvas.NewText(
		fmt.Sprintf("%s · %s · %s", formatTimestamp(file.AddedAt), formatBytes(file.Filesize), statusText),
		colorMuted,
	)
	meta.TextSize = 11
	meta.Alignment = fyne.TextAlignTrailing
	items = append(items, meta)

	if !file.TransferCompleted && file.Status != "failed" && file.Status != "rejected" && file.TotalBytes > 0 {
		progress := widget.NewProgressBar()
		progress.SetValue(float64(file.BytesTransferred) / float64(file.TotalBytes))
		items = append(items, progress)
	}
	if file.TransferCompleted && storedPath != "" {
		showPathBtn := widget.NewButton("Show Path", func() {
			dialog.ShowInformation("File Path", storedPath, parentWindow)
		})
		items = append(items, showPathBtn)
	}

	outgoing := strings.EqualFold(file.Direction, "send")
	bgColor := colorIncomingMsg
	if outgoing {
		bgColor = colorOutgoingMsg
	}
	bubble := newRoundedBg(bgColor, 10, container.NewVBox(items...))

	if outgoing {
		return container.NewGridWithColumns(2, layout.NewSpacer(), bubble)
	}
	return container.NewGridWithColumns(2, bubble, layout.NewSpacer())
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp", ".tiff", ".tif":
		return true
	}
	return false
}

func fileTransferStatusText(file chatFileEntry) string {
	switch strings.ToLower(file.Status) {
	case "rejected":
		return "✗ rejected"
	case "failed":
		return "✗ failed"
	case "complete":
		return "✓✓ complete"
	case "accepted":
		if file.TransferCompleted {
			return "✓✓ complete"
		}
		return "✓ accepted"
	}

	if file.TransferCompleted {
		return "✓✓ complete"
	}
	if file.TotalBytes > 0 {
		return fmt.Sprintf("%.0f%%", float64(file.BytesTransferred)*100.0/float64(file.TotalBytes))
	}
	return "✓ pending"
}

func deliveryStatusMark(status string) string {
	switch strings.ToLower(status) {
	case "delivered":
		return "✓✓"
	case "failed":
		return "✗"
	case "pending":
		return "…"
	default:
		return "✓"
	}
}

func formatTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return time.Now().Format("3:04 PM")
	}
	return time.UnixMilli(timestamp).Format("3:04 PM")
}

func (c *controller) upsertFileTransfer(entry chatFileEntry) {
	if entry.FileID == "" {
		return
	}
	if entry.AddedAt == 0 {
		entry.AddedAt = time.Now().UnixMilli()
	}

	c.chatMu.Lock()
	existing, exists := c.fileTransfers[entry.FileID]
	if exists {
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
		if entry.BytesTransferred == 0 {
			entry.BytesTransferred = existing.BytesTransferred
		}
		if entry.Status == "" {
			entry.Status = existing.Status
		}
		if !entry.TransferCompleted {
			entry.TransferCompleted = existing.TransferCompleted
		}
		if entry.AddedAt == 0 {
			entry.AddedAt = existing.AddedAt
		}
	}
	c.fileTransfers[entry.FileID] = entry
	c.chatMu.Unlock()
}

func (c *controller) fileTransfersForPeer(peerID string) []chatFileEntry {
	if peerID == "" {
		return nil
	}

	c.chatMu.RLock()
	defer c.chatMu.RUnlock()
	out := make([]chatFileEntry, 0)
	for _, entry := range c.fileTransfers {
		if entry.PeerDeviceID == peerID {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt == out[j].AddedAt {
			return out[i].FileID < out[j].FileID
		}
		return out[i].AddedAt < out[j].AddedAt
	})
	return out
}

func (c *controller) handleFileProgress(progress network.FileProgress) {
	entry := chatFileEntry{
		FileID:            progress.FileID,
		PeerDeviceID:      progress.PeerDeviceID,
		Direction:         progress.Direction,
		BytesTransferred:  progress.BytesTransferred,
		TotalBytes:        progress.TotalBytes,
		Status:            "accepted",
		TransferCompleted: progress.TransferCompleted,
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
		entry.Status = meta.TransferStatus
		if meta.TransferStatus == "complete" {
			entry.TransferCompleted = true
		}
	}

	if entry.TransferCompleted {
		entry.Status = "complete"
	}
	if entry.TotalBytes == 0 {
		entry.TotalBytes = entry.Filesize
	}
	if entry.AddedAt == 0 {
		entry.AddedAt = time.Now().UnixMilli()
	}
	c.upsertFileTransfer(entry)

	c.fileHandler.UpdateProgress(TransferProgress{
		FileID:           entry.FileID,
		Filename:         entry.Filename,
		BytesTransferred: entry.BytesTransferred,
		TotalBytes:       entry.TotalBytes,
		Completed:        entry.TransferCompleted,
		Failed:           strings.EqualFold(entry.Status, "failed") || strings.EqualFold(entry.Status, "rejected"),
	})

	if entry.PeerDeviceID != "" {
		c.refreshChatForPeer(entry.PeerDeviceID)
	}
}

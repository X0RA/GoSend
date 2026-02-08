package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gosend/crypto"
	"gosend/network"
	"gosend/storage"
)

type chatFileEntry struct {
	FileID            string
	FolderID          string
	RelativePath      string
	PeerDeviceID      string
	Direction         string
	Filename          string
	Filesize          int64
	Filetype          string
	StoredPath        string
	AddedAt           int64
	BytesTransferred  int64
	TotalBytes        int64
	SpeedBytesPerSec  float64
	ETASeconds        int64
	Status            string
	CompletedAt       int64
	TransferCompleted bool
}

type messageEntry struct {
	widget.Entry
	shiftDown bool
	onSend    func()
}

func newMessageEntry(onSend func()) *messageEntry {
	entry := &messageEntry{
		onSend: onSend,
	}
	entry.MultiLine = true
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *messageEntry) KeyDown(key *fyne.KeyEvent) {
	e.Entry.KeyDown(key)
	if key == nil {
		return
	}
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftDown = true
	}
}

func (e *messageEntry) KeyUp(key *fyne.KeyEvent) {
	e.Entry.KeyUp(key)
	if key == nil {
		return
	}
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftDown = false
	}
}

func (e *messageEntry) TypedKey(key *fyne.KeyEvent) {
	if key == nil {
		return
	}
	if key.Name == fyne.KeyReturn || key.Name == fyne.KeyEnter {
		if e.shiftDown {
			e.Entry.TypedKey(key)
			return
		}
		if e.onSend != nil {
			e.onSend()
		}
		return
	}
	e.Entry.TypedKey(key)
}

func (c *controller) buildChatPane() fyne.CanvasObject {
	c.chatHeader = newClickableLabel("Select a peer to start chatting", c.showSelectedPeerFingerprint)
	c.chatHeader.SetTextStyle(fyne.TextStyle{Bold: true})
	c.chatHeader.SetColor(colorMuted)
	c.peerSettingsBtn = newHintButtonWithIcon("", theme.SettingsIcon(), "Peer settings", c.showSelectedPeerSettingsDialog, c.handleHoverHint)
	c.peerSettingsBtn.Hide()
	header := container.NewPadded(container.NewBorder(nil, nil, nil, c.peerSettingsBtn, c.chatHeader))

	emptyLabel := widget.NewLabel("No messages yet")
	emptyLabel.Alignment = fyne.TextAlignCenter
	emptyLabel.Importance = widget.LowImportance
	c.chatMessagesBox = container.NewVBox(emptyLabel)
	c.chatScroll = container.NewVScroll(c.chatMessagesBox)
	c.chatDropArea = c.chatScroll

	c.messageInput = newMessageEntry(c.sendCurrentMessage)
	c.messageInput.SetPlaceHolder("Type a message...")
	c.messageInput.Wrapping = fyne.TextWrapWord
	c.messageInput.SetMinRowsVisible(2)

	attachBtn := newHintButtonWithIcon("", theme.MailAttachmentIcon(), "Attach file", c.attachFileToCurrentPeer, c.handleHoverHint)
	sendBtn := newHintButton("Send", "Send message", c.sendCurrentMessage, c.handleHoverHint)
	sendBtn.Importance = widget.HighImportance
	controls := container.NewVBox(sendBtn, attachBtn)
	inputPane := container.NewBorder(nil, nil, nil, container.NewPadded(controls), c.messageInput)
	c.chatComposer = container.NewPadded(inputPane)
	c.chatComposer.Hide()

	base := container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), c.chatComposer),
		nil, nil, c.chatScroll,
	)

	dropBg := canvas.NewRectangle(color.NRGBA{R: 76, G: 175, B: 80, A: 36})
	dropBg.StrokeColor = colorOnline
	dropBg.StrokeWidth = 2
	dropText := canvas.NewText("Drop files or folders here", colorOnline)
	dropText.TextStyle = fyne.TextStyle{Bold: true}
	c.chatDropOverlay = container.NewStack(dropBg, container.NewCenter(dropText))
	c.chatDropOverlay.Hide()

	return container.NewStack(base, c.chatDropOverlay)
}

func (c *controller) updateChatHeader() {
	selectedPeerID := c.currentSelectedPeerID()
	peerName := "Select a peer to start chatting"
	hasPeer := false
	statusColor := colorMuted
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
			if strings.EqualFold(peer.Status, "online") {
				statusColor = colorOnline
			}
		}
	}

	fyne.Do(func() {
		if c.chatHeader != nil {
			c.chatHeader.SetText(peerName)
			c.chatHeader.SetColor(statusColor)
		}
		if c.chatComposer != nil {
			if hasPeer {
				c.chatComposer.Show()
			} else {
				c.chatComposer.Hide()
				if c.messageInput != nil {
					c.messageInput.SetText("")
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
		paths, err := c.fileHandler.PickPaths()
		if err != nil {
			if err != errFilePickerCancelled {
				c.setStatus(fmt.Sprintf("Pick file failed: %v", err))
			}
			return
		}
		if len(paths) == 0 {
			return
		}
		c.queuePathsForPeer(peerID, paths)
	}()
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
			if len(fileIDs) == 0 {
				c.setStatus(fmt.Sprintf("Folder accepted with no files to transfer: %s", filepath.Base(path)))
				continue
			}

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

func (c *controller) cancelTransferFromUI(fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" || c.manager == nil {
		return
	}

	fyne.Do(func() {
		dialog.NewConfirm("Cancel Transfer", "Cancel this transfer?", func(confirm bool) {
			if !confirm {
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
					})
					c.refreshChatForPeer(peerID)
				} else {
					c.refreshChatView()
				}
				c.setStatus("Transfer canceled")
			}()
		}, c.window).Show()
	})
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
			})
			c.refreshChatForPeer(peerID)
		} else {
			c.refreshChatView()
		}
		c.setStatus("Transfer re-queued")
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
	displayName := c.peerDisplayName(peer)
	if strings.TrimSpace(displayName) == "" {
		displayName = peer.DeviceName
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = peer.DeviceID
	}
	message := fmt.Sprintf("%s\n\nDevice ID: %s\nFingerprint: %s", displayName, peer.DeviceID, fingerprint)
	settings := c.peerSettingsByID(peer.DeviceID)
	if settings != nil && strings.TrimSpace(settings.CustomName) != "" && settings.CustomName != peer.DeviceName {
		message += fmt.Sprintf("\nDevice Name: %s", valueOrDefault(peer.DeviceName, peer.DeviceID))
	}
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

	fyne.Do(func() {
		if c.chatMessagesBox == nil {
			return
		}
		// Map FileID -> progress bar so we can update the correct bar on progress without rebuilding the list.
		barMap := make(map[string]*widget.ProgressBar)
		registerBar := func(fileID string, bar *widget.ProgressBar) {
			if fileID != "" && bar != nil {
				barMap[fileID] = bar
			}
		}
		rows := buildConversationRows(messages, files, c.cfg.DeviceID, c.window, registerBar, c.cancelTransferFromUI, c.retryTransferFromUI)
		c.fileProgressBarsMu.Lock()
		c.fileProgressBars = barMap
		c.fileProgressBarsMu.Unlock()

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

func buildConversationRows(messages []storage.Message, files []chatFileEntry, localDeviceID string, parentWindow fyne.Window, registerProgressBar onProgressBarCreated, onCancelTransfer func(string), onRetryTransfer func(string)) []fyne.CanvasObject {
	type conversationRow struct {
		timestamp int64
		kind      int
		message   *storage.Message
		file      *chatFileEntry
	}

	rows := make([]conversationRow, 0, len(messages)+len(files))
	msgCopies := make([]storage.Message, len(messages))
	for i := range messages {
		msgCopies[i] = messages[i]
	}
	for i := range messages {
		rows = append(rows, conversationRow{
			timestamp: msgCopies[i].TimestampSent,
			kind:      0,
			message:   &msgCopies[i],
		})
	}
	// Keep distinct copies so each row has its own file entry (avoids loop variable capture when rendering).
	fileCopies := make([]chatFileEntry, len(files))
	for i := range files {
		fileCopies[i] = files[i]
	}
	for i := range files {
		f := &fileCopies[i]
		timestamp := f.AddedAt
		// Keep timestamp stable when 0 so order doesn't jump on each refresh (use 0, tie-break by FileID in sort).
		if timestamp <= 0 {
			timestamp = 0
		}
		rows = append(rows, conversationRow{
			timestamp: timestamp,
			kind:      1,
			file:      f,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].timestamp != rows[j].timestamp {
			return rows[i].timestamp < rows[j].timestamp
		}
		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}
		// Same timestamp and kind: use FileID for file rows so order is stable when two files share AddedAt.
		if rows[i].kind == 1 && rows[i].file != nil && rows[j].file != nil {
			return rows[i].file.FileID < rows[j].file.FileID
		}
		return false
	})

	out := make([]fyne.CanvasObject, 0, len(rows))
	for _, row := range rows {
		if row.message != nil {
			out = append(out, renderMessageRow(*row.message, localDeviceID, parentWindow))
			continue
		}
		if row.file != nil {
			out = append(out, renderFileRow(*row.file, parentWindow, registerProgressBar, onCancelTransfer, onRetryTransfer))
		}
	}
	return out
}

// isTextMessage returns true if the message is plain text (copyable), not a file or image.
func isTextMessage(message storage.Message) bool {
	ct := strings.TrimSpace(strings.ToLower(message.ContentType))
	return ct == "" || ct == "text"
}

func renderMessageRow(message storage.Message, localDeviceID string, parentWindow fyne.Window) fyne.CanvasObject {
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

	// Bottom row: timestamp and optional copy button for text messages
	bottomRow := fyne.CanvasObject(ts)
	if isTextMessage(message) && parentWindow != nil && strings.TrimSpace(message.Content) != "" {
		copyBtn := newHintButtonWithIcon("", theme.ContentCopyIcon(), "Copy to clipboard", func() {
			if parentWindow != nil && parentWindow.Clipboard() != nil {
				parentWindow.Clipboard().SetContent(message.Content)
			}
		}, nil)
		copyBtn.Importance = widget.LowImportance
		bottomRow = container.NewBorder(nil, nil, nil, container.NewPadded(copyBtn), ts)
	}

	content := container.NewVBox(body, bottomRow)
	bubble := newRoundedBg(bgColor, 10, content)

	if outgoing {
		return container.NewGridWithColumns(2, layout.NewSpacer(), bubble)
	}
	return container.NewGridWithColumns(2, bubble, layout.NewSpacer())
}

func isOutgoingMessage(message storage.Message, localDeviceID string) bool {
	return strings.TrimSpace(localDeviceID) != "" && message.FromDeviceID == localDeviceID
}

// onProgressBarCreated is called when a file row has a progress bar so the controller can update it by FileID.
type onProgressBarCreated func(fileID string, bar *widget.ProgressBar)

func renderFileRow(file chatFileEntry, parentWindow fyne.Window, registerProgressBar onProgressBarCreated, onCancelTransfer func(string), onRetryTransfer func(string)) fyne.CanvasObject {
	name := valueOrDefault(file.Filename, file.FileID)
	title := widget.NewLabel("📄 " + name)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateEllipsis

	items := []fyne.CanvasObject{title}
	if rel := strings.TrimSpace(file.RelativePath); rel != "" {
		relLabel := widget.NewLabel(rel)
		relLabel.Importance = widget.LowImportance
		relLabel.Truncation = fyne.TextTruncateEllipsis
		items = append(items, relLabel)
	}

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

	if file.SpeedBytesPerSec > 0 && !file.TransferCompleted && strings.EqualFold(file.Status, "accepted") {
		eta := "ETA --"
		if file.ETASeconds > 0 {
			eta = fmt.Sprintf("ETA %s", (time.Duration(file.ETASeconds) * time.Second).Round(time.Second))
		}
		speed := canvas.NewText(fmt.Sprintf("%s/s · %s", formatBytes(int64(file.SpeedBytesPerSec)), eta), colorMuted)
		speed.TextSize = 11
		speed.Alignment = fyne.TextAlignTrailing
		items = append(items, speed)
	}

	if !file.TransferCompleted && file.Status != "failed" && file.Status != "rejected" && file.TotalBytes > 0 {
		progress := widget.NewProgressBar()
		progress.SetValue(float64(file.BytesTransferred) / float64(file.TotalBytes))
		items = append(items, progress)
		if registerProgressBar != nil && file.FileID != "" {
			registerProgressBar(file.FileID, progress)
		}
	}

	if !file.TransferCompleted && (strings.EqualFold(file.Status, "pending") || strings.EqualFold(file.Status, "accepted")) && onCancelTransfer != nil {
		cancelBtn := widget.NewButton("Cancel", func() {
			onCancelTransfer(file.FileID)
		})
		cancelBtn.Importance = widget.DangerImportance
		items = append(items, cancelBtn)
	}

	if (strings.EqualFold(file.Status, "failed") || strings.EqualFold(file.Status, "rejected")) && strings.EqualFold(file.Direction, "send") && onRetryTransfer != nil {
		retryBtn := widget.NewButton("Retry", func() {
			onRetryTransfer(file.FileID)
		})
		items = append(items, retryBtn)
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
	if strings.EqualFold(file.Status, "pending") {
		return "Waiting..."
	}
	if file.TotalBytes > 0 {
		return fmt.Sprintf("%.0f%%", float64(file.BytesTransferred)*100.0/float64(file.TotalBytes))
	}
	return "Waiting..."
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
		if entry.BytesTransferred == 0 {
			entry.BytesTransferred = existing.BytesTransferred
		}
		if entry.SpeedBytesPerSec == 0 {
			entry.SpeedBytesPerSec = existing.SpeedBytesPerSec
		}
		if entry.ETASeconds == 0 {
			entry.ETASeconds = existing.ETASeconds
		}
		if entry.Status == "" {
			entry.Status = existing.Status
		}
		if !entry.TransferCompleted {
			entry.TransferCompleted = existing.TransferCompleted
		}
		if entry.CompletedAt == 0 {
			entry.CompletedAt = existing.CompletedAt
		}
		if entry.AddedAt == 0 {
			entry.AddedAt = existing.AddedAt
		}
	} else {
		// New entry: set AddedAt once so order in chat stays stable (never overwrite on later progress).
		if entry.AddedAt == 0 {
			entry.AddedAt = time.Now().UnixMilli()
		}
	}
	if entry.TransferCompleted && entry.CompletedAt == 0 {
		entry.CompletedAt = time.Now().UnixMilli()
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
		SpeedBytesPerSec:  progress.SpeedBytesPerSec,
		ETASeconds:        progress.ETASeconds,
		Status:            "accepted",
		TransferCompleted: progress.TransferCompleted,
	}
	if strings.TrimSpace(progress.Status) != "" {
		entry.Status = strings.TrimSpace(progress.Status)
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
		entry.Status = meta.TransferStatus
		if meta.TransferStatus == "complete" {
			entry.TransferCompleted = true
		}
	}

	if entry.TransferCompleted {
		entry.Status = "complete"
		entry.CompletedAt = time.Now().UnixMilli()
	}
	if entry.TotalBytes == 0 {
		entry.TotalBytes = entry.Filesize
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

	if entry.PeerDeviceID == "" {
		return
	}
	if entry.TransferCompleted {
		// Full refresh so we replace progress bar with "Show Path" and update status text.
		c.refreshChatForPeer(entry.PeerDeviceID)
		return
	}
	// Update only this file's progress bar in place so concurrent transfers don't swap or glitch.
	c.fileProgressBarsMu.Lock()
	bar := c.fileProgressBars[progress.FileID]
	c.fileProgressBarsMu.Unlock()
	if bar != nil && entry.TotalBytes > 0 {
		value := float64(entry.BytesTransferred) / float64(entry.TotalBytes)
		fyne.Do(func() {
			bar.SetValue(value)
			bar.Refresh()
		})
	}
}

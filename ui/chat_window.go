package ui

import (
	"fmt"
	"image/color"
	"net/url"
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
	c.chatHeader.SetTextStyle(fyne.TextStyle{Bold: false}) // mockup: peer name medium weight
	c.chatHeader.SetColor(ctpSubtext0)
	c.chatHeaderPeerIDText = canvas.NewText("", ctpOverlay1)
	c.chatHeaderPeerIDText.TextSize = 10
	c.peerSettingsBtn = newFlatButtonWithIcon(iconSettings(), "Peer settings", c.showSelectedPeerSettingsDialog, c.handleHoverHint)
	c.peerSettingsBtn.Hide()
	c.searchBtn = newFlatButtonWithIcon(iconSearch(), "Search chat", c.toggleChatSearch, c.handleHoverHint)
	c.searchBtn.Hide()
	rightControls := container.NewHBox(c.searchBtn, c.peerSettingsBtn)
	headerCenter := container.NewHBox(c.chatHeader, c.chatHeaderPeerIDText)
	headerBg := canvas.NewRectangle(ctpMantle)
	headerBg.SetMinSize(fyne.NewSize(1, 38))
	headerInner := container.New(layout.NewCustomPaddedLayout(1, 1, 8, 8), container.NewBorder(nil, nil, nil, rightControls, headerCenter))
	header := container.NewStack(headerBg, headerInner)

	c.chatSearchEntry = widget.NewEntry()
	c.chatSearchEntry.SetPlaceHolder("Search messages and files")
	c.chatSearchEntry.OnChanged = func(value string) {
		c.chatMu.Lock()
		c.chatSearchQuery = strings.TrimSpace(value)
		c.chatMu.Unlock()
		c.refreshChatView()
	}
	filesOnlyCheck := widget.NewCheck("Files only", func(checked bool) {
		c.chatMu.Lock()
		c.chatFilesOnly = checked
		c.chatMu.Unlock()
		c.refreshChatView()
	})
	clearSearchBtn := widget.NewButtonWithIcon("", iconCancel(), func() {
		filesOnlyCheck.SetChecked(false)
		c.chatSearchEntry.SetText("")
	})
	searchInner := container.NewPadded(container.NewBorder(nil, nil, nil, clearSearchBtn, container.NewHBox(c.chatSearchEntry, filesOnlyCheck)))
	searchBg := canvas.NewRectangle(ctpSurface0)
	searchBg.SetMinSize(fyne.NewSize(1, 36))
	c.chatSearchBar = container.NewStack(searchBg, searchInner)
	c.chatSearchBar.Hide()

	emptyLabel := widget.NewLabel("No messages yet")
	emptyLabel.Alignment = fyne.TextAlignCenter
	emptyLabel.Importance = widget.LowImportance
	c.chatMessagesBox = container.NewVBox(emptyLabel)
	c.chatScroll = container.NewVScroll(c.chatMessagesBox)
	c.chatDropArea = c.chatScroll
	// Mockup: chat transcript area uses base background (#1e1e2e)
	chatContentBg := canvas.NewRectangle(ctpBase)
	chatContentBg.SetMinSize(fyne.NewSize(1, 1))
	scrollWithBg := container.NewStack(chatContentBg, c.chatScroll)

	c.messageInput = newMessageEntry(c.sendCurrentMessage)
	c.messageInput.SetPlaceHolder("Type a message...")
	c.messageInput.Wrapping = fyne.TextWrapWord
	c.messageInput.SetMinRowsVisible(2)

	// Mockup: attach file + attach folder on left, input center, send on right
	attachFileBtn := newFlatButtonWithIcon(iconAttachFile(), "Attach file", c.attachFileToCurrentPeer, c.handleHoverHint)
	attachFolderBtn := newFlatButtonWithIcon(iconFolderOpen(), "Attach folder", c.attachFolderToCurrentPeer, c.handleHoverHint)
	sendBtn := newFlatButtonWithIcon(iconSendPrimary(), "Send message", c.sendCurrentMessage, c.handleHoverHint)
	leftControls := container.NewHBox(attachFileBtn, attachFolderBtn)
	inputPane := container.NewBorder(nil, nil, leftControls, sendBtn, c.messageInput)
	composerInner := container.NewPadded(inputPane)
	composerBg := canvas.NewRectangle(ctpMantle)
	composerBg.SetMinSize(fyne.NewSize(1, 44))
	c.chatComposer = container.NewStack(composerBg, composerInner)
	c.chatComposer.Hide()

	topArea := withBottomDivider(newVNoGap(header, c.chatSearchBar))
	bottomArea := withTopDivider(c.chatComposer)

	base := newNoGapBorder(topArea, bottomArea, scrollWithBg)

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
	peerIDShort := ""
	hasPeer := false
	statusColor := ctpSubtext0
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
				statusColor = ctpGreen
			} else {
				statusColor = ctpText
			}
			if len(selectedPeerID) > 12 {
				peerIDShort = selectedPeerID[:6] + "..." + selectedPeerID[len(selectedPeerID)-4:]
			} else {
				peerIDShort = selectedPeerID
			}
		}
	}

	fyne.Do(func() {
		if c.chatHeader != nil {
			c.chatHeader.SetText(peerName)
			c.chatHeader.SetColor(statusColor)
		}
		if c.chatHeaderPeerIDText != nil {
			if strings.TrimSpace(peerIDShort) == "" {
				c.chatHeaderPeerIDText.Text = ""
			} else {
				c.chatHeaderPeerIDText.Text = " - " + peerIDShort
			}
			c.chatHeaderPeerIDText.Refresh()
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
		if c.searchBtn != nil {
			if hasPeer {
				c.searchBtn.Show()
			} else {
				c.searchBtn.Hide()
			}
		}
		if !hasPeer && c.chatSearchBar != nil {
			c.chatMu.Lock()
			c.chatSearchVisible = false
			c.chatSearchQuery = ""
			c.chatFilesOnly = false
			c.chatMu.Unlock()
			if c.chatSearchEntry != nil {
				c.chatSearchEntry.SetText("")
			}
			c.chatSearchBar.Hide()
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

func (c *controller) attachFolderToCurrentPeer() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer before attaching a folder")
		return
	}

	go func() {
		path, err := c.pickFolderPath()
		if err != nil {
			if err != errFilePickerCancelled {
				c.setStatus(fmt.Sprintf("Pick folder failed: %v", err))
			}
			return
		}
		if path == "" {
			return
		}
		c.queuePathsForPeer(peerID, []string{path})
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
						Status:            "canceled",
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
			addedAt := int64(0)
			if meta.TimestampReceived != nil {
				addedAt = *meta.TimestampReceived
			}
			c.upsertFileTransfer(chatFileEntry{
				FileID:            fileID,
				PeerDeviceID:      peerID,
				Filename:          meta.Filename,
				Filesize:          meta.Filesize,
				StoredPath:        meta.StoredPath,
				AddedAt:           addedAt,
				Status:            "pending",
				TransferCompleted: false,
				BytesTransferred:  0,
				CompletedAt:       0,
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

func (c *controller) toggleChatSearch() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		return
	}

	c.chatMu.Lock()
	c.chatSearchVisible = !c.chatSearchVisible
	visible := c.chatSearchVisible
	c.chatMu.Unlock()

	fyne.Do(func() {
		if c.chatSearchBar == nil {
			return
		}
		if visible {
			c.chatSearchBar.Show()
			if c.chatSearchEntry != nil && c.window != nil && c.window.Canvas() != nil {
				c.window.Canvas().Focus(c.chatSearchEntry)
			}
		} else {
			c.chatSearchBar.Hide()
			if c.chatSearchEntry != nil {
				c.chatSearchEntry.SetText("")
			}
		}
		c.chatSearchBar.Refresh()
	})
	if !visible {
		c.chatMu.Lock()
		c.chatSearchQuery = ""
		c.chatFilesOnly = false
		c.chatMu.Unlock()
	}
	c.refreshChatView()
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
		c.chatMu.Unlock()
		c.renderChatTranscript()
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

	c.chatMu.Lock()
	c.chatMessages = messages
	c.chatFiles = mergedFiles
	c.chatMu.Unlock()
	c.renderChatTranscript()
}

func (c *controller) renderChatTranscript() {
	c.chatMu.RLock()
	messages := make([]storage.Message, len(c.chatMessages))
	copy(messages, c.chatMessages)
	files := make([]chatFileEntry, len(c.chatFiles))
	copy(files, c.chatFiles)
	c.chatMu.RUnlock()

	peerDisplayName := ""
	if peerID := c.currentSelectedPeerID(); peerID != "" {
		if peer := c.peerByID(peerID); peer != nil {
			peerDisplayName = c.peerDisplayName(peer)
			if strings.TrimSpace(peerDisplayName) == "" {
				peerDisplayName = peer.DeviceName
			}
			if strings.TrimSpace(peerDisplayName) == "" {
				peerDisplayName = peer.DeviceID
			}
		}
	}

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
		rows := buildConversationRows(messages, files, c.cfg.DeviceID, peerDisplayName, c.window, registerBar, c.cancelTransferFromUI, c.retryTransferFromUI)
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

func buildConversationRows(messages []storage.Message, files []chatFileEntry, localDeviceID string, peerDisplayName string, parentWindow fyne.Window, registerProgressBar onProgressBarCreated, onCancelTransfer func(string), onRetryTransfer func(string)) []fyne.CanvasObject {
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
			out = append(out, renderMessageRow(*row.message, localDeviceID, peerDisplayName, parentWindow))
			continue
		}
		if row.file != nil {
			out = append(out, renderFileRow(*row.file, localDeviceID, parentWindow, registerProgressBar, onCancelTransfer, onRetryTransfer))
		}
	}
	return out
}

// isTextMessage returns true if the message is plain text (copyable), not a file or image.
func isTextMessage(message storage.Message) bool {
	ct := strings.TrimSpace(strings.ToLower(message.ContentType))
	return ct == "" || ct == "text"
}

func renderMessageRow(message storage.Message, localDeviceID string, peerDisplayName string, parentWindow fyne.Window) fyne.CanvasObject {
	outgoing := isOutgoingMessage(message, localDeviceID)
	senderColor := ctpMauve
	if outgoing {
		senderColor = ctpBlue
	}
	senderLabel := "Peer"
	if outgoing {
		senderLabel = "You"
	}
	if !outgoing && strings.TrimSpace(peerDisplayName) != "" {
		senderLabel = peerDisplayName
	}
	senderText := canvas.NewText(senderLabel, senderColor)
	senderText.TextSize = 12
	senderText.TextStyle = fyne.TextStyle{Bold: true}

	statusStr := ""
	if outgoing {
		statusStr = " " + deliveryStatusMark(message.DeliveryStatus)
	}
	ts := canvas.NewText(formatTimestamp(message.TimestampSent)+statusStr, ctpOverlay1)
	ts.TextSize = 11
	ts.Alignment = fyne.TextAlignTrailing

	topRow := container.NewBorder(nil, nil, senderText, ts)

	body := widget.NewLabel(message.Content)
	body.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(topRow, body)
	if isTextMessage(message) && parentWindow != nil && strings.TrimSpace(message.Content) != "" {
		copyBtn := newHintButtonWithIcon("", iconContentCopy(), "Copy to clipboard", func() {
			if parentWindow != nil && parentWindow.Clipboard() != nil {
				parentWindow.Clipboard().SetContent(message.Content)
			}
		}, nil)
		copyBtn.Importance = widget.LowImportance
		content.Add(container.NewPadded(copyBtn))
	}
	// Discord-like: all messages left-aligned; subtle background tint for sent vs received
	msgBg := ctpSurface1
	if outgoing {
		msgBg = ctpSurface0
	}
	bubble := newRoundedBg(msgBg, 4, content)
	return bubble
}

func isOutgoingMessage(message storage.Message, localDeviceID string) bool {
	return strings.TrimSpace(localDeviceID) != "" && message.FromDeviceID == localDeviceID
}

// onProgressBarCreated is called when a file row has a progress bar so the controller can update it by FileID.
type onProgressBarCreated func(fileID string, bar *widget.ProgressBar)

func renderFileRow(file chatFileEntry, localDeviceID string, parentWindow fyne.Window, registerProgressBar onProgressBarCreated, onCancelTransfer func(string), onRetryTransfer func(string)) fyne.CanvasObject {
	_ = localDeviceID
	name := valueOrDefault(file.Filename, file.FileID)
	storedPath := strings.TrimSpace(file.StoredPath)
	outgoing := strings.EqualFold(file.Direction, "send")

	directionColor := ctpTeal
	if outgoing {
		directionColor = ctpPeach
	}
	directionLabel := "[Receive File]"
	if outgoing {
		directionLabel = "[Send File]"
	}
	dirText := canvas.NewText(directionLabel, directionColor)
	dirText.TextSize = 11
	dirText.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel := widget.NewLabel(name)
	nameLabel.Truncation = fyne.TextTruncateEllipsis
	titleRow := container.NewHBox(dirText, nameLabel)

	var title fyne.CanvasObject
	if file.TransferCompleted && storedPath != "" {
		link := widget.NewHyperlink(name, nil)
		link.OnTapped = func() {
			if err := openContainingFolder(storedPath); err != nil && parentWindow != nil {
				dialog.ShowError(err, parentWindow)
			}
		}
		link.Truncation = fyne.TextTruncateEllipsis
		title = container.NewHBox(dirText, link)
	} else {
		title = titleRow
	}

	items := []fyne.CanvasObject{title}
	if rel := strings.TrimSpace(file.RelativePath); rel != "" {
		relLabel := widget.NewLabel(rel)
		relLabel.Truncation = fyne.TextTruncateEllipsis
		items = append(items, relLabel)
	}

	// Image preview for image files with an available path.
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
		ctpOverlay1,
	)
	meta.TextSize = 11
	items = append(items, meta)

	if file.SpeedBytesPerSec > 0 && !file.TransferCompleted && strings.EqualFold(file.Status, "accepted") {
		eta := "ETA --"
		if file.ETASeconds > 0 {
			eta = fmt.Sprintf("ETA %s", (time.Duration(file.ETASeconds) * time.Second).Round(time.Second))
		}
		speed := canvas.NewText(fmt.Sprintf("%s/s · %s", formatBytes(int64(file.SpeedBytesPerSec)), eta), ctpOverlay1)
		speed.TextSize = 11
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

	if storedPath != "" {
		pathText := canvas.NewText(storedPath, ctpOverlay0)
		pathText.TextSize = 11
		items = append(items, pathText)
	}

	actionBtns := container.NewHBox()
	if !file.TransferCompleted && (strings.EqualFold(file.Status, "pending") || strings.EqualFold(file.Status, "accepted")) && onCancelTransfer != nil {
		cancelBtn := widget.NewButton("Cancel", func() { onCancelTransfer(file.FileID) })
		cancelBtn.Importance = widget.DangerImportance
		actionBtns.Add(cancelBtn)
	}
	if (strings.EqualFold(file.Status, "failed") || strings.EqualFold(file.Status, "rejected")) && strings.EqualFold(file.Direction, "send") && onRetryTransfer != nil {
		retryBtn := widget.NewButton("Retry", func() { onRetryTransfer(file.FileID) })
		actionBtns.Add(retryBtn)
	}
	if file.TransferCompleted && storedPath != "" && strings.EqualFold(file.Status, "complete") {
		showPathBtn := widget.NewButton("Show Path", func() {
			dialog.ShowInformation("File Path", storedPath, parentWindow)
		})
		actionBtns.Add(showPathBtn)
	}
	if storedPath != "" && parentWindow != nil && parentWindow.Clipboard() != nil {
		copyPathBtn := widget.NewButton("Copy Path", func() {
			parentWindow.Clipboard().SetContent(storedPath)
		})
		actionBtns.Add(copyPathBtn)
	}
	if len(actionBtns.Objects) > 0 {
		items = append(items, actionBtns)
	}

	// Discord-like: all file rows left-aligned; subtle background tint for sent vs received
	fileBg := ctpSurface1
	if outgoing {
		fileBg = ctpSurface0
	}
	bubble := newRoundedBg(fileBg, 4, container.NewVBox(items...))
	return bubble
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
		return "Failed"
	case "failed":
		return "Failed"
	case "canceled":
		return "Canceled"
	case "complete":
		return "Complete"
	case "accepted":
		if file.TransferCompleted {
			return "Complete"
		}
		if strings.EqualFold(file.Direction, "send") {
			return "Sending"
		}
		return "Receiving"
	}

	if file.TransferCompleted {
		return "Complete"
	}
	if strings.EqualFold(file.Status, "pending") {
		return "Waiting"
	}
	if file.TotalBytes > 0 {
		if strings.EqualFold(file.Direction, "send") {
			return fmt.Sprintf("Sending (%.0f%%)", float64(file.BytesTransferred)*100.0/float64(file.TotalBytes))
		}
		return fmt.Sprintf("Receiving (%.0f%%)", float64(file.BytesTransferred)*100.0/float64(file.TotalBytes))
	}
	return "Waiting"
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

func openContainingFolder(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("file path is required")
	}

	target := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		target = filepath.Dir(path)
	}
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(target)}

	app := fyne.CurrentApp()
	if app == nil {
		return fmt.Errorf("application context is unavailable")
	}
	return app.OpenURL(u)
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
		terminalStatus := strings.EqualFold(entry.Status, "rejected") || strings.EqualFold(entry.Status, "failed") || strings.EqualFold(entry.Status, "complete") || strings.EqualFold(entry.Status, "canceled")
		if terminalStatus {
			entry.TransferCompleted = true
		} else if strings.EqualFold(entry.Status, "pending") {
			entry.TransferCompleted = false
		} else if !entry.TransferCompleted {
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
		terminal := strings.EqualFold(liveEntry.Status, "complete") ||
			strings.EqualFold(liveEntry.Status, "failed") ||
			strings.EqualFold(liveEntry.Status, "rejected")
		if terminal && liveEntry.TransferCompleted {
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

	terminal := strings.EqualFold(meta.TransferStatus, "complete") ||
		strings.EqualFold(meta.TransferStatus, "failed") ||
		strings.EqualFold(meta.TransferStatus, "rejected")
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
	baseTerminal := strings.EqualFold(merged.Status, "rejected") || strings.EqualFold(merged.Status, "failed") || strings.EqualFold(merged.Status, "complete") || strings.EqualFold(merged.Status, "canceled")
	if strings.TrimSpace(live.Status) != "" && !baseTerminal {
		merged.Status = live.Status
	}
	if strings.EqualFold(live.Status, "pending") {
		merged.TransferCompleted = false
		merged.BytesTransferred = live.BytesTransferred
	}
	if live.TransferCompleted {
		merged.TransferCompleted = true
	}
	if baseTerminal {
		merged.TransferCompleted = true
	}
	if merged.CompletedAt == 0 {
		merged.CompletedAt = live.CompletedAt
	}
	if merged.AddedAt == 0 {
		merged.AddedAt = live.AddedAt
	}
	return merged
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
		metaTerminal := strings.EqualFold(meta.TransferStatus, "rejected") || strings.EqualFold(meta.TransferStatus, "failed") || strings.EqualFold(meta.TransferStatus, "complete")
		isRetryReset := strings.EqualFold(entry.Status, "pending") && !entry.TransferCompleted
		if !isRetryReset && (metaTerminal || !(strings.EqualFold(meta.TransferStatus, "pending") && entry.BytesTransferred > 0)) {
			entry.Status = meta.TransferStatus
		}
		if !isRetryReset && (meta.TransferStatus == "complete" || meta.TransferStatus == "rejected" || meta.TransferStatus == "failed") {
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
	fyne.Do(func() {
		if c.peerList != nil {
			c.peerList.Refresh()
		}
	})
}

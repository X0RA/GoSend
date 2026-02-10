package ui

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"

	"gosend/config"
	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func (c *controller) buildMainWindow() {
	central := widgets.NewQWidget(nil, 0)
	mainLayout := widgets.NewQVBoxLayout()
	mainLayout.SetContentsMargins(0, 0, 0, 0)
	mainLayout.SetSpacing(0)

	// ── Toolbar ──────────────────────────────────────────────
	toolbar := widgets.NewQWidget(nil, 0)
	toolbar.SetObjectName("toolbar")
	toolbarLayout := widgets.NewQHBoxLayout()
	toolbarLayout.SetContentsMargins(12, 6, 12, 6)
	toolbarLayout.SetSpacing(8)

	title := widgets.NewQLabel2("GoSend", nil, 0)
	title.SetObjectName("toolbarTitle")

	queueBtn := widgets.NewQPushButton2("⇄  Transfer Queue", nil)
	queueBtn.SetObjectName("toolbarBtn")
	queueBtn.ConnectClicked(func(bool) { c.showTransferQueuePanel() })
	refreshBtn := widgets.NewQPushButton2("↻  Refresh Discovery", nil)
	refreshBtn.SetObjectName("toolbarBtn")
	refreshBtn.ConnectClicked(func(bool) { go c.refreshDiscovery() })
	discoverBtn := widgets.NewQPushButton2("⌕  Discover", nil)
	discoverBtn.SetObjectName("toolbarBtn")
	discoverBtn.ConnectClicked(func(bool) { c.showDiscoveryDialog() })
	settingsBtn := widgets.NewQPushButton2("⚙  Settings", nil)
	settingsBtn.SetObjectName("toolbarBtn")
	settingsBtn.ConnectClicked(func(bool) { c.showSettingsDialog() })
	c.hold(queueBtn, refreshBtn, discoverBtn, settingsBtn)

	toolbarLayout.AddWidget(title, 0, 0)
	toolbarLayout.AddStretch(1)
	toolbarLayout.AddWidget(queueBtn, 0, 0)
	toolbarLayout.AddWidget(refreshBtn, 0, 0)
	toolbarLayout.AddWidget(discoverBtn, 0, 0)
	toolbarLayout.AddWidget(settingsBtn, 0, 0)
	toolbar.SetLayout(toolbarLayout)

	// ── Content splitter ─────────────────────────────────────
	splitter := widgets.NewQSplitter(nil)
	splitter.SetOrientation(core.Qt__Horizontal)
	splitter.AddWidget(c.buildPeersPane())
	splitter.AddWidget(c.buildChatPane())
	splitter.SetStretchFactor(0, 1)
	splitter.SetStretchFactor(1, 3)
	splitter.SetSizes([]int{200, 900})

	mainLayout.AddWidget(toolbar, 0, 0)
	mainLayout.AddWidget(splitter, 1, 0)
	central.SetLayout(mainLayout)
	c.window.SetCentralWidget(central)

	// ── Status bar ───────────────────────────────────────────
	c.statusBar = widgets.NewQStatusBar(c.window)
	c.statusLabel = widgets.NewQLabel2("Starting...", nil, 0)
	logsBtn := widgets.NewQPushButton2("⊟  Logs", nil)
	logsBtn.ConnectClicked(func(bool) { c.showLogsDialog() })
	c.hold(logsBtn)
	c.statusBar.AddWidget(c.statusLabel, 1)
	c.statusBar.AddPermanentWidget(logsBtn, 0)
	c.window.SetStatusBar(c.statusBar)
}

func (c *controller) initTrayIcon() {
	tray := widgets.NewQSystemTrayIcon(c.window)
	if tray == nil || !tray.IsSystemTrayAvailable() {
		return
	}
	icon := gui.QIcon_FromTheme("applications-internet")
	if icon == nil || icon.IsNull() {
		icon = gui.QIcon_FromTheme("network-workgroup")
	}
	if icon == nil || icon.IsNull() {
		return
	}
	tray.SetIcon(icon)
	tray.SetToolTip("GoSend")
	tray.Show()
	c.trayIcon = tray
	c.hold(icon, tray)
}

func (c *controller) buildPeersPane() *widgets.QWidget {
	pane := widgets.NewQWidget(nil, 0)
	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(0, 4, 0, 0)
	layout.SetSpacing(4)

	// Header row: "PEERS" label + count badge.
	headerRow := widgets.NewQWidget(nil, 0)
	headerLayout := widgets.NewQHBoxLayout()
	headerLayout.SetContentsMargins(10, 4, 10, 0)
	headerLayout.SetSpacing(6)

	header := widgets.NewQLabel2("PEERS", nil, 0)
	header.SetObjectName("peersHeaderLabel")
	c.peerCountBadge = newCountBadge(0)
	headerLayout.AddWidget(header, 0, 0)
	headerLayout.AddWidget(c.peerCountBadge, 0, 0)
	headerLayout.AddStretch(1)
	headerRow.SetLayout(headerLayout)

	c.peerList = widgets.NewQListWidget(nil)
	c.peerList.SetObjectName("peerList")
	c.peerList.ConnectCurrentRowChanged(func(row int) { c.selectPeerByIndex(row) })

	layout.AddWidget(headerRow, 0, 0)
	layout.AddWidget(c.peerList, 1, 0)
	pane.SetLayout(layout)
	return pane
}

func (c *controller) buildChatPane() *widgets.QWidget {
	pane := widgets.NewQWidget(nil, 0)
	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(4, 4, 4, 4)
	layout.SetSpacing(4)

	// ── Chat header ──────────────────────────────────────────
	headerRow := widgets.NewQWidget(nil, 0)
	headerLayout := widgets.NewQHBoxLayout()
	headerLayout.SetContentsMargins(8, 4, 4, 4)
	headerLayout.SetSpacing(8)

	c.chatHeader = widgets.NewQLabel2("Select a peer to start chatting", nil, 0)
	c.chatHeader.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 14px; font-weight: bold; background: transparent;", colorText))
	c.chatFingerprint = widgets.NewQLabel2("", nil, 0)
	c.chatFingerprint.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 12px; background: transparent;", colorSubtext0))

	c.searchBtn = newIconButton("⌕", "Search messages")
	c.searchBtn.ConnectClicked(func(bool) { c.toggleChatSearch() })
	c.searchBtn.Hide()
	c.peerSettingsBtn = newIconButton("⚙", "Peer settings")
	c.peerSettingsBtn.ConnectClicked(func(bool) { c.showSelectedPeerSettingsDialog() })
	c.peerSettingsBtn.Hide()

	headerLayout.AddWidget(c.chatHeader, 0, 0)
	headerLayout.AddWidget(c.chatFingerprint, 0, 0)
	headerLayout.AddStretch(1)
	headerLayout.AddWidget(c.searchBtn, 0, 0)
	headerLayout.AddWidget(c.peerSettingsBtn, 0, 0)
	headerRow.SetLayout(headerLayout)

	// ── Search row ───────────────────────────────────────────
	c.chatSearchRow = widgets.NewQWidget(nil, 0)
	searchLayout := widgets.NewQHBoxLayout()
	searchLayout.SetContentsMargins(0, 0, 0, 0)
	searchLayout.SetSpacing(6)
	c.chatSearchInput = widgets.NewQLineEdit(nil)
	c.chatSearchInput.SetPlaceholderText("Search messages and files")
	c.chatSearchInput.ConnectTextChanged(func(text string) {
		c.chatMu.Lock()
		c.chatSearchQuery = strings.TrimSpace(text)
		c.chatMu.Unlock()
		c.refreshChatView()
	})
	c.chatFilesOnlyChk = widgets.NewQCheckBox2("Files only", nil)
	c.chatFilesOnlyChk.ConnectToggled(func(checked bool) {
		c.chatMu.Lock()
		c.chatFilesOnly = checked
		c.chatMu.Unlock()
		c.refreshChatView()
	})
	clearSearchBtn := widgets.NewQPushButton2("Clear", nil)
	clearSearchBtn.ConnectClicked(func(bool) {
		c.chatFilesOnlyChk.SetChecked(false)
		c.chatSearchInput.SetText("")
	})
	c.hold(clearSearchBtn)
	searchLayout.AddWidget(c.chatSearchInput, 1, 0)
	searchLayout.AddWidget(c.chatFilesOnlyChk, 0, 0)
	searchLayout.AddWidget(clearSearchBtn, 0, 0)
	c.chatSearchRow.SetLayout(searchLayout)
	c.chatSearchRow.Hide()

	// ── Chat list ────────────────────────────────────────────
	c.chatList = widgets.NewQListWidget(nil)
	c.chatList.SetObjectName("chatList")
	c.chatList.ConnectCurrentRowChanged(func(row int) {
		c.onTranscriptSelectionChanged(row)
	})
	c.chatList.ConnectItemDoubleClicked(func(item *widgets.QListWidgetItem) {
		if item == nil {
			return
		}
		row := c.chatList.Row(item)
		c.onTranscriptActivated(row)
	})

	// ── Transfer action row ──────────────────────────────────
	actionRow := widgets.NewQWidget(nil, 0)
	actionLayout := widgets.NewQHBoxLayout()
	actionLayout.SetContentsMargins(4, 0, 4, 0)
	actionLayout.SetSpacing(4)

	c.cancelBtn = widgets.NewQPushButton2("✕ Cancel", nil)
	c.cancelBtn.SetObjectName("actionBtn")
	c.cancelBtn.ConnectClicked(func(bool) {
		if c.selectedFileID != "" {
			c.cancelTransferFromUI(c.selectedFileID)
		}
	})
	c.retryBtn = widgets.NewQPushButton2("↻ Retry", nil)
	c.retryBtn.SetObjectName("actionBtn")
	c.retryBtn.ConnectClicked(func(bool) {
		if c.selectedFileID != "" {
			c.retryTransferFromUI(c.selectedFileID)
		}
	})
	c.showPathBtn = widgets.NewQPushButton2("↗ Show Path", nil)
	c.showPathBtn.SetObjectName("actionBtn")
	c.showPathBtn.ConnectClicked(func(bool) {
		if entry, ok := c.transferByID(c.selectedFileID); ok && strings.TrimSpace(entry.StoredPath) != "" {
			if err := openContainingFolder(entry.StoredPath); err != nil {
				c.setStatus(fmt.Sprintf("Open path failed: %v", err))
			}
		}
	})
	c.copyPathBtn = widgets.NewQPushButton2("⎘ Copy Path", nil)
	c.copyPathBtn.SetObjectName("actionBtn")
	c.copyPathBtn.ConnectClicked(func(bool) {
		if entry, ok := c.transferByID(c.selectedFileID); ok && strings.TrimSpace(entry.StoredPath) != "" {
			clipboard := gui.QGuiApplication_Clipboard()
			if clipboard != nil {
				clipboard.SetText(entry.StoredPath, gui.QClipboard__Clipboard)
				c.setStatus("Path copied")
			}
		}
	})
	actionLayout.AddWidget(c.cancelBtn, 0, 0)
	actionLayout.AddWidget(c.retryBtn, 0, 0)
	actionLayout.AddWidget(c.showPathBtn, 0, 0)
	actionLayout.AddWidget(c.copyPathBtn, 0, 0)
	actionLayout.AddStretch(1)
	actionRow.SetLayout(actionLayout)

	// ── Composer row: [attach] [folder] [input] [send] ──────
	c.messageInput = widgets.NewQTextEdit(nil)
	c.messageInput.SetPlaceholderText("Type a message...")
	c.messageInput.SetAcceptRichText(false)
	c.messageInput.SetMaximumHeight(80)

	attachBtn := widgets.NewQPushButton2("📎", nil)
	attachBtn.SetObjectName("composerAttachBtn")
	attachBtn.SetToolTip("Attach files")
	attachBtn.SetFixedSize2(36, 36)
	attachBtn.ConnectClicked(func(bool) { c.attachFileToCurrentPeer() })

	attachFolderBtn := widgets.NewQPushButton2("📁", nil)
	attachFolderBtn.SetObjectName("composerAttachBtn")
	attachFolderBtn.SetToolTip("Attach folder")
	attachFolderBtn.SetFixedSize2(36, 36)
	attachFolderBtn.ConnectClicked(func(bool) { c.attachFolderToCurrentPeer() })

	sendBtn := widgets.NewQPushButton2("➤", nil)
	sendBtn.SetObjectName("composerBtn")
	sendBtn.SetToolTip("Send message (Ctrl+Enter)")
	sendBtn.SetFixedSize2(36, 36)
	sendBtn.ConnectClicked(func(bool) { c.sendCurrentMessage() })
	c.hold(sendBtn, attachBtn, attachFolderBtn)

	shortcut := widgets.NewQShortcut2(gui.NewQKeySequence2("Ctrl+Return", gui.QKeySequence__NativeText), c.window, "", "", core.Qt__WindowShortcut)
	shortcut.ConnectActivated(func() { c.sendCurrentMessage() })
	c.hold(shortcut)

	c.chatComposer = widgets.NewQWidget(nil, 0)
	c.chatComposer.SetObjectName("composerRow")
	composerLayout := widgets.NewQHBoxLayout()
	composerLayout.SetContentsMargins(0, 4, 0, 0)
	composerLayout.SetSpacing(6)
	composerLayout.AddWidget(attachBtn, 0, core.Qt__AlignBottom)
	composerLayout.AddWidget(attachFolderBtn, 0, core.Qt__AlignBottom)
	composerLayout.AddWidget(c.messageInput, 1, 0)
	composerLayout.AddWidget(sendBtn, 0, core.Qt__AlignBottom)
	c.chatComposer.SetLayout(composerLayout)
	c.chatComposer.Hide()

	layout.AddWidget(headerRow, 0, 0)
	layout.AddWidget(c.chatSearchRow, 0, 0)
	layout.AddWidget(c.chatList, 1, 0)
	layout.AddWidget(actionRow, 0, 0)
	layout.AddWidget(c.chatComposer, 0, 0)
	pane.SetLayout(layout)

	c.updateTransferActionButtons(chatFileEntry{}, false)
	return pane
}

func (c *controller) startServices() error {
	requestedPort := c.cfg.ListeningPort
	if c.cfg.PortMode == config.PortModeAutomatic || requestedPort <= 0 {
		requestedPort = 0
	}

	manager, err := network.NewPeerManager(network.PeerManagerOptions{
		Identity:          c.identity,
		Store:             c.store,
		ListenAddress:     net.JoinHostPort("0.0.0.0", strconv.Itoa(requestedPort)),
		OnKeyChange:       c.promptKeyChangeDecision,
		OnMessageReceived: c.handleIncomingMessage,
		OnQueueOverflow: func(peerDeviceID string, droppedCount int) {
			c.setStatus(fmt.Sprintf("Queue overflow for %s: dropped %d pending messages", peerDeviceID, droppedCount))
			c.refreshChatForPeer(peerDeviceID)
		},
		OnPeerRuntimeStateChanged: c.handlePeerRuntimeStateChanged,
		FilesDir:                  c.currentDownloadDirectory(),
		MaxReceiveFileSize:        c.currentMaxReceiveFileSize(),
		GetDownloadDirectory:      c.currentDownloadDirectory,
		GetMaxReceiveFileSize:     c.currentMaxReceiveFileSize,
		GetMessageRetentionDays:   c.currentMessageRetentionDays,
		GetCleanupDownloadedFiles: c.cleanupDownloadedFilesEnabled,
		OnFileRequest:             c.promptFileRequestDecision,
		OnFileProgress:            c.handleFileProgress,
	})
	if err != nil {
		return fmt.Errorf("create peer manager: %w", err)
	}
	c.manager = manager

	if err := c.manager.Start(); err != nil {
		c.manager = nil
		return fmt.Errorf("start peer manager: %w", err)
	}

	activePort, err := listeningPortFromAddr(c.manager.Addr())
	if err != nil {
		c.manager.Stop()
		c.manager = nil
		return fmt.Errorf("resolve active listening port: %w", err)
	}
	c.activeListenPort = activePort

	discoveryService, err := discovery.Start(discovery.Config{
		SelfDeviceID:      c.cfg.DeviceID,
		DeviceName:        c.cfg.DeviceName,
		ListeningPort:     activePort,
		Ed25519PublicKey:  c.identity.Ed25519PublicKey,
		KnownPeerProvider: c.discoveryKnownPeers,
	})
	if err != nil {
		c.manager.Stop()
		c.manager = nil
		return fmt.Errorf("start discovery: %w", err)
	}
	c.discovery = discoveryService
	return nil
}

func (c *controller) startLoops() {
	c.loopsWg.Add(4)
	go c.discoveryEventLoop()
	go c.pendingAddRequestLoop()
	go c.managerErrorLoop()
	go c.pollLoop()
}

func (c *controller) discoveryEventLoop() {
	defer c.loopsWg.Done()
	if c.discovery == nil || c.discovery.Scanner == nil {
		return
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-c.discovery.Scanner.Events():
			if !ok {
				return
			}
			c.discoveredMu.Lock()
			switch event.Type {
			case discovery.EventPeerUpserted:
				c.discovered[event.Peer.DeviceID] = event.Peer
			case discovery.EventPeerRemoved:
				delete(c.discovered, event.Peer.DeviceID)
			}
			c.discoveredMu.Unlock()
			c.refreshDiscoveryRows()
			if event.Type == discovery.EventPeerUpserted {
				c.tryReconnectDiscoveredPeer(event.Peer)
			}
		}
	}
}

func (c *controller) pendingAddRequestLoop() {
	defer c.loopsWg.Done()
	if c.manager == nil {
		return
	}

	requests := c.manager.PendingAddRequests()
	for {
		select {
		case <-c.ctx.Done():
			return
		case req, ok := <-requests:
			if !ok {
				return
			}
			accept, err := c.promptIncomingPeerAddRequest(req)
			if err != nil {
				c.setStatus(fmt.Sprintf("Peer add prompt failed: %v", err))
				accept = false
			}
			if err := c.manager.ApprovePeerAdd(req.PeerDeviceID, accept); err != nil {
				c.setStatus(fmt.Sprintf("Approve peer add failed: %v", err))
			}
			c.refreshPeersFromStore()
		}
	}
}

func (c *controller) managerErrorLoop() {
	defer c.loopsWg.Done()
	if c.manager == nil {
		return
	}

	errorsCh := c.manager.Errors()
	for {
		select {
		case <-c.ctx.Done():
			return
		case err, ok := <-errorsCh:
			if !ok {
				return
			}
			if err != nil {
				c.setStatus(err.Error())
			}
		}
	}
}

func (c *controller) pollLoop() {
	defer c.loopsWg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.refreshPeersFromStore()
			c.refreshChatView()
		}
	}
}

func (c *controller) setStatus(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = " "
	}
	c.statusMu.Lock()
	c.statusMessage = message
	c.statusMu.Unlock()
	c.appendRuntimeLog("status", message)
	c.enqueueUI(func() {
		if c.statusLabel != nil {
			c.statusLabel.SetText(message)
		}
		if c.statusBar != nil {
			c.statusBar.ShowMessage(message, 0)
		}
	})
}

func (c *controller) appendRuntimeLog(kind, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	line := fmt.Sprintf("[%s] %s: %s", time.Now().Format(time.RFC3339), kind, message)
	c.logsMu.Lock()
	c.runtimeLogs = append(c.runtimeLogs, line)
	if len(c.runtimeLogs) > 500 {
		c.runtimeLogs = append([]string(nil), c.runtimeLogs[len(c.runtimeLogs)-500:]...)
	}
	c.logsMu.Unlock()
}

func (c *controller) showLogsDialog() {
	events, err := c.store.GetSecurityEvents(storage.SecurityEventFilter{Limit: 300})
	if err != nil {
		c.setStatus(fmt.Sprintf("Load logs failed: %v", err))
		return
	}

	c.logsMu.Lock()
	runtimeLogs := append([]string(nil), c.runtimeLogs...)
	c.logsMu.Unlock()

	builder := strings.Builder{}
	builder.WriteString("Runtime\n")
	builder.WriteString("-------\n")
	if len(runtimeLogs) == 0 {
		builder.WriteString("(no runtime logs)\n")
	} else {
		for _, line := range runtimeLogs {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nSecurity Events\n")
	builder.WriteString("--------------\n")
	if len(events) == 0 {
		builder.WriteString("(no security events)\n")
	} else {
		for _, event := range events {
			peer := ""
			if event.PeerDeviceID != nil && strings.TrimSpace(*event.PeerDeviceID) != "" {
				peer = fmt.Sprintf(" peer=%s", *event.PeerDeviceID)
			}
			builder.WriteString(fmt.Sprintf("[%s] %s %s%s\n", time.UnixMilli(event.Timestamp).Format(time.RFC3339), strings.ToUpper(event.Severity), event.EventType, peer))
			if strings.TrimSpace(event.Details) != "" {
				builder.WriteString("  details: ")
				builder.WriteString(event.Details)
				builder.WriteString("\n")
			}
		}
	}

	_ = c.runOnUI(func() {
		dlg := widgets.NewQDialog(c.window, 0)
		dlg.SetWindowTitle("Application Logs")
		dlg.Resize2(880, 560)
		layout := widgets.NewQVBoxLayout()
		text := widgets.NewQTextEdit(nil)
		text.SetReadOnly(true)
		text.SetPlainText(builder.String())
		closeBtn := widgets.NewQPushButton2("Close", nil)
		closeBtn.ConnectClicked(func(bool) { dlg.Accept() })
		layout.AddWidget(text, 1, 0)
		layout.AddWidget(closeBtn, 0, 0)
		dlg.SetLayout(layout)
		dlg.Exec()
		runtime.KeepAlive(closeBtn)
		runtime.KeepAlive(text)
		runtime.KeepAlive(dlg)
	})
}

package ui

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gosend/config"
	appcrypto "gosend/crypto"
	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

var errFilePickerCancelled = errors.New("file picker canceled")

// RunOptions configures the GUI runtime.
type RunOptions struct {
	Config     *config.DeviceConfig
	ConfigPath string
	DataDir    string
	Store      *storage.Store
	Identity   network.LocalIdentity
}

type controller struct {
	app    fyne.App
	window fyne.Window

	cfg     *config.DeviceConfig
	cfgPath string
	dataDir string
	store   *storage.Store

	identity network.LocalIdentity

	discovery *discovery.Service
	manager   *network.PeerManager

	fileHandler *FileHandler

	ctx        context.Context
	cancel     context.CancelFunc
	shutdownMu sync.Once
	loopsWg    sync.WaitGroup

	peersMu        sync.RWMutex
	peers          []storage.Peer
	peerSettings   map[string]storage.PeerSettings
	selectedPeerID string

	discoveredMu    sync.RWMutex
	discovered      map[string]discovery.DiscoveredPeer
	discoveryRows   []discovery.DiscoveredPeer
	discoveryDialog dialog.Dialog
	discoveryList   *widget.List

	chatMu            sync.RWMutex
	chatMessages      []storage.Message
	chatFiles         []chatFileEntry
	fileTransfers     map[string]chatFileEntry
	chatSearchQuery   string
	chatSearchVisible bool
	chatFilesOnly     bool

	fileProgressBars   map[string]*widget.ProgressBar
	fileProgressBarsMu sync.Mutex

	runtimeMu         sync.RWMutex
	peerRuntimeStates map[string]network.PeerRuntimeState

	appStateMu    sync.RWMutex
	appForeground bool

	peerList             *widget.List
	peersOnlineCountText *canvas.Text
	chatHeader           *clickableLabel
	chatHeaderPeerIDText *canvas.Text
	peerSettingsBtn      *flatButton
	searchBtn            *flatButton
	chatSearchBar        *fyne.Container
	chatSearchEntry      *widget.Entry
	chatMessagesBox      *fyne.Container
	chatScroll           *container.Scroll
	chatDropArea         fyne.CanvasObject
	chatDropOverlay      *fyne.Container
	messageInput         *messageEntry
	chatComposer         fyne.CanvasObject
	statusLabel          *widget.Label
	runtimeLogMu         sync.Mutex
	runtimeLogLines      []string

	filePromptPendingMu sync.Mutex
	filePromptPending   map[string]struct{}

	refreshMu      sync.Mutex
	refreshRunning bool
	statusMu       sync.RWMutex
	statusMessage  string
	hoverHint      string
	tooltipMu      sync.Mutex
	tooltipTimer   *time.Timer
	tooltipOverlay fyne.CanvasObject
	tooltipCanvas  fyne.Canvas
	tooltipTarget  fyne.CanvasObject
	tooltipText    string

	activeListenPort int
}

// Run starts the Phase 9 GUI.
func Run(options RunOptions) error {
	if err := options.validate(); err != nil {
		return err
	}

	ui := fyneapp.NewWithID("gosend")
	ui.Settings().SetTheme(newGoSendTheme())
	ctrl, err := newController(ui, options)
	if err != nil {
		return err
	}
	return ctrl.run()
}

func (o RunOptions) validate() error {
	if o.Config == nil {
		return errors.New("config is required")
	}
	if o.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if o.DataDir == "" {
		return errors.New("data dir is required")
	}
	if o.Store == nil {
		return errors.New("store is required")
	}
	if o.Identity.DeviceID == "" || o.Identity.DeviceName == "" {
		return errors.New("identity device fields are required")
	}
	if len(o.Identity.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		return errors.New("identity Ed25519 private key is invalid")
	}
	if len(o.Identity.Ed25519PublicKey) != ed25519.PublicKeySize {
		return errors.New("identity Ed25519 public key is invalid")
	}
	return nil
}

func newController(app fyne.App, options RunOptions) (*controller, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ctrl := &controller{
		app:               app,
		window:            app.NewWindow("GoSend"),
		cfg:               options.Config,
		cfgPath:           options.ConfigPath,
		dataDir:           options.DataDir,
		store:             options.Store,
		identity:          options.Identity,
		ctx:               ctx,
		cancel:            cancel,
		discovered:        make(map[string]discovery.DiscoveredPeer),
		peerSettings:      make(map[string]storage.PeerSettings),
		fileTransfers:     make(map[string]chatFileEntry),
		peerRuntimeStates: make(map[string]network.PeerRuntimeState),
		appForeground:     true,
	}

	ctrl.fileHandler = NewFileHandler(ctrl.pickFilePath)
	ctrl.window.Resize(fyne.NewSize(1200, 760))

	app.Lifecycle().SetOnEnteredForeground(func() {
		ctrl.setAppForeground(true)
	})
	app.Lifecycle().SetOnExitedForeground(func() {
		ctrl.setAppForeground(false)
	})

	ctrl.buildMainWindow()
	ctrl.window.SetOnDropped(ctrl.handleWindowDropped)

	if err := ctrl.startServices(); err != nil {
		ctrl.shutdown()
		return nil, err
	}
	ctrl.startLoops()
	ctrl.refreshPeersFromStore()
	ctrl.syncDiscoveredFromScanner()
	ctrl.reconnectDiscoveredKnownPeers()
	ctrl.refreshChatView()
	if ctrl.activeListenPort > 0 {
		ctrl.setStatus(fmt.Sprintf("Ready (listening on %d)", ctrl.activeListenPort))
	} else {
		ctrl.setStatus("Ready")
	}

	return ctrl, nil
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

func (c *controller) run() error {
	c.window.SetCloseIntercept(func() {
		c.shutdown()
		c.window.SetCloseIntercept(nil)
		c.window.Close()
	})
	c.window.ShowAndRun()
	c.shutdown()
	return nil
}

func (c *controller) shutdown() {
	c.shutdownMu.Do(func() {
		c.cancel()
		c.hideTooltip(nil)
		if c.manager != nil {
			c.manager.Stop()
		}
		if c.discovery != nil {
			c.discovery.Stop()
		}
		c.loopsWg.Wait()
		if c.store != nil {
			_ = c.store.Close()
		}
	})
}

func (c *controller) buildMainWindow() {
	left := c.buildPeersListPane()
	right := c.buildChatPane()

	split := container.NewHSplit(left, right)
	split.Offset = 0.28

	appTitle := canvas.NewText("GoSend", ctpBlue)
	appTitle.TextStyle = fyne.TextStyle{Bold: true}
	appTitle.TextSize = 14
	sep := canvas.NewRectangle(ctpSurface2)
	sep.SetMinSize(fyne.NewSize(1, 20))
	// Mockup: toolbar buttons look like text until hover (flat style)
	queueBtn := newFlatButtonWithIconAndLabel(theme.HistoryIcon(), "Transfer Queue", "Transfer queue", c.showTransferQueuePanel, c.handleHoverHint)
	refreshBtn := newFlatButtonWithIconAndLabel(theme.ViewRefreshIcon(), "Refresh Discovery", "Refresh discovery", func() {
		go c.refreshDiscovery()
	}, c.handleHoverHint)
	discoverBtn := newFlatButtonWithIconAndLabel(theme.SearchIcon(), "Discover", "Discover peers", c.showDiscoveryDialog, c.handleHoverHint)
	settingsBtn := newFlatButtonWithIconAndLabel(theme.SettingsIcon(), "Settings", "Open settings", c.showSettingsDialog, c.handleHoverHint)
	toolbarInner := container.NewHBox(
		container.NewCenter(appTitle),
		container.NewCenter(sep),
		queueBtn, refreshBtn, discoverBtn,
		layout.NewSpacer(),
		settingsBtn,
	)
	toolbarBg := canvas.NewRectangle(ctpMantle)
	toolbarBg.SetMinSize(fyne.NewSize(1, 40))
	toolbar := container.NewStack(toolbarBg, container.NewPadded(toolbarInner))

	c.statusLabel = widget.NewLabel("Starting...")
	c.statusLabel.Importance = widget.LowImportance
	c.statusMessage = "Starting..."
	// Mockup: status bar very thin (h-7 = 28px), minimal padding; Logs = text-like button
	logsBtn := newFlatButtonWithIconAndLabel(theme.DocumentIcon(), "Logs", "View application logs", c.showLogsDialog, c.handleHoverHint)
	c.statusLabel.TextStyle = fyne.TextStyle{}
	statusRow := container.NewBorder(nil, nil, c.statusLabel, logsBtn)
	statusBg := canvas.NewRectangle(ctpCrust)
	statusBg.SetMinSize(fyne.NewSize(1, 28))
	// Use custom insets (2px vertical, 12px horizontal) to keep the bar thin
	statusInner := container.New(layout.NewCustomPaddedLayout(2, 2, 12, 12), statusRow)
	statusBar := container.NewStack(statusBg, statusInner)
	content := container.NewBorder(
		container.NewVBox(toolbar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), statusBar),
		nil, nil, split,
	)
	c.window.SetContent(content)
}

func (c *controller) handleWindowDropped(position fyne.Position, items []fyne.URI) {
	if len(items) == 0 {
		return
	}
	if !c.isDropInsideChatArea(position) {
		return
	}

	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer before dropping files")
		return
	}

	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		path := strings.TrimSpace(item.Path())
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}

	c.flashChatDropIndicator()
	go c.queuePathsForPeer(peerID, paths)
}

func (c *controller) isDropInsideChatArea(position fyne.Position) bool {
	target := c.chatDropArea
	if target == nil {
		return false
	}
	abs := c.app.Driver().AbsolutePositionForObject(target)
	size := target.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return false
	}
	if position.X < abs.X || position.Y < abs.Y {
		return false
	}
	if position.X > abs.X+size.Width || position.Y > abs.Y+size.Height {
		return false
	}
	return true
}

func (c *controller) flashChatDropIndicator() {
	fyne.Do(func() {
		if c.chatDropOverlay == nil {
			return
		}
		c.chatDropOverlay.Show()
		c.chatDropOverlay.Refresh()
	})
	time.AfterFunc(1200*time.Millisecond, func() {
		fyne.Do(func() {
			if c.chatDropOverlay != nil {
				c.chatDropOverlay.Hide()
				c.chatDropOverlay.Refresh()
			}
		})
	})
}

func (c *controller) showTransferQueuePanel() {
	content := container.NewVBox()
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(760, 420))

	currentFilter := "all"
	var subtitleText, footerCountText *canvas.Text
	render := func() {
		entries := c.transferQueueEntriesSnapshot()
		filtered := entries
		switch currentFilter {
		case "active":
			filtered = nil
			for _, e := range entries {
				s := strings.ToLower(e.Status)
				if s == "pending" || s == "accepted" {
					filtered = append(filtered, e)
				}
			}
		case "done":
			filtered = nil
			for _, e := range entries {
				if strings.EqualFold(e.Status, "complete") && e.TransferCompleted {
					filtered = append(filtered, e)
				}
			}
		case "issues":
			filtered = nil
			for _, e := range entries {
				s := strings.ToLower(e.Status)
				if s == "failed" || s == "rejected" {
					filtered = append(filtered, e)
				}
			}
		}

		activeCount, completeCount, issueCount := 0, 0, 0
		for _, e := range entries {
			s := strings.ToLower(e.Status)
			if s == "pending" || s == "accepted" {
				activeCount++
			} else if strings.EqualFold(e.Status, "complete") && e.TransferCompleted {
				completeCount++
			} else if s == "failed" || s == "rejected" {
				issueCount++
			}
		}

		fyne.Do(func() {
			if subtitleText != nil {
				subtitleText.Text = fmt.Sprintf("%d active, %d complete, %d with issues", activeCount, completeCount, issueCount)
				subtitleText.Refresh()
			}
			if footerCountText != nil {
				footerCountText.Text = fmt.Sprintf("Complete: %d  Issues: %d", completeCount, issueCount)
				footerCountText.Refresh()
			}
			content.RemoveAll()
			if len(filtered) == 0 {
				empty := widget.NewLabel("No transfers in this view")
				content.Add(container.NewPadded(empty))
				content.Refresh()
				return
			}

			currentPeer := ""
			for _, entry := range filtered {
				if entry.PeerDeviceID != currentPeer {
					currentPeer = entry.PeerDeviceID
					header := widget.NewLabel(c.transferPeerName(entry.PeerDeviceID))
					header.TextStyle = fyne.TextStyle{Bold: true}
					content.Add(container.NewPadded(header))
				}
				content.Add(c.renderTransferQueueRow(entry))
			}
			content.Refresh()
		})
	}

	render()

	allBtn := widget.NewButton("All", func() { currentFilter = "all"; render() })
	activeBtn := widget.NewButton("Active", func() { currentFilter = "active"; render() })
	doneBtn := widget.NewButton("Completed", func() { currentFilter = "done"; render() })
	issuesBtn := widget.NewButton("Issues", func() { currentFilter = "issues"; render() })
	filterBar := container.NewHBox(allBtn, activeBtn, doneBtn, issuesBtn)
	filterBg := canvas.NewRectangle(ctpSurface0)
	filterBg.SetMinSize(fyne.NewSize(1, 36))
	filterRow := container.NewStack(filterBg, container.NewPadded(filterBar))

	subtitleText = canvas.NewText("0 active, 0 complete, 0 with issues", ctpOverlay1)
	subtitleText.TextSize = 11
	headerTitle := canvas.NewText("Transfer Queue", ctpText)
	headerTitle.TextSize = 14
	headerTitle.TextStyle = fyne.TextStyle{Bold: true}
	headerLeft := container.NewVBox(headerTitle, subtitleText)
	headerBg := canvas.NewRectangle(ctpMantle)
	headerBg.SetMinSize(fyne.NewSize(1, 52))
	header := container.NewStack(headerBg, container.NewPadded(headerLeft))

	clearBtn := widget.NewButton("Clear Completed", func() {
		c.clearCompletedTransfers()
		render()
	})
	closeBtn := widget.NewButton("Close", nil)
	footerCountText = canvas.NewText("Complete: 0  Issues: 0", ctpOverlay1)
	footerCountText.TextSize = 11
	footerRight := container.NewHBox(clearBtn, closeBtn)
	footerBg := canvas.NewRectangle(ctpMantle)
	footerBg.SetMinSize(fyne.NewSize(1, 44))
	footer := container.NewStack(footerBg, container.NewPadded(container.NewBorder(nil, nil, footerCountText, footerRight)))

	panel := container.NewBorder(
		container.NewVBox(header, filterRow),
		footer,
		nil, nil, scroll,
	)
	dlg := dialog.NewCustomWithoutButtons("Transfer Queue", panel, c.window)
	closeBtn.OnTapped = func() { dlg.Hide() }

	stop := make(chan struct{})
	dlg.SetOnClosed(func() { close(stop) })
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				render()
			}
		}
	}()

	dlg.Resize(fyne.NewSize(780, 520))
	dlg.Show()
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
	return "Unknown Peer"
}

func (c *controller) transferQueueEntriesSnapshot() []chatFileEntry {
	now := time.Now().UnixMilli()
	c.chatMu.RLock()
	entries := make([]chatFileEntry, 0, len(c.fileTransfers))
	for _, entry := range c.fileTransfers {
		terminal := strings.EqualFold(entry.Status, "complete") || strings.EqualFold(entry.Status, "failed") || strings.EqualFold(entry.Status, "rejected")
		if terminal {
			completedAt := entry.CompletedAt
			if completedAt == 0 {
				completedAt = entry.AddedAt
			}
			if completedAt > 0 && now-completedAt > int64(30*time.Second/time.Millisecond) {
				continue
			}
		}
		entries = append(entries, entry)
	}
	c.chatMu.RUnlock()

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PeerDeviceID != entries[j].PeerDeviceID {
			return entries[i].PeerDeviceID < entries[j].PeerDeviceID
		}
		if entries[i].AddedAt == entries[j].AddedAt {
			return entries[i].FileID < entries[j].FileID
		}
		return entries[i].AddedAt < entries[j].AddedAt
	})
	return entries
}

func (c *controller) renderTransferQueueRow(entry chatFileEntry) fyne.CanvasObject {
	status := fileTransferStatusText(entry)
	if entry.TransferCompleted && strings.EqualFold(entry.Status, "complete") {
		status = "Complete"
	}
	progressPercent := "--"
	if entry.TotalBytes > 0 {
		progressPercent = fmt.Sprintf("%.0f%%", float64(entry.BytesTransferred)*100/float64(entry.TotalBytes))
	}
	speedLabel := "--"
	if entry.SpeedBytesPerSec > 0 && !entry.TransferCompleted {
		speedLabel = fmt.Sprintf("%s/s", formatBytes(int64(entry.SpeedBytesPerSec)))
	}
	etaLabel := "--"
	if entry.ETASeconds > 0 && !entry.TransferCompleted {
		etaLabel = (time.Duration(entry.ETASeconds) * time.Second).Round(time.Second).String()
	}

	statusColor := ctpOverlay1
	switch {
	case strings.EqualFold(entry.Status, "complete") || (entry.TransferCompleted && strings.EqualFold(entry.Status, "accepted")):
		statusColor = ctpGreen
	case strings.EqualFold(entry.Status, "failed") || strings.EqualFold(entry.Status, "rejected"):
		statusColor = ctpRed
	case strings.EqualFold(entry.Status, "pending") || strings.EqualFold(entry.Status, "accepted"):
		statusColor = ctpBlue
	}

	title := widget.NewLabel(valueOrDefault(entry.Filename, entry.FileID))
	title.TextStyle = fyne.TextStyle{Bold: true}
	metaStr := fmt.Sprintf("%s · %s · %s", formatTimestamp(entry.AddedAt), formatBytes(entry.Filesize), progressPercent)
	if speedLabel != "--" {
		metaStr += " · " + speedLabel
	}
	if etaLabel != "--" {
		metaStr += " · ETA " + etaLabel
	}
	metaText := canvas.NewText(metaStr, ctpOverlay1)
	metaText.TextSize = 11
	statusText := canvas.NewText(" · "+status, statusColor)
	statusText.TextSize = 11
	statusText.TextStyle = fyne.TextStyle{Bold: true}

	rowItems := []fyne.CanvasObject{title, container.NewHBox(metaText, statusText)}
	if !entry.TransferCompleted && (strings.EqualFold(entry.Status, "pending") || strings.EqualFold(entry.Status, "accepted")) {
		cancelBtn := widget.NewButton("Cancel", func() { c.cancelTransferFromUI(entry.FileID) })
		cancelBtn.Importance = widget.DangerImportance
		rowItems = append(rowItems, cancelBtn)
	}
	if (strings.EqualFold(entry.Status, "failed") || strings.EqualFold(entry.Status, "rejected")) && strings.EqualFold(entry.Direction, "send") {
		retryBtn := widget.NewButton("Retry", func() { c.retryTransferFromUI(entry.FileID) })
		rowItems = append(rowItems, retryBtn)
	}
	return container.NewPadded(newRoundedBg(ctpSurface0, 4, container.NewVBox(rowItems...)))
}

func (c *controller) clearCompletedTransfers() {
	c.chatMu.Lock()
	for fileID, entry := range c.fileTransfers {
		if strings.EqualFold(entry.Status, "complete") && entry.TransferCompleted {
			delete(c.fileTransfers, fileID)
		}
	}
	c.chatMu.Unlock()
	c.refreshChatView()
}

const maxRuntimeLogLines = 500

func (c *controller) setStatus(message string) {
	if strings.TrimSpace(message) == "" {
		message = "Ready"
	}
	c.statusMu.Lock()
	c.statusMessage = message
	hoverHint := c.hoverHint
	c.statusMu.Unlock()

	c.runtimeLogMu.Lock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	c.runtimeLogLines = append(c.runtimeLogLines, ts+" "+message)
	if len(c.runtimeLogLines) > maxRuntimeLogLines {
		c.runtimeLogLines = c.runtimeLogLines[len(c.runtimeLogLines)-maxRuntimeLogLines:]
	}
	c.runtimeLogMu.Unlock()

	labelText := message
	if strings.TrimSpace(hoverHint) != "" {
		labelText = hoverHint
	}
	c.applyStatusLabel(labelText)
}

func (c *controller) showLogsDialog() {
	runtimeEntry := widget.NewMultiLineEntry()
	runtimeEntry.SetMinRowsVisible(12)
	runtimeEntry.Disable()
	c.runtimeLogMu.Lock()
	lines := make([]string, len(c.runtimeLogLines))
	copy(lines, c.runtimeLogLines)
	c.runtimeLogMu.Unlock()
	var b strings.Builder
	for i := len(lines) - 1; i >= 0; i-- {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(lines[i])
	}
	runtimeEntry.SetText(b.String())
	runtimeScroll := container.NewScroll(runtimeEntry)
	runtimeScroll.SetMinSize(fyne.NewSize(600, 280))

	securityEntry := widget.NewMultiLineEntry()
	securityEntry.SetMinRowsVisible(12)
	securityEntry.Disable()
	if c.store != nil {
		events, err := c.store.GetSecurityEvents(storage.SecurityEventFilter{Limit: 300})
		if err == nil {
			var sb strings.Builder
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				ts := time.UnixMilli(e.Timestamp).Format("2006-01-02 15:04:05")
				sb.WriteString(fmt.Sprintf("[%s] %s %s", ts, e.Severity, e.EventType))
				if e.PeerDeviceID != nil && *e.PeerDeviceID != "" {
					sb.WriteString(" peer=" + *e.PeerDeviceID)
				}
				sb.WriteString(" " + e.Details)
			}
			securityEntry.SetText(sb.String())
		}
	}
	securityScroll := container.NewScroll(securityEntry)
	securityScroll.SetMinSize(fyne.NewSize(600, 280))

	tabs := container.NewAppTabs(
		container.NewTabItem("Runtime", runtimeScroll),
		container.NewTabItem("Security Events", securityScroll),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	closeBtn := widget.NewButton("Close", nil)
	content := container.NewBorder(nil, container.NewPadded(closeBtn), nil, nil, tabs)
	d := dialog.NewCustomWithoutButtons("Application Logs", content, c.window)
	closeBtn.OnTapped = func() { d.Hide() }
	d.Resize(fyne.NewSize(620, 380))
	d.Show()
}

func (c *controller) setAppForeground(foreground bool) {
	c.appStateMu.Lock()
	c.appForeground = foreground
	c.appStateMu.Unlock()
}

func (c *controller) isAppForeground() bool {
	c.appStateMu.RLock()
	defer c.appStateMu.RUnlock()
	return c.appForeground
}

func (c *controller) handleIncomingMessage(message storage.Message) {
	c.refreshChatView()
	c.refreshPeersFromStore()
	c.maybeNotifyIncomingMessage(message)
}

func (c *controller) handlePeerRuntimeStateChanged(state network.PeerRuntimeState) {
	if strings.TrimSpace(state.PeerDeviceID) == "" {
		return
	}

	c.runtimeMu.Lock()
	c.peerRuntimeStates[state.PeerDeviceID] = state
	c.runtimeMu.Unlock()

	fyne.Do(func() {
		if c.peerList != nil {
			c.peerList.Refresh()
		}
		c.updateChatHeader()
	})
}

func (c *controller) runtimeStateForPeer(peerID string) network.PeerRuntimeState {
	if strings.TrimSpace(peerID) == "" {
		return network.PeerRuntimeState{}
	}
	c.runtimeMu.RLock()
	state, ok := c.peerRuntimeStates[peerID]
	c.runtimeMu.RUnlock()
	if !ok {
		state = network.PeerRuntimeState{
			PeerDeviceID:    peerID,
			ConnectionState: network.StateDisconnected,
		}
	}
	if c.manager != nil {
		state.ConnectionState = c.manager.PeerConnectionState(peerID)
		if snapshot := c.manager.PeerRuntimeStateSnapshot(peerID); snapshot.PeerDeviceID != "" {
			if snapshot.Reconnecting {
				state.Reconnecting = true
				state.NextReconnectAt = snapshot.NextReconnectAt
			} else if state.Reconnecting && snapshot.NextReconnectAt == 0 {
				state.Reconnecting = false
				state.NextReconnectAt = 0
			}
		}
	}
	return state
}

func (c *controller) maybeNotifyIncomingMessage(message storage.Message) {
	peerID := strings.TrimSpace(message.FromDeviceID)
	if peerID == "" {
		return
	}
	if !c.shouldSendNotification(peerID) {
		return
	}

	peerName := c.transferPeerName(peerID)
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = "New message"
	}
	if len(content) > 180 {
		content = content[:177] + "..."
	}
	c.sendDesktopNotification("New message from "+peerName, content)
}

func (c *controller) maybeNotifyIncomingFileRequest(notification network.FileRequestNotification) {
	peerID := strings.TrimSpace(notification.FromDeviceID)
	if peerID == "" {
		return
	}
	if !c.shouldSendNotification(peerID) {
		return
	}

	name := strings.TrimSpace(notification.Filename)
	if name == "" {
		name = "Unnamed file"
	}
	c.sendDesktopNotification(
		"Incoming file request",
		fmt.Sprintf("%s wants to send %s", c.transferPeerName(peerID), name),
	)
}

func (c *controller) shouldSendNotification(peerID string) bool {
	if c == nil || c.cfg == nil || !c.cfg.NotificationsEnabled {
		return false
	}

	settings := c.peerSettingsByID(peerID)
	if settings == nil && c.store != nil {
		if loaded, err := c.store.GetPeerSettings(peerID); err == nil {
			settings = loaded
		}
	}
	if settings != nil && settings.NotificationsMuted {
		return false
	}

	selectedPeerID := c.currentSelectedPeerID()
	if selectedPeerID != peerID {
		return true
	}
	return !c.isAppForeground()
}

func (c *controller) sendDesktopNotification(title, content string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.SendNotification(fyne.NewNotification(title, content))
}

func (c *controller) setHoverHint(message string) {
	c.statusMu.Lock()
	c.hoverHint = strings.TrimSpace(message)
	statusMessage := c.statusMessage
	hoverHint := c.hoverHint
	c.statusMu.Unlock()

	labelText := statusMessage
	if strings.TrimSpace(labelText) == "" {
		labelText = "Ready"
	}
	if hoverHint != "" {
		labelText = hoverHint
	}
	c.applyStatusLabel(labelText)
}

func (c *controller) handleHoverHint(target fyne.CanvasObject, hint string, active bool) {
	if !active {
		c.setHoverHint("")
		c.hideTooltip(target)
		return
	}

	trimmedHint := strings.TrimSpace(hint)
	if target == nil || trimmedHint == "" {
		c.setHoverHint("")
		c.hideTooltip(target)
		return
	}
	c.setHoverHint(trimmedHint)
	c.scheduleTooltip(target, trimmedHint)
}

func (c *controller) scheduleTooltip(target fyne.CanvasObject, hint string) {
	c.tooltipMu.Lock()
	if c.tooltipTimer != nil {
		c.tooltipTimer.Stop()
		c.tooltipTimer = nil
	}
	c.removeTooltipOverlay()
	c.tooltipTarget = target
	c.tooltipText = hint
	c.tooltipTimer = time.AfterFunc(450*time.Millisecond, func() {
		c.showTooltip(target, hint)
	})
	c.tooltipMu.Unlock()
}

func (c *controller) showTooltip(target fyne.CanvasObject, hint string) {
	fyne.Do(func() {
		c.tooltipMu.Lock()
		if c.tooltipTarget != target || c.tooltipText != hint {
			c.tooltipMu.Unlock()
			return
		}
		c.tooltipTimer = nil
		c.tooltipMu.Unlock()

		if target == nil || strings.TrimSpace(hint) == "" {
			return
		}
		drv := c.app.Driver()
		canv := drv.CanvasForObject(target)
		if canv == nil {
			return
		}

		const maxTooltipChars = 18
		displayHint := hint
		if len(displayHint) > maxTooltipChars {
			displayHint = displayHint[:maxTooltipChars-3] + "..."
		}
		tooltipText := canvas.NewText(displayHint, themedColor(theme.ColorNameForeground))
		tooltipText.TextSize = 10
		wrapped := container.NewPadded(container.NewCenter(tooltipText))
		pill := newRoundedBg(themedColor(theme.ColorNameInputBackground), 3, wrapped)
		pillSize := pill.MinSize()
		pill.Resize(pillSize)

		const gap = 6
		const edgePad = 4
		canvasSize := canv.Size()
		targetAbs := drv.AbsolutePositionForObject(target)
		targetSize := target.Size()
		// Prefer right of button; fall back to left if it would overflow.
		pos := targetAbs.Add(fyne.NewPos(targetSize.Width+gap, 0))
		if pos.X+pillSize.Width+edgePad > canvasSize.Width {
			pos.X = targetAbs.X - pillSize.Width - gap
		}
		if pos.X < edgePad {
			pos.X = edgePad
		}
		if pos.Y+pillSize.Height+edgePad > canvasSize.Height {
			pos.Y = canvasSize.Height - pillSize.Height - edgePad
		}
		if pos.Y < edgePad {
			pos.Y = edgePad
		}
		pill.Move(pos)
		box := container.NewWithoutLayout(pill)
		box.Resize(canv.Size())

		c.tooltipMu.Lock()
		if c.tooltipTarget != target || c.tooltipText != hint {
			c.tooltipMu.Unlock()
			canv.Overlays().Remove(box)
			return
		}
		c.removeTooltipOverlay()
		canv.Overlays().Add(box)
		c.tooltipOverlay = box
		c.tooltipCanvas = canv
		c.tooltipMu.Unlock()
	})
}

// removeTooltipOverlay removes the current tooltip from the canvas overlay.
// Must be called with c.tooltipMu held.
func (c *controller) removeTooltipOverlay() {
	if c.tooltipOverlay != nil && c.tooltipCanvas != nil {
		c.tooltipCanvas.Overlays().Remove(c.tooltipOverlay)
		c.tooltipOverlay = nil
		c.tooltipCanvas = nil
	}
}

func (c *controller) hideTooltip(target fyne.CanvasObject) {
	var overlay fyne.CanvasObject
	var canv fyne.Canvas

	c.tooltipMu.Lock()
	if c.tooltipTimer != nil {
		c.tooltipTimer.Stop()
		c.tooltipTimer = nil
	}
	if target == nil || c.tooltipTarget == target {
		overlay = c.tooltipOverlay
		canv = c.tooltipCanvas
		c.tooltipOverlay = nil
		c.tooltipCanvas = nil
		c.tooltipTarget = nil
		c.tooltipText = ""
	}
	c.tooltipMu.Unlock()

	if overlay != nil && canv != nil {
		fyne.Do(func() {
			canv.Overlays().Remove(overlay)
		})
	}
}

func (c *controller) applyStatusLabel(message string) {
	if strings.TrimSpace(message) == "" {
		message = "Ready"
	}
	fyne.Do(func() {
		if c.statusLabel != nil {
			c.statusLabel.SetText(message)
		}
	})
}

func (c *controller) refreshDiscovery() {
	c.refreshMu.Lock()
	if c.refreshRunning {
		c.refreshMu.Unlock()
		c.setStatus("Discovery refresh already in progress")
		return
	}
	c.refreshRunning = true
	c.refreshMu.Unlock()
	defer func() {
		c.refreshMu.Lock()
		c.refreshRunning = false
		c.refreshMu.Unlock()
	}()

	if c.discovery == nil || c.discovery.Scanner == nil {
		c.setStatus("Discovery service is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()

	if err := c.discovery.Scanner.Refresh(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			c.setStatus("Discovery refresh timed out; a previous scan may still be in progress")
			return
		}
		c.setStatus(fmt.Sprintf("Discovery refresh failed: %v", err))
		return
	}
	c.syncDiscoveredFromScanner()
	c.setStatus("Discovery refreshed")
}

func (c *controller) syncDiscoveredFromScanner() {
	if c.discovery == nil || c.discovery.Scanner == nil {
		return
	}

	rows := c.discovery.Scanner.ListPeers()
	c.discoveredMu.Lock()
	next := make(map[string]discovery.DiscoveredPeer, len(rows))
	for _, row := range rows {
		next[row.DeviceID] = row
	}
	c.discovered = next
	c.discoveredMu.Unlock()
	c.refreshDiscoveryRows()
}

func (c *controller) discoveryKnownPeers() []discovery.KnownPeerIdentity {
	peers, err := c.store.ListPeers()
	if err != nil {
		return nil
	}

	known := make([]discovery.KnownPeerIdentity, 0, len(peers))
	for _, peer := range peers {
		deviceID := strings.TrimSpace(peer.DeviceID)
		publicKey := strings.TrimSpace(peer.Ed25519PublicKey)
		if deviceID == "" || publicKey == "" {
			continue
		}
		known = append(known, discovery.KnownPeerIdentity{
			DeviceID:         deviceID,
			Ed25519PublicKey: publicKey,
		})
	}

	known = append(known, discovery.KnownPeerIdentity{
		DeviceID:         c.identity.DeviceID,
		Ed25519PublicKey: base64.StdEncoding.EncodeToString(c.identity.Ed25519PublicKey),
	})
	return known
}

func (c *controller) tryReconnectDiscoveredPeer(peer discovery.DiscoveredPeer) {
	if c.manager == nil || len(peer.Addresses) == 0 || peer.Port <= 0 {
		return
	}
	if !c.isKnownPeer(peer.DeviceID) {
		return
	}
	c.manager.NotifyPeerDiscoveredEndpoints(peer.DeviceID, peer.DeviceName, peer.Addresses, peer.Port)
}

func (c *controller) reconnectDiscoveredKnownPeers() {
	if c.manager == nil {
		return
	}
	discovered := c.listDiscoveredSnapshot()
	for _, dp := range discovered {
		c.tryReconnectDiscoveredPeer(dp)
	}
}

func (c *controller) promptIncomingPeerAddRequest(req network.AddRequestNotification) (bool, error) {
	decision := make(chan bool, 1)

	fyne.Do(func() {
		peerName := req.PeerDeviceName
		if strings.TrimSpace(peerName) == "" {
			peerName = req.PeerDeviceID
		}
		message := fmt.Sprintf("%s wants to add you as a peer. Accept?", peerName)
		dialog.ShowConfirm("Incoming Peer Request", message, func(accept bool) {
			decision <- accept
		}, c.window)
	})

	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case accept := <-decision:
		if accept {
			c.setStatus(fmt.Sprintf("Accepted peer request from %s", req.PeerDeviceName))
		} else {
			c.setStatus(fmt.Sprintf("Rejected peer request from %s", req.PeerDeviceName))
		}
		return accept, nil
	}
}

func (c *controller) promptFileRequestDecision(notification network.FileRequestNotification) (bool, error) {
	c.filePromptPendingMu.Lock()
	if c.filePromptPending == nil {
		c.filePromptPending = make(map[string]struct{})
	}
	if _, already := c.filePromptPending[notification.FileID]; already {
		c.filePromptPendingMu.Unlock()
		return false, nil
	}
	c.filePromptPending[notification.FileID] = struct{}{}
	c.filePromptPendingMu.Unlock()

	c.maybeNotifyIncomingFileRequest(notification)

	decision := make(chan bool, 1)
	info := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("%s wants to send you a file:", c.transferPeerName(notification.FromDeviceID))),
		widget.NewLabel(""),
		widget.NewLabel("Name: "+notification.Filename),
		widget.NewLabel("Size: "+formatBytes(notification.Filesize)),
		widget.NewLabel("Type: "+valueOrDefault(notification.Filetype, "unknown")),
	)

	fyne.Do(func() {
		dlg := dialog.NewCustomConfirm(
			"Incoming File",
			"Accept",
			"Reject",
			info,
			func(accept bool) {
				c.filePromptPendingMu.Lock()
				delete(c.filePromptPending, notification.FileID)
				c.filePromptPendingMu.Unlock()
				decision <- accept
			},
			c.window,
		)
		dlg.Show()
	})

	select {
	case <-c.ctx.Done():
		c.filePromptPendingMu.Lock()
		delete(c.filePromptPending, notification.FileID)
		c.filePromptPendingMu.Unlock()
		return false, errors.New("application is shutting down")
	case accept := <-decision:
		status := "rejected"
		if accept {
			status = "accepted"
		}
		c.upsertFileTransfer(chatFileEntry{
			FileID:       notification.FileID,
			PeerDeviceID: notification.FromDeviceID,
			Direction:    "receive",
			Filename:     notification.Filename,
			Filesize:     notification.Filesize,
			Filetype:     notification.Filetype,
			AddedAt:      time.Now().UnixMilli(),
			Status:       status,
		})
		c.refreshChatForPeer(notification.FromDeviceID)
		return accept, nil
	}
}

func (c *controller) promptKeyChangeDecision(peerDeviceID, existingPublicKeyBase64, receivedPublicKeyBase64 string) (bool, error) {
	decision := make(chan bool, 1)

	oldFingerprint := fingerprintFromBase64(existingPublicKeyBase64)
	newFingerprint := fingerprintFromBase64(receivedPublicKeyBase64)
	peerName := peerDeviceID
	if peer := c.peerByID(peerDeviceID); peer != nil && peer.DeviceName != "" {
		peerName = peer.DeviceName
	}

	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("The identity key for %s has changed.", peerName)),
		widget.NewLabel("This could mean the device was reset, or a MITM attempt."),
		widget.NewLabel(""),
		widget.NewLabel("Old fingerprint: "+oldFingerprint),
		widget.NewLabel("New fingerprint: "+newFingerprint),
	)

	fyne.Do(func() {
		dlg := dialog.NewCustomConfirm(
			"Key Change Warning",
			"Trust New Key",
			"Disconnect",
			content,
			func(trust bool) {
				decision <- trust
			},
			c.window,
		)
		dlg.Show()
	})

	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case trust := <-decision:
		if trust {
			c.setStatus(fmt.Sprintf("Trusted new key for %s", peerName))
		} else {
			c.setStatus(fmt.Sprintf("Rejected key change for %s", peerName))
		}
		return trust, nil
	}
}

func (c *controller) pickFilePath() ([]string, error) {
	path, err := c.pickSingleFilePath()
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func (c *controller) pickSingleFilePath() (string, error) {
	type pickResult struct {
		path string
		err  error
	}
	result := make(chan pickResult, 1)

	fyne.Do(func() {
		dlg := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				result <- pickResult{err: err}
				return
			}
			if reader == nil || reader.URI() == nil {
				result <- pickResult{err: errFilePickerCancelled}
				return
			}
			path := reader.URI().Path()
			_ = reader.Close()
			if strings.TrimSpace(path) == "" {
				result <- pickResult{err: errFilePickerCancelled}
				return
			}
			result <- pickResult{path: path}
		}, c.window)
		dlg.Show()
	})

	select {
	case <-c.ctx.Done():
		return "", errors.New("application is shutting down")
	case picked := <-result:
		return picked.path, picked.err
	}
}

func (c *controller) pickFolderPath() (string, error) {
	type pickResult struct {
		path string
		err  error
	}
	result := make(chan pickResult, 1)

	fyne.Do(func() {
		dlg := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
			if err != nil {
				result <- pickResult{err: err}
				return
			}
			if folder == nil {
				result <- pickResult{err: errFilePickerCancelled}
				return
			}
			path := folder.Path()
			if strings.TrimSpace(path) == "" {
				result <- pickResult{err: errFilePickerCancelled}
				return
			}
			result <- pickResult{path: path}
		}, c.window)
		dlg.Show()
	})

	select {
	case <-c.ctx.Done():
		return "", errors.New("application is shutting down")
	case picked := <-result:
		return picked.path, picked.err
	}
}

func (c *controller) peerByID(deviceID string) *storage.Peer {
	if strings.TrimSpace(deviceID) == "" {
		return nil
	}
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	for i := range c.peers {
		if c.peers[i].DeviceID == deviceID {
			peer := c.peers[i]
			return &peer
		}
	}
	return nil
}

func (c *controller) peerSettingsByID(deviceID string) *storage.PeerSettings {
	if strings.TrimSpace(deviceID) == "" {
		return nil
	}
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	settings, ok := c.peerSettings[deviceID]
	if !ok {
		return nil
	}
	copySettings := settings
	return &copySettings
}

func (c *controller) peerDisplayName(peer *storage.Peer) string {
	if peer == nil {
		return ""
	}
	settings := c.peerSettingsByID(peer.DeviceID)
	if settings != nil && strings.TrimSpace(settings.CustomName) != "" {
		return strings.TrimSpace(settings.CustomName)
	}
	return peer.DeviceName
}

func (c *controller) peerTrustLevel(peerDeviceID string) string {
	settings := c.peerSettingsByID(peerDeviceID)
	if settings == nil {
		return storage.PeerTrustLevelNormal
	}
	if settings.TrustLevel == "" {
		return storage.PeerTrustLevelNormal
	}
	return settings.TrustLevel
}

func (c *controller) listPeersSnapshot() []storage.Peer {
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	out := make([]storage.Peer, len(c.peers))
	copy(out, c.peers)
	return out
}

func (c *controller) listDiscoveredSnapshot() []discovery.DiscoveredPeer {
	c.discoveredMu.RLock()
	defer c.discoveredMu.RUnlock()
	out := make([]discovery.DiscoveredPeer, 0, len(c.discovered))
	for _, peer := range c.discovered {
		out = append(out, peer)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeviceName == out[j].DeviceName {
			return out[i].DeviceID < out[j].DeviceID
		}
		return out[i].DeviceName < out[j].DeviceName
	})
	return out
}

func listeningPortFromAddr(addr net.Addr) (int, error) {
	if addr == nil {
		return 0, errors.New("missing listen address")
	}
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, fmt.Errorf("parse listen address %q: %w", addr.String(), err)
	}
	_ = host
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("invalid listen port %q", portText)
	}
	return port, nil
}

func fingerprintFromBase64(publicKeyBase64 string) string {
	publicKeyRaw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil || len(publicKeyRaw) != ed25519.PublicKeySize {
		trimmed := publicKeyBase64
		if len(trimmed) > 16 {
			trimmed = trimmed[:16] + "..."
		}
		return trimmed
	}
	return appcrypto.FormatFingerprint(appcrypto.KeyFingerprint(ed25519.PublicKey(publicKeyRaw)))
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatBytes(size int64) string {
	if size < 0 {
		return "0 B"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	prefixes := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(prefixes) {
		exp = len(prefixes) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(size)/float64(div), prefixes[exp])
}

func (c *controller) currentDownloadDirectory() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	path := strings.TrimSpace(c.cfg.DownloadDirectory)
	if path != "" {
		return path
	}
	if strings.TrimSpace(c.dataDir) != "" {
		return filepath.Join(c.dataDir, "files")
	}
	return ""
}

func (c *controller) currentMaxReceiveFileSize() int64 {
	if c == nil || c.cfg == nil {
		return 0
	}
	if c.cfg.MaxReceiveFileSize <= 0 {
		return 0
	}
	return c.cfg.MaxReceiveFileSize
}

func (c *controller) currentMessageRetentionDays() int {
	if c == nil || c.cfg == nil {
		return 0
	}
	switch c.cfg.MessageRetentionDays {
	case 30, 90, 365:
		return c.cfg.MessageRetentionDays
	default:
		return 0
	}
}

func (c *controller) cleanupDownloadedFilesEnabled() bool {
	if c == nil || c.cfg == nil {
		return false
	}
	return c.cfg.CleanupDownloadedFiles
}

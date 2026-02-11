package ui

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"image/color"
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
	sqdialog "github.com/sqweek/dialog"

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
	discoveryDialog *widget.PopUp
	discoveryList   *widget.List
	discoverySelect string
	discoveryAddBtn *flatButton
	discoveryAddBg  *canvas.Rectangle

	chatMu            sync.RWMutex
	chatMessages      []storage.Message
	chatFiles         []chatFileEntry
	fileTransfers     map[string]chatFileEntry
	chatSearchQuery   string
	chatSearchVisible bool
	chatFilesOnly     bool

	fileProgressBars   map[string]*thinProgressBar
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
	statusLabel          *canvas.Text
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
	ctrl.window.SetPadded(false)

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
	appTitle.TextStyle = fyne.TextStyle{}
	appTitle.TextSize = 14
	sep := canvas.NewRectangle(ctpSurface2)
	sep.SetMinSize(fyne.NewSize(1, 20))
	// Mockup: toolbar buttons look like text until hover (flat style)
	queueBtn := newFlatButtonWithIconAndLabel(iconHistory(), "Transfer Queue", "Transfer queue", c.showTransferQueuePanel, c.handleHoverHint)
	discoverBtn := newFlatButtonWithIconAndLabel(iconSearch(), "Discover", "Discover peers", c.showDiscoveryDialog, c.handleHoverHint)
	settingsBtn := newFlatButtonWithIconAndLabel(iconSettings(), "Settings", "Open settings", c.showSettingsDialog, c.handleHoverHint)
	toolbarInner := container.NewHBox(
		container.NewCenter(appTitle),
		container.NewCenter(sep),
		queueBtn, discoverBtn,
		layout.NewSpacer(),
		settingsBtn,
	)
	toolbarBg := canvas.NewRectangle(ctpMantle)
	toolbarBg.SetMinSize(fyne.NewSize(1, 40))
	toolbar := container.NewStack(toolbarBg, container.NewPadded(toolbarInner))

	c.statusLabel = canvas.NewText("Starting...", ctpOverlay1)
	c.statusLabel.TextSize = 10
	c.statusMessage = "Starting..."
	// Footer: compact status text and compact Logs action.
	logsBtn := newCompactFlatButtonWithIconAndLabel(iconDocument(), "Logs", "View application logs", c.showLogsDialog, c.handleHoverHint)
	statusRow := container.NewBorder(nil, nil, c.statusLabel, logsBtn)
	statusBg := canvas.NewRectangle(ctpCrust)
	statusBg.SetMinSize(fyne.NewSize(1, 24))
	statusInner := container.New(layout.NewCustomPaddedLayout(1, 1, 10, 10), statusRow)
	statusBar := container.NewStack(statusBg, statusInner)
	content := newNoGapBorder(withBottomDivider(toolbar), withTopDivider(statusBar), split)
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
	var subtitleText, completeCountText, issueCountText *canvas.Text
	filterButtons := make(map[string]*flatButton, 4)
	filterBGs := make(map[string]*canvas.Rectangle, 4)
	var popup *widget.PopUp
	stop := make(chan struct{})
	var stopOnce sync.Once

	closeQueue := func() {
		stopOnce.Do(func() { close(stop) })
		if popup != nil {
			popup.Hide()
		}
	}

	refreshFilterButtons := func() {
		for key, btn := range filterButtons {
			bg := filterBGs[key]
			if btn == nil || bg == nil {
				continue
			}
			selected := key == currentFilter
			if selected {
				bg.FillColor = ctpBlue
				btn.labelColor = ctpBase
				btn.hoverColor = ctpLavender
			} else {
				bg.FillColor = color.Transparent
				btn.labelColor = ctpSubtext0
				btn.hoverColor = ctpSurface1
			}
			bg.Refresh()
			btn.Refresh()
		}
	}

	render := func() {
		entries := c.transferQueueEntriesSnapshot()
		filtered := make([]chatFileEntry, 0, len(entries))
		activeCount, completeCount, issueCount := 0, 0, 0

		for _, e := range entries {
			switch transferQueueCategory(e) {
			case "active":
				activeCount++
			case "done":
				completeCount++
			case "issues":
				issueCount++
			}
			if currentFilter == "all" || currentFilter == transferQueueCategory(e) {
				filtered = append(filtered, e)
			}
		}

		fyne.Do(func() {
			refreshFilterButtons()
			if subtitleText != nil {
				subtitleText.Text = fmt.Sprintf("%d active, %d complete, %d with issues", activeCount, completeCount, issueCount)
				subtitleText.Refresh()
			}
			if completeCountText != nil {
				completeCountText.Text = fmt.Sprintf("✓ Complete: %d", completeCount)
				completeCountText.Refresh()
			}
			if issueCountText != nil {
				issueCountText.Text = fmt.Sprintf("! Issues: %d", issueCount)
				issueCountText.Refresh()
			}
			content.RemoveAll()
			if len(filtered) == 0 {
				empty := canvas.NewText("No transfers in this view", ctpSubtext0)
				empty.TextSize = 22
				content.Add(container.New(layout.NewCustomPaddedLayout(14, 14, 18, 0), empty))
				content.Refresh()
				return
			}

			for _, entry := range filtered {
				content.Add(withBottomDivider(c.renderTransferQueueRow(entry)))
			}
			content.Refresh()
		})
	}

	makeFilterButton := func(key, label string) fyne.CanvasObject {
		btn := newCompactFlatButtonWithIconAndLabel(nil, label, "", func() {
			currentFilter = key
			render()
		}, c.handleHoverHint)
		btn.labelSize = 10
		btn.padTop = 1
		btn.padBottom = 1
		btn.padLeft = 8
		btn.padRight = 8
		btn.hoverColor = ctpSurface1
		btn.labelColor = ctpSubtext0
		bg := canvas.NewRectangle(color.Transparent)
		bg.CornerRadius = 4
		filterButtons[key] = btn
		filterBGs[key] = bg
		return container.NewStack(bg, btn)
	}

	allBtn := makeFilterButton("all", "All")
	activeBtn := makeFilterButton("active", "Active")
	doneBtn := makeFilterButton("done", "Completed")
	issuesBtn := makeFilterButton("issues", "Issues")
	filterBar := container.New(layout.NewCustomPaddedHBoxLayout(4), allBtn, activeBtn, doneBtn, issuesBtn)
	filterBg := canvas.NewRectangle(ctpSurface0)
	filterBg.SetMinSize(fyne.NewSize(1, 36))
	filterRow := withBottomDivider(container.NewStack(filterBg, container.New(layout.NewCustomPaddedLayout(4, 4, 12, 12), filterBar)))

	subtitleText = canvas.NewText("0 active, 0 complete, 0 with issues", ctpOverlay1)
	subtitleText.TextSize = 11
	headerTitle := canvas.NewText("Transfer Queue", ctpText)
	headerTitle.TextSize = 14
	headerTitle.TextStyle = fyne.TextStyle{Bold: true}
	headerLeft := container.NewVBox(headerTitle, subtitleText)

	headerCloseBtn := newPanelIconCloseButton("Close transfer queue", closeQueue, c.handleHoverHint)

	headerBg := canvas.NewRectangle(ctpMantle)
	headerBg.SetMinSize(fyne.NewSize(1, 56))
	headerMain := container.NewHBox(headerLeft, layout.NewSpacer(), headerCloseBtn)
	header := withBottomDivider(container.NewStack(headerBg, container.New(layout.NewCustomPaddedLayout(6, 6, 12, 10), headerMain)))

	clearBtn := newPanelActionButton("Clear Completed", "Clear completed transfers", panelActionSecondary, func() {
		c.clearCompletedTransfers()
		render()
	}, c.handleHoverHint)
	closeBtn := newPanelActionButton("Close", "Close transfer queue", panelActionPrimary, closeQueue, c.handleHoverHint)

	completeCountText = canvas.NewText("✓ Complete: 0", ctpGreen)
	completeCountText.TextSize = 11
	issueCountText = canvas.NewText("! Issues: 0", ctpRed)
	issueCountText.TextSize = 11
	footerCounts := container.New(layout.NewCustomPaddedHBoxLayout(10), completeCountText, issueCountText)
	footerRight := container.New(layout.NewCustomPaddedHBoxLayout(8), clearBtn, closeBtn)
	footer := newPanelFooter(footerCounts, footerRight)
	panel := newPanelFrame(newVNoGap(header, filterRow), footer, scroll)
	render()

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

	popup = newPanelPopup(c.window, panel, fyne.NewSize(780, 520))
	popup.Show()
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
		terminal := transferQueueCategory(entry) != "active"
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
		bucketI, bucketJ := transferQueueSortBucket(entries[i]), transferQueueSortBucket(entries[j])
		if bucketI != bucketJ {
			return bucketI < bucketJ
		}
		if entries[i].AddedAt == entries[j].AddedAt {
			return entries[i].FileID < entries[j].FileID
		}
		return entries[i].AddedAt > entries[j].AddedAt
	})
	return entries
}

func (c *controller) renderTransferQueueRow(entry chatFileEntry) fyne.CanvasObject {
	status := transferQueueStatusText(entry)
	statusColor := transferQueueStatusColor(entry)

	directionLabel := "From"
	directionIcon := iconDownload()
	if strings.EqualFold(entry.Direction, "send") {
		directionLabel = "To"
		directionIcon = iconUpload()
	}

	path := strings.TrimSpace(entry.StoredPath)
	category := transferQueueCategory(entry)
	canPause := false
	canResume := false
	canRetry := category == "issues" && strings.EqualFold(entry.Direction, "send")
	canCancel := category == "active"
	canShowPath := category == "done" && path != ""

	progress := float32(0)
	if entry.TotalBytes > 0 {
		progress = float32(entry.BytesTransferred) / float32(entry.TotalBytes)
	}
	if category == "done" {
		progress = 1
	}
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}

	speedLabel := ""
	if entry.SpeedBytesPerSec > 0 && category == "active" {
		speedLabel = fmt.Sprintf("%s/s", formatBytes(int64(entry.SpeedBytesPerSec)))
	}
	etaLabel := ""
	if entry.ETASeconds > 0 && category == "active" {
		etaLabel = (time.Duration(entry.ETASeconds) * time.Second).Round(time.Second).String()
	}

	nameLabel := widget.NewLabel(valueOrDefault(entry.Filename, entry.FileID))
	nameLabel.Truncation = fyne.TextTruncateEllipsis
	sizeLabel := canvas.NewText(formatBytes(entry.Filesize), ctpOverlay1)
	sizeLabel.TextSize = 11
	sizeLabel.TextStyle = fyne.TextStyle{Monospace: true}

	dirIcon := widget.NewIcon(directionIcon)
	topLeft := container.New(layout.NewCustomPaddedHBoxLayout(8), container.NewGridWrap(fyne.NewSize(14, 14), dirIcon), nameLabel)
	topRow := container.NewBorder(nil, nil, topLeft, sizeLabel, nil)

	peerText := canvas.NewText(fmt.Sprintf("%s %s", directionLabel, c.transferPeerName(entry.PeerDeviceID)), ctpOverlay1)
	peerText.TextSize = 11
	statusText := canvas.NewText(status, statusColor)
	statusText.TextStyle = fyne.TextStyle{Bold: true}
	statusText.TextSize = 11

	metaItems := []fyne.CanvasObject{peerText, statusText}
	if speedLabel != "" {
		speedText := canvas.NewText(speedLabel, ctpOverlay1)
		speedText.TextSize = 11
		metaItems = append(metaItems, speedText)
	}
	if etaLabel != "" {
		etaText := canvas.NewText("ETA "+etaLabel, ctpOverlay1)
		etaText.TextSize = 11
		metaItems = append(metaItems, etaText)
	}
	metaRow := container.New(layout.NewCustomPaddedHBoxLayout(8), metaItems...)

	makeAction := func(icon fyne.Resource, label string, enabled bool, tapped func()) fyne.CanvasObject {
		if tapped == nil {
			tapped = func() {}
		}
		btn := newCompactFlatButtonWithIconAndLabelState(icon, label, enabled, tapped)
		btn.labelSize = 10
		btn.iconSize = 11
		btn.padTop = 1
		btn.padBottom = 1
		btn.padLeft = 6
		btn.padRight = 6
		return btn
	}
	actions := container.New(layout.NewCustomPaddedHBoxLayout(4),
		makeAction(iconPause(), "Pause", canPause, nil),
		makeAction(iconResume(), "Resume", canResume, nil),
		makeAction(iconRefresh(), "Retry", canRetry, func() { c.retryTransferFromUI(entry.FileID) }),
		makeAction(iconBlock(), "Cancel", canCancel, func() { c.cancelTransferFromUI(entry.FileID) }),
		makeAction(iconFolderOpen(), "Show Path", canShowPath, func() {
			if err := openContainingFolder(path); err != nil {
				dialog.ShowError(err, c.window)
			}
		}),
	)

	indent := func(object fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedLayout(0, 0, 22, 0), object)
	}

	items := []fyne.CanvasObject{
		topRow,
		indent(metaRow),
		indent(newTransferQueueProgressBar(progress, statusColor)),
	}
	if path != "" {
		pathText := canvas.NewText(path, ctpOverlay0)
		pathText.TextSize = 11
		pathText.TextStyle = fyne.TextStyle{Monospace: true}
		items = append(items, indent(pathText))
	}
	items = append(items, indent(actions))

	row := container.New(layout.NewCustomPaddedVBoxLayout(4), items...)
	return container.New(layout.NewCustomPaddedLayout(8, 8, 12, 12), row)
}

func transferQueueCategory(entry chatFileEntry) string {
	status := strings.ToLower(strings.TrimSpace(entry.Status))
	switch {
	case strings.EqualFold(status, "complete"), (entry.TransferCompleted && strings.EqualFold(status, "accepted")):
		return "done"
	case strings.EqualFold(status, "failed"), strings.EqualFold(status, "rejected"), strings.EqualFold(status, "canceled"):
		return "issues"
	default:
		return "active"
	}
}

func transferQueueSortBucket(entry chatFileEntry) int {
	switch transferQueueCategory(entry) {
	case "active":
		return 0
	case "issues":
		return 1
	default:
		return 2
	}
}

func transferQueueStatusText(entry chatFileEntry) string {
	status := strings.ToLower(strings.TrimSpace(entry.Status))
	switch {
	case strings.EqualFold(status, "complete"), (entry.TransferCompleted && strings.EqualFold(status, "accepted")):
		return "Complete"
	case strings.EqualFold(status, "failed"), strings.EqualFold(status, "rejected"):
		return "Failed"
	case strings.EqualFold(status, "canceled"):
		return "Canceled"
	case strings.EqualFold(status, "pending"):
		return "Queued"
	default:
		return "Transferring"
	}
}

func transferQueueStatusColor(entry chatFileEntry) color.Color {
	status := strings.ToLower(strings.TrimSpace(entry.Status))
	switch {
	case strings.EqualFold(status, "complete"), (entry.TransferCompleted && strings.EqualFold(status, "accepted")):
		return ctpGreen
	case strings.EqualFold(status, "failed"), strings.EqualFold(status, "rejected"), strings.EqualFold(status, "canceled"):
		return ctpRed
	case strings.EqualFold(status, "pending"):
		return ctpYellow
	default:
		return ctpBlue
	}
}

type thinProgressBar struct {
	widget.BaseWidget
	progress  float32
	fillColor color.Color
}

func clampThinProgress(value float32) float32 {
	p := value
	if p < 0 {
		p = 0
	} else if p > 1 {
		p = 1
	}
	return p
}

func (b *thinProgressBar) currentFillColor() color.Color {
	if b == nil || b.fillColor == nil {
		return ctpBlue
	}
	return b.fillColor
}

type thinProgressBarRenderer struct {
	bar   *thinProgressBar
	track *canvas.Rectangle
	fill  *canvas.Rectangle
}

func (r *thinProgressBarRenderer) Layout(size fyne.Size) {
	r.track.Move(fyne.NewPos(0, 0))
	r.track.Resize(size)

	p := clampThinProgress(r.bar.progress)
	fillWidth := size.Width * p
	if p > 0 && fillWidth < 2 {
		fillWidth = 2
	}
	r.fill.Move(fyne.NewPos(0, 0))
	r.fill.Resize(fyne.NewSize(fillWidth, size.Height))
}

func (r *thinProgressBarRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, 4)
}

func (r *thinProgressBarRenderer) Refresh() {
	r.track.FillColor = ctpSurface2
	r.track.CornerRadius = 2
	r.fill.FillColor = r.bar.currentFillColor()
	r.fill.CornerRadius = 2
	r.track.Refresh()
	r.fill.Refresh()
}

func (r *thinProgressBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.fill}
}

func (r *thinProgressBarRenderer) Destroy() {}

func (b *thinProgressBar) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(ctpSurface2)
	track.CornerRadius = 2
	fill := canvas.NewRectangle(b.currentFillColor())
	fill.CornerRadius = 2
	return &thinProgressBarRenderer{
		bar:   b,
		track: track,
		fill:  fill,
	}
}

func (b *thinProgressBar) SetValue(value float32) {
	if b == nil {
		return
	}
	b.progress = clampThinProgress(value)
	b.Refresh()
}

func (b *thinProgressBar) SetFillColor(fillColor color.Color) {
	if b == nil {
		return
	}
	b.fillColor = fillColor
	b.Refresh()
}

func newThinProgressBar(progress float32, fillColor color.Color) *thinProgressBar {
	bar := &thinProgressBar{
		progress:  clampThinProgress(progress),
		fillColor: fillColor,
	}
	bar.ExtendBaseWidget(bar)
	return bar
}

func newTransferQueueProgressBar(progress float32, fillColor color.Color) fyne.CanvasObject {
	return newThinProgressBar(progress, fillColor)
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
	var popup *widget.PopUp
	closeDialog := func() {
		if popup != nil {
			popup.Hide()
		}
	}

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

	header := newPanelHeader("Application Logs", "Runtime and security event history", closeDialog, c.handleHoverHint)
	footerRight := container.New(layout.NewCustomPaddedHBoxLayout(8),
		newPanelActionButton("Close", "Close logs", panelActionSecondary, closeDialog, c.handleHoverHint),
	)
	footer := newPanelFooter(nil, footerRight)
	panel := newPanelFrame(header, footer, tabs)
	popup = newPanelPopup(c.window, panel, fyne.NewSize(620, 380))
	popup.Show()
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
			c.statusLabel.Text = message
			c.statusLabel.Refresh()
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
	peerName := strings.TrimSpace(req.PeerDeviceName)
	if peerName == "" {
		peerName = strings.TrimSpace(req.PeerDeviceID)
	}
	if peerName == "" {
		peerName = "Unknown peer"
	}

	fyne.Do(func() {
		var popup *widget.PopUp
		var respondOnce sync.Once
		respond := func(accept bool) {
			respondOnce.Do(func() {
				decision <- accept
				if popup != nil {
					popup.Hide()
				}
			})
		}

		message := widget.NewLabel(fmt.Sprintf("%s wants to add you as a peer.", peerName))
		message.Wrapping = fyne.TextWrapWord
		prompt := canvas.NewText("Accept this request?", ctpSubtext0)
		prompt.TextSize = 11
		body := container.New(layout.NewCustomPaddedVBoxLayout(8), message, prompt)
		center := container.New(layout.NewCustomPaddedLayout(12, 10, 12, 12), body)

		header := newPanelHeader("Incoming Peer Request", "Review and decide whether to connect", func() {
			respond(false)
		}, c.handleHoverHint)
		footerRight := container.New(layout.NewCustomPaddedHBoxLayout(8),
			newPanelActionButton("Reject", "Reject peer request", panelActionSecondary, func() {
				respond(false)
			}, c.handleHoverHint),
			newPanelActionButton("Accept", "Accept peer request", panelActionPrimary, func() {
				respond(true)
			}, c.handleHoverHint),
		)
		panel := newPanelFrame(header, newPanelFooter(nil, footerRight), center)
		popup = newPanelPopup(c.window, panel, fyne.NewSize(500, 220))
		popup.Show()
	})

	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case accept := <-decision:
		if accept {
			c.setStatus(fmt.Sprintf("Accepted peer request from %s", peerName))
		} else {
			c.setStatus(fmt.Sprintf("Rejected peer request from %s", peerName))
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
	path, err := c.pickSingleFilePathNative()
	if err == nil || errors.Is(err, errFilePickerCancelled) {
		return path, err
	}
	return c.pickSingleFilePathFyne()
}

func (c *controller) pickSingleFilePathNative() (path string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native file picker failed: %v", recovered)
		}
	}()

	picker := sqdialog.File().Title("Select File")
	if dir := strings.TrimSpace(c.currentDownloadDirectory()); dir != "" {
		picker = picker.SetStartDir(dir)
	}

	path, err = picker.Load()
	if err != nil {
		if errors.Is(err, sqdialog.ErrCancelled) {
			return "", errFilePickerCancelled
		}
		return "", err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return "", errFilePickerCancelled
	}
	return path, nil
}

func (c *controller) pickSingleFilePathFyne() (string, error) {
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
	path, err := c.pickFolderPathNative()
	if err == nil || errors.Is(err, errFilePickerCancelled) {
		return path, err
	}
	return c.pickFolderPathFyne()
}

func (c *controller) pickFolderPathNative() (path string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native folder picker failed: %v", recovered)
		}
	}()

	picker := sqdialog.Directory().Title("Select Folder")
	if dir := strings.TrimSpace(c.currentDownloadDirectory()); dir != "" {
		picker = picker.SetStartDir(dir)
	}

	path, err = picker.Browse()
	if err != nil {
		if errors.Is(err, sqdialog.ErrCancelled) {
			return "", errFilePickerCancelled
		}
		return "", err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return "", errFilePickerCancelled
	}
	return path, nil
}

func (c *controller) pickFolderPathFyne() (string, error) {
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

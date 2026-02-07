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
	selectedPeerID string

	discoveredMu    sync.RWMutex
	discovered      map[string]discovery.DiscoveredPeer
	discoveryRows   []discovery.DiscoveredPeer
	discoveryDialog dialog.Dialog
	discoveryList   *widget.List

	chatMu        sync.RWMutex
	chatMessages  []storage.Message
	fileTransfers map[string]chatFileEntry

	peerList        *widget.List
	chatHeader      *widget.Label
	chatKeyButton   *widget.Button
	chatMessagesBox *fyne.Container
	chatScroll      *container.Scroll
	messageInput    *widget.Entry
	statusLabel     *widget.Label

	refreshMu      sync.Mutex
	refreshRunning bool

	activeListenPort int
}

// Run starts the Phase 9 GUI.
func Run(options RunOptions) error {
	if err := options.validate(); err != nil {
		return err
	}

	ui := fyneapp.NewWithID("gosend")
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
		app:           app,
		window:        app.NewWindow("P2P Chat"),
		cfg:           options.Config,
		cfgPath:       options.ConfigPath,
		dataDir:       options.DataDir,
		store:         options.Store,
		identity:      options.Identity,
		ctx:           ctx,
		cancel:        cancel,
		discovered:    make(map[string]discovery.DiscoveredPeer),
		fileTransfers: make(map[string]chatFileEntry),
	}

	ctrl.fileHandler = NewFileHandler(ctrl.pickFilePath)
	ctrl.window.Resize(fyne.NewSize(1200, 760))

	ctrl.buildMainWindow()

	if err := ctrl.startServices(); err != nil {
		ctrl.shutdown()
		return nil, err
	}
	ctrl.startLoops()
	ctrl.refreshPeersFromStore()
	ctrl.syncDiscoveredFromScanner()
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
		Identity:      c.identity,
		Store:         c.store,
		ListenAddress: net.JoinHostPort("0.0.0.0", strconv.Itoa(requestedPort)),
		OnKeyChange:   c.promptKeyChangeDecision,
		OnMessageReceived: func(_ storage.Message) {
			c.refreshChatView()
			c.refreshPeersFromStore()
		},
		OnQueueOverflow: func(peerDeviceID string, droppedCount int) {
			c.setStatus(fmt.Sprintf("Queue overflow for %s: dropped %d pending messages", peerDeviceID, droppedCount))
			c.refreshChatForPeer(peerDeviceID)
		},
		FilesDir:       filepath.Join(c.dataDir, "files"),
		OnFileRequest:  c.promptFileRequestDecision,
		OnFileProgress: c.handleFileProgress,
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
		SelfDeviceID:   c.cfg.DeviceID,
		DeviceName:     c.cfg.DeviceName,
		ListeningPort:  activePort,
		KeyFingerprint: c.cfg.KeyFingerprint,
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
	split.Offset = 0.3

	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), c.showSettingsDialog)
	refreshBtn := widget.NewButtonWithIcon("Refresh Discovery", theme.ViewRefreshIcon(), func() {
		go c.refreshDiscovery()
	})
	toolbar := container.NewHBox(settingsBtn, refreshBtn, layout.NewSpacer())

	c.statusLabel = widget.NewLabel("Starting...")
	content := container.NewBorder(toolbar, c.statusLabel, nil, nil, split)
	c.window.SetContent(content)
}

func (c *controller) setStatus(message string) {
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
	decision := make(chan bool, 1)
	info := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("%s wants to send you a file:", notification.FromDeviceID)),
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
				decision <- accept
			},
			c.window,
		)
		dlg.Show()
	})

	select {
	case <-c.ctx.Done():
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

func (c *controller) pickFilePath() (string, error) {
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

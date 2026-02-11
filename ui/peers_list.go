package ui

import (
	"fmt"
	"image/color"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func newPeerBadge(label string, textColor, bgColor color.Color) *fyne.Container {
	text := canvas.NewText(label, textColor)
	text.TextSize = 9
	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = 2
	inner := container.New(layout.NewCustomPaddedLayout(0, 0, 4, 4), text)
	chip := container.NewStack(bg, container.NewCenter(inner))
	return container.NewGridWrap(chip.MinSize(), chip)
}

func (c *controller) buildPeersListPane() fyne.CanvasObject {
	c.peerList = widget.NewList(
		func() int {
			c.peersMu.RLock()
			defer c.peersMu.RUnlock()
			return len(c.peers)
		},
		func() fyne.CanvasObject {
			const dotSize = float32(8)
			const dotLeftInset = float32(6)
			nameIndent := dotLeftInset + dotSize + float32(theme.Padding())

			leftBar := canvas.NewRectangle(ctpSurface0)
			leftBar.SetMinSize(fyne.NewSize(2, 1))

			dot := canvas.NewCircle(colorOffline)
			dotBox := container.NewGridWrap(fyne.NewSize(dotSize, dotSize), dot)
			dotSlot := container.New(layout.NewCustomPaddedLayout(0, 0, dotLeftInset, 0), dotBox)
			dotAligned := container.NewCenter(dotSlot)

			name := widget.NewLabel("")
			name.Truncation = fyne.TextTruncateEllipsis

			// Mockup: device names non-bold; Trusted/Verified as separate colored badges
			trustedBadge := newPeerBadge("Trusted", ctpTeal, color.NRGBA{R: 148, G: 226, B: 213, A: 26})
			verifiedBadge := newPeerBadge("Verified", ctpGreen, color.NRGBA{R: 166, G: 227, B: 161, A: 26})
			badges := container.NewHBox(trustedBadge, verifiedBadge)
			badgesAligned := container.NewCenter(badges)
			row1 := container.NewBorder(nil, nil, dotAligned, badgesAligned, name)

			status := canvas.NewText("Offline", colorMuted)
			status.TextSize = 11
			statusIndent := canvas.NewRectangle(color.Transparent)
			statusIndent.SetMinSize(fyne.NewSize(nameIndent, 1))
			statusRow := container.NewHBox(statusIndent, status)
			info := container.NewVBox(row1, statusRow)

			return container.NewBorder(nil, nil, leftBar, nil, info)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			// Border stores only non-nil children; row = Border(nil, nil, leftBar, nil, info) -> 2 objects
			info := row.Objects[0].(*fyne.Container)
			leftBar := row.Objects[1].(*canvas.Rectangle)

			// info = VBox(row1, statusRow)
			row1 := info.Objects[0].(*fyne.Container)
			statusRow := info.Objects[1].(*fyne.Container)

			// row1 = Border(nil, nil, dotAligned, badgesAligned, name)
			// Fyne stores: center(name), left(dotAligned), right(badgesAligned)
			name := row1.Objects[0].(*widget.Label)
			dotAligned := row1.Objects[1].(*fyne.Container)
			badgesAligned := row1.Objects[2].(*fyne.Container)
			dotSlot := dotAligned.Objects[0].(*fyne.Container)
			badges := badgesAligned.Objects[0].(*fyne.Container)
			dotBox := dotSlot.Objects[0].(*fyne.Container)
			dot := dotBox.Objects[0].(*canvas.Circle)
			trustedBadge := badges.Objects[0].(*fyne.Container)
			verifiedBadge := badges.Objects[1].(*fyne.Container)

			// statusRow = HBox(statusIndent, status)
			status := statusRow.Objects[1].(*canvas.Text)

			peer := c.peerByIndex(int(id))
			if peer == nil {
				name.SetText("")
				status.Text = ""
				status.Refresh()
				leftBar.FillColor = ctpMantle
				leftBar.Refresh()
				trustedBadge.Hide()
				verifiedBadge.Hide()
				return
			}

			c.peersMu.RLock()
			selectedID := c.selectedPeerID
			c.peersMu.RUnlock()
			isSelected := peer.DeviceID == selectedID

			if isSelected {
				leftBar.FillColor = ctpBlue
			} else {
				leftBar.FillColor = ctpMantle
			}
			leftBar.Refresh()

			displayName := c.peerDisplayName(peer)
			if strings.TrimSpace(displayName) == "" {
				displayName = peer.DeviceID
			}
			settings := c.peerSettingsByID(peer.DeviceID)
			trusted := c.peerTrustLevel(peer.DeviceID) == storage.PeerTrustLevelTrusted
			verified := settings != nil && settings.Verified
			name.SetText(displayName)
			if trusted {
				trustedBadge.Show()
			} else {
				trustedBadge.Hide()
			}
			if verified {
				verifiedBadge.Show()
			} else {
				verifiedBadge.Hide()
			}

			runtime := c.runtimeStateForPeer(peer.DeviceID)
			stateText, dotColor := c.peerStatePresentation(peer, runtime)
			activeTransfer, transferProgressValue := c.peerTransferProgress(peer.DeviceID)
			if activeTransfer {
				stateText = fmt.Sprintf("Transferring... %.0f%%", transferProgressValue*100)
			}
			if settings != nil && strings.TrimSpace(settings.CustomName) != "" {
				secondary := strings.TrimSpace(peer.DeviceName)
				if secondary == "" {
					secondary = peer.DeviceID
				}
				stateText = fmt.Sprintf("%s • %s", secondary, stateText)
			}

			dot.FillColor = dotColor
			status.Text = stateText
			if dotColor == colorOnline {
				status.Color = ctpGreen
			} else {
				status.Color = ctpOverlay1
			}
			dot.Refresh()
			status.Refresh()
		},
	)
	c.peerList.OnSelected = func(id widget.ListItemID) {
		c.selectPeerByIndex(int(id))
	}

	heading := canvas.NewText("PEERS", ctpOverlay2)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = 12
	onlineCount := c.peersOnlineCount()
	countBadge := canvas.NewText(fmt.Sprintf("%d", onlineCount), ctpOverlay1)
	countBadge.TextSize = 12
	c.peersOnlineCountText = countBadge
	countBg := canvas.NewRectangle(ctpSurface0)
	countBg.CornerRadius = 3
	countWrap := container.NewGridWrap(
		fyne.NewSize(20, 20),
		container.NewStack(countBg, container.NewCenter(countBadge)),
	)
	refreshBtn := newCompactFlatButtonWithIcon(iconRefresh(), "Refresh discovery", func() {
		go c.refreshDiscovery()
	}, c.handleHoverHint)
	refreshBtn.iconSize = 12
	refreshBtn.padTop = 2
	refreshBtn.padBottom = 2
	refreshBtn.padLeft = 2
	refreshBtn.padRight = 2
	refreshWrap := container.NewGridWrap(fyne.NewSize(20, 20), refreshBtn)
	headerActions := container.NewHBox(refreshWrap, countWrap)
	topBar := container.NewBorder(nil, nil, container.NewCenter(heading), headerActions)
	headerBar := container.NewVBox(container.NewPadded(topBar), widget.NewSeparator())

	panelBg := canvas.NewRectangle(ctpMantle)
	panelBg.SetMinSize(fyne.NewSize(220, 1))
	panelContent := container.NewBorder(headerBar, nil, nil, nil, c.peerList)
	return container.NewStack(panelBg, panelContent)
}

func (c *controller) peersOnlineCount() int {
	c.peersMu.RLock()
	defer c.peersMu.RUnlock()
	n := 0
	for i := range c.peers {
		runtime := c.runtimeStateForPeer(c.peers[i].DeviceID)
		if runtime.ConnectionState == network.StateReady || runtime.ConnectionState == network.StateIdle {
			n++
		}
	}
	return n
}

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

	selectedID := ""
	selectedIndex := -1

	c.peersMu.Lock()
	c.peers = filtered
	c.peerSettings = settingsByPeerID
	if c.selectedPeerID != "" {
		for i := range c.peers {
			if c.peers[i].DeviceID == c.selectedPeerID {
				selectedID = c.selectedPeerID
				selectedIndex = i
				break
			}
		}
		if selectedID == "" {
			c.selectedPeerID = ""
		}
	}
	c.peersMu.Unlock()

	fyne.Do(func() {
		if c.peerList != nil {
			c.peerList.Refresh()
			if selectedIndex >= 0 {
				c.peerList.Select(selectedIndex)
			}
		}
		if c.peersOnlineCountText != nil {
			c.peersOnlineCountText.Text = fmt.Sprintf("%d", c.peersOnlineCount())
			c.peersOnlineCountText.Refresh()
		}
		c.updateChatHeader()
	})
	if selectedID == "" {
		c.refreshChatView()
	}
	c.refreshDiscoveryRows()
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

func (c *controller) showDiscoveryDialog() {
	if c.discoveryDialog != nil {
		c.discoveryDialog.Show()
		return
	}

	c.discoverySelect = ""
	c.discoveryAddBtn = nil
	c.discoveryAddBg = nil

	c.discoveryList = widget.NewList(
		func() int {
			c.discoveredMu.RLock()
			defer c.discoveredMu.RUnlock()
			return len(c.discoveryRows)
		},
		func() fyne.CanvasObject {
			rowBg := canvas.NewRectangle(color.Transparent)

			statusIcon := widget.NewIcon(theme.NewDisabledResource(iconWifiOff()))
			statusIconWrap := container.NewGridWrap(fyne.NewSize(14, 14), statusIcon)
			statusSlot := container.New(layout.NewCustomPaddedLayout(0, 0, 0, 6), statusIconWrap)

			name := widget.NewLabel("")
			name.Truncation = fyne.TextTruncateEllipsis

			addedText := canvas.NewText("added", ctpBlue)
			addedText.TextSize = 10
			addedBg := canvas.NewRectangle(color.NRGBA{R: 137, G: 180, B: 250, A: 26})
			addedBg.CornerRadius = 3
			addedInner := container.New(layout.NewCustomPaddedLayout(1, 1, 4, 4), addedText)
			addedChip := container.NewStack(addedBg, container.NewCenter(addedInner))
			addedWrap := container.NewGridWrap(addedChip.MinSize(), addedChip)
			addedSlot := container.New(layout.NewCustomPaddedLayout(0, 0, 6, 0), addedWrap)
			addedWrap.Hide()

			addrLabel := canvas.NewText("", ctpOverlay1)
			addrLabel.TextSize = 11
			addrLabel.TextStyle = fyne.TextStyle{Monospace: true}

			nameRow := container.NewBorder(nil, nil, nil, addedSlot, name)
			line1 := container.NewBorder(nil, nil, statusSlot, addrLabel, nameRow)

			status := canvas.NewText("Offline", ctpOverlay0)
			status.TextSize = 11
			line2 := container.New(layout.NewCustomPaddedLayout(0, 0, 20, 0), status)

			info := container.New(layout.NewCustomPaddedVBoxLayout(1), line1, line2)
			content := container.New(layout.NewCustomPaddedLayout(6, 6, 12, 12), info)
			row := container.NewStack(rowBg, content)
			return withBottomDivider(row)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			if len(row.Objects) < 2 {
				return
			}
			body := row.Objects[0].(*fyne.Container)
			rowBg := body.Objects[0].(*canvas.Rectangle)
			content := body.Objects[1].(*fyne.Container)
			info := content.Objects[0].(*fyne.Container)
			line1 := info.Objects[0].(*fyne.Container)
			line2 := info.Objects[1].(*fyne.Container)
			nameRow := line1.Objects[0].(*fyne.Container)
			addrLabel := line1.Objects[2].(*canvas.Text)
			statusText := line2.Objects[0].(*canvas.Text)
			statusSlot := line1.Objects[1].(*fyne.Container)
			nameLabel := nameRow.Objects[0].(*widget.Label)
			addedSlot := nameRow.Objects[1].(*fyne.Container)
			addedWrap := addedSlot.Objects[0].(*fyne.Container)
			iconWrap := statusSlot.Objects[0].(*fyne.Container)
			statusIcon := iconWrap.Objects[0].(*widget.Icon)

			peer := c.discoveryPeerByIndex(int(id))
			if peer == nil {
				nameLabel.SetText("")
				addrLabel.Text = ""
				addrLabel.Refresh()
				statusText.Text = ""
				statusText.Refresh()
				addedWrap.Hide()
				rowBg.FillColor = color.Transparent
				rowBg.Refresh()
				return
			}

			displayName := c.discoveryDisplayName(peer)
			nameLabel.SetText(displayName)
			addrPort := ""
			if len(peer.Addresses) > 0 && peer.Port > 0 {
				addrPort = net.JoinHostPort(peer.Addresses[0], strconv.Itoa(peer.Port))
			}
			addrLabel.Text = addrPort
			addrLabel.Refresh()

			online := len(peer.Addresses) > 0
			if online {
				statusIcon.SetResource(theme.NewSuccessThemedResource(iconWifi()))
				statusText.Text = "Online"
				statusText.Color = ctpGreen
			} else {
				statusIcon.SetResource(theme.NewDisabledResource(iconWifiOff()))
				statusText.Text = "Offline"
				statusText.Color = ctpOverlay0
			}
			statusIcon.Refresh()
			statusText.Refresh()

			if c.isKnownPeer(peer.DeviceID) {
				addedWrap.Show()
			} else {
				addedWrap.Hide()
			}

			if c.discoverySelect != "" && peer.DeviceID == c.discoverySelect {
				rowBg.FillColor = ctpSurface0
			} else {
				rowBg.FillColor = color.Transparent
			}
			rowBg.Refresh()
		},
	)
	c.discoveryList.OnSelected = func(id widget.ListItemID) {
		peer := c.discoveryPeerByIndex(int(id))
		if peer == nil {
			c.discoverySelect = ""
		} else {
			c.discoverySelect = peer.DeviceID
		}
		if c.discoveryList != nil {
			c.discoveryList.Refresh()
		}
		c.syncDiscoveryAddButton()
	}
	c.discoveryList.OnUnselected = func(widget.ListItemID) {
		c.discoverySelect = ""
		if c.discoveryList != nil {
			c.discoveryList.Refresh()
		}
		c.syncDiscoveryAddButton()
	}

	closeDialog := func() {
		c.closeDiscoveryDialog()
	}
	header := newPanelHeader("Discover Peers", "Peers discovered on your local network", closeDialog, c.handleHoverHint)

	refreshBtn := newCompactFlatButtonWithIconAndLabel(iconRefresh(), "Refresh", "Refresh discovered peers", func() {
		go c.refreshDiscovery()
	}, c.handleHoverHint)
	refreshBtn.labelSize = 10
	refreshBtn.iconSize = 11
	refreshBtn.padTop = 1
	refreshBtn.padBottom = 1
	refreshBtn.padLeft = 8
	refreshBtn.padRight = 8
	refreshBtn.labelColor = ctpSubtext1
	refreshBtn.hoverColor = ctpSurface1
	refreshBg := canvas.NewRectangle(ctpSurface0)
	refreshBg.CornerRadius = 4
	refreshWrap := container.NewStack(refreshBg, refreshBtn)

	c.discoveryAddBtn = newCompactFlatButtonWithIconAndLabel(iconAdd(), "Add Selected", "Add selected peer", func() {
		peer := c.discoveryPeerByDeviceID(c.discoverySelect)
		if !c.canAddDiscoveredPeer(peer) {
			return
		}
		peerCopy := *peer
		go c.addDiscoveredPeer(peerCopy)
	}, c.handleHoverHint)
	c.discoveryAddBtn.labelSize = 10
	c.discoveryAddBtn.iconSize = 11
	c.discoveryAddBtn.padTop = 1
	c.discoveryAddBtn.padBottom = 1
	c.discoveryAddBtn.padLeft = 8
	c.discoveryAddBtn.padRight = 8
	c.discoveryAddBg = canvas.NewRectangle(ctpSurface0)
	c.discoveryAddBg.CornerRadius = 4
	addWrap := container.NewStack(c.discoveryAddBg, c.discoveryAddBtn)

	footerCloseWrap := newPanelActionButton("Close", "Close discover dialog", panelActionSecondary, closeDialog, c.handleHoverHint)

	footerRight := container.New(layout.NewCustomPaddedHBoxLayout(8), addWrap, footerCloseWrap)
	footer := newPanelFooter(refreshWrap, footerRight)

	panel := newPanelFrame(header, footer, c.discoveryList)
	c.discoveryDialog = newPanelPopup(c.window, panel, fyne.NewSize(520, 460))
	c.refreshDiscoveryRows()
	c.syncDiscoveryAddButton()
	c.discoveryDialog.Show()
}

func (c *controller) closeDiscoveryDialog() {
	if c.discoveryDialog != nil {
		c.discoveryDialog.Hide()
	}
	c.discoveryDialog = nil
	c.discoveryList = nil
	c.discoverySelect = ""
	c.discoveryAddBtn = nil
	c.discoveryAddBg = nil
}

func (c *controller) refreshDiscoveryRows() {
	rows := c.listDiscoveredSnapshot()

	c.discoveredMu.Lock()
	c.discoveryRows = rows
	c.discoveredMu.Unlock()

	if c.discoverySelect != "" && c.discoveryPeerByDeviceID(c.discoverySelect) == nil {
		c.discoverySelect = ""
	}

	fyne.Do(func() {
		if c.discoveryList != nil {
			c.discoveryList.Refresh()
			if c.discoverySelect != "" {
				if idx := c.discoveryIndexByDeviceID(c.discoverySelect); idx >= 0 {
					c.discoveryList.Select(idx)
				} else {
					c.discoveryList.UnselectAll()
				}
			} else {
				c.discoveryList.UnselectAll()
			}
		}
		c.syncDiscoveryAddButton()
	})
}

func (c *controller) discoveryPeerByIndex(index int) *discovery.DiscoveredPeer {
	c.discoveredMu.RLock()
	defer c.discoveredMu.RUnlock()
	if index < 0 || index >= len(c.discoveryRows) {
		return nil
	}
	peer := c.discoveryRows[index]
	return &peer
}

func (c *controller) discoveryPeerByDeviceID(deviceID string) *discovery.DiscoveredPeer {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}

	c.discoveredMu.RLock()
	defer c.discoveredMu.RUnlock()
	for i := range c.discoveryRows {
		if c.discoveryRows[i].DeviceID == deviceID {
			peer := c.discoveryRows[i]
			return &peer
		}
	}
	return nil
}

func (c *controller) discoveryIndexByDeviceID(deviceID string) int {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return -1
	}

	c.discoveredMu.RLock()
	defer c.discoveredMu.RUnlock()
	for i := range c.discoveryRows {
		if c.discoveryRows[i].DeviceID == deviceID {
			return i
		}
	}
	return -1
}

func (c *controller) canAddDiscoveredPeer(peer *discovery.DiscoveredPeer) bool {
	if peer == nil {
		return false
	}
	if c.isKnownPeer(peer.DeviceID) {
		return false
	}
	return len(peer.Addresses) > 0
}

func (c *controller) discoveryDisplayName(peer *discovery.DiscoveredPeer) string {
	if peer == nil {
		return "Unknown peer"
	}
	name := strings.TrimSpace(peer.DeviceName)
	if name != "" && name != "..." && !strings.EqualFold(name, "unknown peer") {
		return name
	}
	if known := c.peerByID(peer.DeviceID); known != nil {
		if knownName := strings.TrimSpace(c.peerDisplayName(known)); knownName != "" {
			return knownName
		}
	}
	if name != "" {
		return name
	}
	peerID := strings.TrimSpace(peer.DeviceID)
	if peerID == "" {
		return "Unknown peer"
	}
	if len(peerID) > 18 {
		return peerID[:8] + "..." + peerID[len(peerID)-4:]
	}
	return peerID
}

func (c *controller) syncDiscoveryAddButton() {
	if c.discoveryAddBtn == nil || c.discoveryAddBg == nil {
		return
	}

	canAdd := c.canAddDiscoveredPeer(c.discoveryPeerByDeviceID(c.discoverySelect))
	c.discoveryAddBtn.enabled = canAdd
	c.discoveryAddBtn.hovered = false
	if canAdd {
		c.discoveryAddBtn.labelColor = ctpBase
		c.discoveryAddBtn.iconOn = theme.NewColoredResource(iconAdd(), theme.ColorNameBackground)
		c.discoveryAddBtn.iconOff = theme.NewDisabledResource(iconAdd())
		c.discoveryAddBtn.hoverColor = ctpLavender
		c.discoveryAddBg.FillColor = ctpBlue
	} else {
		c.discoveryAddBtn.labelColor = ctpSurface2
		c.discoveryAddBtn.iconOn = theme.NewColoredResource(iconAdd(), theme.ColorNameBackground)
		c.discoveryAddBtn.iconOff = theme.NewDisabledResource(iconAdd())
		c.discoveryAddBtn.hoverColor = ctpSurface0
		c.discoveryAddBg.FillColor = ctpSurface0
	}
	c.discoveryAddBtn.Refresh()
	c.discoveryAddBg.Refresh()
}

func (c *controller) addDiscoveredPeer(peer discovery.DiscoveredPeer) {
	if c.manager == nil {
		c.setStatus("Peer manager is unavailable")
		return
	}
	address, err := discoveredPeerAddress(peer)
	if err != nil {
		c.setStatus(err.Error())
		return
	}

	c.setStatus(fmt.Sprintf("Connecting to %s", valueOrDefault(peer.DeviceName, peer.DeviceID)))
	conn, err := c.manager.Connect(address)
	if err != nil {
		c.setStatus(fmt.Sprintf("Connect failed: %v", err))
		return
	}

	targetID := peer.DeviceID
	if conn.PeerDeviceID() != "" {
		targetID = conn.PeerDeviceID()
	}
	accepted, err := c.manager.SendPeerAddRequest(targetID)
	if err != nil {
		c.setStatus(fmt.Sprintf("Send add request failed: %v", err))
		return
	}
	if !accepted {
		c.setStatus(fmt.Sprintf("Peer request rejected by %s", valueOrDefault(peer.DeviceName, targetID)))
		return
	}

	c.refreshPeersFromStore()
	c.setStatus(fmt.Sprintf("Added peer %s", valueOrDefault(peer.DeviceName, targetID)))
}

func (c *controller) isKnownPeer(deviceID string) bool {
	if strings.TrimSpace(deviceID) == "" {
		return false
	}
	peers := c.listPeersSnapshot()
	for _, peer := range peers {
		if peer.DeviceID == deviceID {
			return true
		}
	}
	return false
}

func (c *controller) peerStatePresentation(peer *storage.Peer, runtime network.PeerRuntimeState) (string, color.Color) {
	if peer == nil {
		return "Offline", colorOffline
	}

	if runtime.Reconnecting && runtime.NextReconnectAt > 0 {
		remaining := time.Until(time.UnixMilli(runtime.NextReconnectAt))
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Reconnecting in %s...", remaining.Round(time.Second)), ctpYellow
	}

	switch runtime.ConnectionState {
	case network.StateConnecting:
		return "Connecting...", ctpYellow
	case network.StateDisconnecting:
		return "Disconnecting...", ctpPeach
	case network.StateReady, network.StateIdle:
		return "Online", colorOnline
	case network.StateDisconnected:
		if strings.EqualFold(peer.Status, "online") {
			return "Online", colorOnline
		}
		return "Offline", colorOffline
	default:
		if strings.EqualFold(peer.Status, "online") {
			return "Online", colorOnline
		}
		return "Offline", colorOffline
	}
}

func (c *controller) peerTransferProgress(peerDeviceID string) (bool, float64) {
	if strings.TrimSpace(peerDeviceID) == "" {
		return false, 0
	}

	c.chatMu.RLock()
	defer c.chatMu.RUnlock()

	activeCount := 0
	progressTotal := 0.0
	for _, transfer := range c.fileTransfers {
		if transfer.PeerDeviceID != peerDeviceID {
			continue
		}
		if transfer.TransferCompleted {
			continue
		}
		if !strings.EqualFold(transfer.Status, "accepted") {
			continue
		}
		activeCount++
		if transfer.TotalBytes > 0 {
			progressTotal += float64(transfer.BytesTransferred) / float64(transfer.TotalBytes)
		}
	}
	if activeCount == 0 {
		return false, 0
	}
	avg := progressTotal / float64(activeCount)
	if avg < 0 {
		avg = 0
	}
	if avg > 1 {
		avg = 1
	}
	return true, avg
}

func discoveredPeerAddress(peer discovery.DiscoveredPeer) (string, error) {
	if peer.Port <= 0 {
		return "", fmt.Errorf("peer %s has invalid port", peer.DeviceID)
	}
	for _, addr := range peer.Addresses {
		trimmed := strings.TrimSpace(addr)
		if trimmed == "" {
			continue
		}
		return net.JoinHostPort(trimmed, strconv.Itoa(peer.Port)), nil
	}
	return "", fmt.Errorf("peer %s has no reachable addresses", peer.DeviceID)
}
func peerStatusIndicator(status string) string {
	if strings.EqualFold(status, "online") {
		return "●"
	}
	return "○"
}

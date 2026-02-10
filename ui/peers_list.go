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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func (c *controller) buildPeersListPane() fyne.CanvasObject {
	c.peerList = widget.NewList(
		func() int {
			c.peersMu.RLock()
			defer c.peersMu.RUnlock()
			return len(c.peers)
		},
		func() fyne.CanvasObject {
			leftBar := canvas.NewRectangle(ctpSurface0)
			leftBar.SetMinSize(fyne.NewSize(2, 1))
			dot := canvas.NewCircle(colorOffline)
			dotBox := container.NewGridWrap(fyne.NewSize(8, 8), dot)
			name := widget.NewLabel("")
			name.Truncation = fyne.TextTruncateEllipsis
			// Mockup: device names non-bold; Trusted/Verified as separate colored badges
			trustedBg := canvas.NewRectangle(color.NRGBA{R: 148, G: 226, B: 213, A: 26})
			trustedBg.CornerRadius = 2
			trustedLbl := canvas.NewText("Trusted", ctpTeal)
			trustedLbl.TextSize = 10
			trustedBadge := container.NewStack(trustedBg, container.NewCenter(trustedLbl))
			verifiedBg := canvas.NewRectangle(color.NRGBA{R: 166, G: 227, B: 161, A: 26})
			verifiedBg.CornerRadius = 2
			verifiedLbl := canvas.NewText("Verified", ctpGreen)
			verifiedLbl.TextSize = 10
			verifiedBadge := container.NewStack(verifiedBg, container.NewCenter(verifiedLbl))
			// Use Border so name (center) gets remaining space; badges on the right
			badges := container.NewHBox(trustedBadge, verifiedBadge)
			row1 := container.NewBorder(nil, nil, nil, badges, name)
			status := canvas.NewText("Offline", colorMuted)
			status.TextSize = 11
			info := container.NewVBox(row1, status)
			rightPart := container.NewBorder(nil, nil, container.NewCenter(dotBox), nil, info)
			return container.NewBorder(nil, nil, leftBar, nil, rightPart)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			// Border stores only non-nil children; row = Border(nil, nil, leftBar, nil, rightPart) -> 2 objects
			rightPart := row.Objects[0].(*fyne.Container)
			leftBar := row.Objects[1].(*canvas.Rectangle)
			// rightPart = Border(nil, nil, dotCenter, nil, info)
			// Fyne stores: Objects[0]=center (info), Objects[1]=left (dotCenter)
			// rightPart = Border(nil, nil, dotCenter, nil, info)
			// Fyne stores: Objects[0]=center(info), Objects[1]=left(dotCenter)
			info := rightPart.Objects[0].(*fyne.Container)
			dotCenter := rightPart.Objects[1].(*fyne.Container)
			dotBox := dotCenter.Objects[0].(*fyne.Container)
			dot := dotBox.Objects[0].(*canvas.Circle)
			// info = VBox(row1, status)
			// row1 = Border(nil, nil, nil, badges, name)
			// Fyne stores: Objects[0]=center(name), Objects[1]=right(badges)
			row1 := info.Objects[0].(*fyne.Container)
			name := row1.Objects[0].(*widget.Label)
			badges := row1.Objects[1].(*fyne.Container)
			trustedBadge := badges.Objects[0].(*fyne.Container)
			verifiedBadge := badges.Objects[1].(*fyne.Container)
			status := info.Objects[1].(*canvas.Text)

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
	countWrap := container.NewPadded(container.NewStack(canvas.NewRectangle(ctpSurface0), container.NewCenter(countBadge)))
	topBar := container.NewBorder(nil, nil, container.NewCenter(heading), countWrap)
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

	c.discoveryList = widget.NewList(
		func() int {
			c.discoveredMu.RLock()
			defer c.discoveredMu.RUnlock()
			return len(c.discoveryRows)
		},
		func() fyne.CanvasObject {
			dot := canvas.NewCircle(colorOffline)
			dotBox := container.NewGridWrap(fyne.NewSize(8, 8), dot)
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			name.Truncation = fyne.TextTruncateEllipsis
			addrLabel := canvas.NewText("", ctpOverlay1)
			addrLabel.TextSize = 11
			status := canvas.NewText("Online", ctpGreen)
			status.TextSize = 11
			line1 := container.NewHBox(name)
			line2 := container.NewVBox(container.NewHBox(addrLabel, layout.NewSpacer(), status))
			info := container.NewVBox(line1, line2)
			addBtn := widget.NewButton("Add", nil)
			addBtn.Importance = widget.HighImportance
			return container.NewBorder(nil, nil, container.NewCenter(dotBox), addBtn, info)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			if len(row.Objects) < 3 {
				return
			}
			info := row.Objects[0].(*fyne.Container)
			line1 := info.Objects[0].(*fyne.Container)
			line2 := info.Objects[1].(*fyne.Container)
			nameLabel := line1.Objects[0].(*widget.Label)
			addrStatusRow := line2.Objects[0].(*fyne.Container)
			addrLabel := addrStatusRow.Objects[0].(*canvas.Text)
			statusText := addrStatusRow.Objects[2].(*canvas.Text)
			dotCenter := row.Objects[1].(*fyne.Container)
			dotBox := dotCenter.Objects[0].(*fyne.Container)
			dot := dotBox.Objects[0].(*canvas.Circle)
			addBtn := row.Objects[2].(*widget.Button)

			peer := c.discoveryPeerByIndex(int(id))
			if peer == nil {
				nameLabel.SetText("")
				addrLabel.Text = ""
				addrLabel.Refresh()
				statusText.Text = ""
				statusText.Refresh()
				addBtn.Disable()
				addBtn.OnTapped = nil
				return
			}

			displayName := valueOrDefault(peer.DeviceName, peer.DeviceID)
			nameLabel.SetText(displayName)
			addrPort := ""
			if len(peer.Addresses) > 0 && peer.Port > 0 {
				addrPort = net.JoinHostPort(peer.Addresses[0], strconv.Itoa(peer.Port))
			}
			addrLabel.Text = addrPort
			addrLabel.Refresh()

			if len(peer.Addresses) > 0 {
				dot.FillColor = ctpGreen
				statusText.Text = "Online"
				statusText.Color = ctpGreen
			} else {
				dot.FillColor = ctpOverlay0
				statusText.Text = "Offline"
				statusText.Color = ctpOverlay0
			}
			dot.Refresh()
			statusText.Refresh()

			if c.isKnownPeer(peer.DeviceID) {
				addBtn.SetText("added")
				addBtn.Disable()
				addBtn.OnTapped = nil
			} else {
				addBtn.SetText("Add")
				addBtn.Enable()
				peerCopy := *peer
				addBtn.OnTapped = func() {
					go c.addDiscoveredPeer(peerCopy)
				}
			}
			addBtn.Refresh()
		},
	)

	subtitle := canvas.NewText("Peers discovered on your local network", ctpOverlay1)
	subtitle.TextSize = 11
	title := canvas.NewText("Discover Peers", ctpText)
	title.TextSize = 14
	title.TextStyle = fyne.TextStyle{Bold: true}
	headerLeft := container.NewVBox(title, subtitle)
	refreshBtn := newRoundedHintButton("Refresh", iconRefresh(), "Refresh discovered peers", 4, ctpSurface0, func() {
		go c.refreshDiscovery()
	}, c.handleHoverHint)
	header := container.NewBorder(nil, nil, headerLeft, refreshBtn)
	discoveryListPanel := newRoundedBg(ctpSurface0, 4, c.discoveryList)
	content := container.NewBorder(
		container.NewVBox(container.NewPadded(header), widget.NewSeparator()),
		nil, nil, nil, container.NewPadded(discoveryListPanel),
	)

	c.discoveryDialog = dialog.NewCustom("Discover Peers", "Close", content, c.window)
	c.discoveryDialog.SetOnClosed(func() {
		c.discoveryDialog = nil
		c.discoveryList = nil
	})
	c.refreshDiscoveryRows()
	c.discoveryDialog.Resize(fyne.NewSize(760, 460))
	c.discoveryDialog.Show()
}

func (c *controller) refreshDiscoveryRows() {
	rows := c.listDiscoveredSnapshot()

	c.discoveredMu.Lock()
	c.discoveryRows = rows
	c.discoveredMu.Unlock()

	fyne.Do(func() {
		if c.discoveryList != nil {
			c.discoveryList.Refresh()
		}
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

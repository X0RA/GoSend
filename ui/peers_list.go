package ui

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gosend/discovery"
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
			dot := canvas.NewCircle(colorOffline)
			dotBox := container.NewGridWrap(fyne.NewSize(10, 10), dot)
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			name.Truncation = fyne.TextTruncateEllipsis
			status := canvas.NewText("Offline", colorMuted)
			status.TextSize = 11
			info := container.NewVBox(name, status)
			return container.NewBorder(nil, nil, container.NewCenter(dotBox), nil, info)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			// Border(nil, nil, left, nil, center): Objects = [center, left]
			info := row.Objects[0].(*fyne.Container)
			dotCenter := row.Objects[1].(*fyne.Container)
			dotBox := dotCenter.Objects[0].(*fyne.Container)
			dot := dotBox.Objects[0].(*canvas.Circle)
			name := info.Objects[0].(*widget.Label)
			status := info.Objects[1].(*canvas.Text)

			peer := c.peerByIndex(int(id))
			if peer == nil {
				name.SetText("")
				status.Text = ""
				status.Refresh()
				return
			}

			name.SetText(peer.DeviceName)
			if strings.EqualFold(peer.Status, "online") {
				dot.FillColor = colorOnline
				status.Text = "Online"
				status.Color = colorOnline
			} else {
				dot.FillColor = colorOffline
				status.Text = "Offline"
				status.Color = colorMuted
			}
			dot.Refresh()
			status.Refresh()
		},
	)
	c.peerList.OnSelected = func(id widget.ListItemID) {
		c.selectPeerByIndex(int(id))
	}

	heading := widget.NewLabel("Peers")
	heading.TextStyle = fyne.TextStyle{Bold: true}
	addBtn := newHintButtonWithIcon("", theme.ContentAddIcon(), "Discover peers", c.showDiscoveryDialog, c.handleHoverHint)
	topBar := container.NewBorder(nil, nil, heading, addBtn)

	return container.NewBorder(
		container.NewVBox(container.NewPadded(topBar), widget.NewSeparator()),
		nil, nil, nil, c.peerList,
	)
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
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].DeviceName == filtered[j].DeviceName {
			return filtered[i].DeviceID < filtered[j].DeviceID
		}
		return filtered[i].DeviceName < filtered[j].DeviceName
	})

	selectedID := ""
	selectedIndex := -1

	c.peersMu.Lock()
	c.peers = filtered
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
			dotBox := container.NewGridWrap(fyne.NewSize(10, 10), dot)
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			name.Truncation = fyne.TextTruncateEllipsis
			status := canvas.NewText("Online", colorMuted)
			status.TextSize = 11
			info := container.NewVBox(name, status)
			addBtn := newRoundedButton("Add", nil, 10, colorButtonFill, colorButtonMuted, nil)
			// Border(nil, nil, left, right, center): Objects = [center, left, right]
			return container.NewBorder(nil, nil, container.NewCenter(dotBox), addBtn, info)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			if len(row.Objects) < 3 {
				return
			}

			// Border(nil, nil, left, right, center): Objects = [center, left, right]
			info := row.Objects[0].(*fyne.Container)
			nameLabel := info.Objects[0].(*widget.Label)
			statusText := info.Objects[1].(*canvas.Text)
			dotCenter := row.Objects[1].(*fyne.Container)
			dotBox := dotCenter.Objects[0].(*fyne.Container)
			dot := dotBox.Objects[0].(*canvas.Circle)
			addBtn := row.Objects[2].(*roundedButton)

			peer := c.discoveryPeerByIndex(int(id))
			if peer == nil {
				nameLabel.SetText("")
				statusText.Text = ""
				statusText.Refresh()
				addBtn.Disable()
				addBtn.SetOnTapped(nil)
				return
			}

			nameLabel.SetText(valueOrDefault(peer.DeviceName, peer.DeviceID))
			if len(peer.Addresses) > 0 {
				dot.FillColor = colorOnline
				statusText.Text = "Online"
				statusText.Color = colorOnline
			} else {
				dot.FillColor = colorOffline
				statusText.Text = "Offline"
				statusText.Color = colorMuted
			}
			dot.Refresh()
			statusText.Refresh()

			if c.isKnownPeer(peer.DeviceID) {
				addBtn.SetText("Added")
				addBtn.Disable()
				addBtn.SetOnTapped(nil)
			} else {
				addBtn.SetText("Add")
				addBtn.Enable()
				peerCopy := *peer
				addBtn.SetOnTapped(func() {
					go c.addDiscoveredPeer(peerCopy)
				})
			}
			addBtn.Refresh()
		},
	)

	subtitle := widget.NewLabel("Peers discovered on your local network")
	subtitle.Importance = widget.LowImportance
	refreshBtn := newRoundedHintButton("Refresh", theme.ViewRefreshIcon(), "Refresh discovered peers", 10, colorButtonFill, func() {
		go c.refreshDiscovery()
	}, c.handleHoverHint)
	header := container.NewBorder(nil, nil, subtitle, refreshBtn)
	discoveryListPanel := newRoundedBg(colorDialogPanel, 10, c.discoveryList)
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


package ui

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			peer := c.peerByIndex(int(id))
			if peer == nil {
				label.SetText("")
				return
			}
			label.SetText(fmt.Sprintf("%s (%s)", peer.DeviceName, peerStatusIndicator(peer.Status)))
		},
	)
	c.peerList.OnSelected = func(id widget.ListItemID) {
		c.selectPeerByIndex(int(id))
	}

	heading := widget.NewLabel("Peers")
	addBtn := widget.NewButtonWithIcon("Add Peer", theme.ContentAddIcon(), c.showDiscoveryDialog)

	return container.NewBorder(heading, addBtn, nil, nil, c.peerList)
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
			name := widget.NewLabel("")
			status := widget.NewLabel("")
			addBtn := widget.NewButton("Add", nil)
			return container.NewHBox(name, layout.NewSpacer(), status, addBtn)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*fyne.Container)
			if len(row.Objects) != 4 {
				return
			}

			nameLabel, _ := row.Objects[0].(*widget.Label)
			statusLabel, _ := row.Objects[2].(*widget.Label)
			addBtn, _ := row.Objects[3].(*widget.Button)
			if nameLabel == nil || statusLabel == nil || addBtn == nil {
				return
			}

			peer := c.discoveryPeerByIndex(int(id))
			if peer == nil {
				nameLabel.SetText("")
				statusLabel.SetText("")
				addBtn.Disable()
				return
			}

			nameLabel.SetText(valueOrDefault(peer.DeviceName, peer.DeviceID))
			if len(peer.Addresses) > 0 {
				statusLabel.SetText("Online")
			} else {
				statusLabel.SetText("Offline")
			}

			if c.isKnownPeer(peer.DeviceID) {
				addBtn.SetText("Added")
				addBtn.Disable()
				addBtn.OnTapped = nil
				return
			}

			addBtn.SetText("Add")
			addBtn.Enable()
			peerCopy := *peer
			addBtn.OnTapped = func() {
				go c.addDiscoveredPeer(peerCopy)
			}
		},
	)

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		go c.refreshDiscovery()
	})
	actions := container.NewHBox(layout.NewSpacer(), refreshBtn)
	content := container.NewBorder(nil, actions, nil, nil, c.discoveryList)

	c.discoveryDialog = dialog.NewCustom("Available Peers", "Close", content, c.window)
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

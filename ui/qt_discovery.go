package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therecipe/qt/widgets"

	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func (c *controller) showDiscoveryDialog() {
	c.enqueueUI(func() {
		if c.discoveryDialog != nil {
			c.discoveryDialog.Show()
			return
		}
		dlg := widgets.NewQDialog(c.window, 0)
		dlg.SetWindowTitle("Discover Peers")
		dlg.Resize2(520, 460)

		layout := widgets.NewQVBoxLayout()
		layout.SetContentsMargins(0, 0, 0, 0)
		layout.SetSpacing(0)

		// ── Header bar ──────────────────────────────────────
		discHeader := widgets.NewQWidget(nil, 0)
		discHeader.SetObjectName("dialogHeader")
		discHdrLayout := widgets.NewQHBoxLayout()
		discHdrLayout.SetContentsMargins(16, 10, 16, 10)
		discHdrInfo := widgets.NewQVBoxLayout()
		discHdrInfo.SetContentsMargins(0, 0, 0, 0)
		discHdrInfo.SetSpacing(2)
		discTitle := widgets.NewQLabel2("Discover Peers", nil, 0)
		discTitle.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; font-weight: bold; background: transparent;", colorText))
		discSubtitle := widgets.NewQLabel2("Peers discovered on your local network", nil, 0)
		discSubtitle.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))
		discHdrInfo.AddWidget(discTitle, 0, 0)
		discHdrInfo.AddWidget(discSubtitle, 0, 0)
		discHdrLayout.AddLayout(discHdrInfo, 1)
		discHeader.SetLayout(discHdrLayout)
		layout.AddWidget(discHeader, 0, 0)

		list := widgets.NewQListWidget(nil)
		list.SetObjectName("chatList")
		refreshBtn := widgets.NewQPushButton2("↻ Refresh", nil)
		refreshBtn.SetObjectName("secondaryBtn")
		addBtn := widgets.NewQPushButton2("+ Add Selected", nil)
		addBtn.SetObjectName("primaryBtn")
		closeBtn := widgets.NewQPushButton2("Close", nil)
		closeBtn.SetObjectName("secondaryBtn")

		render := func() {
			list.Clear()
			rows := c.listDiscoveredSnapshot()
			for _, peer := range rows {
				status := "offline"
				if len(peer.Addresses) > 0 {
					status = "online"
				}
				known := ""
				if c.isKnownPeer(peer.DeviceID) {
					known = " [added]"
				}
				text := fmt.Sprintf("%s%s\n%s:%d | %s", valueOrDefault(peer.DeviceName, peer.DeviceID), known, strings.Join(peer.Addresses, ","), peer.Port, status)
				_ = widgets.NewQListWidgetItem2(text, list, 0)
			}
		}

		refreshBtn.ConnectClicked(func(bool) {
			go func() {
				c.refreshDiscovery()
				c.enqueueUI(render)
			}()
		})
		addBtn.ConnectClicked(func(bool) {
			idx := list.CurrentRow()
			rows := c.listDiscoveredSnapshot()
			if idx < 0 || idx >= len(rows) {
				return
			}
			peer := rows[idx]
			go func() {
				c.addDiscoveredPeer(peer)
				c.enqueueUI(render)
			}()
		})
		closeBtn.ConnectClicked(func(bool) { dlg.Accept() })
		c.hold(refreshBtn, addBtn, closeBtn)

		// ── Footer bar ──────────────────────────────────────
		discFooter := widgets.NewQWidget(nil, 0)
		discFooter.SetObjectName("dialogFooter")
		btnLayout := widgets.NewQHBoxLayout()
		btnLayout.SetContentsMargins(16, 10, 16, 10)
		btnLayout.SetSpacing(8)
		btnLayout.AddWidget(refreshBtn, 0, 0)
		btnLayout.AddStretch(1)
		btnLayout.AddWidget(addBtn, 0, 0)
		btnLayout.AddWidget(closeBtn, 0, 0)
		discFooter.SetLayout(btnLayout)

		layout.AddWidget(list, 1, 0)
		layout.AddWidget(discFooter, 0, 0)
		dlg.SetLayout(layout)

		dlg.ConnectFinished(func(int) {
			c.discoveryDialog = nil
			c.discoveryList = nil
		})
		c.hold(dlg)

		c.discoveryDialog = dlg
		c.discoveryList = list
		render()
		dlg.Show()
	})
}

func (c *controller) refreshDiscoveryRows() {
	c.enqueueUI(func() {
		if c.discoveryList == nil {
			return
		}
		rows := c.listDiscoveredSnapshot()
		c.discoveryList.Clear()
		for _, peer := range rows {
			status := "offline"
			if len(peer.Addresses) > 0 {
				status = "online"
			}
			known := ""
			if c.isKnownPeer(peer.DeviceID) {
				known = " [added]"
			}
			text := fmt.Sprintf("%s%s\n%s:%d | %s", valueOrDefault(peer.DeviceName, peer.DeviceID), known, strings.Join(peer.Addresses, ","), peer.Port, status)
			_ = widgets.NewQListWidgetItem2(text, c.discoveryList, 0)
		}
	})
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

func (c *controller) peerStatePresentation(peer *storage.Peer, runtime network.PeerRuntimeState) string {
	if peer == nil {
		return "Offline"
	}
	if runtime.Reconnecting && runtime.NextReconnectAt > 0 {
		remaining := time.Until(time.UnixMilli(runtime.NextReconnectAt))
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Reconnecting in %s", remaining.Round(time.Second))
	}

	switch runtime.ConnectionState {
	case network.StateConnecting:
		return "Connecting..."
	case network.StateDisconnecting:
		return "Disconnecting..."
	case network.StateReady, network.StateIdle:
		return "Online"
	case network.StateDisconnected:
		if strings.EqualFold(peer.Status, "online") {
			return "Online"
		}
		return "Offline"
	default:
		if strings.EqualFold(peer.Status, "online") {
			return "Online"
		}
		return "Offline"
	}
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
	known := make([]discovery.KnownPeerIdentity, 0, len(peers)+1)
	for _, peer := range peers {
		deviceID := strings.TrimSpace(peer.DeviceID)
		publicKey := strings.TrimSpace(peer.Ed25519PublicKey)
		if deviceID == "" || publicKey == "" {
			continue
		}
		known = append(known, discovery.KnownPeerIdentity{DeviceID: deviceID, Ed25519PublicKey: publicKey})
	}
	known = append(known, discovery.KnownPeerIdentity{DeviceID: c.identity.DeviceID, Ed25519PublicKey: base64.StdEncoding.EncodeToString(c.identity.Ed25519PublicKey)})
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
	for _, peer := range discovered {
		c.tryReconnectDiscoveredPeer(peer)
	}
}

func (c *controller) promptIncomingPeerAddRequest(req network.AddRequestNotification) (bool, error) {
	result := make(chan bool, 1)
	err := c.runOnUI(func() {
		peerName := strings.TrimSpace(req.PeerDeviceName)
		if peerName == "" {
			peerName = req.PeerDeviceID
		}
		answer := widgets.QMessageBox_Question(c.window, "Incoming Peer Request", fmt.Sprintf("%s wants to add you as a peer. Accept?", peerName), widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)
		result <- answer == widgets.QMessageBox__Yes
	})
	if err != nil {
		return false, err
	}
	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case accept := <-result:
		if accept {
			c.setStatus(fmt.Sprintf("Accepted peer request from %s", req.PeerDeviceName))
		} else {
			c.setStatus(fmt.Sprintf("Rejected peer request from %s", req.PeerDeviceName))
		}
		return accept, nil
	}
}

func (c *controller) promptFileRequestDecision(notification network.FileRequestNotification) (bool, error) {
	c.maybeNotifyIncomingFileRequest(notification)

	result := make(chan bool, 1)
	err := c.runOnUI(func() {
		message := fmt.Sprintf("%s wants to send you a file:\n\nName: %s\nSize: %s\nType: %s\n\nAccept this file?", c.transferPeerName(notification.FromDeviceID), notification.Filename, formatBytes(notification.Filesize), valueOrDefault(notification.Filetype, "unknown"))
		answer := widgets.QMessageBox_Question(c.window, "Incoming File", message, widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__Yes)
		result <- answer == widgets.QMessageBox__Yes
	})
	if err != nil {
		return false, err
	}
	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case accept := <-result:
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
			UpdatedAt:    time.Now().UnixMilli(),
		})
		c.refreshChatForPeer(notification.FromDeviceID)
		return accept, nil
	}
}

func (c *controller) promptKeyChangeDecision(peerDeviceID, existingPublicKeyBase64, receivedPublicKeyBase64 string) (bool, error) {
	result := make(chan bool, 1)
	oldFingerprint := fingerprintFromBase64(existingPublicKeyBase64)
	newFingerprint := fingerprintFromBase64(receivedPublicKeyBase64)
	peerName := peerDeviceID
	if peer := c.peerByID(peerDeviceID); peer != nil && strings.TrimSpace(peer.DeviceName) != "" {
		peerName = peer.DeviceName
	}

	err := c.runOnUI(func() {
		message := fmt.Sprintf("The identity key for %s has changed.\n\nOld fingerprint: %s\nNew fingerprint: %s\n\nTrust the new key?", peerName, oldFingerprint, newFingerprint)
		answer := widgets.QMessageBox_Question(c.window, "Key Change Warning", message, widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)
		result <- answer == widgets.QMessageBox__Yes
	})
	if err != nil {
		return false, err
	}
	select {
	case <-c.ctx.Done():
		return false, errors.New("application is shutting down")
	case trust := <-result:
		if trust {
			c.setStatus(fmt.Sprintf("Trusted new key for %s", peerName))
		} else {
			c.setStatus(fmt.Sprintf("Rejected key change for %s", peerName))
		}
		return trust, nil
	}
}

package ui

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therecipe/qt/widgets"

	appcrypto "gosend/crypto"
	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func (c *controller) handleIncomingMessage(message storage.Message) {
	selectedPeerID := c.currentSelectedPeerID()
	if message.FromDeviceID == selectedPeerID || message.ToDeviceID == selectedPeerID {
		c.refreshChatForPeer(selectedPeerID)
	}
	c.maybeNotifyIncomingMessage(message)
	peerName := c.transferPeerName(message.FromDeviceID)
	if strings.TrimSpace(message.Content) != "" {
		c.setStatus(fmt.Sprintf("New message from %s", peerName))
	}
}

func (c *controller) handlePeerRuntimeStateChanged(state network.PeerRuntimeState) {
	c.runtimeMu.Lock()
	c.peerRuntimeStates[state.PeerDeviceID] = state
	c.runtimeMu.Unlock()
	c.enqueueUI(func() {
		if c.peerList != nil {
			c.renderPeerList(-1)
		}
	})
}

func (c *controller) maybeNotifyIncomingMessage(message storage.Message) {
	peerID := strings.TrimSpace(message.FromDeviceID)
	if peerID == "" || !c.shouldSendNotification(peerID) {
		return
	}
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = "New message"
	}
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	c.sendDesktopNotification("GoSend · "+c.transferPeerName(peerID), content)
}

func (c *controller) maybeNotifyIncomingFileRequest(notification network.FileRequestNotification) {
	peerID := strings.TrimSpace(notification.FromDeviceID)
	if peerID == "" || !c.shouldSendNotification(peerID) {
		return
	}
	content := fmt.Sprintf("%s (%s)", valueOrDefault(notification.Filename, notification.FileID), formatBytes(notification.Filesize))
	c.sendDesktopNotification("Incoming file from "+c.transferPeerName(peerID), content)
}

func (c *controller) shouldSendNotification(peerID string) bool {
	if c.cfg == nil || !c.cfg.NotificationsEnabled {
		return false
	}
	settings := c.peerSettingsByID(peerID)
	if settings != nil && settings.NotificationsMuted {
		return false
	}
	if selected := c.currentSelectedPeerID(); selected == peerID {
		return false
	}
	return true
}

func (c *controller) sendDesktopNotification(title, content string) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" {
		return
	}
	c.enqueueUI(func() {
		if c.trayIcon == nil {
			return
		}
		c.trayIcon.ShowMessage(title, content, widgets.QSystemTrayIcon__Information, 5000)
	})
}

func (c *controller) runtimeStateForPeer(peerID string) network.PeerRuntimeState {
	c.runtimeMu.RLock()
	defer c.runtimeMu.RUnlock()
	state, ok := c.peerRuntimeStates[peerID]
	if !ok {
		return c.manager.PeerRuntimeStateSnapshot(peerID)
	}
	return state
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
	if settings == nil || settings.TrustLevel == "" {
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
	_, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, fmt.Errorf("parse listen address %q: %w", addr.String(), err)
	}
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

func formatTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return time.Now().Format("3:04 PM")
	}
	return time.UnixMilli(timestamp).Format("3:04 PM")
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

func fileTransferStatusText(file chatFileEntry) string {
	switch strings.ToLower(file.Status) {
	case "rejected", "failed":
		return "Failed"
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

func openContainingFolder(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("file path is required")
	}
	target := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		target = filepath.Dir(path)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("explorer", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
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

func selectMaxSizePreset(size int64, allowGlobalDefault bool) string {
	if allowGlobalDefault && size == 0 {
		return maxSizePresetUseGlobal
	}
	if !allowGlobalDefault && size == 0 {
		return maxSizePresetUnlimited
	}
	for label, value := range maxSizePresetValues {
		if value == size {
			return label
		}
	}
	return maxSizePresetCustom
}

func parseMaxSizeSelection(selected, customText string, allowGlobalDefault bool) (int64, error) {
	switch selected {
	case maxSizePresetUseGlobal:
		if !allowGlobalDefault {
			return 0, fmt.Errorf("invalid max file size selection")
		}
		return 0, nil
	case maxSizePresetUnlimited:
		if allowGlobalDefault {
			return 0, fmt.Errorf("invalid max file size selection")
		}
		return 0, nil
	case maxSizePresetCustom:
		value, err := strconv.ParseInt(strings.TrimSpace(customText), 10, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("custom max file size must be a non-negative byte value")
		}
		if allowGlobalDefault && value == 0 {
			return 0, fmt.Errorf("custom max file size must be greater than 0, or use global default")
		}
		return value, nil
	default:
		value, ok := maxSizePresetValues[selected]
		if !ok {
			return 0, fmt.Errorf("invalid max file size selection")
		}
		return value, nil
	}
}

func selectRetentionPreset(days int) string {
	switch days {
	case 30:
		return retentionPreset30Days
	case 90:
		return retentionPreset90Days
	case 365:
		return retentionPreset1Year
	default:
		return retentionPresetKeepForever
	}
}

func parseRetentionSelection(selected string) (int, error) {
	days, ok := messageRetentionValues[selected]
	if !ok {
		return 0, fmt.Errorf("invalid retention selection")
	}
	return days, nil
}

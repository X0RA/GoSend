package ui

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"gosend/config"
	appcrypto "gosend/crypto"
	"gosend/storage"
)

const (
	maxSizePresetUseGlobal = "Use global default"
	maxSizePresetUnlimited = "Unlimited"
	maxSizePreset100MB     = "100 MB"
	maxSizePreset500MB     = "500 MB"
	maxSizePreset1GB       = "1 GB"
	maxSizePreset5GB       = "5 GB"
	maxSizePresetCustom    = "Custom"

	retentionPresetKeepForever = "Keep forever"
	retentionPreset30Days      = "30 days"
	retentionPreset90Days      = "90 days"
	retentionPreset1Year       = "1 year"
)

var maxSizePresetValues = map[string]int64{
	maxSizePreset100MB: 100 * 1024 * 1024,
	maxSizePreset500MB: 500 * 1024 * 1024,
	maxSizePreset1GB:   1024 * 1024 * 1024,
	maxSizePreset5GB:   5 * 1024 * 1024 * 1024,
}

var messageRetentionValues = map[string]int{
	retentionPresetKeepForever: 0,
	retentionPreset30Days:      30,
	retentionPreset90Days:      90,
	retentionPreset1Year:       365,
}

func (c *controller) showSettingsDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(c.cfg.DeviceName)

	portEntry := widget.NewEntry()
	if c.cfg.ListeningPort > 0 {
		portEntry.SetText(strconv.Itoa(c.cfg.ListeningPort))
	}

	portModeOptions := []string{"Automatic", "Fixed"}
	portModeGroup := widget.NewRadioGroup(portModeOptions, func(selected string) {
		if selected == "Fixed" {
			portEntry.Enable()
			return
		}
		portEntry.Disable()
	})
	switch c.cfg.PortMode {
	case config.PortModeFixed:
		portModeGroup.SetSelected("Fixed")
	default:
		portModeGroup.SetSelected("Automatic")
	}
	if portModeGroup.Selected == "Fixed" {
		portEntry.Enable()
	} else {
		portEntry.Disable()
	}

	downloadDirEntry := widget.NewEntry()
	downloadDirEntry.SetText(c.currentDownloadDirectory())
	browseDownloadDirBtn := widget.NewButton("Browse...", func() {
		go func() {
			path, err := c.pickFolderPath()
			if err != nil {
				if err != errFilePickerCancelled {
					c.setStatus(fmt.Sprintf("Select download folder failed: %v", err))
				}
				return
			}
			fyne.Do(func() {
				downloadDirEntry.SetText(path)
			})
		}()
	})

	maxSizeSelect := widget.NewSelect([]string{
		maxSizePresetUnlimited,
		maxSizePreset100MB,
		maxSizePreset500MB,
		maxSizePreset1GB,
		maxSizePreset5GB,
		maxSizePresetCustom,
	}, nil)
	maxSizeCustomEntry := widget.NewEntry()
	maxSizeCustomEntry.SetPlaceHolder("Bytes")
	maxSizeSelect.OnChanged = func(selected string) {
		if selected == maxSizePresetCustom {
			maxSizeCustomEntry.Enable()
			return
		}
		maxSizeCustomEntry.Disable()
		if selected == maxSizePresetUnlimited {
			maxSizeCustomEntry.SetText("0")
			return
		}
		if presetBytes, ok := maxSizePresetValues[selected]; ok {
			maxSizeCustomEntry.SetText(strconv.FormatInt(presetBytes, 10))
		}
	}
	maxSizeSelect.SetSelected(selectMaxSizePreset(c.cfg.MaxReceiveFileSize, false))
	if maxSizeSelect.Selected == maxSizePresetCustom {
		maxSizeCustomEntry.SetText(strconv.FormatInt(c.cfg.MaxReceiveFileSize, 10))
		maxSizeCustomEntry.Enable()
	}

	notificationsEnabled := widget.NewCheck("", nil)
	notificationsEnabled.SetChecked(c.cfg.NotificationsEnabled)

	retentionSelect := widget.NewSelect([]string{
		retentionPresetKeepForever,
		retentionPreset30Days,
		retentionPreset90Days,
		retentionPreset1Year,
	}, nil)
	retentionSelect.SetSelected(selectRetentionPreset(c.cfg.MessageRetentionDays))

	cleanupDownloadedFiles := widget.NewCheck("Delete downloaded files when purging old metadata", nil)
	cleanupDownloadedFiles.SetChecked(c.cfg.CleanupDownloadedFiles)

	fingerprintLabel := widget.NewLabel(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint))
	fingerprintLabel.Wrapping = fyne.TextWrapBreak
	copyFingerprintBtn := widget.NewButtonWithIcon("", iconContentCopy(), func() {
		if c.window != nil && c.window.Clipboard() != nil {
			c.window.Clipboard().SetContent(fingerprintLabel.Text)
		}
		c.setStatus("Fingerprint copied")
	})
	deviceIDLabel := widget.NewLabel(c.cfg.DeviceID)
	deviceIDLabel.Wrapping = fyne.TextWrapBreak

	resetKeysBtn := widget.NewButton("Reset Keys", func() {
		c.confirmResetKeys(fingerprintLabel)
	})
	resetKeysBtn.Importance = widget.DangerImportance

	maxSizeHint := widget.NewLabel("Set to Unlimited or 0 for no size limit on incoming files.")
	maxSizeHint.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(
		widget.NewFormItem("Device Name", nameEntry),
		widget.NewFormItem("Device ID", deviceIDLabel),
		widget.NewFormItem("Fingerprint", container.NewBorder(nil, nil, nil, copyFingerprintBtn, fingerprintLabel)),
		widget.NewFormItem("Port Mode", portModeGroup),
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("Download Location", container.NewBorder(nil, nil, nil, browseDownloadDirBtn, downloadDirEntry)),
		widget.NewFormItem("Max File Size", container.NewVBox(container.NewHBox(maxSizeSelect, maxSizeCustomEntry), maxSizeHint)),
		widget.NewFormItem("Notifications", notificationsEnabled),
		widget.NewFormItem("Message Retention", retentionSelect),
		widget.NewFormItem("File Cleanup", cleanupDownloadedFiles),
	)

	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		resetKeysBtn,
	)

	var popup *widget.PopUp
	closeDialog := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	saveSettings := func() {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			dialog.ShowError(fmt.Errorf("device name is required"), c.window)
			return
		}

		portMode := config.PortModeAutomatic
		if portModeGroup.Selected == "Fixed" {
			portMode = config.PortModeFixed
		}

		port := 0
		if portMode == config.PortModeFixed {
			parsed, err := strconv.Atoi(strings.TrimSpace(portEntry.Text))
			if err != nil || parsed <= 0 {
				dialog.ShowError(fmt.Errorf("port must be a positive number when mode is Fixed"), c.window)
				return
			}
			port = parsed
		}

		downloadDir := strings.TrimSpace(downloadDirEntry.Text)
		if downloadDir == "" {
			dialog.ShowError(fmt.Errorf("download location is required"), c.window)
			return
		}
		if err := os.MkdirAll(downloadDir, 0o700); err != nil {
			dialog.ShowError(fmt.Errorf("create download location: %w", err), c.window)
			return
		}

		maxReceiveFileSize, err := parseMaxSizeSelection(maxSizeSelect.Selected, maxSizeCustomEntry.Text, false)
		if err != nil {
			dialog.ShowError(err, c.window)
			return
		}
		retentionDays, err := parseRetentionSelection(retentionSelect.Selected)
		if err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		nameChanged := c.cfg.DeviceName != name
		portChanged := c.cfg.PortMode != portMode || c.cfg.ListeningPort != port
		changed := nameChanged ||
			portChanged ||
			c.cfg.DownloadDirectory != downloadDir ||
			c.cfg.MaxReceiveFileSize != maxReceiveFileSize ||
			c.cfg.NotificationsEnabled != notificationsEnabled.Checked ||
			c.cfg.MessageRetentionDays != retentionDays ||
			c.cfg.CleanupDownloadedFiles != cleanupDownloadedFiles.Checked
		if !changed {
			c.setStatus("Settings saved")
			closeDialog()
			return
		}

		c.cfg.DeviceName = name
		c.cfg.PortMode = portMode
		c.cfg.ListeningPort = port
		c.cfg.DownloadDirectory = downloadDir
		c.cfg.MaxReceiveFileSize = maxReceiveFileSize
		c.cfg.NotificationsEnabled = notificationsEnabled.Checked
		c.cfg.MessageRetentionDays = retentionDays
		c.cfg.CleanupDownloadedFiles = cleanupDownloadedFiles.Checked

		if err := config.Save(c.cfgPath, c.cfg); err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		if nameChanged && !portChanged {
			if c.manager != nil {
				if err := c.manager.UpdateDeviceName(name); err != nil {
					c.setStatus(fmt.Sprintf("Device name updated in config; runtime update failed: %v", err))
					closeDialog()
					dialog.ShowInformation("Restart Recommended", "Device name was saved, but runtime update failed. Restart the app to fully apply it.", c.window)
					return
				}
			}
			if c.discovery != nil {
				if err := c.discovery.UpdateDeviceName(name); err != nil {
					c.setStatus(fmt.Sprintf("Device name updated in config; mDNS update failed: %v", err))
					closeDialog()
					dialog.ShowInformation("Restart Recommended", "Device name was saved, but discovery update failed. Restart the app to fully apply it.", c.window)
					return
				}
			}
			c.identity.DeviceName = name
			c.setStatus("Settings saved")
			closeDialog()
			return
		}

		if portChanged {
			c.setStatus("Settings saved. Restart required to apply port changes.")
			closeDialog()
			dialog.ShowInformation("Restart Required", "Settings were saved. Restart the app to apply port changes.", c.window)
			return
		}

		c.setStatus("Settings saved")
		closeDialog()
	}

	header := newPanelHeader("Device Settings", "Local device and transfer preferences", closeDialog, c.handleHoverHint)
	footerRight := container.NewHBox(
		newPanelActionButton("Close", "Close device settings", panelActionSecondary, closeDialog, c.handleHoverHint),
		newPanelActionButton("Save", "Save device settings", panelActionPrimary, saveSettings, c.handleHoverHint),
	)
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(640, 460))
	panel := newPanelFrame(header, newPanelFooter(nil, footerRight), scroll)
	popup = newPanelPopup(c.window, panel, fyne.NewSize(660, 560))
	popup.Show()
}

func (c *controller) showSelectedPeerSettingsDialog() {
	peerID := c.currentSelectedPeerID()
	if peerID == "" {
		c.setStatus("Select a peer first")
		return
	}

	peer := c.peerByID(peerID)
	if peer == nil {
		c.setStatus("Selected peer no longer exists")
		return
	}

	if err := c.store.EnsurePeerSettingsExist(peerID); err != nil {
		dialog.ShowError(fmt.Errorf("load peer settings: %w", err), c.window)
		return
	}
	settings, err := c.store.GetPeerSettings(peerID)
	if err != nil {
		dialog.ShowError(fmt.Errorf("load peer settings: %w", err), c.window)
		return
	}

	customNameEntry := widget.NewEntry()
	customNameEntry.SetText(settings.CustomName)

	autoAcceptCheck := widget.NewCheck("", nil)
	autoAcceptCheck.SetChecked(settings.AutoAcceptFiles)

	trustLevel := widget.NewRadioGroup([]string{"Normal", "Trusted"}, nil)
	if settings.TrustLevel == storage.PeerTrustLevelTrusted {
		trustLevel.SetSelected("Trusted")
	} else {
		trustLevel.SetSelected("Normal")
	}
	trustLevelHint := widget.NewLabel("Trust level is informational metadata shown as a badge; it does not change transfer or security behavior.")
	trustLevelHint.Wrapping = fyne.TextWrapWord

	notificationsMutedCheck := widget.NewCheck("", nil)
	notificationsMutedCheck.SetChecked(settings.NotificationsMuted)

	verifiedCheck := widget.NewCheck("Verified out-of-band", nil)
	verifiedCheck.SetChecked(settings.Verified)
	verifiedHint := widget.NewLabel("You manually compared this peer's fingerprint with the real device over a separate trusted channel (e.g. in person).")
	verifiedHint.Wrapping = fyne.TextWrapWord

	maxSizeSelect := widget.NewSelect([]string{
		maxSizePresetUseGlobal,
		maxSizePreset100MB,
		maxSizePreset500MB,
		maxSizePreset1GB,
		maxSizePreset5GB,
		maxSizePresetCustom,
	}, nil)
	maxSizeCustomEntry := widget.NewEntry()
	maxSizeCustomEntry.SetPlaceHolder("Bytes")
	maxSizeSelect.OnChanged = func(selected string) {
		if selected == maxSizePresetCustom {
			maxSizeCustomEntry.Enable()
			return
		}
		maxSizeCustomEntry.Disable()
		if selected == maxSizePresetUseGlobal {
			maxSizeCustomEntry.SetText("0")
			return
		}
		if presetBytes, ok := maxSizePresetValues[selected]; ok {
			maxSizeCustomEntry.SetText(strconv.FormatInt(presetBytes, 10))
		}
	}
	maxSizeSelect.SetSelected(selectMaxSizePreset(settings.MaxFileSize, true))
	if maxSizeSelect.Selected == maxSizePresetCustom {
		maxSizeCustomEntry.SetText(strconv.FormatInt(settings.MaxFileSize, 10))
		maxSizeCustomEntry.Enable()
	}
	maxSizePeerHint := widget.NewLabel("0 or \"Use global default\" means use the device setting; it does not mean unlimited for this peer.")
	maxSizePeerHint.Wrapping = fyne.TextWrapWord

	useGlobalDownloadCheck := widget.NewCheck("Use global download directory", nil)
	downloadDirectoryEntry := widget.NewEntry()
	downloadDirectoryEntry.SetText(settings.DownloadDirectory)
	browseDownloadDirectoryBtn := widget.NewButton("Browse...", func() {
		go func() {
			path, err := c.pickFolderPath()
			if err != nil {
				if err != errFilePickerCancelled {
					c.setStatus(fmt.Sprintf("Select peer download folder failed: %v", err))
				}
				return
			}
			fyne.Do(func() {
				downloadDirectoryEntry.SetText(path)
			})
		}()
	})

	setDownloadControlsEnabled := func(enabled bool) {
		if enabled {
			downloadDirectoryEntry.Enable()
			browseDownloadDirectoryBtn.Enable()
			return
		}
		downloadDirectoryEntry.Disable()
		browseDownloadDirectoryBtn.Disable()
	}
	useGlobalDownloadCheck.OnChanged = func(checked bool) {
		setDownloadControlsEnabled(!checked)
	}
	if strings.TrimSpace(settings.DownloadDirectory) == "" {
		useGlobalDownloadCheck.SetChecked(true)
		setDownloadControlsEnabled(false)
	} else {
		useGlobalDownloadCheck.SetChecked(false)
		setDownloadControlsEnabled(true)
	}

	deviceNameLabel := widget.NewLabel(valueOrDefault(peer.DeviceName, peer.DeviceID))
	deviceNameLabel.Wrapping = fyne.TextWrapBreak
	deviceIDLabel := widget.NewLabel(peer.DeviceID)
	deviceIDLabel.Wrapping = fyne.TextWrapBreak
	peerFingerprintLabel := widget.NewLabel(appcrypto.FormatFingerprint(peer.KeyFingerprint))
	peerFingerprintLabel.Wrapping = fyne.TextWrapBreak
	localFingerprintLabel := widget.NewLabel(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint))
	localFingerprintLabel.Wrapping = fyne.TextWrapBreak
	copyPeerFingerprintBtn := widget.NewButtonWithIcon("", iconContentCopy(), func() {
		if c.window != nil && c.window.Clipboard() != nil {
			c.window.Clipboard().SetContent(peerFingerprintLabel.Text)
		}
		c.setStatus("Peer fingerprint copied")
	})
	copyLocalFingerprintBtn := widget.NewButtonWithIcon("", iconContentCopy(), func() {
		if c.window != nil && c.window.Clipboard() != nil {
			c.window.Clipboard().SetContent(localFingerprintLabel.Text)
		}
		c.setStatus("Local fingerprint copied")
	})

	form := widget.NewForm(
		widget.NewFormItem("Device Name", deviceNameLabel),
		widget.NewFormItem("Device ID", deviceIDLabel),
		widget.NewFormItem("Peer Fingerprint", container.NewBorder(nil, nil, nil, copyPeerFingerprintBtn, peerFingerprintLabel)),
		widget.NewFormItem("Local Fingerprint", container.NewBorder(nil, nil, nil, copyLocalFingerprintBtn, localFingerprintLabel)),
		widget.NewFormItem("Custom Name", customNameEntry),
		widget.NewFormItem("Trust Level", container.NewVBox(trustLevel, trustLevelHint)),
		widget.NewFormItem("Verified", container.NewVBox(verifiedCheck, verifiedHint)),
		widget.NewFormItem("Mute Notifications", notificationsMutedCheck),
		widget.NewFormItem("Auto-Accept Files", autoAcceptCheck),
		widget.NewFormItem("Max File Size", container.NewVBox(container.NewHBox(maxSizeSelect, maxSizeCustomEntry), maxSizePeerHint)),
		widget.NewFormItem("Download Directory", container.NewVBox(
			useGlobalDownloadCheck,
			container.NewBorder(nil, nil, nil, browseDownloadDirectoryBtn, downloadDirectoryEntry),
		)),
	)

	clearHistoryBtn := widget.NewButton("Clear Chat History", func() {
		c.confirmClearPeerHistory(peer.DeviceID)
	})
	clearHistoryBtn.Importance = widget.DangerImportance

	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		clearHistoryBtn,
	)

	var popup *widget.PopUp
	closeDialog := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	saveSettings := func() {
		trustLevelValue := storage.PeerTrustLevelNormal
		if trustLevel.Selected == "Trusted" {
			trustLevelValue = storage.PeerTrustLevelTrusted
		}

		maxFileSize, err := parseMaxSizeSelection(maxSizeSelect.Selected, maxSizeCustomEntry.Text, true)
		if err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		downloadDirectory := ""
		if !useGlobalDownloadCheck.Checked {
			downloadDirectory = strings.TrimSpace(downloadDirectoryEntry.Text)
			if downloadDirectory == "" {
				dialog.ShowError(fmt.Errorf("peer download directory is required when override is enabled"), c.window)
				return
			}
			if err := os.MkdirAll(downloadDirectory, 0o700); err != nil {
				dialog.ShowError(fmt.Errorf("create peer download directory: %w", err), c.window)
				return
			}
		}

		if err := c.store.UpdatePeerSettings(storage.PeerSettings{
			PeerDeviceID:       peer.DeviceID,
			AutoAcceptFiles:    autoAcceptCheck.Checked,
			MaxFileSize:        maxFileSize,
			DownloadDirectory:  downloadDirectory,
			CustomName:         strings.TrimSpace(customNameEntry.Text),
			TrustLevel:         trustLevelValue,
			NotificationsMuted: notificationsMutedCheck.Checked,
			Verified:           verifiedCheck.Checked,
		}); err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		c.refreshPeersFromStore()
		c.refreshChatForPeer(peer.DeviceID)
		c.setStatus("Peer settings saved")
		closeDialog()
	}
	header := newPanelHeader("Peer Settings", "Peer identity and transfer preferences", closeDialog, c.handleHoverHint)
	footerRight := container.NewHBox(
		newPanelActionButton("Close", "Close peer settings", panelActionSecondary, closeDialog, c.handleHoverHint),
		newPanelActionButton("Save", "Save peer settings", panelActionPrimary, saveSettings, c.handleHoverHint),
	)
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(640, 500))
	panel := newPanelFrame(header, newPanelFooter(nil, footerRight), scroll)
	popup = newPanelPopup(c.window, panel, fyne.NewSize(680, 580))
	popup.Show()
}

func (c *controller) confirmClearPeerHistory(peerID string) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return
	}

	dialog.ShowConfirm(
		"Clear Chat History",
		"Delete all messages and transfer history with this peer?",
		func(confirm bool) {
			if !confirm {
				return
			}
			go c.clearPeerHistory(peerID)
		},
		c.window,
	)
}

func (c *controller) clearPeerHistory(peerID string) {
	if _, err := c.store.DeleteMessagesForPeer(peerID); err != nil {
		c.setStatus(fmt.Sprintf("Clear messages failed: %v", err))
		return
	}
	if _, err := c.store.DeleteFileMetadataForPeer(peerID); err != nil {
		c.setStatus(fmt.Sprintf("Clear file history failed: %v", err))
		return
	}

	c.chatMu.Lock()
	for fileID, entry := range c.fileTransfers {
		if entry.PeerDeviceID == peerID {
			delete(c.fileTransfers, fileID)
		}
	}
	c.chatMu.Unlock()

	c.refreshChatForPeer(peerID)
	c.setStatus("Chat history cleared")
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

func (c *controller) confirmResetKeys(fingerprintLabel *widget.Label) {
	dialog.ShowConfirm(
		"Reset Keys",
		"Reset identity keys? Existing peers will see a key-change warning. This requires restart.",
		func(confirm bool) {
			if !confirm {
				return
			}
			go c.resetKeys(fingerprintLabel)
		},
		c.window,
	)
}

func (c *controller) resetKeys(fingerprintLabel *widget.Label) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		c.setStatus(fmt.Sprintf("Generate Ed25519 key failed: %v", err))
		return
	}

	if err := appcrypto.SaveEd25519PrivateKey(c.cfg.Ed25519PrivateKeyPath, privateKey); err != nil {
		c.setStatus(fmt.Sprintf("Save Ed25519 private key failed: %v", err))
		return
	}
	if err := appcrypto.SaveEd25519PublicKey(c.cfg.Ed25519PublicKeyPath, publicKey); err != nil {
		c.setStatus(fmt.Sprintf("Save Ed25519 public key failed: %v", err))
		return
	}

	c.cfg.KeyFingerprint = appcrypto.KeyFingerprint(publicKey)
	if err := config.Save(c.cfgPath, c.cfg); err != nil {
		c.setStatus(fmt.Sprintf("Save config failed: %v", err))
		return
	}

	formatted := appcrypto.FormatFingerprint(c.cfg.KeyFingerprint)
	fyne.Do(func() {
		fingerprintLabel.SetText(formatted)
		dialog.ShowInformation("Keys Reset", "Identity keys were reset. Restart the app before reconnecting to peers.", c.window)
	})
	c.setStatus("Keys reset. Restart required.")
}

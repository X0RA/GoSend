package ui

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"

	"gosend/config"
	appcrypto "gosend/crypto"
	"gosend/storage"
)

func (c *controller) showSettingsDialog() {
	c.enqueueUI(func() {
		dlg := widgets.NewQDialog(c.window, 0)
		dlg.SetWindowTitle("Device Settings")
		dlg.Resize2(540, 600)

		layout := widgets.NewQVBoxLayout()
		layout.SetContentsMargins(0, 0, 0, 0)
		layout.SetSpacing(0)

		// ── Header bar ──────────────────────────────────────
		headerBar := widgets.NewQWidget(nil, 0)
		headerBar.SetObjectName("dialogHeader")
		headerBarLayout := widgets.NewQHBoxLayout()
		headerBarLayout.SetContentsMargins(16, 10, 16, 10)
		dlgTitle := widgets.NewQLabel2("Device Settings", nil, 0)
		dlgTitle.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; font-weight: bold; background: transparent;", colorText))
		headerBarLayout.AddWidget(dlgTitle, 1, 0)
		headerBar.SetLayout(headerBarLayout)
		layout.AddWidget(headerBar, 0, 0)

		// ── Scrollable content area ─────────────────────────
		scroll := widgets.NewQScrollArea(nil)
		scroll.SetWidgetResizable(true)
		scroll.SetFrameShape(widgets.QFrame__NoFrame)
		contentWidget := widgets.NewQWidget(nil, 0)
		contentLayout := widgets.NewQVBoxLayout()
		contentLayout.SetContentsMargins(16, 16, 16, 16)
		contentLayout.SetSpacing(12)

		form := widgets.NewQFormLayout(nil)

		nameEntry := widgets.NewQLineEdit(nil)
		nameEntry.SetText(c.cfg.DeviceName)

		deviceIDLabel := widgets.NewQLabel2(c.cfg.DeviceID, nil, 0)
		fingerprintLabel := widgets.NewQLabel2(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint), nil, 0)
		copyFingerprintBtn := widgets.NewQPushButton2("Copy", nil)
		copyFingerprintBtn.ConnectClicked(func(bool) {
			clipboard := gui.QGuiApplication_Clipboard()
			if clipboard != nil {
				clipboard.SetText(fingerprintLabel.Text(), gui.QClipboard__Clipboard)
				c.setStatus("Fingerprint copied")
			}
		})
		fingerprintRow := widgets.NewQWidget(nil, 0)
		fingerprintLayout := widgets.NewQHBoxLayout()
		fingerprintLayout.SetContentsMargins(0, 0, 0, 0)
		fingerprintLayout.AddWidget(fingerprintLabel, 1, 0)
		fingerprintLayout.AddWidget(copyFingerprintBtn, 0, 0)
		fingerprintRow.SetLayout(fingerprintLayout)

		portMode := widgets.NewQComboBox(nil)
		portMode.AddItems([]string{"Automatic", "Fixed"})
		if c.cfg.PortMode == config.PortModeFixed {
			portMode.SetCurrentText("Fixed")
		} else {
			portMode.SetCurrentText("Automatic")
		}

		portEntry := widgets.NewQLineEdit(nil)
		if c.cfg.ListeningPort > 0 {
			portEntry.SetText(strconv.Itoa(c.cfg.ListeningPort))
		}
		portEntry.SetEnabled(portMode.CurrentText() == "Fixed")
		portMode.ConnectCurrentTextChanged(func(text string) { portEntry.SetEnabled(text == "Fixed") })

		downloadEntry := widgets.NewQLineEdit(nil)
		downloadEntry.SetText(c.currentDownloadDirectory())
		browseDownloadBtn := widgets.NewQPushButton2("Browse", nil)
		browseDownloadBtn.ConnectClicked(func(bool) {
			path := widgets.QFileDialog_GetExistingDirectory(c.window, "Select Download Folder", c.currentDownloadDirectory(), 0)
			if strings.TrimSpace(path) != "" {
				downloadEntry.SetText(path)
			}
		})
		downloadRow := widgets.NewQWidget(nil, 0)
		downloadLayout := widgets.NewQHBoxLayout()
		downloadLayout.SetContentsMargins(0, 0, 0, 0)
		downloadLayout.AddWidget(downloadEntry, 1, 0)
		downloadLayout.AddWidget(browseDownloadBtn, 0, 0)
		downloadRow.SetLayout(downloadLayout)

		maxSizeSelect := widgets.NewQComboBox(nil)
		maxSizeSelect.AddItems([]string{maxSizePresetUnlimited, maxSizePreset100MB, maxSizePreset500MB, maxSizePreset1GB, maxSizePreset5GB, maxSizePresetCustom})
		maxCustom := widgets.NewQLineEdit(nil)
		maxCustom.SetPlaceholderText("Bytes (0 means unlimited)")
		maxSizeSelect.SetCurrentText(selectMaxSizePreset(c.cfg.MaxReceiveFileSize, false))
		if maxSizeSelect.CurrentText() == maxSizePresetCustom {
			maxCustom.SetText(strconv.FormatInt(c.cfg.MaxReceiveFileSize, 10))
			maxCustom.SetEnabled(true)
		} else {
			maxCustom.SetEnabled(false)
			if maxSizeSelect.CurrentText() == maxSizePresetUnlimited {
				maxCustom.SetText("0")
			}
		}
		maxSizeSelect.ConnectCurrentTextChanged(func(selected string) {
			if selected == maxSizePresetCustom {
				maxCustom.SetEnabled(true)
				return
			}
			maxCustom.SetEnabled(false)
			if selected == maxSizePresetUnlimited {
				maxCustom.SetText("0")
				return
			}
			if presetBytes, ok := maxSizePresetValues[selected]; ok {
				maxCustom.SetText(strconv.FormatInt(presetBytes, 10))
			}
		})
		maxHelp := widgets.NewQLabel2("Global max receive size: 0 = unlimited.", nil, 0)
		maxRow := widgets.NewQWidget(nil, 0)
		maxLayout := widgets.NewQVBoxLayout()
		maxLayout.SetContentsMargins(0, 0, 0, 0)
		line := widgets.NewQWidget(nil, 0)
		lineLayout := widgets.NewQHBoxLayout()
		lineLayout.SetContentsMargins(0, 0, 0, 0)
		lineLayout.AddWidget(maxSizeSelect, 0, 0)
		lineLayout.AddWidget(maxCustom, 1, 0)
		line.SetLayout(lineLayout)
		maxLayout.AddWidget(line, 0, 0)
		maxLayout.AddWidget(maxHelp, 0, 0)
		maxRow.SetLayout(maxLayout)

		notifyChk := widgets.NewQCheckBox2("Enable desktop notifications", nil)
		notifyChk.SetChecked(c.cfg.NotificationsEnabled)

		retentionSelect := widgets.NewQComboBox(nil)
		retentionSelect.AddItems([]string{retentionPresetKeepForever, retentionPreset30Days, retentionPreset90Days, retentionPreset1Year})
		retentionSelect.SetCurrentText(selectRetentionPreset(c.cfg.MessageRetentionDays))

		cleanupChk := widgets.NewQCheckBox2("Delete downloaded files when purging metadata", nil)
		cleanupChk.SetChecked(c.cfg.CleanupDownloadedFiles)

		form.AddRow3("Device Name", nameEntry)
		form.AddRow3("Device ID", deviceIDLabel)
		form.AddRow3("Fingerprint", fingerprintRow)
		form.AddRow3("Port Mode", portMode)
		form.AddRow3("Port", portEntry)
		form.AddRow3("Download Location", downloadRow)
		form.AddRow3("Max File Size", maxRow)
		form.AddRow3("Notifications", notifyChk)
		form.AddRow3("Message Retention", retentionSelect)
		form.AddRow3("File Cleanup", cleanupChk)

		resetKeysBtn := widgets.NewQPushButton2("⚠ Reset Identity Keys", nil)
		resetKeysBtn.SetObjectName("dangerBtn")
		resetKeysBtn.ConnectClicked(func(bool) {
			c.confirmResetKeys(func(newFingerprint string) {
				fingerprintLabel.SetText(newFingerprint)
			})
		})

		buttons := widgets.NewQDialogButtonBox3(widgets.QDialogButtonBox__Save|widgets.QDialogButtonBox__Cancel, nil)
		buttons.ConnectAccepted(func() {
			name := strings.TrimSpace(nameEntry.Text())
			if name == "" {
				widgets.QMessageBox_Warning(c.window, "Invalid Settings", "Device name is required.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}

			portModeValue := config.PortModeAutomatic
			if portMode.CurrentText() == "Fixed" {
				portModeValue = config.PortModeFixed
			}
			port := 0
			if portModeValue == config.PortModeFixed {
				parsed, err := strconv.Atoi(strings.TrimSpace(portEntry.Text()))
				if err != nil || parsed <= 0 {
					widgets.QMessageBox_Warning(c.window, "Invalid Settings", "Port must be a positive number in Fixed mode.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
					return
				}
				port = parsed
			}

			downloadDir := strings.TrimSpace(downloadEntry.Text())
			if downloadDir == "" {
				widgets.QMessageBox_Warning(c.window, "Invalid Settings", "Download location is required.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}
			if err := os.MkdirAll(downloadDir, 0o700); err != nil {
				widgets.QMessageBox_Critical(c.window, "Save Failed", fmt.Sprintf("Create download location: %v", err), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}

			maxReceiveFileSize, err := parseMaxSizeSelection(maxSizeSelect.CurrentText(), maxCustom.Text(), false)
			if err != nil {
				widgets.QMessageBox_Warning(c.window, "Invalid Settings", err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}
			retentionDays, err := parseRetentionSelection(retentionSelect.CurrentText())
			if err != nil {
				widgets.QMessageBox_Warning(c.window, "Invalid Settings", err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}

			nameChanged := c.cfg.DeviceName != name
			portChanged := c.cfg.PortMode != portModeValue || c.cfg.ListeningPort != port
			changed := nameChanged || portChanged || c.cfg.DownloadDirectory != downloadDir || c.cfg.MaxReceiveFileSize != maxReceiveFileSize || c.cfg.NotificationsEnabled != notifyChk.IsChecked() || c.cfg.MessageRetentionDays != retentionDays || c.cfg.CleanupDownloadedFiles != cleanupChk.IsChecked()
			if !changed {
				dlg.Accept()
				c.setStatus("Settings saved")
				return
			}

			c.cfg.DeviceName = name
			c.cfg.PortMode = portModeValue
			c.cfg.ListeningPort = port
			c.cfg.DownloadDirectory = downloadDir
			c.cfg.MaxReceiveFileSize = maxReceiveFileSize
			c.cfg.NotificationsEnabled = notifyChk.IsChecked()
			c.cfg.MessageRetentionDays = retentionDays
			c.cfg.CleanupDownloadedFiles = cleanupChk.IsChecked()

			if err := config.Save(c.cfgPath, c.cfg); err != nil {
				widgets.QMessageBox_Critical(c.window, "Save Failed", err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}

			if nameChanged && !portChanged {
				if c.manager != nil {
					if err := c.manager.UpdateDeviceName(name); err != nil {
						c.setStatus(fmt.Sprintf("Device name updated in config; runtime update failed: %v", err))
						widgets.QMessageBox_Information(c.window, "Restart Recommended", "Device name was saved, but runtime update failed. Restart the app to fully apply it.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
						dlg.Accept()
						return
					}
				}
				if c.discovery != nil {
					if err := c.discovery.UpdateDeviceName(name); err != nil {
						c.setStatus(fmt.Sprintf("Device name updated in config; mDNS update failed: %v", err))
						widgets.QMessageBox_Information(c.window, "Restart Recommended", "Device name was saved, but discovery update failed. Restart the app to fully apply it.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
						dlg.Accept()
						return
					}
				}
				c.identity.DeviceName = name
				c.setStatus("Settings saved")
				dlg.Accept()
				return
			}

			if portChanged {
				c.setStatus("Settings saved. Restart required to apply port changes.")
				widgets.QMessageBox_Information(c.window, "Restart Required", "Settings were saved. Restart the app to apply port changes.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				dlg.Accept()
				return
			}

			c.setStatus("Settings saved")
			dlg.Accept()
		})
		buttons.ConnectRejected(func() { dlg.Reject() })

		contentLayout.AddLayout(form, 0)
		contentLayout.AddWidget(resetKeysBtn, 0, 0)
		contentLayout.AddStretch(1)
		contentWidget.SetLayout(contentLayout)
		scroll.SetWidget(contentWidget)
		layout.AddWidget(scroll, 1, 0)

		// ── Footer bar ──────────────────────────────────────
		footerBar := widgets.NewQWidget(nil, 0)
		footerBar.SetObjectName("dialogFooter")
		footerBarLayout := widgets.NewQHBoxLayout()
		footerBarLayout.SetContentsMargins(16, 10, 16, 10)
		footerBarLayout.SetSpacing(8)
		footerBarLayout.AddStretch(1)
		cancelBtn := widgets.NewQPushButton2("Cancel", nil)
		cancelBtn.SetObjectName("secondaryBtn")
		cancelBtn.ConnectClicked(func(bool) { dlg.Reject() })
		saveBtn := widgets.NewQPushButton2("Save", nil)
		saveBtn.SetObjectName("primaryBtn")
		footerBarLayout.AddWidget(cancelBtn, 0, 0)
		footerBarLayout.AddWidget(saveBtn, 0, 0)
		footerBar.SetLayout(footerBarLayout)
		layout.AddWidget(footerBar, 0, 0)

		// Wire up the old button box accept to the new Save button.
		saveBtn.ConnectClicked(func(bool) { buttons.Accepted() })
		buttons.ConnectRejected(func() { dlg.Reject() })

		dlg.SetLayout(layout)
		dlg.Exec()
		runtime.KeepAlive(copyFingerprintBtn)
		runtime.KeepAlive(portMode)
		runtime.KeepAlive(browseDownloadBtn)
		runtime.KeepAlive(maxSizeSelect)
		runtime.KeepAlive(resetKeysBtn)
		runtime.KeepAlive(cancelBtn)
		runtime.KeepAlive(saveBtn)
		runtime.KeepAlive(buttons)
		runtime.KeepAlive(dlg)
	})
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
		c.setStatus(fmt.Sprintf("Load peer settings failed: %v", err))
		return
	}
	settings, err := c.store.GetPeerSettings(peerID)
	if err != nil {
		c.setStatus(fmt.Sprintf("Load peer settings failed: %v", err))
		return
	}

	c.enqueueUI(func() {
		dlg := widgets.NewQDialog(c.window, 0)
		dlg.SetWindowTitle("Peer Settings")
		dlg.Resize2(520, 620)

		layout := widgets.NewQVBoxLayout()
		layout.SetContentsMargins(0, 0, 0, 0)
		layout.SetSpacing(0)

		// ── Header bar ──────────────────────────────────────
		peerHeaderBar := widgets.NewQWidget(nil, 0)
		peerHeaderBar.SetObjectName("dialogHeader")
		peerHdrLayout := widgets.NewQHBoxLayout()
		peerHdrLayout.SetContentsMargins(16, 10, 16, 10)
		peerHdrInfo := widgets.NewQVBoxLayout()
		peerHdrInfo.SetContentsMargins(0, 0, 0, 0)
		peerHdrInfo.SetSpacing(2)
		peerDlgTitle := widgets.NewQLabel2("Peer Settings", nil, 0)
		peerDlgTitle.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; font-weight: bold; background: transparent;", colorText))
		peerDlgSub := widgets.NewQLabel2(c.peerDisplayName(peer), nil, 0)
		peerDlgSub.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))
		peerHdrInfo.AddWidget(peerDlgTitle, 0, 0)
		peerHdrInfo.AddWidget(peerDlgSub, 0, 0)
		peerHdrLayout.AddLayout(peerHdrInfo, 1)
		peerHeaderBar.SetLayout(peerHdrLayout)
		layout.AddWidget(peerHeaderBar, 0, 0)

		// ── Scrollable content ──────────────────────────────
		peerScroll := widgets.NewQScrollArea(nil)
		peerScroll.SetWidgetResizable(true)
		peerScroll.SetFrameShape(widgets.QFrame__NoFrame)
		peerContent := widgets.NewQWidget(nil, 0)
		peerContentLayout := widgets.NewQVBoxLayout()
		peerContentLayout.SetContentsMargins(16, 16, 16, 16)
		peerContentLayout.SetSpacing(12)

		form := widgets.NewQFormLayout(nil)

		deviceNameLabel := widgets.NewQLabel2(valueOrDefault(peer.DeviceName, peer.DeviceID), nil, 0)
		deviceIDLabel := widgets.NewQLabel2(peer.DeviceID, nil, 0)
		peerFingerprint := widgets.NewQLabel2(appcrypto.FormatFingerprint(peer.KeyFingerprint), nil, 0)
		localFingerprint := widgets.NewQLabel2(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint), nil, 0)

		copyPeerBtn := widgets.NewQPushButton2("Copy", nil)
		copyPeerBtn.ConnectClicked(func(bool) {
			clipboard := gui.QGuiApplication_Clipboard()
			if clipboard != nil {
				clipboard.SetText(peerFingerprint.Text(), gui.QClipboard__Clipboard)
				c.setStatus("Peer fingerprint copied")
			}
		})
		copyLocalBtn := widgets.NewQPushButton2("Copy", nil)
		copyLocalBtn.ConnectClicked(func(bool) {
			clipboard := gui.QGuiApplication_Clipboard()
			if clipboard != nil {
				clipboard.SetText(localFingerprint.Text(), gui.QClipboard__Clipboard)
				c.setStatus("Local fingerprint copied")
			}
		})

		customName := widgets.NewQLineEdit(nil)
		customName.SetText(settings.CustomName)

		trustLevel := widgets.NewQComboBox(nil)
		trustLevel.AddItems([]string{"Normal", "Trusted"})
		if settings.TrustLevel == storage.PeerTrustLevelTrusted {
			trustLevel.SetCurrentText("Trusted")
		} else {
			trustLevel.SetCurrentText("Normal")
		}
		trustHelp := widgets.NewQLabel2("Trust level is currently informational metadata shown as a badge.", nil, 0)

		verified := widgets.NewQCheckBox2("Verified out-of-band", nil)
		verified.SetChecked(settings.Verified)
		verifiedHelp := widgets.NewQLabel2("Verified out-of-band means you manually compared fingerprints over a separate trusted channel.", nil, 0)

		muteNotifications := widgets.NewQCheckBox2("Mute notifications for this peer", nil)
		muteNotifications.SetChecked(settings.NotificationsMuted)

		autoAccept := widgets.NewQCheckBox2("Auto-accept incoming files", nil)
		autoAccept.SetChecked(settings.AutoAcceptFiles)

		maxSizeSelect := widgets.NewQComboBox(nil)
		maxSizeSelect.AddItems([]string{maxSizePresetUseGlobal, maxSizePreset100MB, maxSizePreset500MB, maxSizePreset1GB, maxSizePreset5GB, maxSizePresetCustom})
		maxSizeSelect.SetCurrentText(selectMaxSizePreset(settings.MaxFileSize, true))
		maxCustom := widgets.NewQLineEdit(nil)
		maxCustom.SetPlaceholderText("Bytes")
		if maxSizeSelect.CurrentText() == maxSizePresetCustom {
			maxCustom.SetText(strconv.FormatInt(settings.MaxFileSize, 10))
			maxCustom.SetEnabled(true)
		} else {
			maxCustom.SetEnabled(false)
			if maxSizeSelect.CurrentText() == maxSizePresetUseGlobal {
				maxCustom.SetText("0")
			}
		}
		maxSizeSelect.ConnectCurrentTextChanged(func(selected string) {
			if selected == maxSizePresetCustom {
				maxCustom.SetEnabled(true)
				return
			}
			maxCustom.SetEnabled(false)
			if selected == maxSizePresetUseGlobal {
				maxCustom.SetText("0")
				return
			}
			if presetBytes, ok := maxSizePresetValues[selected]; ok {
				maxCustom.SetText(strconv.FormatInt(presetBytes, 10))
			}
		})
		maxHelp := widgets.NewQLabel2("Peer max file size: 0 = use global default (which may be unlimited).", nil, 0)

		useGlobalDownload := widgets.NewQCheckBox2("Use global download directory", nil)
		downloadDir := widgets.NewQLineEdit(nil)
		downloadDir.SetText(settings.DownloadDirectory)
		browseDownload := widgets.NewQPushButton2("Browse", nil)
		browseDownload.ConnectClicked(func(bool) {
			path := widgets.QFileDialog_GetExistingDirectory(c.window, "Select Peer Download Folder", c.currentDownloadDirectory(), 0)
			if strings.TrimSpace(path) != "" {
				downloadDir.SetText(path)
			}
		})
		setDownloadEnabled := func(enabled bool) {
			downloadDir.SetEnabled(enabled)
			browseDownload.SetEnabled(enabled)
		}
		if strings.TrimSpace(settings.DownloadDirectory) == "" {
			useGlobalDownload.SetChecked(true)
			setDownloadEnabled(false)
		} else {
			useGlobalDownload.SetChecked(false)
			setDownloadEnabled(true)
		}
		useGlobalDownload.ConnectToggled(func(checked bool) { setDownloadEnabled(!checked) })

		peerFPRow := widgets.NewQWidget(nil, 0)
		peerFPLayout := widgets.NewQHBoxLayout()
		peerFPLayout.SetContentsMargins(0, 0, 0, 0)
		peerFPLayout.AddWidget(peerFingerprint, 1, 0)
		peerFPLayout.AddWidget(copyPeerBtn, 0, 0)
		peerFPRow.SetLayout(peerFPLayout)

		localFPRow := widgets.NewQWidget(nil, 0)
		localFPLayout := widgets.NewQHBoxLayout()
		localFPLayout.SetContentsMargins(0, 0, 0, 0)
		localFPLayout.AddWidget(localFingerprint, 1, 0)
		localFPLayout.AddWidget(copyLocalBtn, 0, 0)
		localFPRow.SetLayout(localFPLayout)

		maxRow := widgets.NewQWidget(nil, 0)
		maxLayout := widgets.NewQVBoxLayout()
		maxLayout.SetContentsMargins(0, 0, 0, 0)
		maxLine := widgets.NewQWidget(nil, 0)
		maxLineLayout := widgets.NewQHBoxLayout()
		maxLineLayout.SetContentsMargins(0, 0, 0, 0)
		maxLineLayout.AddWidget(maxSizeSelect, 0, 0)
		maxLineLayout.AddWidget(maxCustom, 1, 0)
		maxLine.SetLayout(maxLineLayout)
		maxLayout.AddWidget(maxLine, 0, 0)
		maxLayout.AddWidget(maxHelp, 0, 0)
		maxRow.SetLayout(maxLayout)

		downloadRow := widgets.NewQWidget(nil, 0)
		downloadLayout := widgets.NewQVBoxLayout()
		downloadLayout.SetContentsMargins(0, 0, 0, 0)
		downloadLayout.AddWidget(useGlobalDownload, 0, 0)
		downloadLine := widgets.NewQWidget(nil, 0)
		downloadLineLayout := widgets.NewQHBoxLayout()
		downloadLineLayout.SetContentsMargins(0, 0, 0, 0)
		downloadLineLayout.AddWidget(downloadDir, 1, 0)
		downloadLineLayout.AddWidget(browseDownload, 0, 0)
		downloadLine.SetLayout(downloadLineLayout)
		downloadLayout.AddWidget(downloadLine, 0, 0)
		downloadRow.SetLayout(downloadLayout)

		trustRow := widgets.NewQWidget(nil, 0)
		trustLayout := widgets.NewQVBoxLayout()
		trustLayout.SetContentsMargins(0, 0, 0, 0)
		trustLayout.AddWidget(trustLevel, 0, 0)
		trustLayout.AddWidget(trustHelp, 0, 0)
		trustRow.SetLayout(trustLayout)

		verifiedRow := widgets.NewQWidget(nil, 0)
		verifiedLayout := widgets.NewQVBoxLayout()
		verifiedLayout.SetContentsMargins(0, 0, 0, 0)
		verifiedLayout.AddWidget(verified, 0, 0)
		verifiedLayout.AddWidget(verifiedHelp, 0, 0)
		verifiedRow.SetLayout(verifiedLayout)

		form.AddRow3("Device Name", deviceNameLabel)
		form.AddRow3("Device ID", deviceIDLabel)
		form.AddRow3("Peer Fingerprint", peerFPRow)
		form.AddRow3("Local Fingerprint", localFPRow)
		form.AddRow3("Custom Name", customName)
		form.AddRow3("Trust Level", trustRow)
		form.AddRow3("Verified", verifiedRow)
		form.AddRow3("Notifications", muteNotifications)
		form.AddRow3("Auto-Accept Files", autoAccept)
		form.AddRow3("Max File Size", maxRow)
		form.AddRow3("Download Directory", downloadRow)

		clearHistoryBtn := widgets.NewQPushButton2("🗑 Clear Chat History", nil)
		clearHistoryBtn.SetObjectName("dangerBtn")
		clearHistoryBtn.ConnectClicked(func(bool) { c.confirmClearPeerHistory(peer.DeviceID) })

		buttons := widgets.NewQDialogButtonBox3(widgets.QDialogButtonBox__Save|widgets.QDialogButtonBox__Cancel, nil)
		buttons.ConnectAccepted(func() {
			trustLevelValue := storage.PeerTrustLevelNormal
			if trustLevel.CurrentText() == "Trusted" {
				trustLevelValue = storage.PeerTrustLevelTrusted
			}
			maxFileSize, err := parseMaxSizeSelection(maxSizeSelect.CurrentText(), maxCustom.Text(), true)
			if err != nil {
				widgets.QMessageBox_Warning(c.window, "Invalid Settings", err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}
			downloadDirectory := ""
			if !useGlobalDownload.IsChecked() {
				downloadDirectory = strings.TrimSpace(downloadDir.Text())
				if downloadDirectory == "" {
					widgets.QMessageBox_Warning(c.window, "Invalid Settings", "Peer download directory is required when override is enabled.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
					return
				}
				if err := os.MkdirAll(downloadDirectory, 0o700); err != nil {
					widgets.QMessageBox_Critical(c.window, "Save Failed", fmt.Sprintf("Create peer download directory: %v", err), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
					return
				}
			}

			if err := c.store.UpdatePeerSettings(storage.PeerSettings{
				PeerDeviceID:       peer.DeviceID,
				AutoAcceptFiles:    autoAccept.IsChecked(),
				MaxFileSize:        maxFileSize,
				DownloadDirectory:  downloadDirectory,
				CustomName:         strings.TrimSpace(customName.Text()),
				TrustLevel:         trustLevelValue,
				NotificationsMuted: muteNotifications.IsChecked(),
				Verified:           verified.IsChecked(),
			}); err != nil {
				widgets.QMessageBox_Critical(c.window, "Save Failed", err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				return
			}

			c.refreshPeersFromStore()
			c.refreshChatForPeer(peer.DeviceID)
			c.setStatus("Peer settings saved")
			dlg.Accept()
		})
		buttons.ConnectRejected(func() { dlg.Reject() })

		peerContentLayout.AddLayout(form, 0)
		peerContentLayout.AddWidget(clearHistoryBtn, 0, 0)
		peerContentLayout.AddStretch(1)
		peerContent.SetLayout(peerContentLayout)
		peerScroll.SetWidget(peerContent)
		layout.AddWidget(peerScroll, 1, 0)

		// ── Footer bar ──────────────────────────────────────
		peerFooterBar := widgets.NewQWidget(nil, 0)
		peerFooterBar.SetObjectName("dialogFooter")
		peerFooterLayout := widgets.NewQHBoxLayout()
		peerFooterLayout.SetContentsMargins(16, 10, 16, 10)
		peerFooterLayout.SetSpacing(8)
		peerFooterLayout.AddStretch(1)
		peerCancelBtn := widgets.NewQPushButton2("Cancel", nil)
		peerCancelBtn.SetObjectName("secondaryBtn")
		peerCancelBtn.ConnectClicked(func(bool) { dlg.Reject() })
		peerSaveBtn := widgets.NewQPushButton2("Save", nil)
		peerSaveBtn.SetObjectName("primaryBtn")
		peerFooterLayout.AddWidget(peerCancelBtn, 0, 0)
		peerFooterLayout.AddWidget(peerSaveBtn, 0, 0)
		peerFooterBar.SetLayout(peerFooterLayout)
		layout.AddWidget(peerFooterBar, 0, 0)

		peerSaveBtn.ConnectClicked(func(bool) { buttons.Accepted() })
		buttons.ConnectRejected(func() { dlg.Reject() })

		dlg.SetLayout(layout)
		dlg.Exec()
		runtime.KeepAlive(copyPeerBtn)
		runtime.KeepAlive(copyLocalBtn)
		runtime.KeepAlive(maxSizeSelect)
		runtime.KeepAlive(browseDownload)
		runtime.KeepAlive(useGlobalDownload)
		runtime.KeepAlive(clearHistoryBtn)
		runtime.KeepAlive(peerCancelBtn)
		runtime.KeepAlive(peerSaveBtn)
		runtime.KeepAlive(buttons)
		runtime.KeepAlive(dlg)
	})
}

func (c *controller) confirmClearPeerHistory(peerID string) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return
	}
	answer := widgets.QMessageBox_Question(c.window, "Clear Chat History", "Delete all messages and transfer history with this peer?", widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)
	if answer != widgets.QMessageBox__Yes {
		return
	}
	go c.clearPeerHistory(peerID)
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

func (c *controller) confirmResetKeys(updateLabel func(string)) {
	answer := widgets.QMessageBox_Question(c.window, "Reset Keys", "Reset identity keys? Existing peers will see a key-change warning. This requires restart.", widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)
	if answer != widgets.QMessageBox__Yes {
		return
	}
	go c.resetKeys(updateLabel)
}

func (c *controller) resetKeys(updateLabel func(string)) {
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
	c.enqueueUI(func() {
		if updateLabel != nil {
			updateLabel(formatted)
		}
		widgets.QMessageBox_Information(c.window, "Keys Reset", "Identity keys were reset. Restart the app before reconnecting to peers.", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
	})
	c.setStatus("Keys reset. Restart required.")
}

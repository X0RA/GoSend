package ui

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"gosend/config"
	appcrypto "gosend/crypto"
)

func (c *controller) showSettingsDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(c.cfg.DeviceName)

	portEntry := widget.NewEntry()
	portEntry.SetText(strconv.Itoa(c.cfg.ListeningPort))

	fingerprintLabel := widget.NewLabel(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint))
	deviceIDLabel := widget.NewLabel(c.cfg.DeviceID)

	resetKeysBtn := widget.NewButton("Reset Keys", func() {
		c.confirmResetKeys(fingerprintLabel)
	})

	content := container.NewVBox(
		widget.NewLabel("Device Name"),
		nameEntry,
		widget.NewLabel("Device ID"),
		deviceIDLabel,
		widget.NewLabel("Fingerprint"),
		fingerprintLabel,
		widget.NewLabel("Listening Port"),
		portEntry,
		resetKeysBtn,
	)

	dlg := dialog.NewCustomConfirm("Device Settings", "Save", "Close", content, func(save bool) {
		if !save {
			return
		}

		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			dialog.ShowError(fmt.Errorf("device name is required"), c.window)
			return
		}

		port, err := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		if err != nil || port <= 0 {
			dialog.ShowError(fmt.Errorf("port must be a positive number"), c.window)
			return
		}

		changed := c.cfg.DeviceName != name || c.cfg.ListeningPort != port
		c.cfg.DeviceName = name
		c.cfg.ListeningPort = port

		if err := config.Save(c.cfgPath, c.cfg); err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		if changed {
			c.setStatus("Settings saved. Restart required to apply name/port changes.")
			dialog.ShowInformation("Restart Required", "Settings were saved. Restart the app to apply device name/port changes.", c.window)
			return
		}
		c.setStatus("Settings saved")
	}, c.window)

	dlg.Resize(fyne.NewSize(520, 420))
	dlg.Show()
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

	x25519PrivateKey, err := appcrypto.GenerateX25519PrivateKey()
	if err != nil {
		c.setStatus(fmt.Sprintf("Generate X25519 key failed: %v", err))
		return
	}
	if err := appcrypto.SaveX25519PrivateKey(c.cfg.X25519PrivateKeyPath, x25519PrivateKey); err != nil {
		c.setStatus(fmt.Sprintf("Save X25519 key failed: %v", err))
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

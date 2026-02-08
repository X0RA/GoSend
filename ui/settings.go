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

	fingerprintLabel := widget.NewLabel(appcrypto.FormatFingerprint(c.cfg.KeyFingerprint))
	fingerprintLabel.Wrapping = fyne.TextWrapBreak
	deviceIDLabel := widget.NewLabel(c.cfg.DeviceID)
	deviceIDLabel.Wrapping = fyne.TextWrapBreak

	resetKeysBtn := widget.NewButton("Reset Keys", func() {
		c.confirmResetKeys(fingerprintLabel)
	})
	resetKeysBtn.Importance = widget.DangerImportance

	form := widget.NewForm(
		widget.NewFormItem("Device Name", nameEntry),
		widget.NewFormItem("Device ID", deviceIDLabel),
		widget.NewFormItem("Fingerprint", fingerprintLabel),
		widget.NewFormItem("Port Mode", portModeGroup),
		widget.NewFormItem("Port", portEntry),
	)

	content := container.NewVBox(
		form,
		widget.NewSeparator(),
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

		changed := c.cfg.DeviceName != name || c.cfg.PortMode != portMode || c.cfg.ListeningPort != port
		if !changed {
			c.setStatus("Settings saved")
			return
		}
		c.cfg.DeviceName = name
		c.cfg.PortMode = portMode
		c.cfg.ListeningPort = port

		if err := config.Save(c.cfgPath, c.cfg); err != nil {
			dialog.ShowError(err, c.window)
			return
		}

		c.setStatus("Settings saved. Restart required to apply name/port changes.")
		dialog.ShowInformation("Restart Required", "Settings were saved. Restart the app to apply device name/port changes.", c.window)
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

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/widgets"
)

// ---------------------------------------------------------------------------
// Colour helpers for dynamic widget styling
// ---------------------------------------------------------------------------

// peerStatusDotColor returns the accent colour for a peer connection-state dot.
func peerStatusDotColor(stateText string) string {
	lower := strings.ToLower(stateText)
	switch {
	case strings.Contains(lower, "online"):
		return colorGreen
	case strings.Contains(lower, "connecting..."):
		return colorBlue
	case strings.Contains(lower, "reconnecting"):
		return colorYellow
	case strings.Contains(lower, "disconnecting"):
		return colorOverlay1
	default:
		return colorOverlay0
	}
}

// fileStatusColor returns the accent colour for a transfer-status label.
func fileStatusColor(statusText string) string {
	lower := strings.ToLower(statusText)
	switch {
	case strings.Contains(lower, "complete"):
		return colorGreen
	case strings.Contains(lower, "sending"), strings.Contains(lower, "receiving"):
		return colorYellow
	case strings.Contains(lower, "failed"):
		return colorRed
	default:
		return colorOverlay1
	}
}

// ---------------------------------------------------------------------------
// Reusable small-widget factories
// ---------------------------------------------------------------------------

// newBadgeLabel creates a small coloured pill badge.
func newBadgeLabel(text, bg, fg string) *widgets.QLabel {
	lbl := widgets.NewQLabel2(text, nil, 0)
	lbl.SetStyleSheet(fmt.Sprintf(
		"background: %s; color: %s; padding: 1px 7px; border-radius: 4px; font-size: 10px; font-weight: bold;",
		bg, fg,
	))
	lbl.SetAlignment(core.Qt__AlignCenter)
	return lbl
}

// newCountBadge creates a round numeric badge (e.g. online peer count).
func newCountBadge(count int) *widgets.QLabel {
	lbl := widgets.NewQLabel2(fmt.Sprintf("%d", count), nil, 0)
	lbl.SetAlignment(core.Qt__AlignCenter)
	lbl.SetStyleSheet(fmt.Sprintf(
		"background: %s; color: %s; border-radius: 10px; padding: 2px 7px; font-size: 10px; font-weight: bold; min-width: 14px;",
		colorBlue, colorCrust,
	))
	return lbl
}

// newIconButton creates a compact icon-only button.
func newIconButton(icon, tooltip string) *widgets.QPushButton {
	btn := widgets.NewQPushButton2(icon, nil)
	btn.SetToolTip(tooltip)
	btn.SetObjectName("iconBtn")
	btn.SetFixedSize2(32, 32)
	return btn
}

// ---------------------------------------------------------------------------
// Peer list item widget
// ---------------------------------------------------------------------------

// createPeerItemWidget builds the styled widget shown in each row of the
// peers list: status dot, name + badges, secondary info line.
func createPeerItemWidget(display, stateText string, isTrusted, isVerified bool, secondaryInfo string) *widgets.QWidget {
	w := widgets.NewQWidget(nil, 0)
	w.SetObjectName("peerItemWidget")

	root := widgets.NewQHBoxLayout()
	root.SetContentsMargins(10, 6, 10, 6)
	root.SetSpacing(8)

	// Status dot.
	dot := widgets.NewQLabel2("●", nil, 0)
	dot.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", peerStatusDotColor(stateText)))
	dot.SetFixedWidth(14)
	dot.SetAlignment(core.Qt__AlignTop)

	// Info column.
	info := widgets.NewQVBoxLayout()
	info.SetContentsMargins(0, 0, 0, 0)
	info.SetSpacing(2)

	// Row 1: name + badges.
	nameRow := widgets.NewQHBoxLayout()
	nameRow.SetContentsMargins(0, 0, 0, 0)
	nameRow.SetSpacing(6)

	nameLbl := widgets.NewQLabel2(display, nil, 0)
	nameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-weight: bold; font-size: 13px; background: transparent;", colorText))
	nameRow.AddWidget(nameLbl, 0, 0)

	if isTrusted {
		nameRow.AddWidget(newBadgeLabel("Trusted", colorGreen, colorCrust), 0, 0)
	}
	if isVerified {
		nameRow.AddWidget(newBadgeLabel("Verified", colorLavender, colorCrust), 0, 0)
	}
	nameRow.AddStretch(1)

	// Row 2: secondary / state text.
	statusLine := stateText
	if secondaryInfo != "" {
		statusLine = secondaryInfo + "  ·  " + stateText
	}
	stateLbl := widgets.NewQLabel2(statusLine, nil, 0)
	stateLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	info.AddLayout(nameRow, 0)
	info.AddWidget(stateLbl, 0, 0)

	root.AddWidget(dot, 0, core.Qt__AlignTop)
	root.AddLayout(info, 1)

	w.SetLayout(root)
	return w
}

// ---------------------------------------------------------------------------
// Chat message card
// ---------------------------------------------------------------------------

// createMessageCardWidget builds a card widget for one text message in the
// chat transcript.
func createMessageCardWidget(sender, timestamp, content, deliveryMark string, isOutbound bool) *widgets.QWidget {
	card := widgets.NewQWidget(nil, 0)
	card.SetObjectName("messageCard")
	card.SetStyleSheet(fmt.Sprintf("QWidget#messageCard { background: %s; border-radius: 8px; }", colorSurface0))

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 8, 12, 8)
	layout.SetSpacing(4)

	// Header: sender + timestamp + delivery mark.
	hdr := widgets.NewQHBoxLayout()
	hdr.SetContentsMargins(0, 0, 0, 0)
	hdr.SetSpacing(8)

	senderColor := colorMauve
	if isOutbound {
		senderColor = colorBlue
	}

	senderLbl := widgets.NewQLabel2(sender, nil, 0)
	senderLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-weight: bold; font-size: 12px; background: transparent;", senderColor))
	timeLbl := widgets.NewQLabel2(timestamp, nil, 0)
	timeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	hdr.AddWidget(senderLbl, 0, 0)
	hdr.AddWidget(timeLbl, 0, 0)

	if deliveryMark != "" {
		markColor := colorSubtext0
		switch deliveryMark {
		case "✓✓":
			markColor = colorGreen
		case "✗":
			markColor = colorRed
		case "…":
			markColor = colorYellow
		default:
			markColor = colorSubtext1
		}
		markLbl := widgets.NewQLabel2(deliveryMark, nil, 0)
		markLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 12px; background: transparent;", markColor))
		hdr.AddWidget(markLbl, 0, 0)
	}
	hdr.AddStretch(1)

	contentLbl := widgets.NewQLabel2(content, nil, 0)
	contentLbl.SetWordWrap(true)
	contentLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", colorText))

	layout.AddLayout(hdr, 0)
	layout.AddWidget(contentLbl, 0, 0)
	card.SetLayout(layout)
	return card
}

// ---------------------------------------------------------------------------
// File transfer card
// ---------------------------------------------------------------------------

// createFileTransferCardWidget builds a card widget for one file transfer
// in the chat transcript, including an optional progress bar.
func createFileTransferCardWidget(file chatFileEntry) *widgets.QWidget {
	card := widgets.NewQWidget(nil, 0)
	card.SetObjectName("transferCard")
	card.SetStyleSheet(fmt.Sprintf("QWidget#transferCard { background: %s; border-radius: 8px; }", colorSurface0))

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 8, 12, 8)
	layout.SetSpacing(4)

	isSend := strings.EqualFold(file.Direction, "send")
	dirText := "Receive File"
	dirBg := colorTeal
	if isSend {
		dirText = "Send File"
		dirBg = colorPeach
	}

	// Row 1: icon + direction badge + filename.
	row1 := widgets.NewQHBoxLayout()
	row1.SetContentsMargins(0, 0, 0, 0)
	row1.SetSpacing(8)

	iconLbl := widgets.NewQLabel2("📄", nil, 0)
	iconLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 16px; background: transparent;", dirBg))
	iconLbl.SetFixedWidth(20)

	dirBadge := newBadgeLabel(dirText, dirBg, colorCrust)

	fnameLbl := widgets.NewQLabel2(valueOrDefault(file.Filename, file.FileID), nil, 0)
	fnameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-weight: bold; font-size: 13px; background: transparent;", colorText))

	row1.AddWidget(iconLbl, 0, 0)
	row1.AddWidget(dirBadge, 0, 0)
	row1.AddWidget(fnameLbl, 1, 0)

	// Row 2: timestamp + size + status.
	row2 := widgets.NewQHBoxLayout()
	row2.SetContentsMargins(0, 0, 0, 0)
	row2.SetSpacing(8)

	timeLbl := widgets.NewQLabel2(formatTimestamp(file.AddedAt), nil, 0)
	timeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	sizeLbl := widgets.NewQLabel2(formatBytes(file.Filesize), nil, 0)
	sizeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	statusText := fileTransferStatusText(file)
	statusLbl := widgets.NewQLabel2(statusText, nil, 0)
	statusLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; font-weight: bold; background: transparent;", fileStatusColor(statusText)))

	row2.AddWidget(timeLbl, 0, 0)
	row2.AddWidget(sizeLbl, 0, 0)
	row2.AddWidget(statusLbl, 0, 0)
	row2.AddStretch(1)

	layout.AddLayout(row1, 0)
	layout.AddLayout(row2, 0)

	// Progress bar for active transfers.
	if file.TotalBytes > 0 && !file.TransferCompleted && !isTerminalTransferStatus(file.Status) {
		pct := int(float64(file.BytesTransferred) * 100 / float64(file.TotalBytes))
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		bar := widgets.NewQProgressBar(nil)
		bar.SetMinimum(0)
		bar.SetMaximum(100)
		bar.SetValue(pct)
		bar.SetTextVisible(false)
		bar.SetFixedHeight(6)
		bar.SetStyleSheet(fmt.Sprintf(
			"QProgressBar { background: %s; border: none; border-radius: 3px; } QProgressBar::chunk { background: %s; border-radius: 3px; }",
			colorSurface1, colorBlue,
		))
		layout.AddWidget(bar, 0, 0)
	}

	// Path (muted).
	if strings.TrimSpace(file.StoredPath) != "" {
		pathLbl := widgets.NewQLabel2(file.StoredPath, nil, 0)
		pathLbl.SetWordWrap(true)
		pathLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 10px; background: transparent;", colorOverlay1))
		layout.AddWidget(pathLbl, 0, 0)
	}

	card.SetLayout(layout)
	return card
}

// ---------------------------------------------------------------------------
// Transfer queue dialog item widget
// ---------------------------------------------------------------------------

// createTransferQueueItemWidget builds a styled row for the transfer queue dialog.
func createTransferQueueItemWidget(entry chatFileEntry, peerName string) *widgets.QWidget {
	card := widgets.NewQWidget(nil, 0)
	card.SetObjectName("queueCard")
	card.SetStyleSheet(fmt.Sprintf("QWidget#queueCard { background: %s; border-radius: 8px; }", colorSurface0))

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 8, 12, 8)
	layout.SetSpacing(4)

	// Row 1: peer name | filename.
	row1 := widgets.NewQHBoxLayout()
	row1.SetContentsMargins(0, 0, 0, 0)
	row1.SetSpacing(8)

	peerLbl := widgets.NewQLabel2(peerName, nil, 0)
	peerLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-weight: bold; font-size: 12px; background: transparent;", colorBlue))
	fnameLbl := widgets.NewQLabel2(valueOrDefault(entry.Filename, entry.FileID), nil, 0)
	fnameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 12px; background: transparent;", colorText))
	row1.AddWidget(peerLbl, 0, 0)
	row1.AddWidget(fnameLbl, 1, 0)

	// Row 2: status | pct | speed | ETA.
	row2 := widgets.NewQHBoxLayout()
	row2.SetContentsMargins(0, 0, 0, 0)
	row2.SetSpacing(8)

	statusText := fileTransferStatusText(entry)
	statusLbl := widgets.NewQLabel2(statusText, nil, 0)
	statusLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; font-weight: bold; background: transparent;", fileStatusColor(statusText)))

	pctText := "--"
	if entry.TotalBytes > 0 {
		pctText = fmt.Sprintf("%.0f%%", float64(entry.BytesTransferred)*100/float64(entry.TotalBytes))
	}
	pctLbl := widgets.NewQLabel2(pctText, nil, 0)
	pctLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	speedText := "--"
	if entry.SpeedBytesPerSec > 0 {
		speedText = fmt.Sprintf("%s/s", formatBytes(int64(entry.SpeedBytesPerSec)))
	}
	speedLbl := widgets.NewQLabel2(speedText, nil, 0)
	speedLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	etaText := "--"
	if entry.ETASeconds > 0 {
		etaText = "ETA " + (time.Duration(entry.ETASeconds) * time.Second).Round(time.Second).String()
	}
	etaLbl := widgets.NewQLabel2(etaText, nil, 0)
	etaLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorSubtext0))

	row2.AddWidget(statusLbl, 0, 0)
	row2.AddWidget(pctLbl, 0, 0)
	row2.AddWidget(speedLbl, 0, 0)
	row2.AddWidget(etaLbl, 0, 0)
	row2.AddStretch(1)

	layout.AddLayout(row1, 0)
	layout.AddLayout(row2, 0)

	// Progress bar for active transfers.
	if entry.TotalBytes > 0 && !entry.TransferCompleted && !isTerminalTransferStatus(entry.Status) {
		pct := int(float64(entry.BytesTransferred) * 100 / float64(entry.TotalBytes))
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		bar := widgets.NewQProgressBar(nil)
		bar.SetMinimum(0)
		bar.SetMaximum(100)
		bar.SetValue(pct)
		bar.SetTextVisible(false)
		bar.SetFixedHeight(6)
		bar.SetStyleSheet(fmt.Sprintf(
			"QProgressBar { background: %s; border: none; border-radius: 3px; } QProgressBar::chunk { background: %s; border-radius: 3px; }",
			colorSurface1, colorBlue,
		))
		layout.AddWidget(bar, 0, 0)
	}

	card.SetLayout(layout)
	return card
}

// itemHeightForWidget returns a reasonable size hint height for a widget
// being placed inside a QListWidgetItem. It uses the widget's own size hint
// with a minimum floor.
func itemHeightForWidget(w *widgets.QWidget, minHeight int) int {
	h := w.SizeHint().Height()
	if h < minHeight {
		h = minHeight
	}
	return h
}

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

// peerStatusTextColor returns the colour used for the status text line itself.
func peerStatusTextColor(stateText string) string {
	lower := strings.ToLower(stateText)
	switch {
	case strings.Contains(lower, "online"):
		return colorGreen
	case strings.Contains(lower, "reconnecting"):
		return colorYellow
	case strings.Contains(lower, "connecting"):
		return colorBlue
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

// newCountBadge creates a small mono count indicator (mockup style).
func newCountBadge(count int) *widgets.QLabel {
	lbl := widgets.NewQLabel2(fmt.Sprintf("%d", count), nil, 0)
	lbl.SetAlignment(core.Qt__AlignCenter)
	lbl.SetStyleSheet(fmt.Sprintf(
		"background: %s; color: %s; border-radius: 4px; padding: 1px 6px; font-size: 11px; min-width: 14px;",
		colorSurface0, colorOverlay1,
	))
	return lbl
}

// newIconButton creates a compact icon-only button (mockup style: transparent bg, overlay2 text).
func newIconButton(icon, tooltip string) *widgets.QPushButton {
	btn := widgets.NewQPushButton2(icon, nil)
	btn.SetToolTip(tooltip)
	btn.SetObjectName("iconBtn")
	btn.SetFixedSize2(30, 30)
	return btn
}

// newFileActionButton creates a small inline action button for file cards.
func newFileActionButton(text string, enabled bool) *widgets.QPushButton {
	btn := widgets.NewQPushButton2(text, nil)
	btn.SetObjectName("fileActionBtn")
	btn.SetEnabled(enabled)
	return btn
}

// ---------------------------------------------------------------------------
// Peer list item widget
// ---------------------------------------------------------------------------

// createPeerItemWidget builds the styled widget shown in each row of the
// peers list – matching the mockup's status dot, truncated name, semi-
// transparent badges, and coloured status text.
func createPeerItemWidget(display, stateText string, isTrusted, isVerified bool, secondaryInfo string) *widgets.QWidget {
	w := widgets.NewQWidget(nil, 0)
	w.SetObjectName("peerItemWidget")

	root := widgets.NewQHBoxLayout()
	root.SetContentsMargins(12, 6, 12, 6)
	root.SetSpacing(6)

	// Status dot (8×8 rendered via a small label).
	dot := widgets.NewQLabel2("●", nil, 0)
	dot.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 9px; background: transparent;", peerStatusDotColor(stateText)))
	dot.SetFixedWidth(12)
	dot.SetAlignment(core.Qt__AlignVCenter)

	// Info column.
	info := widgets.NewQVBoxLayout()
	info.SetContentsMargins(0, 0, 0, 0)
	info.SetSpacing(2)

	// Row 1: name + badges.
	nameRow := widgets.NewQHBoxLayout()
	nameRow.SetContentsMargins(0, 0, 0, 0)
	nameRow.SetSpacing(4)

	nameLbl := widgets.NewQLabel2(display, nil, 0)
	nameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", colorText))
	nameRow.AddWidget(nameLbl, 0, 0)

	// Semi-transparent badge backgrounds (matching mockup rgba).
	if isTrusted {
		badge := widgets.NewQLabel2("Trusted", nil, 0)
		badge.SetStyleSheet(fmt.Sprintf(
			"background: rgba(148, 226, 213, 0.1); color: %s; padding: 0px 4px; border-radius: 3px; font-size: 10px;",
			colorTeal,
		))
		badge.SetAlignment(core.Qt__AlignCenter)
		nameRow.AddWidget(badge, 0, 0)
	}
	if isVerified {
		badge := widgets.NewQLabel2("Verified", nil, 0)
		badge.SetStyleSheet(fmt.Sprintf(
			"background: rgba(166, 227, 161, 0.1); color: %s; padding: 0px 4px; border-radius: 3px; font-size: 10px;",
			colorGreen,
		))
		badge.SetAlignment(core.Qt__AlignCenter)
		nameRow.AddWidget(badge, 0, 0)
	}
	nameRow.AddStretch(1)

	// Row 2: status text (coloured) + optional device name.
	statusRow := widgets.NewQHBoxLayout()
	statusRow.SetContentsMargins(0, 0, 0, 0)
	statusRow.SetSpacing(4)

	stateLbl := widgets.NewQLabel2(stateText, nil, 0)
	stateLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", peerStatusTextColor(stateText)))
	statusRow.AddWidget(stateLbl, 0, 0)

	if secondaryInfo != "" {
		secLbl := widgets.NewQLabel2("("+secondaryInfo+")", nil, 0)
		secLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))
		statusRow.AddWidget(secLbl, 0, 0)
	}
	statusRow.AddStretch(1)

	info.AddLayout(nameRow, 0)
	info.AddLayout(statusRow, 0)

	root.AddWidget(dot, 0, core.Qt__AlignVCenter)
	root.AddLayout(info, 1)

	w.SetLayout(root)
	return w
}

// ---------------------------------------------------------------------------
// Chat message widget (mockup: transparent bg, subtext1 content)
// ---------------------------------------------------------------------------

// createMessageCardWidget builds a widget for one text message.
// Mockup style: no card background, sender in blue/mauve, content in subtext1.
func createMessageCardWidget(sender, timestamp, content, deliveryMark string, isOutbound bool) *widgets.QWidget {
	card := widgets.NewQWidget(nil, 0)
	card.SetObjectName("messageCard")
	// NO background – transparent to match mockup.

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 6, 12, 6)
	layout.SetSpacing(2)

	// Header: sender + timestamp + delivery mark.
	hdr := widgets.NewQHBoxLayout()
	hdr.SetContentsMargins(0, 0, 0, 0)
	hdr.SetSpacing(8)

	senderColor := colorMauve
	if isOutbound {
		senderColor = colorBlue
	}

	senderLbl := widgets.NewQLabel2(sender, nil, 0)
	senderLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 12px; background: transparent;", senderColor))
	timeLbl := widgets.NewQLabel2(timestamp, nil, 0)
	timeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))

	hdr.AddWidget(senderLbl, 0, 0)
	hdr.AddWidget(timeLbl, 0, 0)

	if deliveryMark != "" {
		markColor := colorOverlay1
		switch deliveryMark {
		case "✓✓":
			markColor = colorBlue // mockup uses blue for delivered
		case "✓":
			markColor = colorOverlay2
		case "✗":
			markColor = colorRed
		case "…":
			markColor = colorOverlay1
		}
		markLbl := widgets.NewQLabel2(deliveryMark, nil, 0)
		markLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 12px; background: transparent;", markColor))
		hdr.AddWidget(markLbl, 0, 0)
	}
	hdr.AddStretch(1)

	contentLbl := widgets.NewQLabel2(content, nil, 0)
	contentLbl.SetWordWrap(true)
	contentLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", colorSubtext1))

	layout.AddLayout(hdr, 0)
	layout.AddWidget(contentLbl, 0, 0)
	card.SetLayout(layout)
	return card
}

// ---------------------------------------------------------------------------
// fileCardCallbacks holds optional per-card action handlers.
// ---------------------------------------------------------------------------

type fileCardCallbacks struct {
	onCancel   func()
	onRetry    func()
	onShowPath func()
	onCopyPath func()
}

// ---------------------------------------------------------------------------
// File transfer card (mockup: surface0 bg, inline action buttons)
// ---------------------------------------------------------------------------

// createFileTransferCardWidget builds a card for one file transfer in the
// chat transcript. It includes inline action buttons wired to the given
// callbacks, matching the mockup's per-row controls.
func createFileTransferCardWidget(file chatFileEntry, cbs fileCardCallbacks) *widgets.QWidget {
	card := widgets.NewQWidget(nil, 0)
	card.SetObjectName("transferCard")
	card.SetStyleSheet(fmt.Sprintf("QWidget#transferCard { background: %s; border-radius: 4px; }", colorSurface0))

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 8, 12, 8)
	layout.SetSpacing(4)

	isSend := strings.EqualFold(file.Direction, "send")
	dirText := "[Receive File]"
	dirColor := colorTeal
	if isSend {
		dirText = "[Send File]"
		dirColor = colorPeach
	}

	// Row 1: icon + direction label + filename.
	row1 := widgets.NewQHBoxLayout()
	row1.SetContentsMargins(0, 0, 0, 0)
	row1.SetSpacing(8)

	iconLbl := widgets.NewQLabel2("📄", nil, 0)
	iconLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 14px; background: transparent;", dirColor))
	iconLbl.SetFixedWidth(18)

	dirLbl := widgets.NewQLabel2(dirText, nil, 0)
	dirLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", dirColor))

	fnameLbl := widgets.NewQLabel2(valueOrDefault(file.Filename, file.FileID), nil, 0)
	fnameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", colorText))

	row1.AddWidget(iconLbl, 0, 0)
	row1.AddWidget(dirLbl, 0, 0)
	row1.AddWidget(fnameLbl, 1, 0)

	// Row 2: timestamp + size + status (indented to align under text).
	row2 := widgets.NewQHBoxLayout()
	row2.SetContentsMargins(26, 0, 0, 0) // indent past icon
	row2.SetSpacing(8)

	timeLbl := widgets.NewQLabel2(formatTimestamp(file.AddedAt), nil, 0)
	timeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))

	sizeLbl := widgets.NewQLabel2(formatBytes(file.Filesize), nil, 0)
	sizeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))

	statusText := fileTransferStatusText(file)
	statusLbl := widgets.NewQLabel2(statusText, nil, 0)
	statusLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", fileStatusColor(statusText)))

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
		bar.SetFixedHeight(4)
		bar.SetStyleSheet(fmt.Sprintf(
			"QProgressBar { background: %s; border: none; border-radius: 2px; margin-left: 26px; } QProgressBar::chunk { background: %s; border-radius: 2px; }",
			colorSurface2, colorBlue,
		))
		layout.AddWidget(bar, 0, 0)
	}

	// Path (muted mono, indented).
	if strings.TrimSpace(file.StoredPath) != "" {
		pathLbl := widgets.NewQLabel2(file.StoredPath, nil, 0)
		pathLbl.SetWordWrap(true)
		pathLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 10px; background: transparent; padding-left: 26px;", colorOverlay0))
		layout.AddWidget(pathLbl, 0, 0)
	}

	// Inline action buttons (matching mockup per-row controls).
	pendingOrActive := !file.TransferCompleted && (strings.EqualFold(file.Status, "pending") || strings.EqualFold(file.Status, "accepted"))
	canRetry := (strings.EqualFold(file.Status, "failed") || strings.EqualFold(file.Status, "rejected")) && strings.EqualFold(file.Direction, "send")
	hasPath := strings.TrimSpace(file.StoredPath) != ""

	actRow := widgets.NewQHBoxLayout()
	actRow.SetContentsMargins(26, 4, 0, 0)
	actRow.SetSpacing(4)

	cancelBtn := newFileActionButton("✕ Cancel", pendingOrActive)
	if pendingOrActive && cbs.onCancel != nil {
		fn := cbs.onCancel
		cancelBtn.ConnectClicked(func(bool) { fn() })
	}
	retryBtn := newFileActionButton("↻ Retry", canRetry)
	if canRetry && cbs.onRetry != nil {
		fn := cbs.onRetry
		retryBtn.ConnectClicked(func(bool) { fn() })
	}
	showPathBtn := newFileActionButton("↗ Show Path", hasPath)
	if hasPath && cbs.onShowPath != nil {
		fn := cbs.onShowPath
		showPathBtn.ConnectClicked(func(bool) { fn() })
	}
	copyPathBtn := newFileActionButton("⎘ Copy Path", hasPath)
	if hasPath && cbs.onCopyPath != nil {
		fn := cbs.onCopyPath
		copyPathBtn.ConnectClicked(func(bool) { fn() })
	}

	actRow.AddWidget(cancelBtn, 0, 0)
	actRow.AddWidget(retryBtn, 0, 0)
	actRow.AddWidget(showPathBtn, 0, 0)
	actRow.AddWidget(copyPathBtn, 0, 0)
	actRow.AddStretch(1)
	layout.AddLayout(actRow, 0)

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
	card.SetStyleSheet(fmt.Sprintf("QWidget#queueCard { background: %s; border-radius: 4px; }", colorSurface0))

	layout := widgets.NewQVBoxLayout()
	layout.SetContentsMargins(12, 8, 12, 8)
	layout.SetSpacing(4)

	isSend := strings.EqualFold(entry.Direction, "send")

	// Row 1: direction icon + filename + filesize.
	row1 := widgets.NewQHBoxLayout()
	row1.SetContentsMargins(0, 0, 0, 0)
	row1.SetSpacing(8)

	dirIcon := "↓"
	dirIconColor := colorTeal
	if isSend {
		dirIcon = "↑"
		dirIconColor = colorPeach
	}
	iconLbl := widgets.NewQLabel2(dirIcon, nil, 0)
	iconLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", dirIconColor))
	iconLbl.SetFixedWidth(14)

	fnameLbl := widgets.NewQLabel2(valueOrDefault(entry.Filename, entry.FileID), nil, 0)
	fnameLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 13px; background: transparent;", colorText))

	sizeLbl := widgets.NewQLabel2(formatBytes(entry.Filesize), nil, 0)
	sizeLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))

	row1.AddWidget(iconLbl, 0, 0)
	row1.AddWidget(fnameLbl, 1, 0)
	row1.AddWidget(sizeLbl, 0, 0)

	// Row 2: peer info + status + speed + ETA (indented).
	row2 := widgets.NewQHBoxLayout()
	row2.SetContentsMargins(22, 0, 0, 0)
	row2.SetSpacing(8)

	dirPrefix := "From"
	if isSend {
		dirPrefix = "To"
	}
	peerLbl := widgets.NewQLabel2(dirPrefix+" "+peerName, nil, 0)
	peerLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))

	statusText := fileTransferStatusText(entry)
	statusLbl := widgets.NewQLabel2(statusText, nil, 0)
	statusLbl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", fileStatusColor(statusText)))

	speedText := ""
	if entry.SpeedBytesPerSec > 0 {
		speedText = fmt.Sprintf("%s/s", formatBytes(int64(entry.SpeedBytesPerSec)))
	}
	etaText := ""
	if entry.ETASeconds > 0 {
		etaText = "ETA " + (time.Duration(entry.ETASeconds) * time.Second).Round(time.Second).String()
	}

	row2.AddWidget(peerLbl, 0, 0)
	row2.AddWidget(statusLbl, 0, 0)
	if speedText != "" {
		sl := widgets.NewQLabel2(speedText, nil, 0)
		sl.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))
		row2.AddWidget(sl, 0, 0)
	}
	if etaText != "" {
		el := widgets.NewQLabel2(etaText, nil, 0)
		el.SetStyleSheet(fmt.Sprintf("color: %s; font-size: 11px; background: transparent;", colorOverlay1))
		row2.AddWidget(el, 0, 0)
	}
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
		bar.SetFixedHeight(4)
		bar.SetStyleSheet(fmt.Sprintf(
			"QProgressBar { background: %s; border: none; border-radius: 2px; margin-left: 22px; } QProgressBar::chunk { background: %s; border-radius: 2px; }",
			colorSurface2, fileStatusColor(statusText),
		))
		layout.AddWidget(bar, 0, 0)
	}

	card.SetLayout(layout)
	return card
}

// itemHeightForWidget returns a reasonable size hint height for a widget
// being placed inside a QListWidgetItem.
func itemHeightForWidget(w *widgets.QWidget, minHeight int) int {
	h := w.SizeHint().Height()
	if h < minHeight {
		h = minHeight
	}
	return h
}

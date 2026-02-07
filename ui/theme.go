package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// UI color palette.
var (
	colorOnline      = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colorOffline     = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	colorOutgoingMsg = color.NRGBA{R: 28, G: 68, B: 120, A: 255}
	colorIncomingMsg = color.NRGBA{R: 52, G: 53, B: 58, A: 255}
	colorPanelBg     = color.NRGBA{R: 42, G: 43, B: 48, A: 255}
	colorMuted       = color.NRGBA{R: 155, G: 155, B: 160, A: 255}
)

// newRoundedBg creates a container with a rounded colored rectangle behind the content.
func newRoundedBg(bgColor color.Color, radius float32, content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = radius
	return container.NewStack(bg, container.NewPadded(content))
}

// newStatusDot creates a small colored circle indicating online/offline status.
func newStatusDot(online bool) (*canvas.Circle, fyne.CanvasObject) {
	c := colorOffline
	if online {
		c = colorOnline
	}
	dot := canvas.NewCircle(c)
	wrapped := container.NewGridWrap(fyne.NewSize(10, 10), dot)
	return dot, wrapped
}

type hoverHintOverlay struct {
	widget.BaseWidget
	onEnter func()
	onLeave func()
}

func newHoverHintOverlay(onEnter, onLeave func()) *hoverHintOverlay {
	overlay := &hoverHintOverlay{
		onEnter: onEnter,
		onLeave: onLeave,
	}
	overlay.ExtendBaseWidget(overlay)
	return overlay
}

func (h *hoverHintOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (h *hoverHintOverlay) MouseIn(*desktop.MouseEvent) {
	if h.onEnter != nil {
		h.onEnter()
	}
}

func (h *hoverHintOverlay) MouseMoved(*desktop.MouseEvent) {}

func (h *hoverHintOverlay) MouseOut() {
	if h.onLeave != nil {
		h.onLeave()
	}
}

func withHoverStatusHint(control fyne.CanvasObject, hint string, onHoverChanged func(string)) fyne.CanvasObject {
	if control == nil || onHoverChanged == nil {
		return control
	}
	trimmedHint := strings.TrimSpace(hint)
	if trimmedHint == "" {
		return control
	}
	overlay := newHoverHintOverlay(
		func() {
			onHoverChanged(trimmedHint)
		},
		func() {
			onHoverChanged("")
		},
	)
	return container.NewStack(control, overlay)
}

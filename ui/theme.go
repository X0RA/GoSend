package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// UI color palette.
var (
	colorOnline      = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colorOffline     = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	colorOutgoingMsg = color.NRGBA{R: 28, G: 68, B: 120, A: 255}
	colorIncomingMsg = color.NRGBA{R: 52, G: 53, B: 58, A: 255}
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

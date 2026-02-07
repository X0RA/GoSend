package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// UI color palette.
var (
	colorOnline      = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colorOffline     = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	colorOutgoingMsg = color.NRGBA{R: 28, G: 68, B: 120, A: 255}
	colorIncomingMsg = color.NRGBA{R: 52, G: 53, B: 58, A: 255}
	colorDialogPanel = color.NRGBA{R: 44, G: 45, B: 50, A: 255}
	colorButtonFill  = color.NRGBA{R: 63, G: 65, B: 72, A: 255}
	colorButtonMuted = color.NRGBA{R: 52, G: 53, B: 58, A: 255}
	colorMuted       = color.NRGBA{R: 155, G: 155, B: 160, A: 255}
)

// newRoundedBg creates a container with a rounded colored rectangle behind the content.
func newRoundedBg(bgColor color.Color, radius float32, content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = radius
	return container.NewStack(bg, container.NewPadded(content))
}

type roundedButton struct {
	*fyne.Container
	button        *widget.Button
	bg            *canvas.Rectangle
	enabledColor  color.Color
	disabledColor color.Color
}

func newRoundedButton(label string, icon fyne.Resource, radius float32, enabledColor, disabledColor color.Color, tapped func()) *roundedButton {
	button := widget.NewButton(label, tapped)
	button.Icon = icon
	button.Importance = widget.LowImportance

	bg := canvas.NewRectangle(enabledColor)
	bg.CornerRadius = radius

	content := container.NewPadded(button)
	return &roundedButton{
		Container:     container.NewStack(bg, content),
		button:        button,
		bg:            bg,
		enabledColor:  enabledColor,
		disabledColor: disabledColor,
	}
}

func (b *roundedButton) SetText(text string) {
	b.button.SetText(text)
}

func (b *roundedButton) SetOnTapped(tapped func()) {
	b.button.OnTapped = tapped
}

func (b *roundedButton) Enable() {
	b.button.Enable()
	b.bg.FillColor = b.enabledColor
	b.bg.Refresh()
}

func (b *roundedButton) Disable() {
	b.button.Disable()
	b.bg.FillColor = b.disabledColor
	b.bg.Refresh()
}

func (b *roundedButton) Refresh() {
	b.button.Refresh()
	b.bg.Refresh()
	b.Container.Refresh()
}

func newRoundedHintButton(label string, icon fyne.Resource, hint string, radius float32, bgColor color.Color, tapped func(), onHoverChanged func(target fyne.CanvasObject, hint string, active bool)) fyne.CanvasObject {
	button := newHintButtonWithIcon(label, icon, hint, tapped, onHoverChanged)
	button.Importance = widget.LowImportance
	return newRoundedBg(bgColor, radius, button)
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

type hintButton struct {
	widget.Button
	hint           string
	onHoverChanged func(target fyne.CanvasObject, hint string, active bool)
}

func newHintButton(label, hint string, tapped func(), onHoverChanged func(target fyne.CanvasObject, hint string, active bool)) *hintButton {
	return newHintButtonWithIcon(label, nil, hint, tapped, onHoverChanged)
}

func newHintButtonWithIcon(label string, icon fyne.Resource, hint string, tapped func(), onHoverChanged func(target fyne.CanvasObject, hint string, active bool)) *hintButton {
	btn := &hintButton{
		hint:           strings.TrimSpace(hint),
		onHoverChanged: onHoverChanged,
	}
	btn.Text = label
	btn.Icon = icon
	btn.OnTapped = tapped
	btn.ExtendBaseWidget(btn)
	return btn
}

func (b *hintButton) MouseIn(ev *desktop.MouseEvent) {
	b.Button.MouseIn(ev)
	if b.onHoverChanged != nil {
		b.onHoverChanged(b, b.hint, true)
	}
}

func (b *hintButton) MouseMoved(ev *desktop.MouseEvent) {
	b.Button.MouseMoved(ev)
}

func (b *hintButton) MouseOut() {
	b.Button.MouseOut()
	if b.onHoverChanged != nil {
		b.onHoverChanged(b, "", false)
	}
}

func themedColor(name fyne.ThemeColorName) color.Color {
	app := fyne.CurrentApp()
	if app == nil {
		return colorIncomingMsg
	}
	return app.Settings().Theme().Color(name, app.Settings().ThemeVariant())
}

// gosendTheme wraps the default theme so popups (e.g. tooltips) use the dark
// palette instead of the default slate overlay.
type gosendTheme struct {
	fyne.Theme
}

func (t *gosendTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameOverlayBackground:
		return colorDialogPanel
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 128}
	default:
		return t.Theme.Color(name, variant)
	}
}

// newGoSendTheme returns a theme that matches the app's dark UI for overlays and tooltips.
func newGoSendTheme() fyne.Theme {
	return &gosendTheme{Theme: theme.DefaultTheme()}
}

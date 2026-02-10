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

// Catppuccin Mocha palette (see COLORS.md).
var (
	ctpCrust    = color.NRGBA{R: 17, G: 17, B: 27, A: 255}
	ctpMantle   = color.NRGBA{R: 24, G: 24, B: 37, A: 255}
	ctpBase     = color.NRGBA{R: 30, G: 30, B: 46, A: 255}
	ctpSurface0 = color.NRGBA{R: 49, G: 50, B: 68, A: 255}
	ctpSurface1 = color.NRGBA{R: 69, G: 71, B: 90, A: 255}
	ctpSurface2 = color.NRGBA{R: 88, G: 91, B: 112, A: 255}
	ctpOverlay0 = color.NRGBA{R: 108, G: 112, B: 134, A: 255}
	ctpOverlay1 = color.NRGBA{R: 127, G: 132, B: 156, A: 255}
	ctpOverlay2 = color.NRGBA{R: 147, G: 153, B: 178, A: 255}
	ctpSubtext0 = color.NRGBA{R: 166, G: 173, B: 200, A: 255}
	ctpSubtext1 = color.NRGBA{R: 186, G: 194, B: 222, A: 255}
	ctpText     = color.NRGBA{R: 205, G: 214, B: 244, A: 255}
	ctpBlue     = color.NRGBA{R: 137, G: 180, B: 250, A: 255}
	ctpGreen    = color.NRGBA{R: 166, G: 227, B: 161, A: 255}
	ctpRed      = color.NRGBA{R: 243, G: 139, B: 168, A: 255}
	ctpYellow   = color.NRGBA{R: 249, G: 226, B: 175, A: 255}
	ctpTeal     = color.NRGBA{R: 148, G: 226, B: 213, A: 255}
	ctpPeach    = color.NRGBA{R: 250, G: 179, B: 135, A: 255}
	ctpMauve    = color.NRGBA{R: 203, G: 166, B: 247, A: 255}
	ctpLavender = color.NRGBA{R: 180, G: 190, B: 254, A: 255}
)

// Backward-compatible aliases for existing code.
var (
	colorOnline      = ctpGreen
	colorOffline     = ctpOverlay0
	colorOutgoingMsg = ctpSurface0
	colorIncomingMsg = ctpSurface0
	colorDialogPanel = ctpSurface0
	colorButtonFill  = ctpSurface0
	colorButtonMuted = ctpSurface1
	colorMuted       = ctpOverlay1
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

// flatButton is a text-like button: transparent/mantle by default, surface0 on hover. Icon-only or icon+label.
type flatButton struct {
	widget.BaseWidget
	icon       fyne.Resource
	label      string
	hint       string
	onClick    func()
	onHover    func(fyne.CanvasObject, string, bool)
	hovered    bool
	usePrimary bool // true = icon in primary (blue) color
}

// flatButtonRenderer implements fyne.WidgetRenderer for flatButton.
type flatButtonRenderer struct {
	rect  *canvas.Rectangle
	inner fyne.CanvasObject
	btn   *flatButton
}

func (r *flatButtonRenderer) Layout(size fyne.Size) {
	r.rect.Move(fyne.NewPos(0, 0))
	r.rect.Resize(size)
	r.inner.Move(fyne.NewPos(0, 0))
	r.inner.Resize(size)
}

func (r *flatButtonRenderer) MinSize() fyne.Size {
	return r.inner.MinSize()
}

func (r *flatButtonRenderer) Refresh() {
	if r.btn.hovered {
		r.rect.FillColor = ctpSurface0
	} else {
		r.rect.FillColor = color.Transparent
	}
	r.rect.CornerRadius = 4
	r.rect.Refresh()
}

func (r *flatButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect, r.inner}
}

func (r *flatButtonRenderer) Destroy() {}

func newFlatButtonWithIcon(icon fyne.Resource, hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) *flatButton {
	b := &flatButton{
		icon:    icon,
		hint:    hint,
		onClick: onClick,
		onHover: onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

// newFlatButtonWithIconAndLabel creates a flat (text-like) button with icon and label for toolbar/status bar.
func newFlatButtonWithIconAndLabel(icon fyne.Resource, label string, hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) *flatButton {
	b := &flatButton{
		icon:    icon,
		label:   label,
		hint:    hint,
		onClick: onClick,
		onHover: onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *flatButton) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	rect.CornerRadius = 4
	icon := widget.NewIcon(b.icon)
	// Wrap icon so the button has a minimum tap target (theme icon size is typically 24px)
	var inner fyne.CanvasObject
	if b.label != "" {
		lbl := canvas.NewText(b.label, ctpSubtext1)
		lbl.TextSize = 12
		inner = container.NewPadded(container.NewHBox(icon, lbl))
	} else {
		iconBox := container.NewCenter(icon)
		inner = container.NewPadded(iconBox)
	}
	return &flatButtonRenderer{rect: rect, inner: inner, btn: b}
}

func (b *flatButton) Tapped(*fyne.PointEvent) {
	if b.onClick != nil {
		b.onClick()
	}
}

func (b *flatButton) MouseIn(*desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
	if b.onHover != nil {
		b.onHover(b, b.hint, true)
	}
}

func (b *flatButton) MouseMoved(*desktop.MouseEvent) {
	// required by desktop.Hoverable interface
}

func (b *flatButton) MouseOut() {
	b.hovered = false
	b.Refresh()
	if b.onHover != nil {
		b.onHover(b, "", false)
	}
}

func themedColor(name fyne.ThemeColorName) color.Color {
	app := fyne.CurrentApp()
	if app == nil {
		return ctpText
	}
	return app.Settings().Theme().Color(name, app.Settings().ThemeVariant())
}

// gosendTheme wraps the default theme so the app uses Catppuccin Mocha dark palette.
type gosendTheme struct {
	fyne.Theme
}

func (t *gosendTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return ctpBase
	case theme.ColorNameButton:
		return ctpSurface0
	case theme.ColorNameDisabled:
		return ctpOverlay0
	case theme.ColorNameForeground:
		return ctpText
	case theme.ColorNameHeaderBackground:
		return ctpMantle
	case theme.ColorNameInputBackground:
		return ctpSurface0
	case theme.ColorNameInputBorder:
		return ctpSurface2
	case theme.ColorNameMenuBackground:
		return ctpMantle
	case theme.ColorNameOverlayBackground:
		return ctpMantle
	case theme.ColorNamePlaceHolder:
		return ctpOverlay1
	case theme.ColorNamePrimary:
		return ctpBlue
	case theme.ColorNameScrollBar:
		return ctpSurface2
	case theme.ColorNameSeparator:
		return ctpSurface0
	case theme.ColorNameShadow:
		return color.NRGBA{R: 17, G: 17, B: 27, A: 128}
	case theme.ColorNameHover:
		return ctpSurface1
	case theme.ColorNameFocus:
		return ctpBlue
	case theme.ColorNameForegroundOnPrimary:
		return ctpBase
	case theme.ColorNameSelection:
		return ctpSurface1
	case theme.ColorNameHyperlink:
		return ctpBlue
	default:
		return t.Theme.Color(name, variant)
	}
}

// newGoSendTheme returns a theme that matches the app's dark UI for overlays and tooltips.
func newGoSendTheme() fyne.Theme {
	return &gosendTheme{Theme: theme.DefaultTheme()}
}

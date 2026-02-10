package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
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

// newThinDivider renders a 1px horizontal divider line to match the mockup style.
func newThinDivider() fyne.CanvasObject {
	line := canvas.NewRectangle(ctpSurface0)
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
}

// withBottomDivider docks a 1px divider directly under content with no extra spacing.
func withBottomDivider(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&edgeDividerLayout{atTop: false}, content, newThinDivider())
}

// withTopDivider docks a 1px divider directly above content with no extra spacing.
func withTopDivider(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&edgeDividerLayout{atTop: true}, content, newThinDivider())
}

// newVNoGap stacks children vertically with no inter-item spacing.
func newVNoGap(objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(&vNoGapLayout{}, objects...)
}

// newNoGapBorder lays out top, center, bottom regions with zero inter-region padding.
func newNoGapBorder(top, bottom, center fyne.CanvasObject) fyne.CanvasObject {
	if top == nil {
		top = canvas.NewRectangle(color.Transparent)
	}
	if bottom == nil {
		bottom = canvas.NewRectangle(color.Transparent)
	}
	if center == nil {
		center = canvas.NewRectangle(color.Transparent)
	}
	return container.New(&noGapBorderLayout{}, top, center, bottom)
}

type edgeDividerLayout struct {
	atTop bool
}

func (l *edgeDividerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	content := objects[0]
	divider := objects[1]
	dividerHeight := divider.MinSize().Height
	if dividerHeight < 1 {
		dividerHeight = 1
	}
	if dividerHeight > size.Height {
		dividerHeight = size.Height
	}

	if l.atTop {
		divider.Move(fyne.NewPos(0, 0))
		divider.Resize(fyne.NewSize(size.Width, dividerHeight))
		content.Move(fyne.NewPos(0, dividerHeight))
		content.Resize(fyne.NewSize(size.Width, size.Height-dividerHeight))
		return
	}

	content.Move(fyne.NewPos(0, 0))
	content.Resize(fyne.NewSize(size.Width, size.Height-dividerHeight))
	divider.Move(fyne.NewPos(0, size.Height-dividerHeight))
	divider.Resize(fyne.NewSize(size.Width, dividerHeight))
}

func (l *edgeDividerLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 2 {
		return fyne.NewSize(0, 0)
	}
	contentMin := objects[0].MinSize()
	dividerMin := objects[1].MinSize()
	width := contentMin.Width
	if dividerMin.Width > width {
		width = dividerMin.Width
	}
	return fyne.NewSize(width, contentMin.Height+dividerMin.Height)
}

type vNoGapLayout struct{}

func (l *vNoGapLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, obj := range objects {
		if obj == nil || !obj.Visible() {
			continue
		}
		h := obj.MinSize().Height
		obj.Move(fyne.NewPos(0, y))
		obj.Resize(fyne.NewSize(size.Width, h))
		y += h
	}
}

func (l *vNoGapLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	height := float32(0)
	for _, obj := range objects {
		if obj == nil || !obj.Visible() {
			continue
		}
		min := obj.MinSize()
		if min.Width > width {
			width = min.Width
		}
		height += min.Height
	}
	return fyne.NewSize(width, height)
}

type noGapBorderLayout struct{}

func (l *noGapBorderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 3 {
		return
	}
	top := objects[0]
	center := objects[1]
	bottom := objects[2]

	topHeight := objectVisibleMinHeight(top)
	bottomHeight := objectVisibleMinHeight(bottom)

	if topHeight+bottomHeight > size.Height {
		scale := size.Height / (topHeight + bottomHeight)
		topHeight *= scale
		bottomHeight *= scale
	}
	centerHeight := size.Height - topHeight - bottomHeight
	if centerHeight < 0 {
		centerHeight = 0
	}

	top.Move(fyne.NewPos(0, 0))
	top.Resize(fyne.NewSize(size.Width, topHeight))

	center.Move(fyne.NewPos(0, topHeight))
	center.Resize(fyne.NewSize(size.Width, centerHeight))

	bottom.Move(fyne.NewPos(0, topHeight+centerHeight))
	bottom.Resize(fyne.NewSize(size.Width, bottomHeight))
}

func (l *noGapBorderLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 3 {
		return fyne.NewSize(0, 0)
	}
	top := objects[0]
	center := objects[1]
	bottom := objects[2]

	topMin := top.MinSize()
	centerMin := center.MinSize()
	bottomMin := bottom.MinSize()

	width := topMin.Width
	if centerMin.Width > width {
		width = centerMin.Width
	}
	if bottomMin.Width > width {
		width = bottomMin.Width
	}

	height := objectVisibleMinHeight(top) + objectVisibleMinHeight(center) + objectVisibleMinHeight(bottom)
	return fyne.NewSize(width, height)
}

func objectVisibleMinHeight(obj fyne.CanvasObject) float32 {
	if obj == nil || !obj.Visible() {
		return 0
	}
	return obj.MinSize().Height
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

type hoverSurface struct {
	widget.BaseWidget
	content      fyne.CanvasObject
	baseColor    color.Color
	hoverColor   color.Color
	cornerRadius float32
	hovered      bool
}

type hoverSurfaceRenderer struct {
	surface *hoverSurface
	bg      *canvas.Rectangle
}

func newHoverSurface(content fyne.CanvasObject, baseColor, hoverColor color.Color, cornerRadius float32) fyne.CanvasObject {
	if content == nil {
		content = canvas.NewRectangle(color.Transparent)
	}
	if baseColor == nil {
		baseColor = color.Transparent
	}
	if hoverColor == nil {
		hoverColor = baseColor
	}
	h := &hoverSurface{
		content:      content,
		baseColor:    baseColor,
		hoverColor:   hoverColor,
		cornerRadius: cornerRadius,
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hoverSurface) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(h.baseColor)
	bg.CornerRadius = h.cornerRadius
	return &hoverSurfaceRenderer{
		surface: h,
		bg:      bg,
	}
}

func (h *hoverSurface) MouseIn(*desktop.MouseEvent) {
	h.hovered = true
	h.Refresh()
}

func (h *hoverSurface) MouseMoved(*desktop.MouseEvent) {}

func (h *hoverSurface) MouseOut() {
	h.hovered = false
	h.Refresh()
}

func (r *hoverSurfaceRenderer) Layout(size fyne.Size) {
	r.bg.Move(fyne.NewPos(0, 0))
	r.bg.Resize(size)
	r.surface.content.Move(fyne.NewPos(0, 0))
	r.surface.content.Resize(size)
}

func (r *hoverSurfaceRenderer) MinSize() fyne.Size {
	return r.surface.content.MinSize()
}

func (r *hoverSurfaceRenderer) Refresh() {
	if r.surface.hovered {
		r.bg.FillColor = r.surface.hoverColor
	} else {
		r.bg.FillColor = r.surface.baseColor
	}
	r.bg.CornerRadius = r.surface.cornerRadius
	r.bg.Refresh()
	r.surface.content.Refresh()
}

func (r *hoverSurfaceRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.surface.content}
}

func (r *hoverSurfaceRenderer) Destroy() {}

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
	labelSize  float32
	iconSize   float32
	padTop     float32
	padBottom  float32
	padLeft    float32
	padRight   float32
	compact    bool
	enabled    bool
	labelColor color.Color
	disabled   color.Color
	hoverColor color.Color
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
	if r.btn.enabled && r.btn.hovered {
		hover := r.btn.hoverColor
		if hover == nil {
			hover = ctpSurface0
		}
		r.rect.FillColor = hover
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
		icon:       icon,
		enabled:    true,
		labelColor: ctpSubtext1,
		disabled:   ctpSurface2,
		hoverColor: ctpSurface0,
		hint:       hint,
		onClick:    onClick,
		onHover:    onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

// newFlatButtonWithIconAndLabel creates a flat (text-like) button with icon and label for toolbar/status bar.
func newFlatButtonWithIconAndLabel(icon fyne.Resource, label string, hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) *flatButton {
	b := &flatButton{
		icon:       icon,
		label:      label,
		enabled:    true,
		labelColor: ctpSubtext1,
		disabled:   ctpSurface2,
		hoverColor: ctpSurface0,
		hint:       hint,
		onClick:    onClick,
		onHover:    onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

// newCompactFlatButtonWithIcon creates a smaller icon-only flat button.
func newCompactFlatButtonWithIcon(icon fyne.Resource, hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) *flatButton {
	b := &flatButton{
		icon:       icon,
		iconSize:   16,
		padTop:     1,
		padBottom:  1,
		padLeft:    4,
		padRight:   4,
		compact:    true,
		enabled:    true,
		labelColor: ctpSubtext1,
		disabled:   ctpSurface2,
		hoverColor: ctpSurface0,
		hint:       hint,
		onClick:    onClick,
		onHover:    onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

// newCompactFlatButtonWithIconAndLabel creates a smaller footer-style flat button.
func newCompactFlatButtonWithIconAndLabel(icon fyne.Resource, label string, hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) *flatButton {
	b := &flatButton{
		icon:      icon,
		label:     label,
		labelSize: 10,
		iconSize:  14,
		padTop:    1,
		padBottom: 1,
		padLeft:   6,
		padRight:  6,
		compact:   true,
		enabled:   true,
		// Footer/action link style.
		labelColor: ctpSubtext0,
		disabled:   ctpSurface2,
		hoverColor: ctpSurface0,
		hint:       hint,
		onClick:    onClick,
		onHover:    onHover,
	}
	b.ExtendBaseWidget(b)
	return b
}

func newCompactFlatButtonWithIconAndLabelState(icon fyne.Resource, label string, enabled bool, onClick func()) *flatButton {
	btn := newCompactFlatButtonWithIconAndLabel(icon, label, "", onClick, nil)
	btn.labelSize = 11
	btn.iconSize = 12
	btn.padTop = 2
	btn.padBottom = 2
	btn.padLeft = 8
	btn.padRight = 8
	btn.enabled = enabled
	btn.hoverColor = ctpMantle
	return btn
}

func (b *flatButton) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	rect.CornerRadius = 4
	icon := widget.NewIcon(b.icon)
	iconObj := fyne.CanvasObject(icon)
	if b.iconSize > 0 {
		iconObj = container.NewGridWrap(fyne.NewSize(b.iconSize, b.iconSize), icon)
	}
	// Wrap icon so the button has a minimum tap target (theme icon size is typically 24px)
	var inner fyne.CanvasObject
	if b.label != "" {
		textColor := b.labelColor
		if !b.enabled {
			textColor = b.disabled
		}
		lbl := canvas.NewText(b.label, textColor)
		if b.labelSize > 0 {
			lbl.TextSize = b.labelSize
		} else {
			lbl.TextSize = 12
		}
		if b.compact {
			row := container.New(layout.NewCustomPaddedHBoxLayout(4), container.NewCenter(iconObj), container.NewCenter(lbl))
			inner = container.New(layout.NewCustomPaddedLayout(b.padTop, b.padBottom, b.padLeft, b.padRight), row)
		} else {
			row := container.NewHBox(iconObj, lbl)
			inner = container.NewPadded(row)
		}
	} else {
		iconBox := container.NewCenter(iconObj)
		if b.compact {
			inner = container.New(layout.NewCustomPaddedLayout(b.padTop, b.padBottom, b.padLeft, b.padRight), iconBox)
		} else {
			inner = container.NewPadded(iconBox)
		}
	}
	return &flatButtonRenderer{rect: rect, inner: inner, btn: b}
}

func (b *flatButton) Tapped(*fyne.PointEvent) {
	if !b.enabled {
		return
	}
	if b.onClick != nil {
		b.onClick()
	}
}

func (b *flatButton) MouseIn(*desktop.MouseEvent) {
	if !b.enabled {
		return
	}
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
	if !b.enabled {
		return
	}
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

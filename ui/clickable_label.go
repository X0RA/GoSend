package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type clickableLabel struct {
	widget.BaseWidget
	text      string
	textStyle fyne.TextStyle
	color     color.Color
	onTapped  func()
}

func newClickableLabel(text string, onTapped func()) *clickableLabel {
	label := &clickableLabel{
		text:     text,
		color:    themedColor(theme.ColorNameForeground),
		onTapped: onTapped,
	}
	label.ExtendBaseWidget(label)
	return label
}

func (l *clickableLabel) SetText(text string) {
	if l.text == text {
		return
	}
	l.text = text
	l.Refresh()
}

func (l *clickableLabel) SetTextStyle(style fyne.TextStyle) {
	l.textStyle = style
	l.Refresh()
}

func (l *clickableLabel) SetColor(color color.Color) {
	l.color = color
	l.Refresh()
}

func (l *clickableLabel) Tapped(_ *fyne.PointEvent) {
	if l.onTapped != nil {
		l.onTapped()
	}
}

func (l *clickableLabel) CreateRenderer() fyne.WidgetRenderer {
	text := canvas.NewText(l.text, l.color)
	text.TextStyle = l.textStyle
	text.Alignment = fyne.TextAlignLeading
	return &clickableLabelRenderer{
		label: l,
		text:  text,
	}
}

type clickableLabelRenderer struct {
	label *clickableLabel
	text  *canvas.Text
}

func (r *clickableLabelRenderer) Layout(size fyne.Size) {
	r.text.Resize(size)
}

func (r *clickableLabelRenderer) MinSize() fyne.Size {
	return r.text.MinSize()
}

func (r *clickableLabelRenderer) Refresh() {
	r.text.Text = r.label.text
	r.text.TextStyle = r.label.textStyle
	r.text.Color = r.label.color
	r.text.Refresh()
}

func (r *clickableLabelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.text}
}

func (r *clickableLabelRenderer) Destroy() {}

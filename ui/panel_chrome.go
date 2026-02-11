package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type panelActionStyle int

const (
	panelActionSecondary panelActionStyle = iota
	panelActionPrimary
	panelActionDanger
)

func newPanelIconCloseButton(hint string, onClick func(), onHover func(fyne.CanvasObject, string, bool)) fyne.CanvasObject {
	btn := newCompactFlatButtonWithIcon(iconClose(), hint, onClick, onHover)
	btn.iconSize = 12
	btn.padTop = 0
	btn.padBottom = 0
	btn.padLeft = 0
	btn.padRight = 0
	btn.hoverColor = ctpSurface1
	// Keep a consistent square hit-target and avoid full-height stretch in header rows.
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(25, 25), btn))
}

func newPanelActionButton(label, hint string, style panelActionStyle, onClick func(), onHover func(fyne.CanvasObject, string, bool)) fyne.CanvasObject {
	btn := newCompactFlatButtonWithIconAndLabel(nil, label, hint, onClick, onHover)
	btn.labelSize = 10
	btn.padTop = 1
	btn.padBottom = 1
	btn.padLeft = 8
	btn.padRight = 8
	btn.hoverColor = ctpSurface1

	bg := canvas.NewRectangle(ctpSurface0)
	switch style {
	case panelActionPrimary:
		bg.FillColor = ctpBlue
		btn.labelColor = ctpBase
		btn.hoverColor = ctpLavender
	case panelActionDanger:
		bg.FillColor = ctpRed
		btn.labelColor = ctpBase
		btn.hoverColor = ctpPeach
	default:
		btn.labelColor = ctpSubtext0
	}
	bg.CornerRadius = 4
	return container.NewStack(bg, btn)
}

func newPanelHeader(title, subtitle string, onClose func(), onHover func(fyne.CanvasObject, string, bool)) fyne.CanvasObject {
	subtitleText := canvas.NewText(subtitle, ctpOverlay1)
	subtitleText.TextSize = 11
	titleText := canvas.NewText(title, ctpText)
	titleText.TextSize = 14
	titleText.TextStyle = fyne.TextStyle{Bold: true}
	headerLeft := container.NewVBox(titleText, subtitleText)
	closeBtn := newPanelIconCloseButton("Close panel", onClose, onHover)
	row := container.NewHBox(headerLeft, layout.NewSpacer(), closeBtn)
	bg := canvas.NewRectangle(ctpMantle)
	bg.SetMinSize(fyne.NewSize(1, 56))
	return withBottomDivider(container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(6, 6, 12, 10), row)))
}

func newPanelFooter(left, right fyne.CanvasObject) fyne.CanvasObject {
	if left == nil {
		left = canvas.NewRectangle(color.Transparent)
	}
	if right == nil {
		right = canvas.NewRectangle(color.Transparent)
	}
	row := container.NewBorder(nil, nil, left, right, nil)
	bg := canvas.NewRectangle(ctpMantle)
	bg.SetMinSize(fyne.NewSize(1, 42))
	return withTopDivider(container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(5, 5, 12, 12), row)))
}

func newPanelFrame(top, bottom, center fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(ctpBase)
	bg.CornerRadius = 6

	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = ctpSurface1
	border.StrokeWidth = 1
	border.CornerRadius = 6

	content := container.New(layout.NewCustomPaddedLayout(1, 1, 1, 1), newNoGapBorder(top, bottom, center))
	return container.NewStack(bg, content, border)
}

func newPanelPopup(window fyne.Window, panel fyne.CanvasObject, size fyne.Size) *widget.PopUp {
	popupContent := container.New(layout.NewCustomPaddedLayout(-4, -4, -4, -4), panel)
	pop := widget.NewModalPopUp(popupContent, window.Canvas())
	pop.Resize(size)
	return pop
}

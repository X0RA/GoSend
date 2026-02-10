package ui

import "fmt"

// ---------------------------------------------------------------------------
// Catppuccin Mocha – colour palette
//
// Every colour constant maps directly to the Catppuccin Mocha specification.
// Swap this block out for another Catppuccin flavour (Latte, Frappé, Macchiato)
// or any palette you like – the rest of the UI references only these names.
// ---------------------------------------------------------------------------

const (
	// Accent colours
	colorRosewater = "#f5e0dc"
	colorFlamingo  = "#f2cdcd"
	colorPink      = "#f5c2e7"
	colorMauve     = "#cba6f7"
	colorRed       = "#f38ba8"
	colorMaroon    = "#eba0ac"
	colorPeach     = "#fab387"
	colorYellow    = "#f9e2af"
	colorGreen     = "#a6e3a1"
	colorTeal      = "#94e2d5"
	colorSky       = "#89dceb"
	colorSapphire  = "#74c7ec"
	colorBlue      = "#89b4fa"
	colorLavender  = "#b4befe"

	// Text shades
	colorText     = "#cdd6f4"
	colorSubtext1 = "#bac2de"
	colorSubtext0 = "#a6adc8"

	// Overlay shades
	colorOverlay2 = "#9399b2"
	colorOverlay1 = "#7f849c"
	colorOverlay0 = "#6c7086"

	// Surface shades
	colorSurface2 = "#585b70"
	colorSurface1 = "#45475a"
	colorSurface0 = "#313244"

	// Background shades
	colorBase   = "#1e1e2e"
	colorMantle = "#181825"
	colorCrust  = "#11111b"
)

// Semantic aliases – change these to re-map roles without touching the palette.
const (
	themeBackground          = colorBase
	themeBackgroundSecondary = colorMantle
	themeBackgroundTertiary  = colorCrust

	themeSurface          = colorSurface0
	themeSurfaceSecondary = colorSurface1
	themeSurfaceTertiary  = colorSurface2

	themeText          = colorText
	themeTextSecondary = colorSubtext1
	themeTextMuted     = colorSubtext0
	themeTextDisabled  = colorOverlay0

	themeBorder         = colorSurface2
	themeBorderSubtle   = colorSurface1
	themeBorderInactive = colorOverlay0

	themeAccent      = colorBlue
	themeAccentHover = colorSapphire
	themeAccentText  = colorCrust

	themeSuccess = colorGreen
	themeWarning = colorYellow
	themeError   = colorRed
	themeInfo    = colorTeal
)

// themeStyleSheet returns the full Qt Style Sheet (QSS) for the application.
// Every widget type that appears in the app is explicitly styled so that no
// platform-default light colours leak through.
func themeStyleSheet() string {
	return fmt.Sprintf(`
/* =========================================================
   Global defaults
   ========================================================= */
* {
    color: %[1]s;
    font-size: 13px;
}

/* =========================================================
   Top-level containers
   ========================================================= */
QMainWindow {
    background: %[2]s;
}

QDialog {
    background: %[2]s;
    color: %[1]s;
}

QWidget {
    background: transparent;
    color: %[1]s;
}

/* =========================================================
   Labels
   ========================================================= */
QLabel {
    background: transparent;
    color: %[1]s;
    padding: 1px 0;
}

/* =========================================================
   Text inputs
   ========================================================= */
QLineEdit {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    padding: 5px 8px;
    selection-background-color: %[5]s;
    selection-color: %[6]s;
}

QLineEdit:focus {
    border: 1px solid %[7]s;
}

QLineEdit:disabled {
    background: %[8]s;
    color: %[9]s;
}

QTextEdit {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    padding: 4px;
    selection-background-color: %[5]s;
    selection-color: %[6]s;
}

QTextEdit:focus {
    border: 1px solid %[7]s;
}

/* =========================================================
   List widgets
   ========================================================= */
QListWidget {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    padding: 2px;
    outline: none;
}

QListWidget::item {
    background: transparent;
    color: %[1]s;
    border-radius: 4px;
    padding: 6px 8px;
    margin: 1px 2px;
}

QListWidget::item:selected {
    background: %[5]s;
    color: %[6]s;
}

QListWidget::item:hover:!selected {
    background: %[10]s;
}

/* =========================================================
   Buttons
   ========================================================= */
QPushButton {
    background: %[11]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    padding: 6px 14px;
    min-height: 20px;
}

QPushButton:hover {
    background: %[12]s;
    border: 1px solid %[7]s;
}

QPushButton:pressed {
    background: %[7]s;
    color: %[6]s;
}

QPushButton:disabled {
    background: %[8]s;
    color: %[9]s;
    border: 1px solid %[8]s;
}

/* =========================================================
   Combo boxes
   ========================================================= */
QComboBox {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    padding: 5px 10px;
    min-height: 20px;
}

QComboBox:hover {
    border: 1px solid %[7]s;
}

QComboBox::drop-down {
    subcontrol-origin: padding;
    subcontrol-position: top right;
    width: 24px;
    border-left: 1px solid %[4]s;
    border-top-right-radius: 6px;
    border-bottom-right-radius: 6px;
    background: %[11]s;
}

QComboBox::down-arrow {
    image: none;
    width: 0;
    height: 0;
    border-left: 5px solid transparent;
    border-right: 5px solid transparent;
    border-top: 6px solid %[1]s;
    margin-top: 2px;
}

QComboBox QAbstractItemView {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    selection-background-color: %[5]s;
    selection-color: %[6]s;
    outline: none;
    padding: 2px;
}

QComboBox:disabled {
    background: %[8]s;
    color: %[9]s;
}

/* =========================================================
   Check boxes
   ========================================================= */
QCheckBox {
    background: transparent;
    color: %[1]s;
    spacing: 8px;
}

QCheckBox::indicator {
    width: 18px;
    height: 18px;
    border: 2px solid %[4]s;
    border-radius: 4px;
    background: %[3]s;
}

QCheckBox::indicator:checked {
    background: %[7]s;
    border: 2px solid %[7]s;
}

QCheckBox::indicator:hover {
    border: 2px solid %[7]s;
}

QCheckBox::indicator:disabled {
    background: %[8]s;
    border: 2px solid %[8]s;
}

/* =========================================================
   Form layouts (row labels)
   ========================================================= */
QFormLayout {
    background: transparent;
}

/* =========================================================
   Dialog button box
   ========================================================= */
QDialogButtonBox {
    background: transparent;
}

/* =========================================================
   Splitter
   ========================================================= */
QSplitter::handle {
    background: %[4]s;
    width: 2px;
    margin: 2px 4px;
}

QSplitter::handle:hover {
    background: %[7]s;
}

/* =========================================================
   Status bar
   ========================================================= */
QStatusBar {
    background: %[13]s;
    color: %[14]s;
    border-top: 1px solid %[4]s;
    padding: 2px 6px;
}

QStatusBar QLabel {
    background: transparent;
    color: %[14]s;
}

QStatusBar QPushButton {
    padding: 3px 8px;
    font-size: 12px;
}

/* =========================================================
   Scroll bars
   ========================================================= */
QScrollBar:vertical {
    background: %[3]s;
    width: 12px;
    margin: 0;
    border-radius: 6px;
}

QScrollBar::handle:vertical {
    background: %[11]s;
    min-height: 30px;
    border-radius: 6px;
}

QScrollBar::handle:vertical:hover {
    background: %[12]s;
}

QScrollBar::add-line:vertical,
QScrollBar::sub-line:vertical {
    height: 0;
    background: none;
}

QScrollBar::add-page:vertical,
QScrollBar::sub-page:vertical {
    background: none;
}

QScrollBar:horizontal {
    background: %[3]s;
    height: 12px;
    margin: 0;
    border-radius: 6px;
}

QScrollBar::handle:horizontal {
    background: %[11]s;
    min-width: 30px;
    border-radius: 6px;
}

QScrollBar::handle:horizontal:hover {
    background: %[12]s;
}

QScrollBar::add-line:horizontal,
QScrollBar::sub-line:horizontal {
    width: 0;
    background: none;
}

QScrollBar::add-page:horizontal,
QScrollBar::sub-page:horizontal {
    background: none;
}

/* =========================================================
   Message boxes
   ========================================================= */
QMessageBox {
    background: %[2]s;
    color: %[1]s;
}

QMessageBox QLabel {
    color: %[1]s;
}

/* =========================================================
   Tool tips
   ========================================================= */
QToolTip {
    background: %[10]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 4px;
    padding: 4px 8px;
}

/* =========================================================
   Tab widget (future-proofing)
   ========================================================= */
QTabWidget::pane {
    background: %[2]s;
    border: 1px solid %[4]s;
    border-radius: 4px;
}

QTabBar::tab {
    background: %[11]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    padding: 6px 16px;
    margin-right: 2px;
    border-top-left-radius: 6px;
    border-top-right-radius: 6px;
}

QTabBar::tab:selected {
    background: %[5]s;
    color: %[6]s;
}

QTabBar::tab:hover:!selected {
    background: %[12]s;
}

/* =========================================================
   Group box (future-proofing)
   ========================================================= */
QGroupBox {
    background: transparent;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 6px;
    margin-top: 14px;
    padding-top: 14px;
}

QGroupBox::title {
    subcontrol-origin: margin;
    subcontrol-position: top left;
    padding: 0 6px;
    color: %[14]s;
}

/* =========================================================
   Menu bar & menus (future-proofing)
   ========================================================= */
QMenuBar {
    background: %[13]s;
    color: %[1]s;
}

QMenuBar::item:selected {
    background: %[5]s;
    color: %[6]s;
}

QMenu {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    padding: 4px;
}

QMenu::item:selected {
    background: %[5]s;
    color: %[6]s;
}

QMenu::separator {
    height: 1px;
    background: %[4]s;
    margin: 4px 8px;
}

/* =========================================================
   Header view (table/tree header – future-proofing)
   ========================================================= */
QHeaderView::section {
    background: %[11]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    padding: 4px 8px;
}
`,
		themeText,                // [1]  primary text
		themeBackground,          // [2]  main background
		themeSurface,             // [3]  input/list background
		themeBorder,              // [4]  border
		themeAccent,              // [5]  selection / active bg
		themeAccentText,          // [6]  text on accent
		themeAccent,              // [7]  focus / checked accent (reuse blue)
		themeBorderSubtle,        // [8]  disabled background
		themeTextDisabled,        // [9]  disabled text
		themeSurfaceSecondary,    // [10] hover / tooltip bg
		themeSurfaceTertiary,     // [11] button / control bg
		colorOverlay0,            // [12] button hover
		themeBackgroundSecondary, // [13] status bar / menu bar bg
		themeTextSecondary,       // [14] status bar / secondary text
	)
}

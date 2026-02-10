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
// platform-default light colours leak through.  The styles closely follow the
// React mockup design that uses Catppuccin Mocha.
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
    border-radius: 4px;
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
    border-radius: 4px;
    padding: 4px;
    selection-background-color: %[5]s;
    selection-color: %[6]s;
}

QTextEdit:focus {
    border: 1px solid %[7]s;
}

/* =========================================================
   List widgets (generic)
   ========================================================= */
QListWidget {
    background: %[3]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 4px;
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
   Buttons (generic)
   ========================================================= */
QPushButton {
    background: %[11]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    border-radius: 4px;
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
    border-radius: 4px;
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
    border-top-right-radius: 4px;
    border-bottom-right-radius: 4px;
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
    width: 14px;
    height: 14px;
    border: 2px solid %[4]s;
    border-radius: 3px;
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
   Form layouts
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
    background: %[3]s;
    width: 1px;
    margin: 0;
}

QSplitter::handle:hover {
    background: %[7]s;
}

/* =========================================================
   Status bar  (mockup: crust bg, 11px overlay1 text)
   ========================================================= */
QStatusBar {
    background: %[16]s;
    color: %[18]s;
    border-top: 1px solid %[3]s;
    padding: 2px 8px;
    font-size: 11px;
}

QStatusBar QLabel {
    background: transparent;
    color: %[18]s;
    font-size: 11px;
}

QStatusBar QPushButton {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 2px 6px;
    font-size: 11px;
    color: %[18]s;
}

QStatusBar QPushButton:hover {
    background: %[3]s;
}

/* =========================================================
   Scroll bars (thin, subtle)
   ========================================================= */
QScrollBar:vertical {
    background: transparent;
    width: 8px;
    margin: 0;
    border-radius: 4px;
}

QScrollBar::handle:vertical {
    background: %[11]s;
    min-height: 30px;
    border-radius: 4px;
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
    background: transparent;
    height: 8px;
    margin: 0;
    border-radius: 4px;
}

QScrollBar::handle:horizontal {
    background: %[11]s;
    min-width: 30px;
    border-radius: 4px;
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
    border-top-left-radius: 4px;
    border-top-right-radius: 4px;
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
    border-radius: 4px;
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
   Header view (table/tree header)
   ========================================================= */
QHeaderView::section {
    background: %[11]s;
    color: %[1]s;
    border: 1px solid %[4]s;
    padding: 4px 8px;
}

/* =========================================================
   Toolbar  (mockup: mantle bg, transparent buttons)
   ========================================================= */
QWidget#toolbar {
    background: %[13]s;
    border-bottom: 1px solid %[3]s;
    border-radius: 0;
}

QWidget#toolbar QLabel#toolbarTitle {
    color: %[7]s;
    font-size: 14px;
    font-weight: bold;
    background: transparent;
}

QPushButton#toolbarBtn {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 4px 10px;
    font-size: 12px;
    color: %[14]s;
}

QPushButton#toolbarBtn:hover {
    background: %[3]s;
}

/* =========================================================
   Toolbar separator
   ========================================================= */
QFrame#toolbarSep {
    background: %[4]s;
}

/* =========================================================
   Icon buttons (search, peer settings in chat header)
   Mockup: overlay2 colour, transparent bg, hover surface0
   ========================================================= */
QPushButton#iconBtn {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 6px;
    font-size: 15px;
    color: %[17]s;
    min-height: 28px;
    min-width: 28px;
}

QPushButton#iconBtn:hover {
    background: %[3]s;
}

/* =========================================================
   Inline file-action buttons  (mockup: per-card controls)
   ========================================================= */
QPushButton#fileActionBtn {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 11px;
    color: %[14]s;
}

QPushButton#fileActionBtn:hover {
    background: %[13]s;
    color: %[1]s;
}

QPushButton#fileActionBtn:disabled {
    color: %[4]s;
    background: transparent;
}

/* =========================================================
   Global action row buttons  (fallback for global action row)
   ========================================================= */
QPushButton#actionBtn {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 4px 10px;
    font-size: 12px;
    color: %[14]s;
}

QPushButton#actionBtn:hover {
    background: %[3]s;
    color: %[1]s;
}

QPushButton#actionBtn:disabled {
    color: %[9]s;
    background: transparent;
}

/* =========================================================
   Peer list  (mockup: mantle bg, left-border selection)
   ========================================================= */
QListWidget#peerList {
    background: %[13]s;
    border: none;
    border-radius: 0;
    padding: 0;
}

QListWidget#peerList::item {
    background: transparent;
    border-radius: 0;
    border-left: 2px solid transparent;
    padding: 0;
    margin: 0;
}

QListWidget#peerList::item:selected {
    background: %[3]s;
    border-left: 2px solid %[7]s;
}

QListWidget#peerList::item:hover:!selected {
    background: %[3]s;
}

/* =========================================================
   Chat list  (mockup: base bg, no border, transparent items)
   ========================================================= */
QListWidget#chatList {
    background: %[2]s;
    border: none;
    border-radius: 0;
    padding: 8px;
}

QListWidget#chatList::item {
    background: transparent;
    border-radius: 4px;
    padding: 0;
    margin: 2px 0;
}

QListWidget#chatList::item:selected {
    background: transparent;
}

QListWidget#chatList::item:hover:!selected {
    background: transparent;
}

/* =========================================================
   Composer area  (mockup: mantle bg, top border)
   ========================================================= */
QWidget#composerRow {
    background: %[13]s;
    border-top: 1px solid %[3]s;
}

QWidget#composerRow QTextEdit {
    background: %[3]s;
    border: 1px solid %[4]s;
    border-radius: 4px;
    padding: 4px 10px;
    font-size: 13px;
}

QPushButton#composerBtn {
    background: transparent;
    color: %[7]s;
    border: none;
    border-radius: 4px;
    padding: 6px;
    font-size: 16px;
    min-width: 30px;
    min-height: 30px;
}

QPushButton#composerBtn:hover {
    background: %[3]s;
}

QPushButton#composerAttachBtn {
    background: transparent;
    border: none;
    border-radius: 4px;
    padding: 6px;
    font-size: 15px;
    color: %[17]s;
    min-width: 30px;
    min-height: 30px;
}

QPushButton#composerAttachBtn:hover {
    background: %[3]s;
}

/* =========================================================
   Chat header  (mockup: mantle bg, bottom border)
   ========================================================= */
QWidget#chatHeader {
    background: %[13]s;
    border-bottom: 1px solid %[3]s;
}

/* =========================================================
   Peers pane container  (mockup: mantle bg)
   ========================================================= */
QWidget#peersPane {
    background: %[13]s;
}

/* =========================================================
   PEERS header label (mockup: overlay2, uppercase)
   ========================================================= */
QLabel#peersHeaderLabel {
    color: %[17]s;
    font-size: 11px;
    font-weight: bold;
    background: transparent;
    padding: 4px 12px;
}

/* =========================================================
   Settings dialog sections  (mockup: mantle card bg)
   ========================================================= */
QWidget#settingsSection {
    background: %[13]s;
    border: 1px solid %[3]s;
    border-radius: 6px;
}

QLabel#sectionTitle {
    color: %[17]s;
    font-size: 11px;
    font-weight: bold;
    background: transparent;
}

QLabel#sectionHint {
    color: %[12]s;
    font-size: 10px;
    background: transparent;
}

/* =========================================================
   Dialog header & footer bars
   ========================================================= */
QWidget#dialogHeader {
    background: %[13]s;
    border-bottom: 1px solid %[3]s;
}

QWidget#dialogFooter {
    background: %[13]s;
    border-top: 1px solid %[3]s;
}

/* =========================================================
   Primary button  (mockup: blue bg, base text)
   ========================================================= */
QPushButton#primaryBtn {
    background: %[7]s;
    color: %[6]s;
    border: none;
    border-radius: 4px;
    padding: 6px 16px;
    font-size: 12px;
}

QPushButton#primaryBtn:hover {
    background: %[15]s;
}

/* =========================================================
   Secondary button  (mockup: surface0 bg, subtext0 text)
   ========================================================= */
QPushButton#secondaryBtn {
    background: %[3]s;
    color: %[14]s;
    border: none;
    border-radius: 4px;
    padding: 6px 12px;
    font-size: 12px;
}

QPushButton#secondaryBtn:hover {
    background: %[10]s;
}

/* =========================================================
   Danger button  (mockup: red-tinted bg)
   ========================================================= */
QPushButton#dangerBtn {
    background: rgba(243, 139, 168, 0.1);
    color: %[19]s;
    border: none;
    border-radius: 4px;
    padding: 6px 12px;
    font-size: 12px;
}

QPushButton#dangerBtn:hover {
    background: rgba(243, 139, 168, 0.2);
}
`,
		themeText,                // [1]  primary text
		themeBackground,          // [2]  main background (base)
		themeSurface,             // [3]  surface0 (input/list bg, also hover)
		themeBorder,              // [4]  border (surface2)
		themeAccent,              // [5]  selection / active bg (blue)
		themeAccentText,          // [6]  text on accent (crust)
		themeAccent,              // [7]  focus / checked accent (blue)
		themeBorderSubtle,        // [8]  disabled background (surface1)
		themeTextDisabled,        // [9]  disabled text (overlay0)
		themeSurfaceSecondary,    // [10] surface1 (hover / tooltip bg)
		themeSurfaceTertiary,     // [11] surface2 (button / control bg)
		colorOverlay0,            // [12] overlay0 (button hover, hints)
		themeBackgroundSecondary, // [13] mantle (toolbar, panels, header)
		themeTextSecondary,       // [14] subtext1 (secondary text)
		themeAccentHover,         // [15] sapphire (accent hover)
		themeBackgroundTertiary,  // [16] crust (status bar)
		colorOverlay2,            // [17] overlay2 (icon btn colour)
		colorOverlay1,            // [18] overlay1 (muted status text)
		themeError,               // [19] red (danger)
	)
}

package ui

import (
	"context"
	"errors"
	"sync"

	"github.com/therecipe/qt/widgets"

	"gosend/config"
	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

var errFilePickerCancelled = errors.New("file picker canceled")

const (
	maxSizePresetUseGlobal = "Use global default"
	maxSizePresetUnlimited = "Unlimited"
	maxSizePreset100MB     = "100 MB"
	maxSizePreset500MB     = "500 MB"
	maxSizePreset1GB       = "1 GB"
	maxSizePreset5GB       = "5 GB"
	maxSizePresetCustom    = "Custom"

	retentionPresetKeepForever = "Keep forever"
	retentionPreset30Days      = "30 days"
	retentionPreset90Days      = "90 days"
	retentionPreset1Year       = "1 year"
)

var maxSizePresetValues = map[string]int64{
	maxSizePreset100MB: 100 * 1024 * 1024,
	maxSizePreset500MB: 500 * 1024 * 1024,
	maxSizePreset1GB:   1024 * 1024 * 1024,
	maxSizePreset5GB:   5 * 1024 * 1024 * 1024,
}

var messageRetentionValues = map[string]int{
	retentionPresetKeepForever: 0,
	retentionPreset30Days:      30,
	retentionPreset90Days:      90,
	retentionPreset1Year:       365,
}

// RunOptions configures the GUI runtime.
type RunOptions struct {
	Config     *config.DeviceConfig
	ConfigPath string
	DataDir    string
	Store      *storage.Store
	Identity   network.LocalIdentity
}

type chatFileEntry struct {
	FileID            string
	FolderID          string
	RelativePath      string
	PeerDeviceID      string
	Direction         string
	Filename          string
	Filesize          int64
	Filetype          string
	StoredPath        string
	AddedAt           int64
	BytesTransferred  int64
	TotalBytes        int64
	SpeedBytesPerSec  float64
	ETASeconds        int64
	Status            string
	CompletedAt       int64
	TransferCompleted bool
	UpdatedAt         int64
}

type transcriptRow struct {
	isMessage bool
	message   storage.Message
	file      chatFileEntry
}

type controller struct {
	app    *widgets.QApplication
	window *widgets.QMainWindow

	cfg     *config.DeviceConfig
	cfgPath string
	dataDir string
	store   *storage.Store

	identity network.LocalIdentity

	discovery *discovery.Service
	manager   *network.PeerManager

	ctx        context.Context
	cancel     context.CancelFunc
	shutdownMu sync.Once
	loopsWg    sync.WaitGroup

	uiEvents chan func()

	keepAliveMu sync.Mutex
	keepAlive   []any

	peersMu        sync.RWMutex
	peers          []storage.Peer
	peerSettings   map[string]storage.PeerSettings
	selectedPeerID string

	discoveredMu sync.RWMutex
	discovered   map[string]discovery.DiscoveredPeer

	chatMu            sync.RWMutex
	chatMessages      []storage.Message
	chatFiles         []chatFileEntry
	fileTransfers     map[string]chatFileEntry
	chatSearchQuery   string
	chatSearchVisible bool
	chatFilesOnly     bool
	transcriptRows    []transcriptRow

	runtimeMu         sync.RWMutex
	peerRuntimeStates map[string]network.PeerRuntimeState

	statusMu      sync.RWMutex
	statusMessage string
	logsMu        sync.Mutex
	runtimeLogs   []string

	refreshMu      sync.Mutex
	refreshRunning bool

	activeListenPort int

	peerList         *widgets.QListWidget
	chatHeader       *widgets.QLabel
	peerSettingsBtn  *widgets.QPushButton
	searchBtn        *widgets.QPushButton
	chatSearchRow    *widgets.QWidget
	chatSearchInput  *widgets.QLineEdit
	chatFilesOnlyChk *widgets.QCheckBox
	chatList         *widgets.QListWidget
	messageInput     *widgets.QTextEdit
	chatComposer     *widgets.QWidget
	statusBar        *widgets.QStatusBar
	statusLabel      *widgets.QLabel
	trayIcon         *widgets.QSystemTrayIcon

	cancelBtn       *widgets.QPushButton
	retryBtn        *widgets.QPushButton
	showPathBtn     *widgets.QPushButton
	copyPathBtn     *widgets.QPushButton
	selectedFileID  string
	discoveryDialog *widgets.QDialog
	discoveryList   *widgets.QListWidget
}

// Run starts the Qt GUI runtime.

package ui

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"

	"gosend/discovery"
	"gosend/network"
	"gosend/storage"
)

func Run(options RunOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	configureQtEnvironment()

	app, err := newQApplication()
	if err != nil {
		return err
	}
	ctrl, err := newController(app, options)
	if err != nil {
		return err
	}
	return ctrl.run()
}

func newQApplication() (app *widgets.QApplication, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("initialize Qt application: %v", recovered)
		}
	}()

	app = widgets.NewQApplication(len(os.Args), os.Args)
	if app == nil {
		return nil, errors.New("initialize Qt application: QApplication is nil")
	}
	return app, nil
}

func (o RunOptions) validate() error {
	if o.Config == nil {
		return errors.New("config is required")
	}
	if o.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if o.DataDir == "" {
		return errors.New("data dir is required")
	}
	if o.Store == nil {
		return errors.New("store is required")
	}
	if o.Identity.DeviceID == "" || o.Identity.DeviceName == "" {
		return errors.New("identity device fields are required")
	}
	if len(o.Identity.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		return errors.New("identity Ed25519 private key is invalid")
	}
	if len(o.Identity.Ed25519PublicKey) != ed25519.PublicKeySize {
		return errors.New("identity Ed25519 public key is invalid")
	}
	return nil
}

func newController(app *widgets.QApplication, options RunOptions) (*controller, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ctrl := &controller{
		app:               app,
		window:            widgets.NewQMainWindow(nil, 0),
		cfg:               options.Config,
		cfgPath:           options.ConfigPath,
		dataDir:           options.DataDir,
		store:             options.Store,
		identity:          options.Identity,
		ctx:               ctx,
		cancel:            cancel,
		uiEvents:          make(chan func(), 512),
		discovered:        make(map[string]discovery.DiscoveredPeer),
		peerSettings:      make(map[string]storage.PeerSettings),
		fileTransfers:     make(map[string]chatFileEntry),
		peerRuntimeStates: make(map[string]network.PeerRuntimeState),
	}

	ctrl.window.SetWindowTitle("GoSend")
	ctrl.window.Resize2(1240, 780)
	ctrl.setupStyles()
	ctrl.buildMainWindow()
	ctrl.initTrayIcon()

	pump := core.NewQTimer(ctrl.window)
	pump.ConnectTimeout(ctrl.drainUIEvents)
	pump.Start(33)
	ctrl.hold(pump)

	if err := ctrl.startServices(); err != nil {
		ctrl.shutdown()
		return nil, err
	}
	ctrl.startLoops()
	ctrl.refreshPeersFromStore()
	ctrl.syncDiscoveredFromScanner()
	ctrl.reconnectDiscoveredKnownPeers()
	ctrl.refreshChatView()
	if ctrl.activeListenPort > 0 {
		ctrl.setStatus(fmt.Sprintf("Ready (listening on %d)", ctrl.activeListenPort))
	} else {
		ctrl.setStatus("Ready")
	}

	ctrl.window.ConnectCloseEvent(func(event *gui.QCloseEvent) {
		ctrl.shutdown()
		event.Accept()
	})

	return ctrl, nil
}

func (c *controller) run() error {
	if c.window == nil {
		return errors.New("main window is unavailable")
	}
	c.window.Show()
	code := c.app.Exec()
	c.shutdown()
	if code != 0 {
		return fmt.Errorf("qt event loop exited with code %d", code)
	}
	return nil
}

func (c *controller) shutdown() {
	c.shutdownMu.Do(func() {
		c.cancel()
		if c.trayIcon != nil {
			c.trayIcon.Hide()
		}
		if c.manager != nil {
			c.manager.Stop()
		}
		if c.discovery != nil {
			c.discovery.Stop()
		}
		c.loopsWg.Wait()
		if c.store != nil {
			_ = c.store.Close()
		}
	})
}

func (c *controller) setupStyles() {
	if c.app == nil {
		return
	}
	c.app.SetStyle2("Fusion")

	// Apply the Catppuccin Mocha dark palette via a Fusion QPalette so
	// native dialogs (QMessageBox, QFileDialog, etc.) that ignore QSS
	// still render with dark colours.
	palette := gui.NewQPalette()
	palette.SetColor2(gui.QPalette__Window, gui.NewQColor3(30, 30, 46, 255))
	palette.SetColor2(gui.QPalette__WindowText, gui.NewQColor3(205, 214, 244, 255))
	palette.SetColor2(gui.QPalette__Base, gui.NewQColor3(49, 50, 68, 255))
	palette.SetColor2(gui.QPalette__AlternateBase, gui.NewQColor3(69, 71, 90, 255))
	palette.SetColor2(gui.QPalette__ToolTipBase, gui.NewQColor3(69, 71, 90, 255))
	palette.SetColor2(gui.QPalette__ToolTipText, gui.NewQColor3(205, 214, 244, 255))
	palette.SetColor2(gui.QPalette__Text, gui.NewQColor3(205, 214, 244, 255))
	palette.SetColor2(gui.QPalette__Button, gui.NewQColor3(88, 91, 112, 255))
	palette.SetColor2(gui.QPalette__ButtonText, gui.NewQColor3(205, 214, 244, 255))
	palette.SetColor2(gui.QPalette__BrightText, gui.NewQColor3(243, 139, 168, 255))
	palette.SetColor2(gui.QPalette__Highlight, gui.NewQColor3(137, 180, 250, 255))
	palette.SetColor2(gui.QPalette__HighlightedText, gui.NewQColor3(17, 17, 27, 255))
	palette.SetColor2(gui.QPalette__Link, gui.NewQColor3(137, 180, 250, 255))
	palette.SetColor2(gui.QPalette__LinkVisited, gui.NewQColor3(180, 190, 254, 255))

	// Disabled group
	palette.SetColor(gui.QPalette__Disabled, gui.QPalette__WindowText, gui.NewQColor3(108, 112, 134, 255))
	palette.SetColor(gui.QPalette__Disabled, gui.QPalette__Text, gui.NewQColor3(108, 112, 134, 255))
	palette.SetColor(gui.QPalette__Disabled, gui.QPalette__ButtonText, gui.NewQColor3(108, 112, 134, 255))
	palette.SetColor(gui.QPalette__Disabled, gui.QPalette__Button, gui.NewQColor3(69, 71, 90, 255))

	c.app.SetPalette(palette, "")

	// Layer detailed QSS on top for border-radius, padding, and fine control.
	c.app.SetStyleSheet(themeStyleSheet())
}

func configureQtEnvironment() {
	os.Setenv("QT_STYLE_OVERRIDE", "Fusion")

	pluginsRoot := strings.TrimSpace(os.Getenv("QT_PLUGIN_PATH"))
	if pluginsRoot == "" {
		out, err := exec.Command("qmake", "-query", "QT_INSTALL_PLUGINS").Output()
		if err == nil {
			pluginsRoot = strings.TrimSpace(string(out))
			if pluginsRoot != "" {
				os.Setenv("QT_PLUGIN_PATH", pluginsRoot)
			}
		}
	}
	platformPath := strings.TrimSpace(os.Getenv("QT_QPA_PLATFORM_PLUGIN_PATH"))
	if platformPath == "" && pluginsRoot != "" {
		os.Setenv("QT_QPA_PLATFORM_PLUGIN_PATH", filepath.Join(pluginsRoot, "platforms"))
	}

	if runtime.GOOS == "linux" {
		selectedPlatform := strings.TrimSpace(os.Getenv("QT_QPA_PLATFORM"))
		if selectedPlatform == "" || qtPlatformRequestsWayland(selectedPlatform) {
			// therecipe/qt's bundled qtbox runtime exposes xcb but commonly lacks a usable
			// Wayland plugin, so normalize to xcb to avoid startup aborts.
			os.Setenv("QT_QPA_PLATFORM", "xcb")
		}
	}
}

func qtPlatformRequestsWayland(value string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return r == ';' || r == ',' || r == ' '
	})
	for _, part := range parts {
		if strings.HasPrefix(strings.TrimSpace(part), "wayland") {
			return true
		}
	}
	return false
}

func (c *controller) hold(values ...any) {
	if c == nil || len(values) == 0 {
		return
	}
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()
	for _, value := range values {
		if value != nil {
			c.keepAlive = append(c.keepAlive, value)
		}
	}
}

func (c *controller) enqueueUI(fn func()) {
	if fn == nil {
		return
	}
	select {
	case c.uiEvents <- fn:
	default:
		go func() {
			select {
			case c.uiEvents <- fn:
			case <-c.ctx.Done():
			}
		}()
	}
}

func (c *controller) runOnUI(fn func()) error {
	if fn == nil {
		return nil
	}
	done := make(chan struct{})
	select {
	case <-c.ctx.Done():
		return errors.New("application is shutting down")
	case c.uiEvents <- func() {
		fn()
		close(done)
	}:
	}
	select {
	case <-c.ctx.Done():
		return errors.New("application is shutting down")
	case <-done:
		return nil
	}
}

func (c *controller) drainUIEvents() {
	for i := 0; i < 128; i++ {
		select {
		case fn := <-c.uiEvents:
			if fn != nil {
				fn()
			}
		default:
			return
		}
	}
}

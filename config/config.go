package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
)

const (
	// AppDirectoryName is the per-user application data directory name.
	AppDirectoryName = "p2p-chat"
	// DefaultListeningPort is the TCP port used when no user override exists.
	DefaultListeningPort = 9999
	// PortModeAutomatic picks an available port at launch.
	PortModeAutomatic = "automatic"
	// PortModeFixed uses the configured listening port value.
	PortModeFixed = "fixed"
	// configFileName is the persisted configuration file.
	configFileName = "config.json"
)

// DeviceConfig contains persistent local-device settings.
type DeviceConfig struct {
	DeviceID              string `json:"device_id"`
	DeviceName            string `json:"device_name"`
	PortMode              string `json:"port_mode"`
	ListeningPort         int    `json:"listening_port"`
	Ed25519PrivateKeyPath string `json:"ed25519_private_key_path"`
	Ed25519PublicKeyPath  string `json:"ed25519_public_key_path"`
	X25519PrivateKeyPath  string `json:"x25519_private_key_path"`
	KeyFingerprint        string `json:"key_fingerprint"`
}

// ResolveDataDir returns the OS-aware app data directory.
//
// If P2P_CHAT_DATA_DIR is set, its value is used as an explicit override.
func ResolveDataDir() (string, error) {
	if override := os.Getenv("P2P_CHAT_DATA_DIR"); override != "" {
		return override, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, AppDirectoryName), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", AppDirectoryName), nil
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, AppDirectoryName), nil
	}
}

// ConfigPath returns the full path to config.json for a data directory.
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, configFileName)
}

// EnsureDataDirectories creates the app data directory layout if needed.
func EnsureDataDirectories(dataDir string) error {
	dirs := []string{
		dataDir,
		filepath.Join(dataDir, "keys"),
		filepath.Join(dataDir, "files"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	return nil
}

// Load reads and unmarshals config.json from disk.
func Load(path string) (*DeviceConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg DeviceConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// Save marshals and writes config.json to disk.
func Save(path string, cfg *DeviceConfig) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// LoadOrCreate ensures directories and config exist, then returns both.
func LoadOrCreate() (*DeviceConfig, string, error) {
	dataDir, err := ResolveDataDir()
	if err != nil {
		return nil, "", err
	}
	if err := EnsureDataDirectories(dataDir); err != nil {
		return nil, "", err
	}

	cfgPath := ConfigPath(dataDir)
	cfg, err := Load(cfgPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, "", err
		}

		cfg, err = defaultConfig(dataDir)
		if err != nil {
			return nil, "", err
		}
		if err := Save(cfgPath, cfg); err != nil {
			return nil, "", err
		}

		return cfg, cfgPath, nil
	}

	if normalizeDefaults(cfg, dataDir) {
		if err := Save(cfgPath, cfg); err != nil {
			return nil, "", err
		}
	}

	return cfg, cfgPath, nil
}

func defaultConfig(dataDir string) (*DeviceConfig, error) {
	deviceName := "P2P Chat Device"
	if host, err := os.Hostname(); err == nil && host != "" {
		deviceName = host
	}

	keysDir := filepath.Join(dataDir, "keys")
	return &DeviceConfig{
		DeviceID:              uuid.NewString(),
		DeviceName:            deviceName,
		PortMode:              PortModeAutomatic,
		ListeningPort:         0,
		Ed25519PrivateKeyPath: filepath.Join(keysDir, "ed25519_private.pem"),
		Ed25519PublicKeyPath:  filepath.Join(keysDir, "ed25519_public.pem"),
		X25519PrivateKeyPath:  filepath.Join(keysDir, "x25519_private.pem"),
		KeyFingerprint:        "",
	}, nil
}

func normalizeDefaults(cfg *DeviceConfig, dataDir string) bool {
	updated := false
	keysDir := filepath.Join(dataDir, "keys")

	if cfg.DeviceID == "" {
		cfg.DeviceID = uuid.NewString()
		updated = true
	}

	if cfg.DeviceName == "" {
		deviceName := "P2P Chat Device"
		if host, err := os.Hostname(); err == nil && host != "" {
			deviceName = host
		}
		cfg.DeviceName = deviceName
		updated = true
	}

	mode := normalizePortMode(cfg.PortMode)
	if mode == "" {
		if cfg.ListeningPort > 0 {
			mode = PortModeFixed
		} else {
			mode = PortModeAutomatic
		}
	}
	if cfg.PortMode != mode {
		cfg.PortMode = mode
		updated = true
	}

	if cfg.PortMode == PortModeFixed && cfg.ListeningPort == 0 {
		cfg.ListeningPort = DefaultListeningPort
		updated = true
	}
	if cfg.PortMode == PortModeAutomatic && cfg.ListeningPort < 0 {
		cfg.ListeningPort = 0
		updated = true
	}

	if cfg.Ed25519PrivateKeyPath == "" {
		cfg.Ed25519PrivateKeyPath = filepath.Join(keysDir, "ed25519_private.pem")
		updated = true
	}

	if cfg.Ed25519PublicKeyPath == "" {
		cfg.Ed25519PublicKeyPath = filepath.Join(keysDir, "ed25519_public.pem")
		updated = true
	}

	if cfg.X25519PrivateKeyPath == "" {
		cfg.X25519PrivateKeyPath = filepath.Join(keysDir, "x25519_private.pem")
		updated = true
	}

	return updated
}

func normalizePortMode(mode string) string {
	switch mode {
	case PortModeAutomatic:
		return PortModeAutomatic
	case PortModeFixed:
		return PortModeFixed
	default:
		return ""
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreateCreatesAndReloadsConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("P2P_CHAT_DATA_DIR", tempDir)

	firstCfg, firstPath, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("first LoadOrCreate failed: %v", err)
	}
	if firstCfg.DeviceID == "" {
		t.Fatalf("expected non-empty device ID")
	}
	if firstCfg.PortMode != PortModeAutomatic {
		t.Fatalf("expected default port mode %q, got %q", PortModeAutomatic, firstCfg.PortMode)
	}
	if firstCfg.ListeningPort != 0 {
		t.Fatalf("expected automatic mode listening port 0, got %d", firstCfg.ListeningPort)
	}
	expectedDownloadDir := filepath.Join(tempDir, "files")
	if firstCfg.DownloadDirectory != expectedDownloadDir {
		t.Fatalf("expected default download directory %q, got %q", expectedDownloadDir, firstCfg.DownloadDirectory)
	}
	if firstCfg.MaxReceiveFileSize != 0 {
		t.Fatalf("expected default max receive file size 0, got %d", firstCfg.MaxReceiveFileSize)
	}

	expectedConfigPath := filepath.Join(tempDir, "config.json")
	if firstPath != expectedConfigPath {
		t.Fatalf("expected config path %q, got %q", expectedConfigPath, firstPath)
	}

	secondCfg, secondPath, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("second LoadOrCreate failed: %v", err)
	}

	if secondPath != firstPath {
		t.Fatalf("expected config path to be stable, got %q then %q", firstPath, secondPath)
	}
	if secondCfg.DeviceID != firstCfg.DeviceID {
		t.Fatalf("expected stable device ID, got %q then %q", firstCfg.DeviceID, secondCfg.DeviceID)
	}
	if secondCfg.Ed25519PrivateKeyPath != firstCfg.Ed25519PrivateKeyPath {
		t.Fatalf("expected stable key path, got %q then %q", firstCfg.Ed25519PrivateKeyPath, secondCfg.Ed25519PrivateKeyPath)
	}
	if secondCfg.PortMode != firstCfg.PortMode {
		t.Fatalf("expected stable port mode, got %q then %q", firstCfg.PortMode, secondCfg.PortMode)
	}
	if secondCfg.DownloadDirectory != firstCfg.DownloadDirectory {
		t.Fatalf("expected stable download directory, got %q then %q", firstCfg.DownloadDirectory, secondCfg.DownloadDirectory)
	}
	if secondCfg.MaxReceiveFileSize != firstCfg.MaxReceiveFileSize {
		t.Fatalf("expected stable max receive file size, got %d then %d", firstCfg.MaxReceiveFileSize, secondCfg.MaxReceiveFileSize)
	}
}

func TestLoadOrCreateNormalizesLegacyPortModeFromExistingPort(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("P2P_CHAT_DATA_DIR", tempDir)

	cfgPath := filepath.Join(tempDir, "config.json")
	if err := EnsureDataDirectories(tempDir); err != nil {
		t.Fatalf("EnsureDataDirectories failed: %v", err)
	}

	legacy := &DeviceConfig{
		DeviceID:              "legacy-device",
		DeviceName:            "Legacy",
		ListeningPort:         9999,
		Ed25519PrivateKeyPath: filepath.Join(tempDir, "keys", "ed25519_private.pem"),
		Ed25519PublicKeyPath:  filepath.Join(tempDir, "keys", "ed25519_public.pem"),
	}
	if err := Save(cfgPath, legacy); err != nil {
		t.Fatalf("Save legacy config failed: %v", err)
	}

	cfg, _, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}
	if cfg.PortMode != PortModeFixed {
		t.Fatalf("expected legacy config to normalize to fixed mode, got %q", cfg.PortMode)
	}
	if cfg.ListeningPort != 9999 {
		t.Fatalf("expected legacy fixed listening port to be retained, got %d", cfg.ListeningPort)
	}
	if cfg.DownloadDirectory != filepath.Join(tempDir, "files") {
		t.Fatalf("expected legacy config to default download directory to data files dir, got %q", cfg.DownloadDirectory)
	}
}

func TestLoadOrCreateMigratesLegacyX25519PathField(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("P2P_CHAT_DATA_DIR", tempDir)

	cfgPath := filepath.Join(tempDir, "config.json")
	if err := EnsureDataDirectories(tempDir); err != nil {
		t.Fatalf("EnsureDataDirectories failed: %v", err)
	}

	legacyConfig := `{
  "device_id": "legacy-device",
  "device_name": "Legacy Device",
  "port_mode": "automatic",
  "listening_port": 0,
  "ed25519_private_key_path": "` + filepath.Join(tempDir, "keys", "ed25519_private.pem") + `",
  "ed25519_public_key_path": "` + filepath.Join(tempDir, "keys", "ed25519_public.pem") + `",
  "x25519_private_key_path": "` + filepath.Join(tempDir, "keys", "x25519_private.pem") + `",
  "key_fingerprint": ""
}`
	if err := os.WriteFile(cfgPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatalf("write legacy config failed: %v", err)
	}

	if _, _, err := LoadOrCreate(); err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	updatedRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read migrated config failed: %v", err)
	}
	if strings.Contains(string(updatedRaw), "x25519_private_key_path") {
		t.Fatalf("expected x25519_private_key_path to be removed during migration")
	}
}

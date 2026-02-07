package config

import (
	"path/filepath"
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
		X25519PrivateKeyPath:  filepath.Join(tempDir, "keys", "x25519_private.pem"),
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
}

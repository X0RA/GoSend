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
	if firstCfg.ListeningPort != DefaultListeningPort {
		t.Fatalf("expected default listening port %d, got %d", DefaultListeningPort, firstCfg.ListeningPort)
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
}

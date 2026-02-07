package main

import (
	"log"
	"path/filepath"

	"gosend/config"
	"gosend/crypto"
	"gosend/network"
	"gosend/storage"
	"gosend/ui"
)

func main() {
	cfg, cfgPath, err := config.LoadOrCreate()
	if err != nil {
		log.Fatalf("startup failed while loading config: %v", err)
	}

	ed25519PrivateKey, ed25519PublicKey, err := crypto.EnsureEd25519KeyPair(cfg.Ed25519PrivateKeyPath, cfg.Ed25519PublicKeyPath)
	if err != nil {
		log.Fatalf("startup failed while preparing Ed25519 keypair: %v", err)
	}

	if _, err := crypto.EnsureX25519PrivateKey(cfg.X25519PrivateKeyPath); err != nil {
		log.Fatalf("startup failed while preparing X25519 keypair: %v", err)
	}

	fingerprint := crypto.KeyFingerprint(ed25519PublicKey)
	if cfg.KeyFingerprint != fingerprint {
		cfg.KeyFingerprint = fingerprint
		if err := config.Save(cfgPath, cfg); err != nil {
			log.Fatalf("startup failed while persisting key fingerprint: %v", err)
		}
	}

	dataDir := filepath.Dir(cfgPath)
	store, _, err := storage.Open(dataDir)
	if err != nil {
		log.Fatalf("startup failed while opening database: %v", err)
	}

	identity := network.LocalIdentity{
		DeviceID:          cfg.DeviceID,
		DeviceName:        cfg.DeviceName,
		Ed25519PrivateKey: ed25519PrivateKey,
		Ed25519PublicKey:  ed25519PublicKey,
	}

	if err := ui.Run(ui.RunOptions{
		Config:     cfg,
		ConfigPath: cfgPath,
		DataDir:    dataDir,
		Store:      store,
		Identity:   identity,
	}); err != nil {
		log.Fatalf("startup failed while running UI: %v", err)
	}
}

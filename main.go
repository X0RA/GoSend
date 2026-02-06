package main

import (
	"fmt"
	"log"
	"path/filepath"

	"gosend/config"
	"gosend/crypto"
)

func main() {
	cfg, cfgPath, err := config.LoadOrCreate()
	if err != nil {
		log.Fatalf("startup failed while loading config: %v", err)
	}

	_, ed25519PublicKey, err := crypto.EnsureEd25519KeyPair(cfg.Ed25519PrivateKeyPath, cfg.Ed25519PublicKeyPath)
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

	fmt.Printf("Device ID:       %s\n", cfg.DeviceID)
	fmt.Printf("Device Name:     %s\n", cfg.DeviceName)
	fmt.Printf("Listening Port:  %d\n", cfg.ListeningPort)
	fmt.Printf("Fingerprint:     %s\n", crypto.FormatFingerprint(cfg.KeyFingerprint))
	fmt.Printf("Config File:     %s\n", cfgPath)
	fmt.Printf("Data Directory:  %s\n", filepath.Dir(cfgPath))
}

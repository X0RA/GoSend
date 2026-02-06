package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"path/filepath"
	"syscall"

	"gosend/config"
	"gosend/crypto"
	"gosend/discovery"
	"gosend/storage"
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
	dataDir := filepath.Dir(cfgPath)
	fmt.Printf("Data Directory:  %s\n", dataDir)

	store, dbPath, err := storage.Open(dataDir)
	if err != nil {
		log.Fatalf("startup failed while opening database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("database close error: %v", err)
		}
	}()
	fmt.Printf("Database File:   %s\n", dbPath)

	discoveryService, err := discovery.Start(discovery.Config{
		SelfDeviceID:   cfg.DeviceID,
		DeviceName:     cfg.DeviceName,
		ListeningPort:  cfg.ListeningPort,
		KeyFingerprint: cfg.KeyFingerprint,
	})
	if err != nil {
		log.Printf("discovery startup failed: %v", err)
	} else {
		defer discoveryService.Stop()
		fmt.Println("Discovery:       running")
		go logDiscoveryEvents(discoveryService.Scanner.Events())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("Status:          running (press Ctrl+C to stop)")
	<-ctx.Done()
	fmt.Println("Status:          shutting down")
}

func logDiscoveryEvents(events <-chan discovery.Event) {
	for event := range events {
		switch event.Type {
		case discovery.EventPeerUpserted:
			log.Printf("discovery: peer available id=%s name=%q addr=%v port=%d",
				event.Peer.DeviceID, event.Peer.DeviceName, event.Peer.Addresses, event.Peer.Port)
		case discovery.EventPeerRemoved:
			log.Printf("discovery: peer removed id=%s", event.Peer.DeviceID)
		default:
			log.Printf("discovery: event=%s id=%s", event.Type, event.Peer.DeviceID)
		}
	}
}

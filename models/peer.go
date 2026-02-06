package models

// Peer represents a discovered or added remote device.
type Peer struct {
	DeviceID          string `json:"device_id"`
	DeviceName        string `json:"device_name"`
	Ed25519PublicKey  string `json:"ed25519_public_key"`
	KeyFingerprint    string `json:"key_fingerprint"`
	AddedTimestamp    int64  `json:"added_timestamp"`
	LastSeenTimestamp int64  `json:"last_seen_timestamp"`
	Status            string `json:"status"`
	LocalPort         int    `json:"local_port"`
	LocalIP           string `json:"local_ip"`
}

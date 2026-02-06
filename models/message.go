package models

// Message represents a plaintext message entry after decryption.
type Message struct {
	MessageID         string `json:"message_id"`
	FromDeviceID      string `json:"from_device_id"`
	ToDeviceID        string `json:"to_device_id"`
	Content           string `json:"content"`
	ContentType       string `json:"content_type"`
	TimestampSent     int64  `json:"timestamp_sent"`
	TimestampReceived int64  `json:"timestamp_received"`
	IsRead            bool   `json:"is_read"`
	Signature         string `json:"signature"`
}

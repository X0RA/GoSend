package network

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"type":"ping","from_device_id":"a","timestamp":1}`)

	var buffer bytes.Buffer
	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	got, err := ReadFrame(&buffer)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestWriteFrameRejectsOversizedPayload(t *testing.T) {
	payload := make([]byte, MaxFrameSize+1)
	var buffer bytes.Buffer
	if err := WriteFrame(&buffer, payload); err != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestReadControlFrameRejectsOversizedPayload(t *testing.T) {
	payload := make([]byte, MaxControlFrameSize+1)
	var buffer bytes.Buffer
	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	if _, err := ReadControlFrame(&buffer); err != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

package storage

import (
	"testing"
)

func TestFileCRUD(t *testing.T) {
	store := newTestStore(t)
	mustAddPeer(t, store, "self", "Self")
	mustAddPeer(t, store, "peer-1", "Peer One")

	if err := store.SaveMessage(Message{
		MessageID:      "msg-file-1",
		FromDeviceID:   "self",
		ToDeviceID:     "peer-1",
		Content:        "sending file",
		ContentType:    messageContentFile,
		TimestampSent:  nowUnixMilli(),
		DeliveryStatus: deliveryStatusPending,
	}); err != nil {
		t.Fatalf("SaveMessage prerequisite failed: %v", err)
	}

	receivedAt := nowUnixMilli()
	file := FileMetadata{
		FileID:            "file-1",
		MessageID:         "msg-file-1",
		FromDeviceID:      "self",
		ToDeviceID:        "peer-1",
		Filename:          "photo.png",
		Filesize:          2048,
		Filetype:          "image/png",
		StoredPath:        "/tmp/photo.png",
		Checksum:          "abc123",
		TimestampReceived: &receivedAt,
		TransferStatus:    transferStatusPending,
	}

	if err := store.SaveFileMetadata(file); err != nil {
		t.Fatalf("SaveFileMetadata failed: %v", err)
	}

	got, err := store.GetFileByID(file.FileID)
	if err != nil {
		t.Fatalf("GetFileByID failed: %v", err)
	}
	if got.Filename != file.Filename || got.Filesize != file.Filesize {
		t.Fatalf("unexpected file metadata: got %+v", got)
	}
	if got.TransferStatus != transferStatusPending {
		t.Fatalf("unexpected initial transfer status: %q", got.TransferStatus)
	}

	if err := store.UpdateTransferStatus(file.FileID, transferStatusComplete); err != nil {
		t.Fatalf("UpdateTransferStatus failed: %v", err)
	}
	updated, err := store.GetFileByID(file.FileID)
	if err != nil {
		t.Fatalf("GetFileByID after update failed: %v", err)
	}
	if updated.TransferStatus != transferStatusComplete {
		t.Fatalf("expected transfer status %q, got %q", transferStatusComplete, updated.TransferStatus)
	}
}

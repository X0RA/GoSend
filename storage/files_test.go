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

func TestTransferCheckpointCRUD(t *testing.T) {
	store := newTestStore(t)

	checkpoint := TransferCheckpoint{
		FileID:           "file-cp-1",
		Direction:        TransferDirectionSend,
		NextChunk:        42,
		BytesTransferred: 42 * 1024,
		TempPath:         "/tmp/source.bin",
	}
	if err := store.UpsertTransferCheckpoint(checkpoint); err != nil {
		t.Fatalf("UpsertTransferCheckpoint failed: %v", err)
	}

	got, err := store.GetTransferCheckpoint(checkpoint.FileID, checkpoint.Direction)
	if err != nil {
		t.Fatalf("GetTransferCheckpoint failed: %v", err)
	}
	if got.NextChunk != checkpoint.NextChunk || got.BytesTransferred != checkpoint.BytesTransferred {
		t.Fatalf("unexpected checkpoint: %+v", got)
	}

	checkpoint.NextChunk = 99
	checkpoint.BytesTransferred = 99 * 2048
	if err := store.UpsertTransferCheckpoint(checkpoint); err != nil {
		t.Fatalf("UpsertTransferCheckpoint update failed: %v", err)
	}

	updated, err := store.GetTransferCheckpoint(checkpoint.FileID, checkpoint.Direction)
	if err != nil {
		t.Fatalf("GetTransferCheckpoint after update failed: %v", err)
	}
	if updated.NextChunk != checkpoint.NextChunk || updated.BytesTransferred != checkpoint.BytesTransferred {
		t.Fatalf("unexpected updated checkpoint: %+v", updated)
	}

	listed, err := store.ListTransferCheckpoints(TransferDirectionSend)
	if err != nil {
		t.Fatalf("ListTransferCheckpoints failed: %v", err)
	}
	if len(listed) != 1 || listed[0].FileID != checkpoint.FileID {
		t.Fatalf("unexpected listed checkpoints: %+v", listed)
	}

	if err := store.DeleteTransferCheckpoint(checkpoint.FileID, checkpoint.Direction); err != nil {
		t.Fatalf("DeleteTransferCheckpoint failed: %v", err)
	}
	if _, err := store.GetTransferCheckpoint(checkpoint.FileID, checkpoint.Direction); err == nil {
		t.Fatalf("expected ErrNotFound after checkpoint deletion")
	}
}

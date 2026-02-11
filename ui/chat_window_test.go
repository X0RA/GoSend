package ui

import "testing"

func TestUpsertFileTransferActiveRetryClearsTerminalState(t *testing.T) {
	ctrl := &controller{
		fileTransfers: make(map[string]chatFileEntry),
	}

	ctrl.upsertFileTransfer(chatFileEntry{
		FileID:            "file-1",
		Status:            "canceled",
		TransferCompleted: true,
		BytesTransferred:  2048,
		TotalBytes:        4096,
		CompletedAt:       1000,
		AddedAt:           100,
	})

	ctrl.upsertFileTransfer(chatFileEntry{
		FileID:            "file-1",
		Status:            "accepted",
		TransferCompleted: false,
		BytesTransferred:  0,
	})

	got := ctrl.fileTransfers["file-1"]
	if got.TransferCompleted {
		t.Fatalf("expected retry entry to be active, got completed=true")
	}
	if got.Status != "accepted" {
		t.Fatalf("expected status accepted, got %q", got.Status)
	}
	if got.BytesTransferred != 0 {
		t.Fatalf("expected retry bytes to reset to 0, got %d", got.BytesTransferred)
	}
	if got.CompletedAt != 0 {
		t.Fatalf("expected retry completed_at reset to 0, got %d", got.CompletedAt)
	}
}

func TestMergeChatFileEntryLiveAcceptedOverridesTerminalBase(t *testing.T) {
	base := chatFileEntry{
		FileID:            "file-2",
		Status:            "canceled",
		TransferCompleted: true,
		BytesTransferred:  5000,
		CompletedAt:       2000,
	}
	live := chatFileEntry{
		FileID:            "file-2",
		Status:            "accepted",
		TransferCompleted: false,
		BytesTransferred:  256,
	}

	merged := mergeChatFileEntry(base, live)
	if merged.TransferCompleted {
		t.Fatalf("expected merged entry to be active")
	}
	if merged.Status != "accepted" {
		t.Fatalf("expected merged status accepted, got %q", merged.Status)
	}
	if merged.BytesTransferred != 256 {
		t.Fatalf("expected merged bytes to follow live active state, got %d", merged.BytesTransferred)
	}
	if merged.CompletedAt != 0 {
		t.Fatalf("expected merged completed_at reset to 0, got %d", merged.CompletedAt)
	}
}

package network

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"gosend/storage"
)

func TestFileTransferAcceptedAndChecksumMatches(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "sample-5mb.bin", 5*1024*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 64 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 64 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "complete", 15*time.Second)
	receivedMeta := waitForFileStatus(t, b.store, fileID, "complete", 15*time.Second)

	if _, err := os.Stat(receivedMeta.StoredPath); err != nil {
		t.Fatalf("received file not found: %v", err)
	}
	if got, err := fileChecksumHex(receivedMeta.StoredPath); err != nil {
		t.Fatalf("checksum received file failed: %v", err)
	} else if want, err := fileChecksumHex(sourcePath); err != nil {
		t.Fatalf("checksum source file failed: %v", err)
	} else if got != want {
		t.Fatalf("checksum mismatch: got=%s want=%s", got, want)
	}
}

func TestFileTransferRejected(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "sample.bin", 512*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return false, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "rejected", 8*time.Second)
	waitForFileStatus(t, b.store, fileID, "rejected", 8*time.Second)
}

func TestFileTransferAutoRejectedByMaxReceiveSize(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "too-large.bin", 512*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var callbackMu sync.Mutex
	callbackCount := 0
	b := newTestManager(t, testManagerConfig{
		deviceID:           "peer-b",
		name:               "Peer B",
		filesDir:           bFiles,
		fileChunkSize:      32 * 1024,
		maxReceiveFileSize: 128 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			callbackMu.Lock()
			callbackCount++
			callbackMu.Unlock()
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "rejected", 8*time.Second)
	waitForFileStatus(t, b.store, fileID, "rejected", 8*time.Second)

	callbackMu.Lock()
	finalCount := callbackCount
	callbackMu.Unlock()
	if finalCount != 0 {
		t.Fatalf("expected no manual file approval callback for oversized file, got %d", finalCount)
	}
}

func TestFileTransferAutoAcceptAndPeerDownloadOverride(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	bPeerDownloads := filepath.Join(t.TempDir(), "peer-downloads")
	sourcePath := createFixtureFile(t, t.TempDir(), "auto-accept.bin", 300*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var callbackMu sync.Mutex
	callbackCount := 0
	b := newTestManager(t, testManagerConfig{
		deviceID:           "peer-b",
		name:               "Peer B",
		filesDir:           bFiles,
		fileChunkSize:      32 * 1024,
		maxReceiveFileSize: 100 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			callbackMu.Lock()
			callbackCount++
			callbackMu.Unlock()
			return false, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	if err := b.store.UpdatePeerSettings(storage.PeerSettings{
		PeerDeviceID:      "peer-a",
		AutoAcceptFiles:   true,
		MaxFileSize:       1024 * 1024,
		DownloadDirectory: bPeerDownloads,
		CustomName:        "",
		TrustLevel:        storage.PeerTrustLevelNormal,
	}); err != nil {
		t.Fatalf("UpdatePeerSettings failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "complete", 12*time.Second)
	received := waitForFileStatus(t, b.store, fileID, "complete", 12*time.Second)
	if !strings.HasPrefix(received.StoredPath, bPeerDownloads) {
		t.Fatalf("expected received path under peer override directory %q, got %q", bPeerDownloads, received.StoredPath)
	}

	callbackMu.Lock()
	finalCount := callbackCount
	callbackMu.Unlock()
	if finalCount != 0 {
		t.Fatalf("expected no manual file approval callback for auto-accepted peer, got %d", finalCount)
	}
}

func TestFileTransferResumeAfterReconnect(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "resume-5mb.bin", 5*1024*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 64 * 1024,
	})
	defer a.stop()

	var dropOnce sync.Once
	var b *testManager
	b = newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 64 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
		onFileProgress: func(progress FileProgress) {
			if progress.Direction == fileTransferDirectionReceive && progress.ChunkIndex >= 8 {
				dropOnce.Do(func() {
					go func() {
						conn := b.manager.getConnection("peer-a")
						if conn != nil {
							_ = conn.Close()
						}
					}()
				})
			}
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
	received := waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)
	if got, err := fileChecksumHex(received.StoredPath); err != nil {
		t.Fatalf("checksum received file failed: %v", err)
	} else if want, err := fileChecksumHex(sourcePath); err != nil {
		t.Fatalf("checksum source file failed: %v", err)
	} else if got != want {
		t.Fatalf("checksum mismatch after resume: got=%s want=%s", got, want)
	}
}

func TestSendFileQueueRunsSequentiallyPerPeer(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	firstPath := createFixtureFile(t, t.TempDir(), "queued-first.bin", 8*1024*1024)
	secondPath := createFixtureFile(t, t.TempDir(), "queued-second.bin", 256*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 64 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 64 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	firstID, err := a.manager.SendFile("peer-b", firstPath)
	if err != nil {
		t.Fatalf("SendFile(first) failed: %v", err)
	}
	secondID, err := a.manager.SendFile("peer-b", secondPath)
	if err != nil {
		t.Fatalf("SendFile(second) failed: %v", err)
	}

	waitForFileStatusOneOf(t, a.store, firstID, []string{"accepted", "complete"}, 8*time.Second)

	secondMeta, err := a.store.GetFileByID(secondID)
	if err != nil {
		t.Fatalf("GetFileByID(second) failed: %v", err)
	}
	if secondMeta.TransferStatus != "pending" {
		t.Fatalf("expected second transfer to remain queued (pending) while first is active, got %q", secondMeta.TransferStatus)
	}

	waitForFileStatus(t, a.store, firstID, "complete", 20*time.Second)
	waitForFileStatus(t, a.store, secondID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, firstID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, secondID, "complete", 20*time.Second)
}

func TestCancelQueuedTransferBeforeStart(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	firstPath := createFixtureFile(t, t.TempDir(), "cancel-first.bin", 10*1024*1024)
	secondPath := createFixtureFile(t, t.TempDir(), "cancel-second.bin", 512*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 64 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 64 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	firstID, err := a.manager.SendFile("peer-b", firstPath)
	if err != nil {
		t.Fatalf("SendFile(first) failed: %v", err)
	}
	secondID, err := a.manager.SendFile("peer-b", secondPath)
	if err != nil {
		t.Fatalf("SendFile(second) failed: %v", err)
	}

	waitForFileStatusOneOf(t, a.store, firstID, []string{"accepted", "complete"}, 8*time.Second)
	waitForFileStatus(t, a.store, secondID, "pending", 5*time.Second)

	if err := a.manager.CancelTransfer(secondID); err != nil {
		t.Fatalf("CancelTransfer(second) failed: %v", err)
	}

	waitForFileStatus(t, a.store, secondID, "canceled", 8*time.Second)
	waitForFileStatus(t, a.store, firstID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, firstID, "complete", 20*time.Second)

	time.Sleep(500 * time.Millisecond)
	if _, err := b.store.GetFileByID(secondID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected canceled queued transfer to never reach receiver, got err=%v", err)
	}
}

func TestFileTransferDelayedAcceptPromptsOnce(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "delayed-accept.bin", 768*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var approveCalls atomic.Int32
	releaseDecision := make(chan struct{})
	var releaseOnce sync.Once

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			if approveCalls.Add(1) == 1 {
				<-releaseDecision
			}
			return true, nil
		},
	})
	defer b.stop()
	defer releaseOnce.Do(func() { close(releaseDecision) })

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	time.Sleep(5 * time.Second)
	releaseOnce.Do(func() { close(releaseDecision) })

	waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)

	if got := approveCalls.Load(); got != 1 {
		t.Fatalf("expected one approval callback after delayed accept, got %d", got)
	}
}

func TestFileTransferDelayedRejectPromptsOnce(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "delayed-reject.bin", 256*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var rejectCalls atomic.Int32
	releaseDecision := make(chan struct{})
	var releaseOnce sync.Once

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			if rejectCalls.Add(1) == 1 {
				<-releaseDecision
			}
			return false, nil
		},
	})
	defer b.stop()
	defer releaseOnce.Do(func() { close(releaseDecision) })

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	time.Sleep(5 * time.Second)
	releaseOnce.Do(func() { close(releaseDecision) })

	waitForFileStatus(t, a.store, fileID, "rejected", 12*time.Second)
	waitForFileStatus(t, b.store, fileID, "rejected", 12*time.Second)

	if got := rejectCalls.Load(); got != 1 {
		t.Fatalf("expected one approval callback after delayed reject, got %d", got)
	}
}

func TestFileTransferDelayedAcceptWithReplacementConnectionDoesNotReject(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "delayed-accept-reconnect.bin", 1024*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var approveCalls atomic.Int32
	releaseDecision := make(chan struct{})
	var releaseOnce sync.Once

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			call := approveCalls.Add(1)
			if call == 1 {
				<-releaseDecision
				return true, nil
			}
			// Simulate UI behavior where duplicate pending prompts should not cause a reject.
			return false, nil
		},
	})
	defer b.stop()
	defer releaseOnce.Do(func() { close(releaseDecision) })

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("replacement connect failed: %v", err)
	}

	time.Sleep(4 * time.Second)
	releaseOnce.Do(func() { close(releaseDecision) })

	waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)

	if got := approveCalls.Load(); got != 1 {
		t.Fatalf("expected one approval callback across delayed accept + replacement connection, got %d", got)
	}
}

func TestFolderTransferPreservesStructure(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourceRoot := filepath.Join(t.TempDir(), "source-folder")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir sub failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "empty"), 0o700); err != nil {
		t.Fatalf("mkdir empty failed: %v", err)
	}
	sourceA := filepath.Join(sourceRoot, "a.txt")
	sourceB := filepath.Join(sourceRoot, "sub", "b.txt")
	if err := os.WriteFile(sourceA, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write sourceA failed: %v", err)
	}
	if err := os.WriteFile(sourceB, []byte("beta"), 0o600); err != nil {
		t.Fatalf("write sourceB failed: %v", err)
	}

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	folderID, fileIDs, err := a.manager.SendFolder("peer-b", sourceRoot)
	if err != nil {
		t.Fatalf("SendFolder failed: %v", err)
	}
	if folderID == "" {
		t.Fatalf("expected non-empty folder ID")
	}
	if len(fileIDs) != 2 {
		t.Fatalf("expected 2 file IDs queued from folder, got %d", len(fileIDs))
	}

	for _, fileID := range fileIDs {
		waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
		waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)
	}

	folderMeta, err := b.store.GetFolderTransfer(folderID)
	if err != nil {
		t.Fatalf("GetFolderTransfer(receiver) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folderMeta.RootPath, "a.txt")); err != nil {
		t.Fatalf("missing received top-level file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folderMeta.RootPath, "sub", "b.txt")); err != nil {
		t.Fatalf("missing received nested file: %v", err)
	}
	if emptyInfo, err := os.Stat(filepath.Join(folderMeta.RootPath, "empty")); err != nil {
		t.Fatalf("missing received empty directory: %v", err)
	} else if !emptyInfo.IsDir() {
		t.Fatalf("expected empty path to be directory")
	}

	files, err := b.store.ListFilesByFolderID(folderID)
	if err != nil {
		t.Fatalf("ListFilesByFolderID failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 receiver file rows linked to folder, got %d", len(files))
	}
	for _, meta := range files {
		if meta.FolderID != folderID {
			t.Fatalf("expected folder_id %q, got %q", folderID, meta.FolderID)
		}
		if strings.TrimSpace(meta.RelativePath) == "" {
			t.Fatalf("expected non-empty relative_path for folder file %q", meta.FileID)
		}
	}
}

func TestFolderTransferPromptShownOnlyForFolderEnvelope(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourceRoot := filepath.Join(t.TempDir(), "source-folder-prompt-once")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "nested"), 0o700); err != nil {
		t.Fatalf("mkdir nested failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested", "b.txt"), []byte("beta"), 0o600); err != nil {
		t.Fatalf("write b.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested", "c.txt"), []byte("gamma"), 0o600); err != nil {
		t.Fatalf("write c.txt failed: %v", err)
	}

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var folderPrompts atomic.Int32
	var filePrompts atomic.Int32
	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(notification FileRequestNotification) (bool, error) {
			if notification.IsFolder {
				folderPrompts.Add(1)
			} else {
				filePrompts.Add(1)
			}
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	_, fileIDs, err := a.manager.SendFolder("peer-b", sourceRoot)
	if err != nil {
		t.Fatalf("SendFolder failed: %v", err)
	}
	if len(fileIDs) != 3 {
		t.Fatalf("expected 3 file IDs queued from folder, got %d", len(fileIDs))
	}

	for _, fileID := range fileIDs {
		waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
		waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)
	}

	if got := folderPrompts.Load(); got != 1 {
		t.Fatalf("expected one folder approval prompt, got %d", got)
	}
	if got := filePrompts.Load(); got != 0 {
		t.Fatalf("expected no per-file approval prompts for folder transfer, got %d", got)
	}
}

func TestRetryTransferRequeuesFailedOutbound(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "retry.bin", 600*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	var allow atomic.Bool
	allow.Store(false)
	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return allow.Load(), nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
	waitForFileStatus(t, a.store, fileID, "rejected", 10*time.Second)

	allow.Store(true)
	if err := a.manager.RetryTransfer(fileID); err != nil {
		t.Fatalf("RetryTransfer failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "complete", 20*time.Second)
	waitForFileStatus(t, b.store, fileID, "complete", 20*time.Second)
}

func TestCancelActiveTransferPropagatesCanceledStatus(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "cancel-active.bin", 18*1024*1024)

	var fileIDMu sync.RWMutex
	fileID := ""

	var a *testManager
	cancelOnce := sync.Once{}
	a = newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 64 * 1024,
		onFileProgress: func(progress FileProgress) {
			if progress.Direction != fileTransferDirectionSend || progress.TransferCompleted || progress.BytesTransferred <= 0 {
				return
			}
			fileIDMu.RLock()
			targetFileID := fileID
			fileIDMu.RUnlock()
			if targetFileID == "" || progress.FileID != targetFileID {
				return
			}
			cancelOnce.Do(func() {
				go func() {
					_ = a.manager.CancelTransfer(targetFileID)
				}()
			})
		},
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 64 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	sentFileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
	fileIDMu.Lock()
	fileID = sentFileID
	fileIDMu.Unlock()

	waitForFileStatus(t, a.store, sentFileID, "canceled", 20*time.Second)
	waitForFileStatus(t, b.store, sentFileID, "canceled", 20*time.Second)
}

func TestFolderTransferRequestRejectsPathTraversal(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	folderID := uuid.NewString()
	responseCh := a.manager.registerOutboundFolderEventChannel(folderID)
	defer a.manager.unregisterOutboundFolderEventChannel(folderID, responseCh)

	request := FolderTransferRequest{
		Type:         TypeFolderTransferRequest,
		FolderID:     folderID,
		FolderName:   "evil",
		TotalFiles:   1,
		TotalSize:    1,
		Manifest:     []FolderManifestEntry{{RelativePath: "../escape.txt", Size: 1}},
		FromDeviceID: "peer-a",
		ToDeviceID:   "peer-b",
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := a.manager.signFolderTransferRequest(&request); err != nil {
		t.Fatalf("signFolderTransferRequest failed: %v", err)
	}

	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}
	if err := conn.SendMessage(request); err != nil {
		t.Fatalf("send folder request failed: %v", err)
	}

	select {
	case response := <-responseCh:
		if !strings.EqualFold(response.Status, folderResponseStatusRejected) {
			t.Fatalf("expected rejected folder response, got %q", response.Status)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for folder rejection response")
	}
}

func TestFileTransferChecksumMismatchFails(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "bad-checksum.bin", 768*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:      "peer-a",
		name:          "Peer A",
		filesDir:      aFiles,
		fileChunkSize: 32 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:      "peer-b",
		name:          "Peer B",
		filesDir:      bFiles,
		fileChunkSize: 32 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.sendFileWithChecksumOverride("peer-b", sourcePath, strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("sendFileWithChecksumOverride failed: %v", err)
	}

	waitForFileStatus(t, a.store, fileID, "failed", 12*time.Second)
	waitForFileStatus(t, b.store, fileID, "failed", 12*time.Second)
}

func TestFileTransferResumeAfterSenderRestartUsesCheckpoint(t *testing.T) {
	bPort := freeTCPPort(t)
	sourcePath := createFixtureFile(t, t.TempDir(), "resume-after-crash.bin", 16*1024*1024)

	aIdentity := generateIdentity(t, "peer-a", "Peer A")
	aStore, _, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open A store failed: %v", err)
	}
	defer func() {
		_ = aStore.Close()
	}()
	a := newTestManagerWithParts(t, aStore, aIdentity, "127.0.0.1:0")
	defer a.stop()

	bIdentity := generateIdentity(t, "peer-b", "Peer B")
	bStore, _, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open B store failed: %v", err)
	}
	defer func() {
		_ = bStore.Close()
	}()
	b := newTestManagerWithParts(t, bStore, bIdentity, "127.0.0.1:"+bPort)
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	waitForCheckpointProgress(t, aStore, fileID, storage.TransferDirectionSend, 1, 20*time.Second)

	a.stop()

	aRestarted := newTestManagerWithParts(t, aStore, aIdentity, "127.0.0.1:0")
	defer aRestarted.stop()

	waitForFileStatus(t, aStore, fileID, "complete", 25*time.Second)
	waitForFileStatus(t, bStore, fileID, "complete", 25*time.Second)

	if _, err := aStore.GetTransferCheckpoint(fileID, storage.TransferDirectionSend); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected outbound checkpoint cleanup after completion, got err=%v", err)
	}
	if _, err := bStore.GetTransferCheckpoint(fileID, storage.TransferDirectionReceive); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected inbound checkpoint cleanup after completion, got err=%v", err)
	}
}

func TestChunkResponseMissingOrForgedSignatureRejected(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID := uuid.NewString()
	eventCh := a.manager.registerOutboundFileEventChannel(fileID)
	defer a.manager.unregisterOutboundFileEventChannel(fileID, eventCh)

	conn := b.manager.getConnection("peer-a")
	if conn == nil {
		t.Fatalf("expected connection from B to A")
	}

	unsigned := FileResponse{
		Type:         TypeFileResponse,
		FileID:       fileID,
		Status:       fileResponseStatusChunkAck,
		FromDeviceID: "peer-b",
		ChunkIndex:   0,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := conn.SendMessage(unsigned); err != nil {
		t.Fatalf("send unsigned chunk response failed: %v", err)
	}
	expectNoFileTransferEvent(t, eventCh, 250*time.Millisecond)

	forged := FileResponse{
		Type:         TypeFileResponse,
		FileID:       fileID,
		Status:       fileResponseStatusChunkAck,
		FromDeviceID: "peer-b",
		ChunkIndex:   0,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := b.manager.signFileResponse(&forged); err != nil {
		t.Fatalf("sign chunk response failed: %v", err)
	}
	forged.ChunkIndex = 1
	if err := conn.SendMessage(forged); err != nil {
		t.Fatalf("send forged chunk response failed: %v", err)
	}
	expectNoFileTransferEvent(t, eventCh, 250*time.Millisecond)
}

func TestFileCompleteMissingOrForgedSignatureRejected(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID := uuid.NewString()
	eventCh := a.manager.registerOutboundFileEventChannel(fileID)
	defer a.manager.unregisterOutboundFileEventChannel(fileID, eventCh)

	conn := b.manager.getConnection("peer-a")
	if conn == nil {
		t.Fatalf("expected connection from B to A")
	}

	unsigned := FileComplete{
		Type:      TypeFileComplete,
		FileID:    fileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := conn.SendMessage(unsigned); err != nil {
		t.Fatalf("send unsigned file complete failed: %v", err)
	}
	expectNoFileTransferEvent(t, eventCh, 250*time.Millisecond)

	forged := FileComplete{
		Type:      TypeFileComplete,
		FileID:    fileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := b.manager.signFileComplete(&forged); err != nil {
		t.Fatalf("sign file complete failed: %v", err)
	}
	forged.Status = fileCompleteStatusFailed
	if err := conn.SendMessage(forged); err != nil {
		t.Fatalf("send forged file complete failed: %v", err)
	}
	expectNoFileTransferEvent(t, eventCh, 250*time.Millisecond)
}

func waitForFileStatus(t *testing.T, store *storage.Store, fileID, expected string, timeout time.Duration) *storage.FileMetadata {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		meta, err := store.GetFileByID(fileID)
		if err == nil && meta.TransferStatus == expected {
			return meta
		}
		time.Sleep(30 * time.Millisecond)
	}

	meta, err := store.GetFileByID(fileID)
	if err != nil {
		t.Fatalf("timed out waiting for file %q status=%q, final err=%v", fileID, expected, err)
	}
	t.Fatalf("timed out waiting for file %q status=%q, final=%q", fileID, expected, meta.TransferStatus)
	return nil
}

func waitForFileStatusOneOf(t *testing.T, store *storage.Store, fileID string, expected []string, timeout time.Duration) *storage.FileMetadata {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		meta, err := store.GetFileByID(fileID)
		if err == nil {
			for _, status := range expected {
				if meta.TransferStatus == status {
					return meta
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	meta, err := store.GetFileByID(fileID)
	if err != nil {
		t.Fatalf("timed out waiting for file %q status in %v, final err=%v", fileID, expected, err)
	}
	t.Fatalf("timed out waiting for file %q status in %v, final=%q", fileID, expected, meta.TransferStatus)
	return nil
}

func expectNoFileTransferEvent(t *testing.T, events <-chan fileTransferEvent, timeout time.Duration) {
	t.Helper()
	select {
	case event := <-events:
		if event.Response != nil {
			t.Fatalf("unexpected file response event: %+v", *event.Response)
		}
		if event.Complete != nil {
			t.Fatalf("unexpected file complete event: %+v", *event.Complete)
		}
		t.Fatalf("unexpected file transfer event")
	case <-time.After(timeout):
	}
}

func waitForCheckpointProgress(t *testing.T, store *storage.Store, fileID, direction string, minNextChunk int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		checkpoint, err := store.GetTransferCheckpoint(fileID, direction)
		if err == nil && checkpoint.NextChunk >= minNextChunk {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	checkpoint, err := store.GetTransferCheckpoint(fileID, direction)
	if err != nil {
		t.Fatalf("timed out waiting for checkpoint %q/%q, final err=%v", fileID, direction, err)
	}
	t.Fatalf("timed out waiting for checkpoint %q/%q next_chunk >= %d, final=%d", fileID, direction, minNextChunk, checkpoint.NextChunk)
}

func createFixtureFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := make([]byte, size)
	for i := 0; i < size; i++ {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	return path
}

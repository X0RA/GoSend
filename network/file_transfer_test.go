package network

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

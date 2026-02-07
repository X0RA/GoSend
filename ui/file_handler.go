package ui

import (
	"errors"
	"sync"
)

// PickerFunc opens a platform-specific file picker and returns a selected path.
type PickerFunc func() (string, error)

// TransferProgress tracks one transfer's progress for UI binding.
type TransferProgress struct {
	FileID           string
	Filename         string
	BytesTransferred int64
	TotalBytes       int64
	Completed        bool
	Failed           bool
}

// FileHandler keeps file picker and progress tracking logic decoupled from network code.
type FileHandler struct {
	picker PickerFunc

	mu       sync.RWMutex
	progress map[string]TransferProgress
}

// NewFileHandler constructs a file handler with an injected picker function.
func NewFileHandler(picker PickerFunc) *FileHandler {
	return &FileHandler{
		picker:   picker,
		progress: make(map[string]TransferProgress),
	}
}

// PickFile opens the configured picker.
func (h *FileHandler) PickFile() (string, error) {
	if h == nil || h.picker == nil {
		return "", errors.New("file picker is not configured")
	}
	return h.picker()
}

// UpdateProgress stores progress for one file transfer.
func (h *FileHandler) UpdateProgress(progress TransferProgress) {
	if h == nil || progress.FileID == "" {
		return
	}
	h.mu.Lock()
	h.progress[progress.FileID] = progress
	h.mu.Unlock()
}

// Progress returns one transfer progress snapshot.
func (h *FileHandler) Progress(fileID string) (TransferProgress, bool) {
	if h == nil || fileID == "" {
		return TransferProgress{}, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	progress, ok := h.progress[fileID]
	return progress, ok
}

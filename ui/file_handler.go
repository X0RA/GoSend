package ui

import (
	"errors"
	"sync"
)

// PickerFunc opens a platform-specific picker and returns selected file paths.
type PickerFunc func() ([]string, error)

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

// PickPaths opens the configured picker.
func (h *FileHandler) PickPaths() ([]string, error) {
	if h == nil || h.picker == nil {
		return nil, errors.New("file picker is not configured")
	}
	return h.picker()
}

// PickFile returns the first selected path for call sites that only need one.
func (h *FileHandler) PickFile() (string, error) {
	paths, err := h.PickPaths()
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", errors.New("no file selected")
	}
	return paths[0], nil
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

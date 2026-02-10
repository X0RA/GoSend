package network

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	appcrypto "gosend/crypto"
	"gosend/storage"
)

const (
	fileTransferDirectionSend    = "send"
	fileTransferDirectionReceive = "receive"

	fileResponseStatusAccepted  = "accepted"
	fileResponseStatusRejected  = "rejected"
	fileResponseStatusChunkAck  = "chunk_ack"
	fileResponseStatusChunkNack = "chunk_nack"

	folderResponseStatusAccepted = "accepted"
	folderResponseStatusRejected = "rejected"

	fileCompleteStatusComplete = "complete"
	fileCompleteStatusFailed   = "failed"

	outboundCheckpointChunkInterval = 50
	outboundCheckpointByteInterval  = 10 * 1024 * 1024
	inboundCheckpointChunkInterval  = 10
	inboundCheckpointByteInterval   = 2 * 1024 * 1024
)

type outboundFileTransfer struct {
	mu sync.Mutex

	FileID       string
	MessageID    string
	PeerDeviceID string

	FolderID     string
	RelativePath string
	SourcePath   string
	Filename     string
	Filesize     int64
	Filetype     string
	Checksum     string
	ChunkSize    int
	TotalChunks  int

	NextChunk int
	BytesSent int64

	Running  bool
	Done     bool
	Rejected bool
	Canceled bool

	cancel            context.CancelFunc
	lastProgressAt    time.Time
	lastProgressBytes int64
	speedBytesPerSec  float64

	checkpointChunk int
	checkpointBytes int64
}

type inboundFileTransfer struct {
	mu sync.Mutex

	FileID       string
	FromDeviceID string
	Filename     string
	Filesize     int64
	Filetype     string
	Checksum     string
	ChunkSize    int
	TotalChunks  int

	TempPath  string
	FinalPath string

	ReceivedChunks map[int]bool
	NextChunk      int
	BytesReceived  int64

	lastProgressAt    time.Time
	lastProgressBytes int64
	speedBytesPerSec  float64

	checkpointChunk int
	checkpointBytes int64
}

type inboundFolderTransfer struct {
	FolderID      string
	FromDeviceID  string
	FolderName    string
	RootPath      string
	Manifest      map[string]FolderManifestEntry
	AcceptedAt    int64
	ExpectedFiles int
	TotalSize     int64
}

type fileTransferEvent struct {
	Response *FileResponse
	Complete *FileComplete
}

// SendFile starts an outbound file transfer. If the peer is offline, transfer remains pending until reconnect.
func (m *PeerManager) SendFile(peerDeviceID, sourcePath string) (string, error) {
	return m.sendFileWithOptions(peerDeviceID, sourcePath, outboundSendOptions{})
}

// SendFiles queues multiple files in-order for one peer.
func (m *PeerManager) SendFiles(peerDeviceID string, sourcePaths []string) ([]string, error) {
	if peerDeviceID == "" {
		return nil, errors.New("peer device ID is required")
	}
	queued := make([]string, 0, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		fileID, err := m.SendFile(peerDeviceID, sourcePath)
		if err != nil {
			return queued, err
		}
		queued = append(queued, fileID)
	}
	if len(queued) == 0 {
		return nil, errors.New("at least one source path is required")
	}
	return queued, nil
}

// SendFolder sends a folder manifest and queues all folder files after acceptance.
func (m *PeerManager) SendFolder(peerDeviceID, folderPath string) (string, []string, error) {
	if peerDeviceID == "" {
		return "", nil, errors.New("peer device ID is required")
	}
	if strings.TrimSpace(folderPath) == "" {
		return "", nil, errors.New("folder path is required")
	}

	folderInfo, err := os.Stat(folderPath)
	if err != nil {
		return "", nil, fmt.Errorf("stat folder: %w", err)
	}
	if !folderInfo.IsDir() {
		return "", nil, errors.New("folder path must be a directory")
	}

	manifest, files, totalSize, err := buildFolderManifest(folderPath)
	if err != nil {
		return "", nil, err
	}

	conn := m.getConnection(peerDeviceID)
	if conn == nil || conn.State() == StateDisconnected {
		return "", nil, fmt.Errorf("no active connection for peer %q", peerDeviceID)
	}

	folderID := uuid.NewString()
	folderName := sanitizeFolderName(filepath.Base(folderPath))
	if folderName == "" {
		folderName = "folder"
	}

	if err := m.options.Store.UpsertFolderTransfer(storage.FolderTransferMetadata{
		FolderID:       folderID,
		FromDeviceID:   m.options.Identity.DeviceID,
		ToDeviceID:     peerDeviceID,
		FolderName:     folderName,
		RootPath:       folderPath,
		TotalFiles:     len(files),
		TotalSize:      totalSize,
		TransferStatus: "pending",
		Timestamp:      time.Now().UnixMilli(),
	}); err != nil {
		return "", nil, err
	}

	request := FolderTransferRequest{
		Type:         TypeFolderTransferRequest,
		FolderID:     folderID,
		FolderName:   folderName,
		TotalFiles:   len(files),
		TotalSize:    totalSize,
		Manifest:     manifest,
		FromDeviceID: m.options.Identity.DeviceID,
		ToDeviceID:   peerDeviceID,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := m.signFolderTransferRequest(&request); err != nil {
		return "", nil, err
	}

	eventCh := m.registerOutboundFolderEventChannel(folderID)
	defer m.unregisterOutboundFolderEventChannel(folderID, eventCh)

	if err := conn.SendMessage(request); err != nil {
		return "", nil, err
	}

	response, err := m.waitForFolderResponse(m.ctx, eventCh, folderID)
	if err != nil {
		return "", nil, err
	}
	switch strings.ToLower(response.Status) {
	case folderResponseStatusRejected:
		_ = m.options.Store.UpdateFolderTransferStatus(folderID, "rejected")
		return folderID, nil, nil
	case folderResponseStatusAccepted:
		_ = m.options.Store.UpdateFolderTransferStatus(folderID, "accepted")
	default:
		return "", nil, fmt.Errorf("unexpected folder transfer status %q", response.Status)
	}

	queuedFileIDs := make([]string, 0, len(files))
	for _, fileEntry := range files {
		fileID, queueErr := m.sendFileWithOptions(peerDeviceID, fileEntry.AbsolutePath, outboundSendOptions{
			FolderID:     folderID,
			RelativePath: fileEntry.RelativePath,
		})
		if queueErr != nil {
			return folderID, queuedFileIDs, queueErr
		}
		queuedFileIDs = append(queuedFileIDs, fileID)
	}

	if len(queuedFileIDs) == 0 {
		_ = m.options.Store.UpdateFolderTransferStatus(folderID, "complete")
	}
	return folderID, queuedFileIDs, nil
}

func (m *PeerManager) sendFileWithChecksumOverride(peerDeviceID, sourcePath, checksumOverride string) (string, error) {
	return m.sendFileWithOptions(peerDeviceID, sourcePath, outboundSendOptions{
		ChecksumOverride: checksumOverride,
	})
}

type outboundSendOptions struct {
	FileID           string
	MessageID        string
	ChecksumOverride string
	FolderID         string
	RelativePath     string
}

type folderManifestFile struct {
	RelativePath string
	AbsolutePath string
	Size         int64
}

func (m *PeerManager) sendFileWithOptions(peerDeviceID, sourcePath string, options outboundSendOptions) (string, error) {
	if peerDeviceID == "" {
		return "", errors.New("peer device ID is required")
	}
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("source path is required")
	}

	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("stat source file: %w", err)
	}
	if fileInfo.IsDir() {
		return "", errors.New("source path must be a file")
	}

	checksum, err := fileChecksumHex(sourcePath)
	if err != nil {
		return "", err
	}
	if options.ChecksumOverride != "" {
		checksum = options.ChecksumOverride
	}

	relativePath := strings.TrimSpace(options.RelativePath)
	if relativePath != "" {
		relativePath, err = sanitizeTransferRelativePath(relativePath)
		if err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(options.FolderID) != "" && relativePath == "" {
		return "", errors.New("relative path is required for folder file transfer")
	}

	filename := filepath.Base(sourcePath)
	if relativePath != "" {
		filename = filepath.Base(relativePath)
	}
	filetype := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	totalChunks := chunkCount(fileInfo.Size(), m.options.FileChunkSize)
	fileID := options.FileID
	if fileID == "" {
		fileID = uuid.NewString()
	}
	messageID := options.MessageID
	if messageID == "" {
		messageID = uuid.NewString()
	}

	transfer := &outboundFileTransfer{
		FileID:       fileID,
		MessageID:    messageID,
		PeerDeviceID: peerDeviceID,
		FolderID:     strings.TrimSpace(options.FolderID),
		RelativePath: relativePath,
		SourcePath:   sourcePath,
		Filename:     filename,
		Filesize:     fileInfo.Size(),
		Filetype:     filetype,
		Checksum:     checksum,
		ChunkSize:    m.options.FileChunkSize,
		TotalChunks:  totalChunks,
	}

	meta := storage.FileMetadata{
		FileID:         transfer.FileID,
		FolderID:       transfer.FolderID,
		RelativePath:   transfer.RelativePath,
		FromDeviceID:   m.options.Identity.DeviceID,
		ToDeviceID:     peerDeviceID,
		Filename:       transfer.Filename,
		Filesize:       transfer.Filesize,
		Filetype:       transfer.Filetype,
		StoredPath:     transfer.SourcePath,
		Checksum:       transfer.Checksum,
		TransferStatus: "pending",
	}
	if err := m.upsertFileMetadata(meta); err != nil {
		return "", err
	}

	m.fileMu.Lock()
	m.outboundFileTransfers[transfer.FileID] = transfer
	m.fileMu.Unlock()
	m.enqueueOutboundTransfer(peerDeviceID, transfer.FileID, false)
	m.startNextQueuedTransfer(peerDeviceID)

	return transfer.FileID, nil
}

func (m *PeerManager) startOutboundFileTransferDrain(peerDeviceID string, conn *PeerConnection) {
	if peerDeviceID == "" || conn == nil {
		return
	}
	m.startNextQueuedTransfer(peerDeviceID)
}

func (m *PeerManager) beginOutboundFileTransfer(transfer *outboundFileTransfer, conn *PeerConnection) {
	if transfer == nil || conn == nil {
		return
	}

	transfer.mu.Lock()
	if transfer.Running || transfer.Done || transfer.Rejected || transfer.Canceled {
		transfer.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(m.ctx)
	transfer.cancel = cancel
	transfer.Running = true
	transfer.lastProgressAt = time.Now()
	transfer.lastProgressBytes = transfer.BytesSent
	transfer.speedBytesPerSec = 0
	transfer.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		err := m.runOutboundFileTransfer(runCtx, transfer, conn)

		transfer.mu.Lock()
		transfer.Running = false
		transfer.cancel = nil
		pending := !transfer.Done && !transfer.Rejected && !transfer.Canceled
		wasCanceled := transfer.Canceled
		transfer.mu.Unlock()

		if err != nil && !errors.Is(err, context.Canceled) {
			m.reportError(fmt.Errorf("file transfer %q: %w", transfer.FileID, err))
		}
		if pending {
			_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "pending")
			m.enqueueOutboundTransfer(transfer.PeerDeviceID, transfer.FileID, true)
		} else if wasCanceled {
			_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "failed")
			m.removeTransferCheckpoint(transfer.FileID, storage.TransferDirectionSend)
		}
		m.finishOutboundTransfer(transfer.PeerDeviceID, transfer.FileID)
		m.startNextQueuedTransfer(transfer.PeerDeviceID)
		if cancel != nil {
			cancel()
		}
	}()
}

func (m *PeerManager) enqueueOutboundTransfer(peerDeviceID, fileID string, front bool) {
	if strings.TrimSpace(peerDeviceID) == "" || strings.TrimSpace(fileID) == "" {
		return
	}

	m.transferQueueMu.Lock()
	queue := m.outboundTransferQueue[peerDeviceID]
	alreadyQueued := false
	for _, queuedID := range queue {
		if queuedID == fileID {
			alreadyQueued = true
			break
		}
	}
	if !alreadyQueued {
		if front {
			queue = append([]string{fileID}, queue...)
		} else {
			queue = append(queue, fileID)
		}
		m.outboundTransferQueue[peerDeviceID] = queue
	}
	m.transferQueueMu.Unlock()
}

func (m *PeerManager) removeQueuedOutboundTransfer(peerDeviceID, fileID string) bool {
	if strings.TrimSpace(peerDeviceID) == "" || strings.TrimSpace(fileID) == "" {
		return false
	}

	m.transferQueueMu.Lock()
	defer m.transferQueueMu.Unlock()

	queue := m.outboundTransferQueue[peerDeviceID]
	if len(queue) == 0 {
		return false
	}
	filtered := queue[:0]
	removed := false
	for _, queuedID := range queue {
		if queuedID == fileID {
			removed = true
			continue
		}
		filtered = append(filtered, queuedID)
	}
	if len(filtered) == 0 {
		delete(m.outboundTransferQueue, peerDeviceID)
	} else {
		m.outboundTransferQueue[peerDeviceID] = filtered
	}
	return removed
}

func (m *PeerManager) finishOutboundTransfer(peerDeviceID, fileID string) {
	if peerDeviceID == "" || fileID == "" {
		return
	}

	m.transferQueueMu.Lock()
	if current := m.activeOutboundTransfer[peerDeviceID]; current == fileID {
		delete(m.activeOutboundTransfer, peerDeviceID)
	}
	m.transferQueueMu.Unlock()
}

func (m *PeerManager) startNextQueuedTransfer(peerDeviceID string) {
	if strings.TrimSpace(peerDeviceID) == "" {
		return
	}
	conn := m.getConnection(peerDeviceID)
	if conn == nil || conn.State() == StateDisconnected {
		return
	}

	for {
		m.transferQueueMu.Lock()
		if activeID := m.activeOutboundTransfer[peerDeviceID]; activeID != "" {
			m.transferQueueMu.Unlock()
			return
		}

		queue := m.outboundTransferQueue[peerDeviceID]
		if len(queue) == 0 {
			delete(m.outboundTransferQueue, peerDeviceID)
			m.transferQueueMu.Unlock()
			return
		}

		nextID := queue[0]
		remaining := queue[1:]
		if len(remaining) == 0 {
			delete(m.outboundTransferQueue, peerDeviceID)
		} else {
			m.outboundTransferQueue[peerDeviceID] = remaining
		}
		m.activeOutboundTransfer[peerDeviceID] = nextID
		m.transferQueueMu.Unlock()

		m.fileMu.Lock()
		transfer := m.outboundFileTransfers[nextID]
		m.fileMu.Unlock()
		if transfer == nil {
			m.finishOutboundTransfer(peerDeviceID, nextID)
			continue
		}

		transfer.mu.Lock()
		skip := transfer.Done || transfer.Rejected || transfer.Canceled
		transfer.mu.Unlock()
		if skip {
			m.finishOutboundTransfer(peerDeviceID, nextID)
			continue
		}
		m.beginOutboundFileTransfer(transfer, conn)
		return
	}
}

// CancelTransfer cancels a queued or active file transfer.
func (m *PeerManager) CancelTransfer(fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return errors.New("file_id is required")
	}

	m.fileMu.Lock()
	outbound := m.outboundFileTransfers[fileID]
	inbound := m.inboundFileTransfers[fileID]
	m.fileMu.Unlock()

	if outbound != nil {
		outbound.mu.Lock()
		if outbound.Done || outbound.Rejected || outbound.Canceled {
			outbound.mu.Unlock()
			return nil
		}
		outbound.Canceled = true
		cancel := outbound.cancel
		running := outbound.Running
		peerID := outbound.PeerDeviceID
		bytesSent := outbound.BytesSent
		totalBytes := outbound.Filesize
		totalChunks := outbound.TotalChunks
		if !running {
			outbound.Done = true
		}
		outbound.mu.Unlock()

		m.removeQueuedOutboundTransfer(peerID, fileID)
		if running {
			if cancel != nil {
				cancel()
			}
			return nil
		}

		_ = m.options.Store.UpdateTransferStatus(fileID, "failed")
		m.removeTransferCheckpoint(fileID, storage.TransferDirectionSend)
		m.finishOutboundTransfer(peerID, fileID)
		m.startNextQueuedTransfer(peerID)
		m.emitFileProgress(FileProgress{
			FileID:            fileID,
			PeerDeviceID:      peerID,
			Direction:         fileTransferDirectionSend,
			BytesTransferred:  bytesSent,
			TotalBytes:        totalBytes,
			TotalChunks:       totalChunks,
			Status:            "failed",
			TransferCompleted: true,
		})
		return nil
	}

	if inbound != nil {
		peerID := inbound.FromDeviceID
		totalBytes := inbound.Filesize
		totalChunks := inbound.TotalChunks
		m.removeInboundTransfer(fileID)
		m.removeTransferCheckpoint(fileID, storage.TransferDirectionReceive)
		_ = os.Remove(inbound.TempPath)
		_ = m.options.Store.UpdateTransferStatus(fileID, "failed")
		if conn := m.getConnection(peerID); conn != nil {
			_ = m.sendSignedFileComplete(conn, FileComplete{
				Type:      TypeFileComplete,
				FileID:    fileID,
				Status:    fileCompleteStatusFailed,
				Message:   "transfer canceled by receiver",
				Timestamp: time.Now().UnixMilli(),
			})
		}
		m.emitFileProgress(FileProgress{
			FileID:            fileID,
			PeerDeviceID:      peerID,
			Direction:         fileTransferDirectionReceive,
			BytesTransferred:  0,
			TotalBytes:        totalBytes,
			TotalChunks:       totalChunks,
			Status:            "failed",
			TransferCompleted: true,
		})
		return nil
	}

	if _, err := m.options.Store.GetFileByID(fileID); err != nil {
		return err
	}
	return m.options.Store.UpdateTransferStatus(fileID, "failed")
}

// RetryTransfer re-queues a failed outbound transfer from its persisted metadata.
func (m *PeerManager) RetryTransfer(fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return errors.New("file_id is required")
	}

	meta, err := m.options.Store.GetFileByID(fileID)
	if err != nil {
		return err
	}
	if meta.FromDeviceID != m.options.Identity.DeviceID {
		return errors.New("only outbound transfers can be retried")
	}

	sourcePath := strings.TrimSpace(meta.StoredPath)
	if sourcePath == "" {
		return errors.New("missing source path for retry")
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat retry source file: %w", err)
	}
	if stat.IsDir() {
		return errors.New("retry source must be a file")
	}

	m.fileMu.Lock()
	transfer := m.outboundFileTransfers[fileID]
	if transfer == nil {
		totalChunks := chunkCount(meta.Filesize, m.options.FileChunkSize)
		transfer = &outboundFileTransfer{
			FileID:       meta.FileID,
			MessageID:    meta.MessageID,
			PeerDeviceID: meta.ToDeviceID,
			FolderID:     meta.FolderID,
			RelativePath: meta.RelativePath,
			SourcePath:   sourcePath,
			Filename:     meta.Filename,
			Filesize:     meta.Filesize,
			Filetype:     meta.Filetype,
			Checksum:     meta.Checksum,
			ChunkSize:    m.options.FileChunkSize,
			TotalChunks:  totalChunks,
		}
		m.outboundFileTransfers[fileID] = transfer
	}
	m.fileMu.Unlock()

	transfer.mu.Lock()
	if transfer.Running {
		transfer.mu.Unlock()
		return errors.New("transfer is currently running")
	}
	transfer.Done = false
	transfer.Rejected = false
	transfer.Canceled = false
	transfer.NextChunk = 0
	transfer.BytesSent = 0
	transfer.checkpointChunk = 0
	transfer.checkpointBytes = 0
	transfer.cancel = nil
	peerID := transfer.PeerDeviceID
	transfer.mu.Unlock()

	m.removeTransferCheckpoint(fileID, storage.TransferDirectionSend)
	_ = m.options.Store.UpdateTransferStatus(fileID, "pending")
	m.enqueueOutboundTransfer(peerID, fileID, false)
	m.startNextQueuedTransfer(peerID)
	return nil
}

func (m *PeerManager) runOutboundFileTransfer(runCtx context.Context, transfer *outboundFileTransfer, conn *PeerConnection) error {
	eventCh := m.registerOutboundFileEventChannel(transfer.FileID)
	defer m.unregisterOutboundFileEventChannel(transfer.FileID, eventCh)

	request := FileRequest{
		Type:         TypeFileRequest,
		FileID:       transfer.FileID,
		FolderID:     transfer.FolderID,
		RelativePath: transfer.RelativePath,
		FromDeviceID: m.options.Identity.DeviceID,
		ToDeviceID:   transfer.PeerDeviceID,
		Filename:     transfer.Filename,
		Filesize:     transfer.Filesize,
		Filetype:     transfer.Filetype,
		Checksum:     transfer.Checksum,
		Sequence:     conn.NextSendSequence(),
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := m.signFileRequest(&request); err != nil {
		return err
	}
	if err := conn.SendMessage(request); err != nil {
		return err
	}

	acceptResponse, err := m.waitForFileResponse(runCtx, eventCh, func(response FileResponse) bool {
		return response.Status == fileResponseStatusAccepted || response.Status == fileResponseStatusRejected
	})
	if err != nil {
		return err
	}
	if acceptResponse.Status == fileResponseStatusRejected {
		transfer.mu.Lock()
		transfer.Rejected = true
		transfer.mu.Unlock()
		_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "rejected")
		m.removeTransferCheckpoint(transfer.FileID, storage.TransferDirectionSend)
		m.emitFileProgress(FileProgress{
			FileID:            transfer.FileID,
			PeerDeviceID:      transfer.PeerDeviceID,
			Direction:         fileTransferDirectionSend,
			BytesTransferred:  0,
			TotalBytes:        transfer.Filesize,
			Status:            "rejected",
			TransferCompleted: true,
		})
		return nil
	}
	if err := m.options.Store.UpdateTransferStatus(transfer.FileID, "accepted"); err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.reportError(err)
	}

	startChunk := acceptResponse.ResumeFromChunk
	if startChunk < 0 {
		startChunk = 0
	}
	if startChunk > transfer.TotalChunks {
		startChunk = transfer.TotalChunks
	}

	transfer.mu.Lock()
	if transfer.NextChunk > startChunk {
		startChunk = transfer.NextChunk
	}
	transfer.NextChunk = startChunk
	bytesSent := transfer.BytesSent
	minBytes := int64(startChunk) * int64(transfer.ChunkSize)
	if minBytes > transfer.Filesize {
		minBytes = transfer.Filesize
	}
	if bytesSent < minBytes {
		bytesSent = minBytes
		transfer.BytesSent = bytesSent
	}
	transfer.mu.Unlock()
	m.maybePersistOutboundCheckpoint(transfer, true)

	file, err := os.Open(transfer.SourcePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	for chunkIndex := startChunk; chunkIndex < transfer.TotalChunks; chunkIndex++ {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		default:
		}

		chunkData, err := readFileChunk(file, int64(chunkIndex)*int64(transfer.ChunkSize), transfer.ChunkSize)
		if err != nil {
			return err
		}

		delivered := false
		for attempt := 0; attempt < m.options.MaxChunkRetries; attempt++ {
			ciphertext, iv, err := appcrypto.Encrypt(conn.SessionKey(), chunkData)
			if err != nil {
				return err
			}

			message := FileData{
				Type:          TypeFileData,
				FileID:        transfer.FileID,
				ChunkIndex:    chunkIndex,
				TotalChunks:   transfer.TotalChunks,
				ChunkSize:     len(chunkData),
				EncryptedData: base64.StdEncoding.EncodeToString(ciphertext),
				IV:            base64.StdEncoding.EncodeToString(iv),
				Timestamp:     time.Now().UnixMilli(),
			}
			if err := conn.SendMessage(message); err != nil {
				return err
			}

			response, err := m.waitForFileResponse(runCtx, eventCh, func(response FileResponse) bool {
				if response.Status == fileResponseStatusRejected {
					return true
				}
				return response.ChunkIndex == chunkIndex &&
					(response.Status == fileResponseStatusChunkAck || response.Status == fileResponseStatusChunkNack)
			})
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				if errors.Is(err, context.DeadlineExceeded) {
					continue
				}
				continue
			}

			if response.Status == fileResponseStatusRejected {
				transfer.mu.Lock()
				transfer.Rejected = true
				transfer.mu.Unlock()
				_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "rejected")
				m.removeTransferCheckpoint(transfer.FileID, storage.TransferDirectionSend)
				m.emitFileProgress(FileProgress{
					FileID:            transfer.FileID,
					PeerDeviceID:      transfer.PeerDeviceID,
					Direction:         fileTransferDirectionSend,
					BytesTransferred:  transfer.BytesSent,
					TotalBytes:        transfer.Filesize,
					Status:            "rejected",
					TransferCompleted: true,
				})
				return nil
			}
			if response.Status == fileResponseStatusChunkAck {
				delivered = true
				break
			}
		}

		if !delivered {
			return fmt.Errorf("chunk %d delivery failed after %d retries", chunkIndex, m.options.MaxChunkRetries)
		}

		bytesSent += int64(len(chunkData))
		transfer.mu.Lock()
		transfer.NextChunk = chunkIndex + 1
		transfer.BytesSent = bytesSent
		speed, eta := updateTransferSpeedLocked(transfer)
		transfer.mu.Unlock()
		m.maybePersistOutboundCheckpoint(transfer, false)

		m.emitFileProgress(FileProgress{
			FileID:           transfer.FileID,
			PeerDeviceID:     transfer.PeerDeviceID,
			Direction:        fileTransferDirectionSend,
			BytesTransferred: bytesSent,
			TotalBytes:       transfer.Filesize,
			ChunkIndex:       chunkIndex,
			TotalChunks:      transfer.TotalChunks,
			SpeedBytesPerSec: speed,
			ETASeconds:       eta,
			Status:           "accepted",
		})
	}

	completeMessage := FileComplete{
		Type:      TypeFileComplete,
		FileID:    transfer.FileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := m.sendSignedFileComplete(conn, completeMessage); err != nil {
		return err
	}

	complete, err := m.waitForFileComplete(runCtx, eventCh, func(complete FileComplete) bool {
		return complete.Status == fileCompleteStatusComplete || complete.Status == fileCompleteStatusFailed
	})
	if err != nil {
		return err
	}
	if complete.Status != fileCompleteStatusComplete {
		transfer.mu.Lock()
		transfer.Done = true
		transfer.mu.Unlock()
		_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "failed")
		m.removeTransferCheckpoint(transfer.FileID, storage.TransferDirectionSend)
		return nil
	}

	transfer.mu.Lock()
	transfer.Done = true
	transfer.Running = false
	transfer.mu.Unlock()
	_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "complete")
	if err := m.options.Store.UpdateFileTimestampReceived(transfer.FileID, time.Now().UnixMilli()); err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.reportError(err)
	}
	m.removeTransferCheckpoint(transfer.FileID, storage.TransferDirectionSend)

	m.emitFileProgress(FileProgress{
		FileID:            transfer.FileID,
		PeerDeviceID:      transfer.PeerDeviceID,
		Direction:         fileTransferDirectionSend,
		BytesTransferred:  transfer.Filesize,
		TotalBytes:        transfer.Filesize,
		ChunkIndex:        transfer.TotalChunks - 1,
		TotalChunks:       transfer.TotalChunks,
		Status:            "complete",
		TransferCompleted: true,
	})

	return nil
}

func (m *PeerManager) handleFileRequest(conn *PeerConnection, request FileRequest) {
	if request.FileID == "" || request.FromDeviceID == "" || request.Filename == "" || request.Filesize < 0 {
		return
	}
	if request.FromDeviceID != conn.PeerDeviceID() || request.ToDeviceID != m.options.Identity.DeviceID {
		return
	}
	if !withinTimestampSkew(request.Timestamp) {
		m.reportError(fmt.Errorf("rejecting file_request %q: timestamp outside skew", request.FileID))
		return
	}
	if err := conn.ValidateSequence(request.Sequence); err != nil {
		m.reportError(fmt.Errorf("rejecting file_request %q: %w", request.FileID, err))
		if errors.Is(err, ErrSequenceReplay) {
			m.logSecurityEvent(securityEventTypeReplayRejected, request.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
				"message_type": TypeFileRequest,
				"file_id":      request.FileID,
				"reason":       err.Error(),
				"sequence":     request.Sequence,
			})
		}
		return
	}
	if err := m.verifyFileRequest(conn, request); err != nil {
		m.reportError(err)
		m.logSecurityEvent(securityEventTypeSignatureVerificationFailed, request.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
			"message_type": TypeFileRequest,
			"file_id":      request.FileID,
		})
		_ = m.sendErrorMessage(conn, "invalid_signature", "file request signature verification failed", request.FileID)
		return
	}
	if !m.allowFileRequest(request.FromDeviceID, time.Now()) {
		m.logSecurityEvent(securityEventTypeConnectionRateLimitTriggered, request.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
			"limit_type":       "file_request_per_peer",
			"limit_per_window": m.options.FileRequestRateLimit,
			"window_seconds":   m.options.FileRequestWindow.Seconds(),
		})
		response := FileResponse{
			Type:         TypeFileResponse,
			FileID:       request.FileID,
			Status:       fileResponseStatusRejected,
			FromDeviceID: m.options.Identity.DeviceID,
			Timestamp:    time.Now().UnixMilli(),
			Message:      "file request rate limit exceeded",
		}
		if err := m.signFileResponse(&response); err != nil {
			m.reportError(err)
			return
		}
		_ = conn.SendMessage(response)
		return
	}

	peerSettings := m.getPeerSettingsSnapshot(request.FromDeviceID)
	downloadDir := m.resolveInboundDownloadDirectory(peerSettings)
	finalPath := filepath.Join(downloadDir, prefixedFilename(request.FileID, request.Filename))
	relativePath := ""
	folderID := strings.TrimSpace(request.FolderID)

	rejectWithMessage := func(message string) {
		response := FileResponse{
			Type:         TypeFileResponse,
			FileID:       request.FileID,
			Status:       fileResponseStatusRejected,
			FromDeviceID: m.options.Identity.DeviceID,
			Timestamp:    time.Now().UnixMilli(),
			Message:      message,
		}
		if err := m.signFileResponse(&response); err != nil {
			m.reportError(err)
			return
		}
		_ = conn.SendMessage(response)
	}

	if folderID != "" {
		var err error
		relativePath, err = sanitizeTransferRelativePath(request.RelativePath)
		if err != nil {
			m.reportError(fmt.Errorf("rejecting file_request %q invalid relative path: %w", request.FileID, err))
			rejectWithMessage("invalid folder relative path")
			return
		}
		folder := m.getInboundFolderTransfer(folderID)
		if folder == nil || folder.FromDeviceID != request.FromDeviceID {
			rejectWithMessage("unknown folder transfer")
			return
		}
		finalPath, err = joinSanitizedTransferPath(folder.RootPath, relativePath)
		if err != nil {
			m.reportError(fmt.Errorf("rejecting file_request %q invalid folder target path: %w", request.FileID, err))
			rejectWithMessage("invalid folder path")
			return
		}
	}
	tempPath := finalPath + ".part"

	effectiveSizeLimit := m.resolveReceiveLimit(peerSettings)
	accept := true
	rejectMessage := "file transfer rejected by user"

	if effectiveSizeLimit > 0 && request.Filesize > effectiveSizeLimit {
		accept = false
		rejectMessage = fmt.Sprintf("file size exceeds configured limit (%d bytes)", effectiveSizeLimit)
	} else if peerSettings != nil && peerSettings.AutoAcceptFiles {
		accept = true
	} else if m.options.OnFileRequest != nil {
		decision, err := m.options.OnFileRequest(FileRequestNotification{
			FileID:       request.FileID,
			FromDeviceID: request.FromDeviceID,
			Filename:     request.Filename,
			Filesize:     request.Filesize,
			Filetype:     request.Filetype,
			Checksum:     request.Checksum,
		})
		if err != nil {
			m.reportError(err)
			return
		}
		accept = decision
	}

	inbound := m.getInboundTransfer(request.FileID)
	if !accept {
		if inbound != nil {
			m.removeInboundTransfer(request.FileID)
			m.removeTransferCheckpoint(request.FileID, storage.TransferDirectionReceive)
			_ = os.Remove(inbound.TempPath)
		}
		meta := storage.FileMetadata{
			FileID:         request.FileID,
			FolderID:       folderID,
			RelativePath:   relativePath,
			FromDeviceID:   request.FromDeviceID,
			ToDeviceID:     request.ToDeviceID,
			Filename:       request.Filename,
			Filesize:       request.Filesize,
			Filetype:       request.Filetype,
			StoredPath:     finalPath,
			Checksum:       request.Checksum,
			TransferStatus: "rejected",
		}
		if err := m.upsertFileMetadata(meta); err != nil {
			m.reportError(err)
		} else {
			_ = m.options.Store.UpdateTransferStatus(request.FileID, "rejected")
		}
		rejectWithMessage(rejectMessage)
		return
	}

	if inbound == nil {
		if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
			m.reportError(err)
			return
		}
		file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			m.reportError(err)
			return
		}
		_ = file.Close()
		if err := os.Truncate(tempPath, request.Filesize); err != nil {
			m.reportError(err)
			return
		}

		inbound = &inboundFileTransfer{
			FileID:         request.FileID,
			FromDeviceID:   request.FromDeviceID,
			Filename:       request.Filename,
			Filesize:       request.Filesize,
			Filetype:       request.Filetype,
			Checksum:       request.Checksum,
			ChunkSize:      m.options.FileChunkSize,
			TotalChunks:    chunkCount(request.Filesize, m.options.FileChunkSize),
			TempPath:       tempPath,
			FinalPath:      finalPath,
			ReceivedChunks: make(map[int]bool),
		}
		m.setInboundTransfer(inbound)

		meta := storage.FileMetadata{
			FileID:         request.FileID,
			FolderID:       folderID,
			RelativePath:   relativePath,
			FromDeviceID:   request.FromDeviceID,
			ToDeviceID:     request.ToDeviceID,
			Filename:       request.Filename,
			Filesize:       request.Filesize,
			Filetype:       request.Filetype,
			StoredPath:     finalPath,
			Checksum:       request.Checksum,
			TransferStatus: "pending",
		}
		if err := m.upsertFileMetadata(meta); err != nil {
			m.reportError(err)
		}
		m.maybePersistInboundCheckpoint(inbound, true)
	}

	_ = m.options.Store.UpdateTransferStatus(request.FileID, "accepted")
	inbound.mu.Lock()
	resumeFrom := inbound.NextChunk
	inbound.mu.Unlock()

	response := FileResponse{
		Type:            TypeFileResponse,
		FileID:          request.FileID,
		Status:          fileResponseStatusAccepted,
		FromDeviceID:    m.options.Identity.DeviceID,
		ResumeFromChunk: resumeFrom,
		Timestamp:       time.Now().UnixMilli(),
	}
	if err := m.signFileResponse(&response); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(response); err != nil {
		m.reportError(err)
	}
}

func (m *PeerManager) handleFileResponse(conn *PeerConnection, response FileResponse) {
	if response.FileID == "" {
		return
	}
	if response.FromDeviceID == "" {
		m.reportError(fmt.Errorf("rejecting file_response %q: missing from_device_id", response.FileID))
		return
	}
	if response.FromDeviceID != conn.PeerDeviceID() {
		m.reportError(fmt.Errorf("rejecting file_response %q: sender mismatch %q != %q", response.FileID, response.FromDeviceID, conn.PeerDeviceID()))
		return
	}
	if err := m.verifyFileResponse(conn, response); err != nil {
		m.reportError(err)
		m.logSecurityEvent(securityEventTypeSignatureVerificationFailed, response.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
			"message_type": TypeFileResponse,
			"file_id":      response.FileID,
		})
		_ = m.sendErrorMessage(conn, "invalid_signature", "file response signature verification failed", response.FileID)
		return
	}

	m.fileMu.Lock()
	ch := m.outboundFileEventChans[response.FileID]
	m.fileMu.Unlock()
	if ch == nil {
		return
	}

	select {
	case ch <- fileTransferEvent{Response: &response}:
	default:
	}
}

func (m *PeerManager) handleFolderTransferRequest(conn *PeerConnection, request FolderTransferRequest) {
	if request.FolderID == "" || request.FolderName == "" || request.FromDeviceID == "" || request.ToDeviceID == "" {
		return
	}
	if request.FromDeviceID != conn.PeerDeviceID() || request.ToDeviceID != m.options.Identity.DeviceID {
		return
	}
	if !withinTimestampSkew(request.Timestamp) {
		m.reportError(fmt.Errorf("rejecting folder_transfer_request %q: timestamp outside skew", request.FolderID))
		return
	}
	if err := m.verifyFolderTransferRequest(conn, request); err != nil {
		m.reportError(err)
		m.logSecurityEvent(securityEventTypeSignatureVerificationFailed, request.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
			"message_type": TypeFolderTransferRequest,
			"folder_id":    request.FolderID,
		})
		_ = m.sendErrorMessage(conn, "invalid_signature", "folder transfer request signature verification failed", request.FolderID)
		return
	}

	manifest := make([]FolderManifestEntry, 0, len(request.Manifest))
	manifestMap := make(map[string]FolderManifestEntry, len(request.Manifest))
	totalFiles := 0
	var totalSize int64
	for _, entry := range request.Manifest {
		normalizedPath, err := sanitizeTransferRelativePath(entry.RelativePath)
		if err != nil {
			m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "invalid folder manifest path")
			return
		}
		if entry.Size < 0 {
			m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "invalid folder manifest size")
			return
		}
		normalized := entry
		normalized.RelativePath = normalizedPath
		manifest = append(manifest, normalized)
		manifestMap[normalizedPath] = normalized
		if !normalized.IsDirectory {
			totalFiles++
			totalSize += normalized.Size
		}
	}

	if request.TotalFiles > 0 && request.TotalFiles != totalFiles {
		m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "folder manifest file count mismatch")
		return
	}
	if request.TotalSize > 0 && request.TotalSize != totalSize {
		m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "folder manifest size mismatch")
		return
	}

	peerSettings := m.getPeerSettingsSnapshot(request.FromDeviceID)
	effectiveSizeLimit := m.resolveReceiveLimit(peerSettings)
	accept := true
	rejectMessage := "folder transfer rejected by user"
	if effectiveSizeLimit > 0 && totalSize > effectiveSizeLimit {
		accept = false
		rejectMessage = fmt.Sprintf("folder size exceeds configured limit (%d bytes)", effectiveSizeLimit)
	} else if peerSettings != nil && peerSettings.AutoAcceptFiles {
		accept = true
	} else if m.options.OnFileRequest != nil {
		decision, err := m.options.OnFileRequest(FileRequestNotification{
			FileID:       request.FolderID,
			FromDeviceID: request.FromDeviceID,
			Filename:     request.FolderName + "/",
			Filesize:     totalSize,
			Filetype:     "inode/directory",
			Checksum:     "",
		})
		if err != nil {
			m.reportError(err)
			return
		}
		accept = decision
	}

	downloadDir := m.resolveInboundDownloadDirectory(peerSettings)
	rootName := sanitizeFolderName(request.FolderName)
	if rootName == "" {
		rootName = "folder"
	}
	rootPath := filepath.Join(downloadDir, rootName)
	if _, err := os.Stat(rootPath); err == nil {
		rootPath = filepath.Join(downloadDir, request.FolderID+"_"+rootName)
	}

	if !accept {
		_ = m.options.Store.UpsertFolderTransfer(storage.FolderTransferMetadata{
			FolderID:       request.FolderID,
			FromDeviceID:   request.FromDeviceID,
			ToDeviceID:     request.ToDeviceID,
			FolderName:     request.FolderName,
			RootPath:       rootPath,
			TotalFiles:     totalFiles,
			TotalSize:      totalSize,
			TransferStatus: "rejected",
			Timestamp:      time.Now().UnixMilli(),
		})
		m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, rejectMessage)
		return
	}

	if err := os.MkdirAll(rootPath, 0o700); err != nil {
		m.reportError(err)
		m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "create folder path failed")
		return
	}
	for _, entry := range manifest {
		targetPath, err := joinSanitizedTransferPath(rootPath, entry.RelativePath)
		if err != nil {
			m.reportError(err)
			m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "invalid folder entry path")
			return
		}
		if entry.IsDirectory {
			if err := os.MkdirAll(targetPath, 0o700); err != nil {
				m.reportError(err)
				m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "create subdirectory failed")
				return
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			m.reportError(err)
			m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusRejected, "create parent directory failed")
			return
		}
	}

	if err := m.options.Store.UpsertFolderTransfer(storage.FolderTransferMetadata{
		FolderID:       request.FolderID,
		FromDeviceID:   request.FromDeviceID,
		ToDeviceID:     request.ToDeviceID,
		FolderName:     request.FolderName,
		RootPath:       rootPath,
		TotalFiles:     totalFiles,
		TotalSize:      totalSize,
		TransferStatus: "accepted",
		Timestamp:      time.Now().UnixMilli(),
	}); err != nil {
		m.reportError(err)
	}

	m.setInboundFolderTransfer(&inboundFolderTransfer{
		FolderID:      request.FolderID,
		FromDeviceID:  request.FromDeviceID,
		FolderName:    request.FolderName,
		RootPath:      rootPath,
		Manifest:      manifestMap,
		AcceptedAt:    time.Now().UnixMilli(),
		ExpectedFiles: totalFiles,
		TotalSize:     totalSize,
	})

	m.sendFolderTransferResponse(conn, request.FolderID, folderResponseStatusAccepted, "")
}

func (m *PeerManager) handleFolderTransferResponse(conn *PeerConnection, response FolderTransferResponse) {
	if response.FolderID == "" || response.FromDeviceID == "" {
		return
	}
	if response.FromDeviceID != conn.PeerDeviceID() {
		m.reportError(fmt.Errorf("rejecting folder_transfer_response %q: sender mismatch %q != %q", response.FolderID, response.FromDeviceID, conn.PeerDeviceID()))
		return
	}
	if err := m.verifyFolderTransferResponse(conn, response); err != nil {
		m.reportError(err)
		m.logSecurityEvent(securityEventTypeSignatureVerificationFailed, response.FromDeviceID, storage.SecuritySeverityWarning, map[string]any{
			"message_type": TypeFolderTransferResponse,
			"folder_id":    response.FolderID,
		})
		_ = m.sendErrorMessage(conn, "invalid_signature", "folder transfer response signature verification failed", response.FolderID)
		return
	}

	m.fileMu.Lock()
	ch := m.outboundFolderEventChans[response.FolderID]
	m.fileMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- response:
	default:
	}
}

func (m *PeerManager) sendFolderTransferResponse(conn *PeerConnection, folderID, status, message string) {
	response := FolderTransferResponse{
		Type:         TypeFolderTransferResponse,
		FolderID:     folderID,
		Status:       status,
		FromDeviceID: m.options.Identity.DeviceID,
		Message:      message,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := m.signFolderTransferResponse(&response); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(response); err != nil {
		m.reportError(err)
	}
}

func (m *PeerManager) handleFileData(conn *PeerConnection, data FileData) {
	if data.FileID == "" {
		return
	}
	transfer := m.getInboundTransfer(data.FileID)
	if transfer == nil {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "unknown file transfer")
		return
	}
	if data.ChunkIndex < 0 || data.ChunkIndex >= transfer.TotalChunks {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "invalid chunk index")
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(data.EncryptedData)
	if err != nil {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "invalid encrypted data")
		return
	}
	iv, err := base64.StdEncoding.DecodeString(data.IV)
	if err != nil {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "invalid iv")
		return
	}
	plaintext, err := appcrypto.Decrypt(conn.SessionKey(), iv, ciphertext)
	if err != nil {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "decryption failed")
		return
	}
	if data.ChunkSize > 0 && len(plaintext) != data.ChunkSize {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "chunk size mismatch")
		return
	}

	transfer.mu.Lock()
	offset := int64(data.ChunkIndex) * int64(transfer.ChunkSize)
	chunkAlreadyReceived := transfer.ReceivedChunks[data.ChunkIndex]
	transfer.mu.Unlock()

	file, err := os.OpenFile(transfer.TempPath, os.O_WRONLY, 0o600)
	if err != nil {
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "open temp file failed")
		return
	}
	if _, err := file.WriteAt(plaintext, offset); err != nil {
		_ = file.Close()
		m.sendChunkNack(conn, data.FileID, data.ChunkIndex, "write chunk failed")
		return
	}
	_ = file.Close()

	transfer.mu.Lock()
	if !chunkAlreadyReceived {
		transfer.ReceivedChunks[data.ChunkIndex] = true
		transfer.BytesReceived += int64(len(plaintext))
		for transfer.ReceivedChunks[transfer.NextChunk] {
			transfer.NextChunk++
		}
	}
	bytesReceived := transfer.BytesReceived
	totalBytes := transfer.Filesize
	totalChunks := transfer.TotalChunks
	speed, eta := updateInboundTransferSpeedLocked(transfer)
	transfer.mu.Unlock()
	m.maybePersistInboundCheckpoint(transfer, false)

	ack := FileResponse{
		Type:         TypeFileResponse,
		FileID:       data.FileID,
		Status:       fileResponseStatusChunkAck,
		FromDeviceID: m.options.Identity.DeviceID,
		ChunkIndex:   data.ChunkIndex,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := m.signFileResponse(&ack); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(ack); err != nil {
		m.reportError(err)
	}

	m.emitFileProgress(FileProgress{
		FileID:           data.FileID,
		PeerDeviceID:     conn.PeerDeviceID(),
		Direction:        fileTransferDirectionReceive,
		BytesTransferred: bytesReceived,
		TotalBytes:       totalBytes,
		ChunkIndex:       data.ChunkIndex,
		TotalChunks:      totalChunks,
		SpeedBytesPerSec: speed,
		ETASeconds:       eta,
		Status:           "accepted",
	})
}

func (m *PeerManager) handleFileComplete(conn *PeerConnection, complete FileComplete) {
	if complete.FileID == "" {
		return
	}
	if err := m.verifyFileComplete(conn, complete); err != nil {
		m.reportError(err)
		m.logSecurityEvent(securityEventTypeSignatureVerificationFailed, conn.PeerDeviceID(), storage.SecuritySeverityWarning, map[string]any{
			"message_type": TypeFileComplete,
			"file_id":      complete.FileID,
		})
		_ = m.sendErrorMessage(conn, "invalid_signature", "file complete signature verification failed", complete.FileID)
		return
	}

	inbound := m.getInboundTransfer(complete.FileID)
	if inbound == nil {
		m.fileMu.Lock()
		ch := m.outboundFileEventChans[complete.FileID]
		outbound := m.outboundFileTransfers[complete.FileID]
		m.fileMu.Unlock()
		if ch != nil {
			select {
			case ch <- fileTransferEvent{Complete: &complete}:
			default:
			}
			return
		}
		// Sender timed out waiting; if receiver sends FileComplete late, still mark complete so sender UI updates
		if outbound != nil && complete.Status == fileCompleteStatusComplete {
			outbound.mu.Lock()
			allSent := outbound.BytesSent == outbound.Filesize
			if allSent && !outbound.Done {
				outbound.Done = true
				outbound.Running = false
				fileID := outbound.FileID
				peerID := outbound.PeerDeviceID
				filesize := outbound.Filesize
				totalChunks := outbound.TotalChunks
				outbound.mu.Unlock()
				_ = m.options.Store.UpdateTransferStatus(fileID, "complete")
				if err := m.options.Store.UpdateFileTimestampReceived(fileID, time.Now().UnixMilli()); err != nil && !errors.Is(err, storage.ErrNotFound) {
					m.reportError(err)
				}
				m.removeTransferCheckpoint(fileID, storage.TransferDirectionSend)
				m.emitFileProgress(FileProgress{
					FileID:            fileID,
					PeerDeviceID:      peerID,
					Direction:         fileTransferDirectionSend,
					BytesTransferred:  filesize,
					TotalBytes:        filesize,
					ChunkIndex:        totalChunks - 1,
					TotalChunks:       totalChunks,
					Status:            "complete",
					TransferCompleted: true,
				})
			} else {
				outbound.mu.Unlock()
			}
		}
		return
	}

	if complete.Status != fileCompleteStatusComplete {
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		m.removeInboundTransfer(complete.FileID)
		m.removeTransferCheckpoint(complete.FileID, storage.TransferDirectionReceive)
		if err := m.sendSignedFileComplete(conn, FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "sender marked transfer failed",
			Timestamp: time.Now().UnixMilli(),
		}); err != nil {
			m.reportError(err)
		}
		return
	}

	checksum, err := fileChecksumHex(inbound.TempPath)
	if err != nil {
		m.reportError(err)
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		m.removeInboundTransfer(complete.FileID)
		m.removeTransferCheckpoint(complete.FileID, storage.TransferDirectionReceive)
		if sendErr := m.sendSignedFileComplete(conn, FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "checksum verification failed",
			Timestamp: time.Now().UnixMilli(),
		}); sendErr != nil {
			m.reportError(sendErr)
		}
		return
	}
	if !strings.EqualFold(checksum, inbound.Checksum) {
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		m.removeInboundTransfer(complete.FileID)
		m.removeTransferCheckpoint(complete.FileID, storage.TransferDirectionReceive)
		if err := m.sendSignedFileComplete(conn, FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "checksum mismatch",
			Timestamp: time.Now().UnixMilli(),
		}); err != nil {
			m.reportError(err)
		}
		return
	}

	if err := os.Rename(inbound.TempPath, inbound.FinalPath); err != nil {
		m.reportError(err)
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		m.removeInboundTransfer(complete.FileID)
		m.removeTransferCheckpoint(complete.FileID, storage.TransferDirectionReceive)
		if sendErr := m.sendSignedFileComplete(conn, FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "finalize file failed",
			Timestamp: time.Now().UnixMilli(),
		}); sendErr != nil {
			m.reportError(sendErr)
		}
		return
	}

	_ = m.options.Store.UpdateTransferStatus(complete.FileID, "complete")
	if err := m.options.Store.UpdateFileTimestampReceived(complete.FileID, time.Now().UnixMilli()); err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.reportError(err)
	}
	m.removeInboundTransfer(complete.FileID)
	m.removeTransferCheckpoint(complete.FileID, storage.TransferDirectionReceive)

	if err := m.sendSignedFileComplete(conn, FileComplete{
		Type:      TypeFileComplete,
		FileID:    complete.FileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	}); err != nil {
		m.reportError(err)
	}

	m.emitFileProgress(FileProgress{
		FileID:            complete.FileID,
		PeerDeviceID:      conn.PeerDeviceID(),
		Direction:         fileTransferDirectionReceive,
		BytesTransferred:  inbound.Filesize,
		TotalBytes:        inbound.Filesize,
		ChunkIndex:        inbound.TotalChunks - 1,
		TotalChunks:       inbound.TotalChunks,
		Status:            "complete",
		TransferCompleted: true,
	})
}

func (m *PeerManager) sendChunkNack(conn *PeerConnection, fileID string, chunkIndex int, msg string) {
	response := FileResponse{
		Type:         TypeFileResponse,
		FileID:       fileID,
		Status:       fileResponseStatusChunkNack,
		FromDeviceID: m.options.Identity.DeviceID,
		ChunkIndex:   chunkIndex,
		Message:      msg,
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := m.signFileResponse(&response); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(response); err != nil {
		m.reportError(err)
	}
}

func (m *PeerManager) sendSignedFileComplete(conn *PeerConnection, complete FileComplete) error {
	if err := m.signFileComplete(&complete); err != nil {
		return err
	}
	return conn.SendMessage(complete)
}

func (m *PeerManager) registerOutboundFileEventChannel(fileID string) chan fileTransferEvent {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	if existing := m.outboundFileEventChans[fileID]; existing != nil {
		return existing
	}
	ch := make(chan fileTransferEvent, 256)
	m.outboundFileEventChans[fileID] = ch
	return ch
}

func (m *PeerManager) unregisterOutboundFileEventChannel(fileID string, ch chan fileTransferEvent) {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	current := m.outboundFileEventChans[fileID]
	if current == ch {
		delete(m.outboundFileEventChans, fileID)
	}
}

func (m *PeerManager) registerOutboundFolderEventChannel(folderID string) chan FolderTransferResponse {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	if existing := m.outboundFolderEventChans[folderID]; existing != nil {
		return existing
	}
	ch := make(chan FolderTransferResponse, 16)
	m.outboundFolderEventChans[folderID] = ch
	return ch
}

func (m *PeerManager) unregisterOutboundFolderEventChannel(folderID string, ch chan FolderTransferResponse) {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	current := m.outboundFolderEventChans[folderID]
	if current == ch {
		delete(m.outboundFolderEventChans, folderID)
	}
}

func (m *PeerManager) waitForFileResponse(ctx context.Context, events <-chan fileTransferEvent, match func(FileResponse) bool) (FileResponse, error) {
	timer := time.NewTimer(m.options.FileResponseTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return FileResponse{}, ctx.Err()
		case <-timer.C:
			return FileResponse{}, context.DeadlineExceeded
		case event := <-events:
			if event.Response != nil && match(*event.Response) {
				return *event.Response, nil
			}
		}
	}
}

func (m *PeerManager) waitForFolderResponse(ctx context.Context, events <-chan FolderTransferResponse, folderID string) (FolderTransferResponse, error) {
	timer := time.NewTimer(m.options.FileResponseTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return FolderTransferResponse{}, ctx.Err()
		case <-timer.C:
			return FolderTransferResponse{}, context.DeadlineExceeded
		case response := <-events:
			if response.FolderID == folderID {
				return response, nil
			}
		}
	}
}

func (m *PeerManager) waitForFileComplete(ctx context.Context, events <-chan fileTransferEvent, match func(FileComplete) bool) (FileComplete, error) {
	timeout := m.options.FileCompleteTimeout
	if timeout <= 0 {
		timeout = m.options.FileResponseTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return FileComplete{}, ctx.Err()
		case <-timer.C:
			return FileComplete{}, context.DeadlineExceeded
		case event := <-events:
			if event.Complete != nil && match(*event.Complete) {
				return *event.Complete, nil
			}
		}
	}
}

func (m *PeerManager) upsertFileMetadata(meta storage.FileMetadata) error {
	err := m.options.Store.SaveFileMetadata(meta)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: files.file_id") {
		return nil
	}
	return err
}

func (m *PeerManager) loadTransferCheckpoints() error {
	checkpoints, err := m.options.Store.ListTransferCheckpoints("")
	if err != nil {
		return fmt.Errorf("list transfer checkpoints: %w", err)
	}

	for _, checkpoint := range checkpoints {
		switch checkpoint.Direction {
		case storage.TransferDirectionSend:
			if err := m.restoreOutboundTransfer(checkpoint); err != nil {
				m.reportError(err)
			}
		case storage.TransferDirectionReceive:
			if err := m.restoreInboundTransfer(checkpoint); err != nil {
				m.reportError(err)
			}
		}
	}

	return nil
}

func (m *PeerManager) restoreOutboundTransfer(checkpoint storage.TransferCheckpoint) error {
	meta, err := m.options.Store.GetFileByID(checkpoint.FileID)
	if errors.Is(err, storage.ErrNotFound) {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionSend)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load outbound checkpoint metadata %q: %w", checkpoint.FileID, err)
	}
	if isTerminalTransferStatus(meta.TransferStatus) {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionSend)
		return nil
	}
	if strings.TrimSpace(meta.ToDeviceID) == "" {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionSend)
		return nil
	}

	sourcePath := strings.TrimSpace(meta.StoredPath)
	if sourcePath == "" {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionSend)
		return nil
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		_ = m.options.Store.UpdateTransferStatus(checkpoint.FileID, "failed")
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionSend)
		return fmt.Errorf("resume outbound transfer %q source missing: %w", checkpoint.FileID, statErr)
	}

	totalChunks := chunkCount(meta.Filesize, m.options.FileChunkSize)
	nextChunk := checkpoint.NextChunk
	if nextChunk < 0 {
		nextChunk = 0
	}
	if nextChunk > totalChunks {
		nextChunk = totalChunks
	}
	bytesSent := checkpoint.BytesTransferred
	if bytesSent < 0 {
		bytesSent = 0
	}
	if bytesSent > meta.Filesize {
		bytesSent = meta.Filesize
	}

	transfer := &outboundFileTransfer{
		FileID:          meta.FileID,
		MessageID:       meta.MessageID,
		PeerDeviceID:    meta.ToDeviceID,
		FolderID:        meta.FolderID,
		RelativePath:    meta.RelativePath,
		SourcePath:      sourcePath,
		Filename:        meta.Filename,
		Filesize:        meta.Filesize,
		Filetype:        meta.Filetype,
		Checksum:        meta.Checksum,
		ChunkSize:       m.options.FileChunkSize,
		TotalChunks:     totalChunks,
		NextChunk:       nextChunk,
		BytesSent:       bytesSent,
		checkpointChunk: nextChunk,
		checkpointBytes: bytesSent,
	}

	m.fileMu.Lock()
	if _, exists := m.outboundFileTransfers[transfer.FileID]; !exists {
		m.outboundFileTransfers[transfer.FileID] = transfer
	}
	m.fileMu.Unlock()
	m.enqueueOutboundTransfer(transfer.PeerDeviceID, transfer.FileID, false)
	return nil
}

func (m *PeerManager) restoreInboundTransfer(checkpoint storage.TransferCheckpoint) error {
	meta, err := m.options.Store.GetFileByID(checkpoint.FileID)
	if errors.Is(err, storage.ErrNotFound) {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionReceive)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load inbound checkpoint metadata %q: %w", checkpoint.FileID, err)
	}
	if isTerminalTransferStatus(meta.TransferStatus) {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionReceive)
		return nil
	}

	tempPath := strings.TrimSpace(checkpoint.TempPath)
	if tempPath == "" {
		tempPath = strings.TrimSpace(meta.StoredPath) + ".part"
	}
	finalPath := strings.TrimSpace(meta.StoredPath)
	if tempPath == "" || finalPath == "" {
		m.removeTransferCheckpoint(checkpoint.FileID, storage.TransferDirectionReceive)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(tempPath), 0o700); err != nil {
		return fmt.Errorf("restore inbound checkpoint %q create temp dir: %w", checkpoint.FileID, err)
	}
	file, openErr := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0o600)
	if openErr != nil {
		return fmt.Errorf("restore inbound checkpoint %q open temp file: %w", checkpoint.FileID, openErr)
	}
	if truncateErr := file.Truncate(meta.Filesize); truncateErr != nil {
		_ = file.Close()
		return fmt.Errorf("restore inbound checkpoint %q truncate temp file: %w", checkpoint.FileID, truncateErr)
	}
	_ = file.Close()

	totalChunks := chunkCount(meta.Filesize, m.options.FileChunkSize)
	nextChunk := checkpoint.NextChunk
	if nextChunk < 0 {
		nextChunk = 0
	}
	if nextChunk > totalChunks {
		nextChunk = totalChunks
	}
	bytesReceived := checkpoint.BytesTransferred
	if bytesReceived < 0 {
		bytesReceived = 0
	}
	if bytesReceived > meta.Filesize {
		bytesReceived = meta.Filesize
	}

	receivedChunks := make(map[int]bool, nextChunk)
	for chunkIndex := 0; chunkIndex < nextChunk; chunkIndex++ {
		receivedChunks[chunkIndex] = true
	}

	if meta.FolderID != "" && m.getInboundFolderTransfer(meta.FolderID) == nil {
		rootPath := filepath.Dir(finalPath)
		folderName := filepath.Base(rootPath)
		if folderMeta, folderErr := m.options.Store.GetFolderTransfer(meta.FolderID); folderErr == nil {
			if strings.TrimSpace(folderMeta.RootPath) != "" {
				rootPath = strings.TrimSpace(folderMeta.RootPath)
			}
			if strings.TrimSpace(folderMeta.FolderName) != "" {
				folderName = strings.TrimSpace(folderMeta.FolderName)
			}
		}
		m.setInboundFolderTransfer(&inboundFolderTransfer{
			FolderID:      meta.FolderID,
			FromDeviceID:  meta.FromDeviceID,
			FolderName:    folderName,
			RootPath:      rootPath,
			Manifest:      make(map[string]FolderManifestEntry),
			AcceptedAt:    time.Now().UnixMilli(),
			ExpectedFiles: 0,
			TotalSize:     0,
		})
	}

	transfer := &inboundFileTransfer{
		FileID:          meta.FileID,
		FromDeviceID:    meta.FromDeviceID,
		Filename:        meta.Filename,
		Filesize:        meta.Filesize,
		Filetype:        meta.Filetype,
		Checksum:        meta.Checksum,
		ChunkSize:       m.options.FileChunkSize,
		TotalChunks:     totalChunks,
		TempPath:        tempPath,
		FinalPath:       finalPath,
		ReceivedChunks:  receivedChunks,
		NextChunk:       nextChunk,
		BytesReceived:   bytesReceived,
		checkpointChunk: nextChunk,
		checkpointBytes: bytesReceived,
	}

	m.setInboundTransfer(transfer)
	return nil
}

func isTerminalTransferStatus(status string) bool {
	switch status {
	case "complete", "rejected", "failed":
		return true
	default:
		return false
	}
}

func (m *PeerManager) getPeerSettingsSnapshot(peerDeviceID string) *storage.PeerSettings {
	if strings.TrimSpace(peerDeviceID) == "" || m.options.Store == nil {
		return nil
	}

	settings, err := m.options.Store.GetPeerSettings(peerDeviceID)
	if err == nil {
		return settings
	}
	if !errors.Is(err, storage.ErrNotFound) {
		m.reportError(fmt.Errorf("load peer settings for %q: %w", peerDeviceID, err))
		return nil
	}

	if ensureErr := m.options.Store.EnsurePeerSettingsExist(peerDeviceID); ensureErr != nil {
		m.reportError(fmt.Errorf("ensure peer settings for %q: %w", peerDeviceID, ensureErr))
		return nil
	}
	settings, err = m.options.Store.GetPeerSettings(peerDeviceID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			m.reportError(fmt.Errorf("reload peer settings for %q: %w", peerDeviceID, err))
		}
		return nil
	}
	return settings
}

func (m *PeerManager) resolveInboundDownloadDirectory(settings *storage.PeerSettings) string {
	if settings != nil {
		if peerDir := strings.TrimSpace(settings.DownloadDirectory); peerDir != "" {
			return peerDir
		}
	}
	return m.currentDownloadDirectory()
}

func (m *PeerManager) resolveReceiveLimit(settings *storage.PeerSettings) int64 {
	if settings != nil && settings.MaxFileSize > 0 {
		return settings.MaxFileSize
	}
	return m.currentMaxReceiveFileSize()
}

func (m *PeerManager) verifyFileRequest(conn *PeerConnection, request FileRequest) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(request.Signature)
	if err != nil {
		return fmt.Errorf("decode file request signature: %w", err)
	}
	signable := request
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid file request signature")
	}
	return nil
}

func (m *PeerManager) signFileRequest(request *FileRequest) error {
	signable := *request
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	request.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) signFileResponse(response *FileResponse) error {
	signable := *response
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	response.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) signFileComplete(complete *FileComplete) error {
	signable := *complete
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	complete.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyFileResponse(conn *PeerConnection, response FileResponse) error {
	if response.Signature == "" {
		return errors.New("file response signature is required")
	}
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(response.Signature)
	if err != nil {
		return fmt.Errorf("decode file response signature: %w", err)
	}
	signable := response
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid file response signature")
	}
	return nil
}

func (m *PeerManager) verifyFileComplete(conn *PeerConnection, complete FileComplete) error {
	if complete.Signature == "" {
		return errors.New("file complete signature is required")
	}
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(complete.Signature)
	if err != nil {
		return fmt.Errorf("decode file complete signature: %w", err)
	}
	signable := complete
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid file complete signature")
	}
	return nil
}

func (m *PeerManager) signFolderTransferRequest(request *FolderTransferRequest) error {
	signable := *request
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	request.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) signFolderTransferResponse(response *FolderTransferResponse) error {
	signable := *response
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	response.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyFolderTransferRequest(conn *PeerConnection, request FolderTransferRequest) error {
	if request.Signature == "" {
		return errors.New("folder transfer request signature is required")
	}
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(request.Signature)
	if err != nil {
		return fmt.Errorf("decode folder transfer request signature: %w", err)
	}
	signable := request
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid folder transfer request signature")
	}
	return nil
}

func (m *PeerManager) verifyFolderTransferResponse(conn *PeerConnection, response FolderTransferResponse) error {
	if response.Signature == "" {
		return errors.New("folder transfer response signature is required")
	}
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(response.Signature)
	if err != nil {
		return fmt.Errorf("decode folder transfer response signature: %w", err)
	}
	signable := response
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid folder transfer response signature")
	}
	return nil
}

func (m *PeerManager) getInboundTransfer(fileID string) *inboundFileTransfer {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	return m.inboundFileTransfers[fileID]
}

func (m *PeerManager) setInboundTransfer(transfer *inboundFileTransfer) {
	m.fileMu.Lock()
	m.inboundFileTransfers[transfer.FileID] = transfer
	m.fileMu.Unlock()
}

func (m *PeerManager) removeInboundTransfer(fileID string) {
	m.fileMu.Lock()
	delete(m.inboundFileTransfers, fileID)
	m.fileMu.Unlock()
}

func (m *PeerManager) getInboundFolderTransfer(folderID string) *inboundFolderTransfer {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	return m.inboundFolderTransfers[folderID]
}

func (m *PeerManager) setInboundFolderTransfer(folder *inboundFolderTransfer) {
	if folder == nil || folder.FolderID == "" {
		return
	}
	m.fileMu.Lock()
	m.inboundFolderTransfers[folder.FolderID] = folder
	m.fileMu.Unlock()
}

func (m *PeerManager) removeInboundFolderTransfer(folderID string) {
	if strings.TrimSpace(folderID) == "" {
		return
	}
	m.fileMu.Lock()
	delete(m.inboundFolderTransfers, folderID)
	m.fileMu.Unlock()
}

func (m *PeerManager) maybePersistOutboundCheckpoint(transfer *outboundFileTransfer, force bool) {
	if transfer == nil {
		return
	}

	transfer.mu.Lock()
	if !force {
		chunkDelta := transfer.NextChunk - transfer.checkpointChunk
		byteDelta := transfer.BytesSent - transfer.checkpointBytes
		if chunkDelta < outboundCheckpointChunkInterval && byteDelta < outboundCheckpointByteInterval {
			transfer.mu.Unlock()
			return
		}
	}

	checkpoint := storage.TransferCheckpoint{
		FileID:           transfer.FileID,
		Direction:        storage.TransferDirectionSend,
		NextChunk:        transfer.NextChunk,
		BytesTransferred: transfer.BytesSent,
		TempPath:         transfer.SourcePath,
		UpdatedAt:        time.Now().UnixMilli(),
	}
	transfer.checkpointChunk = transfer.NextChunk
	transfer.checkpointBytes = transfer.BytesSent
	transfer.mu.Unlock()

	if err := m.options.Store.UpsertTransferCheckpoint(checkpoint); err != nil {
		m.reportError(fmt.Errorf("upsert outbound transfer checkpoint %q: %w", transfer.FileID, err))
	}
}

func (m *PeerManager) maybePersistInboundCheckpoint(transfer *inboundFileTransfer, force bool) {
	if transfer == nil {
		return
	}

	transfer.mu.Lock()
	if !force {
		chunkDelta := transfer.NextChunk - transfer.checkpointChunk
		byteDelta := transfer.BytesReceived - transfer.checkpointBytes
		if chunkDelta < inboundCheckpointChunkInterval && byteDelta < inboundCheckpointByteInterval {
			transfer.mu.Unlock()
			return
		}
	}

	checkpoint := storage.TransferCheckpoint{
		FileID:           transfer.FileID,
		Direction:        storage.TransferDirectionReceive,
		NextChunk:        transfer.NextChunk,
		BytesTransferred: transfer.BytesReceived,
		TempPath:         transfer.TempPath,
		UpdatedAt:        time.Now().UnixMilli(),
	}
	transfer.checkpointChunk = transfer.NextChunk
	transfer.checkpointBytes = transfer.BytesReceived
	transfer.mu.Unlock()

	if err := m.options.Store.UpsertTransferCheckpoint(checkpoint); err != nil {
		m.reportError(fmt.Errorf("upsert inbound transfer checkpoint %q: %w", transfer.FileID, err))
	}
}

func (m *PeerManager) removeTransferCheckpoint(fileID, direction string) {
	if strings.TrimSpace(fileID) == "" {
		return
	}
	if err := m.options.Store.DeleteTransferCheckpoint(fileID, direction); err != nil {
		m.reportError(fmt.Errorf("delete transfer checkpoint %q/%s: %w", fileID, direction, err))
	}
}

func (m *PeerManager) emitFileProgress(progress FileProgress) {
	if m.options.OnFileProgress != nil {
		m.options.OnFileProgress(progress)
	}
}

func readFileChunk(file *os.File, offset int64, chunkSize int) ([]byte, error) {
	buffer := make([]byte, chunkSize)
	n, err := file.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read file chunk at offset %d: %w", offset, err)
	}
	if n == 0 {
		return nil, io.EOF
	}
	return buffer[:n], nil
}

func fileChecksumHex(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for checksum: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func chunkCount(size int64, chunkSize int) int {
	if size <= 0 || chunkSize <= 0 {
		return 0
	}
	chunks := int(size / int64(chunkSize))
	if size%int64(chunkSize) != 0 {
		chunks++
	}
	return chunks
}

func prefixedFilename(fileID, filename string) string {
	base := filepath.Base(filename)
	if base == "" {
		base = "file.bin"
	}
	return fileID + "_" + base
}

func buildFolderManifest(rootPath string) ([]FolderManifestEntry, []folderManifestFile, int64, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return nil, nil, 0, errors.New("folder path is required")
	}

	manifest := make([]FolderManifestEntry, 0)
	files := make([]folderManifestFile, 0)
	var totalSize int64

	walkErr := filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == rootPath {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// Symlinks are skipped to avoid surprising cross-tree traversal.
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relative, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			return relErr
		}
		normalized, normErr := sanitizeTransferRelativePath(relative)
		if normErr != nil {
			return normErr
		}

		if entry.IsDir() {
			manifest = append(manifest, FolderManifestEntry{
				RelativePath: normalized,
				Size:         0,
				IsDirectory:  true,
			})
			return nil
		}

		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		size := info.Size()
		if size < 0 {
			return fmt.Errorf("invalid file size for %q", normalized)
		}

		manifest = append(manifest, FolderManifestEntry{
			RelativePath: normalized,
			Size:         size,
		})
		files = append(files, folderManifestFile{
			RelativePath: normalized,
			AbsolutePath: path,
			Size:         size,
		})
		totalSize += size
		return nil
	})
	if walkErr != nil {
		return nil, nil, 0, fmt.Errorf("enumerate folder: %w", walkErr)
	}

	sort.SliceStable(manifest, func(i, j int) bool {
		if manifest[i].IsDirectory != manifest[j].IsDirectory {
			return manifest[i].IsDirectory
		}
		return manifest[i].RelativePath < manifest[j].RelativePath
	})
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return manifest, files, totalSize, nil
}

func sanitizeTransferRelativePath(relativePath string) (string, error) {
	trimmed := strings.TrimSpace(relativePath)
	if trimmed == "" {
		return "", errors.New("relative path is required")
	}
	trimmed = filepath.ToSlash(filepath.Clean(trimmed))
	if trimmed == "." {
		return "", errors.New("relative path cannot be root")
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", errors.New("relative path must not be absolute")
	}
	if trimmed == ".." || strings.HasPrefix(trimmed, "../") || strings.Contains(trimmed, "/../") {
		return "", errors.New("relative path must not traverse parent directories")
	}
	return trimmed, nil
}

func joinSanitizedTransferPath(rootPath, relativePath string) (string, error) {
	normalized, err := sanitizeTransferRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	rootClean := filepath.Clean(rootPath)
	if strings.TrimSpace(rootClean) == "" {
		return "", errors.New("root path is required")
	}
	target := filepath.Clean(filepath.Join(rootClean, filepath.FromSlash(normalized)))
	rootWithSep := rootClean + string(os.PathSeparator)
	if target != rootClean && !strings.HasPrefix(target, rootWithSep) {
		return "", errors.New("resolved path escapes root")
	}
	return target, nil
}

func sanitizeFolderName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	if name == "." || name == string(os.PathSeparator) {
		return ""
	}
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, "..", "_")
	return strings.TrimSpace(name)
}

func updateTransferSpeedLocked(transfer *outboundFileTransfer) (float64, int64) {
	now := time.Now()
	if transfer.lastProgressAt.IsZero() {
		transfer.lastProgressAt = now
		transfer.lastProgressBytes = transfer.BytesSent
		return 0, 0
	}

	elapsed := now.Sub(transfer.lastProgressAt).Seconds()
	if elapsed > 0 {
		deltaBytes := transfer.BytesSent - transfer.lastProgressBytes
		if deltaBytes >= 0 {
			transfer.speedBytesPerSec = float64(deltaBytes) / elapsed
		}
	}
	transfer.lastProgressAt = now
	transfer.lastProgressBytes = transfer.BytesSent

	remaining := transfer.Filesize - transfer.BytesSent
	if remaining <= 0 || transfer.speedBytesPerSec <= 0 {
		return transfer.speedBytesPerSec, 0
	}
	etaSeconds := int64(float64(remaining) / transfer.speedBytesPerSec)
	if etaSeconds < 0 {
		etaSeconds = 0
	}
	return transfer.speedBytesPerSec, etaSeconds
}

func updateInboundTransferSpeedLocked(transfer *inboundFileTransfer) (float64, int64) {
	now := time.Now()
	if transfer.lastProgressAt.IsZero() {
		transfer.lastProgressAt = now
		transfer.lastProgressBytes = transfer.BytesReceived
		return 0, 0
	}

	elapsed := now.Sub(transfer.lastProgressAt).Seconds()
	if elapsed > 0 {
		deltaBytes := transfer.BytesReceived - transfer.lastProgressBytes
		if deltaBytes >= 0 {
			transfer.speedBytesPerSec = float64(deltaBytes) / elapsed
		}
	}
	transfer.lastProgressAt = now
	transfer.lastProgressBytes = transfer.BytesReceived

	remaining := transfer.Filesize - transfer.BytesReceived
	if remaining <= 0 || transfer.speedBytesPerSec <= 0 {
		return transfer.speedBytesPerSec, 0
	}
	etaSeconds := int64(float64(remaining) / transfer.speedBytesPerSec)
	if etaSeconds < 0 {
		etaSeconds = 0
	}
	return transfer.speedBytesPerSec, etaSeconds
}

package network

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
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

	fileCompleteStatusComplete = "complete"
	fileCompleteStatusFailed   = "failed"
)

type outboundFileTransfer struct {
	mu sync.Mutex

	FileID       string
	MessageID    string
	PeerDeviceID string

	SourcePath  string
	Filename    string
	Filesize    int64
	Filetype    string
	Checksum    string
	ChunkSize   int
	TotalChunks int

	NextChunk int
	BytesSent int64

	Running  bool
	Done     bool
	Rejected bool
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
}

type fileTransferEvent struct {
	Response *FileResponse
	Complete *FileComplete
}

// SendFile starts an outbound file transfer. If the peer is offline, transfer remains pending until reconnect.
func (m *PeerManager) SendFile(peerDeviceID, sourcePath string) (string, error) {
	return m.sendFileWithChecksumOverride(peerDeviceID, sourcePath, "")
}

func (m *PeerManager) sendFileWithChecksumOverride(peerDeviceID, sourcePath, checksumOverride string) (string, error) {
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
	if checksumOverride != "" {
		checksum = checksumOverride
	}

	filename := filepath.Base(sourcePath)
	filetype := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	totalChunks := chunkCount(fileInfo.Size(), m.options.FileChunkSize)

	transfer := &outboundFileTransfer{
		FileID:       uuid.NewString(),
		MessageID:    uuid.NewString(),
		PeerDeviceID: peerDeviceID,
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

	if conn := m.getConnection(peerDeviceID); conn != nil && conn.State() != StateDisconnected {
		m.beginOutboundFileTransfer(transfer, conn)
	}

	return transfer.FileID, nil
}

func (m *PeerManager) startOutboundFileTransferDrain(peerDeviceID string, conn *PeerConnection) {
	if peerDeviceID == "" || conn == nil {
		return
	}

	m.fileMu.Lock()
	transfers := make([]*outboundFileTransfer, 0)
	for _, transfer := range m.outboundFileTransfers {
		if transfer.PeerDeviceID == peerDeviceID {
			transfers = append(transfers, transfer)
		}
	}
	m.fileMu.Unlock()

	for _, transfer := range transfers {
		m.beginOutboundFileTransfer(transfer, conn)
	}
}

func (m *PeerManager) beginOutboundFileTransfer(transfer *outboundFileTransfer, conn *PeerConnection) {
	if transfer == nil || conn == nil {
		return
	}

	transfer.mu.Lock()
	if transfer.Running || transfer.Done || transfer.Rejected {
		transfer.mu.Unlock()
		return
	}
	transfer.Running = true
	transfer.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		err := m.runOutboundFileTransfer(transfer, conn)

		transfer.mu.Lock()
		transfer.Running = false
		pending := !transfer.Done && !transfer.Rejected
		transfer.mu.Unlock()

		if err != nil {
			m.reportError(fmt.Errorf("file transfer %q: %w", transfer.FileID, err))
		}
		if pending {
			_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "pending")
			if latest := m.getConnection(transfer.PeerDeviceID); latest != nil && latest != conn && latest.State() != StateDisconnected {
				m.beginOutboundFileTransfer(transfer, latest)
			}
		}
	}()
}

func (m *PeerManager) runOutboundFileTransfer(transfer *outboundFileTransfer, conn *PeerConnection) error {
	eventCh := m.registerOutboundFileEventChannel(transfer.FileID)
	defer m.unregisterOutboundFileEventChannel(transfer.FileID, eventCh)

	request := FileRequest{
		Type:         TypeFileRequest,
		FileID:       transfer.FileID,
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

	acceptResponse, err := m.waitForFileResponse(m.ctx, eventCh, func(response FileResponse) bool {
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
	transfer.mu.Unlock()

	file, err := os.Open(transfer.SourcePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	for chunkIndex := startChunk; chunkIndex < transfer.TotalChunks; chunkIndex++ {
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

			response, err := m.waitForFileResponse(m.ctx, eventCh, func(response FileResponse) bool {
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
		transfer.mu.Unlock()

		m.emitFileProgress(FileProgress{
			FileID:           transfer.FileID,
			PeerDeviceID:     transfer.PeerDeviceID,
			Direction:        fileTransferDirectionSend,
			BytesTransferred: bytesSent,
			TotalBytes:       transfer.Filesize,
			ChunkIndex:       chunkIndex,
			TotalChunks:      transfer.TotalChunks,
		})
	}

	if err := conn.SendMessage(FileComplete{
		Type:      TypeFileComplete,
		FileID:    transfer.FileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	}); err != nil {
		return err
	}

	complete, err := m.waitForFileComplete(m.ctx, eventCh, func(complete FileComplete) bool {
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
		return nil
	}

	transfer.mu.Lock()
	transfer.Done = true
	transfer.Running = false
	transfer.mu.Unlock()
	_ = m.options.Store.UpdateTransferStatus(transfer.FileID, "complete")

	m.emitFileProgress(FileProgress{
		FileID:            transfer.FileID,
		PeerDeviceID:      transfer.PeerDeviceID,
		Direction:         fileTransferDirectionSend,
		BytesTransferred:  transfer.Filesize,
		TotalBytes:        transfer.Filesize,
		ChunkIndex:        transfer.TotalChunks - 1,
		TotalChunks:       transfer.TotalChunks,
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
		return
	}
	if err := m.verifyFileRequest(conn, request); err != nil {
		m.reportError(err)
		_ = m.sendErrorMessage(conn, "invalid_signature", "file request signature verification failed", request.FileID)
		return
	}

	accept := true
	if m.options.OnFileRequest != nil {
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

	finalPath := filepath.Join(m.options.FilesDir, prefixedFilename(request.FileID, request.Filename))
	tempPath := finalPath + ".part"

	inbound := m.getInboundTransfer(request.FileID)
	if !accept {
		meta := storage.FileMetadata{
			FileID:         request.FileID,
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
		response := FileResponse{
			Type:         TypeFileResponse,
			FileID:       request.FileID,
			Status:       fileResponseStatusRejected,
			FromDeviceID: m.options.Identity.DeviceID,
			Timestamp:    time.Now().UnixMilli(),
			Message:      "file transfer rejected by user",
		}
		if err := m.signFileResponse(&response); err != nil {
			m.reportError(err)
			return
		}
		_ = conn.SendMessage(response)
		return
	}

	if inbound == nil {
		if err := os.MkdirAll(m.options.FilesDir, 0o700); err != nil {
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
	if response.FromDeviceID != "" && response.FromDeviceID != conn.PeerDeviceID() {
		return
	}
	if err := m.verifyFileResponse(conn, response); err != nil {
		m.reportError(err)
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
	transfer.mu.Unlock()

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
	})
}

func (m *PeerManager) handleFileComplete(conn *PeerConnection, complete FileComplete) {
	if complete.FileID == "" {
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
				m.emitFileProgress(FileProgress{
					FileID:            fileID,
					PeerDeviceID:      peerID,
					Direction:         fileTransferDirectionSend,
					BytesTransferred:  filesize,
					TotalBytes:        filesize,
					ChunkIndex:        totalChunks - 1,
					TotalChunks:       totalChunks,
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
		_ = conn.SendMessage(FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "sender marked transfer failed",
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	checksum, err := fileChecksumHex(inbound.TempPath)
	if err != nil {
		m.reportError(err)
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		_ = conn.SendMessage(FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "checksum verification failed",
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}
	if !strings.EqualFold(checksum, inbound.Checksum) {
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		_ = conn.SendMessage(FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "checksum mismatch",
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	if err := os.Rename(inbound.TempPath, inbound.FinalPath); err != nil {
		m.reportError(err)
		_ = m.options.Store.UpdateTransferStatus(complete.FileID, "failed")
		_ = conn.SendMessage(FileComplete{
			Type:      TypeFileComplete,
			FileID:    complete.FileID,
			Status:    fileCompleteStatusFailed,
			Message:   "finalize file failed",
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	_ = m.options.Store.UpdateTransferStatus(complete.FileID, "complete")
	m.removeInboundTransfer(complete.FileID)

	_ = conn.SendMessage(FileComplete{
		Type:      TypeFileComplete,
		FileID:    complete.FileID,
		Status:    fileCompleteStatusComplete,
		Timestamp: time.Now().UnixMilli(),
	})

	m.emitFileProgress(FileProgress{
		FileID:            complete.FileID,
		PeerDeviceID:      conn.PeerDeviceID(),
		Direction:         fileTransferDirectionReceive,
		BytesTransferred:  inbound.Filesize,
		TotalBytes:        inbound.Filesize,
		ChunkIndex:        inbound.TotalChunks - 1,
		TotalChunks:       inbound.TotalChunks,
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

func (m *PeerManager) verifyFileResponse(conn *PeerConnection, response FileResponse) error {
	if response.Signature == "" {
		return nil
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
	if !ed25519.Verify(publicKey, raw, signature) {
		return errors.New("invalid file response signature")
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

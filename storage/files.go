package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// SaveFileMetadata inserts a new file metadata row.
func (s *Store) SaveFileMetadata(file FileMetadata) error {
	if file.FileID == "" {
		return errors.New("file_id is required")
	}
	if file.Filename == "" {
		return errors.New("filename is required")
	}
	if file.StoredPath == "" {
		return errors.New("stored_path is required")
	}
	if file.Checksum == "" {
		return errors.New("checksum is required")
	}
	if file.TransferStatus == "" {
		file.TransferStatus = transferStatusPending
	}
	if err := validateTransferStatus(file.TransferStatus); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO files (
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.FileID,
		nullString(stringPointer(file.MessageID)),
		nullString(stringPointer(file.FolderID)),
		nullString(stringPointer(file.RelativePath)),
		nullString(stringPointer(file.FromDeviceID)),
		nullString(stringPointer(file.ToDeviceID)),
		file.Filename,
		file.Filesize,
		nullString(stringPointer(file.Filetype)),
		file.StoredPath,
		file.Checksum,
		nullInt64(file.TimestampReceived),
		file.TransferStatus,
	)
	if err != nil {
		return fmt.Errorf("insert file metadata %q: %w", file.FileID, err)
	}

	return nil
}

// UpdateTransferStatus updates transfer_status for a file row.
func (s *Store) UpdateTransferStatus(fileID, status string) error {
	if fileID == "" {
		return errors.New("file_id is required")
	}
	if err := validateTransferStatus(status); err != nil {
		return err
	}

	res, err := s.db.Exec(
		`UPDATE files
		SET transfer_status = ?
		WHERE file_id = ?`,
		status,
		fileID,
	)
	if err != nil {
		return fmt.Errorf("update file transfer status %q: %w", fileID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for file transfer status %q: %w", fileID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// GetFileByID fetches file metadata by file ID.
func (s *Store) GetFileByID(fileID string) (*FileMetadata, error) {
	row := s.db.QueryRow(
		`SELECT
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		FROM files
		WHERE file_id = ?`,
		fileID,
	)

	file, err := scanFileMetadata(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get file metadata %q: %w", fileID, err)
	}

	return file, nil
}

// ListFilesForPeer returns file metadata rows for one conversation peer.
func (s *Store) ListFilesForPeer(peerID string, limit, offset int) ([]FileMetadata, error) {
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}
	if limit <= 0 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(
		`SELECT
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		FROM files
		WHERE from_device_id = ? OR to_device_id = ?
		ORDER BY COALESCE(timestamp_received, 0) ASC, rowid ASC
		LIMIT ? OFFSET ?`,
		peerID,
		peerID,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list files for peer %q: %w", peerID, err)
	}
	defer rows.Close()

	files := make([]FileMetadata, 0)
	for rows.Next() {
		file, scanErr := scanFileMetadata(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan file metadata row: %w", scanErr)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file metadata rows: %w", err)
	}
	return files, nil
}

// SearchFilesForPeer returns file rows for one peer matching a text query.
func (s *Store) SearchFilesForPeer(peerID, query string, limit, offset int) ([]FileMetadata, error) {
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}
	if query == "" {
		return nil, errors.New("query is required")
	}
	if limit <= 0 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	pattern := "%" + query + "%"

	rows, err := s.db.Query(
		`SELECT
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		FROM files
		WHERE (from_device_id = ? OR to_device_id = ?)
		  AND (filename LIKE ? OR COALESCE(relative_path, '') LIKE ?)
		ORDER BY COALESCE(timestamp_received, 0) ASC, rowid ASC
		LIMIT ? OFFSET ?`,
		peerID,
		peerID,
		pattern,
		pattern,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search files for peer %q: %w", peerID, err)
	}
	defer rows.Close()

	files := make([]FileMetadata, 0)
	for rows.Next() {
		file, scanErr := scanFileMetadata(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan searched file metadata row: %w", scanErr)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate searched file metadata rows: %w", err)
	}
	return files, nil
}

// UpdateFileTimestampReceived sets timestamp_received for one file row.
func (s *Store) UpdateFileTimestampReceived(fileID string, timestampReceived int64) error {
	if fileID == "" {
		return errors.New("file_id is required")
	}
	if timestampReceived <= 0 {
		timestampReceived = nowUnixMilli()
	}

	res, err := s.db.Exec(
		`UPDATE files
		SET timestamp_received = ?
		WHERE file_id = ?`,
		timestampReceived,
		fileID,
	)
	if err != nil {
		return fmt.Errorf("update file timestamp_received %q: %w", fileID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update file timestamp_received %q: %w", fileID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListCompletedFilesOlderThan lists completed files with timestamp_received older than cutoff.
func (s *Store) ListCompletedFilesOlderThan(cutoffTimestamp int64) ([]FileMetadata, error) {
	if cutoffTimestamp <= 0 {
		return nil, errors.New("cutoff timestamp must be > 0")
	}

	rows, err := s.db.Query(
		`SELECT
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		FROM files
		WHERE transfer_status = ?
		  AND timestamp_received IS NOT NULL
		  AND timestamp_received < ?
		ORDER BY timestamp_received ASC, file_id ASC`,
		transferStatusComplete,
		cutoffTimestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("list completed files older than cutoff: %w", err)
	}
	defer rows.Close()

	files := make([]FileMetadata, 0)
	for rows.Next() {
		file, scanErr := scanFileMetadata(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan completed file metadata row: %w", scanErr)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completed file metadata rows: %w", err)
	}
	return files, nil
}

// DeleteFileMetadata removes one files row.
func (s *Store) DeleteFileMetadata(fileID string) error {
	if fileID == "" {
		return errors.New("file_id is required")
	}

	res, err := s.db.Exec(`DELETE FROM files WHERE file_id = ?`, fileID)
	if err != nil {
		return fmt.Errorf("delete file metadata %q: %w", fileID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for delete file metadata %q: %w", fileID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFileMetadataForPeer removes all file rows for one conversation peer.
func (s *Store) DeleteFileMetadataForPeer(peerID string) (int64, error) {
	if peerID == "" {
		return 0, errors.New("peer_id is required")
	}

	res, err := s.db.Exec(
		`DELETE FROM files
		WHERE from_device_id = ? OR to_device_id = ?`,
		peerID,
		peerID,
	)
	if err != nil {
		return 0, fmt.Errorf("delete file metadata for peer %q: %w", peerID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read rows affected for delete peer file metadata %q: %w", peerID, err)
	}
	return rowsAffected, nil
}

// UpsertFolderTransfer stores folder transfer envelope metadata.
func (s *Store) UpsertFolderTransfer(folder FolderTransferMetadata) error {
	if folder.FolderID == "" {
		return errors.New("folder_id is required")
	}
	if folder.FolderName == "" {
		return errors.New("folder_name is required")
	}
	if folder.RootPath == "" {
		return errors.New("root_path is required")
	}
	if folder.TotalFiles < 0 {
		return errors.New("total_files must be >= 0")
	}
	if folder.TotalSize < 0 {
		return errors.New("total_size must be >= 0")
	}
	if folder.TransferStatus == "" {
		folder.TransferStatus = transferStatusPending
	}
	if err := validateTransferStatus(folder.TransferStatus); err != nil {
		return err
	}
	if folder.Timestamp == 0 {
		folder.Timestamp = nowUnixMilli()
	}

	_, err := s.db.Exec(
		`INSERT INTO folder_transfers (
			folder_id,
			from_device_id,
			to_device_id,
			folder_name,
			root_path,
			total_files,
			total_size,
			transfer_status,
			timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(folder_id) DO UPDATE SET
			from_device_id = excluded.from_device_id,
			to_device_id = excluded.to_device_id,
			folder_name = excluded.folder_name,
			root_path = excluded.root_path,
			total_files = excluded.total_files,
			total_size = excluded.total_size,
			transfer_status = excluded.transfer_status,
			timestamp = excluded.timestamp`,
		folder.FolderID,
		nullString(stringPointer(folder.FromDeviceID)),
		nullString(stringPointer(folder.ToDeviceID)),
		folder.FolderName,
		folder.RootPath,
		folder.TotalFiles,
		folder.TotalSize,
		folder.TransferStatus,
		folder.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("upsert folder transfer %q: %w", folder.FolderID, err)
	}
	return nil
}

// UpdateFolderTransferStatus updates transfer_status for a folder transfer row.
func (s *Store) UpdateFolderTransferStatus(folderID, status string) error {
	if folderID == "" {
		return errors.New("folder_id is required")
	}
	if err := validateTransferStatus(status); err != nil {
		return err
	}

	res, err := s.db.Exec(
		`UPDATE folder_transfers
		SET transfer_status = ?
		WHERE folder_id = ?`,
		status,
		folderID,
	)
	if err != nil {
		return fmt.Errorf("update folder transfer status %q: %w", folderID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for folder transfer status %q: %w", folderID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetFolderTransfer fetches folder transfer metadata by ID.
func (s *Store) GetFolderTransfer(folderID string) (*FolderTransferMetadata, error) {
	if folderID == "" {
		return nil, errors.New("folder_id is required")
	}

	row := s.db.QueryRow(
		`SELECT
			folder_id,
			from_device_id,
			to_device_id,
			folder_name,
			root_path,
			total_files,
			total_size,
			transfer_status,
			timestamp
		FROM folder_transfers
		WHERE folder_id = ?`,
		folderID,
	)

	folder, err := scanFolderTransferMetadata(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get folder transfer %q: %w", folderID, err)
	}
	return folder, nil
}

// ListFilesByFolderID returns file rows linked to a folder transfer.
func (s *Store) ListFilesByFolderID(folderID string) ([]FileMetadata, error) {
	if folderID == "" {
		return nil, errors.New("folder_id is required")
	}

	rows, err := s.db.Query(
		`SELECT
			file_id,
			message_id,
			folder_id,
			relative_path,
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		FROM files
		WHERE folder_id = ?
		ORDER BY COALESCE(relative_path, ''), file_id`,
		folderID,
	)
	if err != nil {
		return nil, fmt.Errorf("list files by folder_id %q: %w", folderID, err)
	}
	defer rows.Close()

	files := make([]FileMetadata, 0)
	for rows.Next() {
		file, scanErr := scanFileMetadata(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan file metadata by folder_id %q: %w", folderID, scanErr)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate files by folder_id %q: %w", folderID, err)
	}
	return files, nil
}

// UpsertTransferCheckpoint inserts or updates resumable transfer state.
func (s *Store) UpsertTransferCheckpoint(checkpoint TransferCheckpoint) error {
	if checkpoint.FileID == "" {
		return errors.New("file_id is required")
	}
	if err := validateTransferDirection(checkpoint.Direction); err != nil {
		return err
	}
	if checkpoint.NextChunk < 0 {
		return errors.New("next_chunk must be >= 0")
	}
	if checkpoint.BytesTransferred < 0 {
		return errors.New("bytes_transferred must be >= 0")
	}
	if checkpoint.UpdatedAt == 0 {
		checkpoint.UpdatedAt = nowUnixMilli()
	}

	_, err := s.db.Exec(
		`INSERT INTO transfer_checkpoints (
			file_id,
			direction,
			next_chunk,
			bytes_transferred,
			temp_path,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_id, direction) DO UPDATE SET
			next_chunk = excluded.next_chunk,
			bytes_transferred = excluded.bytes_transferred,
			temp_path = excluded.temp_path,
			updated_at = excluded.updated_at`,
		checkpoint.FileID,
		checkpoint.Direction,
		checkpoint.NextChunk,
		checkpoint.BytesTransferred,
		checkpoint.TempPath,
		checkpoint.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert transfer checkpoint %q/%q: %w", checkpoint.FileID, checkpoint.Direction, err)
	}
	return nil
}

// DeleteTransferCheckpoint removes one transfer checkpoint row.
func (s *Store) DeleteTransferCheckpoint(fileID, direction string) error {
	if fileID == "" {
		return errors.New("file_id is required")
	}
	if err := validateTransferDirection(direction); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`DELETE FROM transfer_checkpoints
		WHERE file_id = ? AND direction = ?`,
		fileID,
		direction,
	)
	if err != nil {
		return fmt.Errorf("delete transfer checkpoint %q/%q: %w", fileID, direction, err)
	}
	return nil
}

// GetTransferCheckpoint fetches one checkpoint by file and direction.
func (s *Store) GetTransferCheckpoint(fileID, direction string) (*TransferCheckpoint, error) {
	if fileID == "" {
		return nil, errors.New("file_id is required")
	}
	if err := validateTransferDirection(direction); err != nil {
		return nil, err
	}

	row := s.db.QueryRow(
		`SELECT
			file_id,
			direction,
			next_chunk,
			bytes_transferred,
			temp_path,
			updated_at
		FROM transfer_checkpoints
		WHERE file_id = ? AND direction = ?`,
		fileID,
		direction,
	)

	checkpoint, err := scanTransferCheckpoint(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get transfer checkpoint %q/%q: %w", fileID, direction, err)
	}
	return checkpoint, nil
}

// ListTransferCheckpoints returns persisted checkpoints, optionally filtered by direction.
func (s *Store) ListTransferCheckpoints(direction string) ([]TransferCheckpoint, error) {
	query := `SELECT
		file_id,
		direction,
		next_chunk,
		bytes_transferred,
		temp_path,
		updated_at
	FROM transfer_checkpoints`
	args := make([]any, 0, 1)
	if direction != "" {
		if err := validateTransferDirection(direction); err != nil {
			return nil, err
		}
		query += " WHERE direction = ?"
		args = append(args, direction)
	}
	query += " ORDER BY updated_at DESC, file_id, direction"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list transfer checkpoints: %w", err)
	}
	defer rows.Close()

	checkpoints := make([]TransferCheckpoint, 0)
	for rows.Next() {
		checkpoint, scanErr := scanTransferCheckpoint(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan transfer checkpoint row: %w", scanErr)
		}
		checkpoints = append(checkpoints, *checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transfer checkpoint rows: %w", err)
	}
	return checkpoints, nil
}

func scanFileMetadata(row scanner) (*FileMetadata, error) {
	var (
		file              FileMetadata
		messageID         sql.NullString
		folderID          sql.NullString
		relativePath      sql.NullString
		fromDeviceID      sql.NullString
		toDeviceID        sql.NullString
		fileType          sql.NullString
		timestampReceived sql.NullInt64
	)

	if err := row.Scan(
		&file.FileID,
		&messageID,
		&folderID,
		&relativePath,
		&fromDeviceID,
		&toDeviceID,
		&file.Filename,
		&file.Filesize,
		&fileType,
		&file.StoredPath,
		&file.Checksum,
		&timestampReceived,
		&file.TransferStatus,
	); err != nil {
		return nil, err
	}

	if messageID.Valid {
		file.MessageID = messageID.String
	}
	if folderID.Valid {
		file.FolderID = folderID.String
	}
	if relativePath.Valid {
		file.RelativePath = relativePath.String
	}
	if fromDeviceID.Valid {
		file.FromDeviceID = fromDeviceID.String
	}
	if toDeviceID.Valid {
		file.ToDeviceID = toDeviceID.String
	}
	if fileType.Valid {
		file.Filetype = fileType.String
	}
	file.TimestampReceived = int64Ptr(timestampReceived)

	return &file, nil
}

func scanFolderTransferMetadata(row scanner) (*FolderTransferMetadata, error) {
	var (
		folder       FolderTransferMetadata
		fromDeviceID sql.NullString
		toDeviceID   sql.NullString
	)
	if err := row.Scan(
		&folder.FolderID,
		&fromDeviceID,
		&toDeviceID,
		&folder.FolderName,
		&folder.RootPath,
		&folder.TotalFiles,
		&folder.TotalSize,
		&folder.TransferStatus,
		&folder.Timestamp,
	); err != nil {
		return nil, err
	}
	if fromDeviceID.Valid {
		folder.FromDeviceID = fromDeviceID.String
	}
	if toDeviceID.Valid {
		folder.ToDeviceID = toDeviceID.String
	}
	return &folder, nil
}

func scanTransferCheckpoint(row scanner) (*TransferCheckpoint, error) {
	var checkpoint TransferCheckpoint
	if err := row.Scan(
		&checkpoint.FileID,
		&checkpoint.Direction,
		&checkpoint.NextChunk,
		&checkpoint.BytesTransferred,
		&checkpoint.TempPath,
		&checkpoint.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func stringPointer(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

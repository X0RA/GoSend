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
			from_device_id,
			to_device_id,
			filename,
			filesize,
			filetype,
			stored_path,
			checksum,
			timestamp_received,
			transfer_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.FileID,
		nullString(stringPointer(file.MessageID)),
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
		fromDeviceID      sql.NullString
		toDeviceID        sql.NullString
		fileType          sql.NullString
		timestampReceived sql.NullInt64
	)

	if err := row.Scan(
		&file.FileID,
		&messageID,
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

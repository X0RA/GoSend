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
		file.MessageID,
		file.FromDeviceID,
		file.ToDeviceID,
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

func stringPointer(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

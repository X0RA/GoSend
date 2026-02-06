package models

// File represents metadata for a transferred file.
type File struct {
	FileID            string `json:"file_id"`
	MessageID         string `json:"message_id"`
	Filename          string `json:"filename"`
	Filesize          int64  `json:"filesize"`
	Filetype          string `json:"filetype"`
	StoredPath        string `json:"stored_path"`
	Checksum          string `json:"checksum"`
	TimestampReceived int64  `json:"timestamp_received"`
	TransferStatus    string `json:"transfer_status"`
}

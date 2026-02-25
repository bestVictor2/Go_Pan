package model

import "time"

type FileChunk struct {
	ID uint64 `gorm:"primaryKey"`

	UploadID string `gorm:"column:upload_id;size:36;not null;uniqueIndex:idx_upload_chunk"`

	ChunkIndex int    `gorm:"column:chunk_index;not null;uniqueIndex:idx_upload_chunk"`
	ChunkSize  int64  `gorm:"column:chunk_size;not null"`
	ChunkPath  string `gorm:"column:chunk_path;size:512;not null"`

	Status int `gorm:"column:status;not null;default:0"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the database table name.
func (FileChunk) TableName() string {
	return "file_chunk"
}

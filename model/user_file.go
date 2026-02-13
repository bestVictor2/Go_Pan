package model

import (
	"gorm.io/gorm"
	"time"
)

type UserFile struct {
	ID uint64 `gorm:"primaryKey" json:"id,omitempty"`

	UserID uint64 `gorm:"column:user_id;not null;uniqueIndex:uk_user_parent_name,priority:1" json:"user_id,omitempty"`
	User   User   `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`

	ParentID *uint64   `gorm:"column:parent_id;index;uniqueIndex:uk_user_parent_name,priority:2" json:"parent_id,omitempty"`
	Parent   *UserFile `gorm:"foreignKey:ParentID;references:ID"`

	Name string `gorm:"column:name;size:255;not null;uniqueIndex:uk_user_parent_name,priority:3" json:"name,omitempty"`

	IsDir bool `gorm:"column:is_dir;not null;default:false" json:"is_dir,omitempty"`

	ObjectID *uint64     `gorm:"column:object_id;index" json:"object_id,omitempty"`
	Object   *FileObject `gorm:"foreignKey:ObjectID;references:ID" json:"object,omitempty"`

	Size int64 `gorm:"column:size;not null;default:0" json:"size,omitempty"`

	IsDeleted bool           `gorm:"column:is_deleted;default:false" json:"is_deleted,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// TableName returns the database table name.
func (UserFile) TableName() string {
	return "user_file"
}




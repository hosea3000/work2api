package model

import (
	"time"
)

// AdminUser 单管理员账户。username 保留字段以便二期放开多用户。
type AdminUser struct {
	Id        int64  `gorm:"primarykey;autoIncrement"`
	Username  string `gorm:"uniqueIndex;size:64;not null"`
	Password  string `gorm:"size:128;not null"` // bcrypt
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AdminUser) TableName() string { return "admin_user" }

// APIKey sk- 密钥；仅存 SHA-256 哈希。
type APIKey struct {
	Id        string `gorm:"primaryKey;size:36"`
	Name      string `gorm:"size:128;not null"`
	KeyHash   string `gorm:"uniqueIndex;size:64;not null"`
	KeySuffix string `gorm:"size:16;not null"`
	Disabled  bool   `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (APIKey) TableName() string { return "api_key" }

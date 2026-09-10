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

// Credential CodeBuddy 上游凭证。SQLite 为持久层真相，内存池由它刷新。
type Credential struct {
	Id                 string  `gorm:"primaryKey;size:36"`
	BearerToken        string  `gorm:"type:text;not null"`
	UserId             string  `gorm:"index;size:128;not null"`
	AccountUid         *string `gorm:"size:128"`
	Domain             *string `gorm:"size:255"`
	EnterpriseId       *string `gorm:"size:128"`
	DepartmentFullName *string `gorm:"size:512"`
	AuthSource         string  `gorm:"size:16;not null;default:manual"`
	Provider           string  `gorm:"size:16;not null;default:codebuddy;index"` // codebuddy | trae
	Status             string  `gorm:"size:16;not null;default:active;index"`    // active | expired | disabled
	// TRAE 专用列（网页登录时生成的设备标识，登录态与凭证绑定）
	MachineID *string `gorm:"size:64"`
	DeviceID  *string `gorm:"size:64"`
	ExpiresAt *int64
	// OAuth 扩展列（manual 凭证为 NULL）
	RefreshToken     *string `gorm:"type:text"`
	RefreshExpiresAt *int64
	ExpiresIn        *int64
	SessionState     *string `gorm:"size:255"`
	Scope            *string `gorm:"size:512"`
	LastRefreshAt    *int64
	// 用户身份展示字段（JWT 解析，manual/oauth 均可提取）
	Nickname          *string `gorm:"size:255"`
	PreferredUsername *string `gorm:"size:255"`
	Email             *string `gorm:"size:255"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Credential) TableName() string { return "credential" }

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

// CheckinRecord 每次签到尝试的记录。
type CheckinRecord struct {
	Id           int64  `gorm:"primarykey;autoIncrement"`
	CredentialId string `gorm:"index:idx_cred_date,unique;size:36;not null"`
	CheckinDate  string `gorm:"index:idx_cred_date,unique;size:10;not null"` // 本地时区 YYYY-MM-DD
	Success      bool   `gorm:"not null"`
	Code         *int
	Message      string `gorm:"size:255"`
	Credit       *float64
	AttemptedAt  int64 `gorm:"not null"`
	CheckedInAt  *int64
	CreatedAt    time.Time
}

func (CheckinRecord) TableName() string { return "checkin_record" }

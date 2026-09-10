package model

import "time"

// CodeBuddyCredential CodeBuddy 上游凭证。与 TraeCredential 物理分表，互不引用。
type CodeBuddyCredential struct {
	Id                 string  `gorm:"primaryKey;size:36"`
	BearerToken        string  `gorm:"type:text;not null"`
	UserId             string  `gorm:"index;size:128;not null"`
	AccountUid         *string `gorm:"size:128"`
	Domain             *string `gorm:"size:255"`
	EnterpriseId       *string `gorm:"size:128"`
	DepartmentFullName *string `gorm:"size:512"`
	AuthSource         string  `gorm:"size:16;not null;default:manual"` // manual | oauth
	Status             string  `gorm:"size:16;not null;default:active;index"`
	ExpiresAt          *int64
	// OAuth 扩展列（manual 凭证为 NULL）
	RefreshToken     *string `gorm:"type:text"`
	RefreshExpiresAt *int64
	ExpiresIn        *int64
	SessionState     *string `gorm:"size:255"`
	Scope            *string `gorm:"size:512"`
	LastRefreshAt    *int64
	// 用户身份展示字段（JWT 解析）
	Nickname          *string `gorm:"size:255"`
	PreferredUsername *string `gorm:"size:255"`
	Email             *string `gorm:"size:255"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (CodeBuddyCredential) TableName() string { return "codebuddy_credential" }

// TraeCredential TRAE SOLO 凭证。与 CodeBuddyCredential 物理分表，互不引用。
type TraeCredential struct {
	Id           string  `gorm:"primaryKey;size:36"`
	BearerToken  string  `gorm:"type:text;not null"`
	UserId       string  `gorm:"index;size:128;not null"`
	MachineID    *string `gorm:"size:64"`
	DeviceID     *string `gorm:"size:64"`
	AuthSource   string  `gorm:"size:16;not null;default:web_login"` // web_login
	Status       string  `gorm:"size:16;not null;default:active;index"`
	ExpiresAt    *int64
	RefreshToken *string `gorm:"type:text"`
	LastRefreshAt *int64
	Nickname     *string `gorm:"size:255"`
	Email        *string `gorm:"size:255"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TraeCredential) TableName() string { return "trae_credential" }

// CodeBuddyCheckinRecord codebuddy 每次签到尝试的记录。
type CodeBuddyCheckinRecord struct {
	Id           int64  `gorm:"primarykey;autoIncrement"`
	CredentialId string `gorm:"index:idx_cb_cred_date,unique;size:36;not null"`
	CheckinDate  string `gorm:"index:idx_cb_cred_date,unique;size:10;not null"` // 本地时区 YYYY-MM-DD
	Success      bool   `gorm:"not null"`
	Code         *int
	Message      string `gorm:"size:255"`
	Credit       *float64
	AttemptedAt  int64 `gorm:"not null"`
	CheckedInAt  *int64
	CreatedAt    time.Time
}

func (CodeBuddyCheckinRecord) TableName() string { return "codebuddy_checkin_record" }

// TraeCheckinRecord trae 每次签到尝试的记录。
type TraeCheckinRecord struct {
	Id           int64  `gorm:"primarykey;autoIncrement"`
	CredentialId string `gorm:"index:idx_trae_cred_date,unique;size:36;not null"`
	CheckinDate  string `gorm:"index:idx_trae_cred_date,unique;size:10;not null"`
	Success      bool   `gorm:"not null"`
	Code         *int
	Message      string `gorm:"size:255"`
	Credit       *float64
	AttemptedAt  int64 `gorm:"not null"`
	CheckedInAt  *int64
	CreatedAt    time.Time
}

func (TraeCheckinRecord) TableName() string { return "trae_checkin_record" }

// PoolState 每个 provider 的池状态（当前凭证 + 轮换开关），持久化以跨重启保持。
type PoolState struct {
	Provider            string  `gorm:"primaryKey;size:16"` // codebuddy | trae
	AutoRotation        bool    `gorm:"not null;default:true"`
	CurrentCredentialId *string `gorm:"size:36"`
	UpdatedAt           time.Time
}

func (PoolState) TableName() string { return "pool_state" }

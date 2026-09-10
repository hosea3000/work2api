package model

// RequestRecord 每次 chat completions 调用的持久化记录（仅用于计数与成功率）。
type RequestRecord struct {
	Id        int64  `gorm:"primarykey;autoIncrement"`
	StartedAt int64  `gorm:"index;not null"`   // Unix 秒
	Outcome   string `gorm:"size:16;not null"` // success | failure
}

func (RequestRecord) TableName() string { return "request_record" }

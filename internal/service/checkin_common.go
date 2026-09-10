package service

import "time"

// CheckinDetail 单次签到结果（两 provider 共用）。
type CheckinDetail struct {
	CredentialId string
	CheckinDate  string
	Success      bool
	Code         *int
	Message      string
	Credit       *float64
	AttemptedAt  int64
	CheckedInAt  *int64
}

func localDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func checkinMap(credentialId string, d CheckinDetail) map[string]any {
	return map[string]any{
		"credential_id": credentialId,
		"success":       d.Success,
		"code":          d.Code,
		"message":       d.Message,
		"credit":        d.Credit,
		"date":          d.CheckinDate,
	}
}

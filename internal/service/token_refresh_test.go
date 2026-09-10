package service

import (
	"testing"
)

func strPtr(s string) *string { return &s }
func i64Ptr(i int64) *int64   { return &i }

func TestShouldRefreshMatrix(t *testing.T) {
	now := int64(1789000000)
	rt := strPtr("rt")
	emptyRT := strPtr("")
	oauth := "oauth"
	manual := "manual"

	cases := []struct {
		name string
		cred credentialForRefresh
		now  int64
		want bool
	}{
		{"临期(差1h)→刷", credentialForRefresh{oauth, rt, i64Ptr(now + 86400*30), i64Ptr(now + 3600)}, now, true},
		{"正好跨过24h线→刷", credentialForRefresh{oauth, rt, i64Ptr(now + 86400*30), i64Ptr(now + RefreshWindowSeconds)}, now, true},
		{"还有30天才到期→不刷", credentialForRefresh{oauth, rt, i64Ptr(now + 86400*30), i64Ptr(now + 86400*30 + 3600)}, now, false},
		{"已过期→刷(临期)", credentialForRefresh{oauth, rt, i64Ptr(now + 86400*30), i64Ptr(now - 10)}, now, true},
		{"manual来源→永不刷", credentialForRefresh{manual, rt, i64Ptr(now + 86400*30), i64Ptr(now + 3600)}, now, false},
		{"无refresh_token→不刷", credentialForRefresh{oauth, nil, i64Ptr(now + 86400*30), i64Ptr(now + 3600)}, now, false},
		{"refresh_token空串→不刷", credentialForRefresh{oauth, emptyRT, i64Ptr(now + 86400*30), i64Ptr(now + 3600)}, now, false},
		{"refresh_token已过期→不刷", credentialForRefresh{oauth, rt, i64Ptr(now - 5), i64Ptr(now + 3600)}, now, false},
		{"无expires_at→不刷", credentialForRefresh{oauth, rt, i64Ptr(now + 86400*30), nil}, now, false},
	}
	for _, tc := range cases {
		if got := ShouldRefresh(&tc.cred, tc.now); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	// nil 凭证安全
	if ShouldRefresh(nil, now) {
		t.Error("nil credential must not refresh")
	}
}

func TestScanSkipsTraeCredentials(t *testing.T) {
	// trae 凭证不得进入 codebuddy 刷新扫描
	if !isCodebuddyView(CredentialView{Provider: "codebuddy"}) {
		t.Error("codebuddy view should pass filter")
	}
	if !isCodebuddyView(CredentialView{}) {
		t.Error("legacy empty provider should pass filter")
	}
	if isCodebuddyView(CredentialView{Provider: "trae"}) {
		t.Error("trae view must be excluded from codebuddy refresh scan")
	}
}

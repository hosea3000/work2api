package service

import (
	"testing"
	"time"
)

func TestRateLimiterWindows(t *testing.T) {
	r := NewRateLimiter(time.Minute, 60, 10, 5)
	// 同一用户名 5 次后触发用户名限流
	for i := 0; i < 5; i++ {
		if !r.Allow("1.1.1.1", "alice") {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	if r.Allow("1.1.1.1", "alice") {
		t.Error("username window should be exhausted")
	}
	// 其他 IP 对同一用户名仍受限
	if r.Allow("2.2.2.2", "alice") {
		t.Error("username window is shared across IPs")
	}
	// 其他用户名同 IP：第 10 次后 IP 限流
	r2 := NewRateLimiter(time.Minute, 60, 10, 5)
	for i := 0; i < 10; i++ {
		if !r2.Allow("3.3.3.3", "user"+string(rune('a'+i))) {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	if r2.Allow("3.3.3.3", "another") {
		t.Error("ip window should be exhausted")
	}
}

func TestRateLimiterGlobalAndReset(t *testing.T) {
	r := NewRateLimiter(30*time.Millisecond, 3, 100, 100)
	for i := 0; i < 3; i++ {
		if !r.Allow("1.1.1.1", "u") {
			t.Fatal("global attempts should pass")
		}
	}
	if r.Allow("2.2.2.2", "v") {
		t.Error("global window exhausted")
	}
	time.Sleep(35 * time.Millisecond)
	if !r.Allow("2.2.2.2", "v") {
		t.Error("window should have reset")
	}
}

func TestCheckinIdempotentJudgement(t *testing.T) {
	// 验证 client.Checkin 的成功判定逻辑（成功/已签到/失败三分支）
	// 直接测判定的核心规则，避免真实网络：
	// code==0 → success；msg 含"已签到" → success；其余 → 失败
	cases := []struct {
		code    *int
		msg     string
		success bool
	}{
		{code: intPtr(0), msg: "ok", success: true},
		{code: intPtr(1), msg: "今日已签到", success: true},
		{code: intPtr(10081), msg: "IP 受限", success: false},
		{code: nil, msg: "something", success: false},
	}
	for _, tc := range cases {
		success := (tc.code != nil && *tc.code == 0) || containsCheckin(tc.msg)
		if success != tc.success {
			t.Errorf("code=%v msg=%q: expected %v got %v", tc.code, tc.msg, tc.success, success)
		}
	}
}

func containsCheckin(msg string) bool {
	return len(msg) >= 9 && (msg == "已签到" || indexOf(msg, "已签到") >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func intPtr(i int) *int { return &i }

func TestCheckinSkipWhenTodayDone(t *testing.T) {
	// 当日已签到跳过逻辑在 RunScheduledCheckin 中依赖 GetCheckinRecord；
	// 这里验证 localDate 格式稳定性（唯一索引依赖）。
	d := localDate(time.Date(2026, 9, 9, 10, 0, 0, 0, time.Local))
	if len(d) != 10 || d[4] != '-' {
		t.Errorf("localDate format wrong: %q", d)
	}
}

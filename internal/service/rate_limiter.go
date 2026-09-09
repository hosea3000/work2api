package service

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter 三滑动窗口限流：全局 / IP / 用户名。
type RateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	globalMax   int
	ipMax       int
	usernameMax int
	events      map[string][]time.Time // key → 时间戳列表
}

func NewRateLimiter(window time.Duration, globalMax, ipMax, usernameMax int) *RateLimiter {
	return &RateLimiter{
		window:      window,
		globalMax:   globalMax,
		ipMax:       ipMax,
		usernameMax: usernameMax,
		events:      map[string][]time.Time{},
	}
}

// Allow 记录一次尝试并判定是否放行。
func (r *RateLimiter) Allow(ip, username string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	prune := func(key string) []time.Time {
		evs := r.events[key]
		kept := evs[:0]
		for _, t := range evs {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		r.events[key] = kept
		return kept
	}
	global := prune("__global__")
	ipEvents := prune("ip:" + ip)
	userEvents := prune("user:" + username)
	if len(global) >= r.globalMax || len(ipEvents) >= r.ipMax || len(userEvents) >= r.usernameMax {
		return false
	}
	r.events["__global__"] = append(global, now)
	r.events["ip:"+ip] = append(ipEvents, now)
	r.events["user:"+username] = append(userEvents, now)
	return true
}

// ClientIP 提取客户端 IP。
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		for i, part := range splitAndTrim(fwd, ",") {
			if i == 0 {
				return part
			}
		}
	}
	if r.RemoteAddr != "" {
		if idx := lastColon(r.RemoteAddr); idx > 0 {
			return r.RemoteAddr[:idx]
		}
		return r.RemoteAddr
	}
	return "unknown"
}

func splitAndTrim(s, sep string) []string {
	var out []string
	for _, p := range splitStr(s, sep) {
		out = append(out, trimSpace(p))
	}
	return out
}

func splitStr(s, sep string) []string {
	var out []string
	for len(s) > 0 {
		idx := indexStr(s, sep)
		if idx < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:idx])
		s = s[idx+len(sep):]
	}
	return out
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

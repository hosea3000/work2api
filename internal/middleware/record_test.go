package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRecordRequestOutcomeByStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		status  int
		success bool
	}{
		{http.StatusOK, true},
		{http.StatusBadRequest, false},
		{http.StatusInternalServerError, false},
	}
	for _, tc := range cases {
		stats := &fakeStats{}
		r := gin.New()
		r.POST("/chat", RecordRequest(nil, stats), func(c *gin.Context) { c.Status(tc.status) })

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/chat", nil)
		r.ServeHTTP(w, req)

		if stats.calls != 1 {
			t.Fatalf("status %d: record calls = %d, want 1", tc.status, stats.calls)
		}
		if stats.lastSuccess == nil || *stats.lastSuccess != tc.success {
			t.Errorf("status %d: recorded success = %v, want %v", tc.status, stats.lastSuccess, tc.success)
		}
	}
}

func TestRecordRequestIgnoresRecordError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := &fakeStats{recordErr: context.DeadlineExceeded}
	r := gin.New()
	r.POST("/chat", RecordRequest(nil, stats), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/chat", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("record error should not change response, got %d", w.Code)
	}
}

type fakeStats struct {
	calls       int
	lastSuccess *bool
	recordErr   error
}

func (f *fakeStats) Record(_ context.Context, _ int64, success bool) error {
	f.calls++
	f.lastSuccess = &success
	return f.recordErr
}

func (f *fakeStats) Overview(_ context.Context, _, _ int64) (int64, int64, error) {
	return 0, 0, nil
}

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/upstream/trae"
)

func mkTraeExec(t *testing.T, handler http.HandlerFunc) (*TraeChatExecutor, TraeCredentialService, *fakeTraeRepo) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := trae.New()
	c.AgentHost = srv.URL

	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	return NewTraeChatExecutor(svc, c), svc, repo
}

func putTraeCred(repo *fakeTraeRepo, id string) {
	device := "dev-1"
	machine := "mach-1"
	repo.saved = append(repo.saved, model.TraeCredential{
		Id: id, AuthSource: "web_login", Status: "active",
		BearerToken: "at-" + id, UserId: "u-" + id, DeviceID: &device, MachineID: &machine,
	})
}

func chatBody(stream bool) map[string]any {
	return map[string]any{
		"model":    "glm-5.2",
		"stream":   stream,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
}

func TestTraeExecuteChatNonStream(t *testing.T) {
	exec, svc, repo := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != trae.EpChat {
			t.Errorf("path=%s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Cloud-IDE-JWT at-") {
			t.Error("missing SOLO auth")
		}
		_, _ = w.Write([]byte("event:output\ndata:{\"response\":\"你好\",\"reasoning_content\":\"想\",\"tool_calls\":null}\n\n" +
			"event:token_usage\ndata:{\"total_tokens\":5}\n\n" +
			"event:done\ndata:{\"finish_reason\":\"stop\"}\n\n"))
	})
	putTraeCred(repo, "t1")
	svc.PoolReload(context.Background())

	res, chatErr, done := exec.ExecuteChat(context.Background(), chatBody(false), nil)
	if chatErr != nil || !done {
		t.Fatalf("chatErr=%v done=%v", chatErr, done)
	}
	if res["model"] != "glm-5.2" {
		t.Errorf("model=%v", res["model"])
	}
	msg := res["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "你好" || msg["reasoning_content"] != "想" {
		t.Errorf("message=%v", msg)
	}
}

func TestTraeExecuteChatStream(t *testing.T) {
	exec, svc, repo := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("event:output\ndata:{\"response\":\"hi\",\"reasoning_content\":\"\",\"tool_calls\":null}\n\n" +
			"event:done\ndata:{\"finish_reason\":\"stop\"}\n\n"))
	})
	putTraeCred(repo, "t1")
	svc.PoolReload(context.Background())

	rec := httptest.NewRecorder()
	_, chatErr, done := exec.ExecuteChat(context.Background(), chatBody(true), rec)
	if chatErr != nil || done {
		t.Fatalf("chatErr=%v done=%v", chatErr, done)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "chat.completion.chunk") || !strings.Contains(body, "data: [DONE]") {
		t.Errorf("stream body=%q", body)
	}
}

func TestTraeExecuteChatNoCredential(t *testing.T) {
	exec, _, _ := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {})
	_, chatErr, done := exec.ExecuteChat(context.Background(), chatBody(false), nil)
	if chatErr == nil || chatErr.Status != 503 || !done {
		t.Fatalf("want 503, got %+v", chatErr)
	}
}

func TestTraeExecuteChatPlanLimitCoolsDown(t *testing.T) {
	exec, svc, repo := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("event:error\ndata:{\"code\":1005,\"message\":\"plan limit\"}\n\n"))
	})
	putTraeCred(repo, "t1")
	svc.PoolReload(context.Background())

	_, chatErr, _ := exec.ExecuteChat(context.Background(), chatBody(false), nil)
	if chatErr == nil || chatErr.Status != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %+v", chatErr)
	}
	if _, ok := svc.SelectForChat(context.Background()); ok {
		t.Error("credential should be cooling after 1005")
	}
}

func TestTraeExecuteChatHTTP401MarksExpired(t *testing.T) {
	exec, svc, repo := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":1001}`))
	})
	putTraeCred(repo, "t1")
	svc.PoolReload(context.Background())

	_, chatErr, _ := exec.ExecuteChat(context.Background(), chatBody(false), nil)
	if chatErr == nil || chatErr.Status != http.StatusUnauthorized {
		t.Fatalf("want 401, got %+v", chatErr)
	}
	if repo.saved[0].Status != "expired" {
		t.Errorf("status=%s want expired", repo.saved[0].Status)
	}
}

func TestTraeExecuteChatValidatesMessages(t *testing.T) {
	exec, _, _ := mkTraeExec(t, func(w http.ResponseWriter, r *http.Request) {})
	_, chatErr, done := exec.ExecuteChat(context.Background(), map[string]any{"model": "x"}, nil)
	if chatErr == nil || chatErr.Status != 400 || !done {
		t.Fatalf("want 400, got %+v", chatErr)
	}
	body := chatBody(false)
	body["stop"] = []any{"END"}
	_, chatErr, _ = exec.ExecuteChat(context.Background(), body, nil)
	if chatErr != nil && chatErr.Status == 400 {
		t.Errorf("trae should pass through stop, got %+v", chatErr)
	}
}

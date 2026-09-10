package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hosea3000/work2api/internal/upstream/trae"
)

func TestTraeModelsAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != trae.EpModels {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"config_info_list":[
			{"config_name":"glm-5.2","display_config":{"display_name":"GLM-5.2"}},
			{"config_name":"glm-5.3","display_config":{"display_name":"GLM-5.3"}}
		]}`))
	}))
	defer srv.Close()

	c := trae.New()
	c.AgentHost = srv.URL
	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	repo.saved = append(repo.saved, newTraeCred("t1", "active"))
	svc.PoolReload(context.Background())

	models := NewTraeModelsService(c, svc, 30).Available(context.Background())
	if len(models) != 2 || models[0] != "glm-5.2" || models[1] != "glm-5.3" {
		t.Fatalf("models=%v", models)
	}
}

func TestTraeModelsEmptyWhenNoCredential(t *testing.T) {
	c := trae.New()
	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	if got := NewTraeModelsService(c, svc, 30).Available(context.Background()); len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

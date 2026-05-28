package store

import (
	"testing"

	"windsurf-tools-wails/backend/models"
)

func newTestProviderStore(t *testing.T) *ProviderAccountStore {
	t.Helper()
	s, err := NewProviderAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewProviderAccountStore: %v", err)
	}
	return s
}

func TestCandidatesForActive(t *testing.T) {
	s := newTestProviderStore(t)
	// 三张同 model 卡 + 一张不同 model + 一张禁用
	accs := []models.ProviderAccount{
		{ID: "b", Provider: "openai", BaseURL: "https://x/v1", AuthToken: "k1", ActiveModel: "gpt-4o", Status: "active", Activated: true},
		{ID: "a", Provider: "openai", BaseURL: "https://x/v1", AuthToken: "k2", ActiveModel: "gpt-4o", Status: "active"},
		{ID: "c", Provider: "openai", BaseURL: "https://x/v1", AuthToken: "k3", ActiveModel: "gpt-4o", Status: "disabled"},
		{ID: "d", Provider: "openai", BaseURL: "https://x/v1", AuthToken: "k4", ActiveModel: "o3", Status: "active"},
	}
	if errs := s.AddProviderBatch(accs); errs != nil {
		for _, e := range errs {
			if e != nil {
				t.Fatalf("AddProviderBatch: %v", e)
			}
		}
	}

	got := s.CandidatesForActive()
	// 期望:激活卡(b)排第一,其余同 model active 卡按 ID 升序(a);禁用(c)与异 model(d)排除
	if len(got) != 2 {
		t.Fatalf("候选数 = %d, want 2 (got %+v)", len(got), got)
	}
	if got[0].ID != "b" {
		t.Errorf("候选[0] = %q, want 激活卡 b", got[0].ID)
	}
	if got[1].ID != "a" {
		t.Errorf("候选[1] = %q, want a", got[1].ID)
	}
}

func TestCandidatesForActiveNoActive(t *testing.T) {
	s := newTestProviderStore(t)
	_ = s.AddProviderBatch([]models.ProviderAccount{
		{ID: "a", Provider: "openai", BaseURL: "https://x/v1", AuthToken: "k1", ActiveModel: "gpt-4o", Status: "active"},
	})
	if got := s.CandidatesForActive(); got != nil {
		t.Errorf("无激活卡时应返回 nil, got %+v", got)
	}
}

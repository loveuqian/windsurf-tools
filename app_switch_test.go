package main

import (
	"testing"
	"time"
	"windsurf-tools-wails/backend/models"
)

func TestPickNextSwitchableAccount_RespectsFilterAndSkipsExhausted(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Trial"},
		{ID: "trial-empty", Email: "trial-empty@example.com", Token: "tok-trial-empty", PlanName: "Trial", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "pro-ok", Email: "pro-ok@example.com", Token: "tok-pro-ok", PlanName: "Pro", DailyRemaining: "32.00%"},
		{ID: "trial-ok", Email: "trial-ok@example.com", Token: "tok-trial-ok", PlanName: "Trial", DailyRemaining: "88.00%"},
	}

	got, err := pickNextSwitchableAccount(accounts, "current", "trial", false)
	if err != nil {
		t.Fatalf("pickNextSwitchableAccount() error = %v", err)
	}
	if got.ID != "trial-ok" {
		t.Fatalf("pickNextSwitchableAccount() picked %q, want %q", got.ID, "trial-ok")
	}
}

func TestPickNextSwitchableAccount_SkipsInvalidCandidates(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Pro"},
		{ID: "api-key-only", Email: "api-key-only@example.com", WindsurfAPIKey: "sk-ws-1", PlanName: "Pro", DailyRemaining: "52.00%"},
		{ID: "disabled", Email: "disabled@example.com", Token: "tok-disabled", PlanName: "Pro", Status: "disabled"},
		{ID: "expired", Email: "expired@example.com", Token: "tok-expired", PlanName: "Pro", Status: "expired"},
		{ID: "ok", Email: "ok@example.com", Token: "tok-ok", PlanName: "Pro", DailyRemaining: "12.00%"},
	}

	got, err := pickNextSwitchableAccount(accounts, "current", "all", false)
	if err != nil {
		t.Fatalf("pickNextSwitchableAccount() error = %v", err)
	}
	if got.ID != "ok" {
		t.Fatalf("pickNextSwitchableAccount() picked %q, want %q", got.ID, "ok")
	}
}

func TestAccountEligibleForUsage(t *testing.T) {
	cases := []struct {
		name          string
		acc           models.Account
		requireAPIKey bool
		bypassQuota   bool
		want          bool
	}{
		{
			name: "api key candidate is allowed",
			acc:  models.Account{ID: "api", Email: "api@example.com", WindsurfAPIKey: "sk-ws-1", PlanName: "Pro", DailyRemaining: "88.00%"},
			want: true,
		},
		{
			name: "refresh token candidate is allowed",
			acc:  models.Account{ID: "refresh", Email: "refresh@example.com", RefreshToken: "rt-1", PlanName: "Pro", DailyRemaining: "33.00%"},
			want: true,
		},
		{
			name: "exhausted account is denied",
			acc:  models.Account{ID: "empty", Email: "empty@example.com", Token: "tok", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
			want: false,
		},
		{
			name:          "mitm requires api key",
			acc:           models.Account{ID: "jwt", Email: "jwt@example.com", Token: "tok", PlanName: "Pro", DailyRemaining: "50.00%"},
			requireAPIKey: true,
			want:          false,
		},
		{
			// SmartFriend(F7) bypass：服务端按 SMART_FRIEND 计费、绕过日/周限额，
			// 「显示已耗尽」的账号必须保留在号池里。
			name:        "exhausted account is allowed when bypassQuota",
			acc:         models.Account{ID: "empty", Email: "empty@example.com", Token: "tok", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
			bypassQuota: true,
			want:        true,
		},
		{
			name:        "disabled account remains denied even with bypassQuota",
			acc:         models.Account{ID: "d", Email: "d@example.com", Token: "tok", PlanName: "Pro", Status: "disabled", DailyRemaining: "50.00%"},
			bypassQuota: true,
			want:        false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountEligibleForUsage(&tc.acc, "all", tc.requireAPIKey, tc.bypassQuota); got != tc.want {
				t.Fatalf("accountEligibleForUsage() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollectEligibleMitmAPIKeysSkipsExhaustedAndDuplicates(t *testing.T) {
	accounts := []models.Account{
		{ID: "pro-ok", Email: "pro-ok@example.com", WindsurfAPIKey: "sk-ws-ok", PlanName: "Pro", DailyRemaining: "42.00%"},
		{ID: "pro-empty", Email: "pro-empty@example.com", WindsurfAPIKey: "sk-ws-empty", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "dup", Email: "dup@example.com", WindsurfAPIKey: "sk-ws-ok", PlanName: "Pro", DailyRemaining: "25.00%"},
		{ID: "trial", Email: "trial@example.com", WindsurfAPIKey: "sk-ws-trial", PlanName: "Trial", DailyRemaining: "50.00%"},
	}

	got := collectEligibleMitmAPIKeys(accounts, "pro", false)
	if len(got) != 1 {
		t.Fatalf("collectEligibleMitmAPIKeys() len = %d, want 1", len(got))
	}
	if got[0] != "sk-ws-ok" {
		t.Fatalf("collectEligibleMitmAPIKeys() first = %q, want %q", got[0], "sk-ws-ok")
	}

	// SmartFriend(F7) bypass：耗尽的 sk-ws-empty 也应保留进号池。
	gotBypass := collectEligibleMitmAPIKeys(accounts, "pro", true)
	wantBypass := map[string]bool{"sk-ws-ok": true, "sk-ws-empty": true}
	if len(gotBypass) != len(wantBypass) {
		t.Fatalf("collectEligibleMitmAPIKeys(bypass) = %v, want keys %v", gotBypass, wantBypass)
	}
	for _, k := range gotBypass {
		if !wantBypass[k] {
			t.Fatalf("collectEligibleMitmAPIKeys(bypass) contains unexpected key %q", k)
		}
	}
}

func TestPickNextMitmSwitchableAccount_RequiresAPIKey(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", WindsurfAPIKey: "sk-ws-current", PlanName: "Pro", DailyRemaining: "60.00%"},
		{ID: "token-only", Email: "token-only@example.com", Token: "tok-only", PlanName: "Pro", DailyRemaining: "88.00%"},
		{ID: "empty", Email: "empty@example.com", WindsurfAPIKey: "sk-ws-empty", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "next", Email: "next@example.com", WindsurfAPIKey: "sk-ws-next", PlanName: "Pro", DailyRemaining: "42.00%"},
	}

	got, err := pickNextMitmSwitchableAccount(accounts, "current", "pro", false)
	if err != nil {
		t.Fatalf("pickNextMitmSwitchableAccount() error = %v", err)
	}
	if got.ID != "next" {
		t.Fatalf("pickNextMitmSwitchableAccount() picked %q, want %q", got.ID, "next")
	}
}

func TestPickNextMitmSwitchableAccount_IncludesStaleQuotaCandidateAfterReset(t *testing.T) {
	pastReset := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	oldSync := time.Now().Add(-5 * time.Hour).Format(time.RFC3339)
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", WindsurfAPIKey: "sk-ws-current", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{
			ID:              "stale-reset",
			Email:           "stale-reset@example.com",
			WindsurfAPIKey:  "sk-ws-stale",
			PlanName:        "Pro",
			DailyRemaining:  "0.00%",
			WeeklyRemaining: "0.00%",
			DailyResetAt:    pastReset,
			LastQuotaUpdate: oldSync,
		},
	}

	got, err := pickNextMitmSwitchableAccount(accounts, "current", "pro", false)
	if err != nil {
		t.Fatalf("pickNextMitmSwitchableAccount() error = %v", err)
	}
	if got.ID != "stale-reset" {
		t.Fatalf("pickNextMitmSwitchableAccount() picked %q, want %q", got.ID, "stale-reset")
	}
}

func TestPickNextSwitchableAccount_ReturnsErrorWhenNoCandidateMatches(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Teams"},
		{ID: "free", Email: "free@example.com", Token: "tok-free", PlanName: "Free", DailyRemaining: "99.00%"},
	}

	if _, err := pickNextSwitchableAccount(accounts, "current", "trial", false); err == nil {
		t.Fatal("pickNextSwitchableAccount() expected error when nothing matches plan filter")
	}
}

// 当所有 Trial 账号都「显示耗尽」时，SmartFriend bypass 应能选出其中一个，
// 因为服务端按 SMART_FRIEND 计费、绕过日/周限额——这是「开 F7 后没有额度也可以切号过去」
// 的核心保障。
func TestPickNextMitmSwitchableAccount_BypassQuotaAllowsExhaustedCandidate(t *testing.T) {
	// LastQuotaUpdate 设近期时间，避免被 quotaDataIsStale 当作「过期可能已重置」纳入候选。
	now := time.Now().Format(time.RFC3339)
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", WindsurfAPIKey: "sk-ws-current", PlanName: "Pro", DailyRemaining: "60.00%", LastQuotaUpdate: now},
		{ID: "empty-1", Email: "empty-1@example.com", WindsurfAPIKey: "sk-ws-empty-1", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%", LastQuotaUpdate: now},
		{ID: "empty-2", Email: "empty-2@example.com", WindsurfAPIKey: "sk-ws-empty-2", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%", LastQuotaUpdate: now},
	}

	// 非 bypass：都耗尽且数据非过期 → 报错。
	if _, err := pickNextMitmSwitchableAccount(accounts, "current", "pro", false); err == nil {
		t.Fatal("pickNextMitmSwitchableAccount(bypass=false) expected error when all candidates exhausted")
	}

	// SmartFriend bypass：依然能选出耗尽账号。
	got, err := pickNextMitmSwitchableAccount(accounts, "current", "pro", true)
	if err != nil {
		t.Fatalf("pickNextMitmSwitchableAccount(bypass=true) error = %v", err)
	}
	if got.ID != "empty-1" && got.ID != "empty-2" {
		t.Fatalf("pickNextMitmSwitchableAccount(bypass=true) picked %q, want one of empty-1/empty-2", got.ID)
	}
}

package main

import (
	"testing"

	"windsurf-tools-wails/backend/models"
)

func TestDedupNonEmpty(t *testing.T) {
	in := []string{"a", "", "b", "a", "c", "  ", "b"}
	got := dedupNonEmpty(in)
	want := []string{"a", "b", "c", "  "} // 注意 dedupNonEmpty 用 == "" 判空，不 trim
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d, got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestStringSliceEqual(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{}, nil, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, false},
		{[]string{"a"}, []string{"a", "b"}, false},
	}
	for i, c := range cases {
		if got := stringSliceEqual(c.a, c.b); got != c.want {
			t.Errorf("[%d] stringSliceEqual(%v, %v) = %v, want %v", i, c.a, c.b, got, c.want)
		}
	}
}

func TestRotationPoolMemberUsable(t *testing.T) {
	cases := []struct {
		name        string
		acc         models.Account
		bypassQuota bool
		want        bool
	}{
		{
			name: "has key + active + quota OK",
			acc:  models.Account{Email: "a", WindsurfAPIKey: "sk-ws-a", Status: "active", DailyRemaining: "60.00%"},
			want: true,
		},
		{
			name: "no credentials",
			acc:  models.Account{Email: "b"},
			want: false,
		},
		{
			name: "disabled status",
			acc:  models.Account{Email: "c", WindsurfAPIKey: "sk-ws-c", Status: "disabled", DailyRemaining: "60.00%"},
			want: false,
		},
		{
			name: "expired status",
			acc:  models.Account{Email: "d", WindsurfAPIKey: "sk-ws-d", Status: "expired", DailyRemaining: "60.00%"},
			want: false,
		},
		{
			name: "exhausted account is denied",
			acc:  models.Account{Email: "e", WindsurfAPIKey: "sk-ws-e", Status: "active", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
			want: false,
		},
		{
			// SmartFriend(F7) bypass：服务端按 SMART_FRIEND 计费、绕过日/周限额，
			// 轮换池中「显示耗尽」的账号仍应被选中。
			name:        "exhausted account allowed when bypassQuota",
			acc:         models.Account{Email: "e", WindsurfAPIKey: "sk-ws-e", Status: "active", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
			bypassQuota: true,
			want:        true,
		},
		{
			name:        "disabled remains denied even with bypassQuota",
			acc:         models.Account{Email: "f", WindsurfAPIKey: "sk-ws-f", Status: "disabled", DailyRemaining: "60.00%"},
			bypassQuota: true,
			want:        false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rotationPoolMemberUsable(&c.acc, c.bypassQuota); got != c.want {
				t.Errorf("got %v, want %v (account=%+v)", got, c.want, c.acc)
			}
		})
	}
}

func TestPickNextRotationPoolMember_RotatesThroughPool(t *testing.T) {
	all := []models.Account{
		{ID: "a", Email: "a@x.com", WindsurfAPIKey: "sk-a", Status: "active", DailyRemaining: "80%"},
		{ID: "b", Email: "b@x.com", WindsurfAPIKey: "sk-b", Status: "active", DailyRemaining: "70%"},
		{ID: "c", Email: "c@x.com", WindsurfAPIKey: "sk-c", Status: "active", DailyRemaining: "90%"},
	}
	members := []string{"a", "b", "c"}

	// 当前 a → 下一个 b
	next := pickNextRotationPoolMember(all, members, "a", false)
	if next == nil || next.ID != "b" {
		t.Errorf("from a, next should be b, got %v", next)
	}
	// 当前 b → 下一个 c
	next = pickNextRotationPoolMember(all, members, "b", false)
	if next == nil || next.ID != "c" {
		t.Errorf("from b, next should be c, got %v", next)
	}
	// 当前 c → 绕回 a
	next = pickNextRotationPoolMember(all, members, "c", false)
	if next == nil || next.ID != "a" {
		t.Errorf("from c, next should be a (wrap), got %v", next)
	}
	// 当前不在池内 → 取第一个可用 (a)
	next = pickNextRotationPoolMember(all, members, "outside", false)
	if next == nil || next.ID != "a" {
		t.Errorf("from outside, next should be a, got %v", next)
	}
}

func TestPickNextRotationPoolMember_SkipsUnusable(t *testing.T) {
	all := []models.Account{
		{ID: "a", Email: "a", WindsurfAPIKey: "sk-a", Status: "active", DailyRemaining: "80%"},
		{ID: "b", Email: "b", Status: "disabled"}, // 不可用
		{ID: "c", Email: "c", WindsurfAPIKey: "sk-c", Status: "active", DailyRemaining: "90%"},
	}
	members := []string{"a", "b", "c"}
	// 当前 a → 下一个 b 不可用 → c
	next := pickNextRotationPoolMember(all, members, "a", false)
	if next == nil || next.ID != "c" {
		t.Errorf("from a (b skipped), next should be c, got %v", next)
	}
}

// SmartFriend bypass：轮换池里全部「额度耗尽」的账号，F7 开启时仍应能轮转。
func TestPickNextRotationPoolMember_BypassQuotaAllowsExhausted(t *testing.T) {
	all := []models.Account{
		{ID: "a", Email: "a", WindsurfAPIKey: "sk-a", Status: "active", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "b", Email: "b", WindsurfAPIKey: "sk-b", Status: "active", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "c", Email: "c", WindsurfAPIKey: "sk-c", Status: "active", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
	}
	members := []string{"a", "b", "c"}

	// 非 bypass：都耗尽 → nil
	if got := pickNextRotationPoolMember(all, members, "a", false); got != nil {
		t.Errorf("bypass=false, all exhausted: got %v, want nil", got)
	}

	// SmartFriend bypass=true：从 a 转下一个 → b
	next := pickNextRotationPoolMember(all, members, "a", true)
	if next == nil || next.ID != "b" {
		t.Errorf("bypass=true, from a should pick b, got %v", next)
	}
}

func TestPickNextRotationPoolMember_AllUnusableReturnsNil(t *testing.T) {
	all := []models.Account{
		{ID: "a", Status: "disabled"},
		{ID: "b", Status: "expired"},
	}
	members := []string{"a", "b"}
	if got := pickNextRotationPoolMember(all, members, "a", false); got != nil {
		t.Errorf("all unusable should return nil, got %v", got)
	}
}

func TestPickNextRotationPoolMember_MissingFromAllSkipped(t *testing.T) {
	all := []models.Account{
		{ID: "a", Email: "a", WindsurfAPIKey: "sk-a", Status: "active", DailyRemaining: "80%"},
	}
	members := []string{"a", "ghost", "missing"}
	next := pickNextRotationPoolMember(all, members, "a", false)
	// 池里指向不存在账号会被 skip，绕回 a 本身但 a == currentID 被排除 → nil
	if next != nil {
		t.Errorf("from a with no other valid members, should return nil, got %v", next)
	}
}

func TestIntersectByID(t *testing.T) {
	accounts := []models.Account{
		{ID: "a", Email: "a"},
		{ID: "b", Email: "b"},
		{ID: "c", Email: "c"},
	}

	// 部分交集
	got := intersectByID(accounts, []string{"b", "c"})
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "c" {
		t.Errorf("intersect by [b,c]: %+v", got)
	}

	// 空 allowed → 空结果（这是 RotationPool 关闭时的语义）
	got = intersectByID(accounts, nil)
	if got != nil {
		t.Errorf("nil allowed should return nil, got %v", got)
	}

	// allowed 含不存在 ID → 静默 skip
	got = intersectByID(accounts, []string{"a", "ghost"})
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("intersect with ghost: %+v", got)
	}

	// 顺序按 source 保留（不是 allowed 的顺序）
	got = intersectByID(accounts, []string{"c", "a"})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("order should follow source, not allowed: %+v", got)
	}
}

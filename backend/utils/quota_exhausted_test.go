package utils

import (
	"testing"

	"windsurf-tools-wails/backend/models"
)

func TestAccountQuotaExhausted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		acc  models.Account
		want bool
	}{
		{"nil guard", models.Account{}, false},
		{"monthly cap", models.Account{TotalQuota: 100, UsedQuota: 100}, true},
		{"monthly not full", models.Account{TotalQuota: 100, UsedQuota: 99}, false},
		{"daily zero only", models.Account{DailyRemaining: "0.00%"}, true},
		{"daily partial", models.Account{DailyRemaining: "12.00%"}, false},
		{"weekly zero only", models.Account{DailyRemaining: "100.00%", WeeklyRemaining: "0.00%"}, true},
		{"both zero", models.Account{DailyRemaining: "0%", WeeklyRemaining: "0.00%"}, true},
		{"daily zero weekly ok", models.Account{DailyRemaining: "0%", WeeklyRemaining: "50%"}, true},
		{"weekly missing with reset blocks usage", models.Account{DailyRemaining: "100.00%", WeeklyRemaining: "", WeeklyResetAt: "2026-03-29T08:00:00Z"}, true},
		{"weekly missing without reset stays unknown", models.Account{DailyRemaining: "100.00%", WeeklyRemaining: ""}, false},
		// Extra usage 兜底:包含额度见底但有正余额 → 仍可用
		{"weekly zero but positive extra balance", models.Account{DailyRemaining: "100%", WeeklyRemaining: "0.00%", HasExtraUsageBalance: true, ExtraUsageBalanceMicros: 5000000}, false},
		{"weekly missing but positive extra balance", models.Account{DailyRemaining: "100%", WeeklyRemaining: "", WeeklyResetAt: "2026-03-29T08:00:00Z", HasExtraUsageBalance: true, ExtraUsageBalanceMicros: 1}, false},
		// 负余额/欠费不兜底(对应截图 $-0.79)
		{"weekly zero with negative extra balance", models.Account{DailyRemaining: "100%", WeeklyRemaining: "0.00%", HasExtraUsageBalance: true, ExtraUsageBalanceMicros: -787965}, true},
		{"weekly zero with zero extra balance", models.Account{DailyRemaining: "100%", WeeklyRemaining: "0.00%", HasExtraUsageBalance: true, ExtraUsageBalanceMicros: 0}, true},
		// extra 余额正,但包含额度还没见底 → 不该因 extra 影响判定(本就 false)
		{"included not exhausted ignores extra", models.Account{DailyRemaining: "50%", WeeklyRemaining: "50%", HasExtraUsageBalance: true, ExtraUsageBalanceMicros: 9000000}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			acc := tc.acc
			if got := AccountQuotaExhausted(&acc); got != tc.want {
				t.Fatalf("AccountQuotaExhausted = %v, want %v", got, tc.want)
			}
		})
	}
}

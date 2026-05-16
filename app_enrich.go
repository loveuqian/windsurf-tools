package main

import (
	"log"
	"strings"
	"windsurf-tools-wails/backend/app/accountmeta"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 辅助：账号信息 enrich
//
// 纯账号元信息推导（normalize / parseEnd / choosePreferredExpiry 等）已迁到
// backend/app/accountmeta；本文件只保留依赖 *App 的实例方法（grpc 调用 / 联表 enrich）。
// 旧名通过下方 var alias 暴露给本文件其他位置 + 邻近 app_*.go 复用，调用点零改动。
// ═══════════════════════════════════════

var (
	applyJWTClaims                         = accountmeta.ApplyJWTClaims
	applyAccountProfile                    = accountmeta.ApplyProfile
	applyAccessErrorStatus                 = accountmeta.ApplyAccessErrorStatus
	choosePreferredSubscriptionExpiry      = accountmeta.ChoosePreferredExpiry
	normalizeAccountPlanAndStatus          = accountmeta.NormalizeAccount
	derivePlanNameFromClaims               = accountmeta.DerivePlanNameFromClaims
	parseSubscriptionEndTime               = accountmeta.ParseSubscriptionEnd
	subscriptionEndBeforeAccountCreated    = accountmeta.SubscriptionEndBeforeAccountCreated
	subscriptionEndLooksLikeStalePlanStart = accountmeta.SubscriptionEndLooksLikeStalePlanStart
	manualSubscriptionExpiryHint           = accountmeta.ManualExpiryHint
	parseManualSubscriptionExpiryHint      = accountmeta.ParseManualExpiryHint
	looksLikeDatePrefix                    = accountmeta.LooksLikeDatePrefix
	formatQuotaPercent                     = accountmeta.FormatQuotaPercent
)

// enrichAccountQuotaOnly 热轮询额度用尽检测：只更新 JWT 解析 + 额度相关 profile，不做 RegisterUser / GetAccountInfo。
// 返回 true 表示至少获取到了部分有效额度数据。
func (a *App) enrichAccountQuotaOnly(acc *models.Account) bool {
	return a.enrichAccountQuotaOnlyWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountQuotaOnlyWithService(svc *services.WindsurfService, acc *models.Account) bool {
	if acc == nil || svc == nil {
		return false
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	gotData := false
	utils.DLog("[enrich] %s 开始 (hasToken=%v hasKey=%v plan=%s)", label, acc.Token != "", acc.WindsurfAPIKey != "", acc.PlanName)
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
	}
	// ── 主路径: gRPC GetUserStatus（快速、不依赖 Firebase / 代理）──
	needJSONFallback := false
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			utils.DLog("[enrich] %s GetUserStatus OK: plan=%s daily=%v weekly=%v total=%d used=%d",
				label, profile.PlanName,
				profile.DailyQuotaRemaining, profile.WeeklyQuotaRemaining,
				profile.TotalCredits, profile.UsedCredits)
			applyAccountProfile(acc, profile)
			gotData = true
			// gRPC 拿不到百分比（Pro/Teams 某些号）→ 标记需要 JSON 兜底
			if profile.DailyQuotaRemaining == nil && profile.WeeklyQuotaRemaining == nil {
				needJSONFallback = true
			}
		} else {
			utils.DLog("[enrich] %s GetUserStatus 失败: %v", label, err)
			log.Printf("[enrich] %s GetUserStatus 失败: %v", label, err)
			applyAccessErrorStatus(acc, err)
			needJSONFallback = true // gRPC 完全失败也尝试 JSON
		}
	} else {
		needJSONFallback = true // 无 API key，只能走 JSON
	}

	// ── 兜底: Firebase token → JSON API（需要代理才能访问 Firebase）──
	// GetJWTByAPIKey 返回的 Windsurf JWT 被 JSON API 拒绝(401)，必须用 Firebase ID token。
	if needJSONFallback {
		firebaseToken := ""
		if acc.RefreshToken != "" {
			if resp, err := svc.RefreshToken(acc.RefreshToken); err == nil {
				firebaseToken = resp.IDToken
				acc.RefreshToken = resp.RefreshToken // 更新 refresh token
				utils.DLog("[enrich] %s RefreshToken→Firebase OK", label)
			} else {
				utils.DLog("[enrich] %s RefreshToken 失败: %v", label, err)
			}
		}
		if firebaseToken == "" && acc.Email != "" && acc.Password != "" {
			if resp, err := svc.LoginWithEmail(acc.Email, acc.Password); err == nil {
				firebaseToken = resp.IDToken
				if resp.RefreshToken != "" {
					acc.RefreshToken = resp.RefreshToken
				}
				utils.DLog("[enrich] %s Login→Firebase OK", label)
			} else {
				utils.DLog("[enrich] %s Login 失败: %v", label, err)
			}
		}
		if firebaseToken != "" {
			if plan, err := svc.GetPlanStatusJSON(firebaseToken); err == nil {
				utils.DLog("[enrich] %s GetPlanStatusJSON OK: plan=%s daily=%v weekly=%v total=%d used=%d remaining=%d",
					label, plan.PlanName,
					plan.DailyQuotaRemaining, plan.WeeklyQuotaRemaining,
					plan.TotalCredits, plan.UsedCredits, plan.RemainingCredits)
				applyAccountProfile(acc, plan)
				gotData = true
			} else {
				utils.DLog("[enrich] %s GetPlanStatusJSON 失败: %v", label, err)
			}
		}
	}
	utils.DLog("[enrich] %s 结果: gotData=%v plan=%s daily=%s weekly=%s totalQ=%d usedQ=%d",
		label, gotData, acc.PlanName, acc.DailyRemaining, acc.WeeklyRemaining, acc.TotalQuota, acc.UsedQuota)
	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}
	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
	return gotData
}

// enrichAccountInfoLite 批量导入时使用：只做本地 JWT 解析，避免 RegisterUser / GetPlan / GetUserStatus 等串行请求拖死界面。
func (a *App) enrichAccountInfoLite(acc *models.Account) {
	a.enrichAccountInfoLiteWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoLiteWithService(svc *services.WindsurfService, acc *models.Account) {
	if acc == nil || svc == nil {
		return
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
	}
	// lite 只走 gRPC（快速），不走 Firebase→JSON（需代理、耗时）
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			applyAccountProfile(acc, profile)
		} else {
			log.Printf("[enrich-lite] %s GetUserStatus 失败: %v", label, err)
		}
	}
	if acc.Nickname == "" && acc.Email != "" {
		if at := strings.Index(acc.Email, "@"); at > 0 {
			acc.Nickname = acc.Email[:at]
		}
	}
	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
}

func (a *App) enrichAccountInfo(acc *models.Account) bool {
	return a.enrichAccountInfoWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoWithService(svc *services.WindsurfService, acc *models.Account) bool {
	if acc == nil || svc == nil {
		return false
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	gotData := false
	utils.DLog("[enrichFull] %s 开始 (hasToken=%v hasKey=%v hasRefresh=%v hasPass=%v plan=%s)",
		label, acc.Token != "", acc.WindsurfAPIKey != "", acc.RefreshToken != "", acc.Password != "", acc.PlanName)

	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
			utils.DLog("[enrichFull] %s JWT解码: email=%s plan=%s pro=%v tier=%s", label, claims.Email, acc.PlanName, claims.Pro, claims.TeamsTier)
		} else {
			utils.DLog("[enrichFull] %s JWT解码失败: %v", label, err)
		}
	}

	if acc.Token != "" && (acc.RefreshToken != "" || acc.Password != "") {
		if acc.Email == "" {
			if email, err := svc.GetAccountInfo(acc.Token); err == nil && email != "" {
				acc.Email = email
				utils.DLog("[enrichFull] %s GetAccountInfo: email=%s", label, email)
			}
		}
		if strings.TrimSpace(acc.WindsurfAPIKey) == "" {
			if reg, err := svc.RegisterUser(acc.Token); err == nil && reg != nil && reg.APIKey != "" {
				acc.WindsurfAPIKey = reg.APIKey
				utils.DLog("[enrichFull] %s RegisterUser: 获得APIKey=%s...", label, reg.APIKey[:min(12, len(reg.APIKey))])
			} else if err != nil {
				utils.DLog("[enrichFull] %s RegisterUser 失败: %v", label, err)
				maybeBackfillAuth1SessionKey(svc, acc, label)
			}
		}
	}

	// ── 主路径: gRPC GetUserStatus（快速、不依赖 Firebase / 代理）──
	needJSONFallback := false
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			utils.DLog("[enrichFull] %s GetUserStatus OK: plan=%s daily=%v weekly=%v total=%d used=%d",
				label, profile.PlanName, profile.DailyQuotaRemaining, profile.WeeklyQuotaRemaining, profile.TotalCredits, profile.UsedCredits)
			applyAccountProfile(acc, profile)
			gotData = true
			if profile.DailyQuotaRemaining == nil && profile.WeeklyQuotaRemaining == nil {
				needJSONFallback = true
			}
		} else {
			utils.DLog("[enrichFull] %s GetUserStatus 失败: %v", label, err)
			log.Printf("[enrich] %s GetUserStatus 失败: %v", label, err)
			applyAccessErrorStatus(acc, err)
			needJSONFallback = true
		}
	} else {
		needJSONFallback = true
	}

	// ── 兜底: Firebase token → JSON API ──
	if needJSONFallback {
		firebaseToken := ""
		if acc.RefreshToken != "" {
			if resp, err := svc.RefreshToken(acc.RefreshToken); err == nil {
				firebaseToken = resp.IDToken
				acc.RefreshToken = resp.RefreshToken
				utils.DLog("[enrichFull] %s RefreshToken→Firebase OK", label)
			} else {
				utils.DLog("[enrichFull] %s RefreshToken 失败: %v", label, err)
			}
		}
		if firebaseToken == "" && acc.Email != "" && acc.Password != "" {
			if resp, err := svc.LoginWithEmail(acc.Email, acc.Password); err == nil {
				firebaseToken = resp.IDToken
				if resp.RefreshToken != "" {
					acc.RefreshToken = resp.RefreshToken
				}
				utils.DLog("[enrichFull] %s Login→Firebase OK", label)
			} else {
				utils.DLog("[enrichFull] %s Login 失败: %v", label, err)
			}
		}
		if firebaseToken != "" {
			if plan, err := svc.GetPlanStatusJSON(firebaseToken); err == nil {
				utils.DLog("[enrichFull] %s GetPlanStatusJSON OK: plan=%s daily=%v weekly=%v total=%d used=%d",
					label, plan.PlanName, plan.DailyQuotaRemaining, plan.WeeklyQuotaRemaining, plan.TotalCredits, plan.UsedCredits)
				applyAccountProfile(acc, plan)
				gotData = true
			} else {
				utils.DLog("[enrichFull] %s GetPlanStatusJSON 失败: %v", label, err)
			}
		}
	}
	utils.DLog("[enrichFull] %s 结果: gotData=%v plan=%s daily=%s weekly=%s totalQ=%d usedQ=%d",
		label, gotData, acc.PlanName, acc.DailyRemaining, acc.WeeklyRemaining, acc.TotalQuota, acc.UsedQuota)

	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}

	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
	return gotData
}

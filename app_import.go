package main

// app_import.go ── 薄壳。真正实现已迁到 backend/app/importsvc。
//   - main 包保留 6 个 wails binding 类型（ImportResult / EmailPasswordItem
//     / TokenItem / APIKeyItem / JWTItem / EmailAPIKeyItem）和 5 个 Import*
//     方法 + AddSingleAccount 的导出面，签名/JSON tag 完全不变。
//   - 内部转换：把 main 类型字段拷贝成 importsvc 的同字段类型，调子包，
//     再把结果反向拷贝回 main 类型。这样 wails binding 路径
//     (main.ImportResult / main.APIKeyItem...) 100% 保持不变。

import "windsurf-tools-wails/backend/app/importsvc"

// ── Wails binding 暴露的入参 / 出参类型 ──

type ImportResult struct {
	Email   string `json:"email"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type EmailPasswordItem struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	AltPassword string `json:"alt_password,omitempty"`
	Remark      string `json:"remark"`
}

type TokenItem struct {
	Token  string `json:"token"`
	Remark string `json:"remark"`
}

type APIKeyItem struct {
	APIKey string `json:"api_key"`
	Remark string `json:"remark"`
}

type JWTItem struct {
	JWT    string `json:"jwt"`
	Remark string `json:"remark"`
}

type EmailAPIKeyItem struct {
	Email  string `json:"email"`
	APIKey string `json:"api_key"`
	Remark string `json:"remark"`
}

// ── 类型字段拷贝助手（main ↔ importsvc）──

func subResultsToMain(in []importsvc.Result) []ImportResult {
	out := make([]ImportResult, len(in))
	for i, r := range in {
		out[i] = ImportResult{Email: r.Email, Success: r.Success, Error: r.Error}
	}
	return out
}

func toSubEmailPassword(in []EmailPasswordItem) []importsvc.EmailPasswordItem {
	out := make([]importsvc.EmailPasswordItem, len(in))
	for i, it := range in {
		out[i] = importsvc.EmailPasswordItem{
			Email: it.Email, Password: it.Password, AltPassword: it.AltPassword, Remark: it.Remark,
		}
	}
	return out
}

func toSubToken(in []TokenItem) []importsvc.TokenItem {
	out := make([]importsvc.TokenItem, len(in))
	for i, it := range in {
		out[i] = importsvc.TokenItem{Token: it.Token, Remark: it.Remark}
	}
	return out
}

func toSubAPIKey(in []APIKeyItem) []importsvc.APIKeyItem {
	out := make([]importsvc.APIKeyItem, len(in))
	for i, it := range in {
		out[i] = importsvc.APIKeyItem{APIKey: it.APIKey, Remark: it.Remark}
	}
	return out
}

func toSubJWT(in []JWTItem) []importsvc.JWTItem {
	out := make([]importsvc.JWTItem, len(in))
	for i, it := range in {
		out[i] = importsvc.JWTItem{JWT: it.JWT, Remark: it.Remark}
	}
	return out
}

func toSubEmailAPIKey(in []EmailAPIKeyItem) []importsvc.EmailAPIKeyItem {
	out := make([]importsvc.EmailAPIKeyItem, len(in))
	for i, it := range in {
		out[i] = importsvc.EmailAPIKeyItem{Email: it.Email, APIKey: it.APIKey, Remark: it.Remark}
	}
	return out
}

// ── Wails 暴露方法（薄壳 → 委托给 a.importMod）──

func (a *App) ImportByEmailPassword(items []EmailPasswordItem) []ImportResult {
	if a == nil || a.importMod == nil {
		return make([]ImportResult, len(items))
	}
	return subResultsToMain(a.importMod.ByEmailPassword(toSubEmailPassword(items)))
}

func (a *App) ImportByRefreshToken(items []TokenItem) []ImportResult {
	if a == nil || a.importMod == nil {
		return make([]ImportResult, len(items))
	}
	return subResultsToMain(a.importMod.ByRefreshToken(toSubToken(items)))
}

func (a *App) ImportByAPIKey(items []APIKeyItem) []ImportResult {
	if a == nil || a.importMod == nil {
		return make([]ImportResult, len(items))
	}
	return subResultsToMain(a.importMod.ByAPIKey(toSubAPIKey(items)))
}

func (a *App) ImportByJWT(items []JWTItem) []ImportResult {
	if a == nil || a.importMod == nil {
		return make([]ImportResult, len(items))
	}
	return subResultsToMain(a.importMod.ByJWT(toSubJWT(items)))
}

func (a *App) ImportByEmailAPIKey(items []EmailAPIKeyItem) []ImportResult {
	if a == nil || a.importMod == nil {
		return make([]ImportResult, len(items))
	}
	return subResultsToMain(a.importMod.ByEmailAPIKey(toSubEmailAPIKey(items)))
}

// AddSingleAccount 单个添加；mode = api_key / jwt / refresh_token / password。
func (a *App) AddSingleAccount(mode string, value string, remark string) ImportResult {
	if a == nil || a.importMod == nil {
		return ImportResult{Email: "?", Success: false, Error: "应用未初始化"}
	}
	r := a.importMod.AddSingle(mode, value, remark)
	return ImportResult{Email: r.Email, Success: r.Success, Error: r.Error}
}

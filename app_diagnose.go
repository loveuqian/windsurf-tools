package main

// app_diagnose.go ── 薄壳。真正实现已迁到 backend/app/diagnose。
//   - DiagnoseReport / DiagnoseCheck 类型字段保留在 main 包，确保 wails
//     binding 路径 `main.DiagnoseReport` 不动。
//   - App.RunDiagnostics 调子包 Run 拿 Report，再字段拷贝回 main 类型。

import "windsurf-tools-wails/backend/app/diagnose"

// DiagnoseCheck 单项诊断结果。字段必须与 wails binding main.DiagnoseCheck 一致。
type DiagnoseCheck struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
	FixHint string `json:"fix_hint"`
}

// DiagnoseReport 完整诊断报告。字段必须与 wails binding main.DiagnoseReport 一致。
type DiagnoseReport struct {
	Platform string          `json:"platform"`
	Arch     string          `json:"arch"`
	OK       int             `json:"ok"`
	Warn     int             `json:"warn"`
	Error    int             `json:"error"`
	Checks   []DiagnoseCheck `json:"checks"`
}

// RunDiagnostics 跑全套诊断，返回报告。
//
// 实现细节：
//   - 数据来源（DataDir / Clash 配置）通过 Deps 注入子包；
//   - 子包返回 diagnose.Report，本 wrapper 字段拷贝成 main.DiagnoseReport。
func (a *App) RunDiagnostics() DiagnoseReport {
	deps := diagnose.Deps{}
	if a != nil && a.store != nil {
		deps.DataDir = a.store.DataDir()
		s := a.store.GetSettings()
		deps.ClashControllerURL = s.ClashControllerURL
		deps.ClashSecret = s.ClashSecret
	}
	r := diagnose.Run(deps)
	checks := make([]DiagnoseCheck, len(r.Checks))
	for i, c := range r.Checks {
		checks[i] = DiagnoseCheck{
			ID:      c.ID,
			Title:   c.Title,
			Status:  c.Status,
			Detail:  c.Detail,
			FixHint: c.FixHint,
		}
	}
	return DiagnoseReport{
		Platform: r.Platform,
		Arch:     r.Arch,
		OK:       r.OK,
		Warn:     r.Warn,
		Error:    r.Error,
		Checks:   checks,
	}
}

package main

import (
	"runtime"
	"testing"

	"windsurf-tools-wails/backend/store"
)

func TestRunDiagnostics_ReportPopulated(t *testing.T) {
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths error: %v", err)
	}
	app := &App{store: s}
	report := app.RunDiagnostics()

	if report.Platform != runtime.GOOS {
		t.Errorf("Platform = %q, want %q", report.Platform, runtime.GOOS)
	}
	if report.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", report.Arch, runtime.GOARCH)
	}
	if len(report.Checks) == 0 {
		t.Fatal("Checks 应至少包含基础项")
	}
	// 计数总和应等于 checks 长度
	total := report.OK + report.Warn + report.Error
	// n/a 不计入；允许 total < len 因为有些项可能 n/a，但至少 OK 要 > 0
	if total < 1 {
		t.Errorf("应至少有 1 个 ok/warn/error，实际 %d", total)
	}
	if report.OK == 0 {
		t.Error("应至少 1 个 ok（macOS osascript / Windows powershell / Linux notify-send 通常存在）")
	}
}

func TestRunDiagnostics_AllCheckIDsUnique(t *testing.T) {
	s, _ := store.NewStoreInPaths(t.TempDir())
	app := &App{store: s}
	report := app.RunDiagnostics()
	seen := map[string]bool{}
	for _, c := range report.Checks {
		if c.ID == "" {
			t.Error("空 ID 不允许")
		}
		if seen[c.ID] {
			t.Errorf("重复 ID: %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestRunDiagnostics_StatusEnumeration(t *testing.T) {
	s, _ := store.NewStoreInPaths(t.TempDir())
	app := &App{store: s}
	report := app.RunDiagnostics()
	allowed := map[string]bool{"ok": true, "warn": true, "error": true, "n/a": true}
	for _, c := range report.Checks {
		if !allowed[c.Status] {
			t.Errorf("Check %q 状态 %q 非法（应 ok/warn/error/n/a）", c.ID, c.Status)
		}
	}
}

func TestIntToStr(t *testing.T) {
	cases := map[int]string{
		0: "0", 1: "1", 9: "9", 10: "10", 100: "100",
		42: "42", -7: "-7", -100: "-100",
	}
	for n, want := range cases {
		if got := intToStr(n); got != want {
			t.Errorf("intToStr(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestWindsurfCandidatesByOS_NotEmpty(t *testing.T) {
	cands := windsurfCandidatesByOS()
	if len(cands) == 0 {
		t.Error("应返回至少 1 个候选路径（即使 OS 不识别）")
	}
}

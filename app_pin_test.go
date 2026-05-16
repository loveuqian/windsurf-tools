package main

import (
	"testing"

	"windsurf-tools-wails/backend/app/pin"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/store"
)

func newPinTestApp(t *testing.T) *App {
	t.Helper()
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	app := &App{store: s}
	app.pinMod = pin.New(app.store, nil)
	return app
}

func TestSetManualPin_PersistsToSettings(t *testing.T) {
	app := newPinTestApp(t)
	ok := app.setManualPin("acc-123")
	if !ok {
		t.Fatal("setManualPin returned false")
	}
	s := app.store.GetSettings()
	if !s.ManualPinEnabled {
		t.Error("ManualPinEnabled should be true after setManualPin")
	}
	if s.ManualPinAccountID != "acc-123" {
		t.Errorf("ManualPinAccountID = %q, want %q", s.ManualPinAccountID, "acc-123")
	}
}

func TestSetManualPin_IsIdempotent(t *testing.T) {
	app := newPinTestApp(t)
	app.setManualPin("acc-X")
	ok := app.setManualPin("acc-X")
	if !ok {
		t.Error("repeated setManualPin should return true (no-op)")
	}
	s := app.store.GetSettings()
	if s.ManualPinAccountID != "acc-X" {
		t.Errorf("idempotent setManualPin changed ID: %q", s.ManualPinAccountID)
	}
}

func TestSetManualPin_OverwritesPreviousPin(t *testing.T) {
	app := newPinTestApp(t)
	app.setManualPin("acc-A")
	app.setManualPin("acc-B")
	s := app.store.GetSettings()
	if s.ManualPinAccountID != "acc-B" {
		t.Errorf("pin not overwritten: ID=%q, want acc-B", s.ManualPinAccountID)
	}
}

func TestSetManualPin_RejectsEmptyID(t *testing.T) {
	app := newPinTestApp(t)
	if app.setManualPin("") {
		t.Error("setManualPin(\"\") should return false")
	}
	if app.setManualPin("   ") {
		t.Error("setManualPin(whitespace) should return false")
	}
}

func TestUnpinManualAccount_ClearsState(t *testing.T) {
	app := newPinTestApp(t)
	app.setManualPin("acc-1")
	if err := app.UnpinManualAccount(); err != nil {
		t.Fatalf("UnpinManualAccount error: %v", err)
	}
	s := app.store.GetSettings()
	if s.ManualPinEnabled {
		t.Error("ManualPinEnabled should be false after unpin")
	}
	if s.ManualPinAccountID != "" {
		t.Errorf("ManualPinAccountID should be cleared, got %q", s.ManualPinAccountID)
	}
}

func TestUnpinManualAccount_IdempotentWhenNotPinned(t *testing.T) {
	app := newPinTestApp(t)
	// 从未 pin，调 unpin 不应报错
	if err := app.UnpinManualAccount(); err != nil {
		t.Errorf("unpin without prior pin should succeed, got: %v", err)
	}
}

func TestIsManuallyPinned(t *testing.T) {
	app := newPinTestApp(t)
	if app.isManuallyPinned() {
		t.Error("fresh app should not be pinned")
	}
	app.setManualPin("acc-1")
	if !app.isManuallyPinned() {
		t.Error("isManuallyPinned should return true after setManualPin")
	}
	_ = app.UnpinManualAccount()
	if app.isManuallyPinned() {
		t.Error("isManuallyPinned should return false after unpin")
	}
}

func TestGetManualPinStatus_EnrichesEmailNickname(t *testing.T) {
	app := newPinTestApp(t)
	_ = app.store.AddAccount(models.Account{
		ID:       "acc-99",
		Email:    "pinned@example.com",
		Nickname: "Pinned User",
	})
	app.setManualPin("acc-99")
	st := app.GetManualPinStatus()
	if !st.Enabled || st.AccountID != "acc-99" {
		t.Errorf("status not set: %+v", st)
	}
	if st.Email != "pinned@example.com" {
		t.Errorf("Email should be enriched: %q", st.Email)
	}
	if st.Nickname != "Pinned User" {
		t.Errorf("Nickname should be enriched: %q", st.Nickname)
	}
}

func TestGetManualPinStatus_DisabledByDefault(t *testing.T) {
	app := newPinTestApp(t)
	st := app.GetManualPinStatus()
	if st.Enabled {
		t.Error("fresh app should report Enabled=false")
	}
}

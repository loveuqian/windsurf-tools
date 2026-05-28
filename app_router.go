package main

// app_router.go ── App 直接实现 services.Router 接口。

import (
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
)

func (a *App) RouteMode() string {
	if a == nil || a.store == nil {
		return ""
	}
	return a.store.GetSettings().MitmRouteMode
}

func (a *App) ActiveAccount() (models.ProviderAccount, bool) {
	if a == nil || a.providerStore == nil {
		return models.ProviderAccount{}, false
	}
	return a.providerStore.GetActivated()
}

func (a *App) Candidates() []models.ProviderAccount {
	if a == nil || a.providerStore == nil {
		return nil
	}
	return a.providerStore.CandidatesForActive()
}

var _ services.Router = (*App)(nil)

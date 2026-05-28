package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"windsurf-tools-wails/backend/models"
)

func TestFrontendSettingsModelCoversBackendSettingsFields(t *testing.T) {
	data, err := os.ReadFile("frontend/src/utils/settingsModel.ts")
	if err != nil {
		t.Fatalf("read settingsModel.ts: %v", err)
	}
	src := string(data)

	checks := map[string][2]string{
		"createDefaultSettings": {"export function createDefaultSettings(): models.Settings", "export function normalizeSettings"},
		"normalizeSettings":     {"export function normalizeSettings(raw: unknown): models.Settings", "/** 规范化存储"},
		"SettingsForm":          {"export type SettingsForm = {", "export function settingsToForm"},
		"settingsToForm":        {"export function settingsToForm(s: models.Settings): SettingsForm", "export function formToSettings"},
		"formToSettings":        {"export function formToSettings(form: SettingsForm): models.Settings", "export const quotaPolicyOptions"},
	}
	sections := make(map[string]string, len(checks))
	for name, bounds := range checks {
		section, ok := sectionBetween(src, bounds[0], bounds[1])
		if !ok {
			t.Fatalf("section %s not found in settingsModel.ts", name)
		}
		sections[name] = section
	}

	settingsType := reflect.TypeOf(models.Settings{})
	for i := 0; i < settingsType.NumField(); i++ {
		field := settingsType.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		for sectionName, section := range sections {
			if !strings.Contains(section, jsonName) {
				t.Fatalf("settings field %q (%s) missing in frontend %s", jsonName, field.Name, sectionName)
			}
		}
	}
}

func sectionBetween(src, start, end string) (string, bool) {
	i := strings.Index(src, start)
	if i < 0 {
		return "", false
	}
	j := strings.Index(src[i+len(start):], end)
	if j < 0 {
		return "", false
	}
	return src[i : i+len(start)+j], true
}

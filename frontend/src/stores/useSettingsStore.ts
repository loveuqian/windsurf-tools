import { create } from "zustand";
import { APIInfo } from "../api/wails";
import { models } from "../../wailsjs/go/models";
import {
  createDefaultSettings,
  formToSettings,
  normalizeSettings,
  normalizeSwitchPlanFilter,
  settingsToForm,
} from "../utils/settingsModel";

interface SettingsState {
  settings: models.Settings | null;
  isLoading: boolean;
  isRefreshing: boolean;
  hasLoadedOnce: boolean;

  fetchSettings: (force?: boolean) => Promise<void> | void;
  updateSettings: (payload: models.Settings) => Promise<void>;
  saveAutoSwitchPlanFilter: (filter: string) => Promise<void>;
}

let fetchInFlight: Promise<void> | null = null;
let lastFetchedAt = 0;

export const useSettingsStore = create<SettingsState>((set, get) => ({
  settings: null,
  isLoading: true,
  isRefreshing: false,
  hasLoadedOnce: false,

  fetchSettings: async (force = false) => {
    const now = Date.now();
    if (fetchInFlight && !force) return fetchInFlight;
    if (!force && get().settings && now - lastFetchedAt < 2500) return;
    const blocking = !get().hasLoadedOnce || get().settings == null;
    set(blocking ? { isLoading: true } : { isRefreshing: true });
    fetchInFlight = (async () => {
      try {
        const data = await APIInfo.getSettings();
        set({ settings: normalizeSettings(data) });
      } catch (e) {
        console.error("Failed to fetch settings:", e);
        set({ settings: createDefaultSettings() });
      } finally {
        lastFetchedAt = Date.now();
        set({ hasLoadedOnce: true });
        if (blocking) set({ isLoading: false });
        else set({ isRefreshing: false });
        fetchInFlight = null;
      }
    })();
    return fetchInFlight;
  },

  updateSettings: async (payload) => {
    await APIInfo.updateSettings(payload);
    set({ settings: normalizeSettings(payload) });
  },

  saveAutoSwitchPlanFilter: async (filter) => {
    const base = normalizeSettings(get().settings ?? createDefaultSettings());
    const form = settingsToForm(base);
    form.auto_switch_plan_filter = normalizeSwitchPlanFilter(filter);
    await get().updateSettings(formToSettings(form));
  },
}));

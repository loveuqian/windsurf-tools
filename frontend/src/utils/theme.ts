import { create } from "zustand";

const STORAGE_KEY = "windsurf-tools-theme";

export type ThemeMode = "system" | "light" | "dark";

function readStored(): ThemeMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY) as ThemeMode | null;
    if (v === "light" || v === "dark" || v === "system") {
      return v;
    }
  } catch {
    /* ignore */
  }
  return "system";
}

function applyToDocument(mode: ThemeMode): void {
  const root = document.documentElement;
  let dark = false;
  if (mode === "dark") {
    dark = true;
  } else if (mode === "light") {
    dark = false;
  } else {
    dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  }
  if (dark) {
    root.classList.add("dark");
  } else {
    root.classList.remove("dark");
  }
}

interface ThemeState {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  cycle: () => void;
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  mode: readStored(),
  setMode: (mode) => {
    set({ mode });
    applyToDocument(mode);
    try {
      localStorage.setItem(STORAGE_KEY, mode);
    } catch {
      /* ignore */
    }
  },
  cycle: () => {
    const order: ThemeMode[] = ["system", "light", "dark"];
    const i = order.indexOf(get().mode);
    get().setMode(order[(i + 1) % order.length]!);
  },
}));

// 模块加载阶段立即应用一次（避免页面初始闪烁）
applyToDocument(useThemeStore.getState().mode);

if (typeof window !== "undefined") {
  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", () => {
      if (useThemeStore.getState().mode === "system") {
        applyToDocument("system");
      }
    });
}

export function themeLabel(mode: ThemeMode): string {
  switch (mode) {
    case "system":
      return "主题：跟随系统";
    case "light":
      return "主题：浅色";
    case "dark":
      return "主题：深色";
    default:
      return "主题";
  }
}

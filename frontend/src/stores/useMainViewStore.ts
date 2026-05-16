import { create } from "zustand";
import { DEFAULT_MAIN_VIEW, type ShellViewTab } from "../utils/appMode";

/** 主界面当前标签（纯 MITM 模式下保留总览 / 号池 / 中转 / 设置） */
type MainViewState = {
  activeTab: ShellViewTab;
  setActiveTab: (tab: ShellViewTab) => void;
};

export const useMainViewStore = create<MainViewState>((set) => ({
  activeTab: DEFAULT_MAIN_VIEW,
  setActiveTab: (tab) => set({ activeTab: tab }),
}));

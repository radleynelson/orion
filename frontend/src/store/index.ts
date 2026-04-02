import { create } from 'zustand';

export interface Tab {
  id: string;
  label: string;
  terminalId: string;
  type: 'shell' | 'claude' | 'codex' | 'server';
}

interface OrionState {
  tabs: Tab[];
  activeTabId: string | null;

  addTab: (tab: Tab) => void;
  removeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
}

let tabCounter = 0;

export function generateTabId(): string {
  tabCounter++;
  return `tab-${tabCounter}-${Date.now()}`;
}

export function generateTerminalId(): string {
  tabCounter++;
  return `term-${tabCounter}-${Date.now()}`;
}

export const useStore = create<OrionState>((set, get) => ({
  tabs: [],
  activeTabId: null,

  addTab: (tab) => {
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tab.id,
    }));
  },

  removeTab: (id) => {
    set((state) => {
      const newTabs = state.tabs.filter((t) => t.id !== id);
      let newActiveId = state.activeTabId;

      if (state.activeTabId === id) {
        const idx = state.tabs.findIndex((t) => t.id === id);
        if (newTabs.length > 0) {
          newActiveId = newTabs[Math.min(idx, newTabs.length - 1)].id;
        } else {
          newActiveId = null;
        }
      }

      return { tabs: newTabs, activeTabId: newActiveId };
    });
  },

  setActiveTab: (id) => set({ activeTabId: id }),
}));

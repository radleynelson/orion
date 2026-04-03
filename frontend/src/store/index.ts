import { create } from 'zustand';
import { workspace } from '../../wailsjs/go/models';

// --- Pane tree types ---

export interface PaneLeaf {
  type: 'terminal';
  id: string;
  terminalId: string;
}

export interface PaneSplit {
  type: 'horizontal' | 'vertical';
  id: string;
  children: Pane[];
  sizes: number[]; // percentage for each child (e.g., [50, 50])
}

export type Pane = PaneLeaf | PaneSplit;

// --- Tab type ---

export interface Tab {
  id: string;
  label: string;
  rootPane: Pane;
  tabType: 'shell' | 'claude' | 'codex' | 'server' | 'mixed';
  workspacePath: string;
}

interface ProjectState {
  name: string;
  root: string;
  mainBranch: string;
}

interface OrionState {
  project: ProjectState | null;
  setProject: (p: ProjectState) => void;

  workspaces: workspace.Workspace[];
  activeWorkspacePath: string | null;
  setWorkspaces: (ws: workspace.Workspace[]) => void;
  setActiveWorkspace: (path: string) => void;

  tabs: Tab[];
  activeTabId: string | null;
  focusedPaneId: string | null;
  addTab: (tab: Tab) => void;
  removeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
  setFocusedPane: (paneId: string) => void;

  // Split operations
  splitPane: (paneId: string, direction: 'horizontal' | 'vertical', newTerminalId: string) => void;
  closePane: (paneId: string) => string | null; // returns terminalId of closed pane, or null
  resizePanes: (splitId: string, sizes: number[]) => void;
  navigatePane: (direction: 'next' | 'prev') => string | null; // returns focused pane's terminalId

  // Merge & rearrange
  mergeTabInto: (sourceTabId: string, targetTabId: string) => void;
  swapPane: (direction: 'next' | 'prev') => void;
  rotateSplit: () => void;
  renameTab: (tabId: string, newLabel: string) => void;
  detachPane: () => void; // pop focused pane out into its own tab

  // Helpers
  getAllTerminalIds: (tab: Tab) => string[];
  getFocusedTerminalId: () => string | null;
}

let counter = 0;

export function generateId(prefix: string): string {
  counter++;
  return `${prefix}-${counter}-${Date.now()}`;
}

// --- Pane tree helpers ---

function collectLeaves(pane: Pane): PaneLeaf[] {
  if (pane.type === 'terminal') return [pane];
  return pane.children.flatMap(collectLeaves);
}

function findPaneParent(root: Pane, targetId: string): PaneSplit | null {
  if (root.type === 'terminal') return null;
  for (const child of root.children) {
    if (child.id === targetId) return root;
    if (child.type !== 'terminal') {
      const found = findPaneParent(child, targetId);
      if (found) return found;
    }
  }
  return null;
}

function replacePaneInTree(root: Pane, targetId: string, replacement: Pane): Pane {
  if (root.id === targetId) return replacement;
  if (root.type === 'terminal') return root;
  return {
    ...root,
    children: root.children.map((c) => replacePaneInTree(c, targetId, replacement)),
  };
}

function removePaneFromTree(root: Pane, targetId: string): Pane | null {
  if (root.id === targetId) return null;
  if (root.type === 'terminal') return root;

  const newChildren = root.children
    .map((c) => removePaneFromTree(c, targetId))
    .filter((c): c is Pane => c !== null);

  if (newChildren.length === 0) return null;
  if (newChildren.length === 1) return newChildren[0]; // collapse single-child splits

  // Recalculate sizes proportionally
  const totalSize = root.sizes.reduce((a, b) => a + b, 0);
  const remainingIndices: number[] = [];
  root.children.forEach((c, i) => {
    if (newChildren.some((nc) => nc.id === c.id)) {
      remainingIndices.push(i);
    }
  });
  let newSizes = remainingIndices.map((i) => root.sizes[i]);
  const newTotal = newSizes.reduce((a, b) => a + b, 0);
  newSizes = newSizes.map((s) => (s / newTotal) * totalSize);

  return { ...root, children: newChildren, sizes: newSizes };
}

function updateSizesInTree(root: Pane, splitId: string, sizes: number[]): Pane {
  if (root.id === splitId && root.type !== 'terminal') {
    return { ...root, sizes };
  }
  if (root.type === 'terminal') return root;
  return {
    ...root,
    children: root.children.map((c) => updateSizesInTree(c, splitId, sizes)),
  };
}

// --- Store ---

export const useStore = create<OrionState>((set, get) => ({
  project: null,
  setProject: (p) => set({ project: p }),

  workspaces: [],
  activeWorkspacePath: null,
  setWorkspaces: (ws) => set({ workspaces: ws }),
  setActiveWorkspace: (path) => {
    set({ activeWorkspacePath: path });
    const tabs = get().tabs.filter((t) => t.workspacePath === path);
    if (tabs.length > 0) {
      set({ activeTabId: tabs[0].id });
      const leaves = collectLeaves(tabs[0].rootPane);
      if (leaves.length > 0) set({ focusedPaneId: leaves[0].id });
    } else {
      set({ activeTabId: null, focusedPaneId: null });
    }
  },

  tabs: [],
  activeTabId: null,
  focusedPaneId: null,

  addTab: (tab) => {
    const leaves = collectLeaves(tab.rootPane);
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tab.id,
      focusedPaneId: leaves.length > 0 ? leaves[0].id : null,
    }));
  },

  removeTab: (id) => {
    set((state) => {
      const newTabs = state.tabs.filter((t) => t.id !== id);
      let newActiveId = state.activeTabId;
      let newFocusedPane = state.focusedPaneId;

      if (state.activeTabId === id) {
        const removedTab = state.tabs.find((t) => t.id === id);
        const wsPath = removedTab?.workspacePath;
        const sameTabs = newTabs.filter((t) => t.workspacePath === wsPath);
        if (sameTabs.length > 0) {
          const idx = state.tabs.findIndex((t) => t.id === id);
          const nextTab = sameTabs[Math.min(idx, sameTabs.length - 1)];
          newActiveId = nextTab?.id ?? null;
          if (nextTab) {
            const leaves = collectLeaves(nextTab.rootPane);
            newFocusedPane = leaves.length > 0 ? leaves[0].id : null;
          }
        } else {
          newActiveId = null;
          newFocusedPane = null;
        }
      }

      return { tabs: newTabs, activeTabId: newActiveId, focusedPaneId: newFocusedPane };
    });
  },

  setActiveTab: (id) => {
    const tab = get().tabs.find((t) => t.id === id);
    if (tab) {
      const leaves = collectLeaves(tab.rootPane);
      set({ activeTabId: id, focusedPaneId: leaves.length > 0 ? leaves[0].id : null });
    }
  },

  setFocusedPane: (paneId) => set({ focusedPaneId: paneId }),

  splitPane: (paneId, direction, newTerminalId) => {
    set((state) => {
      const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
      if (!activeTab) return state;

      const newLeaf: PaneLeaf = {
        type: 'terminal',
        id: generateId('pane'),
        terminalId: newTerminalId,
      };

      const newSplit: PaneSplit = {
        type: direction,
        id: generateId('split'),
        children: [
          // Find the existing pane and keep it, add new one
          { type: 'terminal', id: paneId, terminalId: '' } as PaneLeaf, // placeholder
          newLeaf,
        ],
        sizes: [50, 50],
      };

      // Find the actual pane to preserve its terminalId
      const findPane = (p: Pane): PaneLeaf | null => {
        if (p.id === paneId && p.type === 'terminal') return p;
        if (p.type !== 'terminal') {
          for (const c of p.children) {
            const found = findPane(c);
            if (found) return found;
          }
        }
        return null;
      };

      const existingPane = findPane(activeTab.rootPane);
      if (!existingPane) return state;

      newSplit.children[0] = { ...existingPane };

      const newRoot = replacePaneInTree(activeTab.rootPane, paneId, newSplit);

      return {
        tabs: state.tabs.map((t) =>
          t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
        ),
        focusedPaneId: newLeaf.id,
      };
    });
  },

  closePane: (paneId) => {
    const state = get();
    const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
    if (!activeTab) return null;

    // Find the pane to get its terminalId
    const findPane = (p: Pane): PaneLeaf | null => {
      if (p.id === paneId && p.type === 'terminal') return p;
      if (p.type !== 'terminal') {
        for (const c of p.children) {
          const found = findPane(c);
          if (found) return found;
        }
      }
      return null;
    };

    const pane = findPane(activeTab.rootPane);
    if (!pane) return null;

    const leaves = collectLeaves(activeTab.rootPane);

    // If this is the last pane, remove the whole tab
    if (leaves.length <= 1) {
      get().removeTab(activeTab.id);
      return pane.terminalId;
    }

    // Remove pane from tree
    const newRoot = removePaneFromTree(activeTab.rootPane, paneId);
    if (!newRoot) {
      get().removeTab(activeTab.id);
      return pane.terminalId;
    }

    // Focus the next available pane
    const remainingLeaves = collectLeaves(newRoot);
    const newFocused = remainingLeaves.length > 0 ? remainingLeaves[0].id : null;

    set((s) => ({
      tabs: s.tabs.map((t) =>
        t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
      ),
      focusedPaneId: newFocused,
    }));

    return pane.terminalId;
  },

  resizePanes: (splitId, sizes) => {
    set((state) => {
      const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
      if (!activeTab) return state;
      const newRoot = updateSizesInTree(activeTab.rootPane, splitId, sizes);
      return {
        tabs: state.tabs.map((t) =>
          t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
        ),
      };
    });
  },

  navigatePane: (direction) => {
    const state = get();
    const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
    if (!activeTab) return null;

    const leaves = collectLeaves(activeTab.rootPane);
    if (leaves.length === 0) return null;

    const currentIdx = leaves.findIndex((l) => l.id === state.focusedPaneId);
    let nextIdx: number;
    if (direction === 'next') {
      nextIdx = currentIdx >= 0 ? (currentIdx + 1) % leaves.length : 0;
    } else {
      nextIdx = currentIdx > 0 ? currentIdx - 1 : leaves.length - 1;
    }

    set({ focusedPaneId: leaves[nextIdx].id });
    return leaves[nextIdx].terminalId;
  },

  mergeTabInto: (sourceTabId, targetTabId) => {
    set((state) => {
      const sourceTab = state.tabs.find((t) => t.id === sourceTabId);
      const targetTab = state.tabs.find((t) => t.id === targetTabId);
      if (!sourceTab || !targetTab || sourceTabId === targetTabId) return state;

      // Create a vertical split with target's pane on the left, source's on the right
      const newRoot: PaneSplit = {
        type: 'vertical',
        id: generateId('split'),
        children: [targetTab.rootPane, sourceTab.rootPane],
        sizes: [50, 50],
      };

      // Update target tab with merged pane tree, remove source tab
      const newTabs = state.tabs
        .filter((t) => t.id !== sourceTabId)
        .map((t) => t.id === targetTabId ? { ...t, rootPane: newRoot, label: t.label } : t);

      return {
        tabs: newTabs,
        activeTabId: targetTabId,
      };
    });
  },

  swapPane: (direction) => {
    set((state) => {
      const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
      if (!activeTab || !state.focusedPaneId) return state;

      const leaves = collectLeaves(activeTab.rootPane);
      if (leaves.length < 2) return state;

      const currentIdx = leaves.findIndex((l) => l.id === state.focusedPaneId);
      if (currentIdx < 0) return state;

      const swapIdx = direction === 'next'
        ? (currentIdx + 1) % leaves.length
        : (currentIdx - 1 + leaves.length) % leaves.length;

      // Swap the two leaves' terminalIds and ids in the tree
      const currentLeaf = leaves[currentIdx];
      const swapLeaf = leaves[swapIdx];

      const swapInTree = (pane: Pane): Pane => {
        if (pane.type === 'terminal') {
          if (pane.id === currentLeaf.id) {
            return { ...pane, terminalId: swapLeaf.terminalId };
          }
          if (pane.id === swapLeaf.id) {
            return { ...pane, terminalId: currentLeaf.terminalId };
          }
          return pane;
        }
        return { ...pane, children: pane.children.map(swapInTree) };
      };

      const newRoot = swapInTree(activeTab.rootPane);
      return {
        tabs: state.tabs.map((t) =>
          t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
        ),
        // Keep focus on the same pane position (so it follows the swap)
        focusedPaneId: swapLeaf.id,
      };
    });
  },

  rotateSplit: () => {
    set((state) => {
      const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
      if (!activeTab || !state.focusedPaneId) return state;

      // Find the parent split of the focused pane
      const parent = findPaneParent(activeTab.rootPane, state.focusedPaneId);
      if (!parent) return state;

      // Toggle between horizontal and vertical
      const newDirection = parent.type === 'vertical' ? 'horizontal' : 'vertical';
      const rotated: Pane = { ...parent, type: newDirection };
      const newRoot = replacePaneInTree(activeTab.rootPane, parent.id, rotated);

      return {
        tabs: state.tabs.map((t) =>
          t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
        ),
      };
    });
  },

  renameTab: (tabId, newLabel) => {
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId ? { ...t, label: newLabel } : t
      ),
    }));
  },

  detachPane: () => {
    set((state) => {
      const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
      if (!activeTab || !state.focusedPaneId) return state;

      const leaves = collectLeaves(activeTab.rootPane);
      if (leaves.length <= 1) return state; // can't detach the only pane

      // Find the focused pane
      const focusedLeaf = leaves.find((l) => l.id === state.focusedPaneId);
      if (!focusedLeaf) return state;

      // Remove it from the current tab's pane tree
      const newRoot = removePaneFromTree(activeTab.rootPane, state.focusedPaneId);
      if (!newRoot) return state;

      // Focus remaining pane in old tab
      const remainingLeaves = collectLeaves(newRoot);

      // Create a new tab with the detached pane
      const newTab: Tab = {
        id: generateId('tab'),
        label: 'Detached',
        rootPane: { ...focusedLeaf },
        tabType: activeTab.tabType,
        workspacePath: activeTab.workspacePath,
      };

      return {
        tabs: [
          ...state.tabs.map((t) =>
            t.id === activeTab.id ? { ...t, rootPane: newRoot } : t
          ),
          newTab,
        ],
        activeTabId: newTab.id,
        focusedPaneId: focusedLeaf.id,
      };
    });
  },

  getAllTerminalIds: (tab) => collectLeaves(tab.rootPane).map((l) => l.terminalId),

  getFocusedTerminalId: () => {
    const state = get();
    const activeTab = state.tabs.find((t) => t.id === state.activeTabId);
    if (!activeTab) return null;
    const leaves = collectLeaves(activeTab.rootPane);
    const focused = leaves.find((l) => l.id === state.focusedPaneId);
    return focused?.terminalId ?? null;
  },
}));

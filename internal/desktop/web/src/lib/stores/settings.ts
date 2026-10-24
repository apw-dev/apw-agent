import { writable, get } from "svelte/store";
import {
  getNativeShellState,
  saveNativeUIPreferences,
  setNativeTheme,
  type UIPreferences,
  type WorkspaceSelection
} from "../shell";

export type ThemeMode = "system" | "light" | "dark";
export type CenterView = "thread" | "autonomy" | "board";
export type RightPanelView = "cards" | "diff" | "file" | "files" | "symbols" | "graph";

export interface ContextMenuState {
  x: number;
  y: number;
  copyText?: string;
  filePath?: string;
}

const LS_KEY = "apw_desktop_ui_v2";

function loadLocalPrefs(): UIPreferences & { theme?: ThemeMode } {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) return JSON.parse(raw);
  } catch {
    /* storage unavailable */
  }
  return {};
}

function saveLocalPrefs(prefs: UIPreferences & { theme?: ThemeMode }) {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(prefs));
  } catch {
    /* storage unavailable */
  }
}

export const themeMode = writable<ThemeMode>("system");
export const resolvedTheme = writable<"light" | "dark">("light");
export const recentWorkspaces = writable<string[]>([]);
export const leftWidth = writable<number>(240);
export const rightWidth = writable<number>(360);
export const leftCollapsed = writable<boolean>(false);
export const rightCollapsed = writable<boolean>(false);
export const collapsedCards = writable<Record<string, boolean>>({});
export const workspaceSelections = writable<Record<string, WorkspaceSelection>>({});

export const centerView = writable<CenterView>("thread");
export const rightPanelView = writable<RightPanelView>("cards");
export const selectedDiffFile = writable<string>("");
export const highlightedFileLine = writable<number | null>(null);
export const composerDraftText = writable<string>("");
export const settingsModalOpen = writable<boolean>(false);
export const workspacePopoverOpen = writable<boolean>(false);
export const contextMenu = writable<ContextMenuState | null>(null);

export function resolveEffectiveTheme(mode: ThemeMode): "light" | "dark" {
  if (mode === "light" || mode === "dark") return mode;
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return "light";
}

export function applyThemeToDocument(mode: ThemeMode) {
  const effective = resolveEffectiveTheme(mode);
  resolvedTheme.set(effective);
  if (typeof document !== "undefined") {
    document.documentElement.setAttribute("data-theme", effective);
  }
}

export async function initSettingsStore(): Promise<void> {
  const local = loadLocalPrefs();
  if (local.theme) themeMode.set(local.theme);
  if (local.left_width) leftWidth.set(local.left_width);
  if (local.right_width) rightWidth.set(local.right_width);
  if (typeof local.left_collapsed === "boolean") leftCollapsed.set(local.left_collapsed);
  if (typeof local.right_collapsed === "boolean") rightCollapsed.set(local.right_collapsed);
  if (local.collapsed_cards) collapsedCards.set(local.collapsed_cards);
  if (local.selections) workspaceSelections.set(local.selections);

  const shell = await getNativeShellState();
  if (shell) {
    if (shell.theme) themeMode.set(shell.theme);
    if (shell.recent) recentWorkspaces.set(shell.recent);
    if (shell.ui) {
      if (shell.ui.left_width) leftWidth.set(shell.ui.left_width);
      if (shell.ui.right_width) rightWidth.set(shell.ui.right_width);
      if (typeof shell.ui.left_collapsed === "boolean") leftCollapsed.set(shell.ui.left_collapsed);
      if (typeof shell.ui.right_collapsed === "boolean") rightCollapsed.set(shell.ui.right_collapsed);
      if (shell.ui.collapsed_cards) collapsedCards.set(shell.ui.collapsed_cards);
      if (shell.ui.selections) workspaceSelections.set(shell.ui.selections);
    }
  }
  applyThemeToDocument(get(themeMode));
}

export function persistUIPreferences() {
  const ui: UIPreferences = {
    left_width: get(leftWidth),
    right_width: get(rightWidth),
    left_collapsed: get(leftCollapsed),
    right_collapsed: get(rightCollapsed),
    collapsed_cards: get(collapsedCards),
    selections: get(workspaceSelections)
  };
  saveLocalPrefs({ ...ui, theme: get(themeMode) });
  void saveNativeUIPreferences(ui);
}

export async function updateTheme(mode: ThemeMode): Promise<void> {
  themeMode.set(mode);
  applyThemeToDocument(mode);
  persistUIPreferences();
  await setNativeTheme(mode);
}

export function toggleCardCollapsed(cardId: string) {
  collapsedCards.update((cur) => {
    const next = { ...cur, [cardId]: !cur[cardId] };
    return next;
  });
  persistUIPreferences();
}

export function toggleLeftSidebar() {
  leftCollapsed.update((v) => !v);
  persistUIPreferences();
}

export function toggleRightPanel() {
  rightCollapsed.update((v) => !v);
  persistUIPreferences();
}

export function rememberSelectionForWorkspace(workspace: string, patch: Partial<WorkspaceSelection>) {
  const key = workspace || "default";
  workspaceSelections.update((cur) => {
    const next: Record<string, WorkspaceSelection> = { ...cur };
    if (patch.task_id === "") {
      for (const k of Object.keys(next)) {
        next[k] = { ...next[k], task_id: "" };
      }
    }
    const prev = next[key] || {};
    next[key] = { ...prev, ...patch };
    return next;
  });
  persistUIPreferences();
  if (patch.session_id !== undefined) {
    try {
      localStorage.setItem("apw_desktop_session", patch.session_id || "");
    } catch {
      /* storage unavailable */
    }
  }
  if (patch.task_id !== undefined) {
    try {
      localStorage.setItem("apw_desktop_task", patch.task_id || "");
    } catch {
      /* storage unavailable */
    }
  }
}

export function getSelectionForWorkspace(workspace: string): WorkspaceSelection {
  const map = get(workspaceSelections);
  const fromMap = map[workspace || "default"] || {};
  let fallbackSession = "";
  let fallbackTask = "";
  try {
    fallbackSession = localStorage.getItem("apw_desktop_session") || "";
    fallbackTask = localStorage.getItem("apw_desktop_task") || "";
  } catch {
    /* storage unavailable */
  }
  const hasExplicitTask = Object.prototype.hasOwnProperty.call(fromMap, "task_id");
  const resolvedTask = hasExplicitTask ? fromMap.task_id || undefined : fallbackTask || undefined;
  return {
    session_id: fromMap.session_id || fallbackSession || undefined,
    task_id: resolvedTask
  };
}

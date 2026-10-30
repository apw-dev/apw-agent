export interface WorkspaceSelection {
  session_id?: string;
  task_id?: string;
}

export interface UIPreferences {
  language?: "en" | "th";
  left_width?: number;
  right_width?: number;
  left_collapsed?: boolean;
  right_collapsed?: boolean;
  collapsed_cards?: Record<string, boolean>;
  selections?: Record<string, WorkspaceSelection>;
}

export interface ProjectSessionItem {
  id: string;
  title: string;
  updated_at: string;
  turn_count: number;
  running?: boolean;
}

export interface ProjectFolderView {
  path: string;
  name: string;
  active: boolean;
  running?: boolean;
  sessions: ProjectSessionItem[];
}

export interface ShellState {
  current_workspace: string;
  recent: string[];
  projects?: ProjectFolderView[];
  theme: "system" | "light" | "dark";
  ui: UIPreferences;
}

declare global {
  interface Window {
    runtime?: {
      BrowserOpenURL?: (url: string) => void;
    };
    go?: {
      main?: {
        App?: {
          ChooseWorkspace: () => Promise<void>;
          ChooseWorkspaceNewWindow?: () => Promise<void>;
          OpenWorkspace: (dir: string) => Promise<void>;
          SwitchProjectSession?: (dir: string, sessionId: string) => Promise<void>;
          RemoveProject?: (dir: string) => Promise<void>;
          OpenNewWindow?: (dir: string) => Promise<void>;
          SetTheme: (mode: string) => Promise<void>;
          GetShellState?: () => Promise<ShellState>;
          SaveUIPreferences?: (ui: UIPreferences) => Promise<void>;
          RevealInExplorer?: (target: string) => Promise<void>;
        };
      };
    };
  }
}

export function inDesktopShell(): boolean {
  return typeof window !== "undefined" && !!(window.runtime && window.go);
}

export async function chooseWorkspace(): Promise<void> {
  if (window.go?.main?.App?.ChooseWorkspace) {
    await window.go.main.App.ChooseWorkspace();
  }
}

export async function openWorkspaceDir(dir: string): Promise<void> {
  if (window.go?.main?.App?.OpenWorkspace) {
    await window.go.main.App.OpenWorkspace(dir);
  }
}

export async function switchProjectSession(dir: string, sessionId: string): Promise<void> {
  if (window.go?.main?.App?.SwitchProjectSession) {
    await window.go.main.App.SwitchProjectSession(dir, sessionId);
  } else if (window.go?.main?.App?.OpenWorkspace) {
    await window.go.main.App.OpenWorkspace(dir);
  }
}

export async function removeProjectFolder(dir: string): Promise<void> {
  if (window.go?.main?.App?.RemoveProject) {
    await window.go.main.App.RemoveProject(dir);
  }
}

export async function openNewWindow(dir = ""): Promise<void> {
  if (window.go?.main?.App?.OpenNewWindow) {
    await window.go.main.App.OpenNewWindow(dir);
  }
}

export async function chooseWorkspaceNewWindow(): Promise<void> {
  if (window.go?.main?.App?.ChooseWorkspaceNewWindow) {
    await window.go.main.App.ChooseWorkspaceNewWindow();
  }
}

export async function setNativeTheme(mode: "system" | "light" | "dark"): Promise<void> {
  if (window.go?.main?.App?.SetTheme) {
    try {
      await window.go.main.App.SetTheme(mode);
    } catch {
      return;
    }
  }
}

export async function getNativeShellState(): Promise<ShellState | null> {
  if (window.go?.main?.App?.GetShellState) {
    try {
      return await window.go.main.App.GetShellState();
    } catch {
      return null;
    }
  }
  return null;
}

export async function saveNativeUIPreferences(ui: UIPreferences): Promise<void> {
  if (window.go?.main?.App?.SaveUIPreferences) {
    try {
      await window.go.main.App.SaveUIPreferences(ui);
    } catch {
      return;
    }
  }
}

export async function revealInExplorer(target: string): Promise<void> {
  if (window.go?.main?.App?.RevealInExplorer) {
    try {
      await window.go.main.App.RevealInExplorer(target);
    } catch {
      return;
    }
  }
}

export function openExternalURL(url: string): void {
  if (inDesktopShell() && window.runtime?.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url);
  } else if (typeof window !== "undefined") {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

export const BLOCKED_CTRL_KEYS = new Set(["r", "f", "p", "u", "=", "+", "-", "0"]);
export const BLOCKED_CTRL_CODES = new Set([
  "KeyR",
  "KeyF",
  "KeyP",
  "KeyU",
  "Equal",
  "Minus",
  "Digit0",
  "NumpadAdd",
  "NumpadSubtract",
  "Numpad0"
]);

export function isBlockedBrowserShortcut(e: KeyboardEvent): boolean {
  const key = (e.key || "").toLowerCase();
  const code = e.code || "";
  const ctrl = e.ctrlKey || e.metaKey;
  if (e.key === "F5" || e.key === "F12" || code === "F5" || code === "F12") {
    return true;
  }
  if (ctrl && e.shiftKey && (key === "i" || code === "KeyI")) {
    return true;
  }
  if (ctrl && !e.shiftKey && (BLOCKED_CTRL_KEYS.has(key) || BLOCKED_CTRL_CODES.has(code))) {
    return true;
  }
  return false;
}

# APW Desktop Design System (`DESIGN.md`)

## 1. Design Direction & Domain

- **Product:** APW Desktop (`apw-desktop`), a native Windows developer environment powered by Wails v2 + Svelte 5 for autonomous software engineering in isolated Git worktrees.
- **Visual Direction:** Warm Editorial Utility (Warm Paper in Light Mode, Carbon Slate in Dark Mode). High-density, keyboard-first 3-pane workbench where every surface, badge, and diff line serves code inspection, verification evidence, or human-in-the-loop approval.
- **No Decorative Slop:** No ambient gradients, no decorative blurs on non-overlay panels, no generic AI sparkle icons, and no em dashes (`—`) in UI copy.

---

## 2. Color Tokens & WCAG AA Contrast Contract

All text/surface pairings meet or exceed **WCAG 2.1 AA** (`4.5:1` for normal text, `3.0:1` for large text and active UI indicators).

### Light Mode (`:root`, `[data-theme="light"]`)

| Token | Value | Role & Verified Contrast |
| :--- | :--- | :--- |
| `--bg` | `#f7f7f5` | Primary workbench canvas |
| `--side-solid` | `#efeeea` | Left navigation sidebar surface |
| `--panel` | `#ffffff` | Card, modal, and review panel surface |
| `--panel-subtle` | `#f1f0ec` | Code log and chip container surface |
| `--line` / `--line-strong` | `#e3e1db` / `#c9c7c0` | Hairline structural borders and scrollbars |
| `--text` | `#1f1e1c` | Primary copy (`15.8:1` on `--bg`) |
| `--muted` | `#5c5953` | Secondary copy (`6.4:1` on `--bg`) |
| `--faint` | `#66635c` | Metadata, line numbers, kbd hints (`5.59:1` on `--bg`, `5.16:1` on `--side-solid`) |
| `--accent` | `#a84b2b` | Terracotta brand & interactive accent (`5.67:1` with `--accent-fg: #ffffff`, `4.87:1` on `--accent-soft`) |
| `--accent-fg` | `#ffffff` | Foreground on solid `--accent` controls |
| `--ok` / `--warn` / `--err` | `#256b42` / `#9a5200` / `#b92516` | Semantic verification, trust, and failure states |

### Dark Mode (`[data-theme="dark"]`)

| Token | Value | Role & Verified Contrast |
| :--- | :--- | :--- |
| `--bg` | `#1f1e1c` | Primary workbench canvas |
| `--side-solid` | `#181715` | Left navigation sidebar surface |
| `--panel` | `#272623` | Card, modal, and review panel surface |
| `--panel-subtle` | `#2e2d29` | Code log and chip container surface |
| `--line` / `--line-strong` | `#383632` / `#4c4944` | Hairline structural borders and scrollbars |
| `--text` | `#f2f0eb` | Primary copy (`14.9:1` on `--bg`) |
| `--muted` | `#b8b5ad` | Secondary copy (`7.8:1` on `--bg`) |
| `--faint` | `#949087` | Metadata, line numbers, kbd hints (`4.75:1` on `--panel`, `5.63:1` on `--side-solid`) |
| `--accent` | `#e08565` | Warm terracotta accent (`5.11:1` on `--accent-soft`, `6.58:1` with `--accent-fg: #181715`) |
| `--accent-fg` | `#181715` | Foreground on solid `--accent` controls |
| `--ok` / `--warn` / `--err` | `#5ce08e` / `#fbbf24` / `#f87171` | Semantic verification, trust, and failure states |

---

## 3. Typography

- **UI & Thai Copy:** `"Noto Sans"`, `"Noto Sans Thai"`, `"Segoe UI Variable"`, `"Segoe UI"`, `"Leelawadee UI"`, `system-ui`, `sans-serif` (bundled offline as `.woff2` assets; no remote font requests).
- **Code, Diffs, & Identifiers:** `"Cascadia Code"`, `Consolas`, `monospace`.
- **Scale & Rhythm:** Base `13px` with `line-height: 1.6` and `letter-spacing: 0` to preserve Thai vowel/tone-mark vertical clearance (`สระและวรรณยุกต์ไม่ซ้อนทับ`).

---

## 4. Layout & Interaction Principles

1. **Three-Pane Workbench:**
   - **Left Sidebar (`180px–380px`):** Workspace switcher, trust indicator, active tasks, day-grouped conversations, settings, and 24/7 autonomy entry.
   - **Center Pane (`flex: 1`, `max-width: 760px` column):** Conversation thread, execution plans, human-in-the-loop approval cards, task progress cards, and multi-mode composer (`execute`, `preview`, `review`, `unverified`).
   - **Right Review Panel (`280px–580px`):** Collapsible cards (`Changes`, `Verification`, `Status`, `Conflict`, `Workspace`) and full Shiki-highlighted diff/file/symbol/graph viewers with a 3,000-line plain-text fast path.
2. **Keyboard & Screen Reader Operability:**
   - Global shortcuts (`Ctrl+N`, `Ctrl+K`, `Ctrl+B`, `Ctrl+J`, `Ctrl+,`, `Ctrl+O`, `Escape`).
   - Pane resizers expose `role="separator"`, `aria-orientation="vertical"`, `tabindex="0"`, and arrow-key resizing.
   - Interactive diff previews and modals expose semantic roles, focus visibility (`:focus-visible`), and keyboard activation.

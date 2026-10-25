# Portable Windows release & NSIS installer

Build the portable executables (`apw.exe`, `apwd.exe`, and `APW.exe`) from the same saved source:

```powershell
npm --prefix internal/desktop/web ci
npm --prefix internal/desktop/web run build
python scripts/package_release.py --version 0.1.0-rc.14
```

The output is a new directory and ZIP under `.apw/releases`, a SHA-256 checksum, a file manifest, and matching version/revision/time/source fingerprints embedded in the executables. The builder refuses an existing output directory or source changes during compilation. `--install` additionally installs the matching CLI/daemon pair through Go. A checksum detects changed bytes; this package is not code-signed (unsigned Windows builds trigger a SmartScreen warning).

## Native Desktop (`APW.exe`) and NSIS installer

`APW.exe` is the native Windows Desktop shell built with Wails v2 (`cmd/apw-desktop`). It hosts the workspace's `internal/desktop` server in-process via the Wails AssetServer (no localhost port opened) and embeds the compiled Svelte 5 + TypeScript + Vite frontend (`internal/desktop/web/dist`).

To build the NSIS installer (`APW-Setup-<version>.exe`) with Start Menu shortcut, uninstaller, and optional PATH registration:

```powershell
cd cmd/apw-desktop
wails build -s -clean -skipbindings
go -C ../.. build -mod=readonly -trimpath -o cmd/apw-desktop/build/bin/cli/apw.exe ./cmd/apw
go -C ../.. build -mod=readonly -trimpath -o cmd/apw-desktop/build/bin/apwd.exe ./cmd/apwd
Copy-Item ../../apw.example.json build/bin/apw.example.json -Force
makensis /DINFO_PRODUCTVERSION="0.1.0" build/windows/installer/project.nsi
```

On a machine without an `apw.json` (and without `ANTHROPIC_API_KEY`), both roles start on the built-in placeholder `apw-builtin`, which cannot plan or edit code. When a workspace opens in `APW.exe` or `apw desktop` while a role still uses it, APW adds the models of connected accounts (AGY, Claude Code, Codex/OpenAI, Ollama) to that workspace's `apw.json`, as `/model` does in the shell; it does not choose the roles. Until the architect and reflex roles name a real model (Settings, `Ctrl+,`), tasks and plans are refused with a message that lists the models found or what to connect. Prerequisites on the target machine: Git, at least one signed-in model account or `ANTHROPIC_API_KEY` or a running Ollama, and the project's own toolchain for verification.

Extract or install `APW.exe`, `apw.exe`, and `apwd.exe` into the same folder. Add that folder to PATH or call `apw.exe` / `APW.exe` by its absolute path. The CLI and Desktop prefer their sibling `apwd.exe` over another version on PATH, and `apw desktop` on Windows automatically launches a sibling `APW.exe` when present (falling back to Edge `--app` only when `APW.exe` is absent or `--port`/`--no-open` is passed). Check the pair with `apw version --json` and `apwd --version`.

Before upgrading, inspect current tasks and stop the daemon for each active workspace with `apw daemon --config <path> stop`. Keep the existing binaries as a rollback copy and close interactive CLI and `APW.exe` sessions. Replace all binaries together. Preserve the workspace's `apw.json`, `.apw`, `.git` and `.worktrees` directories. Start the replacement CLI or Desktop and inspect tasks before explicitly resuming interrupted work. The CLI rejects a running packaged daemon with a different source fingerprint.

Do not replace an existing configuration with `apw.example.json`. It is a template for a new workspace. This release supports Go verification; a Git worktree does not sandbox malicious build/test code. Trust a repository before allowing its code to execute.

Legacy worktree recovery exports evidence and preserves the old checkout. It does not silently transplant an old snapshot into the current workspace. Review the exported changes before preparing a new verified task.

Release acceptance is recorded in `docs/acceptance.md` in the repository and `ACCEPTANCE.md` in the ZIP. Unconfigured API credentials or chat channels are not claimed as tested. The portable installation check runs on the current Windows host from a fresh extracted folder, keeping the original PATH to test sibling daemon preference; it is not a clean Windows VM certification. A dirty-tree build records its base Git revision and a separate fingerprint of the saved Go sources, rather than claiming to represent a clean commit.

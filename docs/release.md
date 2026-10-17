# Portable Windows release

Build both executables from the same saved source:

```powershell
python scripts/package_release.py --version 0.1.0-rc.1
```

The output is a new directory and ZIP under `.apw/releases`, a SHA-256 checksum, a file manifest, and matching version/revision/time/source fingerprints embedded in both executables. The builder refuses an existing output directory or source changes during compilation. `--install` additionally installs the matching pair through Go. A checksum detects changed bytes; this package is not code-signed.

Extract both `apw.exe` and `apwd.exe` into the same folder. Add that folder to PATH or call `apw.exe` by its absolute path. The CLI prefers its sibling daemon over another version on PATH. Check the pair with `apw version --json` and `apwd --version`.

Before upgrading, inspect current tasks and stop the daemon for each active workspace with `apw daemon --config <path> stop`. Keep the existing binaries as a rollback copy and close interactive CLI sessions. Replace both binaries together. Preserve the workspace's `apw.json`, `.apw`, `.git` and `.worktrees` directories. Start the replacement CLI and inspect tasks before explicitly resuming interrupted work. The CLI rejects a running packaged daemon with a different source fingerprint.

Do not replace an existing configuration with `apw.example.json`. It is a template for a new workspace. This release supports Go verification; a Git worktree does not sandbox malicious build/test code. Trust a repository before allowing its code to execute.

Legacy worktree recovery exports evidence and preserves the old checkout. It does not silently transplant an old snapshot into the current workspace. Review the exported changes before preparing a new verified task.

Release acceptance is recorded in `docs/acceptance.md` in the repository and `ACCEPTANCE.md` in the ZIP. Unconfigured API credentials or chat channels are not claimed as tested. The portable installation check runs on the current Windows host from a fresh extracted folder, keeping the original PATH to test sibling daemon preference; it is not a clean Windows VM certification. A dirty-tree build records its base Git revision and a separate fingerprint of the saved Go sources, rather than claiming to represent a clean commit.

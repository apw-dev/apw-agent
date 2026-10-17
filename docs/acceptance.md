# Core CLI acceptance

Checked on 2026-10-17, Windows/amd64, Go 1.27.0. This records the tested scope, not certification of every provider or channel.

| Lane | Result and evidence |
| --- | --- |
| Selected Codex CLI, `gpt-6.1-sol` | Live nonce response passed. The CLI stderr model header matched the requested model; this is not independent server attestation. |
| Selected AGY, `gemini-3.8-flash-high` | Live nonce and surgical patch passed with the explicit model argument. Text and JSON output expose no independent model identity. Do not label the requested name as observed metadata. |
| Real model workflow | Codex planned, Flash patched, executable acceptance and Go verification passed, explicit apply succeeded, independent Go tests passed. Existing test bytes were preserved. Original source stayed unchanged before apply. |
| Crash/restart/resume | Local fixture: terminate the owned daemon after step 1, restart without model replay, explicitly resume only the unfinished step, verify and apply. Passed. |
| Windows Ctrl+C | Actual CLI received CTRL_C_EVENT in its own hidden console, exited 130 and persisted cancellation. Passed. |
| Offline regressions | Race suite covers source safety, owner isolation, deduplication, quota classification, configured fallback, transaction recovery and process cancellation. Real quota was not intentionally exhausted. |
| Legacy task | Task `task-1792166521189239500` remains at its original 7/13 checkpoint. Exported 16 dirty source files, binary patch, hashes, plan and checkpoint for review. No replay or automatic transplant of the old checkout. |
| Portable pair and upgrade | RC `0.1.0-rc.1`, source `53bc8d459f4d`: ZIP checksum and all manifest hashes passed. Fresh extraction into a spaced path used its matching sibling daemon with the original PATH. Crash/resume and console Ctrl+C passed again. The new CLI rejected the older daemon before changing jobs; restart with the matching pair preserved both existing terminal jobs. Installed binaries match the portable pair. |
| Portable pair rc.4 | RC `0.1.0-rc.4`, revision `d331698`, source `af913876ac8e`: ZIP checksum and all manifest hashes passed. Fresh extraction into a spaced path. `accept_cli.py` passed in a real console: matching sibling daemon with the original PATH, crash/resume of the exact step without model replay, Ctrl+C exit 130 and persisted cancellation (`.apw/validation/rc4-accept.json`). This build adds the performance changes, architect-pool failover for planning/review and retention. The live Codex/AGY lanes above were not re-run on this build. |
| Cleanup dry run | rc.4 `apw cleanup --dry-run` on the real workspace (no task store yet, so nothing to clean) and on the rc.4 fixture store (2 recent tasks kept, 0 rows to prune). `tasks.db` and its WAL stayed untouched; only SQLite's shared-memory index `tasks.db-shm` changes, as it does for any reader. The rc.3 build rewrote `tasks.db` pages during a dry run; fixed in `d331698`. |

Local evidence is in `.apw/validation/live`: `codex-evidence.json`, `flash-envelope.json`, `real-workflow.json`, `crash-console.json` and subsequent portable/upgrade reports. The initial Flash probe used an older adapter that echoed the request as an observed name; that field is not model identity evidence.

The immutable ZIP contains the acceptance scope/procedure available at build time. Its post-build checks are in `package.json`, `portable.json`, `upgrade.json` and the adjacent release validation record. Reproduce with `scripts/package_release.py` and `scripts/accept_cli.py --bin <extracted-folder> --output <report>`. These fixture runs use a local HTTP response server; the separate real workflow report establishes the selected-provider lane. Run `accept_cli.py` from a real console: a shell that starts processes with Ctrl+C disabled (some IDE or agent tool shells) cannot deliver the console event, so the Ctrl+C step times out there for every build.

Remaining acceptance: API transports and real local-model servers have no configured credentials/endpoints for this run; Claude CLI was not selected; real Telegram delivery needs token/allowlist and an authorized destination. Real provider quota/reset and server-side model identity cannot be inferred from offline tests. A portable test on this host does not certify a fresh Windows VM. Retention is an explicit `apw cleanup` command (offline tests cover dry-run, protection and pruning); it has been dry-run against the real workspace and an rc.4 fixture, but a real (non-dry) cleanup has not yet been run on a long-lived workspace, and the legacy task stays protected.

Desktop visual redesign is the final phase. Keep the existing application service/task store as its backend. Discord is future work.

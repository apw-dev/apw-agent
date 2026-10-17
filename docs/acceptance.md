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

Local evidence is in `.apw/validation/live`: `codex-evidence.json`, `flash-envelope.json`, `real-workflow.json`, `crash-console.json` and subsequent portable/upgrade reports. The initial Flash probe used an older adapter that echoed the request as an observed name; that field is not model identity evidence.

The immutable ZIP contains the acceptance scope/procedure available at build time. Its post-build checks are in `package.json`, `portable.json`, `upgrade.json` and the adjacent release validation record. Reproduce with `scripts/package_release.py` and `scripts/accept_cli.py --bin <extracted-folder> --output <report>`. These fixture runs use a local HTTP response server; the separate real workflow report establishes the selected-provider lane.

Remaining acceptance: API transports and real local-model servers have no configured credentials/endpoints for this run; Claude CLI was not selected; real Telegram delivery needs token/allowlist and an authorized destination. Real provider quota/reset and server-side model identity cannot be inferred from offline tests. A portable test on this host does not certify a fresh Windows VM. Retention is an explicit `apw cleanup` command (offline tests cover dry-run, protection and pruning); it has not yet been run against the real workspace, and the legacy task stays protected.

Desktop visual redesign is the final phase. Keep the existing application service/task store as its backend. Discord is future work.

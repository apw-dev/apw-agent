# Measuring APW (`apw eval`)

`apw eval` runs APW on a suite of small, real coding tasks and reports how often it solves them.
Each task in `eval/testdata/<name>/` has:

- `task.json`: the instruction, the check command (`["go", "test", "./..."]`) and a timeout.
- `repo/`: the fixture project. Its own tests pass.
- `hidden/`: tests APW never sees. They are copied in only after APW applied its change.
- `solution/`: a reference fix, used only by the suite self-test.

For every task the harness copies `repo/` into a fresh Git workspace, runs the normal in-process
service with the selected configuration (plan, patch, verify in a worktree, apply), then adds
`hidden/` and runs the check. Outcomes:

| Outcome | Meaning |
| --- | --- |
| `solved` | APW applied a change and the hidden tests pass. |
| `false_positive` | APW verified and applied a change, but the hidden tests fail. This is the number to drive to zero: APW claimed success it did not deliver. |
| `unsolved` | APW produced no applicable change (it failed, refused, or verified without changing any file). A safe failure. |
| `error` | The harness or environment failed. |

```powershell
apw eval --config .\apw.json                      # all tasks, real providers from apw.json
apw eval --only clamp,paginate                     # a subset
apw eval --baseline .apw\eval\20261018-0910.json   # show changes against an earlier run
apw eval --keep                                    # keep each task workspace for inspection
```

The table shows outcome, plan steps, repair attempts and time per task, plus totals. The JSON
report goes to `.apw/eval/<time>.json` (or `--out`). The exit code is non-zero when any false
positive or error occurred. The run uses the configuration's providers, roles, failover, sandbox,
research and planning settings; each task workspace is trusted, gets its own worktree root, and
uses the detected pipeline (custom `verify.stages` are not carried over). API keys saved with
`apw auth` in the configured workspace are made available to the run through the process
environment only.

The suite currently has 12 Go tasks (bug fixes, Unicode, edge cases, features, HTTP, errors,
parsing, a multi-file rename and an API change). `go test ./internal/eval` proves every task
discriminates: the fixture's own tests pass, the hidden tests fail on the unchanged fixture, and
they pass with the reference solution. Add tasks in the same layout; keep fixtures gofmt-clean.

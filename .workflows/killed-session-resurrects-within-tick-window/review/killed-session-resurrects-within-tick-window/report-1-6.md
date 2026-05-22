TASK: Real-tmux kill→bootstrap canonical symptom integration test (killed-session-resurrects-within-tick-window-1-6)

ACCEPTANCE CRITERIA:
- AC1: kill paths leave sessions.json without killed session before hook subprocess exits.
- AC2: fresh bootstrap does not reconstruct killed session as skeleton.
- AC3: fresh portal does not list killed session in TUI.
- AC4: `@portal-restoring` set → byte-identical sessions.json.
- AC5: `_portal-saver` self-kill marker-clear → omitted via underscore filter, user sessions intact.
- AC6: `_portal-saver` self-kill marker-set → byte-identical short-circuit.

STATUS: Complete

SPEC CONTEXT: Spec § Testing Requirements → Integration Tests enumerates required real-tmux fixtures. Task 1-6 covers the canonical timeline plus the two saver-kill branches and the `@portal-restoring` defence — one external kill exercises the same `session-closed` seam that all five user-facing kill paths funnel through.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now_symptom_integration_test.go` (634 lines, three sub-tests + fixture/helpers)
  - `cmd/bootstrap/phase5_integration_test.go:106` (session-closed → `portal state commit-now` expectation updated to match migration)
- Notes:
  - Per-sub-test tmux server and state dir via `tmuxtest.New` + `t.TempDir()` give clean isolation.
  - `PORTAL_STATE_DIR` set before `tmuxtest.New` so server subprocesses inherit it.
  - `_anchor` underscore session keeps server alive without polluting `sessions.json`.
  - PATH-prepend via `portalbintest.StagePortalBinary` ensures every `portal` subprocess resolves to freshly built binary.
  - Drives initial `sessions.json` via direct `runPortalCommitNow` rather than waiting on daemon.
  - Sub-test 3 clears marker in `t.Cleanup` (belt-and-braces) and kills B post-clear to verify per-invocation gate.

TESTS:
- Status: Adequate (with one minor non-load-bearing concern)
- Coverage:
  - Sub-test 1 (canonical symptom): kills B externally → polls for `A present, B absent` with two-consecutive-consistent discipline, then runs second bootstrap and asserts B not in live tmux. Covers AC1, AC2, AC3.
  - Sub-test 2 (saver self-kill marker-clear): kills `_portal-saver`, polls for `A+B present, saver absent`. Covers AC5.
  - Sub-test 3 (`@portal-restoring` defence): sets marker, kills saver, asserts byte-identical sessions.json; clears marker, kills B, asserts B removed. Covers AC4 and AC6.
  - Diagnostic helper emits state-dir + raw tmux session/pane lists on failure.
  - `pollSessionsJSON` uses `state.ReadIndex`'s skip flag and ENOENT to reset the consecutive counter.
- Notes:
  - Sub-test 2's shape is identical to pre-kill baseline; doesn't strongly distinguish "commit-now ran" from "commit-now never ran" — underscore filter unit-tested separately, but the integration assertion is weaker than its siblings. A `save.requested` mtime check or `portal.log` scan would tighten.
  - Sub-test 3 only exercises saver-kill marker-set, not user-session kill marker-set. Spec AC6 phrased around saver self-kill — in-scope.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`, default-lane integration, `tmuxtest`/`portalbintest` skip helpers, `cmd_test` external package avoiding `sessionNames` collision.
- SOLID: Good. Fixture struct bundles per-sub-test state; helpers single-responsibility (`runPortalSubprocess`, `runPortalList`, `runPortalCommitNow`, `pollSessionsJSON`, `assertSessionsJSONHas`).
- Complexity: Low. Numbered step-list structure per sub-test.
- Modern idioms: Yes. `errors.Is(err, fs.ErrNotExist)`, `context.WithTimeout`, `t.Setenv`, `t.TempDir`, `t.Cleanup`.
- Readability: Excellent. Comment density high, comments document why not what.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Sub-test 2's poll shape is identical to pre-kill baseline — cannot prove `commit-now` actually executed. Tighten with mtime delta, log scan, or `save.requested` assertion.
- [quickfix] `state_commit_now_symptom_integration_test.go:358` comment "t.Setenv is scoped to the parent test" is misleading — `newSymptomFixture` receives the sub-test's `t`; setenv is sub-test-scoped.
- [idea] `keysOf` declared here; if not yet shared across `cmd_test` package, candidate for promotion to a small test-helper file.
- [idea] Four-event spec inventory ("TUI K, portal kill, Option-Q, M-q, external") asserted by convergence reasoning rather than exercising each path. Intentional per plan; flagging for future readers.

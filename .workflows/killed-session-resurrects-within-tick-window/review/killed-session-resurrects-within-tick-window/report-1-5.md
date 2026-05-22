TASK: Real-tmux re-entrancy integration gate (killed-session-resurrects-within-tick-window-1-5)

ACCEPTANCE CRITERIA:
- Real-tmux integration fixture confirms `commit-now` invoked from inside `session-closed` completes without deadlock/hang within reasonable bound (~1s; task budget 1.5s).
- Hang/deadlock must fail visibly (not time out silently) — signals spec-level pivot.
- tmux unreachable must skip cleanly.

STATUS: Complete

SPEC CONTEXT:
§ Hook Re-entrancy + § Testing Requirements → Integration Tests → "Hook re-entrancy validation": `commit-now` makes synchronous tmux client calls back into the same server from inside the `session-closed` hook subprocess. Spec mandates this gate pass BEFORE the rest of the implementation is taken complete; failure routes back to specification (not implementation).

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_commit_now_reentrancy_integration_test.go` (367 lines, single test + four helpers).
- Notes:
  - `TestCommitNowFromSessionClosedHook_NoDeadlockUnderRealTmux` (line 135) is the gate.
  - Skip discipline (lines 136, 149): `tmuxtest.SkipIfNoTmux` + `portalbintest.StagePortalBinary`.
  - Hook installation uses production `tmux.RegisterPortalHooks(client, nil)` (line 184) — proves task 1-4 migration is wired into same code path bootstrap step 2 invokes.
  - Anchor session `_anchor` (line 175) keeps tmux server alive after kill; underscore prefix means `keepSessionNames` filters it from assertions.
  - `PORTAL_STATE_DIR` set via `t.Setenv` propagates: test → tmux server fork → `run-shell` hook subprocess.
  - Budget `reentrancyHookBudget = 1500ms` with reasoned headroom over spec's 1s example.
  - Failure dump (lines 224-235) emits three diagnostics: state dir contents, live tmux sessions, live tmux panes. Comments distinguish (i) re-entrancy deadlock, (ii) PATH failure, (iii) slow-but-progressing.
  - Kill driven by `sock.Run(t, "kill-session", "-t", "B")` exercises real tmux `kill-session → session-closed → run-shell portal state commit-now` chain.

TESTS:
- Status: Adequate
- Coverage:
  - Happy path: hook completes within budget; `sessions.json` reflects kill (A present, B absent) via two-consecutive-consistent reads.
  - "Fail visibly, not silently": `context.WithTimeout` at 1.5s; on timeout `t.Fatalf` emits rich diagnostics.
  - "tmux unreachable": skipped via `SkipIfNoTmux` / `StagePortalBinary`.
  - Belt-and-braces elapsed-time check after success (line 246).
  - Post-poll steady-state assertion via `state.ReadIndex` (line 257).
- Notes:
  - `reentrancyConsecutiveReads = 2` guards against landing on pre-rename file mid-commit (per spec § Concurrent commit-now invocations).
  - 25ms poll cadence reasonable.
  - Not over-tested: single focused test for one gate.

CODE QUALITY:
- Project conventions: Followed
  - No `t.Parallel()` (lines 35-39 cite CLAUDE.md).
  - Default-lane integration (no `//go:build integration`), explicitly justified at lines 41-45.
  - `package cmd_test` (external) — appropriate for subprocess-driven end-to-end.
- SOLID: Good — single-purpose test, small focused helpers.
- Complexity: Low — linear setup → kill → poll → assert → diagnostic.
- Modern idioms: `t.Setenv`, `t.TempDir`, `context.WithTimeout`, `errors.Is`.
- Readability: Excellent — heavy rationale comments tie each design choice to spec sections.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Three-way failure-mode classification at lines 215-223 could be condensed into a brief table comment near constants for quicker scanning during on-call debugging.
- [idea] `dumpStateDir` here vs. near-variant `dumpStateDirForNotifyTest` in `cmd/state_notify_six_event_eventual_consistency_test.go` — consolidate to shared helper if they diverge.

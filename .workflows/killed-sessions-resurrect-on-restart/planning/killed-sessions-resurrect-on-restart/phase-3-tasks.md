---
phase: 3
phase_name: Drop Outer sh -c Wrapper in buildHydrateCommand
total: 4
---

## killed-sessions-resurrect-on-restart-3-1 | approved

### Task killed-sessions-resurrect-on-restart-3-1: Update buildHydrateCommand snapshot test to assert bare-command shape, then drop the outer `sh -c '…; exec $SHELL'` wrapper in `internal/restore/session.go`

**Problem**: `buildHydrateCommand` currently emits `sh -c 'portal state hydrate …; exec $SHELL'` for `respawn-pane -k`. The trailing `; exec $SHELL` is unreachable on every observed exit (both `execShellAndExit` and `execShellOrHookAndExit` `syscall.Exec` their replacement, never returning to the wrapper), and the outer wrapper produces three downstream defects: (1) a parked `sh` parent process on every restored pane for its lifetime, (2) `exit` typed in the user's shell drops back into the wrapper which then runs `; exec $SHELL` and replaces itself with a fresh shell, requiring two `exit`s to close the pane, and (3) a parent process tree that diverges from non-restored panes. This is Defect D in the spec problem statement.

**Solution**: Flip the existing snapshot test `TestSessionRestorer_HydrateCommandFormat` (`internal/restore/session_test.go` ~lines 568-594) red-first to assert the bare-command shape, then change `buildHydrateCommand` (`internal/restore/session.go` ~lines 409-426) to return the bare invocation `portal state hydrate --fifo <fifo> --file <file> --hook-key <hookKey>` with each value-arg shell-escaped via the existing local `shellQuoteSingle` helper (~lines 428-433). The `respawn-pane -k` invocation passes the bare command string directly; tmux execs `portal state hydrate` as the pane's initial process, and the helper's own `syscall.Exec` chain replaces it under tmux with no parked parent.

**Outcome**: `buildHydrateCommand` returns the bare form, the snapshot test asserts the new shape exactly, and `RespawnPane` is invoked with no `sh -c` envelope. Restored panes have `portal state hydrate` as their immediate child of tmux (not `sh -c …`), and after the helper's exec chain runs, the user's shell is the immediate child — no parked `sh` parent.

**Do**:
- In `internal/restore/session_test.go` ~lines 568-594, edit `TestSessionRestorer_HydrateCommandFormat` to construct `wantCmd` as `fmt.Sprintf("portal state hydrate --fifo %s --file %s --hook-key %s", wantFIFO, wantFile, "work:0.0")` — drop the `sh -c '…; exec $SHELL'` envelope and the surrounding single quotes. Run the test — it must fail against the unchanged production code (red).
- In `internal/restore/session.go` ~lines 419-426, replace the `fmt.Sprintf` body of `buildHydrateCommand` with `fmt.Sprintf("portal state hydrate --fifo %s --file %s --hook-key %s", shellQuoteSingle(fifo), shellQuoteSingle(file), shellQuoteSingle(hookKey))`. Keep `shellQuoteSingle` (~lines 428-433) unchanged — its single-quote-escaping behaviour stays correct because tmux still tokenises the command via shell rules when a single argument string is passed to `respawn-pane`.
- Run the snapshot test — it must now pass (green).
- Run `go test ./internal/restore/...` to confirm no other test in the package regresses on the new shape.

**Acceptance Criteria**:
- `TestSessionRestorer_HydrateCommandFormat` asserts the exact bare string `portal state hydrate --fifo <fifo> --file <file> --hook-key work:0.0` (no `sh -c`, no trailing `; exec $SHELL`) and passes.
- `buildHydrateCommand` source no longer contains the literal substrings `sh -c` or `exec $SHELL`.
- `shellQuoteSingle` is still invoked for `fifo`, `file`, and `hookKey` — single-quote-bearing inputs round-trip through the same escape behaviour as before.
- An empty / unset `hookKey` value still produces a valid bare invocation (the trailing `--hook-key ''` arg is well-formed and `portal state hydrate` already accepts it as it does today).
- All existing tests under `internal/restore/...` remain green.

**Tests**:
- `TestSessionRestorer_HydrateCommandFormat`: asserts the bare-command shape on representative inputs (paneKey `work:0.0`, scrollback file under `t.TempDir()`).
- Edge case — single-quote-bearing input: extend or add a sub-test that constructs a session whose generated paneKey or scrollback path contains a literal `'` character (or invoke `buildHydrateCommand` directly via an exported test seam if one exists; otherwise a table-driven sub-test of `shellQuoteSingle` that asserts the existing close-escape-reopen behaviour is preserved). The expected output preserves the `'\''` escape sequence inside the bare command.
- Edge case — empty hook key: assert `buildHydrateCommand("/x.fifo", "/y.bin", "")` returns `portal state hydrate --fifo /x.fifo --file /y.bin --hook-key ''` (or equivalent — the empty string is single-quoted by `shellQuoteSingle` of an empty string, which today returns `""` since there are no quotes to replace; the existing call site already handles this — confirm the behaviour is unchanged).

**Edge Cases**:
- Paths/hook-keys containing single quotes still escaped correctly via the existing `shellQuoteSingle` helper — no behavioural change to escape sequences, only the outer wrapper is dropped.
- Empty / unset hook-key value still produces a valid bare invocation — `--hook-key ''` is a well-formed argument that `portal state hydrate` parses today.

**Context**:
> From spec section "Fix 3: Wrapper Drop in buildHydrateCommand": "The wrapper exists for two stated reasons; both fail to materialise in practice. (1) Trailing `; exec $SHELL` as crash-resilience — the helper does not have a reachable code path that returns control to the outer wrapper. (2) Comment-stated 'exiting the shell ends the pane' — empirically broken under the wrapper: when the user types `exit`, the inner shell exits and control returns to the wrapper sh, which then runs `; exec $SHELL` and replaces itself with a *fresh* shell."
>
> From spec "Argument Quoting": "`buildHydrateCommand` returns a single shell-safe command string (no `[]string` argv split). `RespawnPane`'s interface signature is unchanged — it continues to accept a single command-string argument."
>
> From spec "Inner Hook-Firing Wrapper Is Untouched": the inner `sh -c '<HOOK>; exec $SHELL'` constructed inside `execShellOrHookAndExit` (in `cmd/state_hydrate.go`) when an on-resume hook is registered is independent of and unchanged by this task — do not touch `cmd/state_hydrate.go`'s `execShellOrHookAndExit`.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` — section "Fix 3: Wrapper Drop in `buildHydrateCommand`" (lines 173-217); AC5 (line 228).

## killed-sessions-resurrect-on-restart-3-2 | approved

### Task killed-sessions-resurrect-on-restart-3-2: Refresh `buildHydrateCommand` doc comment and confirm `RespawnPane` interface signature is unchanged (still single command-string)

**Problem**: The current `buildHydrateCommand` doc comment (`internal/restore/session.go` ~lines 409-418) describes the spec-canonical `sh -c '...; exec $SHELL'` form and explains the outer wrapper's intent ("trailing `exec $SHELL` inside sh -c lets the helper hand off…"). After Task 3-1 drops the outer wrapper, this doc becomes actively misleading: it documents an envelope the function no longer emits and rationale that no longer applies. A separate cheap task is required to rewrite the comment so it reflects the post-wrapper-drop contract; without it, future readers will assume the wrapper was inadvertently lost and may try to reintroduce it.

**Solution**: Rewrite the doc comment on `buildHydrateCommand` to describe the new bare-command shape, explain that `respawn-pane -k` atomically replaces the pane's initial process with `portal state hydrate` (no `exec` prefix needed), and call out that the helper's own `syscall.Exec` chain hands off to the user's shell directly — eliminating any parked parent. Audit the `RespawnPane` signature in `internal/tmux/tmux.go` ~line 577 (`func (c *Client) RespawnPane(target, command string) error`) and confirm it still accepts a single command-string argument; document that confirmation in this task's outcome (no signature change). Leave `shellQuoteSingle` and its doc comment unchanged — its escape-sequence rationale is still valid for the bare form's value-args.

**Outcome**: `buildHydrateCommand`'s doc comment matches the bare-command implementation, no longer references the dropped `sh -c` envelope, and explains why the outer wrapper was removed (unreachable trailer, parked-sh parent, double-`exit` bug). `RespawnPane`'s `(target, command string) error` signature is unchanged — verified via `grep` and a fresh read of `internal/tmux/tmux.go` ~lines 577-583.

**Do**:
- In `internal/restore/session.go` ~lines 409-418, replace the existing doc comment on `buildHydrateCommand` with a comment that:
  - States the function returns the bare `portal state hydrate --fifo <fifo> --file <file> --hook-key <hookKey>` form (no outer `sh -c` wrapper).
  - Notes that `respawn-pane -k` atomically replaces the pane's initial process with this command, so no `exec` prefix is required (and would be redundant — tmux's respawn already replaces, not stacks).
  - Notes that the helper's own `syscall.Exec` chain in `cmd/state_hydrate.go` hands off to the user's shell directly under tmux — there is no parked parent process for the lifetime of the pane.
  - Briefly references why the previous outer wrapper was removed: the `; exec $SHELL` trailer is unreachable (both helper exit paths `syscall.Exec` their replacement) and the wrapper caused a double-`exit` bug (user's `exit` returned control to the wrapper sh, which then exec'd a fresh shell).
- Open `internal/tmux/tmux.go` ~lines 569-583 and visually confirm the `RespawnPane` signature reads exactly `func (c *Client) RespawnPane(target, command string) error` — single command-string argument, unchanged. Run `grep -n "RespawnPane" internal/tmux/*.go internal/restore/*.go` (via the Grep tool) to confirm no caller passes a `[]string` argv split.
- Do not change `shellQuoteSingle` or its doc comment — the escape behaviour is still load-bearing for the bare form's value-args.
- Do not touch `cmd/state_hydrate.go`'s `execShellOrHookAndExit` — the inner `sh -c '<HOOK>; exec $SHELL'` envelope it constructs when an on-resume hook is registered is the inner wrapper, independent of the outer wrapper this work drops, and its semantics are preserved exactly.

**Acceptance Criteria**:
- `buildHydrateCommand`'s doc comment no longer references `sh -c`, `exec $SHELL`, or "trailing exec $SHELL inside sh -c".
- The new doc comment explicitly states the bare form, the `respawn-pane -k` atomic-replace contract, and the absence of a parked parent process.
- `RespawnPane`'s signature in `internal/tmux/tmux.go` ~line 577 remains `(target, command string) error` — confirmed via direct file read.
- No production code outside `internal/restore/session.go`'s `buildHydrateCommand` is modified by this task.
- `go build ./...` succeeds; no test files changed by this task.

**Tests**:
- No new test cases — this task is documentation-only. The existing `TestSessionRestorer_HydrateCommandFormat` (updated in Task 3-1) covers the function's emitted shape; no comment-asserting test is appropriate.
- Run `go test ./internal/restore/...` and `go test ./internal/tmux/...` after the comment edit — both must remain green (this is purely a sanity check that the comment edit did not corrupt the file).

**Edge Cases**: None — documentation-only change.

**Context**:
> From spec "Argument Quoting": "`RespawnPane`'s interface signature is unchanged — it continues to accept a single command-string argument."
>
> From spec "Inner Hook-Firing Wrapper Is Untouched": "This change drops the **outer** wrapper at the `respawn-pane` site. The **inner** `sh -c '<HOOK>; exec $SHELL'` constructed inside `execShellOrHookAndExit` (when an on-resume hook is registered) is unchanged. The two wrappers are independent — the outer wraps the helper invocation; the inner wraps the user's hook command." This task documents that boundary in the comment so future readers do not conflate them.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` — section "Fix 3: Wrapper Drop in `buildHydrateCommand`" → "Argument Quoting" (lines 209-212); "Inner Hook-Firing Wrapper Is Untouched" (lines 199-201).

## killed-sessions-resurrect-on-restart-3-3 | approved

### Task killed-sessions-resurrect-on-restart-3-3: Add integration test (real-tmux fixture): typed `exit` once in a restored pane closes the pane — `tmux list-panes` shows pane gone, not respawned with a fresh shell (AC5)

**Problem**: AC5 ("`exit` typed in a restored pane closes the pane on the first invocation. No orphan `sh` parent process under tmux for any restored pane.") is not directly covered by any existing automated test. The unit-level snapshot updated in Task 3-1 proves the emitted command string is bare, but cannot prove that under a real tmux server, typing `exit` once into a restored pane actually closes the pane (the pre-fix behaviour required two `exit`s because the wrapper sh ran `; exec $SHELL` after the inner shell exited). Without a regression test, a future reintroduction of the outer wrapper would silently re-break the double-`exit` user-visible behaviour.

**Solution**: Add a real-tmux integration test (`//go:build integration`) under `internal/restore/` that mirrors the scaffolding pattern of `internal/restore/integration_full_test.go` (`tmuxtest.Socket` + `restoretest.BuildPortalBinaryDir` + `restoretest.PrependPATH`). The test creates a single saved session with one pane, runs the capture → kill → restore cycle, drives the eager hydrate signal so the helper hands off cleanly to `$SHELL`, then sends `exit\n` via `tmux send-keys` to the restored pane and asserts (via polling `tmux list-panes -s -t <session>`) that the pane is gone within a short timeout. Three sub-tests cover the matrix from the task table edge cases: (a) restored pane with an on-resume hook registered, (b) restored pane without a hook, (c) `pgrep -fa "sh -c.*portal state hydrate"` returns no rows post-restore.

**Outcome**: A new integration test in `internal/restore/` (or a new file alongside `integration_full_test.go`) gated `//go:build integration`, with three sub-tests that pass post-fix and would fail pre-fix. The test runs under `go test -tags=integration ./internal/restore/...` and is skipped on `-short`.

**Do**:
- Add a new file `internal/restore/exit_closes_pane_integration_test.go` gated `//go:build integration` in package `restore_test`, importing `internal/restore`, `internal/restoretest`, `internal/state`, `internal/tmux`, `internal/tmuxtest`.
- Top of the test: `if testing.Short() { t.Skip("integration test; -short") }` and `tmuxtest.SkipIfNoTmux(t)`. Build the portal binary via `restoretest.BuildPortalBinaryDir(t)` and prepend its dir to `$PATH` via `restoretest.PrependPATH(t, binDir)`. Set `PORTAL_STATE_DIR` and `PORTAL_HOOKS_FILE` to per-test temp paths via `t.Setenv`.
- Sub-test 1 — `TestExitClosesRestoredPane_NoHook`:
  - Create a tmux session via `tmuxtest.New(t, "ptl-3-3-")` with a single window and a single pane in a fresh tempdir cwd.
  - Capture structure (`state.CaptureStructure`), persist via `state.EncodeIndex`, kill the live session, run `restore.SessionRestorer.Restore(...)` to rebuild the skeleton.
  - Drive the eager hydrate signal by writing one byte directly to the FIFO at `state.FIFOPath(stateDir, paneKey)` (mirror the byte-write pattern used in `integration_full_test.go`'s round-trip).
  - Poll `state.ListSkeletonMarkers` with a 2s timeout to confirm the helper has handed off (marker absent → helper has unset and exec'd `$SHELL`).
  - Send `exit\n` via `ts.Run(t, "send-keys", "-t", "<session>:0.0", "exit", "Enter")`.
  - Poll `ts.Run(t, "list-panes", "-s", "-t", "<session>", "-F", "#{window_index}:#{pane_index}")` with a 2s timeout; pass condition is the command exits non-zero (session gone) or returns empty output (no panes).
- Sub-test 2 — `TestExitClosesRestoredPane_WithHook`:
  - Identical scaffold but register an on-resume hook for the saved pane via the hooks store: write `{"<paneKey>": "echo hook-fired > /tmp/hook-fired-<random>"}` JSON into `PORTAL_HOOKS_FILE` before the restore. (Use the existing `internal/hooks` store API if convenient, or write the JSON directly.)
  - After restore + signal-drive, wait for the hook to have fired (poll for the marker file's existence with a short timeout). Then send `exit\n` and assert the pane closes — the inner `sh -c '<HOOK>; exec $SHELL'` exec chain is unaffected by the outer wrapper drop, so `exit` typed in the post-hook shell still closes the pane on first invocation.
- Sub-test 3 — `TestNoParkedShWrapperPostRestore`:
  - Identical scaffold (no hook). After restore + signal-drive + 2s settle, run `pgrep -fa "sh -c.*portal state hydrate"` via `exec.Command` and assert it returns no rows (exit code 1 = no matches; non-empty stdout = fail). This catches any future regression that reintroduces an outer wrapper, even if pane-close behaviour is somehow unaffected.
- Reuse `restoretest` and `tmuxtest` helpers throughout — do not duplicate scaffolding inline. If a needed helper (e.g. polling list-panes for pane-gone) does not exist, add it to `internal/restoretest/restoretest.go` rather than inlining in this test file.
- Run the test via `go test -tags=integration ./internal/restore/-run TestExitClosesRestoredPane`. Confirm all three sub-tests pass against the post-Task-3-1 codebase. Confirm sub-test 1 fails against pre-Task-3-1 code (revert `buildHydrateCommand` locally to verify the test catches the regression — restore after).

**Acceptance Criteria**:
- New file `internal/restore/exit_closes_pane_integration_test.go` exists, gated `//go:build integration`.
- Three sub-tests `TestExitClosesRestoredPane_NoHook`, `TestExitClosesRestoredPane_WithHook`, `TestNoParkedShWrapperPostRestore` are defined and pass under `go test -tags=integration ./internal/restore/...`.
- All three sub-tests skip cleanly under `-short` and when tmux is unavailable (`tmuxtest.SkipIfNoTmux`).
- Sub-tests 1 and 2 assert pane closure within a 2s polling window after sending `exit\n`.
- Sub-test 3 asserts `pgrep -fa "sh -c.*portal state hydrate"` returns zero matches.
- Default `go test ./...` (no integration tag) is unaffected — the new file's tests do not run on the default lane.
- The test, when run against the unchanged production code post-Task-3-1, passes deterministically across at least three consecutive invocations (no flakes).

**Tests** (test names in this file):
- `TestExitClosesRestoredPane_NoHook` — restored pane without an on-resume hook closes on first `exit`.
- `TestExitClosesRestoredPane_WithHook` — restored pane with an on-resume hook still closes on first `exit` (inner `sh -c '<HOOK>; exec $SHELL'` exec chain unaffected by the outer wrapper drop).
- `TestNoParkedShWrapperPostRestore` — `pgrep -fa "sh -c.*portal state hydrate"` returns no rows post-restore (no parked outer wrapper parent under tmux).

**Edge Cases**:
- Restored pane with on-resume hook registered: the inner `sh -c '<HOOK>; exec $SHELL'` envelope inside `execShellOrHookAndExit` is the **inner** wrapper, independent of the dropped outer wrapper. Sub-test 2 explicitly asserts that exit-on-first-`exit` is preserved when the hook chain is in play.
- Restored pane without a hook: `execShellOrHookAndExit` falls through to `execShellAndExit` which `syscall.Exec`s a bare `$SHELL`. Sub-test 1 covers this path.
- No parked `sh -c .*portal state hydrate` parent process under tmux post-restore: sub-test 3 enforces this directly via `pgrep` rather than relying on pane-close timing alone, so a regression that re-adds a wrapper but masks the double-`exit` symptom (e.g. via shell-builtin tricks) still fails the test.

**Context**:
> From spec section "Side Effects" (Fix 3): "Orphan `sh` parent eliminated. Every restored pane currently leaves a parked `sh` parent under tmux for the lifetime of the pane (observed at ~20 hours uptime in the investigation addendum). After this change, `portal state hydrate` is the pane's initial process and `syscall.Exec`s its replacement directly under tmux — no parked parent. Pane closes on first `exit`. Matches the documented `buildHydrateCommand` intent and aligns with non-restored panes, which already close on first `exit`."
>
> From spec "Manual Verification Protocol" additional checks: "`pgrep -fa "sh -c.*portal state hydrate"` returns no rows for any restored pane (no parked wrapper parents). `exit` typed once in a restored pane closes the pane (no second `exit` required)." This task automates both checks at the integration-test layer.
>
> Scaffolding precedent: `internal/restore/integration_full_test.go` is the canonical pattern for `restoretest.BuildPortalBinaryDir` + `tmuxtest.New` + `t.Setenv("PORTAL_STATE_DIR", …)` + capture/restore round-trips with direct FIFO byte-writes. Mirror its top-of-test setup verbatim where applicable.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` — section "Fix 3: Wrapper Drop in `buildHydrateCommand`" → "Side Effects" (lines 204-206); "Manual Verification Protocol" additional checks (lines 349-351); AC5 (line 228).

## killed-sessions-resurrect-on-restart-3-4 | approved

### Task killed-sessions-resurrect-on-restart-3-4: Execute Manual Verification Protocol on a real machine and record pre/post observations in the PR description (DoD item 3, AC6)

**Problem**: Spec § "Definition of Done" item 3 mandates the Manual Verification Protocol be executed once on a real machine with pre-fix and post-fix observations recorded in the PR description (or linked). AC6 ("WARN log volume drops to zero in the steady state") is also explicitly an observational gate verified via protocol step 2 — not a gated automated test. Without a deliverable task, the DoD is unsatisfied even if all automated tests pass.

**Solution**: Execute the 6-step Manual Verification Protocol from spec § "Manual Verification Protocol" on a real machine: (1) cold-start Portal so bootstrap step 5 reconstructs saved sessions; (2) inspect `~/.config/portal/state/portal.log` for the two WARN substrings; (3) inspect server options for stuck `@portal-skeleton-*` markers; (4) kill an affected session; (5) `portal open` again and confirm session stays gone; (6) verify on-resume hook ran in an affected pane. Plus the two additional Defect-D checks: `pgrep -fa "sh -c.*portal state hydrate"` returns no rows, and `exit` typed once closes a restored pane. Record each step's observation (pre-fix and post-fix) in the PR description.

**Outcome**: PR description contains a Manual Verification section with each protocol step's pre-fix and post-fix observation. AC6 is gated by the absence of the two named WARN substrings in `~/.config/portal/state/portal.log` after a clean cold-start with N≥2 saved sessions.

**Do**:
- Set up two side-by-side environments: one on `main` pre-fix, one on the integration branch post-fix. (Or run pre-fix observations once before merging Phase 1, and post-fix observations once after Phase 3 lands.)
- For each environment, walk through the 6 protocol steps and the two additional Defect-D checks. Record observations verbatim.
- Paste a structured Markdown table into the PR description with columns: `Step`, `Pre-fix observation`, `Post-fix observation`, `Pass/Fail`.
- AC6 specifically: confirm `~/.config/portal/state/portal.log` does **not** contain the substrings `WARN | hydrate | write fifo` or `WARN | hydrate | timeout waiting for signal` after a clean cold-start with N≥2 saved sessions.

**Acceptance Criteria**:
- [ ] PR description contains a "Manual Verification" section with all 6 protocol steps and the 2 additional Defect-D checks.
- [ ] Each step has a pre-fix and post-fix observation recorded.
- [ ] AC6 step 2: post-fix log contains zero occurrences of `WARN | hydrate | write fifo` and `WARN | hydrate | timeout waiting for signal`.
- [ ] AC5 additional check: post-fix `pgrep` returns zero rows; `exit` typed once closes the pane.
- [ ] Pre-fix observations were taken on a build that does not yet include Fixes 1/2/3 (e.g. `main` immediately before this work unit's branch was merged, or the integration branch with all three fixes reverted locally).

**Tests**: None — this task is observational verification per spec.

**Edge Cases**:
- N≥2 saved sessions are required to reproduce the multi-session bug behaviour pre-fix (single-saved-session users are unaffected).
- If a real machine is not available, the task can be deferred to a reviewer who has one — but DoD item 3 still requires it before merge.

**Context**:
> Spec § "Manual Verification Protocol" lines 338-351: the canonical 6-step protocol plus the two Defect-D checks.
>
> Spec § "Acceptance Criteria → Logging → AC6": "Verification is via the Manual Verification Protocol step 2 — observational, not a gated automated test."
>
> Spec § "Definition of Done" item 3: "The Manual Verification Protocol has been executed once on a real machine; pre-fix and post-fix observations recorded in the PR description (or linked)."

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Manual Verification Protocol", "Acceptance Criteria → AC6", "Definition of Done → item 3".

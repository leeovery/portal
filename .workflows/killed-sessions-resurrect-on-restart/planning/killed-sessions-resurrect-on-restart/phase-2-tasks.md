---
phase: 2
phase_name: Timeout-Path Recovery Corrections
total: 7
---

## killed-sessions-resurrect-on-restart-2-1 | approved

### Task killed-sessions-resurrect-on-restart-2-1: Flip TestHydrate_TimeoutDoesNotUnsetSkeletonMarker to assert marker-unset on timeout, then make handleHydrateTimeout call unsetSkeletonMarkerOrLog

**Problem**: `handleHydrateTimeout` (`cmd/state_hydrate.go:248-266`) deliberately leaves `@portal-skeleton-<paneKey>` set on the timeout path, encoded by the comment "marker stays set so the next attach re-signals". The marker can never be re-signaled because the same handler unlinks the FIFO immediately before, leaving no reader. The leaked marker indefinitely suppresses scrollback save for the affected pane (Symptom C) and feeds Symptom A's resurrection path. The current test `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` (line 1148) actively pins the wrong behaviour.

**Solution**: Flip the existing test's assertion (TDD red): rename it to `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` and assert the timeout handler issues the same `set-option -su @portal-skeleton-<paneKey>` argv that the file-missing path emits. Then implement the change in `handleHydrateTimeout` by calling `unsetSkeletonMarkerOrLog(cfg)` (the canonical wrapper around `state.UnsetSkeletonMarkerForFIFO`). The implementation mirrors `handleHydrateFileMissing` (`cmd/state_hydrate.go:298`) — same primitive, same logger, same soft-warning posture on failure.

**Outcome**: A 3-second hydrate timeout leaves the pane's `@portal-skeleton-*` marker unset. The daemon's `captureAndCommit` loop resumes capturing that pane on the next tick. The marker-unset call is observable in the recording commander's call log as a single `set-option -su @portal-skeleton-<paneKey>` invocation per timeout. The existing `TestHydrate_FileMissing_UnsetsSkeletonMarkerWithSetOptionSU` (line 913) is the verbatim shape to mirror.

**Do**:
- Edit `cmd/state_hydrate_test.go` line 1148: rename `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` to `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU`. Replace the negative assertion (loop searching for `set-option -su` and erroring if found) with the positive shape used at line 937: `want := []string{"set-option", "-su", "@portal-skeleton-tu__0.0"}` and a `found` loop that errors if absent.
- Edit `cmd/state_hydrate.go` `handleHydrateTimeout`: insert `unsetSkeletonMarkerOrLog(cfg)` between the existing warn-log call and the `return nil`. Place it at the position where the deleted comment 4 lives — it is the new step 4, replacing the "deliberately NO UnsetServerOption" stub. Do not touch the FIFO unlink (`os.Remove`) or the `cfg.Logger.Warn(...)` lines; they keep their current ordering.
- Confirm that `cfg.Client` is non-nil on every code path that reaches `handleHydrateTimeout` — it is, because `runHydrate` is constructed in `stateHydrateCmd.RunE` with `tmux.DefaultClient()`, and tests construct it via `tmux.NewClient(&recordingCommander{})`. No nil-guard is required at the handler.

**Acceptance Criteria**:
- [ ] `handleHydrateTimeout` invokes `unsetSkeletonMarkerOrLog(cfg)` on every entry, after the FIFO unlink and warn-log lines.
- [ ] The argv `["set-option", "-su", "@portal-skeleton-<paneKey>"]` appears exactly once in the recording commander's calls per timeout in the unit test.
- [ ] `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` passes; the old `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` no longer exists.
- [ ] `state.UnsetSkeletonMarkerForFIFO` failure is non-fatal: `unsetSkeletonMarkerOrLog`'s existing soft-warn-and-return contract is preserved (the wrapper logs via `cfg.Logger.Warn` and does not propagate the error). Subsequent exec fall-through still proceeds.
- [ ] paneKey derivation is via the existing seam (`state.PaneKeyFromFIFOPath`) — no new derivation logic added at the handler call-site.

**Tests**:
- `"TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU"` — flipped assertion; verifies argv shape `["set-option", "-su", "@portal-skeleton-tu__0.0"]` appears in the recording commander's `Calls` slice exactly once.
- `"set-option -su argv observed exactly once per timeout"` — count assertion inside the same test, defensive against duplicate invocations.
- `"unset-marker failure on timeout logs soft warning and does not block exec"` — extension test that injects a recording commander returning an error on `set-option -su`; verify `cfg.Logger` recorded a WARN line with substring `unset marker @portal-skeleton-` and the subsequent `execShellAndExit` (or `execShellOrHookAndExit` after task 2-3) was still invoked.
- `"paneKey is derived from FIFO basename via PaneKeyFromFIFOPath"` — assert the marker name in the issued argv matches `state.SkeletonMarkerPrefix + state.PaneKeyFromFIFOPath(cfg.FIFO)`.

**Edge Cases**:
- `state.UnsetSkeletonMarkerForFIFO` failure → `unsetSkeletonMarkerOrLog` logs `WARN | hydrate | unset marker <name>: <err>` (existing log shape at `cmd/state_hydrate.go:313`) and returns. The handler then returns nil and `runHydrate` falls through to the exec path (currently `execShellAndExit`; will be `execShellOrHookAndExit` after task 2-3). The exec must not be blocked by the unset failure.
- paneKey derivation: `cfg.FIFO` basename `hydrate-<paneKey>.fifo` → `state.PaneKeyFromFIFOPath` already handles this. No new code path; reuse the existing seam.
- `set-option -su` argv must be observed exactly once per timeout — the unit test scans `recordingCommander.Calls` and asserts `count == 1` (not just `>= 1`) to catch any double-invocation regression.

**Context**:
> Spec § "Fix 2: Timeout-Path Corrections in `handleHydrateTimeout`" → "Specific Changes" → item 1: "Unset `@portal-skeleton-<paneKey>` on timeout. The handler calls `unsetSkeletonMarkerOrLog` — the cmd-layer wrapper that internally invokes the `state.UnsetSkeletonMarkerForFIFO` primitive and logs a soft warning on failure. This is the canonical primitive used by `handleHydrateFileMissing`; tests reuse the same mock pattern by overriding the `state.UnsetSkeletonMarkerForFIFO` seam. Failure to unset is logged as a soft warning; does not block the shell exec."

> Spec § "Spec Supersession (Original Resurrection Spec)" — original line 838 of the built-in-session-resurrection spec ("Helper does NOT unset marker on FIFO timeout — next attach re-signals, retry happens naturally") is superseded by this task's behaviour because the FIFO is unlinked before any next attach could write to it.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Specific Changes → 1"

---

## killed-sessions-resurrect-on-restart-2-2 | approved

### Task killed-sessions-resurrect-on-restart-2-2: Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note

**Problem**: After task 2-1, the comment block at `cmd/state_hydrate.go:262-264` ("Deliberately NO UnsetServerOption — marker stays set so the next attach re-signals. / Deliberately NO 100ms sleep — nothing was dumped to settle.") encodes a non-deliverable invariant that has now been deliberately reversed (the marker is unset). Leaving the comment intact would mislead the next reader of the file.

**Solution**: Replace the multi-line "deliberately NO UnsetServerOption" comment with a single-line note that documents the post-task-2-1 recovery contract: "Marker unset above (recovery path matches handleHydrateFileMissing); the 100 ms settle sleep is still skipped — no scrollback was dumped, so there is no PTY-parser settle to wait on." The adjacent comments documenting the FIFO unlink (line 252-255) and the warn-log (line 258-259) stay verbatim — they describe behaviour that is unchanged.

**Outcome**: The handler's comment block reads coherently against the new behaviour. The 100 ms settle-sleep absence is still explicitly documented as deliberate so a future reader does not "fix" the absence by adding a sleep that has no scrollback to settle.

**Do**:
- Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// 100 ms settle sleep is deliberately skipped — nothing was dumped to settle, so there is no PTY-parser ingestion window to wait on.`
- Preserve verbatim the comments at lines 252-255 (FIFO unlink rationale) and lines 258-259 (warn-log purpose). Diff should touch only the deleted "Deliberately NO UnsetServerOption …" wording.
- This task is comment-only — no behavioural change. All existing tests must still pass without any test-file edit (the behavioural assertions are owned by tasks 2-1, 2-3, 2-4, 2-5).

**Acceptance Criteria**:
- [ ] The exact substring "marker stays set so the next attach re-signals" no longer appears anywhere in `cmd/state_hydrate.go`.
- [ ] The single-line replacement comment documents the 100 ms settle-sleep absence as deliberate.
- [ ] The FIFO-unlink comment at lines 252-255 is preserved verbatim.
- [ ] The warn-log comment at lines 258-259 is preserved verbatim.
- [ ] `go build ./...` succeeds and `go test ./cmd -run TestHydrate_Timeout` passes without any test-file edits in this task.

**Tests**:
- `"comment-only change: existing TestHydrate_Timeout* tests pass unchanged"` — regression posture; no new test added by this task.
- `"grep TestHydrate_Timeout under cmd/state_hydrate_test.go runs to completion"` — manual check; CI signal is the existing test suite.

**Edge Cases**:
- No behavioural change in this task — any test failure indicates the comment edit accidentally touched a code line; revert and re-edit.
- The 100 ms settle-sleep absence must remain documented as deliberate so a future drive-by edit does not introduce a sleep symmetrically with the success path.
- Adjacent FIFO-unlink and warn-log comments must be byte-identical post-edit (use `git diff` to verify only the deletion site is touched).

**Context**:
> Spec § "Fix 2 → Specific Changes → 3": "Remove the 'marker stays set so the next attach re-signals' comment at line 262. That comment encoded a non-deliverable invariant: the FIFO is unlinked at line 256 of the same handler before any subsequent attach could write to it, so 'next attach re-signals' was a no-op that just re-fired ENOENT. The comment is replaced with a one-line note explaining the recovery contract."

> Spec § "Fix 2 → Specific Changes → 4": "The 100 ms settle-sleep is preserved before exec — same posture as the success path, gives tmux time to settle the post-restore state before respawn-pane chains take over." (Note: applies to the success path's settle-sleep, not the timeout path's — the timeout path still skips the sleep because no scrollback was dumped. The replacement comment documents that distinction explicitly.)

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Specific Changes → 3 and 4"

---

## killed-sessions-resurrect-on-restart-2-3 | approved

### Task killed-sessions-resurrect-on-restart-2-3: Flip TestHydrate_Timeout_NeverFiresHookEvenIfRegistered into TestHydrate_Timeout_FiresHookWhenRegistered, then route runHydrate timeout fall-through through execShellOrHookAndExit

**Problem**: `runHydrate` (`cmd/state_hydrate.go:109`) routes the timeout fall-through through `execShellAndExit` (bare shell), not through `execShellOrHookAndExit` (hook-firing exec). The file-missing recovery path at line 141 already uses `execShellOrHookAndExit` — the divergence between the two recovery paths is the cause of Symptom B (on-resume hooks never fire on the previously-common timeout path). Test `TestHydrate_Timeout_NeverFiresHookEvenIfRegistered` (line 1423) actively pins the wrong contract.

**Solution**: Flip the existing test's intent (TDD red): rename `TestHydrate_Timeout_NeverFiresHookEvenIfRegistered` to `TestHydrate_Timeout_FiresHookWhenRegistered`. Update its assertions to mirror `TestHydrate_FileMissing_ExecsHookChainWhenHookRegistered` (line 1346): `exec.target == "/bin/sh"`, `exec.args == ["sh", "-c", "<HOOK>; exec /bin/zsh"]`. Then change `runHydrate`'s timeout-error-handler block from `execShellAndExit(cfg)` to `execShellOrHookAndExit(cfg)`. `cfg.HookKey` is already in scope at this call-site — no plumbing changes.

**Outcome**: When the timeout path fires and the pane has an `on-resume` hook registered for `cfg.HookKey`, the hook runs before the user's shell. When no hook is registered, the path still execs bare `$SHELL` (covered by task 2-4). The timeout and file-missing recovery paths now share the same exec contract.

**Do**:
- Edit `cmd/state_hydrate_test.go` line 1423: rename `TestHydrate_Timeout_NeverFiresHookEvenIfRegistered` to `TestHydrate_Timeout_FiresHookWhenRegistered`. Update the assertions to match the file-missing-hook test pattern at line 1346 — `exec.target == "/bin/sh"`, `exec.args == ["sh", "-c", "should-not-fire; exec /bin/zsh"]` (rename the seeded hook command to a positive-meaning string like `"echo hi"` to avoid lying-by-name). Remove the defensive "hook command leaked into argv" loop check at the bottom — it was guarding against the wrong outcome.
- Edit `cmd/state_hydrate.go` line 109 (the `cfg.HandleTimeout` post-call block, currently containing the comment "Timeout path falls through to a bare-shell exec — pane gets an empty $SHELL prompt; no hook firing on this path." and the call `execShellAndExit(cfg)`): replace with `execShellOrHookAndExit(cfg)` and update the comment to match the file-missing branch's wording at line 138 — e.g. `// Timeout path fires on-resume hooks per the timeout-recovery contract (handler has cleared the marker; lookup happens here, then exec).`
- Verify no `--hook-key` plumbing change is added: `cfg.HookKey` is already populated by `stateHydrateCmd.RunE` from the `--hook-key` flag (line 344, 372). `execShellOrHookAndExit` already reads `cfg.HookKey` (`cmd/state_hydrate.go:226`). The change is purely the swap of one exec function for another.

**Acceptance Criteria**:
- [ ] `runHydrate` timeout fall-through (after `cfg.HandleTimeout(cfg)` returns nil) calls `execShellOrHookAndExit(cfg)`, not `execShellAndExit(cfg)`.
- [ ] `TestHydrate_Timeout_FiresHookWhenRegistered` passes with `exec.target == "/bin/sh"` and `exec.args == ["sh", "-c", "<hook>; exec <shell>"]`.
- [ ] No new flag, parameter, or struct field is added to `runHydrate`, `hydrateConfig`, or `stateHydrateCmd`. `cfg.HookKey` threads through from existing scope.
- [ ] The file-missing-path tests (`TestHydrate_FileMissing_ExecsHookChainWhenHookRegistered`, `TestHydrate_FileMissing_ExecsBareShellWhenNoHookRegistered`) remain green — the change is symmetric.

**Tests**:
- `"TestHydrate_Timeout_FiresHookWhenRegistered"` — flipped assertion; mirrors `TestHydrate_FileMissing_ExecsHookChainWhenHookRegistered` shape.
- `"hook command sits in its own argv slot on timeout path"` — assert `exec.args[1] == "-c"` and `exec.args[2] == <hook>; exec <shell>`, matching line 1301-1304 verbatim shape.
- `"no --hook-key plumbing added to runHydrate"` — manual diff check; cfg.HookKey is already in scope.
- `"file-missing-path hook tests remain green after change"` — regression posture; existing tests at lines 1346, 1388 must pass unchanged.

**Edge Cases**:
- `cfg.HookKey` is threaded as-is from existing scope; no new flag added to `runHydrate`. `execShellOrHookAndExit` already handles the HookKey lookup internally.
- Exec target when hook registered: `sh -c '<HOOK>; exec $SHELL'` — same shape as file-missing path.
- The "hooks fire on timeout" semantic is safe even though scrollback was not replayed: per spec § "Hook-Firing Safety on Timeout", hooks are command-launchers (e.g. `claude --resume`) and do not depend on scrollback being on stdout. The visible terminal is blank rather than restored, but the hook command runs.

**Context**:
> Spec § "Fix 2 → Specific Changes → 2": "Route timeout fall-through through `execShellOrHookAndExit` (the hook-firing exec) instead of `execShellAndExit` (bare shell). The timeout and file-missing recovery paths now share the same exec contract: both unset the marker, both fire hooks if registered, both exec `$SHELL` if not. The current divergence between them is eliminated. **No new `--hook-key` plumbing is required.** `runHydrate` already holds the hook key in scope as `cfg.HookKey`; both `handleHydrateTimeout` and `handleHydrateFileMissing` recovery paths can call `execShellOrHookAndExit(cfg.HookKey)` symmetrically."

> Spec § "Spec Supersession" — original line 873 of the built-in-session-resurrection spec ("Resume hooks fire only from inside the hydrate helper's exec chain, at the end of successful hydration") is refined: hooks fire on any non-fatal terminal path, including timeout recovery.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Specific Changes → 2"

---

## killed-sessions-resurrect-on-restart-2-4 | approved

### Task killed-sessions-resurrect-on-restart-2-4: Unit test: runHydrate timeout fall-through with no registered hook still execs bare $SHELL via execShellOrHookAndExit

**Problem**: After task 2-3 routes the timeout fall-through through `execShellOrHookAndExit`, the no-hook degradation paths must be re-verified for the timeout branch. `execShellOrHookAndExit` (`cmd/state_hydrate.go:221`) has three degrade-to-bare-shell branches: nil HookStore, lookup-not-found, and lookup-error (with a single WARN). The existing file-missing path tests (lines 1388, 1459) cover these for file-missing; the timeout path needs equivalent coverage so a future regression in `execShellOrHookAndExit` does not silently re-introduce Symptom B for hookless panes.

**Solution**: Add three new unit tests in `cmd/state_hydrate_test.go` mirroring the file-missing branch coverage, but using `timeoutCfg` (line 1056) as the config builder so they exercise the timeout fall-through. Each test injects a different degenerate HookStore configuration and asserts `exec.target` is bare `$SHELL` (e.g. `/bin/zsh`) and `exec.args == ["/bin/zsh"]`.

**Outcome**: The timeout-recovery exec contract now has explicit test coverage for all three no-hook branches: nil store, store-with-no-entry-for-key, and store-that-returns-error-on-lookup. Lookup errors produce a single WARN log line and do not produce a panic or exec failure.

**Do**:
- Add `TestHydrate_Timeout_NoHookStore_ExecsBareShell` — replicate `TestHydrate_FileMissing_ExecsBareShellWhenNoHookRegistered` (line 1388) shape but use `timeoutCfg` and leave `cfg.HookStore = nil` (override post-construction). Assert `exec.target == "/bin/zsh"`, `exec.args == ["/bin/zsh"]`. Use `t.Setenv("SHELL", "/bin/zsh")` and `instantTimeoutOpenFIFO`.
- Add `TestHydrate_Timeout_LookupNotFound_ExecsBareShell` — `seedHookStore(t, dir, map[string]map[string]string{})` (empty store). Assert bare-shell exec.
- Add `TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning` — replicate `TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning` (line 1459) shape. Make `hooks.json` a directory so `hooks.LookupOnResume` returns EISDIR. Capture log via `state.OpenLogger(logPath, false)` (matching pattern at line 1173) and assert (a) bare-shell exec and (b) log file contains a single line with substring `lookup on-resume hook for` (the WARN line at `cmd/state_hydrate.go:228`).
- All three tests use `timeoutCfg` to construct the base config; the only deviations are the HookStore configuration and the optional logger injection.

**Acceptance Criteria**:
- [ ] Three new tests added under `cmd/state_hydrate_test.go`: `TestHydrate_Timeout_NoHookStore_ExecsBareShell`, `TestHydrate_Timeout_LookupNotFound_ExecsBareShell`, `TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning`.
- [ ] All three tests pass against the post-task-2-3 implementation.
- [ ] The lookup-error test asserts exactly one WARN log line is produced (count, not just presence) — defensive against duplicate logging.
- [ ] None of the three tests use `t.Parallel()` (forbidden by `CLAUDE.md` "Build & Test").

**Tests**:
- `"TestHydrate_Timeout_NoHookStore_ExecsBareShell"` — nil HookStore degrades to bare shell.
- `"TestHydrate_Timeout_LookupNotFound_ExecsBareShell"` — store lookup returns `(_, false, nil)` → bare shell.
- `"TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning"` — store lookup returns `(_, _, err)` → bare shell + single WARN.

**Edge Cases**:
- nil HookStore: `execShellOrHookAndExit` short-circuits at line 222 — no log line, exec bare `$SHELL`.
- Lookup not found: `hooks.LookupOnResume` returns `("", false, nil)` — no log line, exec bare `$SHELL`.
- Lookup error: `hooks.LookupOnResume` returns `("", false, err)` — single WARN line via `cfg.Logger.Warn(state.ComponentHydrate, "lookup on-resume hook for %s: %v", cfg.HookKey, err)`. Defensive against double-logging in case a future change accidentally calls `cfg.Logger.Warn` from a sibling call-site.

**Context**:
> `cmd/state_hydrate.go:221-239` — `execShellOrHookAndExit` body. The three degrade-to-bare-shell branches:
>   1. `if cfg.HookStore == nil { execShellAndExit(cfg); return }`
>   2. `if err != nil { cfg.Logger.Warn(state.ComponentHydrate, "lookup on-resume hook for %s: %v", cfg.HookKey, err); execShellAndExit(cfg); return }`
>   3. `if !found { execShellAndExit(cfg); return }`

> `cmd/state_hydrate_test.go:1459-1505` — `TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning` is the canonical lookup-error test shape; the timeout-path version mirrors it with `timeoutCfg` substituted for the manual hydrateConfig.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Specific Changes → 2" (timeout and file-missing share the same exec contract — must include the no-hook degradation paths)

---

## killed-sessions-resurrect-on-restart-2-5 | approved

### Task killed-sessions-resurrect-on-restart-2-5: Unit test: handleHydrateTimeout preserves the 100 ms settle-sleep absence and FIFO-unlink ordering

**Problem**: Task 2-1 inserts an `unsetSkeletonMarkerOrLog` call into `handleHydrateTimeout`, and task 2-2 rewrites the comment block. Two pre-existing invariants of the timeout handler must remain intact post-edit: (a) no 100 ms settle sleep — nothing was dumped, so there is no PTY-parser settle to wait on, and (b) `os.Remove(cfg.FIFO)` tolerates a missing FIFO silently. A future drive-by edit could accidentally re-introduce a sleep symmetric with the success path, or replace `os.Remove` with a stricter primitive. Existing test `TestHydrate_TimeoutDoesNotSleep100ms` (line 1114) covers (a) at the `runHydrate` boundary; existing test `TestHydrate_TimeoutToleratesMissingFIFOSilently` (line 1237) covers (b) loosely. Neither asserts the relative ordering of the unset call vs. the FIFO unlink and exec fall-through.

**Solution**: Add one new unit test `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` that exercises `handleHydrateTimeout` directly (not through `runHydrate`) and asserts: (a) handler elapsed time is well under `hydrateSettleSleep` (100 ms); (b) `os.Remove(cfg.FIFO)` on a non-existent FIFO returns nil from the handler; (c) the marker-unset call (`set-option -su`) is observed in the recording commander's call log before the handler returns (i.e. before `runHydrate` would issue the exec fall-through). Combined with the existing `TestHydrate_TimeoutPathInvokesHandleTimeout` (which asserts handler is called before the exec), this establishes the full ordering invariant.

**Outcome**: The 100 ms-absence and ordering invariants are pinned by an explicit test that fails if a future change introduces a sleep, swaps `os.Remove` for an erroring primitive, or reorders the unset call after the exec fall-through.

**Do**:
- Add `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` in `cmd/state_hydrate_test.go`. Construct a minimal `hydrateConfig` directly (not via `timeoutCfg` — this test calls the handler in isolation, not through `runHydrate`):
  - `dir := t.TempDir()`; `fifo := filepath.Join(dir, "hydrate-ord__0.0.fifo")` — do NOT mkfifo (drives the missing-FIFO branch).
  - `cmder := &recordingCommander{}`; `cfg := hydrateConfig{FIFO: fifo, HookKey: "ord:0.0", Stdout: io.Discard, Client: tmux.NewClient(cmder)}`.
- Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
- Assert `err == nil`.
- Assert `elapsed < 50*time.Millisecond` (well under `hydrateSettleSleep = 100*time.Millisecond`). Use 50 ms not 100 ms so a sleep regression cannot squeak past with timing jitter.
- Assert `_, statErr := os.Stat(fifo); errors.Is(statErr, os.ErrNotExist)` — handler tolerated the absent FIFO and did not return an error.
- Assert the marker-unset argv `["set-option", "-su", "@portal-skeleton-ord__0.0"]` is present in `cmder.Calls`. The handler returns nil before the exec fall-through is reached, so the call log captures only the marker-unset.

**Acceptance Criteria**:
- [ ] `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` passes.
- [ ] Handler elapsed time on a missing-FIFO input is `< 50 ms`.
- [ ] `os.Remove` on a non-existent FIFO is silent — handler returns nil.
- [ ] `set-option -su @portal-skeleton-<paneKey>` appears in the recording commander's calls before the handler returns.
- [ ] Test does not use `t.Parallel()`.

**Tests**:
- `"TestHydrate_TimeoutHandler_OrderingAndTimingInvariants"` — combined timing + ordering + FIFO-tolerance assertion.
- `"elapsed time on timeout handler stays well under hydrateSettleSleep"` — sub-50 ms bound, defensive against any sleep introduction.
- `"os.Remove(cfg.FIFO) tolerates missing FIFO silently"` — pre-existing in `TestHydrate_TimeoutToleratesMissingFIFOSilently` (line 1237) at the `runHydrate` boundary; this task adds the handler-level direct assertion.
- `"marker-unset call ordered before exec fall-through"` — the handler must issue `set-option -su` and return; only then does `runHydrate` invoke `execShellOrHookAndExit`. Verified by absence of any exec-related calls in the handler's recording commander log.

**Edge Cases**:
- Elapsed handler time on missing-FIFO must stay well under `hydrateSettleSleep` (100 ms) — bound at 50 ms in the assertion.
- `os.Remove(cfg.FIFO)` on a path that does not exist must return silently — already true (line 256 uses `_ = os.Remove(...)`).
- Marker-unset call ordering: `unsetSkeletonMarkerOrLog` must execute before the handler returns. The handler does not invoke any exec primitive — that is `runHydrate`'s responsibility post-handler-return.

**Context**:
> `cmd/state_hydrate.go:248-266` — pre-task-2-1 `handleHydrateTimeout` body. The post-task-2-1 ordering is: (1) reset preamble → (2) `os.Remove(cfg.FIFO)` → (3) warn-log → (4) `unsetSkeletonMarkerOrLog(cfg)` → return nil. The 100 ms sleep is deliberately absent per `cmd/state_hydrate.go:264` (post-edit, the absence is documented by the single-line replacement comment from task 2-2).

> `cmd/state_hydrate_test.go:1114-1132` — `TestHydrate_TimeoutDoesNotSleep100ms` is the canonical no-sleep timing assertion at the `runHydrate` boundary. This task adds a handler-direct equivalent so the invariant is pinned even if a future change moves the sleep call into the handler itself.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Specific Changes → 4" (100 ms settle-sleep posture)

---

## killed-sessions-resurrect-on-restart-2-6 | approved

### Task killed-sessions-resurrect-on-restart-2-6: Integration test (real tmux): cold-start with non-attached saved session + registered on-resume hook fires end-to-end (AC2)

**Problem**: AC2 ("On-resume hooks registered via `portal hooks set --on-resume "<cmd>"` fire end-to-end on cold-start for every restored pane that has a hook registered, regardless of which session the user attached to") is the user-visible verification that Symptom B is closed. The unit tests added by tasks 2-3 and 2-4 cover the timeout-path exec contract in isolation, but do not exercise the full skeleton-restore → eager-signal → hydrate-helper → hook-fire chain against a real tmux server with N≥2 saved sessions where the hook is on the non-attached session. Existing `cmd/bootstrap/reboot_roundtrip_test.go` already covers the attached-session hook-fire case (`verifyHookFiredOnce`); the new integration test specifically exercises the previously-broken non-attached case.

**Solution**: Add a new integration test `TestPhase2_HookFiresOnNonAttachedSession_AC2` under `cmd/bootstrap/` (gated by `//go:build integration` consistent with the `phase5_marker_suppression_integration_test.go` pattern). The test seeds a saved sessions.json with N=2 entries (one of them carrying a hook in `hooks.json`), runs the orchestrator against an isolated tmux instance via `internal/tmuxtest`, and verifies the hook ran in the restored pane on the non-attached session — observed via a sentinel side-effect file written by the hook command.

**Outcome**: AC2 is verified end-to-end on a real tmux server. The test passes regardless of whether the eager-signaling step (Phase 1) cleared markers pre-timeout (the success path) or whether the timeout path fired (defensive — the test should still pass because timeout fall-through now fires hooks per task 2-3). This dual-path safety is the integration-level confirmation that Phase 1 + Phase 2 together close Symptom B.

**Do**:
- Create `cmd/bootstrap/phase2_hook_fire_integration_test.go` with the build tag `//go:build integration` (mirror `phase5_marker_suppression_integration_test.go`'s build tag and package layout). Use `package bootstrap_test`.
- Reuse the harness pattern from `cmd/bootstrap/reboot_roundtrip_test.go`:
  - `tmuxtest.SkipIfNoTmux(t)` then `ts := tmuxtest.New(t, "ptl-p2-")` for an isolated socket.
  - `client := ts.Client(); client.EnsureServer()`.
  - Write a sentinel side-effect path: `sentinelDir := t.TempDir(); sentinelFile := filepath.Join(sentinelDir, "hook-fired")`.
  - Seed a `sessions.json` snapshot with two saved sessions: `alpha` (the session the bootstrap will attach to — first in list) and `beta` (the non-attached session). Place a single pane in `beta` whose structural hook key is recorded.
  - Seed `hooks.json` via `hooks.NewStore(...).Set(betaHookKey, "on-resume", fmt.Sprintf("touch %s", sentinelFile))`. Use `t.Setenv("PORTAL_CONFIG_HOME", ...)` to point the helper at the test's hooks.json.
  - Build the orchestrator via `buildIntegrationOrchestrator` (existing helper used by `phase5_integration_test.go`) with real `Saver`, `Restore`, `Hooks`, and the new `EagerSignaler` from Phase 1.
  - Invoke `o.Run(context.Background())`.
- After bootstrap returns, poll for the sentinel file with a 2-second budget (mirroring AC1's poll-pattern used by task 1-6): `for deadline := time.Now().Add(2*time.Second); time.Now().Before(deadline); time.Sleep(50*time.Millisecond) { if _, err := os.Stat(sentinelFile); err == nil { return } }; t.Fatalf("hook did not fire within 2s")`.
- Assert via `ts.Run(t, "list-sessions", "-F", "#{session_name}")` that both `alpha` and `beta` are alive — the hook firing implies the helper exec'd into the chained `sh -c '<HOOK>; exec $SHELL'`, which keeps the pane alive.
- Note: the hook fires in the restored pane on `beta`. Whether it fires via the eager-signal success path (Phase 1) or the timeout fall-through (Phase 2 task 2-3) is irrelevant to the test — both produce the same sentinel-file outcome. The test is deliberately oblivious to which path delivered it. This is captured in a comment at the top of the test.

**Acceptance Criteria**:
- [ ] `TestPhase2_HookFiresOnNonAttachedSession_AC2` passes against a real tmux instance via `internal/tmuxtest`.
- [ ] Sentinel file is created within 2 seconds of `o.Run` returning.
- [ ] Both saved sessions (`alpha` and `beta`) appear in `tmux list-sessions` post-bootstrap.
- [ ] Test is gated by `//go:build integration` and skips cleanly if `tmux` is not on PATH.
- [ ] Test does not use `t.Parallel()`.
- [ ] Test passes when eager-signaling (Phase 1) has already cleared the marker pre-timeout (the helper hits the success path and fires the hook there) AND when the timeout path fires (the fall-through fires the hook per task 2-3) — the test does not pin which path delivered the side-effect.

**Tests**:
- `"TestPhase2_HookFiresOnNonAttachedSession_AC2"` — end-to-end hook-fire verification on the previously-broken non-attached session.
- `"hook stdout/effect observable in restored pane"` — sentinel file presence is the observable; `touch <path>` is the simplest observable side-effect that does not depend on terminal capture.
- `"test still passes when eager-signaling has already cleared markers pre-timeout"` — covered by the test's path-agnostic design; documented at the top of the file as a deliberate property.
- `"both saved sessions alive post-bootstrap"` — guards against accidental session loss during restore.

**Edge Cases**:
- N≥2 saved sessions with the hook on the non-attached session — the original Symptom B reproduction shape. The eager-signal step (Phase 1) drives hydration on the non-attached session; the hook fires on the success path. If eager signaling fails for any reason, the timeout fall-through (task 2-3) fires the hook on the recovery path. Either outcome produces the sentinel file.
- Hook stdout/effect must be observable in the restored pane without requiring a real PTY attach — `touch <path>` writes to the filesystem and is observable from the test goroutine. Avoid hooks that require terminal interaction.
- Test must still pass if Phase 1 has cleared the marker before the helper hits its 3 s timeout — the success path is the most likely outcome post-Phase-1; the timeout path is the defensive fallback. Both paths fire the hook (task 2-3 contract).

**Context**:
> Spec § "Test Plan → Integration → End-to-end hook firing on cold-start": "Register an on-resume hook for a non-attached saved session. Cold-start. Assert the hook ran in the restored pane."

> `cmd/bootstrap/reboot_roundtrip_test.go` lines 185-220 — the existing pattern for seeding `hooks.json` via `hooks.NewStore(...).Set(savedHookKey, "on-resume", hookCmd)` and observing a sentinel side-effect file written by the hook command. Reuse the same shape for the non-attached-session test.

> `cmd/bootstrap/phase5_marker_suppression_integration_test.go` — canonical example of a `//go:build integration`-gated test in the `bootstrap_test` package that wires real Saver+Restore against `tmuxtest`. Reuse the build-tag and harness pattern.

> Acceptance Criterion AC2 from the specification — this test is the canonical AC2 verification.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → AC2" and § "Test Plan → Integration → End-to-end hook firing on cold-start"

---

## killed-sessions-resurrect-on-restart-2-7 | approved

### Task killed-sessions-resurrect-on-restart-2-7: Record spec supersession of built-in-session-resurrection lines 838 and 873 in this work unit's planning notes (no in-place edit of the original spec)

**Problem**: This phase's behavioural changes (marker unset on timeout; hooks fire on timeout) deliberately supersede two invariants from `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`. The original spec must NOT be edited in place (per spec § "Fix 2 → Spec Supersession": "The original session-resurrection spec is not modified in place; the supersession is recorded here as the canonical updated semantic for the timeout path"). A small markdown note in this work unit's planning directory makes the supersession discoverable to future readers of the original spec without rewriting history.

**Solution**: Author a single markdown file `phase-2-supersession.md` under `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/` that quotes the two original-spec lines verbatim, states the replaced semantic alongside each, and links the supersession back to AC2 and AC6 of this work unit's spec. The file is implementation deliverable for this task — task description specifies its content shape; the implementer writes it.

**Outcome**: A discoverable supersession record exists in the planning directory. Anyone reading the original built-in-session-resurrection spec who searches the repo for the quoted invariants finds this note and learns that the post-Phase-2 semantics differ. The original spec file is byte-identical pre/post this task.

**Do**:
- Create `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/phase-2-supersession.md` with the following content shape:
  - **Title**: `# Spec Supersession: built-in-session-resurrection (Phase 2)`.
  - **One-paragraph preamble**: state that this work unit (`killed-sessions-resurrect-on-restart`) supersedes two invariants from the original `built-in-session-resurrection` specification, that the original file is intentionally not edited, and link the original path: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`.
  - **Section "Superseded Invariant 1 (Original line 838)"**:
    - Quote the original verbatim: `"Helper does NOT unset marker on FIFO timeout — next attach re-signals, retry happens naturally."`
    - State the replaced semantic: `Helper unsets the @portal-skeleton-<paneKey> marker on FIFO timeout via unsetSkeletonMarkerOrLog. The original "next attach re-signals" promise was non-deliverable: the FIFO is unlinked at the same site, leaving no reader for any subsequent signal. Leaving the marker set fed Symptom C (stuck markers suppress scrollback save indefinitely).`
    - Reference: link to AC2 and AC6 of the killed-sessions-resurrect-on-restart spec (`.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → AC2 / AC6") and to phase-2-tasks.md tasks 2-1 (marker unset) and 2-3 (hook fires).
  - **Section "Superseded Invariant 2 (Original line 873)"**:
    - Quote the original verbatim: `"Resume hooks fire only from inside the hydrate helper's exec chain, at the end of successful hydration."`
    - State the refined semantic: `Resume hooks fire from inside the hydrate helper's exec chain on any non-fatal terminal path — successful hydration, file-missing recovery, and timeout recovery. The original phrasing reflected an assumption that timeout was an exceptional condition; in practice (pre-Fix 1) it was the steady state, which made the "hooks unsafe on timeout" rationale incoherent. With Fix 1 (eager signaling) in place, timeout is genuinely rare; when it fires, the recovery path matches file-missing's already-tested behaviour.`
    - Reference: same AC2/AC6 and tasks-2-3/2-4 links as above.
  - **Section "Verification Trail"**:
    - One-line entries linking to the unit tests that pin the new semantics: `cmd/state_hydrate_test.go::TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` (task 2-1), `TestHydrate_Timeout_FiresHookWhenRegistered` (task 2-3), and the integration test `cmd/bootstrap/phase2_hook_fire_integration_test.go::TestPhase2_HookFiresOnNonAttachedSession_AC2` (task 2-6).
- Do NOT touch `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`. Verify with `git diff` that the only file added by this task is `phase-2-supersession.md`.

**Acceptance Criteria**:
- [ ] `phase-2-supersession.md` exists at `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/phase-2-supersession.md`.
- [ ] Both original-spec lines (838 and 873) are quoted verbatim in fenced quote blocks.
- [ ] Each superseded invariant has its replaced semantic stated alongside.
- [ ] Each invariant's section links back to AC2 and AC6 of this work unit's specification.
- [ ] The original file `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` is byte-identical pre/post this task — verified via `git diff`.
- [ ] The Verification Trail section links to the three tests added by tasks 2-1, 2-3, and 2-6.

**Tests**:
- `"phase-2-supersession.md exists at the canonical path"` — manual verification; this task is documentation, not code.
- `"original built-in-session-resurrection spec is byte-identical post-task"` — `git diff .workflows/built-in-session-resurrection/specification/` shows no changes.
- `"both quoted invariants appear verbatim in the supersession note"` — `grep -F "Helper does NOT unset marker on FIFO timeout"` and `grep -F "Resume hooks fire only from inside the hydrate helper's exec chain"` against the new file return matches.
- `"supersession note links AC2 and AC6"` — search the file for substrings `AC2` and `AC6`.

**Edge Cases**:
- Original spec file untouched — verify with `git diff` that the only file added by this task is `phase-2-supersession.md`.
- Supersession note links Phase 2 acceptance back to AC2 and AC6 — both must appear as explicit substrings in the note (not transitively via the spec link).
- Lines 838 and 873 quoted verbatim — copy from the file, do not paraphrase. The replaced semantic is stated alongside each quote, not embedded in the quote block.

**Context**:
> Spec § "Fix 2 → Spec Supersession (Original Resurrection Spec)": "This change deliberately supersedes two invariants from `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`: ... The original session-resurrection spec is not modified in place; the supersession is recorded here as the canonical updated semantic for the timeout path."

> Spec § "Acceptance Criteria → Spec Conformance": Phase 2's behavioural change deliberately deviates from line 873 of the original spec; this supersession note is the discoverable record of that deviation for future readers.

> `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` lines 838 and 873 — the two invariants quoted verbatim in the new supersession note.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Spec Supersession (Original Resurrection Spec)"

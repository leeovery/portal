---
phase: 2
phase_name: Timeout-Path Recovery Corrections
total: 6
---

## killed-sessions-resurrect-on-restart-2-1 | approved

### Task killed-sessions-resurrect-on-restart-2-1: Flip TestHydrate_TimeoutDoesNotUnsetSkeletonMarker to assert marker-unset on timeout, then make handleHydrateTimeout call unsetSkeletonMarkerOrLog

**Problem**: `handleHydrateTimeout` (`cmd/state_hydrate.go:248-266`) deliberately leaves `@portal-skeleton-<paneKey>` set on the timeout path, encoded by the comment "marker stays set so the next attach re-signals". The marker can never be re-signaled because the same handler unlinks the FIFO immediately before, leaving no reader. The leaked marker indefinitely suppresses scrollback save for the affected pane (Symptom C) and feeds Symptom A's resurrection path. The current test `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` (line 1148) actively pins the wrong behaviour.

**Solution**: Flip the existing test's assertion (TDD red): rename it to `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` and assert the timeout handler issues the same `set-option -su @portal-skeleton-<paneKey>` argv that the file-missing path emits. Then implement the change in `handleHydrateTimeout` by calling `unsetSkeletonMarkerOrLog(cfg)` (the canonical wrapper around `state.UnsetSkeletonMarkerForFIFO`). The implementation mirrors `handleHydrateFileMissing` (`cmd/state_hydrate.go:298`) — same primitive, same logger, same soft-warning posture on failure.

**Outcome**: A 3-second hydrate timeout leaves the pane's `@portal-skeleton-*` marker unset. The daemon's `captureAndCommit` loop resumes capturing that pane on the next tick. The marker-unset call is observable in the recording commander's call log as a single `set-option -su @portal-skeleton-<paneKey>` invocation per timeout. The existing `TestHydrate_FileMissing_UnsetsSkeletonMarkerWithSetOptionSU` (line 913) is the verbatim shape to mirror.

**Do**:
- Edit `cmd/state_hydrate_test.go` line 1148: rename `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` to `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU`. Replace the negative assertion (loop searching for `set-option -su` and erroring if found) with the positive shape used at line 937: `want := []string{"set-option", "-su", "@portal-skeleton-tu__0.0"}` and a `found` loop that errors if absent.
- Edit `cmd/state_hydrate.go` `handleHydrateTimeout`: insert `unsetSkeletonMarkerOrLog(cfg)` between the existing warn-log call and the `return nil`. Place it at the position where the deleted comment 4 lives — it is the new step 4, replacing the "deliberately NO UnsetServerOption" stub. Do not touch the FIFO unlink (`os.Remove`) or the `cfg.Logger.Warn(...)` lines; they keep their current ordering.
- Edit `cmd/state_hydrate.go` `runHydrate`'s timeout branch (currently lines 103-110): insert `time.Sleep(hydrateSettleSleep)` between the `cfg.HandleTimeout(cfg)` call and the `execShellAndExit(cfg)` call (i.e. after the handler returns nil, before the exec fall-through). Per spec § "Fix 2 → Specific Changes → 4" the timeout fall-through must pay the same 100 ms settle sleep as the success path. The sleep lives in `runHydrate`, not in the handler, because `runHydrate` is the single owner of the post-handler exec sequence (mirrors how the success path's `time.Sleep(hydrateSettleSleep)` at line 171 lives in `runHydrate`'s straight-line body, not in a handler).
- Confirm that `cfg.Client` is non-nil on every code path that reaches `handleHydrateTimeout` — it is, because `runHydrate` is constructed in `stateHydrateCmd.RunE` with `tmux.DefaultClient()`, and tests construct it via `tmux.NewClient(&recordingCommander{})`. No nil-guard is required at the handler.

**Acceptance Criteria**:
- [ ] `handleHydrateTimeout` invokes `unsetSkeletonMarkerOrLog(cfg)` on every entry, after the FIFO unlink and warn-log lines.
- [ ] The argv `["set-option", "-su", "@portal-skeleton-<paneKey>"]` appears exactly once in the recording commander's calls per timeout in the unit test.
- [ ] `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` passes; the old `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` no longer exists.
- [ ] `state.UnsetSkeletonMarkerForFIFO` failure is non-fatal: `unsetSkeletonMarkerOrLog`'s existing soft-warn-and-return contract is preserved (the wrapper logs via `cfg.Logger.Warn` and does not propagate the error). Subsequent exec fall-through still proceeds.
- [ ] paneKey derivation is via the existing seam (`state.PaneKeyFromFIFOPath`) — no new derivation logic added at the handler call-site.
- [ ] `runHydrate`'s timeout branch pays `time.Sleep(hydrateSettleSleep)` between `cfg.HandleTimeout` returning nil and `execShellAndExit(cfg)` — same posture as the success-path settle sleep at line 171. Verified by a recorded elapsed time of at least `hydrateSettleSleep` at the `runHydrate` boundary on the timeout path.
- [ ] The pre-existing `TestHydrate_TimeoutDoesNotSleep100ms` (cmd/state_hydrate_test.go:1114) is renamed/flipped in this same task to `TestHydrate_Timeout_PreservesSettleSleepBeforeExec` and asserts `elapsed >= hydrateSettleSleep` instead of the previous `elapsed < 50 ms`. This is the first task that changes the timeout-path elapsed-time invariant; no later task can ship green while the old assertion still pins the opposite invariant.

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

**Solution**: Replace the "deliberately NO UnsetServerOption — marker stays set so the next attach re-signals" comment block with a single-line recovery-contract note immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, e.g. `// Recovery path matches handleHydrateFileMissing: marker unset above, then runHydrate execs through execShellOrHookAndExit with the 100 ms settle sleep preserved (same posture as the success path — gives tmux time to settle the post-restore state before respawn-pane chains take over).` The adjacent comments documenting the FIFO unlink (line 252-255) and the warn-log (line 258-259) stay verbatim — they describe behaviour that is unchanged.

**Outcome**: The handler's comment block reads coherently against the new behaviour. The 100 ms settle-sleep is preserved on the exec fall-through (per spec § "Fix 2 → Specific Changes → 4") — same posture as the success path; the comment documents that the recovery contract matches `handleHydrateFileMissing`, with the sleep paid before exec by `runHydrate`'s shared post-handler block.

**Do**:
- Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// Recovery path matches handleHydrateFileMissing: marker unset above; the 100 ms settle sleep is paid by runHydrate before exec (inserted in task 2-1, mirrors the success-path sleep posture).` This task is comment-only — task 2-1 already inserted the sleep call in `runHydrate`'s timeout branch, so this task does not touch any code lines.
- Preserve verbatim the comments at lines 252-255 (FIFO unlink rationale) and lines 258-259 (warn-log purpose). Diff should touch only the deleted "Deliberately NO UnsetServerOption …" wording.
- This task is comment-only — no behavioural change. All existing tests must still pass without any test-file edit (the behavioural assertions are owned by tasks 2-1, 2-3, 2-4, 2-5).

**Acceptance Criteria**:
- [ ] The exact substring "marker stays set so the next attach re-signals" no longer appears anywhere in `cmd/state_hydrate.go`.
- [ ] The single-line replacement comment documents that the recovery contract matches `handleHydrateFileMissing` and that the 100 ms settle-sleep is preserved before exec by `runHydrate`'s shared post-handler block (same posture as the success path).
- [ ] The FIFO-unlink comment at lines 252-255 is preserved verbatim.
- [ ] The warn-log comment at lines 258-259 is preserved verbatim.
- [ ] `go build ./...` succeeds and `go test ./cmd -run TestHydrate_Timeout` passes without any test-file edits in this task.

**Tests**:
- `"comment-only change: existing TestHydrate_Timeout* tests pass unchanged"` — regression posture; no new test added by this task.
- `"grep TestHydrate_Timeout under cmd/state_hydrate_test.go runs to completion"` — manual check; CI signal is the existing test suite.

**Edge Cases**:
- No behavioural change in this task — any test failure indicates the comment edit accidentally touched a code line; revert and re-edit.
- The 100 ms settle-sleep is preserved on the exec fall-through per spec § "Fix 2 → Specific Changes → 4" — same posture as the success path; `runHydrate`'s shared post-handler block owns the sleep call.
- Adjacent FIFO-unlink and warn-log comments must be byte-identical post-edit (use `git diff` to verify only the deletion site is touched).

**Context**:
> Spec § "Fix 2 → Specific Changes → 3": "Remove the 'marker stays set so the next attach re-signals' comment at line 262. That comment encoded a non-deliverable invariant: the FIFO is unlinked at line 256 of the same handler before any subsequent attach could write to it, so 'next attach re-signals' was a no-op that just re-fired ENOENT. The comment is replaced with a one-line note explaining the recovery contract."

> Spec § "Fix 2 → Specific Changes → 4": "The 100 ms settle-sleep is preserved before exec — same posture as the success path, gives tmux time to settle the post-restore state before respawn-pane chains take over." This applies to **all** post-handler exec fall-throughs (success, file-missing, and timeout). The replacement comment documents the unified recovery contract.

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

### Task killed-sessions-resurrect-on-restart-2-5: Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance

**Problem**: Task 2-1 inserts an `unsetSkeletonMarkerOrLog` call into `handleHydrateTimeout`, and task 2-2 rewrites the comment block. Two invariants must hold post-edit: (a) the 100 ms settle-sleep is preserved on the exec fall-through (per spec § "Fix 2 → Specific Changes → 4" — same posture as the success path; `runHydrate` owns the sleep), and (b) `os.Remove(cfg.FIFO)` tolerates a missing FIFO silently. A future drive-by edit could accidentally remove the sleep, or replace `os.Remove` with a stricter primitive. Existing test `TestHydrate_TimeoutToleratesMissingFIFOSilently` (line 1237) covers (b) loosely. Neither covers the post-fix relative ordering of the unset call vs. the FIFO unlink, nor pins the preserved-sleep invariant on the timeout-recovery boundary.

**Solution**: Add one new unit test `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` that exercises `handleHydrateTimeout` directly (not through `runHydrate`) and asserts: (a) `os.Remove(cfg.FIFO)` on a non-existent FIFO returns nil from the handler; (b) the marker-unset call (`set-option -su`) is observed in the recording commander's call log before the handler returns (i.e. before `runHydrate` would issue the exec fall-through). Add a sibling test at the `runHydrate` boundary that asserts the timeout fall-through pays the 100 ms settle-sleep before exec (`elapsed >= hydrateSettleSleep`). Combined with the existing `TestHydrate_TimeoutPathInvokesHandleTimeout` (which asserts handler is called before the exec), this establishes the full ordering and preserved-sleep invariants.

**Outcome**: The preserved-sleep and ordering invariants are pinned by explicit tests that fail if a future change removes the 100 ms sleep on the timeout fall-through, swaps `os.Remove` for an erroring primitive, or reorders the unset call after the exec fall-through.

**Do**:
- Add `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` in `cmd/state_hydrate_test.go`. Construct a minimal `hydrateConfig` directly (not via `timeoutCfg` — this test calls the handler in isolation, not through `runHydrate`):
  - `dir := t.TempDir()`; `fifo := filepath.Join(dir, "hydrate-ord__0.0.fifo")` — do NOT mkfifo (drives the missing-FIFO branch).
  - `cmder := &recordingCommander{}`; `cfg := hydrateConfig{FIFO: fifo, HookKey: "ord:0.0", Stdout: io.Discard, Client: tmux.NewClient(cmder)}`.
- Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
- Assert `err == nil`.
- Assert the marker-unset call ordered before the handler returns (recording-commander check). The handler itself does NOT pay the 100 ms sleep — task 2-1 places the sleep inside `runHydrate` (post-handler, pre-exec). Therefore at the handler boundary, assert `elapsed < 50 ms` (the handler is fast — no sleep, no I/O beyond the FIFO unlink and the single `set-option -su` call).
- Assert `_, statErr := os.Stat(fifo); errors.Is(statErr, os.ErrNotExist)` — handler tolerated the absent FIFO and did not return an error.
- Assert the marker-unset argv `["set-option", "-su", "@portal-skeleton-ord__0.0"]` is present in `cmder.Calls`. The handler returns nil before the exec fall-through is reached, so the call log captures only the marker-unset.
- The `runHydrate`-boundary elapsed-time assertion (`elapsed >= hydrateSettleSleep`) is owned by task 2-1's renamed `TestHydrate_Timeout_PreservesSettleSleepBeforeExec`; this task's handler-direct test is intentionally complementary, pinning the handler's *no-sleep* posture so a future drive-by edit does not accidentally move the sleep from `runHydrate` into the handler (which would break ordering with marker-unset).

**Acceptance Criteria**:
- [ ] `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` passes.
- [ ] Handler-boundary elapsed time is well under `hydrateSettleSleep` (e.g. `elapsed < 50 ms`) — the handler does not own the sleep; `runHydrate` does (per task 2-1).
- [ ] `os.Remove` on a non-existent FIFO is silent — handler returns nil.
- [ ] `set-option -su @portal-skeleton-<paneKey>` appears in the recording commander's calls before the handler returns.
- [ ] No exec-related calls appear in the handler's recording commander log — the handler returns nil and `runHydrate` (not the handler) issues the exec fall-through.
- [ ] Test does not use `t.Parallel()`.

**Tests**:
- `"TestHydrate_TimeoutHandler_OrderingAndTimingInvariants"` — direct-handler ordering + FIFO-tolerance assertion.
- `"TestHydrate_Timeout_PreservesSettleSleepBeforeExec"` (sibling test at the `runHydrate` boundary) — asserts `elapsed >= hydrateSettleSleep` on the timeout fall-through, defensive against any future drive-by removal of the sleep.
- `"os.Remove(cfg.FIFO) tolerates missing FIFO silently"` — pre-existing in `TestHydrate_TimeoutToleratesMissingFIFOSilently` (line 1237) at the `runHydrate` boundary; this task adds the handler-level direct assertion.
- `"marker-unset call ordered before exec fall-through"` — the handler must issue `set-option -su` and return; only then does `runHydrate` invoke `execShellOrHookAndExit`. Verified by absence of any exec-related calls in the handler's recording commander log.

**Edge Cases**:
- 100 ms settle sleep on the timeout path is preserved per spec — same posture as the success path. The assertion is at the `runHydrate` boundary, not the handler boundary, since `runHydrate` owns the sleep regardless of whether `handleHydrateTimeout` or `handleHydrateFileMissing` returned.
- `os.Remove(cfg.FIFO)` on a path that does not exist must return silently — already true (line 256 uses `_ = os.Remove(...)`).
- Marker-unset call ordering: `unsetSkeletonMarkerOrLog` must execute before the handler returns. The handler does not invoke any exec primitive — that is `runHydrate`'s responsibility post-handler-return.

**Context**:
> `cmd/state_hydrate.go:248-266` — pre-task-2-1 `handleHydrateTimeout` body. The post-task-2-1 ordering is: (1) reset preamble → (2) `os.Remove(cfg.FIFO)` → (3) warn-log → (4) `unsetSkeletonMarkerOrLog(cfg)` → return nil. The 100 ms settle sleep is preserved on the exec fall-through per spec § "Fix 2 → Specific Changes → 4" (same posture as the success path); `runHydrate` owns the sleep regardless of whether `handleHydrateTimeout` or `handleHydrateFileMissing` returned.

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

[Removed in cycle 2 traceability review: task killed-sessions-resurrect-on-restart-2-7 (planning-side supersession artifact) — the spec already records the supersession at lines 156–163 of the killed-sessions spec; no additional planning-side deliverable is required.]

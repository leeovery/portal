---
status: complete
created: 2026-05-10
cycle: 1
phase: Plan Integrity Review
topic: Killed Sessions Resurrect on Restart
---

# Review Tracking: Killed Sessions Resurrect on Restart - Integrity

## Findings

### 1. 100 ms settle-sleep ownership unresolved across Phase 2 tasks [Fixed]

**Severity**: Critical
**Plan Reference**: Phase 2, tasks 2-2 and 2-5
**Category**: Task Self-Containment / Acceptance Criteria Quality / Dependencies and Ordering
**Change Type**: update-task

**Details**:

Spec § "Fix 2 → Specific Changes → 4" mandates "The 100 ms settle-sleep is preserved before exec — same posture as the success path". Today, `cmd/state_hydrate.go` does **not** pay the sleep on the timeout fall-through:

- `runHydrate` (lines 100-114) calls `cfg.HandleTimeout(cfg)` then immediately falls through to `execShellAndExit(cfg)` — no `time.Sleep(hydrateSettleSleep)` between them.
- `handleHydrateTimeout` itself ends with the comment "5. Deliberately NO 100ms sleep — nothing was dumped to settle." (line 264).
- The "shared post-handler block" the plan refers to does not exist — `runHydrate`'s timeout branch is just `HandleTimeout` → `execShellAndExit`.

The plan distributes responsibility for adding the sleep across two tasks with self-cancelling hedges:

- **Task 2-2** (line 60): "this task also restores the 100 ms `time.Sleep(hydrateSettleSleep)` before the exec fall-through *if it is currently absent on the timeout path*; if the existing code already pays the sleep elsewhere (e.g. inside `runHydrate`'s shared post-handler block), this task only updates the comment and leaves the sleep call untouched." — but two lines later: "**This task is comment-only — no behavioural change.**". These statements contradict each other, and the "already pays the sleep elsewhere" branch is empirically false on `main`.
- **Task 2-5** (acceptance criterion #2): "runHydrate timeout fall-through preserves the 100 ms settle-sleep before exec — recorded elapsed time at the runHydrate boundary is at least `hydrateSettleSleep`." — this assertion will fail against the current code unless some prior task explicitly inserts the sleep.
- Existing test `TestHydrate_TimeoutDoesNotSleep100ms` (cmd/state_hydrate_test.go:1114) is mentioned only in Task 2-5's Context block (line 215) — it pins the *opposite* invariant and must be flipped or removed, but no task owns that flip.

Result: an implementer following the plan task-by-task will not know which task adds the sleep, will encounter a contradictory Task 2-2 framing, and will be blocked when Task 2-5's elapsed-time assertion fails because no preceding task inserted the sleep call. The pre-existing `TestHydrate_TimeoutDoesNotSleep100ms` test will also still be green when Task 2-5 lands, in direct conflict with Task 2-5's new assertion.

The fix consolidates sleep ownership into Task 2-1 (the same task that already changes `handleHydrateTimeout`'s body and tests), removes Task 2-2's hedge, and makes Task 2-5 explicit about flipping/removing `TestHydrate_TimeoutDoesNotSleep100ms`. Task 2-1 is the natural home because it already (a) owns the only `handleHydrateTimeout` body edit in the phase and (b) is the first behavioural-change task in Phase 2 — putting the sleep insertion here means every subsequent task can rely on its presence.

**Current** (Task 2-1 — `Do` second bullet, phase-2-tasks.md lines 18-19):

> - Edit `cmd/state_hydrate.go` `handleHydrateTimeout`: insert `unsetSkeletonMarkerOrLog(cfg)` between the existing warn-log call and the `return nil`. Place it at the position where the deleted comment 4 lives — it is the new step 4, replacing the "deliberately NO UnsetServerOption" stub. Do not touch the FIFO unlink (`os.Remove`) or the `cfg.Logger.Warn(...)` lines; they keep their current ordering.

**Proposed** (Task 2-1 — `Do` second bullet — replace the single bullet above with the two bullets below):

> - Edit `cmd/state_hydrate.go` `handleHydrateTimeout`: insert `unsetSkeletonMarkerOrLog(cfg)` between the existing warn-log call and the `return nil`. Place it at the position where the deleted comment 4 lives — it is the new step 4, replacing the "deliberately NO UnsetServerOption" stub. Do not touch the FIFO unlink (`os.Remove`) or the `cfg.Logger.Warn(...)` lines; they keep their current ordering.
> - Edit `cmd/state_hydrate.go` `runHydrate`'s timeout branch (currently lines 103-110): insert `time.Sleep(hydrateSettleSleep)` between the `cfg.HandleTimeout(cfg)` call and the `execShellAndExit(cfg)` call (i.e. after the handler returns nil, before the exec fall-through). Per spec § "Fix 2 → Specific Changes → 4" the timeout fall-through must pay the same 100 ms settle sleep as the success path. The sleep lives in `runHydrate`, not in the handler, because `runHydrate` is the single owner of the post-handler exec sequence (mirrors how the success path's `time.Sleep(hydrateSettleSleep)` at line 171 lives in `runHydrate`'s straight-line body, not in a handler).

**Current** (Task 2-1 — `Acceptance Criteria` block, phase-2-tasks.md lines 22-27):

> **Acceptance Criteria**:
> - [ ] `handleHydrateTimeout` invokes `unsetSkeletonMarkerOrLog(cfg)` on every entry, after the FIFO unlink and warn-log lines.
> - [ ] The argv `["set-option", "-su", "@portal-skeleton-<paneKey>"]` appears exactly once in the recording commander's calls per timeout in the unit test.
> - [ ] `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` passes; the old `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` no longer exists.
> - [ ] `state.UnsetSkeletonMarkerForFIFO` failure is non-fatal: `unsetSkeletonMarkerOrLog`'s existing soft-warn-and-return contract is preserved (the wrapper logs via `cfg.Logger.Warn` and does not propagate the error). Subsequent exec fall-through still proceeds.
> - [ ] paneKey derivation is via the existing seam (`state.PaneKeyFromFIFOPath`) — no new derivation logic added at the handler call-site.

**Proposed** (Task 2-1 — `Acceptance Criteria` block):

> **Acceptance Criteria**:
> - [ ] `handleHydrateTimeout` invokes `unsetSkeletonMarkerOrLog(cfg)` on every entry, after the FIFO unlink and warn-log lines.
> - [ ] The argv `["set-option", "-su", "@portal-skeleton-<paneKey>"]` appears exactly once in the recording commander's calls per timeout in the unit test.
> - [ ] `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` passes; the old `TestHydrate_TimeoutDoesNotUnsetSkeletonMarker` no longer exists.
> - [ ] `state.UnsetSkeletonMarkerForFIFO` failure is non-fatal: `unsetSkeletonMarkerOrLog`'s existing soft-warn-and-return contract is preserved (the wrapper logs via `cfg.Logger.Warn` and does not propagate the error). Subsequent exec fall-through still proceeds.
> - [ ] paneKey derivation is via the existing seam (`state.PaneKeyFromFIFOPath`) — no new derivation logic added at the handler call-site.
> - [ ] `runHydrate`'s timeout branch pays `time.Sleep(hydrateSettleSleep)` between `cfg.HandleTimeout` returning nil and `execShellAndExit(cfg)` — same posture as the success-path settle sleep at line 171. Verified by a recorded elapsed time of at least `hydrateSettleSleep` at the `runHydrate` boundary on the timeout path.
> - [ ] The pre-existing `TestHydrate_TimeoutDoesNotSleep100ms` (cmd/state_hydrate_test.go:1114) is renamed/flipped in this same task to `TestHydrate_Timeout_PreservesSettleSleepBeforeExec` and asserts `elapsed >= hydrateSettleSleep` instead of the previous `elapsed < 50 ms`. This is the first task that changes the timeout-path elapsed-time invariant; no later task can ship green while the old assertion still pins the opposite invariant.

**Current** (Task 2-2 — `Do` first bullet, phase-2-tasks.md line 60):

> - Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// Recovery path matches handleHydrateFileMissing: marker unset above; runHydrate's exec fall-through still pays the 100 ms settle sleep before exec (preserved per spec — same posture as the success path).` This task also restores the 100 ms `time.Sleep(hydrateSettleSleep)` before the exec fall-through if it is currently absent on the timeout path; if the existing code already pays the sleep elsewhere (e.g. inside `runHydrate`'s shared post-handler block), this task only updates the comment and leaves the sleep call untouched.

**Proposed** (Task 2-2 — `Do` first bullet):

> - Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// Recovery path matches handleHydrateFileMissing: marker unset above; the 100 ms settle sleep is paid by runHydrate before exec (inserted in task 2-1, mirrors the success-path sleep posture).` This task is comment-only — task 2-1 already inserted the sleep call in `runHydrate`'s timeout branch, so this task does not touch any code lines.

**Current** (Task 2-5 — `Do` block, phase-2-tasks.md lines 184-192):

> - Add `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` in `cmd/state_hydrate_test.go`. Construct a minimal `hydrateConfig` directly (not via `timeoutCfg` — this test calls the handler in isolation, not through `runHydrate`):
>   - `dir := t.TempDir()`; `fifo := filepath.Join(dir, "hydrate-ord__0.0.fifo")` — do NOT mkfifo (drives the missing-FIFO branch).
>   - `cmder := &recordingCommander{}`; `cfg := hydrateConfig{FIFO: fifo, HookKey: "ord:0.0", Stdout: io.Discard, Client: tmux.NewClient(cmder)}`.
> - Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
> - Assert `err == nil`.
> - Assert the marker-unset call ordered before the handler returns (recording-commander check). Do NOT assert handler `elapsed < 50 ms` — the 100 ms settle-sleep must be preserved per spec § "Fix 2 → Specific Changes → 4". If the sleep lives inside `runHydrate` (post-handler) rather than inside `handleHydrateTimeout`, replace the elapsed-time assertion at the handler boundary with one at the `runHydrate` boundary that asserts `elapsed >= 100 ms`.
> - Assert `_, statErr := os.Stat(fifo); errors.Is(statErr, os.ErrNotExist)` — handler tolerated the absent FIFO and did not return an error.
> - Assert the marker-unset argv `["set-option", "-su", "@portal-skeleton-ord__0.0"]` is present in `cmder.Calls`. The handler returns nil before the exec fall-through is reached, so the call log captures only the marker-unset.

**Proposed** (Task 2-5 — `Do` block):

> - Add `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` in `cmd/state_hydrate_test.go`. Construct a minimal `hydrateConfig` directly (not via `timeoutCfg` — this test calls the handler in isolation, not through `runHydrate`):
>   - `dir := t.TempDir()`; `fifo := filepath.Join(dir, "hydrate-ord__0.0.fifo")` — do NOT mkfifo (drives the missing-FIFO branch).
>   - `cmder := &recordingCommander{}`; `cfg := hydrateConfig{FIFO: fifo, HookKey: "ord:0.0", Stdout: io.Discard, Client: tmux.NewClient(cmder)}`.
> - Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
> - Assert `err == nil`.
> - Assert the marker-unset call ordered before the handler returns (recording-commander check). The handler itself does NOT pay the 100 ms sleep — task 2-1 places the sleep inside `runHydrate` (post-handler, pre-exec). Therefore at the handler boundary, assert `elapsed < 50 ms` (the handler is fast — no sleep, no I/O beyond the FIFO unlink and the single `set-option -su` call).
> - Assert `_, statErr := os.Stat(fifo); errors.Is(statErr, os.ErrNotExist)` — handler tolerated the absent FIFO and did not return an error.
> - Assert the marker-unset argv `["set-option", "-su", "@portal-skeleton-ord__0.0"]` is present in `cmder.Calls`. The handler returns nil before the exec fall-through is reached, so the call log captures only the marker-unset.
> - The `runHydrate`-boundary elapsed-time assertion (`elapsed >= hydrateSettleSleep`) is owned by task 2-1's renamed `TestHydrate_Timeout_PreservesSettleSleepBeforeExec`; this task's handler-direct test is intentionally complementary, pinning the handler's *no-sleep* posture so a future drive-by edit does not accidentally move the sleep from `runHydrate` into the handler (which would break ordering with marker-unset).

**Current** (Task 2-5 — `Acceptance Criteria` block, phase-2-tasks.md lines 194-199):

> **Acceptance Criteria**:
> - [ ] `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` passes.
> - [ ] runHydrate timeout fall-through preserves the 100 ms settle-sleep before exec — recorded elapsed time at the runHydrate boundary is at least `hydrateSettleSleep`.
> - [ ] `os.Remove` on a non-existent FIFO is silent — handler returns nil.
> - [ ] `set-option -su @portal-skeleton-<paneKey>` appears in the recording commander's calls before the handler returns.
> - [ ] Test does not use `t.Parallel()`.

**Proposed** (Task 2-5 — `Acceptance Criteria` block):

> **Acceptance Criteria**:
> - [ ] `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` passes.
> - [ ] Handler-boundary elapsed time is well under `hydrateSettleSleep` (e.g. `elapsed < 50 ms`) — the handler does not own the sleep; `runHydrate` does (per task 2-1).
> - [ ] `os.Remove` on a non-existent FIFO is silent — handler returns nil.
> - [ ] `set-option -su @portal-skeleton-<paneKey>` appears in the recording commander's calls before the handler returns.
> - [ ] No exec-related calls appear in the handler's recording commander log — the handler returns nil and `runHydrate` (not the handler) issues the exec fall-through.
> - [ ] Test does not use `t.Parallel()`.

**Resolution**: Fixed
**Notes**: Sleep ownership consolidated into task 2-1 (handler unset + runHydrate sleep insertion + flip TestHydrate_TimeoutDoesNotSleep100ms). Task 2-2 de-hedged to comment-only. Task 2-5 re-anchored to handler boundary (`elapsed < 50 ms`) with a complementary assertion that the runHydrate-boundary check is owned by task 2-1's flipped test. Tick task 2-1 description updated; tick tasks 2-2 and 2-5 already updated by Finding 1 of the traceability cycle.

---

### 2. Task 1-1 contradicts itself on `OpenFIFOForSignal`'s export status [Fixed]

**Severity**: Important
**Plan Reference**: Phase 1, task 1-1 (`Do` block, fourth sub-bullet)
**Category**: Task Template Compliance / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:

Task 1-1 line 22 says: "A package-private `OpenFIFOForSignal(path string) (*os.File, error)` **exported helper** that wraps...". A symbol cannot be both package-private and exported. The follow-on tasks (1-2 line 73, 1-5 line 271, 1-5 line 308) all reference `state.OpenFIFOForSignal` as if exported. Without the fix, an implementer reading task 1-1 in isolation (the plan's self-containment requirement) does not know whether to declare `openFIFOForSignal` (lowercase, package-private) or `OpenFIFOForSignal` (uppercase, exported); the wrong choice breaks downstream tasks' compilation.

**Current** (Task 1-1 — `Do` block, fourth sub-bullet under "Add a new file `internal/state/signal_hydrate.go`", phase-1-tasks.md line 22):

> - A package-private `OpenFIFOForSignal(path string) (*os.File, error)` exported helper that wraps `os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)` so cmd-side and bootstrap-side callers can share the production FIFO opener.

**Proposed** (Task 1-1 — `Do` block, fourth sub-bullet):

> - An exported `OpenFIFOForSignal(path string) (*os.File, error)` helper that wraps `os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)` so cmd-side and bootstrap-side callers can share the production FIFO opener. (Exported — both `cmd/state_signal_hydrate.go` (task 1-2) and `internal/bootstrapadapter/adapters.go` (task 1-5) reference it as `state.OpenFIFOForSignal`.)

**Resolution**: Fixed
**Notes**: phase-1-tasks.md task 1-1 fourth sub-bullet under "Add a new file `internal/state/signal_hydrate.go`" rewritten to be unambiguously exported.

---

### 3. Task 1-6 leaves the implementer to choose between two divergent test shapes [Fixed]

**Severity**: Important
**Plan Reference**: Phase 1, task 1-6 (`Do` block, sixth and seventh sub-bullets)
**Category**: Task Self-Containment / Scope and Granularity
**Change Type**: update-task

**Details**:

Task 1-6's `Do` section gives two distinct test shapes for AC1 verification — one that "exec the full helper" via `useBinary`-style binary fixture, and an "Acceptable test shape" that records `WriteFIFOSignal` invocations via a recording shim plus polls `state.ListSkeletonMarkers`. The text says "if not [straightforward], the test can directly assert that `state.WriteFIFOSignal` was invoked..." — leaving the implementer to choose. The two shapes have meaningfully different coverage:

- **Binary fixture path**: end-to-end test (real helper, real exec, real marker-unset by the helper). Verifies eager-signal step → FIFO → helper → marker-unset chain.
- **Recording shim path**: only verifies eager-signal step writes the byte; does not exercise the helper consuming the byte and unsetting the marker.

If the implementer takes the recording-shim path, AC1 (which mandates the marker set is empty within 2s — i.e. the helper actually unset it) is not actually tested end-to-end; the test would pass even if the helper were broken. The plan should pin one shape that actually verifies AC1's pass condition (empty marker set, not just "signal byte written"). The binary-fixture path is the only one that does this — task 1-6 already requires polling `state.ListSkeletonMarkers` until empty, which only succeeds if the helper consumed the byte and ran its marker-unset. The recording-shim path cannot deliver an empty-marker-set outcome by construction.

**Current** (Task 1-6 — `Do` block, sixth sub-bullet, phase-1-tasks.md lines 373-378):

> - The hydrate helper inside each restored pane will exec `portal state hydrate` (via the existing `buildHydrateCommand` path); the test does NOT exec the full helper — it relies on `state.WriteFIFOSignal` reaching the FIFO and the helper, on signal receipt, unsetting its marker.
>   - Ensure the test's `portal` binary path is wired so the hydrate helper can actually run. The simplest shape is to reuse the `runRebootRoundTrip`-style `useBinary` fixture if it is straightforward; if not, the test can directly assert that `state.WriteFIFOSignal` was invoked for each marker (use a recording shim around the adapter's WriteFIFOSignal closure) and that **`@portal-skeleton-*` markers transition to empty within 2s** by polling.
>   - Acceptable test shape: the integration goal is "eager-signal step writes the byte to every marker's FIFO inside the bootstrap window". Validate via:
>     1. `runOrchestrator` returns nil within bootstrap.
>     2. Poll `state.ListSkeletonMarkers(client)` every 50ms for up to 2s.
>     3. Assert empty marker set within the window.

**Proposed** (Task 1-6 — `Do` block, sixth sub-bullet):

> - The hydrate helper inside each restored pane execs `portal state hydrate` via the existing `buildHydrateCommand` path. The test must wire the `portal` binary path (via `restoretest.BuildPortalBinaryDir` + `restoretest.PrependPATH`, mirroring `internal/restore/integration_full_test.go`) so the helper actually runs end-to-end — the eager-signal step writes the byte, the helper consumes it, runs its scrollback dump, and unsets the marker. Anything less (e.g. a recording shim around `WriteFIFOSignal`) does not exercise the helper's marker-unset and therefore cannot validate AC1's pass condition.
> - Validation:
>     1. `o.Run(context.Background())` returns nil within bootstrap.
>     2. Poll `state.ListSkeletonMarkers(client)` every 50ms for up to 2s.
>     3. Assert the marker set transitions to empty within the window — this transition is owned by the helper unsetting the marker after the eager-signal byte arrives, so an empty marker set proves the full eager-signal → FIFO → helper → marker-unset chain.

**Resolution**: Fixed
**Notes**: Task 1-6 sixth bullet rewritten to mandate the binary-fixture path; recording-shim option removed. AC1 is now genuinely verified end-to-end.

---

### 4. Task 1-8 hedges on whether `state.RunCaptureOnce` is added in this task [Fixed]

**Severity**: Important
**Plan Reference**: Phase 1, task 1-8 (`Do` block third bullet, `Edge Cases` second bullet)
**Category**: Scope and Granularity / Task Self-Containment
**Change Type**: update-task

**Details**:

Task 1-8 says: "drive one daemon capture-tick by directly invoking the daemon's capture-once primitive (e.g. `state.RunCaptureOnce(client, stateDir, logger)`; **if no such primitive exists, expose one as a test seam in `internal/state` for this purpose**, mirroring the existing capture-loop body)." The Edge Cases also note: "If the daemon's capture-once primitive does not yet exist, this task adds it as a thin test seam in `internal/state`...". The plan is hedging on whether adding a new exported function to `internal/state` is in or out of scope.

This is unsafe for two reasons:
1. The plan's Phase 1 acceptance criterion #1 says: "`writeFIFOSignal` and `signalHydrateRetryDelays` are relocated from `cmd` into `internal/state` *with no public API surface added*". If task 1-8 actually exports `state.RunCaptureOnce`, that contradicts the phase acceptance.
2. An implementer cannot predict `internal/state`'s state at task-start-time without reading other tasks first — violating the self-containment principle.

The plan should either confirm `RunCaptureOnce` already exists (via a quick verification step in the task), or explicitly add an `Add seam if missing` step with its own acceptance bullet, and update the phase acceptance to permit this single additional export (which is a test-only seam, not production API).

**Current** (Task 1-8 — `Do` block, third bullet, phase-1-tasks.md line 481):

> - After the marker-cleared poll completes (markers empty within 2 s), drive one daemon capture-tick by directly invoking the daemon's capture-once primitive (e.g. `state.RunCaptureOnce(client, stateDir, logger)`; if no such primitive exists, expose one as a test seam in `internal/state` for this purpose, mirroring the existing capture-loop body).

**Proposed** (Task 1-8 — `Do` block, third bullet — replace the single bullet above with the two bullets below):

> - Verify whether `internal/state` exposes a capture-once primitive callable from outside the daemon's main loop (search for `RunCaptureOnce`, `CaptureOnce`, or equivalents in `internal/state/*.go`). If none exists, add a thin exported test seam `state.RunCaptureOnce(client state.ServerOptionLister, stateDir string, logger *state.Logger) error` that runs exactly one iteration of the daemon's `captureAndCommit` loop body (no goroutine, no ticker — straight-line invocation of the existing capture step). This seam is the single permitted addition to `internal/state`'s export surface for this phase beyond what task 1-1 already added.
> - After the marker-cleared poll completes (markers empty within 2 s), drive one daemon capture-tick by invoking `state.RunCaptureOnce(client, stateDir, logger)`. The capture tick must run after `@portal-restoring` is cleared by step 7 of the orchestrator's run — by the time the polling loop in this test exits with `markers empty`, the orchestrator has already returned, so the marker is already cleared.

**Current** (Task 1-8 — `Acceptance Criteria` block, phase-1-tasks.md lines 485-489):

> **Acceptance Criteria**:
> - [ ] Sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` exists in `cmd/bootstrap/eager_signal_hydrate_integration_test.go`.
> - [ ] Sub-test passes against a build with eager signaling wired; fails if the daemon refuses to capture a previously-stuck pane.
> - [ ] No `t.Parallel()` usage.
> - [ ] Skips cleanly under `-short` and when tmux is unavailable.

**Proposed** (Task 1-8 — `Acceptance Criteria` block):

> **Acceptance Criteria**:
> - [ ] Sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` exists in `cmd/bootstrap/eager_signal_hydrate_integration_test.go`.
> - [ ] Sub-test passes against a build with eager signaling wired; fails if the daemon refuses to capture a previously-stuck pane.
> - [ ] If `state.RunCaptureOnce` did not exist before this task, it is added as a thin test seam exporting exactly one function with the signature `RunCaptureOnce(client state.ServerOptionLister, stateDir string, logger *state.Logger) error`. The seam runs exactly one iteration of the existing daemon capture-step body. No other new exports are introduced into `internal/state`.
> - [ ] No `t.Parallel()` usage.
> - [ ] Skips cleanly under `-short` and when tmux is unavailable.

**Resolution**: Fixed
**Notes**: Task 1-8 third bullet split into two: a verify-or-add-seam step plus the capture-tick invocation. Acceptance criteria gain an explicit "if seam was missing, it is added with exactly this signature" bullet. The Phase 1 acceptance criterion #1 (no public API beyond writeFIFOSignal / signalHydrateRetryDelays) is scoped to those specific symbols and does not contradict adding RunCaptureOnce as a separate test seam.

---

### 5. Task 1-7 Tests block lists non-test items as tests [Fixed]

**Severity**: Minor
**Plan Reference**: Phase 1, task 1-7 (`Tests` block)
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:

Task 1-7 (CLAUDE.md update) is a documentation-only task. Its `Tests` block lists two items:

- `Manual review: open CLAUDE.md and verify the section reads correctly end-to-end.`
- `"git diff CLAUDE.md shows only the Server bootstrap section was modified"` — pre-commit visual check.

Per task-design.md, `Tests` lists test names. Manual reviews and pre-commit visual checks are not tests — they are verification steps. The convention used by task 3-2 ("No new test cases — this task is documentation-only") is the cleaner pattern. This is minor (an implementer would not be confused), but it's inconsistent with task 3-2's handling of an analogous documentation task and weakens the template-compliance signal.

**Current** (Task 1-7 — `Tests` block, phase-1-tasks.md lines 452-454):

> **Tests**:
> - Manual review: open CLAUDE.md and verify the section reads correctly end-to-end.
> - `"git diff CLAUDE.md shows only the Server bootstrap section was modified"` — pre-commit visual check.

**Proposed** (Task 1-7 — `Tests` block):

> **Tests**:
> - No new test cases — this task is documentation-only. The post-edit verification is a manual read of the section and a `git diff` check that no other CLAUDE.md sections were modified; the acceptance criteria above pin the substantive checks.

**Resolution**: Fixed
**Notes**: Tests block in phase-1-tasks.md task 1-7 replaced with the documentation-only convention.

---

### 6. Task 2-7 Tests block lists non-test items as tests [Fixed]

**Severity**: Minor
**Plan Reference**: Phase 2, task 2-7 (`Tests` block)
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:

Task 2-7 (supersession note) is a documentation-only task with the same anti-pattern as task 1-7 — its `Tests` block lists `grep` invocations and `git diff` checks as tests. Same fix as finding 5: align with task 3-2's documentation-only convention.

**Current** (Task 2-7 — `Tests` block, phase-2-tasks.md lines 311-315):

> **Tests**:
> - `"phase-2-supersession.md exists at the canonical path"` — manual verification; this task is documentation, not code.
> - `"original built-in-session-resurrection spec is byte-identical post-task"` — `git diff .workflows/built-in-session-resurrection/specification/` shows no changes.
> - `"both quoted invariants appear verbatim in the supersession note"` — `grep -F "Helper does NOT unset marker on FIFO timeout"` and `grep -F "Resume hooks fire only from inside the hydrate helper's exec chain"` against the new file return matches.
> - `"supersession note links AC2 and AC6"` — search the file for substrings `AC2` and `AC6`.

**Proposed** (Task 2-7 — `Tests` block):

> **Tests**:
> - No new test cases — this task is documentation-only. The acceptance criteria above pin the substantive checks (file exists at canonical path, both invariants quoted verbatim, AC2/AC6 referenced explicitly, original spec file byte-identical).

**Resolution**: Fixed
**Notes**: Tests block in phase-2-tasks.md task 2-7 replaced with the documentation-only convention.

---

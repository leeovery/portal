---
phase: 2
phase_name: Context-aware captureAndCommit (daemon side of the kill-barrier contract)
total: 6
---

## saver-kill-respawn-loop-leaks-daemons-2-1 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-1: Thread `ctx` from `defaultDaemonRun` through `tick` into `captureAndCommit` (signature change + happy-path regression)

**Problem**: `defaultDaemonRun` (`cmd/state_daemon.go:70`) runs `tick(deps)` synchronously inside the ticker's `select` arm; `tick` calls `captureAndCommit(deps)` (`cmd/state_daemon.go:110`); neither inner function sees `ctx`. Because `ctx.Done()` is structurally unreachable during a tick, the daemon cannot honour cancellation while a capture is in flight — the 5s `killBarrierTimeout` lapses on any tick whose aggregate `tmux capture-pane` wall time exceeds it (Defect 2). Before the per-iteration cancellation observation points (Tasks 2-2 / 2-3 / 2-4) can be added, `ctx` must reach the inside of `captureAndCommit`.

**Solution**: Change the signatures of `tick` and `captureAndCommit` (both in `cmd/state_daemon.go`) to accept `ctx context.Context` as the first parameter. Update the single call site of `tick` inside `defaultDaemonRun` to pass the loop's `ctx`. Update the two call sites of `captureAndCommit` (one inside `tick`, one inside `defaultShutdownFlush`) to pass `ctx` and `context.Background()` respectively — the shutdown flush must remain non-cancellable so the on-exit save is not aborted by the very signal that triggered it. **No `ctx.Done()` observations are added in this task** — they ship in Tasks 2-2, 2-3, 2-4. This task is the plumbing-only step plus a happy-path regression guard that proves the threading did not change behaviour.

**Outcome**: `tick(ctx, deps)` and `captureAndCommit(ctx, deps)` exist in `cmd/state_daemon.go` with `ctx` plumbed from `defaultDaemonRun`. `defaultShutdownFlush` continues to call `captureAndCommit(context.Background(), deps)` so the shutdown flush is preserved as non-cancellable. `internal/state/capture.go` is untouched (its `CaptureStructure` and `CaptureAndHashPane` signatures are unchanged per spec §Change 2). All existing unit tests in `cmd/state_daemon_test.go` and `cmd/state_daemon_run_test.go` remain green — the happy-path regression guard.

**Do**:
- Open `cmd/state_daemon.go`.
- Change `func tick(deps *daemonDeps)` (line 94) to `func tick(ctx context.Context, deps *daemonDeps)`. Do not add any `ctx.Done()` observations inside `tick` — leave the body unchanged except for the updated `captureAndCommit` call.
- Change `func captureAndCommit(deps *daemonDeps) error` (line 132) to `func captureAndCommit(ctx context.Context, deps *daemonDeps) error`. Do not add any `ctx.Done()` observations inside `captureAndCommit` — leave the body unchanged.
- Inside `defaultDaemonRun` (line 76), change `tick(deps)` to `tick(ctx, deps)`.
- Inside `tick` (line 110), change `captureAndCommit(deps)` to `captureAndCommit(ctx, deps)`.
- Inside `defaultShutdownFlush` (line 198), change `captureAndCommit(deps)` to `captureAndCommit(context.Background(), deps)`. Add a brief comment above that line documenting why: "shutdown flush is non-cancellable — the cancelled context is what triggered the flush; passing it through would abort the very save we are exiting to perform."
- Confirm `internal/state/capture.go` is not touched. Confirm no other file in the repo references `tick(` or `captureAndCommit(` (grep to verify). Adjust any in-package test callers (e.g. tests in `cmd/state_daemon_test.go` / `cmd/state_daemon_run_test.go`) that invoke `tick` or `captureAndCommit` directly to pass `context.Background()` — the happy-path regression guard relies on those tests staying green.
- Add one new explicit happy-path regression test in `cmd/state_daemon_test.go` (or extend the closest existing test) — `"captureAndCommit with uncancelled context behaves identically to pre-threading implementation"` — which drives a multi-pane fixture against the in-package fakes, calls `captureAndCommit(context.Background(), deps)`, and asserts the pre-existing commit semantics (`PrevIndex` is replaced with the fresh index, `state.Commit` is invoked once, all panes are processed). This protects against silent regression introduced by the signature change itself.
- Run `go build -o portal .` and `go test ./cmd/...` and confirm green.

**Acceptance Criteria**:
- [ ] `tick` and `captureAndCommit` both accept `ctx context.Context` as their first parameter.
- [ ] `defaultDaemonRun` calls `tick(ctx, deps)`; `tick` calls `captureAndCommit(ctx, deps)`; `defaultShutdownFlush` calls `captureAndCommit(context.Background(), deps)`.
- [ ] The comment above the shutdown-flush call documents why `context.Background()` is used (non-cancellable shutdown flush).
- [ ] `internal/state/capture.go` is unchanged. `CaptureStructure` and `CaptureAndHashPane` signatures are unchanged.
- [ ] No file outside `cmd/state_daemon.go` and its sibling test files is modified.
- [ ] All existing tests in `cmd/...` that invoke `tick` or `captureAndCommit` are updated to pass `context.Background()` and remain green.
- [ ] The new happy-path regression test passes.
- [ ] `go build -o portal .` succeeds; `go test ./cmd/...` is green.

**Tests**:
- `"captureAndCommit with uncancelled context behaves identically to pre-threading implementation"` — multi-pane fixture, `context.Background()`, asserts `PrevIndex` replacement, `state.Commit` invocation count, all panes processed.
- `"defaultShutdownFlush passes context.Background() to captureAndCommit so the shutdown save is not cancellable"` — observe via a fake (or by inspecting the value at call time through a package-level seam if one is already present) that the shutdown path's ctx is not the cancelled tick-loop ctx.
- `"defaultDaemonRun passes the loop's ctx through to tick"` — uses an in-test `tick`-style seam (or by observing behaviour in 2-2 / 2-3 / 2-4 tests; if no seam exists in this task, document the assertion as covered transitively by the cancellation tests in 2-2).
- All existing tests in `cmd/state_daemon_test.go` and `cmd/state_daemon_run_test.go` remain green after their call sites are updated.

**Edge Cases**:
- `defaultShutdownFlush` calling `captureAndCommit` with the cancelled loop ctx would deadlock-or-skip the on-exit save (spec §Risk & Rollout: "Verify that `daemonShutdownFunc` does not depend on the cancelled tick's output"). The fix is to pass `context.Background()` explicitly — do not "thread it through" from the shutdown path.
- Signature changes must not propagate outside `cmd/state_daemon.go`. If grep finds a caller in another package, the threading approach is wrong — re-read the spec §Change 2 constraint.
- In-package test files that invoke `tick` or `captureAndCommit` directly must be updated in the same task; otherwise the build breaks. Do not split the test-update work into a follow-up task — it is part of the signature change.
- `internal/state/capture.go` signatures are explicitly out of scope. If touching them seems necessary, the design is wrong — `ctx` is observed *adjacent* to the `CaptureAndHashPane` call in `captureAndCommit`, not inside it.

**Context**:
> Spec §Change 2: "Signature changes are local to this file. The per-pane `state.CaptureAndHashPane` call is invoked directly from inside `captureAndCommit`'s loop (currently `cmd/state_daemon.go:152`), so the `ctx.Done()` check sits adjacent to that call in the same file. `internal/state/capture.go` is not modified — its `CaptureStructure` and `CaptureAndHashPane` signatures remain unchanged."
>
> Spec §Change 2 (Cancellation semantics): "Shutdown flush behaviour (`daemonShutdownFunc`) is unchanged — it still runs on the cancelled-context path after the tick-loop returns."
>
> Spec §Risk & Rollout: "Ctx cancellation between per-pane iterations must not introduce a deadlock where the shutdown flush waits for a tick that just got cancelled. Verify that `daemonShutdownFunc` does not depend on the cancelled tick's output."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 2

---

## saver-kill-respawn-loop-leaks-daemons-2-2 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-2: Add `ctx.Done()` check at `captureAndCommit` entry (pre-enumeration) with cancel-before-first unit test

**Problem**: After Task 2-1 plumbs `ctx` into `captureAndCommit`, the function still does not observe cancellation. The spec mandates three distinct observation points; this task installs the **first** one — at function entry, before `state.ListSkeletonMarkers` is called. Without this check, an already-cancelled context (e.g. SIGHUP arrives during the `tick`-period sleep, cancel fires, ticker delivers an event before the cancel is observed at the outer `select`) would still trigger a full marker-listing + structure-capture + per-pane sweep before the daemon could exit.

**Solution**: At the very first statement inside `captureAndCommit` (before the existing `state.ListSkeletonMarkers(deps.Client)` call at `cmd/state_daemon.go:133`), check `ctx.Done()` via a non-blocking `select { case <-ctx.Done(): return nil; default: }`. On cancellation, return `nil` (clean abandon — not an error; the loop in `defaultDaemonRun` will subsequently observe the cancellation at its own `select` and call `daemonShutdownFunc`). No marker enumeration, no `CaptureStructure` call, no `Commit`, no `PrevIndex` mutation, no `LastSaveAt` update.

**Outcome**: `captureAndCommit` returns early with `nil` if `ctx` is already cancelled at entry. None of the per-tick side effects fire on this path. A new unit test in `cmd/state_daemon_test.go` drives this directly: a pre-cancelled ctx + a fake tmux client whose `ListSessions` would record a call → asserts the call counter is zero. Existing tests remain green.

**Do**:
- Open `cmd/state_daemon.go` and locate `captureAndCommit` (line 132, now with `ctx` first parameter after Task 2-1).
- Insert at the very first statement of the function body (before the existing `state.ListSkeletonMarkers(deps.Client)` call):
  ```go
  select {
  case <-ctx.Done():
      return nil
  default:
  }
  ```
- Place a comment immediately above the select documenting "observation point 1 of 3: pre-enumeration; ensures a cancellation that arrives between ticker fire and tick entry returns immediately without any tmux work or commit." Reference spec §Change 2 cancellation semantics.
- Do not return an error on cancellation. Cancellation is a clean abandon, not a failure — `defaultDaemonRun`'s outer `select` is the authoritative shutdown driver, and `defaultShutdownFlush` is what runs on the cancelled-context exit path.
- Add unit test "cancel before first per-pane iteration → early return, no commits" in `cmd/state_daemon_test.go`:
  - Construct `daemonDeps` with a fake tmux client that records each call (use the existing fake-client seam pattern in that file).
  - `ctx, cancel := context.WithCancel(context.Background())`; `cancel()` immediately.
  - Call `captureAndCommit(ctx, deps)`.
  - Assert: return value is `nil`; the fake's `ListSkeletonMarkers` / `ListSessions` / `CaptureStructure` proxy call counter is zero; `deps.PrevIndex` is unchanged; no `state.Commit` invocation observed.
- Run `go test ./cmd/...` and confirm green; existing tests must remain green (the new check is gated on cancellation and is a no-op when ctx is live).

**Acceptance Criteria**:
- [ ] `captureAndCommit` checks `ctx.Done()` at its first statement before any other work.
- [ ] On a pre-cancelled context, the function returns `nil` (not an error).
- [ ] `state.ListSkeletonMarkers` is not invoked when `ctx` is already cancelled at entry.
- [ ] `state.CaptureStructure` is not invoked.
- [ ] No `state.Commit` invocation occurs.
- [ ] `deps.PrevIndex` is not mutated.
- [ ] `deps.LastSaveAt` is not mutated (the mutation lives in `tick` after `captureAndCommit` returns successfully, but assert by sampling before/after).
- [ ] A unit test in `cmd/state_daemon_test.go` named to describe "cancel before first per-pane iteration" pins the behaviour.
- [ ] Existing tests in `cmd/state_daemon_test.go` and `cmd/state_daemon_run_test.go` remain green.

**Tests**:
- `"captureAndCommit with already-cancelled ctx at entry returns nil and performs no tmux work"` — primary assertion: call-count on the fake client's enumeration methods is zero.
- `"captureAndCommit with already-cancelled ctx at entry does not invoke state.Commit"` — pin no-commit invariant.
- `"captureAndCommit with already-cancelled ctx at entry does not mutate PrevIndex"` — pin no-`PrevIndex`-replacement invariant.
- `"captureAndCommit with uncancelled ctx still completes a full tick (regression guard)"` — sanity check that the new select does not accidentally short-circuit on live ctx.

**Edge Cases**:
- The `select { ... default: }` pattern is the canonical Go non-blocking ctx check. Do **not** use `if ctx.Err() != nil` instead — the spec's three observation points are explicitly framed around `<-ctx.Done()` to align with the same channel mechanic the outer loop uses.
- Returning `nil` on cancellation (not an error) is load-bearing: `tick` calls `deps.Logger.Warn(... "tick: %v", err)` on a non-nil return, and we do not want a spurious WARN line on the (expected) cancellation path.
- The check must precede `state.ListSkeletonMarkers` because that call hits tmux — even if fast, it is the "first observable side effect" and the spec wants cancellation to suppress it.
- `LastSaveAt` is updated in `tick`, not `captureAndCommit`. The assertion is that `tick` never reaches its `deps.LastSaveAt = time.Now()` line when `captureAndCommit` returns nil on cancellation — but since `tick` does set `LastSaveAt = time.Now()` after a *successful* `captureAndCommit` (line 115), and `captureAndCommit` returning nil on cancellation is indistinguishable from nil on success, this is a real subtlety. **Mitigation in this task**: do not change `tick`'s `LastSaveAt` update logic in this task. The spec's acceptance is "no partial commits" — `LastSaveAt` being updated on a cancelled-tick is acceptable because no observable state changed on disk. Document this in the test's comment.

**Context**:
> Spec §Change 2 (Cancellation semantics): "`ctx.Done()` is observed at three points inside `captureAndCommit`: 1. Before pane enumeration begins — checked at function entry, ensures cancellation while a tick is queued returns immediately."
>
> Spec §Change 2: "On cancellation, return early without committing partial state. The current tick is abandoned cleanly — no half-applied scrollback writes, no partial commit."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 2 (Cancellation semantics, point 1)

---

## saver-kill-respawn-loop-leaks-daemons-2-3 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-3: Add `ctx.Done()` check post-enumeration, pre-first-iteration with unit test

**Problem**: `state.CaptureStructure` (called at `cmd/state_daemon.go:138`) is described in the spec as a fast call but it is still a tmux subprocess invocation. If cancellation arrives during or immediately after `CaptureStructure` returns but before the per-pane loop begins, the daemon must observe it before doing any per-pane work. Without this second observation point, the daemon would commit to the bulk of the per-pane sweep based on a stale `idx` snapshot, even though cancellation is already pending.

**Solution**: Insert the **second** `ctx.Done()` observation point immediately after `state.CaptureStructure` returns (after the existing error check at `cmd/state_daemon.go:138-141`) and before the `for _, sess := range idx.Sessions {` loop begins (current line 144). On cancellation: return `nil` without entering the loop, without invoking `Commit`, and without replacing `deps.PrevIndex` with the freshly-captured `idx`.

**Outcome**: When cancellation arrives between `CaptureStructure` returning and the first per-pane iteration starting, `captureAndCommit` returns `nil` with no per-pane work done, no `state.Commit` call, no `PrevIndex` replacement. A new unit test drives this exact timing: a fake `state.CaptureStructure` (via the seam used in existing daemon tests) returns a multi-pane `idx`, then a `cancel()` is called from the same fake before the test inspects post-call state.

**Do**:
- Open `cmd/state_daemon.go` and locate the line immediately after the `state.CaptureStructure` error-check block (current line 141, the closing brace of the `if err != nil` block) — i.e. just before the `anyScrollbackChanged := false` initialisation on line 143.
- Insert:
  ```go
  select {
  case <-ctx.Done():
      return nil
  default:
  }
  ```
- Place a comment immediately above documenting "observation point 2 of 3: post-enumeration, pre-first-iteration; covers cancellation during the `CaptureStructure` subprocess call. Returns before any per-pane work or `Commit` invocation." Reference spec §Change 2 cancellation semantics point 2.
- Return `nil`, not an error (same reasoning as Task 2-2).
- Add unit test "cancel between CaptureStructure and first per-pane iteration" in `cmd/state_daemon_test.go`:
  - Construct a fake whose `CaptureStructure`-equivalent seam returns a populated multi-pane `idx` (reuse the existing multi-pane fixture from the happy-path test added in Task 2-1, or construct a minimal 2-session × 2-window × 2-pane fixture).
  - The fake's `CaptureStructure` implementation calls `cancel()` *before returning* — this is the easiest way to drive the exact race the observation point is designed to catch.
  - Call `captureAndCommit(ctx, deps)` with that ctx.
  - Assert: return value is `nil`; the fake's `CaptureAndHashPane` and `WriteScrollbackIfChanged` and `state.Commit` proxies record zero calls; `deps.PrevIndex` is the value it had before the call (the freshly-captured `idx` was *not* assigned).
- Run `go test ./cmd/...` and confirm green.

**Acceptance Criteria**:
- [ ] `captureAndCommit` checks `ctx.Done()` immediately after `state.CaptureStructure` returns successfully and before the per-pane loop begins.
- [ ] On cancellation observed at this point, the function returns `nil`.
- [ ] No iteration of `for _, sess := range idx.Sessions` begins.
- [ ] `state.CaptureAndHashPane` is not invoked.
- [ ] `state.WriteScrollbackIfChanged` is not invoked.
- [ ] `state.Commit` is not invoked.
- [ ] `deps.PrevIndex` is **not** replaced with the freshly-captured `idx`.
- [ ] A unit test in `cmd/state_daemon_test.go` named to describe "cancel post-CaptureStructure, pre-first-iteration" pins the behaviour.
- [ ] All Phase 2 prior-task tests (including the Task 2-2 cancel-before-first test) remain green.
- [ ] Existing tests in `cmd/state_daemon_test.go` and `cmd/state_daemon_run_test.go` remain green.

**Tests**:
- `"captureAndCommit observes cancellation after CaptureStructure returns but before first per-pane iteration"` — drive cancellation from inside the fake's `CaptureStructure` implementation.
- `"captureAndCommit cancelled post-enumeration does not invoke CaptureAndHashPane on any pane"` — call-counter on the fake's `CaptureAndHashPane`.
- `"captureAndCommit cancelled post-enumeration does not invoke state.Commit"` — call-counter on the fake's `Commit` proxy.
- `"captureAndCommit cancelled post-enumeration does not replace PrevIndex"` — sample `deps.PrevIndex` pointer before and after; pin equality.

**Edge Cases**:
- Cancellation observed **here** vs. cancellation observed at **entry** (Task 2-2) are distinct timings — the test must explicitly drive the cancel from inside the fake's `CaptureStructure` implementation, not at construction time, otherwise it would be exercising Task 2-2's path instead.
- `CaptureStructure` itself is **not** cancelled mid-flight by this check — the spec is explicit ("Cancellation is not honoured mid-`capture-pane` invocation. The `tmux list-panes` enumeration call and any in-flight `tmux capture-pane` invocation complete before the cancel is observed"). The observation point sits *after* `CaptureStructure` returns. No goroutine, no subprocess kill.
- Even though the freshly-captured `idx` is discarded on this path, the per-pane `state.WriteScrollbackIfChanged` writes that the previous (successful) tick committed are not touched — they are atomic per-pane files, and the spec's "no partial commit" requirement is about `sessions.json`, not about rolling back successfully-written scrollback files.

**Context**:
> Spec §Change 2 (Cancellation semantics): "`ctx.Done()` is observed at three points inside `captureAndCommit`: ... 2. After enumeration, before the first per-pane iteration — covers cancellation during the (fast) `CaptureStructure` call."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 2 (Cancellation semantics, point 2)

---

## saver-kill-respawn-loop-leaks-daemons-2-4 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-4: Add `ctx.Done()` check between per-pane iterations with cancel-mid-loop unit test on multi-pane fixture

**Problem**: The bulk of `captureAndCommit`'s wall time is the per-pane `state.CaptureAndHashPane` invocation (one `tmux capture-pane -e -p -S -` subprocess per pane). On the affected user's profile (~23 panes × ~1.2MB rendered text), the aggregate exceeds the 5s `killBarrierTimeout`. Cancelling **before** the loop (Tasks 2-2 / 2-3) caps the latency at one full-loop traversal, but real-world cancellation timing has the daemon mid-loop. Without an observation point between iterations, the daemon would finish all 23 panes even when cancellation arrived after pane 1.

**Solution**: Insert the **third** `ctx.Done()` observation point at the top of the innermost (pane) loop body — i.e. immediately after `for _, pane := range win.Panes {` opens (current line 146) and before any per-pane work begins for that iteration (before the `paneKey := state.SanitizePaneKey(...)` line at 147). On cancellation: return `nil` from `captureAndCommit` without invoking `state.Commit`, without replacing `deps.PrevIndex`, and without rolling back per-pane scrollback writes for panes already processed in this iteration cycle.

This caps worst-case SIGHUP-to-exit latency at one pane's `capture-pane` wall time (per spec §Change 2: "This caps worst-case daemon-exit latency at one pane's `capture-pane` wall time rather than 'all panes' aggregated wall time'").

**Outcome**: When cancellation arrives mid-loop after `k` of `N` panes have been processed, `captureAndCommit` exits cleanly: `state.Commit` is not invoked, `deps.PrevIndex` is not replaced with the freshly-captured `idx`, and the `anyScrollbackChanged` accumulator is discarded. Per-pane scrollback writes that already landed via `state.WriteScrollbackIfChanged` for the completed `k` panes are **not** rolled back (per-pane writes are atomic; the spec's "no partial commit" requirement is about `sessions.json`, not per-pane file rollback — see edge cases below).

**Do**:
- Open `cmd/state_daemon.go` and locate the innermost pane loop at line 146 (`for _, pane := range win.Panes {`).
- Insert at the very first statement inside the pane-loop body (immediately after the `{`):
  ```go
  select {
  case <-ctx.Done():
      return nil
  default:
  }
  ```
- Place a comment immediately above documenting "observation point 3 of 3: between per-pane iterations; caps worst-case exit latency at one pane's `capture-pane` wall time. Returns before this iteration's `CaptureAndHashPane`. Per-pane scrollback writes from prior iterations in this cycle are not rolled back — they are atomic, and the spec's 'no partial commit' invariant is about `sessions.json`, not per-pane files." Reference spec §Change 2 cancellation semantics point 3.
- Return `nil`, not an error.
- Add unit test "cancel mid-loop after k of N panes processed" in `cmd/state_daemon_test.go`:
  - Construct a multi-pane fixture with at least 3 panes (e.g. 1 session × 1 window × 3 panes).
  - Configure the fake `CaptureAndHashPane` to call `cancel()` after the first pane is processed (e.g. by counting calls and triggering cancel on the 2nd invocation, or by triggering cancel after the 1st invocation returns).
  - Call `captureAndCommit(ctx, deps)`.
  - Assert:
    - Return value is `nil`.
    - `state.CaptureAndHashPane` was invoked exactly once (for pane 1) — pane 2's iteration starts but observes the cancel before calling `CaptureAndHashPane`. (The exact count depends on cancellation timing; assert ≥ 1 and < N.)
    - `state.Commit` was **not** invoked.
    - `deps.PrevIndex` was **not** replaced with the freshly-captured `idx`.
    - `anyScrollbackChanged` is discarded (assert via the fake's `Commit` non-invocation — if `Commit` were called, the boolean would have flowed in).
- Add a second test "uncancelled multi-pane fixture processes all panes and commits" — regression guard ensuring the new select does not short-circuit on live ctx.
- Run `go test ./cmd/...` and confirm green.

**Acceptance Criteria**:
- [ ] `captureAndCommit` checks `ctx.Done()` at the first statement inside the innermost pane loop body.
- [ ] On cancellation observed mid-loop, the function returns `nil`.
- [ ] `state.Commit` is not invoked when cancellation is observed mid-loop.
- [ ] `deps.PrevIndex` is not replaced when cancellation is observed mid-loop.
- [ ] Per-pane scrollback writes for completed iterations are **not** rolled back (spec explicitly does not require this; document in the comment at the observation site).
- [ ] A unit test in `cmd/state_daemon_test.go` named to describe "cancel mid-loop after k of N panes" pins the behaviour on a multi-pane fixture.
- [ ] A regression-guard test confirms uncancelled multi-pane runs still commit fully.
- [ ] All Phase 2 prior-task tests (Tasks 2-1, 2-2, 2-3) remain green.

**Tests**:
- `"captureAndCommit observes cancellation between per-pane iterations on a multi-pane fixture"` — primary mid-loop cancellation assertion.
- `"captureAndCommit cancelled mid-loop does not invoke state.Commit"` — pin no-commit invariant.
- `"captureAndCommit cancelled mid-loop does not replace PrevIndex"` — pin no-`PrevIndex`-replacement invariant.
- `"captureAndCommit cancelled mid-loop does not roll back already-written per-pane scrollback (atomic writes preserved)"` — pin spec's explicit non-rollback semantics; assert by inspecting test-fixture filesystem after the cancelled call.
- `"captureAndCommit on uncancelled multi-pane fixture processes all panes and commits exactly once"` — regression guard.

**Edge Cases**:
- **Per-pane scrollback writes for completed iterations**: spec §Change 2 says "no half-applied scrollback writes, no partial commit." The "no half-applied scrollback writes" refers to a single pane's scrollback being written half (impossible — `WriteScrollbackIfChanged` uses `AtomicWrite` semantics, the file either fully lands or does not). The "no partial commit" refers to `sessions.json` being committed when only some panes contributed to the index. Per-pane scrollback files for the first `k` panes that successfully completed `WriteScrollbackIfChanged` **stay on disk** — they are atomically valid in isolation. This is the spec's intended behaviour per the task table's edge-case note ("per-pane writes are atomic; spec requires no partial commit of sessions.json, not rollback of per-pane scrollback files"). Document this in the test comment.
- **`anyScrollbackChanged` accumulator**: on cancellation, the accumulator is discarded. The next un-cancelled tick will re-evaluate dedup against the current on-disk hash map; any pane whose scrollback was written by the cancelled tick will be deduped (its hash now matches what's on disk).
- **Cancellation between session/window/pane loop levels**: the third observation point sits at the innermost (pane) loop. Cancellation observed at pane 0 of window 1 of session 2 vs. pane N of window M of session K is the same exit code path — no need for additional observation points at the outer loops because the per-pane check fires on the next iteration regardless of which session/window it's nested in.
- **Recycle-induced sweep pressure** (spec §Defect 2 self-amplifying property): back-to-back `session-closed`/`session-created` hooks fire `save.requested` events, pushing the daemon into a back-to-back sweep regime. The mid-loop observation point must remain interruptible under this pressure — i.e. the check is non-blocking (`select { default: }`), not a poll/sleep. Confirmed by the existing pattern; do not introduce any blocking variant.

**Context**:
> Spec §Change 2 (Cancellation semantics): "`ctx.Done()` is observed at three points inside `captureAndCommit`: ... 3. Between per-pane iterations — covers cancellation during the bulk of the work."
>
> Spec §Change 2: "This caps worst-case daemon-exit latency at one pane's `capture-pane` wall time rather than 'all panes' aggregated wall time' — bounded by per-pane scrollback size, no longer by the user's total pane count."
>
> Spec §Change 2 (Cancellation semantics): "On cancellation, return early without committing partial state. The current tick is abandoned cleanly — no half-applied scrollback writes, no partial commit."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 2 (Cancellation semantics, point 3)

---

## saver-kill-respawn-loop-leaks-daemons-2-5 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-5: Integration test — daemon mid-tick + SIGHUP exits within bounded window (real-tmux fixture, multi-pane synthetic scrollback)

**Problem**: The unit tests added in Tasks 2-2, 2-3, and 2-4 pin the three cancellation observation points individually against fakes, but they do not pin the user-visible end-to-end contract: "on a real tmux server with a real daemon mid-tick, SIGHUP causes the daemon process to exit within a bounded window." Without this integration test, the Defect 2 responsiveness contract is unprotected — a future refactor could re-introduce a blocking subprocess call inside `captureAndCommit`'s pre-loop work and the unit tests would not catch it.

**Solution**: Add a real-tmux integration test (using the existing `tmuxtest` real-tmux socket fixture) that: (a) bootstraps a real `_portal-saver` with the daemon running, (b) loads multiple panes with synthetic scrollback content sized to push aggregate per-tick wall time above 2 seconds, (c) forces a `save.requested` to trigger a tick, (d) sends SIGHUP to the daemon mid-tick, (e) asserts the daemon process exits within a bounded window. The threshold is initially set at **2s heuristic** (per spec §Testing Requirements > Integration tests #2: "target: under 2s on the test fixture"), but the implementation MUST take a fresh wall-time measurement of one pane's `capture-pane` invocation against the test fixture and either confirm 2s or adjust the threshold accordingly.

**Outcome**: A new integration test in the existing real-tmux integration test file pins Defect 2's responsiveness contract. The threshold value is anchored to a fresh empirical measurement taken during implementation, not the heuristic 2s. The test exits cleanly under the 5s `killBarrierTimeout` (unchanged) on the test fixture.

**Do**:
- Locate the existing real-tmux integration test file for the daemon (search `cmd/` for `*_integration_test.go` referencing `state daemon` or `defaultDaemonRun`; if none exists, search `cmd/bootstrap/` and `internal/tmux/` — likely landing in `cmd/state_daemon_integration_test.go` as a new file, or extending `internal/tmux/portal_saver_integration_test.go` if shared helpers there suit).
- Apply the same build-tag/env-var guard used by the existing real-tmux integration tests (e.g. `//go:build integration` or `TMUX_INTEGRATION=1`); do not run on the default `go test` path if other real-tmux tests are gated.
- Add a new test function (suggested name: `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow`). Do **not** use `t.Parallel()`.
- Test setup:
  1. Allocate fresh state dir under `t.TempDir()`.
  2. Start a tmux server on a private socket (via the `tmuxtest` fixture).
  3. Launch the daemon: spawn `portal state daemon` as a subprocess of the test, pointed at the state dir (use the same launching idiom as the existing daemon integration tests; if none exists, build the binary via `go build` to a temp path and exec it with appropriate env).
  4. Wait until `state.DaemonAlive(stateDir)` returns true and the daemon holds `daemon.lock` (poll with 50ms tick, 5s ceiling).
  5. Create multiple panes on the tmux fixture (e.g. 8 panes is enough on most fixtures, but allow scaling; the test should produce per-tick aggregate wall time clearly above 2s to ensure mid-tick SIGHUP is non-trivial).
  6. Load each pane with synthetic scrollback content — write a script (or `tmux send-keys`) that emits several megabytes of text into each pane's history. Use `tmux capture-pane -p -S - <target> | wc -c` to verify each pane's scrollback is large enough that `capture-pane` is non-instantaneous.
  7. **Take a fresh wall-time measurement** of one pane's `capture-pane -e -p -S -` invocation on the test fixture. Record this value as `singlePaneWallTime`. If `singlePaneWallTime` is > 2s, adjust the threshold to `2 * singlePaneWallTime` (one pane in flight + one buffer). If `singlePaneWallTime` is < 2s, keep the 2s threshold. **Document the measurement in a code comment** at the threshold declaration so future readers see the anchor.
- Test action:
  1. Trigger a tick: `os.WriteFile(state.SaveRequested(stateDir), nil, 0o644)`.
  2. Wait briefly (e.g. 1.2s) to allow the ticker (1s period) to fire and the tick to start.
  3. Record `tickStart := time.Now()`.
  4. Send SIGHUP to the daemon process: `daemonProcess.Signal(syscall.SIGHUP)`.
  5. Wait for the daemon process to exit (via `Process.Wait`).
  6. Record `exitTime := time.Now()`.
- Test assertions:
  1. `exitTime.Sub(tickStart) < threshold` where `threshold` is the value anchored above (2s heuristic OR `2 * singlePaneWallTime`, whichever is larger).
  2. `exitTime.Sub(tickStart) < 5 * time.Second` (the unchanged `killBarrierTimeout`) — pins the responsiveness within the existing barrier without changing it.
  3. The daemon process's exit status is zero (clean shutdown).
- Teardown:
  - `tmux kill-server` on the test socket via `t.Cleanup()`.
  - If the daemon process is still alive after the test (latency overshoot), kill it via SIGKILL and fail the test with a descriptive message.
- Run `go test ./... -tags=integration` (or whatever guard is in use) and confirm green.

**Acceptance Criteria**:
- [ ] A new integration test function exists in a real-tmux integration test file (new or existing as appropriate).
- [ ] The test uses the `tmuxtest` real-tmux socket fixture (or an equivalent real-tmux launching idiom matching the existing daemon-integration patterns).
- [ ] The test does not use `t.Parallel()`.
- [ ] The test loads multiple panes with synthetic scrollback sized so that per-tick aggregate wall time clearly exceeds 2s.
- [ ] The test takes a fresh wall-time measurement of one pane's `capture-pane` invocation and anchors the threshold to that measurement (documented in a code comment).
- [ ] The test asserts the daemon process exits within the anchored threshold after SIGHUP.
- [ ] The test asserts the exit happens within the unchanged 5s `killBarrierTimeout`.
- [ ] The test asserts a clean (zero-status) exit.
- [ ] Existing tests in the same package remain green.
- [ ] `killBarrierTimeout` is unchanged in production code.

**Tests**:
- `"daemon mid-tick + SIGHUP exits within bounded window anchored to one pane's capture-pane wall time"` — primary responsiveness assertion.
- `"daemon mid-tick + SIGHUP exits with clean status (zero)"` — pin the clean-shutdown invariant.
- `"daemon mid-tick + SIGHUP exit happens well inside 5s killBarrierTimeout"` — pins acceptance criterion 7 from the spec's Daemon Responsiveness section.
- `"recycle-induced sweep pressure does not block cancellation"` — optional: load `save.requested` repeatedly during the tick to simulate the back-to-back `session-closed`/`session-created` regime described in spec §Defect 2 self-amplifying property; assert the bounded-window exit still holds.

**Edge Cases**:
- **Threshold confirmation/adjustment from fresh measurement**: do not blindly hardcode 2s. The spec is explicit: "The 2s figure is a heuristic threshold, not anchored to a fresh measurement — no fresh wall-time measurement of one pane's `capture-pane` invocation against a representative scrollback fixture was taken during the investigation. Implementation should take that measurement and either confirm 2s as appropriate or adjust the threshold from the measurement." Document the measurement in a code comment.
- **Recycle-induced sweep pressure**: the optional final test variant exercises the self-amplifying property described in spec §Defect 2. Even when `save.requested` keeps firing, the mid-loop observation point should still cancel within the bounded window.
- **Exit bounded under 5s `killBarrierTimeout`**: this is the load-bearing acceptance — the responsiveness fix's purpose is to keep daemon exit inside the existing barrier. If the threshold derived from `singlePaneWallTime` exceeds 5s, the test fixture is unrealistic for the bug being fixed; halve the per-pane scrollback size and re-measure.
- **`killBarrierTimeout` stays at 5s**: do not modify `killBarrierTimeout`'s value in this task. If the test cannot meet the threshold, the production code is wrong, not the timeout.
- **Process lifecycle on test failure**: if the daemon process does not exit, the test must kill it via SIGKILL and emit a descriptive failure message. Leaking daemon processes between integration test runs corrupts subsequent test fixtures.
- **Build-tag/env-var guard parity**: match whatever guard the existing real-tmux integration tests use. If none exists, the test is safe to run on the default path provided tmux is available — but document the dependency at the top of the file.

**Context**:
> Spec §Testing Requirements > Integration tests #2: "'Daemon mid-tick, SIGHUP arrives' → on a fixture with multiple panes loaded with synthetic scrollback, send SIGHUP while a tick is in progress. The daemon process exits within a bounded window (target: under 2s on the test fixture). The 2s figure is a heuristic threshold, not anchored to a fresh measurement — no fresh wall-time measurement of one pane's `capture-pane` invocation against a representative scrollback fixture was taken during the investigation. Implementation should take that measurement and either confirm 2s as appropriate or adjust the threshold from the measurement. Pins Defect 2's responsiveness contract."
>
> Spec §Defect 2 self-amplifying property: "The kill-respawn path itself emits `session-closed` and `session-created` hooks; both fire `save.requested` events on the surviving daemon, pushing it into a back-to-back sweep regime. This widens the cancel-to-exit window precisely on the recycle path the barrier is meant to defend. Change 2's ctx-aware loop must remain interruptible under this pressure."
>
> Spec §Acceptance Criteria > Daemon responsiveness #7: "SIGHUP-to-exit latency is bounded by one pane's capture wall time. When `tmux kill-session -t _portal-saver` is issued mid-tick on a real-tmux fixture with many panes, the daemon process exits within 'one pane's `capture-pane` wall time' of receiving SIGHUP — empirically verifiable inside the 5s `killBarrierTimeout` on the affected user's profile."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Testing Requirements > Integration tests #2, §Acceptance Criteria > Daemon responsiveness

---

## saver-kill-respawn-loop-leaks-daemons-2-6 | approved

### Task saver-kill-respawn-loop-leaks-daemons-2-6: Fault-injection integration test — lock-loser daemon's pane exit destroys `_portal-saver` (cascade regression guard)

**Problem**: Phase 1 eliminates the *natural* trigger of the kill-respawn-loop cascade (the kill-respawn path no longer fires on healthy bootstrap). Phase 2's responsiveness fix caps daemon-exit latency under SIGHUP. But the cascade chain itself — "lock-loser daemon exits cleanly without writing pidfile/version → its pane's initial process exits normally → tmux destroys the `_portal-saver` session → immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing `no such session`" — is still reachable via forced lock contention. Without a permanent regression guard on this chain, a future refactor could re-introduce a path that takes the cascade and unit tests would not catch it.

**Solution**: Add a real-tmux integration test that uses **fault injection** to force the lock-contention scenario (a sentinel goroutine in the test holds `daemon.lock` via `state.AcquireDaemonLock`), then invokes `BootstrapPortalSaver`. The test asserts the full cascade chain remains observable: the new daemon exits cleanly within ~1s, `tmux has-session -t _portal-saver` returns failure after the daemon process exits, and the immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing the substring `no such session`. Post-fix, this test continues to pass because forced lock contention remains a reachable condition — only the natural trigger is eliminated.

**Outcome**: A permanent regression-guard integration test pins the cascade chain. The test uses the fault-injection harness described in spec §Testing Requirements > Integration tests #3. The regression-watch suites from adjacent closed bugfixes (`multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`) remain green.

**Do**:
- Locate the existing real-tmux integration test file for `portal_saver` (`internal/tmux/portal_saver_integration_test.go`, confirmed by Task 1-6 — the cascade scenario sits naturally alongside the other saver integration tests).
- Apply the same build-tag/env-var guard used by the existing real-tmux integration tests there.
- Add a new test function (suggested name: `TestBootstrapPortalSaver_LockContention_CascadeChainReachable`). Do **not** use `t.Parallel()`.
- Test setup:
  1. Allocate a fresh state dir under `t.TempDir()`.
  2. Start a tmux server on a private socket (via the `tmuxtest` fixture).
  3. **Sentinel goroutine**: start a goroutine that calls `state.AcquireDaemonLock(stateDir)` and holds the returned `*os.File` for the duration of the test. The goroutine must signal readiness via a `chan struct{}` before the main test goroutine proceeds, so the lock is provably held before `BootstrapPortalSaver` runs. Use `t.Cleanup()` to close the sentinel's `*os.File` after the test (releasing the flock).
  4. Confirm `state.AcquireDaemonLock(stateDir)` from the main test goroutine would return `state.ErrDaemonLockHeld` (optional sanity check; do not consume the lock).
- Test action:
  1. Invoke `BootstrapPortalSaver(client, stateDir, currentVersion)` (where `currentVersion` is a non-dev, non-empty string).
  2. The new daemon process spawned by `BootstrapPortalSaver` will start, attempt to acquire `daemon.lock`, fail with `ErrDaemonLockHeld`, log one WARN line, and exit status 0 (the lock-loser path).
  3. The daemon's pane process exits normally; tmux destroys the `_portal-saver` session because its only pane's initial process exited normally (not because of `destroy-unattached`).
- Test assertions (the cascade chain):
  1. **The new daemon's process exits cleanly within a bounded window** — poll with 50ms tick, 1s ceiling, until the spawned daemon process is no longer alive (e.g. via `process.Signal(syscall.Signal(0))` returning an error, or by waiting on the subprocess if available).
  2. **`tmux has-session -t _portal-saver` returns failure after the daemon process exits** — poll with 100ms tick, 2s ceiling. Use `client.HasSession("_portal-saver")` or the equivalent.
  3. **The immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing the substring `no such session`** — call `client.SetSessionOption("_portal-saver", "destroy-unattached", "off")` and assert the returned error is non-nil, its message contains `exit status 1`, and the underlying tmux stderr contains `no such session`.
- Teardown:
  - Close the sentinel goroutine's `*os.File` to release the flock (via `t.Cleanup()`).
  - `tmux kill-server` on the test socket via `t.Cleanup()`.
- Confirm the regression-watch suites listed below remain green by running their packages in the same `go test` invocation:
  - `multiple-state-daemons-running-concurrently` tests (likely in `cmd/` and `internal/state/` — search for `AcquireDaemonLock` test files).
  - `daemon-merge-reintroduces-dead-sessions` tests (search for `daemon-merge` or merge-pipeline tests in `internal/state/`).
  - `killed-sessions-resurrect-on-restart` tests (search for `restore`/`resurrect` tests in `internal/restore/` and `cmd/bootstrap/`).
- Run `go test ./...` (with appropriate integration guard) and confirm all green.

**Acceptance Criteria**:
- [ ] A new integration test function exists in `internal/tmux/portal_saver_integration_test.go` (or the equivalent file housing the existing real-tmux saver tests).
- [ ] The test uses fault injection: a sentinel goroutine in the test holds `daemon.lock` via `state.AcquireDaemonLock` before `BootstrapPortalSaver` is invoked.
- [ ] The test does not use `t.Parallel()`.
- [ ] The sentinel goroutine releases the lock via `t.Cleanup()` so subsequent tests are unaffected.
- [ ] The test asserts the lock-loser daemon process exits cleanly within ~1s (poll: 50ms tick, 1s ceiling).
- [ ] The test asserts `tmux has-session -t _portal-saver` returns failure after the daemon process exits (poll: 100ms tick, 2s ceiling).
- [ ] The test asserts `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing the substring `no such session`.
- [ ] The regression-watch suites listed above all remain green.

**Tests**:
- `"sentinel holding daemon.lock forces lock-loser path for BootstrapPortalSaver-spawned daemon"` — primary fault-injection assertion.
- `"lock-loser daemon exits cleanly within 1s"` — pin daemon exit latency.
- `"tmux has-session -t _portal-saver returns failure after lock-loser daemon's pane exit"` — pin the session destruction step.
- `"SetSessionOption(_portal-saver, destroy-unattached, off) returns exit status 1 with 'no such session' on the cascade path"` — pin the final cascade observation.
- `"regression-watch: multiple-state-daemons-running-concurrently tests remain green"` — implicit via test run.
- `"regression-watch: daemon-merge-reintroduces-dead-sessions tests remain green"` — implicit via test run.
- `"regression-watch: killed-sessions-resurrect-on-restart tests remain green"` — implicit via test run.

**Edge Cases**:
- **Sentinel goroutine readiness signalling**: the sentinel goroutine must signal it has acquired the lock before the main goroutine invokes `BootstrapPortalSaver`. Use a `chan struct{}` closed-on-ready pattern; do not rely on `time.Sleep`.
- **Lock release on teardown**: closing the sentinel's `*os.File` is what releases the kernel-side flock. The `t.Cleanup()` closure must be registered before any code path that could `t.Fatal`/`t.Skip`, otherwise the lock would leak across tests.
- **`has-session` polling**: 100ms tick × 2s ceiling matches spec §Testing Requirements > Integration tests #3 verbatim. Do not adjust the ratios — they are tuned to the cascade chain's observable timing.
- **`SetSessionOption` exact error shape**: the spec specifies the error contains both `exit status 1` and `no such session`. Both substrings must be asserted independently (an exit-status-1 from an unrelated cause must not cause a false positive).
- **Sentinel daemon's lifetime**: the sentinel goroutine must hold the lock until `t.Cleanup` releases it, even if the main test goroutine fails or panics. Using `t.Cleanup` to close the `*os.File` is the correct idiom because Go's testing harness invokes cleanup callbacks even on `t.Fatal`.
- **Regression-watch suites running in the same `go test` invocation**: do not stub or skip the adjacent-bugfix tests. They must run end-to-end and stay green. If discovering them requires `git log --all --grep=<topic>` per spec §Regression preservation, do that discovery during implementation.
- **Process discovery for the lock-loser daemon**: if `BootstrapPortalSaver` spawns the daemon as a detached subprocess (via the tmux `new-session -d` idiom), the test does not have a direct `*os.Process` handle. In that case, poll for daemon exit by checking `state.DaemonAlive(stateDir)` returning false instead of using `process.Signal(0)`.

**Context**:
> Spec §Testing Requirements > Integration tests #3: "'Lock-loser daemon's pane exit destroys `_portal-saver` session' → uses fault injection to force the lock contention scenario: a sentinel process holds `daemon.lock` (via `state.AcquireDaemonLock` in a test goroutine), then `BootstrapPortalSaver` is invoked. The test asserts the chain: The new daemon's process exits cleanly within a bounded window (target: under 1s). `tmux has-session -t _portal-saver` returns failure after the daemon process exits (poll with 100ms tick, 2s ceiling). The immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing the substring `no such session`. Observation mechanism: combination of process-wait + `has-session` polling + direct call assertion. Post-fix, this test continues to pass because the conditions for the cascade (forced lock contention) remain reachable via the fault-injection harness — only the natural trigger (kill-barrier giving up early) is eliminated. The test is a permanent regression guard on the cascade chain, not on the conditions that trigger it."
>
> Spec §Coordination with prior bugfix: "Adjacent closed bugfixes — regression-watch list. The following exercise adjacent daemon/restore surfaces. This bugfix does not touch their logic; their tests should remain green: `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Testing Requirements > Integration tests #3, §Coordination with prior bugfix

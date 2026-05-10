# Plan: Killed Sessions Resurrect on Restart

## Pre-Flight Notes

### Empirical reconfirmation of Symptom A on `main`

Per spec § "Empirical Reconfirmation Before Implementation Starts", the kill → reopen check against current `main` is required before scoping tasks. The planning agent has **deferred this check to the implementer** because the planning environment lacks a real tmux + Portal cold-start fixture; the spec's branch-behaviour contract is preserved by carrying both branches as conditional plan scope below.

**Required action before Phase 1 starts** — the implementer runs the verification command and records the outcome here, then applies the matching branch:
- *Neutralised*: Symptom A does not reproduce on `main` (the companion daemon-merge live-set filter is in effect). AC3 remains a regression guard and is satisfied by existing coverage in `internal/state/capture_test.go` filter tests and `cmd/bootstrap/stale_marker_cleanup_test.go`. No additional Symptom-A-specific task is added; Phase 1 task count stays at 8.
- *Still reproduces*: Symptom A reproduces on `main`. AC3 graduates to "verified fix"; an explicit regression test is added as task `killed-sessions-resurrect-on-restart-1-9` (kill → reopen → assert absent). Phase 1 task count increases to 9 and Phase 1 acceptance is updated to include AC3 verification.

**Outcome**: [TO BE FILLED BY THE IMPLEMENTER BEFORE PHASE 1 STARTS]

**Verification command**: Boot a tmux server, `portal open` a saved session via Portal, kill the session via TUI `K`, then `portal open` again and confirm whether the killed session reappears in the list.

**Relationship to fix scope**: Either branch ships Fix 1 / Fix 2 / Fix 3 unchanged — reconfirmation only affects whether a Symptom-A-specific test task is added, not whether the upstream-trigger fix proceeds.

## Phases

### Phase 1: Eager-Signal Hydrate Step (Root-Cause Fix)
status: approved
approved_at: 2026-05-10

**Goal**: Insert a new bootstrap orchestrator step between Restore and Clear `@portal-restoring` that writes the hydrate signal byte to every freshly-armed skeleton pane's FIFO, eliminating the per-session signaling gap that leaves N−1 sessions' helpers waiting for a signal that never arrives.

**Why this order**: This is the architectural root-cause fix. Phases 2 and 3 are defensive corrections at code sites whose semantics change once eager signaling makes the timeout path rare rather than steady-state. Landing Fix 1 first means Phases 2/3 reason about exceptional, not common, behaviour.

**Acceptance**:
- [ ] `writeFIFOSignal` and `signalHydrateRetryDelays` are relocated from `cmd` into `internal/state` with no public API surface added; `cmd/state_signal_hydrate.go` and the new bootstrap step both call into the shared package.
- [ ] `EagerHydrateSignaler` seam interface is defined in `cmd/bootstrap` with a production adapter wired in `internal/bootstrapadapter` against `state.ListSkeletonMarkers` and `state.WriteFIFOSignal`.
- [ ] The new `EagerSignalHydrate` step runs after step 5 (Restore) and before step 6 (Clear `@portal-restoring`) — verified by an orchestrator ordering test.
- [ ] Per-FIFO write failures log a soft warning of shape `WARN | hydrate | eager-signal: write fifo <path>: <error>` and continue; the step never escalates to a fatal bootstrap error.
- [ ] Zero-marker post-Restore is a no-op — no FIFO writes attempted, step returns nil.
- [ ] Multi-session integration test (real tmux, N≥2 saved sessions): `state.ListSkeletonMarkers` returns empty within a 2-second poll window after bootstrap (AC1).
- [ ] AC4 verified end-to-end: a daemon capture tick post-eager-signal produces a non-empty scrollback dump for a previously-non-attached session's pane (task 1-8).
- [ ] AC8 invariant preserved: daemon `captureAndCommit` suppression during the `@portal-restoring` window is intact; no race introduced between the eager step and helper-driven scrollback replay.
- [ ] `CLAUDE.md` "Server bootstrap" section updated in the same change with renumbered step list and one-paragraph `EagerSignalHydrate` description.
- [ ] All existing happy-path resurrection integration tests and companion daemon-merge fix tests remain green.

#### Tasks
status: approved
approved_at: 2026-05-10

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-sessions-resurrect-on-restart-1-1 | Relocate writeFIFOSignal and signalHydrateRetryDelays into internal/state | ENXIO/EAGAIN retry ladder preserved verbatim, ENOENT surfaces immediately, retries-exhausted wrapping unchanged |
| killed-sessions-resurrect-on-restart-1-2 | Repoint cmd/state_signal_hydrate.go at the shared internal/state writer | nil logger no-op, list-markers failure soft-warns and returns nil, per-pane write failure does not abort sibling panes |
| killed-sessions-resurrect-on-restart-1-3 | Define EagerHydrateSignaler seam and EagerSignalHydrate step in cmd/bootstrap | zero-marker no-op returns nil with no FIFO writes, per-FIFO write failure logs WARN and continues, step never escalates to fatal |
| killed-sessions-resurrect-on-restart-1-4 | Insert EagerSignalHydrate into Orchestrator between Restore and Clear @portal-restoring | runs while @portal-restoring still set (AC8), runs after Restore populates marker set, ordering test asserts position 6 |
| killed-sessions-resurrect-on-restart-1-5 | Wire production EagerHydrateSignaler adapter in internal/bootstrapadapter | stateDir resolved once at orchestrator construction, FIFOPath derivation per paneKey, no new public API surface |
| killed-sessions-resurrect-on-restart-1-6 | Multi-session cold-start integration test asserting empty marker set within 2s (AC1) | N>=2 saved sessions, polls state.ListSkeletonMarkers, no client attach required to drive unset |
| killed-sessions-resurrect-on-restart-1-7 | Update CLAUDE.md Server bootstrap section with renumbered 10-step list and EagerSignalHydrate paragraph | preserve "Return is the post-step boundary" framing, renumber subsequent steps, one-paragraph step description |
| killed-sessions-resurrect-on-restart-1-8 | Integration test asserting daemon captureAndCommit resumes for previously-stuck-marker panes (AC4) | sub-test extends task 1-6's file under //go:build integration, capture tick must run post-Clear @portal-restoring, expose state.RunCaptureOnce as a test seam if not present |

### Phase 2: Timeout-Path Recovery Corrections
status: approved
approved_at: 2026-05-10

**Goal**: Rewrite `handleHydrateTimeout` in `cmd/state_hydrate.go` from a leaky bypass into a correct recovery path — unset the `@portal-skeleton-<paneKey>` marker and route the fall-through through `execShellOrHookAndExit` so hooks fire on the timeout path.

**Why this order**: With Phase 1 in place, the timeout path is now exceptional rather than steady-state, so this phase converges the timeout and file-missing recovery paths onto the same exec contract. Sequencing after Phase 1 means tests assert behaviour against the post-eager-signaling steady state where timeout fires only on genuine signal-flow bugs.

**Acceptance**:
- [ ] `handleHydrateTimeout` calls `unsetSkeletonMarkerOrLog` (`state.UnsetSkeletonMarkerForFIFO` under the hood) before exec; failure is a soft warning, not a block on shell exec.
- [ ] Timeout fall-through routes through `execShellOrHookAndExit(cfg.HookKey)` instead of `execShellAndExit`; no new `--hook-key` plumbing is added to `runHydrate`.
- [ ] The "marker stays set so the next attach re-signals" comment at line 262 is removed and replaced with a one-line note describing the recovery contract.
- [ ] The 100 ms settle-sleep before exec is preserved.
- [ ] Unit test asserts `handleHydrateTimeout` invokes the marker-unset primitive (mocked via the existing `state.UnsetSkeletonMarkerForFIFO` seam pattern from `handleHydrateFileMissing` tests).
- [ ] Unit test asserts `runHydrate` timeout fall-through targets `execShellOrHookAndExit` (replicates the file-missing-path test shape).
- [ ] Unit test asserts hook-firing on timeout end-to-end: registered on-resume hook + forced `ErrHydrateTimeout` produces exec target `sh -c '<HOOK>; exec $SHELL'`.
- [ ] Integration test: register an on-resume hook for a non-attached saved session, cold-start, assert the hook ran in the restored pane (AC2 end-to-end).
- [ ] Combined with Phase 1, the behavioural prerequisites for AC6 are met — the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines no longer fire in steady-state cold-start. AC6's observational verification gate is owned by task 3-4 (Manual Verification Protocol step 2); this phase does not close AC6 on its own.
- [ ] Spec supersession is recorded in the killed-sessions spec (lines 156–163) and is satisfied by Phase 2's behavioural changes — task 2-1 supersedes "Helper does NOT unset marker on FIFO timeout" (built-in-session-resurrection spec line 838) and task 2-3 supersedes "Resume hooks fire only at the end of successful hydration" (line 873). No in-place edit of the original spec; no separate planning-side artifact required.

#### Tasks
status: approved
approved_at: 2026-05-10

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-sessions-resurrect-on-restart-2-1 | Flip TestHydrate_TimeoutDoesNotUnsetSkeletonMarker to assert marker-unset on timeout, then make handleHydrateTimeout call unsetSkeletonMarkerOrLog | UnsetSkeletonMarkerForFIFO failure logs soft warning and does not block subsequent exec, paneKey derived from FIFO basename via existing seam, set-option -su argv observed exactly once per timeout |
| killed-sessions-resurrect-on-restart-2-2 | Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note | preserve adjacent FIFO-unlink and warn-log comments verbatim, no behavioural change in this task, comment documents that runHydrate (per task 2-1) owns the 100 ms settle-sleep before exec |
| killed-sessions-resurrect-on-restart-2-3 | Flip TestHydrate_Timeout_NeverFiresHookEvenIfRegistered into TestHydrate_Timeout_FiresHookWhenRegistered, then route runHydrate timeout fall-through through execShellOrHookAndExit | exec target is sh -c '<HOOK>; exec $SHELL' when hook registered, no new --hook-key plumbing added to runHydrate, cfg.HookKey threaded as-is from existing scope |
| killed-sessions-resurrect-on-restart-2-4 | Unit test: runHydrate timeout fall-through with no registered hook still execs bare $SHELL via execShellOrHookAndExit | nil HookStore degrades to bare shell, lookup-not-found degrades to bare shell, lookup-error degrades to bare shell with single WARN |
| killed-sessions-resurrect-on-restart-2-5 | Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance | elapsed time on timeout handler stays well under hydrateSettleSleep (handler does not own the sleep), os.Remove(cfg.FIFO) still tolerates missing FIFO silently, marker-unset call ordered before exec fall-through |
| killed-sessions-resurrect-on-restart-2-6 | Integration test (real tmux): cold-start with non-attached saved session + registered on-resume hook fires end-to-end (AC2) | N>=2 saved sessions where hook is on the non-attached session, hook stdout/effect observable in restored pane, test still passes when eager-signaling (Phase 1) has already cleared markers pre-timeout |

### Phase 3: Drop Outer sh -c Wrapper in buildHydrateCommand
status: approved
approved_at: 2026-05-10

**Goal**: Change `buildHydrateCommand` in `internal/restore/session.go` from emitting `sh -c 'portal state hydrate …; exec $SHELL'` to emitting the bare `portal state hydrate …` form, eliminating the unreachable trailer, the parked `sh` parent on every restored pane, and the double-`exit` bug.

**Why this order**: This is an independent defensive change at the same code-site cluster as Phases 1 and 2 and is bundled per the spec's "treating them in one work product is cheaper than splitting" rationale. It is sequenced last because it has no dependency on Phases 1 or 2, and its acceptance criteria (pane closes on first `exit`, no orphan `sh` parent) are visible only on a fully-restored pane — which Phases 1 and 2 already ensure behaves correctly.

**Acceptance**:
- [ ] `buildHydrateCommand` returns the bare `portal state hydrate --fifo <fifo> --file <file> --hook-key <hookKey>` string with each value-arg shell-escaped via the existing `internal/tmux` quoting helper; no `sh -c` envelope, no `; exec $SHELL` trailer.
- [ ] `RespawnPane` interface signature is unchanged — still accepts a single command-string argument.
- [ ] Unit/snapshot test in `session_test.go` updated to assert the new bare-command shape on representative inputs.
- [ ] Inner `sh -c '<HOOK>; exec $SHELL'` constructed inside `execShellOrHookAndExit` when an on-resume hook is registered is untouched — hook-firing semantics preserved exactly.
- [ ] Integration test: `exit` typed once in a restored pane closes the pane (tmux `list-panes` shows the pane gone, not respawned with a fresh shell) — AC5.
- [ ] Integration / manual check: `pgrep -fa "sh -c.*portal state hydrate"` returns no rows for any restored pane.
- [ ] All existing happy-path resurrection integration tests remain green.
- [ ] Manual Verification Protocol executed on a real machine; pre-fix and post-fix observations recorded in the PR description (DoD item 3, AC6 observational gate via protocol step 2).

#### Tasks
status: approved
approved_at: 2026-05-10

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-sessions-resurrect-on-restart-3-1 | Update buildHydrateCommand snapshot test to assert bare-command shape, then drop the outer `sh -c '…; exec $SHELL'` wrapper in `internal/restore/session.go` | paths/hook-keys containing single quotes still escaped correctly via existing single-quote helper; empty / unset hook-key value still produces a valid bare invocation |
| killed-sessions-resurrect-on-restart-3-2 | Refresh `buildHydrateCommand` doc comment and confirm `RespawnPane` interface signature is unchanged (still single command-string) | none |
| killed-sessions-resurrect-on-restart-3-3 | Add integration test (real-tmux fixture): typed `exit` once in a restored pane closes the pane — `tmux list-panes` shows pane gone, not respawned with a fresh shell (AC5) | restored pane with on-resume hook registered (inner `sh -c '<HOOK>; exec $SHELL'` exec chain unaffected — exit still closes the pane); restored pane without a hook (bare `$SHELL` exec — exit closes the pane); no parked `sh -c .*portal state hydrate` parent process under tmux post-restore |
| killed-sessions-resurrect-on-restart-3-4 | Execute Manual Verification Protocol on a real machine and record pre/post observations in the PR description (DoD item 3, AC6) | N>=2 saved sessions required for pre-fix repro; observational only (no automated test); deferrable to a reviewer with a real machine but DoD-blocking before merge |

## Definition of Done

Per spec § "Definition of Done":

- [ ] All unit and integration tests in the Test Plan pass in CI — covered by Phase 1/2/3 task acceptance criteria.
- [ ] Existing tests under "Regression Coverage to Preserve" remain green — Phase 1 final acceptance criterion.
- [ ] Manual Verification Protocol has been executed once on a real machine; pre-fix and post-fix observations recorded in the PR description — task 3-4.
- [ ] `CLAUDE.md` "Server bootstrap" section is updated with the new step list — task 1-7.
- [ ] PR is reviewed and merged to `main` — out of scope for the planning artifact; tracked on the PR itself.

# Plan: Killed Sessions Resurrect on Restart

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
- [ ] Combined with Phase 1, the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines are absent in steady-state cold-start logs (AC6 fully satisfied).
- [ ] Spec supersession recorded: original `built-in-session-resurrection` invariants at lines 838 and 873 are explicitly superseded by this phase's behaviour (no in-place edit of the original spec).

#### Tasks
status: approved
approved_at: 2026-05-10

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-sessions-resurrect-on-restart-2-1 | Flip TestHydrate_TimeoutDoesNotUnsetSkeletonMarker to assert marker-unset on timeout, then make handleHydrateTimeout call unsetSkeletonMarkerOrLog | UnsetSkeletonMarkerForFIFO failure logs soft warning and does not block subsequent exec, paneKey derived from FIFO basename via existing seam, set-option -su argv observed exactly once per timeout |
| killed-sessions-resurrect-on-restart-2-2 | Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note | preserve adjacent FIFO-unlink and warn-log comments verbatim, no behavioural change in this task, 100 ms settle-sleep absence still documented as deliberate |
| killed-sessions-resurrect-on-restart-2-3 | Flip TestHydrate_Timeout_NeverFiresHookEvenIfRegistered into TestHydrate_Timeout_FiresHookWhenRegistered, then route runHydrate timeout fall-through through execShellOrHookAndExit | exec target is sh -c '<HOOK>; exec $SHELL' when hook registered, no new --hook-key plumbing added to runHydrate, cfg.HookKey threaded as-is from existing scope |
| killed-sessions-resurrect-on-restart-2-4 | Unit test: runHydrate timeout fall-through with no registered hook still execs bare $SHELL via execShellOrHookAndExit | nil HookStore degrades to bare shell, lookup-not-found degrades to bare shell, lookup-error degrades to bare shell with single WARN |
| killed-sessions-resurrect-on-restart-2-5 | Unit test: handleHydrateTimeout preserves the 100 ms settle-sleep absence and FIFO-unlink ordering | elapsed time on timeout handler stays well under hydrateSettleSleep, os.Remove(cfg.FIFO) still tolerates missing FIFO silently, marker-unset call ordered before exec fall-through |
| killed-sessions-resurrect-on-restart-2-6 | Integration test (real tmux): cold-start with non-attached saved session + registered on-resume hook fires end-to-end (AC2) | N>=2 saved sessions where hook is on the non-attached session, hook stdout/effect observable in restored pane, test still passes when eager-signaling (Phase 1) has already cleared markers pre-timeout |
| killed-sessions-resurrect-on-restart-2-7 | Record spec supersession of built-in-session-resurrection lines 838 and 873 in this work unit's planning notes (no in-place edit of the original spec) | original spec file untouched, supersession note links Phase 2 acceptance back to AC2 and AC6, lines 838/873 quoted verbatim with the replaced semantic stated alongside |

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

#### Tasks
status: approved
approved_at: 2026-05-10

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-sessions-resurrect-on-restart-3-1 | Update buildHydrateCommand snapshot test to assert bare-command shape, then drop the outer `sh -c '…; exec $SHELL'` wrapper in `internal/restore/session.go` | paths/hook-keys containing single quotes still escaped correctly via existing single-quote helper; empty / unset hook-key value still produces a valid bare invocation |
| killed-sessions-resurrect-on-restart-3-2 | Refresh `buildHydrateCommand` doc comment and confirm `RespawnPane` interface signature is unchanged (still single command-string) | none |
| killed-sessions-resurrect-on-restart-3-3 | Add integration test (real-tmux fixture): typed `exit` once in a restored pane closes the pane — `tmux list-panes` shows pane gone, not respawned with a fresh shell (AC5) | restored pane with on-resume hook registered (inner `sh -c '<HOOK>; exec $SHELL'` exec chain unaffected — exit still closes the pane); restored pane without a hook (bare `$SHELL` exec — exit closes the pane); no parked `sh -c .*portal state hydrate` parent process under tmux post-restore |

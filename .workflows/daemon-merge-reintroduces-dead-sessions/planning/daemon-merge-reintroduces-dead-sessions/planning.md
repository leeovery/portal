# Plan: Daemon Merge Reintroduces Dead Sessions

## Phases

### Phase 1: Live-set filter in `mergeSkippedPanes` (Fix Component A)
status: approved
approved_at: 2026-05-09

**Goal**: Stop the resurrection symptom at the merge layer by filtering prev panes against the freshly-built `idx.Sessions` at session, window, and pane levels; replace the test that codifies the bug; prove `sessions.json` self-heals on the next daemon tick.

**Why this order**: This single change resolves the user-visible bug — `sessions.json` self-heals once the merge no longer reintroduces dead sessions. The work is internal to `internal/state/capture.go` with no orchestrator or adapter surgery, making it the strongest foundation. Phase 2's cleanup step is explicitly safe to run concurrently with the daemon only because Phase 1 has already neutralised the marker's authority over the merge — sequencing Phase 1 first removes a class of concurrency reasoning from Phase 2.

**Acceptance**:
- [ ] `mergeSkippedPanes` (in `internal/state/capture.go`) filters every prev pane against the fresh `idx.Sessions` at session, window, and pane levels — a paneKey in `skipSet` is only retained when all three structural levels exist in fresh; the public signature of `mergeSkippedPanes` is unchanged and the structural map is built locally from `idx.Sessions`.
- [ ] `mergePane` and `findOrAppendSession` are not given belt-and-braces defensive checks — the merge entry point is the single point of enforcement.
- [ ] The buggy test `TestCaptureStructureMergeSkippedPanes/merges a skipped pane's session and window from prev when missing from fresh` (`internal/state/capture_test.go:570-617`) is replaced with its inverse: a prev session whose name is not in fresh must NOT be merged, even when its paneKey is in `skipSet`.
- [ ] New unit tests cover window-level filtering and pane-level filtering (each a stale marker for a structural level missing in fresh within an otherwise-live parent must be dropped from the merge result).
- [ ] A regression test mirrors the empirical scenario (marker set, session killed, daemon tick → fresh capture must NOT reintroduce the session); the test seeds `prev.Sessions` (or runs an initial tick) so it would fail on the buggy code rather than going false-green via the `prev != nil` gate.
- [ ] Existing happy-path skeleton-marker tests in `internal/restore/session_markers_test.go` and the legitimate hydrate-in-progress merge behaviour remain green; phase-A skeleton-restored panes (marker set, session/window/pane present in tmux) are still merged from prev as expected.
- [ ] The synthetic repro (set marker via `tmux set-option -s @portal-skeleton-<paneKey> 1`, `tmux kill-session`, wait one daemon tick) does not reintroduce the killed session into `~/.config/portal/state/sessions.json`; on the next tick after a previously-polluted commit, `sessions.json` self-heals.
- [ ] `go test ./...` passes.

#### Tasks
status: draft

| ID | Task | Summary | Edge cases |
|----|------|---------|------------|
| daemon-merge-reintroduces-dead-sessions-1-1 | Replace codifying-bug test with session-level filter + add session-level filter | Invert the buggy test at `internal/state/capture_test.go:570-617` so a prev session not in fresh is NOT merged despite its paneKey being in `skipSet`; build the local fresh-structural map inside `mergeSkippedPanes` and gate session merging on session-name presence in the fresh index. | prev session name absent from fresh; paneKey present in skipSet; legitimate hydrate-in-progress (all three levels live) still merges; `mergePane`/`findOrAppendSession` signatures unchanged |
| daemon-merge-reintroduces-dead-sessions-1-2 | Add window-level filtering to `mergeSkippedPanes` | Extend the local fresh-structural map and filter so a prev pane whose session is live but whose window index is missing from fresh is dropped from the merge result, even when its paneKey is in `skipSet`. | live session with stale window; mixed live/stale windows in the same prev session; canonical ordering preserved after partial drop; helpers untouched |
| daemon-merge-reintroduces-dead-sessions-1-3 | Add pane-level filtering to `mergeSkippedPanes` | Extend the structural map and filter so a prev pane whose session and window are live but whose pane index is missing from fresh is dropped from the merge result, even when its paneKey is in `skipSet`. | live window with stale pane index; existing pane-index replacement contract preserved; no defensive checks added inside `mergePane` / `findOrAppendSession` |
| daemon-merge-reintroduces-dead-sessions-1-4 | Add empirical-scenario regression test (kill-mid-flight self-heal) | Add a regression test mirroring the in-the-wild repro: seed `prev.Sessions` (or run an initial tick) with the session, mark its paneKey in `skipSet`, drop it from the fresh enumeration (kill), assert the killed session is NOT reintroduced and the polluted prev self-heals on a follow-up tick. | prev-population precondition is load-bearing (without it test passes on buggy code via `prev != nil` gate); marker stays in `skipSet` throughout; two-tick self-heal sequence |
| daemon-merge-reintroduces-dead-sessions-1-5 | Preserve hydrate-in-progress merge behaviour (positive test) | Add or extend a positive test asserting that a phase-A skeleton-restored pane (marker set, session/window/pane all present in fresh) is still merged from prev with prev's authoritative pane state; confirm `internal/restore/session_markers_test.go` and remaining `TestCaptureStructureMergeSkippedPanes` subtests stay green. | prev pane state (CWD/CurrentCommand) wins at matching coords; sessions present in both fresh and prev not duplicated; canonical ordering survives merge |

### Phase 2: Bootstrap stale-marker cleanup step (Fix Component B)
status: approved
approved_at: 2026-05-09

**Goal**: Add a new bootstrap step between current step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs) that unsets `@portal-skeleton-*` markers whose paneKey has no corresponding live pane, closing the silent scrollback-save gap (`cmd/state_daemon.go:131-133`) and preventing indefinite marker accumulation across the tmux server's lifetime.

**Why this order**: Phase 1 has already resolved the user-visible resurrection symptom and established the live-truth invariant in the merge layer. With that invariant in place, the cleanup step requires no serialisation against the daemon — concurrent reads of a marker about to be unset cannot resurrect a dead session. Sequencing this phase second lets it focus on its own narrow surface (orchestrator step, seam, adapter, paneKey normalisation correctness) without re-litigating merge-layer concerns. The phase also depends on tests written in Phase 1 staying green, confirming no regression to hydrate-in-progress merge behaviour.

**Acceptance**:
- [ ] A new bootstrap step is inserted in the orchestrator (`cmd/bootstrap/`) between current step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs); subsequent steps renumber accordingly. The implementation file lives in `cmd/bootstrap/` with co-located `_test.go`.
- [ ] The step enumerates markers via `state.ListSkeletonMarkers` (canonical paneKey form), enumerates live panes via the **error-propagating** `(*tmux.Client).ListAllPanesWithFormat` with format `#{session_name}:#{window_index}.#{pane_index}` — `ListAllPanes` is **not** used — and converts each live-pane line to canonical paneKey form via `state.SanitizePaneKey` (rightmost-`:` split, then `.` split, `strconv.Atoi` for window/pane) before computing the set difference.
- [ ] Mass-unset hazard guard: the cleanup skips the unset pass and emits a soft warning when live-pane enumeration returns an error OR returns zero panes; malformed `session:window.pane` lines are skipped (not added to the live-pane set, not aborting cleanup) with a soft warning when unexpected.
- [ ] Stale markers (paneKey not in the live-pane set) are unset via `(*tmux.Client).UnsetServerOption("@portal-skeleton-" + paneKey)` using the existing `SkeletonMarkerPrefix` constant.
- [ ] Failure modes (tmux unavailable, individual unset error, live-pane enumeration error) degrade to soft warnings collected by the orchestrator and drained post-bootstrap, mirroring `CleanStale`'s soft-warning posture; the step never escalates to a fatal abort.
- [ ] A new orchestrator seam interface is exposed in `cmd/bootstrap/` (one composite or three small interfaces — implementation choice, consistent with existing bootstrap conventions) with marker enumeration, live-pane enumeration, and marker-unset responsibilities each independently mockable. The production adapter is wired in `internal/bootstrapadapter/`.
- [ ] Unit tests (co-located with the new step) cover: stale marker unset; live marker preserved; **paneKey normalisation correctness** with a fixture mixing tmux's `session:window.pane` form (live-pane side) with canonical `session__window.pane` form (marker side), asserting the same logical pane is recognised across both sides; a complementary negative test where two paneKeys differ only by separator must NOT be treated as equivalent; zero-live-pane guard; malformed-line skip.
- [ ] Bootstrap integration tests in `cmd/bootstrap/bootstrap_test.go` assert the new step runs between step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs), and that the step degrades to a warning on tmux failure matching the soft-warning posture of `CleanStale`. Production-adapter wiring is covered in `internal/bootstrapadapter/adapters_test.go`.
- [ ] After a successful bootstrap that did not surface a soft warning, no `@portal-skeleton-*` marker exists for a paneKey that has no live pane.
- [ ] Scrollback-save resumption: after the cleanup unsets a stale marker whose underlying pane is still live (the leaked-marker-but-pane-retained case), the next daemon tick saves scrollback for that pane (the skip-save guard at `cmd/state_daemon.go:131-133` no longer applies).
- [ ] No locks or sequencing constraints are added between the cleanup step and the daemon; the marker-set path (`internal/restore/session.go:380-384`) and the hydrate-helper unset path (`cmd/state_hydrate.go:312`) are not modified.
- [ ] `go build -o portal .` and `go test ./...` pass.

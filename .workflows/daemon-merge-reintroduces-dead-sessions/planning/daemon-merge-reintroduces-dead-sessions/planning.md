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
status: approved
approved_at: 2026-05-09

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

#### Tasks
status: approved
approved_at: 2026-05-09

| ID | Task | Summary | Edge cases |
|----|------|---------|------------|
| daemon-merge-reintroduces-dead-sessions-2-1 | Introduce stale-marker cleanup seam and happy-path implementation | Define the orchestrator seam in `cmd/bootstrap/` (marker enumeration + live-pane enumeration + marker unset, independently mockable per existing convention) and implement the cleanup function that diffs canonical-paneKey markers against live-pane paneKeys sanitised via `state.SanitizePaneKey`, unsetting stale markers via `SkeletonMarkerPrefix + paneKey`. Co-located unit test covers stale-marker unset and live-marker preservation. | stale marker unset; live marker preserved; canonical paneKey on both sides; `mergePane` / `findOrAppendSession` and marker-set / hydrate-helper-unset paths untouched |
| daemon-merge-reintroduces-dead-sessions-2-2 | Add paneKey normalisation correctness fixture | Unit fixture mixing tmux `session:window.pane` form (live-pane side) with canonical `session__window.pane` form (marker side) asserting the same logical pane is recognised across both; complementary negative test where two paneKeys differ only by separator must NOT be treated as equivalent. | rightmost-`:` split for session names containing `:`; `strconv.Atoi` parse for window and pane; positive recognition across separators; negative non-equivalence guard against naive string equality |
| daemon-merge-reintroduces-dead-sessions-2-3 | Mass-unset hazard guard for zero-panes and enum errors | Cleanup skips the unset pass and emits a soft warning when `ListAllPanesWithFormat` returns an error OR returns zero panes; markers remain untouched in both branches; guard precedes any unset call. | enum-error path; zero-panes-with-markers path; zero-panes-no-markers no-op; guard runs before any unset; never falls through to "live set empty, unset everything" |
| daemon-merge-reintroduces-dead-sessions-2-4 | Soft-warning posture for per-marker unset and malformed-line skip | Per-marker `UnsetServerOption` failures collected as soft warnings mirroring `CleanStale`, never escalating to fatal; malformed `session:window.pane` lines skipped (not added to live-pane set, not aborting cleanup) with a soft warning when unexpected. | per-unset failure does not abort remaining unsets; malformed-line skip preserves cleanup continuity; warnings drained post-bootstrap; cleanup never returns fatal |
| daemon-merge-reintroduces-dead-sessions-2-5 | Insert cleanup step into orchestrator sequence between step 6 and step 7 | In `cmd/bootstrap/bootstrap.go` insert the cleanup step between Clear `@portal-restoring` (current step 6) and SweepOrphanFIFOs (current step 7); renumber SweepOrphanFIFOs to 8, CleanStale to 9, Return to 10; update package doc comment and step-entry Debug labels; bootstrap integration test in `cmd/bootstrap/bootstrap_test.go` asserts execution position and soft-warning degradation matching `CleanStale`'s posture. | ordering after step 6 / before step 7; Debug-label and package-doc drift; soft-warning collected into `[]Warning`; no locks or sequencing constraints vs the daemon |
| daemon-merge-reintroduces-dead-sessions-2-6 | Wire production adapter in `internal/bootstrapadapter/` | Concrete adapter wrapping `*tmux.Client` exposing marker enumeration via `state.ListSkeletonMarkers`, live-pane enumeration via the error-propagating `(*tmux.Client).ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")`, and marker unset via `(*tmux.Client).UnsetServerOption(SkeletonMarkerPrefix + paneKey)`; adapter test in `adapters_test.go` mirroring the existing `FIFOSweeper` adapter test shape. | `ListAllPanes` deliberately NOT used (swallows errors); error propagation from `ListAllPanesWithFormat`; adapter reusable from integration tests; nil-client convention matches sibling adapters |
| daemon-merge-reintroduces-dead-sessions-2-7 | Scrollback-save resumption end-to-end regression | Integration test proving that after the cleanup step unsets a stale marker whose underlying pane is still live (leaked-marker-but-pane-retained case), the next daemon tick saves scrollback for that pane — the skip-save guard at `cmd/state_daemon.go:131-133` no longer applies — confirming the secondary harm closed by Fix Component B is actually resolved. | marker stale with pane re-created at same paneKey or never killed; tick post-cleanup observes scrollback save; marker-set path (`internal/restore/session.go:380-384`) and hydrate-helper-unset path (`cmd/state_hydrate.go:312`) unmodified |

### Phase 3: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| ID | Task | Edge cases |
|----|------|------------|
| daemon-merge-reintroduces-dead-sessions-3-1 | Consolidate duplicated daemon-tick test helpers in cmd/bootstrap | One source of truth for daemon-tick simulation; helper accepts `skipSet` and `useEmptyScrollback` knobs; both former call sites preserve original semantics; helper gated `//go:build integration`. |
| daemon-merge-reintroduces-dead-sessions-3-2 | Extract shared bootstrap.Orchestrator builder for integration tests | Eleven inline `Orchestrator{...}` literals replaced with one builder; `orchestratorOpts` defaults unset fields to NoOp; `RestoringMarker` always real; adding a hypothetical new step interface requires editing exactly one file. |
| daemon-merge-reintroduces-dead-sessions-3-3 | Extract shared stateDir + logger preamble for cmd/bootstrap integration tests | Nine sites collapse to one helper; `t.TempDir()`-rooted state dir, `t.Setenv("PORTAL_STATE_DIR", ...)`, `state.EnsureDir()`, non-rotating logger registered with `t.Cleanup` for close; helper gated `//go:build integration`. |
| daemon-merge-reintroduces-dead-sessions-3-4 | Align bootstrap step-count nomenclature (nine-step vs ten-step) | `bootstrap.go` doc/Run/comments switch to nine-step framing with CleanStaleMarkers as step 7; CLAUDE.md "Server bootstrap" section updated; no remaining "ten-step" wording in `cmd/bootstrap/`; existing tests pass without modification. |
| daemon-merge-reintroduces-dead-sessions-3-5 | Resolve StaleMarkerCleaner dual-type collision and per-call inner construction | Rename `cmd/bootstrap.StaleMarkerCleaner` to `MarkerCleanupCore`; `bootstrapadapter.StaleMarkerCleaner` constructs inner cleaner once; `MarkerCleaner` interface name unchanged; no two types named `StaleMarkerCleaner` in the codebase; collision-avoidance doc removed. |
| daemon-merge-reintroduces-dead-sessions-3-6 | Reclassify mass-unset hazard guard from error to warn-and-return-nil | Zero-live-panes-with-markers branch returns `nil` and emits `Logger.Warn` with `ComponentBootstrap`; `ErrZeroLivePanesWithMarkers` sentinel deleted (or no longer returned for the deferral case); orchestrator no longer special-cases the sentinel; mass-unset assertion preserved; genuine `ListAllPanesWithFormat` error still propagates. |

### Phase 4: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| ID | Task | Edge cases |
|----|------|------------|
| daemon-merge-reintroduces-dead-sessions-4-1 | Fix step-number docstring drift in cmd/bootstrap_production.go and add missing adapter to inventory | `cleanStaleAdapter` docstring drift (line 51): "Step 8" → "Step 9"; inventory comment at lines 16-17 missing `StaleMarkerCleaner` — add it; wiring unchanged (docs-only); existing `cmd/bootstrap/...` and `internal/bootstrapadapter/...` tests pass without modification. |
| daemon-merge-reintroduces-dead-sessions-4-2 | Fix step-number drift in adapters_test.go FIFOSweeper docstring | Only line 41 changes ("step-7" → "step-8"); line 139 (StaleMarkerCleaner step-7 reference) must stay as-is; production docstrings in `adapters.go:107` and `:131` already correct; `go test ./internal/bootstrapadapter/...` passes. |
| daemon-merge-reintroduces-dead-sessions-4-3 | Re-type MarkerCleanupCore.Logger to the bootstrap.Logger interface | Production `*state.Logger` already satisfies `bootstrap.Logger` — verify no compile break; helper removal conditional on no remaining references; Warn assertions on component and marker count preserved through the port; `recordingLogger{}` is the existing convention in cmd/bootstrap tests. |
| daemon-merge-reintroduces-dead-sessions-4-4 | Eliminate redundant StaleMarkerCleaner adapter pass-through | Inventory comment update from task 4-1 should be removed (if landed) — coordinate; `staleClientStub`-driven tests in `adapters_test.go` and `adapters_internal_test.go` deleted along with the adapter; `*MarkerCleanupCore`-level coverage subsumes any adapter-level-unique cases (zero-panes-guard Warn, list-skeleton-markers error, normal cleanup); inline `&bootstrap.MarkerCleanupCore{...}` construction matches `cleanStaleAdapter`/`saverAdapter` pattern. |

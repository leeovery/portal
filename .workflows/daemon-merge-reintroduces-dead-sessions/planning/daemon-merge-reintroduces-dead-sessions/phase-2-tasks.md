---
phase: 2
phase_name: Bootstrap stale-marker cleanup step (Fix Component B)
total: 7
---

## daemon-merge-reintroduces-dead-sessions-2-1 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-1: Introduce stale-marker cleanup seam and happy-path implementation

**Problem**: The bootstrap orchestrator has no path to remove `@portal-skeleton-*` server-option markers whose corresponding pane no longer exists. Stale markers indefinitely suppress scrollback save for any pane sharing that paneKey (`cmd/state_daemon.go:131-133`) and accumulate across the tmux server's lifetime. A first-class orchestrator step is needed, with a seam isolating the three responsibilities (marker enumeration, live-pane enumeration, marker unset) so each can be mocked independently.

**Solution**: Define a new orchestrator seam in `cmd/bootstrap/` exposing the three responsibilities, and implement the cleanup function that diffs canonical-paneKey markers against live-pane paneKeys (sanitised via `state.SanitizePaneKey`), unsetting any marker whose paneKey is not in the live-pane set via `(*tmux.Client).UnsetServerOption(SkeletonMarkerPrefix + paneKey)`. Co-locate the implementation file (e.g. `cmd/bootstrap/stale_marker_cleanup.go`) and unit test (`cmd/bootstrap/stale_marker_cleanup_test.go`). This task covers the happy path (stale marker → unset; live marker → preserved); other tasks in this phase cover normalisation correctness, mass-unset hazard guard, soft-warning posture, orchestrator wiring, adapter wiring, and end-to-end regression.

**Outcome**: The cleanup function exists in `cmd/bootstrap/`, is independently testable, and given a paneKey-set of markers and a paneKey-set of live panes, unsets exactly the markers whose paneKey is absent from the live-pane set. Live markers are left untouched. The seam interfaces are independently mockable per the existing bootstrap convention (each interface has 1-3 methods).

**Do**:
- Create `cmd/bootstrap/stale_marker_cleanup.go`. Define seam(s) consistent with the existing `FIFOSweeper` / `StaleCleaner` pattern in `bootstrap.go`. Recommended shape — a single composite interface (e.g. `StaleMarkerCleaner` with a method `CleanStaleMarkers() error`) backed by three small narrowly-typed dependencies that can be swapped per-test:
  - `MarkerLister` — `ListSkeletonMarkers() (map[string]struct{}, error)` returning canonical paneKey-keyed map (delegates to `state.ListSkeletonMarkers`).
  - `LivePaneLister` — `ListAllPanesWithFormat(format string) (string, error)` (delegates to `(*tmux.Client).ListAllPanesWithFormat`).
  - `MarkerUnsetter` — `UnsetServerOption(name string) error` (delegates to `(*tmux.Client).UnsetServerOption`).
- The composite vs three-small-interfaces choice is implementation discretion; mirror the convention used by adjacent seams in `bootstrap.go` (e.g. `FIFOSweeper` is a single-method composite; `RestoringMarker` exposes two methods on one interface). The acceptance criterion is that each responsibility is independently mockable in tests.
- Implement the cleanup function (e.g. method on the composite struct or a free function called from it):
  1. Call `MarkerLister.ListSkeletonMarkers()` — yields canonical paneKeys (no prefix).
  2. Call `LivePaneLister.ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")`.
  3. Parse each non-empty trimmed line into `(session, window, pane)` and convert to canonical paneKey via `state.SanitizePaneKey(session, window, pane)`. Build a `map[string]struct{}` keyed by canonical paneKey.
  4. For each marker paneKey not in the live set, call `MarkerUnsetter.UnsetServerOption(state.SkeletonMarkerPrefix + paneKey)`.
- Tasks 2-2, 2-3, 2-4 extend this implementation with normalisation correctness, the mass-unset hazard guard, and per-marker error / malformed-line handling. This task only needs to satisfy the happy-path tests below; later tasks add the remaining behaviour and tests.
- Co-locate `cmd/bootstrap/stale_marker_cleanup_test.go` covering the happy-path cases below. Use lightweight fakes following the `stepRecorder` style in `bootstrap_test.go` — record calls, allow per-method canned outputs/errors.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/stale_marker_cleanup.go` exists and exports a seam (composite or three-small-interfaces) for marker enumeration, live-pane enumeration, and marker-unset, each independently mockable.
- [ ] Given a marker set `{"foo__0.0", "bar__1.2"}` and a live-pane set whose canonical paneKeys are `{"foo__0.0"}` only, the cleanup invokes `UnsetServerOption("@portal-skeleton-bar__1.2")` exactly once and does NOT invoke `UnsetServerOption` for `"@portal-skeleton-foo__0.0"`.
- [ ] Live-pane enumeration uses the format string `"#{session_name}:#{window_index}.#{pane_index}"` literal (no other format).
- [ ] Marker unset uses `state.SkeletonMarkerPrefix + paneKey` for the option name (no string-literal duplication of `@portal-skeleton-`).
- [ ] `mergePane` / `findOrAppendSession` (`internal/state/capture.go`), `setSkeletonMarker` (`internal/restore/session.go:380-384`), and `UnsetSkeletonMarkerForFIFO` (`cmd/state_hydrate.go:312`) are not modified by this task.
- [ ] Co-located unit tests pass for the two happy-path scenarios (stale marker unset, live marker preserved) using mock implementations of the seam interfaces.
- [ ] `go build ./...` succeeds.

**Tests**:
- `"it unsets a marker whose paneKey is not present in the live-pane set"` — given marker `{x}` and zero live panes... wait, this overlaps with task 2-3's zero-panes guard. Use markers `{stale__0.0, live__0.0}` and live panes `{live:0.0}` so the live-pane set is non-empty and the zero-panes guard does not skip. Assert exactly one unset call for `@portal-skeleton-stale__0.0`.
- `"it leaves a marker alone whose paneKey is present in the live-pane set"` — given marker `{live__0.0}` and live pane `live:0.0`, assert zero unset calls.
- `"it requests live panes with the canonical session:window.pane format"` — assert the format string passed to `ListAllPanesWithFormat` is `#{session_name}:#{window_index}.#{pane_index}` exactly.
- `"it composes the option name from SkeletonMarkerPrefix"` — assert the option name passed to `UnsetServerOption` is `@portal-skeleton-<paneKey>` (constructed via the `state.SkeletonMarkerPrefix` constant; verifiable by checking the literal value matches `state.SkeletonMarkerPrefix + paneKey`).

**Edge Cases**:
- Empty marker set + non-empty live-pane set → no unset calls; cleanup is a no-op.
- Both sides non-empty with full overlap → no unset calls.
- Both sides non-empty with no overlap (and live set is non-empty) → unset every marker.
- Whitespace-only / blank lines from `ListAllPanesWithFormat` are tolerated (trimmed and skipped).

**Context**:
> Spec §Fix Component B → Behavior: "Enumerate the live `@portal-skeleton-*` server-option markers via tmux. Enumerate live tmux panes (paneKeys). Compute the set difference: markers whose paneKey is not present in the live pane set. For each stale marker, unset it via tmux (`set-option -us @portal-skeleton-<key>`)."
>
> Spec §Fix Component B → Adapter Wiring: "The seam should expose three methods (one per responsibility) so each can be mocked independently in tests; whether they live on a single composite interface or three small interfaces is an implementation choice consistent with existing bootstrap conventions."
>
> `state.SkeletonMarkerPrefix` is `@portal-skeleton-` per `internal/state/markers.go:14`. `state.ListSkeletonMarkers` returns paneKeys with the prefix already stripped (`internal/state/markers.go:89`). `(*tmux.Client).UnsetServerOption` runs `set-option -su <name>` per `internal/tmux/tmux.go:589-595`.
>
> Existing seam-naming convention from `bootstrap.go`: single-purpose interfaces typed by their behaviour (e.g. `FIFOSweeper` exposes `Sweep() error`; `RestoringMarker` exposes `Set() error`/`Clear() error`).

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Behavior, Adapter Wiring), §Acceptance Criteria #4.

## daemon-merge-reintroduces-dead-sessions-2-2 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-2: Add paneKey normalisation correctness fixture

**Problem**: The cleanup compares two paneKey representations: tmux's live-pane format `session:window.pane` (e.g. `my-session:0.1`, returned by `ListAllPanesWithFormat`) and the canonical form `session__window.pane` (e.g. `my-session__0.1`, used by skeleton markers and `state.SanitizePaneKey`). If the conversion is dropped, applied to the wrong side, or replaced with naive string equality, the diff becomes meaningless — every marker would be computed as stale because no live entry would match. A regression here re-creates the mass-unset hazard from a different angle.

**Solution**: Add a unit fixture in `cmd/bootstrap/stale_marker_cleanup_test.go` (the file from task 2-1) that mixes both paneKey representations and asserts the same logical pane is recognised across both sides. Add a complementary negative test where two paneKeys differ ONLY by separator (`:` vs `__`) and assert they are NOT treated as equivalent — this guards against a future regression where someone replaces the conversion with naive string concatenation or lenient matching.

**Outcome**: A test fixture proves the cleanup correctly recognises a logical pane regardless of which representation each side supplies, AND proves the conversion is not bypassed by a string-equality shortcut. Together these tests pin the conversion contract so a future change cannot silently drop or weaken it.

**Do**:
- In `cmd/bootstrap/stale_marker_cleanup_test.go`, add a sub-test or new test function `TestStaleMarkerCleanup_PaneKeyNormalisation` (name to taste; convention-following).
- **Positive case**: marker side seeded with canonical key `my-session__0.1`; live-pane enumeration returns `my-session:0.1` (tmux format). After cleanup, assert the marker is NOT unset (the canonical key is correctly recognised as a live pane).
- **Negative case (separator-only-differs)**: marker side seeded with `my-session:0.1` (note the `:`, an unsanitised form that must NOT be treated as the canonical paneKey for the matching live entry). Live-pane enumeration returns `my-session:0.1` (which the cleanup sanitises to `my-session__0.1`). The marker `my-session:0.1` is NOT in the live set after sanitisation, so the test asserts cleanup unsets it. This proves the cleanup operates on canonical form only — naive equality with the raw tmux output would skip the unset and pass the test silently on broken code.
- **Session-name-with-colon case**: marker side seeded with `host:1234__0.0` (canonical form for a session literally named `host:1234`); live-pane enumeration returns `host:1234:0.0` (the full tmux line). The cleanup must split on the rightmost `:` to recover `(session=host:1234, window=0, pane=0)`, sanitise, and recognise it as a match — assert the marker is NOT unset.
- Use the same mock seams from task 2-1 to drive the assertions.

**Acceptance Criteria**:
- [ ] Positive recognition: marker `session__win.pane`, live-pane line `session:win.pane` → marker is preserved (no unset).
- [ ] Negative non-equivalence: marker raw form `session:win.pane`, live-pane line `session:win.pane` → marker is unset (proving naive string equality between the two raw forms is NOT used).
- [ ] Session-name-with-colon recognition: marker `host:1234__0.0`, live line `host:1234:0.0` → rightmost-`:` split correctly recovers session=`host:1234`, marker is preserved.
- [ ] Each test fails on a stub implementation where `state.SanitizePaneKey` is dropped (live side compared verbatim against marker map) — this is the regression guard the test is designed to catch.

**Tests**:
- `"it recognises a marker in canonical form against a live pane in tmux session:win.pane form"` — positive case described above.
- `"it does not treat raw session:win.pane and canonical session__win.pane as equivalent"` — negative case proving conversion is not a no-op string equality.
- `"it splits on the rightmost colon to recover session names containing colons"` — session-name-with-colon case.

**Edge Cases**:
- Session name contains multiple `:` characters (e.g. `host:1234:dev`) — rightmost-`:` split is the contract; a leftmost-split or first-`:` parser would mis-attribute the session and fail the test.
- Pane index > 9 (multi-digit) — `strconv.Atoi` rather than single-byte parsing must be used.
- Window index = 0 — boundary value; covered by the positive case (`session__0.1`).

**Context**:
> Spec §Fix Component B → Adapter Wiring → PaneKey conversion: "Each live-pane entry from the format string above is of form `session:window.pane` (e.g. `my-session:0.1`). The cleanup step must convert each entry to canonical paneKey form via `state.SanitizePaneKey(session, window, pane)` (which produces `session__window.pane` form, e.g. `my-session__0.1`) before computing the set difference. Without this conversion the two sides have incompatible separators (`:` vs `__`) and the diff is meaningless."
>
> Spec §Testing Requirements → PaneKey normalisation correctness: "test fixture must mix tmux's `session:window.pane` form (live-pane side) with canonical `session__window.pane` form (marker side) and assert that the same logical pane is recognised across both sides. A complementary negative test where two paneKeys differ only by separator must not be treated as equivalent. This guards against a regression where the `state.SanitizePaneKey` conversion is dropped, applied to the wrong side, or replaced with a naive string equality."
>
> Spec §Fix Component B → Parse contract: "Split on the rightmost `:` (tmux session names may contain `:`; the rightmost `:` separates session from `window.pane`). Split the right half on `.` to obtain `window` and `pane` strings. Parse `window` and `pane` as integers via `strconv.Atoi`."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Adapter Wiring → PaneKey conversion, Parse contract), §Testing Requirements (PaneKey normalisation correctness).

## daemon-merge-reintroduces-dead-sessions-2-3 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-3: Mass-unset hazard guard for zero-panes and enum errors

**Problem**: If `ListAllPanesWithFormat` fails (tmux unavailable, transient socket error) or returns zero panes during a tmux instability window, computing the set difference treats EVERY `@portal-skeleton-*` marker as stale — including markers protecting genuinely live, hydrate-in-progress panes. Mass-unsetting markers under tmux instability would destabilise a still-live tmux server. The cleanup must NEVER fall through to "live set is empty, therefore unset all markers"; it must skip the unset pass entirely and emit a soft warning.

**Solution**: In the cleanup implementation from task 2-1, add an explicit guard before any unset call: if `ListAllPanesWithFormat` returns a non-nil error OR returns zero parsed panes, skip the unset pass entirely, emit a soft warning, and return. Markers remain untouched in both branches. The guard precedes any unset call so it is structurally impossible for an empty live set to drive a mass-unset.

**Outcome**: On enumeration error, the cleanup performs zero unset calls and surfaces a soft warning. On zero-panes return (with no enumeration error), the cleanup performs zero unset calls and surfaces a soft warning if markers exist (a true no-pane-no-marker startup is a no-op with no warning needed). Markers are never mass-unset.

**Do**:
- In the cleanup function, after the `ListAllPanesWithFormat` call:
  - If err != nil → record the error as a soft warning (return it to the caller via the orchestrator's warning collection per task 2-5; the cleanup's own return signature is `error`, which the orchestrator treats as soft per task 2-5's wiring) and return without invoking any unset.
  - Parse the live-pane lines into the canonical paneKey set (per task 2-1).
  - If the parsed live-pane set is empty AND the marker set is non-empty → record a soft warning (e.g. "stale-marker cleanup: zero live panes; skipping to avoid mass-unset hazard") and return without invoking any unset.
  - If the parsed live-pane set is empty AND the marker set is also empty → return cleanly with no warning (genuine no-op startup).
- The "soft warning" surface from this step is the cleanup function's `error` return — the orchestrator (task 2-5) Warn-and-swallows it analogous to step 7 (FIFOSweeper) and step 8 (CleanStale). The cleanup function never returns a `*FatalError`.
- Add unit tests in `cmd/bootstrap/stale_marker_cleanup_test.go`.

**Acceptance Criteria**:
- [ ] When `ListAllPanesWithFormat` returns a non-nil error, the cleanup invokes zero `UnsetServerOption` calls.
- [ ] When `ListAllPanesWithFormat` returns no error but a parsed live-pane count of zero AND the marker set is non-empty, the cleanup invokes zero `UnsetServerOption` calls and returns a non-nil error suitable for the orchestrator's Warn-and-swallow path.
- [ ] When `ListAllPanesWithFormat` returns zero panes AND the marker set is also empty, the cleanup is a clean no-op (returns nil; no warning).
- [ ] The zero-panes guard runs strictly before any `UnsetServerOption` call; no execution path reaches `UnsetServerOption` when the live-pane set is empty.
- [ ] The cleanup never falls through to "live pane set is empty, therefore unset all markers" — assertion: in any test where the live-pane set is empty, `UnsetServerOption` call count is zero regardless of marker set size.

**Tests**:
- `"it skips unset and emits a warning when ListAllPanesWithFormat returns an error"` — mock returns sentinel error; assert zero unset calls, returned error wraps the sentinel.
- `"it skips unset and emits a warning when zero live panes are returned with markers present"` — mock returns empty string, no error; markers `{a__0.0, b__0.0}`; assert zero unset calls, returned error is non-nil.
- `"it is a clean no-op when zero live panes are returned with zero markers"` — assert returned error is nil and no unset call.
- `"the zero-panes guard runs before any unset"` — variant of the second test using a `MarkerUnsetter` mock that records the call order; assert no unset call occurred.
- `"it never mass-unsets when ListAllPanesWithFormat fails"` — mock returns non-empty marker set + enum error; assert zero unset calls (regression guard against the spec's mass-unset hazard).

**Edge Cases**:
- Whitespace-only response from `ListAllPanesWithFormat` (`"   \n  \n"`) → parses to zero live panes → guard triggers.
- All lines in response are malformed (covered by task 2-4) → parsed live-pane set empty → guard triggers (overlap with task 2-4 is intentional; both tasks' guards must protect the unset path).
- Marker set is empty + live-pane set is empty → no warning (returns nil); the warning is only useful when there's actually something at risk.

**Context**:
> Spec §Fix Component B → Soft-Warning Posture → Mass-unset hazard guard: "The cleanup must never treat a silently-empty live-pane result as authoritative. If live-pane enumeration fails or returns zero panes due to tmux instability, the cleanup must skip the unset pass and emit a soft warning. The hazard guarded against: an empty live-pane set would cause every `@portal-skeleton-*` marker to be computed as stale, including markers protecting legitimate hydrate-in-progress panes — destabilising a still-live tmux server. The error-propagating live-pane call (above) is the primary defence; an explicit 'if zero panes, skip cleanup' guard is recommended belt-and-braces if the runtime can plausibly observe zero live panes when markers exist."
>
> Spec §Fix Component B → Adapter Wiring: `(*tmux.Client).ListAllPanes()` is unsuitable because it swallows errors and returns `([]string{}, nil)`. `ListAllPanesWithFormat` propagates errors per `internal/tmux/tmux.go:528-534`.

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Soft-Warning Posture, Mass-unset hazard guard, Adapter Wiring).

## daemon-merge-reintroduces-dead-sessions-2-4 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-4: Soft-warning posture for per-marker unset and malformed-line skip

**Problem**: Per-marker `UnsetServerOption` calls can fail (e.g. tmux dropped the option between enumeration and unset, transient socket error). One failed unset must not abort the rest. Likewise, malformed `session:window.pane` lines from `ListAllPanesWithFormat` must be skipped — including a malformed line in the live-pane set could cause spurious "live" entries; aborting cleanup would leave genuinely stale markers in place. The cleanup must mirror `CleanStale`'s soft-warning posture: collect failures, never escalate to fatal, drain post-bootstrap.

**Solution**: Wrap each `UnsetServerOption` call so a failure is recorded (logged or aggregated) and the loop continues to the next marker. Wrap the line-parse step so a malformed line is skipped (not added to the live-pane set, not aborting cleanup) with a soft warning emitted when unexpected. The cleanup function returns a single `error` (joined or first-encountered for non-empty failures, nil for clean runs) which the orchestrator Warn-and-swallows.

**Outcome**: A cleanup run with N markers and M unset failures still attempts every unset (logging failures); a run with malformed live-pane lines processes the well-formed remainder; in both cases the cleanup never returns a `*FatalError` and never aborts mid-loop.

**Do**:
- In the cleanup loop:
  - Per-marker unset failure: record (e.g. via an injected `Logger` or by accumulating an `errors.Join`-style multi-error). Continue to the next marker. Do NOT return early.
  - Malformed line (rightmost-`:` split fails, `.` split yields fewer than 2 parts, or `strconv.Atoi` fails on window or pane): skip the line, do NOT add it to the live-pane set, log a soft warning (recommended) and continue parsing the remaining lines.
  - The cleanup's return value: nil if every well-formed marker/line was handled cleanly; non-nil aggregating the failures otherwise. The orchestrator wires this as a Warn (per task 2-5).
- For logging, accept a `*state.Logger` field on the cleanup struct (nil-safe per the codebase's `*state.Logger` convention), mirroring `bootstrapadapter.FIFOSweeper`'s pattern.
- The cleanup function MUST NOT return a `*FatalError` under any code path; only `error` values that the orchestrator surfaces as soft warnings.
- Add unit tests in `cmd/bootstrap/stale_marker_cleanup_test.go`.

**Acceptance Criteria**:
- [ ] When N markers are stale and the K-th unset fails, the cleanup still attempts unsets N-1 through N (every other unset call), records the failure, and returns a non-nil error.
- [ ] When `ListAllPanesWithFormat` returns a mix of well-formed and malformed lines, the well-formed lines are added to the live-pane set; malformed lines are skipped and do not abort cleanup.
- [ ] Malformed-line cases include: missing `:` (no rightmost-colon split possible), missing `.` after the colon, non-integer window, non-integer pane.
- [ ] Cleanup never returns a `*FatalError` (no code path constructs one).
- [ ] When all lines are malformed AND markers exist, the live-pane set is empty → the zero-panes guard from task 2-3 triggers (no mass-unset).

**Tests**:
- `"it continues attempting unsets when one fails mid-loop"` — markers `{a__0.0, b__0.0, c__0.0}` all stale; mock unset returns error on the second call; assert all three unset calls attempted, returned error non-nil.
- `"it skips malformed live-pane lines without aborting cleanup"` — live-pane response has lines `[good:0.0, malformed-no-colon, good2:1.0]`; assert cleanup processes both good lines into the live-pane set and the malformed line is silently skipped (or logged as a warning).
- `"it skips a line whose window index is not an integer"` — line `good:abc.0`; skipped.
- `"it skips a line whose pane index is not an integer"` — line `good:0.xyz`; skipped.
- `"it skips a line missing the dot separator"` — line `good:01`; skipped.
- `"it skips a line missing the colon separator"` — line `goodonly`; skipped.
- `"the cleanup never returns a fatal error"` — under per-unset failure AND malformed-line conditions, assert `errors.As(err, &*bootstrap.FatalError)` is false.

**Edge Cases**:
- All markers' unsets fail → the cleanup attempted every unset, returned non-nil error, did not return fatal.
- Per-unset failure AND zero-panes guard cannot both fire (the guard short-circuits before the loop) — verify the guard takes precedence in any test where both conditions are configured.
- Logger is nil → logging path is a no-op (relies on `*state.Logger` nil-safety; matches `FIFOSweeper` convention).

**Context**:
> Spec §Fix Component B → Soft-Warning Posture: "Best-effort, mirrors the warning-soft semantics of the existing `CleanStale` step (step 8). Failure (tmux unavailable, individual unset error, live-pane enumeration error) surfaces as a soft warning collected by the orchestrator and drained post-bootstrap; it never escalates to a fatal abort."
>
> Spec §Fix Component B → Adapter Wiring → Parse contract: "On parse failure for any line, skip that line (do not abort cleanup; do not include it in the live-pane set; emit a soft warning if the failure is unexpected). A malformed line must not cause cleanup to mass-unset markers."
>
> Existing convention from `cmd/bootstrap/bootstrap.go:237-241` (step 7 SweepOrphanFIFOs) and `bootstrap.go:243-248` (step 8 CleanStale): a non-nil error from the step is logged via `o.Logger.Warn` and swallowed; the orchestrator continues.

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Soft-Warning Posture, Adapter Wiring → Parse contract).

## daemon-merge-reintroduces-dead-sessions-2-5 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-5: Insert cleanup step into orchestrator sequence between step 6 and step 7

**Problem**: The cleanup function from tasks 2-1 through 2-4 exists in `cmd/bootstrap/` but is not yet invoked by the orchestrator. It must run between current step 6 (Clear `@portal-restoring`) and current step 7 (SweepOrphanFIFOs) — after the restoring marker is cleared (so it observes the post-restore tmux state) and before the FIFO sweep (so any stale markers protecting orphan FIFOs are unset first, allowing those FIFOs to be reclaimed in the same bootstrap). The package doc comment, step-entry Debug labels, and existing tests must be updated to reflect the new ten-step sequence.

**Solution**: Add a new field to `Orchestrator` (e.g. `StaleMarkers StaleMarkerCleaner`), invoke it in `Run` between current step 6 and current step 7, renumber the affected steps in the package doc and Debug labels, and update bootstrap integration tests in `bootstrap_test.go` to assert ordering and soft-warning degradation. Mirror the soft-warning posture of `CleanStale` (step 8): a non-nil return is logged via `o.Logger.Warn` and swallowed; the step never escalates to a fatal abort.

**Outcome**: `Run` invokes the cleanup step between Clear and Sweep. The package doc lists the new ten-step sequence with the cleanup at position 7. Step-entry Debug log labels match the new numbering. Tests in `bootstrap_test.go` assert the new ordering and the soft-warning degradation; existing ordering tests are updated for the renumbered subsequent steps.

**Do**:
- In `cmd/bootstrap/bootstrap.go`:
  - Update the package doc comment (lines 1-15) to list the new ten-step sequence:
    1. EnsureServer
    2. RegisterPortalHooks
    3. Set @portal-restoring
    4. EnsureSaver
    5. Restore
    6. Clear @portal-restoring
    7. **CleanStaleMarkers** (new)
    8. SweepOrphanFIFOs
    9. CleanStale
    10. Return
  - Add a new step seam to the `Orchestrator` struct and a top-level interface declaration. Convention: `StaleMarkers StaleMarkerCleaner` on `Orchestrator`, with `StaleMarkerCleaner` interface defined alongside `FIFOSweeper` / `StaleCleaner`.
  - In `Run`, after the existing step 6 (Clear `@portal-restoring`) and before the existing step 7 (SweepOrphanFIFOs), insert the new step block:
    ```
    // Step 7 — CleanStaleMarkers (best-effort).
    o.Logger.Debug(state.ComponentBootstrap, "step 7 (CleanStaleMarkers): entering")
    if err := o.StaleMarkers.CleanStaleMarkers(); err != nil {
        o.Logger.Warn(state.ComponentBootstrap, "step 7 (CleanStaleMarkers) failed: %v", err)
    }
    ```
  - Renumber the existing SweepOrphanFIFOs step entry comment and Debug label from "step 7" to "step 8". Renumber CleanStale from "step 8" to "step 9". Renumber Return from "step 9" to "step 10".
  - Update the godoc on `FIFOSweeper` (currently mentions "Step 7") to "Step 8".
  - Update the godoc on `StaleCleaner` if it mentions a step number.
  - Update the godoc on `Run` (lines 165-?? currently lists soft-warning paths) to add the new step's soft-warning path.
- In `cmd/bootstrap/bootstrap_test.go`:
  - Extend `stepRecorder` with a `CleanStaleMarkers() error` method (and a corresponding `CleanStaleMarkersErr` field) so it satisfies the new interface.
  - Update `newOrchestrator` to wire `r` into the new `StaleMarkers` field.
  - Update every existing happy-path call-order test (`TestOrchestratorRun_executesStepsInSpecOrder`, `TestOrchestratorRun_continuesPastEnsureSaverFailureAndAppendsWarning`, `TestOrchestratorRun_isIdempotentAcrossInvocations`, `TestOrchestratorRun_emitsDebugLinePerExecutedStep`) to include `"CleanStaleMarkers"` in the expected call slice between `"Clear"` and `"Sweep"`.
  - Update `TestOrchestratorRun_runsSweepBetweenClearAndCleanStale` to also assert `CleanStaleMarkers` runs between `Clear` and `Sweep`.
  - Add a new test `TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep` asserting strict ordering: `Clear` < `CleanStaleMarkers` < `Sweep` < `CleanStale`.
  - Add a new test `TestOrchestratorRun_continuesPastCleanStaleMarkersFailure` mirroring `TestOrchestratorRun_continuesPastSweepFailure`: the step's error is logged via Warn, swallowed, and `*FatalError` is NOT returned. The Warn message must embed both `"step 7"` and the underlying cause string.
  - Add a regression check: `*Orchestrator` still satisfies `Runner` (compile-time assertion already present via `var _ Runner = (*Orchestrator)(nil)`).
- DO NOT add any locks, sequencing, or coordination logic between this step and the daemon — the spec explicitly requires no serialisation.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/bootstrap.go` package doc comment lists ten steps in the canonical order with `CleanStaleMarkers` at position 7.
- [ ] `Orchestrator.Run` invokes `o.StaleMarkers.CleanStaleMarkers()` strictly after `o.Restoring.Clear()` and strictly before `o.Sweeper.Sweep()`.
- [ ] A non-nil return from `CleanStaleMarkers()` is logged via `o.Logger.Warn` (with `state.ComponentBootstrap`) and swallowed — `Run` continues to step 8 (Sweep) and ultimately returns nil.
- [ ] The Warn log line embeds both the canonical step label `"step 7 (CleanStaleMarkers)"` and the underlying error's `Error()` string verbatim (so portal.log preserves the cause).
- [ ] `*FatalError` is NEVER returned from this step (assert via `errors.As` in the failure test).
- [ ] `TestOrchestratorRun_executesStepsInSpecOrder` and the other call-order tests include `"CleanStaleMarkers"` between `"Clear"` and `"Sweep"`.
- [ ] `TestOrchestratorRun_emitsDebugLinePerExecutedStep` includes `"CleanStaleMarkers"` in the executed-steps assertion.
- [ ] No locks, mutexes, or sequencing constraints are added between the cleanup step and the daemon (`cmd/state_daemon.go` is not modified by this task).
- [ ] The marker-set path (`internal/restore/session.go:380-384`) and the hydrate-helper unset path (`cmd/state_hydrate.go:312`) are not modified.
- [ ] `go build ./...` passes; `go test ./cmd/bootstrap/...` passes.

**Tests**:
- `"it runs CleanStaleMarkers between Clear and Sweep"` — `TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep`; assert ordering Clear < CleanStaleMarkers < Sweep < CleanStale.
- `"it continues past CleanStaleMarkers failure and does not return fatal"` — `TestOrchestratorRun_continuesPastCleanStaleMarkersFailure`; assert returned err is nil, Warn was called with both `"step 7"` and the sentinel cause, `errors.As(err, &fatal)` is false, and Sweep + CleanStale still run.
- `"existing ordering tests pass with CleanStaleMarkers in the call slice"` — updated `TestOrchestratorRun_executesStepsInSpecOrder`, `TestOrchestratorRun_continuesPastEnsureSaverFailureAndAppendsWarning`, `TestOrchestratorRun_isIdempotentAcrossInvocations`, `TestOrchestratorRun_emitsDebugLinePerExecutedStep` all pass with the new step in the expected call slice.

**Edge Cases**:
- CleanStaleMarkers fails AND CleanStale fails → both are logged as Warn; Run still returns nil; both Warn lines appear in the recordingLogger.warnings slice.
- CleanStaleMarkers returns nil (clean run) → no Warn line, only Debug entry; downstream steps run normally.
- No locks or sequencing constraints — concurrent daemon ticks are NOT serialised against this step (per spec; Phase 1 already neutralised the marker's authority over the merge).

**Context**:
> Spec §Fix Component B → Location: "New step in the bootstrap orchestrator (`cmd/bootstrap/`), inserted between current step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs) — becoming the new step 7, with subsequent steps renumbered."
>
> Spec §Fix Component B → Concurrency with the Daemon: "The cleanup step does not require serialisation with daemon ticks because Fix Component A neutralises the marker's authority over the merge — concurrent daemon reads of a marker about to be unset cannot resurrect a dead session, and the merge's structural filter rejects any prev session/window/pane no longer present in tmux regardless of marker state. Implementers should not add locks or sequencing constraints between the cleanup step and the daemon."
>
> Spec §Fix Component B → Synergy with `SweepOrphanFIFOs`: "The cleanup runs immediately before `SweepOrphanFIFOs` (step 7 of the existing sequence). `SweepOrphanFIFOs` removes orphan `hydrate-*.fifo` files whose paneKey is no longer represented by a live `@portal-skeleton-*` marker. Because the new step unsets stale markers immediately before the sweep, FIFOs whose markers were stale become eligible for sweep in the same bootstrap. This compound cleanup is intentional."
>
> Existing pattern: `bootstrap.go:243-248` shows the canonical Warn-and-swallow shape for step 8 (CleanStale): `o.Logger.Debug(...)`, then `if err := ...; err != nil { o.Logger.Warn(...) }`, no fatal return.

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Location, Soft-Warning Posture, Concurrency with the Daemon, Synergy with SweepOrphanFIFOs).

## daemon-merge-reintroduces-dead-sessions-2-6 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-6: Wire production adapter in `internal/bootstrapadapter/`

**Problem**: The orchestrator step from task 2-5 needs a concrete production-shape adapter that wires `*tmux.Client` to the cleanup step's seam interfaces. Without this adapter, the orchestrator can be tested but cannot actually run against a real tmux server. Critically: the adapter must use `(*tmux.Client).ListAllPanesWithFormat` (which propagates errors) rather than `(*tmux.Client).ListAllPanes` (which swallows errors and returns `([]string{}, nil)`) — using the wrong call would re-introduce the mass-unset hazard the task 2-3 guard was designed to defend against.

**Solution**: Add a production adapter in `internal/bootstrapadapter/adapters.go` (e.g. `StaleMarkerCleaner` struct) wrapping `*tmux.Client` and exposing the cleanup behaviour to the orchestrator. The adapter delegates: marker enumeration to `state.ListSkeletonMarkers`, live-pane enumeration to `(*tmux.Client).ListAllPanesWithFormat`, marker unset to `(*tmux.Client).UnsetServerOption`. Add an adapter test in `adapters_test.go` mirroring the existing `FIFOSweeper` adapter test shape (a stub that simulates a tmux failure path so we don't need a live tmux for the regression guard).

**Outcome**: `internal/bootstrapadapter/` exposes a `StaleMarkerCleaner` (or similar) production adapter that satisfies `bootstrap.StaleMarkerCleaner`. The adapter uses `ListAllPanesWithFormat` (NOT `ListAllPanes`). The adapter test asserts a tmux enumeration failure surfaces from the adapter as a non-nil error containing the underlying cause (mirroring `TestFIFOSweeper_PropagatesListSkeletonMarkersError`).

**Do**:
- In `internal/bootstrapadapter/adapters.go`:
  - Define a new struct (recommended name `StaleMarkerCleaner`) with fields:
    - `Client *tmux.Client` — the long-lived tmux client (or alternatively three separate seam fields for marker/live-pane/unset, if the cleanup function from task 2-1 takes them individually).
    - `Logger *state.Logger` — nil-tolerant per the codebase convention.
  - Implement `CleanStaleMarkers() error` (matching the seam method name from task 2-5). The implementation either:
    - **Delegates to a free function in `cmd/bootstrap/`** that takes the seam interfaces (preferred — keeps cleanup logic in `cmd/bootstrap/` and adapter as a thin wrapper).
    - **OR holds the cleanup logic itself**, using `state.ListSkeletonMarkers(s.Client)`, `s.Client.ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")`, `s.Client.UnsetServerOption(state.SkeletonMarkerPrefix + paneKey)`.
    - The choice depends on how task 2-1 structured the cleanup. Both are valid; mirror the convention used by `FIFOSweeper` (which holds the logic itself on the adapter).
  - The format string `"#{session_name}:#{window_index}.#{pane_index}"` MUST be the literal passed to `ListAllPanesWithFormat`. Do NOT use `ListAllPanes` (which swallows errors per `internal/tmux/tmux.go:551-557`).
- In `internal/bootstrapadapter/adapters_test.go`:
  - Add `TestStaleMarkerCleaner_PropagatesListAllPanesWithFormatError`: stub `*tmux.Client` is impractical (it's a concrete struct), so introduce a small seam stub OR exercise the adapter by injecting a stub-able interface field. Mirror the shape of `TestFIFOSweeper_PropagatesListSkeletonMarkersError`. If the adapter is a thin wrapper that takes interface fields, the stub is straightforward; if the adapter holds `*tmux.Client` directly, consider exposing the adapter through smaller seam-typed fields (matching `FIFOSweeper`'s `Client state.ServerOptionLister` shape).
  - Add `TestStaleMarkerCleaner_PropagatesListSkeletonMarkersError`: similar — assert a `ShowAllServerOptions` failure surfaces as a wrapped error.
  - Add a positive smoke test (`tmuxtest.SkipIfNoTmux`) gating on a live tmux server: set a marker for a non-existent paneKey, call `CleanStaleMarkers`, assert the marker is unset; set a marker for a live paneKey, call cleanup, assert the marker is preserved. Mirrors the live-tmux pattern in `TestRestoringMarker_SetClearsTogglesServerOption`.
  - Compile-time assertion: `var _ bootstrap.StaleMarkerCleaner = (*StaleMarkerCleaner)(nil)` (or whatever the seam interface is named). Mirrors the pattern in `bootstrap_test.go:716` (`var _ Runner = (*Orchestrator)(nil)`).
- The nil-client convention matches the sibling adapters (`RestoringMarker`, `HookRegistrar`, `FIFOSweeper`): undefined behaviour with nil; panic at first call.
- `cmd/state_daemon.go`, `cmd/state_hydrate.go`, and `internal/restore/session.go` are NOT modified by this task.

**Acceptance Criteria**:
- [ ] `internal/bootstrapadapter/adapters.go` exports a `StaleMarkerCleaner` struct (or similar name consistent with the seam) with `*tmux.Client` (or interface) and `*state.Logger` fields.
- [ ] The adapter's cleanup method calls `(*tmux.Client).ListAllPanesWithFormat` with the literal format `"#{session_name}:#{window_index}.#{pane_index}"`.
- [ ] The adapter does NOT call `(*tmux.Client).ListAllPanes` (which swallows errors).
- [ ] The adapter calls `(*tmux.Client).UnsetServerOption` with `state.SkeletonMarkerPrefix + paneKey` for unset.
- [ ] A `ListAllPanesWithFormat` enumeration failure surfaces from the adapter as a non-nil error containing the underlying cause.
- [ ] A `ShowAllServerOptions` failure (via `ListSkeletonMarkers`) surfaces from the adapter as a non-nil error containing the underlying cause.
- [ ] Compile-time assertion: the adapter satisfies the `bootstrap.StaleMarkerCleaner` interface.
- [ ] The adapter is reusable from integration tests (no `cmd/`-package globals; no ldflags-injected version; mirrors the `FIFOSweeper` reusability claim).
- [ ] `go build ./...` passes; `go test ./internal/bootstrapadapter/...` passes.

**Tests**:
- `"it propagates a ListAllPanesWithFormat error wrapped with context"` — `TestStaleMarkerCleaner_PropagatesListAllPanesWithFormatError`.
- `"it propagates a ListSkeletonMarkers error wrapped with context"` — `TestStaleMarkerCleaner_PropagatesListSkeletonMarkersError`.
- `"it unsets a marker whose paneKey has no live pane against a real tmux server"` — live-tmux smoke test (gated by `tmuxtest.SkipIfNoTmux`); set a marker for `nonexistent__99.99`, run cleanup, assert via `client.TryGetServerOption` that the marker is absent.
- `"it preserves a marker whose paneKey has a live pane against a real tmux server"` — live-tmux smoke test; pre-existing pane's paneKey gets a marker, cleanup runs, marker still present.
- `"the adapter satisfies bootstrap.StaleMarkerCleaner at compile time"` — `var _ bootstrap.StaleMarkerCleaner = (*StaleMarkerCleaner)(nil)`.

**Edge Cases**:
- `*tmux.Client` returns an empty string from `ListAllPanesWithFormat` on a server with no sessions — adapter passes through to the cleanup function; the zero-panes guard from task 2-3 governs behaviour.
- `*state.Logger` field is nil — adapter does not panic (relies on `*state.Logger` nil-safety per the codebase).
- `tmuxtest.SkipIfNoTmux` lanes are deferred when tmux is unavailable; non-tmux tests run unconditionally.

**Context**:
> Spec §Fix Component B → Adapter Wiring → Marker enumeration: "`state.ListSkeletonMarkers` is the canonical source. It already returns a `map[string]struct{}` keyed by paneKey (the `<paneKey>` portion of `@portal-skeleton-<paneKey>`, with the prefix stripped). No additional parsing is required on the marker side."
>
> Spec §Fix Component B → Adapter Wiring → Live pane enumeration: "Must use an error-propagating tmux call so that a tmux failure surfaces as a soft warning rather than a silently-empty result. `(*tmux.Client).ListAllPanes()` is unsuitable here because it swallows tmux errors and returns `([]string{}, nil)` on failure (`internal/tmux/tmux.go:551-557`); using it would cause every `@portal-skeleton-*` marker to be computed as stale and unset on tmux failure, including markers protecting genuinely live hydrate-in-progress panes. The cleanup step must use `(*tmux.Client).ListAllPanesWithFormat(format)` (which propagates errors per `internal/tmux/tmux.go:528-534`) with a format such as `#{session_name}:#{window_index}.#{pane_index}` and parse the result."
>
> Spec §Fix Component B → Adapter Wiring → Marker unset: "`(*tmux.Client).UnsetServerOption(name)` with the full option name `@portal-skeleton-<paneKey>` (i.e. the `SkeletonMarkerPrefix` constant followed by the canonical paneKey)."
>
> Existing pattern: `bootstrapadapter.FIFOSweeper` (`adapters.go:119-136`) is the reference shape for a thin pass-through adapter that wraps a tmux/state seam, holds a `Logger`, and is reusable from integration tests. `TestFIFOSweeper_PropagatesListSkeletonMarkersError` (`adapters_test.go:39-57`) is the reference shape for the failure-propagation regression test.

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Fix Component B (Adapter Wiring → Marker enumeration, Live pane enumeration, Marker unset).

## daemon-merge-reintroduces-dead-sessions-2-7 | approved

### Task daemon-merge-reintroduces-dead-sessions-2-7: Scrollback-save resumption end-to-end regression

**Problem**: Fix Component A alone resolves the user-visible resurrection symptom. Fix Component B closes a quieter side effect: while a marker is live for a paneKey, the daemon's capture loop skips scrollback save for that pane (`cmd/state_daemon.go:131-133`). For panes whose markers leaked but whose underlying sessions are still alive (or were re-created with the same paneKey), scrollback content is silently not being saved. Without an end-to-end test that exercises the leaked-marker-but-pane-retained case, a future regression that disables the cleanup step (or accidentally short-circuits before the unset call) would not be caught — the user-facing symptom is silent.

**Solution**: Add an integration test (gated behind `//go:build integration` per the existing `cmd/bootstrap/reboot_roundtrip_test.go` convention, or as a non-integration test if it can be exercised without a live tmux server) that proves: (1) a stale marker exists for a paneKey whose pane is live; (2) the bootstrap's CleanStaleMarkers step runs and unsets the marker; (3) the next daemon `captureAndCommit` call saves scrollback for that pane (the skip-save guard at `cmd/state_daemon.go:131-133` no longer applies). Confirm via observation that the scrollback file for that paneKey has content (or the index records a non-empty scrollback hash).

**Outcome**: An integration test in `cmd/bootstrap/` (e.g. `cmd/bootstrap/scrollback_resumption_test.go`) drives the full flow against a real tmux server: seed marker → run bootstrap (which invokes CleanStaleMarkers) → run one daemon tick → assert scrollback was saved for the previously-marked pane. The test fails on a build where Fix Component B is disabled (skip-save guard still applies because marker is still set).

**Do**:
- Create `cmd/bootstrap/scrollback_resumption_test.go` (integration-tagged: `//go:build integration` matching `reboot_roundtrip_test.go:1`).
- Test setup:
  - Use the `tmuxtest.New` harness pattern (per `phase5_integration_test.go`, `reboot_roundtrip_test.go`) to spin up an isolated tmux server.
  - Create a real session with at least one pane.
  - Identify the pane's paneKey via `state.SanitizePaneKey(sess, win, pane)`.
  - Manually set the skeleton marker via `client.SetServerOption(state.SkeletonMarkerPrefix + paneKey, "1")` (or equivalent helper) — simulating a leaked marker.
  - Wire a `bootstrap.Orchestrator` with the production `bootstrapadapter.StaleMarkerCleaner` adapter from task 2-6 (other steps can be stubbed; the focus is on the CleanStaleMarkers step's effect on the daemon's skip-save logic).
  - Run `Run(context.Background())` — the cleanup step unsets the stale marker.
  - Confirm via `client.TryGetServerOption(state.SkeletonMarkerPrefix + paneKey)` that the marker is now absent.
- Daemon-tick driving:
  - Construct a `daemonDeps` (or invoke the daemon's `captureAndCommit` directly via an exported test seam, or via `daemon-loop-test` style) with the live `*tmux.Client`, a fresh state directory, and `PrevIndex` seeded to a non-nil value so the merge runs.
  - Run one tick.
  - Assert: the scrollback file for that paneKey exists in the state directory AND has non-zero size (`state.ScrollbackPath(stateDir, paneKey)` or equivalent path helper).
  - Alternative assertion path: inspect the committed `sessions.json` index and confirm the pane has a non-empty `ScrollbackHash` (or equivalent field marking that scrollback was captured this tick).
- Negative-control / regression-guard variant:
  - Same setup but DO NOT run the cleanup step (e.g. swap a no-op cleaner). Assert the marker remains and the scrollback file is NOT written (skip-save guard still fires). This proves the test would fail on broken code where cleanup did not unset the marker.
  - This is the load-bearing assertion: without it, the positive test could pass on broken code where the daemon happened to save scrollback for unrelated reasons.
- The test file MUST NOT modify the marker-set path (`internal/restore/session.go:380-384`) or the hydrate-helper unset path (`cmd/state_hydrate.go:312`) — both are out of scope for this work unit per the spec.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/scrollback_resumption_test.go` exists, is gated behind the `//go:build integration` tag (matching `reboot_roundtrip_test.go`), and is skipped when tmux is unavailable.
- [ ] The test seeds a stale marker for a paneKey whose pane is live in the test tmux server.
- [ ] The test runs the bootstrap orchestrator with the production `StaleMarkerCleaner` adapter; after Run completes, the marker is absent (verified via `TryGetServerOption`).
- [ ] After running one daemon `captureAndCommit` cycle, the scrollback file for that paneKey is present and non-empty.
- [ ] A negative-control variant in the same file (or sub-test) confirms that without the cleanup step, the marker remains AND the scrollback file is NOT written — proving the test catches regressions where Fix Component B is disabled.
- [ ] The test does NOT modify `internal/restore/session.go:380-384` or `cmd/state_hydrate.go:312`.
- [ ] `go test -tags=integration ./cmd/bootstrap/...` passes.
- [ ] The negative-control sub-test would fail on a build where the orchestrator's CleanStaleMarkers step is replaced with a no-op (regression guard).

**Tests**:
- `"the next daemon tick saves scrollback for a pane whose stale marker was unset by bootstrap"` — primary positive case.
- `"without the cleanup step, the next daemon tick does NOT save scrollback for a leaked-marker pane"` — negative-control regression guard.
- `"the cleanup step unsets only the stale marker, not markers for live hydrate-in-progress panes"` — variant to confirm the cleanup does not over-reach (overlaps with task 2-1 unit but exercised end-to-end here).

**Edge Cases**:
- Pane is "re-created at same paneKey" — the spec mentions both "leaked but pane retained" and "leaked but pane re-created with same key". The test covers the retained case (simpler); the re-created case is structurally identical to the daemon (the marker maps to a paneKey that is now live again).
- Marker exists but the live-pane enumeration window is racy — relies on the test serialising `Run` → tick (no concurrent activity), so no race expected in CI.
- State directory permissions — use `t.TempDir()` per the existing convention so the test doesn't pollute `~/.config/portal/state/`.

**Context**:
> Spec §Why This Step Is Needed: "Fix Component A alone resolves the user-visible resurrection symptom because `sessions.json` self-heals once the merge filter rejects dead sessions. However, a quieter side-effect remains: while a marker is live for a paneKey, the daemon's capture loop skips scrollback save for that pane (`cmd/state_daemon.go:131-133`). For panes whose markers leaked but whose underlying sessions are still alive (or were re-created with the same key), scrollback content is silently not being saved. The cleanup step closes this gap and prevents indefinite marker accumulation across the tmux server's lifetime."
>
> Spec §Acceptance Criteria #8: "Scrollback-save resumption — After the cleanup step unsets a stale marker whose underlying pane is still live (the case where a marker leaked but the pane was retained or re-created), the next daemon tick saves scrollback for that pane (i.e. the skip-save guard at `cmd/state_daemon.go:131-133` no longer applies). This verifies the secondary harm closed by Fix Component B (scrollback save was being silently skipped for live-marker panes) is actually resolved, not merely the resurrection symptom."
>
> Skip-save guard exact location (`cmd/state_daemon.go:131-133`):
> ```
> if _, skipped := skipSet[paneKey]; skipped {
>     continue
> }
> ```
> The guard fires when `paneKey` is in `skipSet` — and `skipSet` comes from `state.ListSkeletonMarkers`. So unsetting the marker is observably equivalent to the guard no longer firing on the next tick.
>
> Existing integration-test convention: `cmd/bootstrap/reboot_roundtrip_test.go` uses `//go:build integration`, `tmuxtest.SkipIfNoTmux`, and the `tmuxtest.New` harness for isolated tmux servers. Mirror that shape.

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` — §Why This Step Is Needed, §Acceptance Criteria #8, §Out of Scope (marker production path unchanged).

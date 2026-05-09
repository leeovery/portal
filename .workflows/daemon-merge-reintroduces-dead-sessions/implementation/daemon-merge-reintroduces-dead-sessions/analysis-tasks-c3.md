# Analysis Tasks: daemon-merge-reintroduces-dead-sessions (Cycle 3)

```
topic: daemon-merge-reintroduces-dead-sessions
cycle: 3
total_proposed: 2
```

## Task 1: Re-type MarkerCleanupCore.Markers to state.ServerOptionLister, eliminating closure adapter glue
status: approved
severity: medium
sources: architecture

**Problem**: `MarkerLister` is declared as a 0-arg seam (`ListSkeletonMarkers() (map[string]struct{}, error)`), but the underlying `state.ListSkeletonMarkers(c ServerOptionLister)` takes the tmux client as an argument. Every production-shape wiring site must wrap it in a closure-implementing-an-interface adapter (`markerListerFunc`). This is asymmetric with the sibling `FIFOSweeper` seam, which accepts `state.ServerOptionLister` directly so `*tmux.Client` satisfies it implicitly. Consequences: (a) `markerListerFunc` is duplicated verbatim in `cmd/bootstrap_production.go:59-61` and `cmd/bootstrap/scrollback_resumption_test.go:46-52`; (b) the cycle-2 inline construction at `cmd/bootstrap_production.go:132-139` carries six lines of closure boilerplate. This subsumes cycle-2 finding 4's "unavoidable per Go's package-boundary" acceptance.

**Solution**: Re-type `MarkerCleanupCore.Markers` from `MarkerLister` to `state.ServerOptionLister`, mirroring the `FIFOSweeper` pattern. Have `CleanStaleMarkers` invoke `state.ListSkeletonMarkers(c.Markers)` internally. Delete both `markerListerFunc` declarations. Update unit-test fakes to satisfy `ShowAllServerOptions() (string, error)`.

**Outcome**: `*tmux.Client` satisfies `MarkerCleanupCore.Markers` directly. Inline construction collapses to `Markers: client,` matching `Panes:` / `Unsetter:`. Both `markerListerFunc` declarations gone. Two stale-marker seams share consistent shape.

**Do**:
1. In `cmd/bootstrap/stale_marker_cleanup.go`: change the `Markers` field type from `MarkerLister` to `state.ServerOptionLister`. Update `CleanStaleMarkers` to call `state.ListSkeletonMarkers(c.Markers)` internally. Delete the `MarkerLister` interface declaration if no longer used.
2. In `cmd/bootstrap_production.go`: delete the `markerListerFunc` declaration. Replace the inline closure construction with `Markers: client,`.
3. In `cmd/bootstrap/scrollback_resumption_test.go`: delete the `markerListerFunc` declaration. Pass the test client / fake directly as `Markers`.
4. In `cmd/bootstrap/stale_marker_cleanup_test.go`: update unit-test fakes to satisfy `state.ServerOptionLister` (implement `ShowAllServerOptions() (string, error)` returning raw `tmux show-options -s` output) instead of returning a pre-parsed `map[string]struct{}`.
5. Run `go build ./... && go test ./... && go test -tags=integration ./cmd/bootstrap/...`.

**Acceptance Criteria**:
- `MarkerCleanupCore.Markers` is typed as `state.ServerOptionLister`.
- `CleanStaleMarkers` invokes `state.ListSkeletonMarkers(c.Markers)` internally.
- Zero `markerListerFunc` declarations remain.
- `cmd/bootstrap_production.go` `MarkerCleanupCore` construction reads `Markers: client,`.
- `go build` and full test suite pass under both default and integration lanes.
- `MarkerLister` interface removed if it has no remaining consumers.

**Tests**:
- Existing `stale_marker_cleanup_test.go` unit tests pass with the updated fake shape (implementing `ShowAllServerOptions`).
- `scrollback_resumption_test.go` integration test continues to exercise the cleanup path end-to-end.
- `phase5_integration_test.go` still passes.

---

## Task 2: Fix stale step-numbering and forward-reference docstrings
status: approved
severity: low
sources: standards

**Problem**: Three docstring drifts escaped earlier cycles: (1) `internal/bootstrapadapter/adapters.go:77` says "FIFOSweeper / step 7" but FIFOSweeper is step 8. (2) `cmd/bootstrap/phase5_integration_test.go` lines 225, 231, 272, 317 — `TestPhase5_FIFOSweeperRemovesOrphansAfterRestore`'s docstring and inline comments refer to FIFOSweeper as "step 7" four times. (Note: `scrollback_resumption_test.go`'s "step 7" refer to CleanStaleMarkers — correct, do not change.) (3) `internal/state/capture.go:153-155` — `buildLiveStructure`'s docstring says "Window/pane levels are populated now to keep the helper's shape stable as additional filtering levels land in subsequent tasks." Per the now-merged Fix Component A, all three filtering levels are implemented in the same function.

**Solution**: Update the three sites to reflect current reality.

**Outcome**: All FIFOSweeper docstrings consistently say "step 8". The `buildLiveStructure` docstring describes its current responsibility.

**Do**:
1. `internal/bootstrapadapter/adapters.go:77` — change "FIFOSweeper / step 7" to "FIFOSweeper / step 8".
2. `cmd/bootstrap/phase5_integration_test.go` — replace the four FIFOSweeper-attributed "step 7" references at lines 225, 231, 272, 317 with "step 8". Do not touch unrelated step-number references.
3. `internal/state/capture.go:153-155` — rewrite the trailing sentence of `buildLiveStructure`'s docstring to describe its current responsibility: building the nested live-truth lookup consumed by the three-level filter in `mergeSkippedPanes`. No code change.
4. Run `go build ./... && go test ./...`.

**Acceptance Criteria**:
- Zero FIFOSweeper-attributed "step 7" references in `adapters.go` or `phase5_integration_test.go`.
- `scrollback_resumption_test.go` "step 7" references (referring to CleanStaleMarkers) are untouched.
- `buildLiveStructure`'s docstring describes current behaviour with no "subsequent tasks" forward-reference.
- `go build` and `go test` pass.

**Tests**:
- Docstring-only changes; existing test suite must continue to pass without modification.

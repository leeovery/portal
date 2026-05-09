AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:

- FINDING: MarkerLister seam shape forces a closure adapter at every wiring site; asymmetric with the sibling FIFOSweeper seam
  SEVERITY: medium
  FILES: cmd/bootstrap/stale_marker_cleanup.go:22-24, cmd/bootstrap_production.go:59-61, cmd/bootstrap_production.go:132-139, cmd/bootstrap/scrollback_resumption_test.go:46-67, internal/bootstrapadapter/adapters.go:121-138
  DESCRIPTION: `MarkerLister` is declared as a 0-arg seam (`ListSkeletonMarkers() (map[string]struct{}, error)`). Because the underlying `state.ListSkeletonMarkers(c ServerOptionLister)` takes the tmux client as an argument, every production-shape wiring site must wrap it in a closure-implementing-an-interface adapter (`markerListerFunc`). Compare to the sibling `FIFOSweeper` seam: it accepts `state.ServerOptionLister` directly and calls `state.ListSkeletonMarkers(s.Client)` inside its own `Sweep()` — `*tmux.Client` satisfies the interface implicitly, no adapter needed.

  Two consequences flow from the asymmetric shape:
  (a) `markerListerFunc` is now duplicated verbatim in production (`cmd/bootstrap_production.go:59-61`) AND in the integration test (`cmd/bootstrap/scrollback_resumption_test.go:46-52`) — same 3-line definition with the same comment, two homes.
  (b) The cycle-2 inline construction at `cmd/bootstrap_production.go:132-139` carries six lines of closure boilerplate that a typed seam would erase.

  Re-typing `MarkerCleanupCore.Markers` as `state.ServerOptionLister` (and having `CleanStaleMarkers` invoke `state.ListSkeletonMarkers(c.Markers)` internally, mirroring FIFOSweeper) would let every production and integration-test site pass `client` directly with zero glue, delete both `markerListerFunc` declarations, and bring the seam family into shape consistency. The unit-test fakes at `cmd/bootstrap/stale_marker_cleanup_test.go` would shift to satisfying `ShowAllServerOptions() (string, error)` instead of fabricating the parsed `map[string]struct{}` output — a thin mechanical change, and arguably more faithful to the production data flow.

  RECOMMENDATION: Replace `MarkerLister` with `state.ServerOptionLister` on the `MarkerCleanupCore.Markers` field; have `CleanStaleMarkers` call `state.ListSkeletonMarkers(c.Markers)` internally. Delete both `markerListerFunc` adapter declarations (production and test). Update the unit-test fakes to satisfy `ShowAllServerOptions`. The cycle-2 inline construction then collapses to `Markers: client,` matching the surrounding `Panes: client, Unsetter: client` lines.

SUMMARY: The cycle-2 inline construction at `cmd/bootstrap_production.go` is structurally fine, but it surfaces a pre-existing seam-shape asymmetry: `MarkerLister`'s 0-arg method requires closure glue at every wiring site (now duplicated across production and the integration test), whereas the sibling FIFOSweeper seam accepts `state.ServerOptionLister` directly. Re-typing `Markers` to match the FIFOSweeper pattern would erase the duplication, simplify the inline literal, and bring the two stale-marker seams into consistent shape.

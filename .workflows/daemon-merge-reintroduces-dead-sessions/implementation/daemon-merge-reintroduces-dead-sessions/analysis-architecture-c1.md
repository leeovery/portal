AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 6

FINDINGS:

- FINDING: Dual `StaleMarkerCleaner` concrete types (one per package) create cross-package naming collision
  SEVERITY: low
  FILES: cmd/bootstrap/stale_marker_cleanup.go:68, internal/bootstrapadapter/adapters.go:193
  DESCRIPTION: Both `cmd/bootstrap.StaleMarkerCleaner` (the cleanup-loop core) and `bootstrapadapter.StaleMarkerCleaner` (the production wiring) exist as concrete types with identical names. They are layered: the adapter type's `CleanStaleMarkers()` constructs a fresh inner `bootstrap.StaleMarkerCleaner` per call (adapters.go:218-225). The `MarkerCleaner` interface uses a third name. The doc at bootstrap.go:92-94 explicitly acknowledges the awkwardness ("the interface name is distinct from the concrete type only to avoid an identifier collision within the same package"). Adjacent seams sidestep this — `FIFOSweeper` adapter wraps `state.SweepOrphanFIFOs`, not a `bootstrap.FIFOSweeper`.
  RECOMMENDATION: Rename one side. Cleanest fix is to rename the cmd/bootstrap concrete type to `staleMarkerCleanupRunner` (unexported) or `MarkerCleanupCore`. Alternatively rename the bootstrapadapter type to `StaleMarkerCleanupAdapter` mirroring `RestoreAdapter`/`HookRegistrar`.

- FINDING: Per-call construction of inner `bootstrap.StaleMarkerCleaner` inside `bootstrapadapter` adapter discards a stable composition opportunity
  SEVERITY: low
  FILES: internal/bootstrapadapter/adapters.go:206-226
  DESCRIPTION: `bootstrapadapter.(*StaleMarkerCleaner).CleanStaleMarkers` constructs a brand-new `&bootstrap.StaleMarkerCleaner{...}` on every invocation including a fresh `markerListerFunc` closure capturing `a.Client` (line 220). The struct has no per-tick mutable state — the inner cleaner is pure execution surface. The defensive per-call construction obscures the simpler model.
  RECOMMENDATION: Construct the inner `bootstrap.StaleMarkerCleaner` once at adapter construction (private field of `bootstrapadapter.StaleMarkerCleaner`, or collapse into the outer struct). Eliminates one allocation per bootstrap, makes the wrapping look like the other adapters in the file, and removes the closure-recapture indirection.

- FINDING: `buildLiveStructure` builds a three-level nested map but every call site uses presence-only checks
  SEVERITY: low
  FILES: internal/state/capture.go:122-170
  DESCRIPTION: `mergeSkippedPanes` consumes the full three-level live structure and the spec's three-level filtering is correct. But the merge loop (lines 125-145) only ever uses key-existence checks at each level — never reads pane values. A flat `map[string]struct{}` keyed by `SanitizePaneKey(session, win, pane)` would collapse three nested lookups into one and eliminate per-level early-exit branching.
  RECOMMENDATION: Consider rewriting as `buildLivePaneSet(idx) map[string]struct{}` keyed by SanitizePaneKey. Merge becomes a single per-prev-pane lookup. Same algorithmic complexity, fewer branches, simpler invariant. Defer if implementer expects future filter rules to want session/window-level granularity. Not a blocker — current code is correct and well-tested.

- FINDING: `MarkerLister` interface in cmd/bootstrap is satisfied in production only via `markerListerFunc` shim
  SEVERITY: low
  FILES: cmd/bootstrap/stale_marker_cleanup.go:25-27, internal/bootstrapadapter/adapters.go:198-204
  DESCRIPTION: The `MarkerLister` interface (one method: `ListSkeletonMarkers()`) is satisfied in production exclusively via `markerListerFunc(func() ... { return state.ListSkeletonMarkers(a.Client) })`. No type structurally satisfies this interface in production; only tests use `fakeMarkerLister`. Three layers (free function → func-shim → interface → struct field) where a function field on `bootstrap.StaleMarkerCleaner` would suffice.
  RECOMMENDATION: Either change `bootstrap.StaleMarkerCleaner.Markers` to a function field (`func() (map[string]struct{}, error)`) eliminating both `MarkerLister` and `markerListerFunc`, OR have an existing struct satisfy `MarkerLister` directly. Cosmetic.

- FINDING: Mass-unset hazard guard couples error semantics to operational state ("zero panes" returns error)
  SEVERITY: medium
  FILES: cmd/bootstrap/stale_marker_cleanup.go:12-20, cmd/bootstrap/stale_marker_cleanup.go:122-128
  DESCRIPTION: When `parseLivePaneSet` returns empty AND markers exist, `CleanStaleMarkers` returns `ErrZeroLivePanesWithMarkers` — a sentinel signaling "I refused to act, treat as soft warning." This conflates actionable success vs. defensive deferral with genuine tmux failures (e.g. `ListAllPanesWithFormat` error) in the same return channel. The orchestrator at bootstrap.go:267-270 Warn-and-swallows indiscriminately so behaviorally no defect surfaces, but the guard's behavior is "skip this run; next bootstrap retries" — that is a successful soft outcome, not a failure.
  RECOMMENDATION: Consider returning `nil` for the zero-panes-with-markers case and surfacing the deferral via Logger.Warn (the function already has the logger). Puts the deferral signal where deferral signals live (portal.log under ComponentBootstrap) and reserves the error return for genuine failures.

- FINDING: Bootstrap step numbering says "ten-step" but only nine concrete steps exist (Return is metadata, not a step)
  SEVERITY: low
  FILES: cmd/bootstrap/bootstrap.go:1-21, cmd/bootstrap/bootstrap.go:159, cmd/bootstrap/bootstrap.go:175, cmd/bootstrap/bootstrap_test.go:141-151
  DESCRIPTION: Package doc claims "ten-step" and lists 1-10 with step 10 being "Return." Run's internal comments call it "ten bootstrap steps" (line 175). But step 10 is just "return the result" — not a dependency, not a seam, not testable in isolation. Tests list 9 step names; the orchestrator structurally has 9 dependency-driven steps.
  RECOMMENDATION: Either drop "Return" from the numbered list and call this a nine-step sequence everywhere (doc, comments, tests), or keep "Return" but explicitly mark it as a non-step convergence point. Pick one and align.

SUMMARY: Architecture is sound at the seam level — interfaces are narrow, the new step composes cleanly with adjacent FIFO/CleanStale steps, and the live-set filter contract is single-source. Notable issues are name collisions across the two `StaleMarkerCleaner` types, per-call adapter construction obscuring intent, and a `MarkerLister` interface only ever satisfied through a func-shim. None block the fix; all are cleanups that would tighten the resulting layout.

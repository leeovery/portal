TASK: Soft-warning posture for per-marker unset and malformed-line skip (2-4)

ACCEPTANCE CRITERIA:
- Per-marker `UnsetServerOption` failures collected as soft warnings mirroring `CleanStale`, never fatal.
- Malformed `session:window.pane` lines skipped (not in live-pane set, not aborting cleanup).
- Per-unset failure does not abort remaining unsets.
- Warnings drained post-bootstrap.
- Cleanup never returns fatal.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix Component B (Soft-Warning Posture): mirrors `CleanStale` — soft warnings collected, never fatal. §Adapter Wiring: rightmost-`:` split, `.` split, `strconv.Atoi`; on parse failure skip the line, do not abort.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/stale_marker_cleanup.go`
- Per-unset failure handling: lines 147-163 — each `UnsetServerOption` failure appended to `unsetErrs`, loop continues, aggregate via `errors.Join`. `Logger.Warn(state.ComponentBootstrap, ...)` breadcrumb at line 157.
- Malformed-line skip: `parseLivePaneSet` lines 177-210 handles four malformed shapes (missing colon, missing dot, non-int window, non-int pane), emitting `Logger.Warn` and `continue`ing.
- No fatal escalation: returns plain `error`, never `*FatalError`. Orchestrator (`bootstrap.go:268-272`) Warn-and-swallows.
- Logger nil-safety: `noopLogger{}` substituted when nil.

TESTS:
- Status: Adequate
- Locations: `cmd/bootstrap/stale_marker_cleanup_test.go`, `cmd/bootstrap/bootstrap_test.go`
- Coverage:
  - `TestStaleMarkerCleanup_SoftWarningPosture` — three-unset run with mid-loop sentinel, all attempted; `errors.Is` wraps sentinel.
  - "it attempts every unset when all fail" — explicit `errors.As(err, *FatalError)` is false.
  - "the cleanup never returns a fatal error" — combined per-unset + malformed line.
  - "it skips malformed live-pane lines without aborting cleanup" — malformed skipped; well-formed entries enter live set; stale marker still unset.
  - Per-shape malformed: non-int window, non-int pane, missing dot, missing colon.
  - "logger is nil-safe under per-unset failure and malformed lines".
  - `TestOrchestratorRun_continuesPastCleanStaleMarkersFailure` — orchestrator-level: never propagates from Run; surfaces via `Logger.Warn` with `step 7` + `CleanStaleMarkers` labels; downstream Sweep + CleanStale still execute.

CODE QUALITY:
- Project conventions: Followed. Small single-method interfaces. No `t.Parallel()`.
- SOLID: Good. `LivePaneLister`, `MarkerUnsetter`, `state.ServerOptionLister` each single-responsibility.
- Complexity: Low.
- Modern idioms: Yes. `errors.Join`, `errors.Is`, `errors.As`.
- Readability: Good. Algorithm doc comment walks each step in spec order. Per-shape Warn messages operator-friendly.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Four `Logger.Warn` call sites in `parseLivePaneSet` duplicate prefix. A small helper could reduce repetition; current spelling is clear and grep-friendly.
- [idea] Spec phrase "soft warning when unexpected" implemented as "always Warn on malformed line". If transient malformed lines occur, log noise could increase; revisit if needed.

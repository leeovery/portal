# Architecture Findings — killed-sessions-resurrect-on-restart (cycle 8)

```
AGENT: architecture
FINDINGS: none

SUMMARY: Cycle 7's two cleanup tasks landed cleanly and resolved the last two residual deltas without introducing any new architectural concerns:

- T10-1 — buildHydrateCommand docstring gofmt drift (internal/restore/session.go:422-431): the literal `'\''` token that gofmt kept rewriting was replaced with the unambiguous prose "close-escape-reopen idiom for embedding a single quote inside a single-quoted string". The function body is unchanged and bare-form semantics are preserved; this was a pure docstring fix.

- T10-2 — last inline OpenLogger preamble migration (internal/restore/exit_closes_pane_integration_test.go:374-375): the five-line OpenLogger + t.Cleanup-Close preamble in setupExitClosesPane is now `logger := restoretest.OpenTestLogger(t, stateDir)`. This was the last in-scope sibling of cycle 6's 13-site migration; OpenTestLogger is now the single canonical helper across every in-scope test site.

Architectural picture across the full 45-file delta after cycle 7:

- Step seam vocabulary remains uniform: bootstrap.HookRegistrar / RestoringMarker / Restorer / EagerHydrateSignaler / MarkerCleaner / FIFOSweeper / StaleCleaner are all single-method (1-3 method) interfaces; production "Core" implementations (EagerSignalCore, MarkerCleanupCore) share the Markers/Panes/Unsetter dependency-shape pattern and the local-no-op Logger substitution at entry.
- Inverse-pair symmetry intact: state.FIFOPath / state.PaneKeyFromFIFOPath are co-located, and state.UnsetSkeletonMarkerForFIFO encodes the FIFO-basename ⇄ paneKey invariant in one place.
- Three-layer FIFO-signaling lattice unchanged: state.WriteFIFOSignal (retry-bearing primitive) ← state.SendHydrateSignal (production no-seam entry point) ← state.DefaultFIFOSignaler / state.FIFOSignaler (orchestrator seam). Each layer is justified by a distinct test-coverage need (retry-ladder, no-seam production, seam injection).
- Step ordering doc-comment lattice consistent across bootstrap.go's package doc (steps 1-10), the per-step Step-N tags in Orchestrator.Run, the seam interface docstrings, and CLAUDE.md "Server bootstrap".
- runSignalHydrate (cmd/state_signal_hydrate.go) and EagerSignalCore.EagerSignalHydrate (cmd/bootstrap/eager_signal_hydrate.go) have different enumeration shapes by design (session-scoped pane intersection vs. all-marker iteration) — they are not composition candidates and the shared seam (state.FIFOSignaler) is correctly threaded through both.
- bootstrap.NewWithDefaults (cmd/bootstrap/defaults.go) centralises the defaulting policy; its conditional real-vs-NoOp EagerSignaler selection is exercised by three dedicated tests in cmd/bootstrap/orchestrator_builder_eager_default_test.go.

No new architectural concerns surface from cycle 7's deltas. The implementation's architecture is sound across all 45 files in scope — clean boundaries between cmd, cmd/bootstrap, internal/state, internal/restore, internal/bootstrapadapter, internal/restoretest, internal/statetest; appropriate adapter/core split; proper seam injection at every test boundary; and good composition opportunities exercised (NewRestoreAdapter, NewWithDefaults, OpenTestLogger, RecordingFIFOSignaler, RecordingSleep).
```

STATUS: clean

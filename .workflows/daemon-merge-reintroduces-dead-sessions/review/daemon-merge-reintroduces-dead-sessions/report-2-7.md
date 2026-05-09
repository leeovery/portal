TASK: Scrollback-save resumption end-to-end regression (2-7)

ACCEPTANCE CRITERIA:
- After cleanup unsets a stale marker whose underlying pane is still live, next daemon tick saves scrollback (skip-save guard at `cmd/state_daemon.go:131-133` no longer applies).
- After successful bootstrap that did not surface a soft warning, no `@portal-skeleton-*` marker exists for a paneKey that has no live pane.
- Marker-set path (`internal/restore/session.go:380-384`) and hydrate-helper unset path (`cmd/state_hydrate.go:312`) unmodified.

STATUS: Complete

SPEC CONTEXT: Spec §Why This Step Is Needed (lines 127-129) and §AC #8 require integration regression proving Fix Component B closes the silent scrollback-save gap. §Out of Scope (lines 217-225) forbids touching marker-set or hydrate-helper-unset paths.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/scrollback_resumption_test.go` (3 integration tests; 354 lines)
- Notes: File `//go:build integration` gated and honours `testing.Short()`. Drives real tmux server via `tmuxtest.New`/`tmuxtest.SkipIfNoTmux` and isolated state dir via `newIntegrationStateDir`. Uses shared `runDaemonTick` helper which faithfully mirrors `cmd/state_daemon.go captureAndCommit` including skip-save guard. `_seed` session keeps live-pane set non-empty so mass-unset hazard guard does not trip. Marker seeded via `client.SetServerOption`. Out-of-scope paths confirmed unmodified at `internal/restore/session.go:383-387` and `cmd/state_hydrate.go:311-315`.

TESTS:
- Status: Adequate
- Coverage:
  - `TestScrollbackResumption_DaemonTickSavesScrollbackAfterCleanup` — primary positive: stale marker unset → re-created pane → scrollback file present and non-empty.
  - `TestScrollbackResumption_WithoutCleanupScrollbackNotSaved` — negative control with `NoOpMarkerCleaner`: marker survives → skip-save guard fires → scrollback file absent. Sharply proves regression coverage.
  - `TestScrollbackResumption_LiveHydrateInProgressMarkerPreserved` — selectivity: stale unset, live preserved.
- Notes: Three-test triangle well-sized. Non-empty-size assertion catches zero-byte regression.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`. Uses shared helpers from phase 3.
- SOLID: Good. `newProductionMarkerCleaner` localises production wiring shape.
- Complexity: Low.
- Modern idioms: Yes. `t.Setenv`, `t.TempDir`, `t.Cleanup`, functional options.
- Readability: Good. Doc comments anchor each test to spec.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Three test bodies share substantial setup; a small helper (`seedLeakedMarker`) would reduce duplication.
- [idea] `TestScrollbackResumption_LiveHydrateInProgressMarkerPreserved` overlaps marker presence/absence with cheaper unit coverage. Could be slimmed if runtime concern.

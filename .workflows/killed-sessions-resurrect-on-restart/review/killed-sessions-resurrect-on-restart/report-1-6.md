TASK: killed-sessions-resurrect-on-restart-1-6 — Multi-session cold-start integration test asserting empty marker set within 2s (AC1)

ACCEPTANCE CRITERIA (Phase 1 subset):
- Multi-session integration test (real tmux, N>=2 saved sessions): `state.ListSkeletonMarkers` returns empty within a 2-second poll window after bootstrap (AC1).
- Edge cases: N>=2 saved sessions, polls state.ListSkeletonMarkers, no client attach required to drive unset.

STATUS: Complete

SPEC CONTEXT: AC1 (specification.md line 224): N>=2 saved sessions; all `@portal-skeleton-<paneKey>` markers unset within 2s post-bootstrap; no client attach required. Test Plan → Integration (line 282) reinforces the 2-second poll contract.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go lines 109-241 (`TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s` + shared body `runEagerSignalMultiSessionAC1`).
- Notes:
  - `//go:build integration` + `testing.Short()` skip + `tmuxtest.SkipIfNoTmux` correctly fence integration-only execution.
  - Parameterised sub-tests cover N=2 and N=3.
  - Portal binary built once at parent scope (`restoretest.BuildPortalBinaryDir`) and PATH-prepended per sub-test so the in-pane `portal state hydrate` helper resolves.
  - Pre-condition guard confirms no live sessions before Run — prevents vacuous AC1 pass.
  - EagerSignaler uses NewWithDefaults' auto-default (Restore real → real EagerSignalCore).
  - Post-Run non-vacuity guard asserts each named session is live.
  - Polling delegated to `restoretest.WaitForSkeletonMarkersCleared(t, client, 2*time.Second, 50*time.Millisecond)`.
  - `dumpPortalLogOnFailure` surfaces portal.log on failure.
  - No client attach, no switch-client — orchestrator step 6 alone drives the unset.

TESTS:
- Status: Adequate
- Coverage: 2-second deadline + empty-set pass condition; N>=2 invariant via N=2 and N=3 sub-tests; no-attach-required; polls `state.ListSkeletonMarkers`; non-vacuity guard; diagnostic dump on failure.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Excellent. CI-parallelism caveat at lines 46-75.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] 2s timeout + 50ms tick hardcoded at call site. A named constant (e.g. `restoretest.AC1MarkerClearanceBudget`) would centralise the value.
- [idea] `runEagerSignalMultiSessionAC1` does not return the orchestrator/client for downstream reuse; AC4 test (task 1-8) rebuilds its own fixture — intentional.

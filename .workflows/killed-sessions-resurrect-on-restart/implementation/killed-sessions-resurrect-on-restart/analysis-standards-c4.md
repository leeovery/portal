# Standards Findings — killed-sessions-resurrect-on-restart (cycle 4)

```
AGENT: standards
FINDINGS: none

SUMMARY: Cycle 3's two cleanup tasks both landed cleanly:

- T6-1 — the three stale doc-comment cross-references now name post-T1-1 / post-T5-1 primitives:
  - cmd/bootstrap/eager_signal_hydrate.go:28 names statetest.RecordingFIFOSignaler.
  - internal/restoretest/restoretest.go:152 names state.WriteFIFOSignal / state.SendHydrateSignal.
  - internal/restoretest/restoretest.go:256 names state.WriteFIFOSignal.
  - grep -rn "recordingFIFOSignaler|writeFIFOSignal" --include='*.go' returns zero hits.

- T6-2 — NewRestoreAdapter is defined at internal/bootstrapadapter/adapters.go:104 and adopted at all four new integration-test sites:
  - cmd/bootstrap/eager_signal_hydrate_integration_test.go:203, :329
  - cmd/bootstrap/phase2_hook_fire_integration_test.go:174
  - cmd/bootstrap/phase5_marker_suppression_integration_test.go:118

Spec conformance verification:
- Fix 1 (Bootstrap eager-signaling step): cmd/bootstrap/bootstrap.go step 6 EagerSignalHydrate runs between Restore (step 5) and Clear @portal-restoring (step 7). Soft Warn-and-swallow error posture. Production wiring at cmd/bootstrap_production.go:131-136 uses state.DefaultFIFOSignaler{} per spec.
- Fix 2 (Timeout-path corrections): cmd/state_hydrate.go:260-277 handleHydrateTimeout calls unsetSkeletonMarkerOrLog (the state.UnsetSkeletonMarkerForFIFO wrapper) and runHydrate's timeout branch (lines 102-117) routes the fall-through through execShellOrHookAndExit symmetrically with file-missing. The 100ms settle-sleep is preserved at line 111.
- Fix 3 (Wrapper drop): internal/restore/session.go:426-433 buildHydrateCommand emits the bare `portal state hydrate --fifo X --file Y --hook-key Z` form with no sh -c envelope.
- CLAUDE.md "Server bootstrap" at lines 71-83 accurately enumerates the ten-step list with EagerSignalHydrate at step 6, including the post-T5-3 primitive naming (state.DefaultFIFOSignaler / state.SendHydrateSignal).

No new or remaining spec or project-convention drift introduced after cycle 3's cleanup landed.
```

STATUS: clean

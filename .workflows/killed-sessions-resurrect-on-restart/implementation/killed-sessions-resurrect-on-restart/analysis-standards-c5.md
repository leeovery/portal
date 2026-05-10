# Standards Findings — killed-sessions-resurrect-on-restart (cycle 5)

```
AGENT: standards
FINDINGS: none

SUMMARY: Cycle 4's two cleanup tasks both landed cleanly:

- T7-1 — pollUntilMarkersCleared has been collapsed into restoretest.WaitForSkeletonMarkersCleared via a parameterised tick:
  - internal/restoretest/restoretest.go:297 signature is now (t, client, timeout, tick time.Duration); docstring at lines 284-296 documents tick as mandatory.
  - grep -rn "pollUntilMarkersCleared" --include='*.go' returns zero hits.
  - Both AC1 sites in cmd/bootstrap/eager_signal_hydrate_integration_test.go (:240 and :355) call WaitForSkeletonMarkersCleared(t, client, 2*time.Second, 50*time.Millisecond).
  - The five pre-existing 10s-budget call sites (cmd/bootstrap/reboot_roundtrip_test.go:407, :1021, :1241; internal/restore/integration_full_test.go:250; internal/restore/exit_closes_pane_integration_test.go:410) all pass ", 50*time.Millisecond" per the new signature.

- T7-2 — NewRestoreAdapter docstring corrected at internal/bootstrapadapter/adapters.go:93-115. The corrected docstring accurately notes that production wiring (cmd/bootstrap_production.go:122) retains its open-coded form for parity with surrounding inline-struct adapters, and confirms migration is mechanical and out of scope. *state.Logger nil-safety is documented at lines 104-106.

Spec conformance verification (re-confirmed against cycle-4 baseline):
- Fix 1 (Bootstrap eager-signaling step): cmd/bootstrap/bootstrap.go:289-300 EagerSignalHydrate runs between Restore (step 5) and Clear @portal-restoring (step 7). Soft Warn-and-swallow error posture per spec § Failure Posture.
- Fix 2 (Timeout-path corrections): cmd/state_hydrate.go:260-277 handleHydrateTimeout calls unsetSkeletonMarkerOrLog (spec § Fix 2 → Specific Changes → 1); runHydrate's timeout branch (lines 102-117) routes the fall-through through execShellOrHookAndExit symmetrically with file-missing (spec § Fix 2 → Specific Changes → 2). The 100ms settle-sleep is preserved at line 111.
- Fix 3 (Wrapper drop): internal/restore/session.go:426-433 buildHydrateCommand emits the bare `portal state hydrate --fifo X --file Y --hook-key Z` form with no sh -c envelope per spec § Fix 3 → Behaviour.
- CLAUDE.md "Server bootstrap" at lines 69-83 accurately enumerates the ten-step list with EagerSignalHydrate at step 6 per spec § Bootstrap Step Numbering Update.

No new or remaining spec or project-convention drift introduced after cycle 4's cleanup landed.
```

STATUS: clean

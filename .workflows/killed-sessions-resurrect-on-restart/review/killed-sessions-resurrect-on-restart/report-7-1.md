TASK: killed-sessions-resurrect-on-restart-7-1 — Collapse pollUntilMarkersCleared into restoretest.WaitForSkeletonMarkersCleared via parameterised tick

ACCEPTANCE CRITERIA:
- grep for "pollUntilMarkersCleared" in *.go returns zero hits.
- restoretest.WaitForSkeletonMarkersCleared signature is (t, client, timeout, tick).
- All seven call sites compile against new signature.
- AC1 sites pass 2*time.Second, 50*time.Millisecond; pre-existing sites pass 10*time.Second, 50*time.Millisecond.
- Integration build tag preserved on all touched files.

STATUS: Complete

SPEC CONTEXT: Phase 7 cycle 4 — duplicate skeleton-marker poll helper between pollUntilMarkersCleared and restoretest.WaitForSkeletonMarkersCleared. Both ran the same loop. Cycle 4 consolidates onto shared helper by parameterising tick.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/restoretest/restoretest.go:297 — new signature `WaitForSkeletonMarkersCleared(t, client, timeout, tick time.Duration)`; hardcoded 50ms replaced by time.Sleep(tick) at line 308.
  - Docstring at lines 284-296 updated.
  - cmd/bootstrap/eager_signal_hydrate_integration_test.go:240 and :355 — two AC1 call sites pass 2*time.Second, 50*time.Millisecond.
  - cmd/bootstrap/eager_signal_hydrate_integration_test.go — old pollUntilMarkersCleared function deleted.
  - cmd/bootstrap/reboot_roundtrip_test.go:401, :1010, :1224 — three pre-existing sites pass 10*time.Second.
  - internal/restore/integration_full_test.go:250; internal/restore/exit_closes_pane_integration_test.go:406.
- Notes: All seven call sites use new parameterised form. No "AC1 violation:" prefix remains. Build tags preserved.

TESTS:
- Status: Adequate
- Coverage: Helper exercised by all seven integration call sites.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] All seven call sites pass the same 50ms tick; if future cycles confirm tick consistency, helper could revert to (t, client, timeout) with documented 50ms default.
- [idea] Docstring historical context paragraph could be trimmed at next doc-refresh.

TASK: killed-sessions-resurrect-on-restart-9-2 — Delete duplicate TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags in cmd-package

ACCEPTANCE CRITERIA:
- Delete TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags from cmd/state_signal_hydrate_test.go.
- Preserve canonical state-package test (TestOpenFIFOForSignal_NonBlockingFlags in internal/state/signal_hydrate_test.go).
- Drop runtime/syscall imports from the cmd test file only if no other consumers remain.
- Retain cobra-Execute integration test at line 405.

STATUS: Complete

SPEC CONTEXT: The seam state.OpenFIFOForSignal lives in internal/state; the canonical low-level POSIX behavioral test belongs there. The cmd layer's legitimate test surface is cobra/pflag argv-parse + RunE composition.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/state_signal_hydrate_test.go
- Verification:
  - Grep for OpenFIFOForSignalUsesNonBlockingFlags across cmd/ returns zero matches.
  - Canonical TestOpenFIFOForSignal_NonBlockingFlags preserved at internal/state/signal_hydrate_test.go:244-274.
  - Import list at cmd/state_signal_hydrate_test.go:5-16 contains only bytes, errors, fmt, os, strings, testing + three internal packages — runtime/syscall correctly dropped.
  - Cobra-Execute integration retained: TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute at line 366 (t.Fatalf at line 405).
  - TestSignalHydrate_RunEDefersLoggerClose at line 325 also retained — distinct cmd-layer concern.

TESTS:
- Status: Adequate
- Coverage: Canonical POSIX seam test lives in internal/state. cmd-layer retains tests for cmd-only concerns (cobra parsing, RunE composition, signaler delegation). No coverage gap.

CODE QUALITY:
- Project conventions: Followed — no t.Parallel(), package-level seam (signalHydrateRunFunc) restored via t.Cleanup().
- SOLID: Good. Concern relocation respects package boundaries.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. cmd-package test file now scoped to cmd-layer concerns only.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

TASK: 1-1 — Run the mandated repo-wide reference sweep for all required tokens

ACCEPTANCE CRITERIA:
- Every mandated token grepped + captured (six core + bare "ui"/"browser" + wider acceptance-gate symbol set).
- Bare quoted "ui"/"browser" greps run explicitly (incl. surface-audit preExistingPackages keys path-prefixed greps miss).
- Each hit records file:line:content + context-type tag.
- Hit set complete enough for Task 1-2 to reconcile without re-running.

STATUS: Complete

SPEC CONTEXT: The implementation-start re-sweep mandated as a hard precondition for deletion. Catches references added since investigation, especially gate-blind classes: non-compiled doc/prose and the bare-name surface-audit allow-list keys. Analysis/sweep task — no code edited.

IMPLEMENTATION:
- Status: Implemented (sweep correctness confirmed via end-state re-grep)
- Evidence: All core-token hits land only in `.workflows/`/`.tick/` artifacts — zero in `*.go`, README.md, CLAUDE.md. Both package dirs gone (Glob empty). Bare-quoted "ui"/"browser"/"browse" across `*.go`: no matches; surface-audit allow-list keys absent at pagepreview_surface_audit_test.go:292-322 (direct evidence the bare-quoted grep ran). Wider symbol set: no matches. Rename landed (TestCommandPendingNKey at model_test.go:6171, no BrowseAndNKey).
- "3 sites outside manifest" note: clean end state confirms these were genuine reconciliation catches handed to 1-2, not missed sites.

TESTS:
- Status: N/A (no production code; per spec net test delta is removal). Authoritative verification is green build/test + zero-reference grep — both satisfied.

CODE QUALITY: N/A (no code produced).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

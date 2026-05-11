TASK: killed-sessions-resurrect-on-restart-5-4 — Reconcile internal/restoretest package doc with current build-tag reality

ACCEPTANCE CRITERIA:
- Package doc-comment no longer claims package-level //go:build integration gating.
- Doc explicitly enumerates which helpers are integration-only and which are always-built.
- Doc-comment host file's own build tag does not contradict the package-level claim.

STATUS: Issues Found

SPEC CONTEXT: Phase 4 tasks 4-4 (SeedSessionsJSON*) and 4-6 (WaitForFileExists) added untagged helpers while the package doc still claimed package-level //go:build integration gating. Cycle 2 task: rewrite doc to reflect mixed-tag reality.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/restoretest/doc.go:1-41 — new untagged file dedicated to package-level doc-comment.
  - /Users/leeovery/Code/portal/internal/restoretest/restoretest.go:1-17 — host file retains //go:build integration but doc-comment was relocated to doc.go.
- Notes:
  - doc.go is untagged. Edge case satisfied.
  - doc.go:8-31 enumerates symbols by category. Always-built block: SeedSessionsJSON / SeedSessionsJSONWithSavedAt / WaitForFileExists. Integration-only block: BuildPortalBinaryDir / BuildPortalBinaryStable / ProjectRoot / PrependPATH / DriveSignalHydrate / DriveSignalHydrateBinary / WaitForSkeletonMarkersCleared / SortedKeySet.
  - doc.go:33-37 records file-tagging convention.

TESTS:
- Status: Adequate (documentation-only)
- Coverage: go vet + default/integration test invocations confirm no compile regression.

CODE QUALITY:
- Project conventions: Followed.
- Modern idioms: Idiomatic Go package-doc layout.
- Readability: Good.
- Issues: One staleness gap introduced by later task 10-2.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] doc.go does not enumerate OpenTestLogger (defined in internal/restoretest/logger.go, always-built). logger.go was added by Phase 10 task 10-2 AFTER 5-4 landed — this is a doc-staleness regression introduced by 10-2, not a defect of 5-4. Fix: append a single line to the always-built block at doc.go:9-14: `OpenTestLogger — *state.Logger opener that registers t.Cleanup, defined in logger.go.`

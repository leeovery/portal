TASK: Add SetTestHandler test-only seam restoring prior handler via t.Cleanup (portal-observability-layer-1-5)

ACCEPTANCE CRITERIA:
- SetTestHandler(t, h) causes subsequent For(...) records to route to h.
- After the test ends (cleanup), the previously-pinned handler is restored.
- Two nested SetTestHandler calls restore in correct LIFO order.
- A test that calls SetTestHandler but never logs still restores cleanly (no panic).

STATUS: Complete

SPEC CONTEXT:
Spec § The internal/log package (Public API + Init/For contract). SetTestHandler(t, h) is a test-only seam for in-process capture/silence without subprocess. "Configured once in prod" preserved by convention + this seam, explicitly NOT by panicking. Swappable atomic-guarded indirection; For-cached loggers route to the swapped handler.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/log/testhandler.go:24-29 — SetTestHandler: t.Helper(); prev := currentHandler(); setHandler(h); t.Cleanup(func(){ setHandler(prev) })
  - log.go:123-134 — unexported setHandler/currentHandler; currentHandler reads inner from same atomic cell (no mod chain), symmetric capture/restore
- Notes: All four ACs met. *testing.T-first param structurally enforces test-only consumption (mirrors portaltest.IsolateStateForTest). LIFO restoration is a free property of t.Cleanup reverse order, not bespoke logic. Never-logged-restore inherently safe (cleanup only does atomic store of valid handler). Routing-after-swap relies on swapHandler deferred-mods design but does not re-implement it. Doc comment states this is the ONLY sanctioned non-Init handler replacement and prod must never call it.

TESTS:
- Status: Adequate
- Location: internal/log/settesthandler_test.go (4 tests)
- Coverage: RoutesRecordsToTestHandler (AC1 via For-derived logger, exactly 1 record + msg); RestoresPriorHandlerViaCleanup (AC2 via t.Run subtest); RestoresNestedSwapsInLIFOOrder (AC3, distinguishes outer vs original so a non-LIFO bug fails); RestoresCleanlyWhenNeverLogged (AC4).
- Notes: Identity comparisons use currentHandler() (raw inner) against recordingHandler pointer — correct granularity. Not under/over-tested; one property per test.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; *testing.T-first seam; doc-commented export).
- SOLID: Good — single responsibility; indirection mechanics live in swapHandler/setHandler.
- Complexity: Low (4 statements).
- Modern idioms: Yes — t.Helper(), t.Cleanup(), atomic.Pointer seam.
- Readability: Good — doc comment explains LIFO and never-logged invariants.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] currentHandler() and test helper snapshotHandler() both capture/restore but differ (raw inner vs swap.load() with mods applied); both correct for their callers but a mild readability snag — consider a comment or migrating snapshotHandler callers to SetTestHandler where a *testing.T is in scope.

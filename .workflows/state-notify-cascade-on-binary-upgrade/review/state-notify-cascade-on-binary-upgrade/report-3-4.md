TASK: Rename test functions that reference deleted migration helpers (state-notify-cascade-on-binary-upgrade-3-4)

STATUS: Complete

ACCEPTANCE CRITERIA: no test function name references migrateHydrationHooks/migrateSessionClosedHook/convergeSessionClosed; each renamed test name describes the convergence behaviour; bodies/assertions/accurate doc comments unchanged; go test ./internal/tmux/... passes; -run TestRegisterPortalHooks discovers renamed functions.

SPEC CONTEXT: Migration helpers deleted; behaviour subsumed by convergeEvent driven through RegisterPortalHooks.

IMPLEMENTATION:
- Status: Implemented
- hooks_migration_test.go — 8 functions renamed to TestRegisterPortalHooks_Hydration* (:66, :112, :152, :185, :237, :285, :337, :387). hooks_register_warn_test.go — :87 TestRegisterPortalHooks_HydrationReadFailureEmitsCanonicalWarn (9th former TestMigrateHydrationHooks_*); :124 TestRegisterPortalHooks_SessionClosedReadFailureEmitsCanonicalWarn (was TestConvergeSessionClosed_...).
- `func TestMigrateHydrationHooks` / `func TestConvergeSessionClosed` → zero matches. Any test NAME containing the deleted symbols → zero, repo-wide. convergeEvent present (hooks_register.go:322).

TESTS:
- Status: Adequate (test-only rename chore). Bodies/assertions unchanged; accurate doc comments + file-level history note remain. `-run TestRegisterPortalHooks` discovers all 10 renamed functions (unanchored regex, literal prefix). Full suite green.

CODE QUALITY:
- Project conventions: Followed (TestSubject_Behaviour naming, anchored on surviving public entry point; no t.Parallel). Complexity: Low. Readability: Good (names match what bodies exercise).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

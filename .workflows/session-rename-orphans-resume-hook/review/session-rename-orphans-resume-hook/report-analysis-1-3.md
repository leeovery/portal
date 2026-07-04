TASK: Update the stale ListAllPanes prose in the shared stale-cleanup helper (runHookStaleCleanup) and clean.go so the internal docs name the enumeration actually invoked (ListAllPaneHookKeys), preserving the spec's name-based-vs-hook-key distinction. (tick-6cdf8c / T-analysis-1-3)

ACCEPTANCE CRITERIA:
- All eight comment references name ListAllPaneHookKeys (or a neutral live-key-enumeration phrasing), consistent with the actual call at run_hook_stale_cleanup.go:89.
- No code/logic change; go build ./... and go test ./cmd/... pass.
- Tests: no new test (documentation-only correction).

STATUS: Complete

SPEC CONTEXT:
Spec (specification.md §Stale-cleanup / Stage 2) draws a load-bearing distinction between the name-based structural enumeration (StructuralKeyFormat / ListAllPanes / ListAllPanesWithFormat — retained for tmux targeting and non-hook structural use) and the hook-key enumeration (HookKeyFormat / ListAllPaneHookKeys). Spec line 70: "CleanStale(liveKeys) ... The live-key enumeration that feeds it (today ListAllPanes() -> ListAllPanesWithFormat(StructuralKeyFormat)) is changed to enumerate live panes' hook keys via HookKeyFormat. ... The name-based StructuralKeyFormat / ListAllPanes remain available for any non-hook structural use; only the hook-cleanup enumeration switches to the hook-key format." Spec §Risks (line 173) flags steering a future caller back to name-based keying as the primary orphan risk — precisely the confusion these stale prose references created. This task retires that residual prose so the docs no longer point a reader back toward the name-based path.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/run_hook_stale_cleanup.go (comments at lines 16, 31, 46, 66); cmd/clean.go (comments at lines 118, 119, 144, 150). Commit c9a11aa ("T-analysis-1-3 — update ListAllPanes prose in stale-cleanup helper").
- Notes: All eight targeted prose references now read ListAllPaneHookKeys, consistent with the live call at run_hook_stale_cleanup.go:89 (`livePanes, err := lister.ListAllPaneHookKeys()`). Verified via `git show c9a11aa`: the diff is exactly 8 changed comment lines (4 per file), zero executable-line changes, no call-site or interface-signature change. `grep "ListAllPanes\b"` over both files now returns no matches (exit 1). The remaining `ListAllPanes` hits elsewhere in cmd/ are legitimately out of scope: (a) test files describing test scenarios (not the algorithm prose this task targeted), and (b) ListAllPanesWithFormat / StructuralKeyFormat in cmd/bootstrap/stale_marker_cleanup.go — a genuinely different name-based enumeration the spec explicitly retains and which must NOT be renamed. The AllPaneLister interface doc-comment (clean.go:15-21) already correctly describes hook-key form, so it needed no change.

TESTS:
- Status: Adequate (N/A for new tests)
- Coverage: No new test warranted — this is a comment-only documentation correction with zero behavioural surface. Existing cmd/ tests (run_hook_stale_cleanup_test.go, clean_test.go, bootstrap_production_test.go) already exercise the helper's error/swallow/hazard branches and pin the log format strings; the interface method under test is ListAllPaneHookKeys (run_hook_stale_cleanup_test.go:30 explicitly notes it calls "the hook-key method rather than the name-based ListAllPanes"), so the code-side behaviour is already covered. Adding a test for prose wording would be over-testing.
- Notes: Per agent constraints the suite was not executed; adequacy assessed by reading. Since the change touches only comment text, `go build`/`go test` outcomes are unchanged from the prior green state.

CODE QUALITY:
- Project conventions: Followed. Comment-only; naming aligns with the actual method name and the spec's HookKeyFormat vocabulary.
- SOLID principles: N/A (no code change).
- Complexity: N/A (no code change).
- Modern idioms: N/A.
- Readability: Improved — the algorithm/policy prose now names the enumeration actually invoked, removing the false trail that steered readers back toward the name-based ListAllPanes path.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

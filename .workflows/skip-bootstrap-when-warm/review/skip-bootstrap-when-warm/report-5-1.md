TASK: skip-bootstrap-when-warm-5-1 — Simplify runHookStaleCleanup to its post-step-11 usage — drop the dead swallowListError axis and fix the stale contract doc

ACCEPTANCE CRITERIA:
- runHookStaleCleanup no longer accepts a swallowListError parameter and contains no `return err` on the ListAllPanes-error path.
- Both production callers (cmd/state_daemon.go, cmd/clean.go) compile and behave identically to before (ListAllPanes errors logged-and-swallowed; Load/CleanStale errors still surfaced to the caller's own Warn/return).
- The contract doc names the daemon maybeRunHookCleanup and portal-clean cleanCmd.RunE as the two callers and contains no reference to step 11, cleanStaleAdapter, or StaleCleaner.
- go build ./... succeeds and go test ./cmd/... is green.

STATUS: Complete

SPEC CONTEXT:
The feature removes bootstrap step 11 (CleanStale hooks) from the orchestrator entirely and re-homes hooks cleanup on the `_portal-saver` daemon (spec §"Decision — remove hooks cleanup from the orchestrator; home it on the daemon", lines 238-258). The spec pins that the daemon reuses the existing shared cmd/run_hook_stale_cleanup.go helper (which carries the mass-deletion hazard guard) via the daemon's throttled `maybeRunHookCleanup` on the tick idle branch, with portal-clean remaining a second caller. That step-11 removal deleted the only caller that passed swallowListError=false, leaving the boolean policy axis vestigial — which this analysis-cycle task collapses.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/run_hook_stale_cleanup.go:83-97 — signature is now `runHookStaleCleanup(lister, store, logger, onRemoved)` (no bool); ListAllPanes-error branch (94-97) emits the Warn and unconditionally `return nil`.
  - cmd/run_hook_stale_cleanup.go:99-103 (store.Load → return err) and :121-124 (store.CleanStale → return err) left untouched, exactly as scoped.
  - cmd/state_daemon.go:429 — `runHookStaleCleanup(deps.Client, deps.HookStore, deps.Logger, nil)`; surrounding Warn-guard retained.
  - cmd/clean.go:155-162 — `true` argument dropped; onRemoved stdout writer + `_ =` discard preserved.
  - cmd/run_hook_stale_cleanup.go:3-57 (package-doc block) and :65-82 (function doc) rewritten: names daemon maybeRunHookCleanup + portal-clean cleanCmd.RunE → cleanStaleHooks as the two live callers, states "There is no policy parameter", keeps onRemoved + nil-logger paragraphs.
- Notes: Verified via grep that no `swallowListError`, `StaleCleaner`, or in-file "step 11" references remain, and all runHookStaleCleanup call sites use the 4-arg form. The only surviving `cleanStaleAdapter` mentions are historical removal-rationale comments in test files (bootstrap_production_test.go:4, run_hook_stale_cleanup_test.go:33-34), which are out of this task's doc-rewrite scope (that scope was the helper's own contract doc).

TESTS:
- Status: Adequate
- Coverage: cmd/run_hook_stale_cleanup_test.go retuned to the single behaviour. The "ListAllPanes error logs Warn and returns nil" subtest (107-137) now asserts nil return + exactly-one list-panes Warn + no entry Debug + no hazard Warn + hooks.json unmodified — correctly replacing the old `false → return err` assertion. The former separate swallowListError=true case is folded away (one behaviour). Retained: hazard guard, both-sides-empty no-op, Load-error → non-nil + Warn, onRemoved-once-per-removed, nil-onRemoved safe, happy-path entry+completion Debug, nil-logger tolerated. All call sites updated to 4 args. Would fail if the ListAllPanes branch regressed to propagating the error.
- Notes: The store.CleanStale-error branch (`return err`, line 123) has no dedicated subtest here, though the task's Tests list mentioned a "CleanStale error → non-nil return" retained case. That branch is unchanged by this task and is hard to trigger against a real *hooks.Store; the gap (if any) is pre-existing and out of the swallowListError scope. Not a regression introduced here.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI (AllPaneLister) preserved; error handling matches golang-error-handling (sentinel branches propagate, transient tmux read is Warn-and-continue). Removing the boolean parameter aligns with the flagged boolean-parameter anti-pattern.
- SOLID principles: Good. Collapsing the seam to its single policy variant improves single-responsibility clarity; the helper remains the one source of truth for the prune algorithm and its log format strings.
- Complexity: Low. One fewer parameter and one fewer branch; cyclomatic complexity reduced.
- Modern idioms: Yes.
- Readability: Good. Contract doc now matches the actual two callers and names the daemon dependency, removing the maintainer trap of being pointed at deleted code.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] .workflows/skip-bootstrap-when-warm/specification/skip-bootstrap-when-warm/specification.md:251-253 — the pinned dependency-wiring still documents the old 5-arg signature `runHookStaleCleanup(lister AllPaneLister, store *hooks.Store, logger *slog.Logger, swallowListError bool, onRemoved func(string))` ("takes five arguments") and its swallowListError bullet. Update to the 4-arg form and drop the swallowListError bullet so the spec matches the shipped signature. (Divergence from this pinned detail is legitimate — it follows directly from the spec's own step-11 removal at line 243 — but the spec text is now stale.)
- [do-now] cmd/run_hook_stale_cleanup_test.go:30-34 — the doc comment describes a helper `newTempHooksStoreForHelper` "re-declared here", but it is attached to `func TestRunHookStaleCleanup` and no such helper is declared in this file (the real helper is `newTempHooksStore` in cmd/bootstrap_production_test.go:131). Fix or drop the orphaned/misleading comment.
- [idea] cmd/run_hook_stale_cleanup_test.go — no dedicated subtest exercises the `store.CleanStale` error path (line 123 `return err`), despite the task's Tests list naming a retained "CleanStale error → non-nil return" case. Decide whether to add coverage (would require inducing a write failure on a real *hooks.Store, e.g. read-only dir post-Load) or confirm it is covered elsewhere.

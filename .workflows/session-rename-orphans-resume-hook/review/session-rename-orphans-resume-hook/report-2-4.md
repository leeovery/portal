TASK: 2-4 — Repoint the stale-cleanup live-key enumeration (AllPaneLister) to hook keys (tick-f57f07)

ACCEPTANCE CRITERIA:
- The hook-cleanup AllPaneLister enumerates live hook keys via ListAllPaneHookKeys (HookKeyFormat); runHookStaleCleanup calls lister.ListAllPaneHookKeys(), not ListAllPanes().
- A freshly-registered stamped-session hook survives cleanup; a truly-stale entry is removed.
- list-panes error still preserves hooks (swallow policy intact under portal clean; propagated under bootstrap adapter).
- Empty live set still triggers the mass-deletion hazard guard (deferral).
- Name-based ListAllPanes / StructuralKeyFormat unchanged; their non-hook callers (cmd/bootstrap/stale_marker_cleanup.go, cmd/state_daemon.go) untouched.
- internal/hooks/store.go CleanStale unchanged (no diff).
- Existing run_hook_stale_cleanup_test.go / clean_test.go stay green after mechanical stub rename.
- go build + go test ./cmd/... + integration matrix pass.

STATUS: Complete

SPEC CONTEXT: Specification §"Hook-Key Derivation → Stage 2" (lines 69-70) mandates exactly this change: the live-key enumeration feeding CleanStale(liveKeys) switches from ListAllPanes() → ListAllPanesWithFormat(StructuralKeyFormat) to enumerate live panes' hook keys via HookKeyFormat, because "liveKeys must be produced by the same rule as registration, or cleanup mass-orphans every stamped session's hook." The spec explicitly states name-based StructuralKeyFormat / ListAllPanes remain available for non-hook structural use; only the hook-cleanup enumeration switches. §Risks (line 173) names the missed-key-producing-site as the primary risk this closes. This is the live half of the "every key-producing site derives the same key" invariant (registration half was Task 2-2; the tmux ListAllPaneHookKeys method was Task 2-3).

IMPLEMENTATION:
- Status: Implemented (clean, minimal, matches spec verbatim)
- Location:
  - cmd/clean.go:22-24 — AllPaneLister interface method renamed to ListAllPaneHookKeys() ([]string, error); doc-comment (lines 15-21) rewritten to describe hook-key form <@portal-id or session_name>:w.p via HookKeyFormat.
  - cmd/run_hook_stale_cleanup.go:89 — single line switched: livePanes, err := lister.ListAllPaneHookKeys(). livePanes local name retained. Six-branch algorithm, hazard guard (lines 111-118), swallow policy (lines 92-95), onRemoved callback (126-130), and all log format strings unchanged. Doc block prose updated to reference ListAllPaneHookKeys (lines 16, 31, 46, 66).
  - cmd/bootstrap_production.go:94 — compile-time assertion var _ AllPaneLister = (*tmux.Client)(nil) holds (client satisfies it via Task 2-3's ListAllPaneHookKeys); cleanStaleAdapter{lister: client} wiring (line 153) unchanged; godoc updated (line 71).
  - cmd/clean.go:38-43 buildCleanPaneLister() and cmd/clean.go:89-170 cleanStaleHooks — no logic change (comment references updated only).
  - Test stubs renamed (mechanical): stubAllPaneLister (bootstrap_production_test.go:107-116), mockCleanPaneLister (clean_test.go:726-734), panickingPaneLister (cleanstale_transient_listpanes_clean_integration_test.go:82-89 — panic message updated). Compile-time var _ AllPaneLister assertions present in both test files (bootstrap_production_test.go:121, integration test:340).
- Notes: The production *tmux.Client's ListAllPaneHookKeys (internal/tmux/tmux.go:901-907) delegates to ListAllPanesWithFormat(HookKeyFormat) and shares the discriminating error contract (nil,err on tmux failure — never an empty slice), so an errored read cannot mass-orphan. Verified no remaining .ListAllPanes() call sites or AllPaneLister ListAllPanes method definitions exist in cmd/. internal/hooks/store.go CleanStale(liveKeys []string) signature/body unchanged (git diff vs main empty); it treats keys opaquely as set membership. Out-of-scope name-based callers untouched: stale_marker_cleanup.go:141 still uses ListAllPanesWithFormat(StructuralKeyFormat); state_daemon.go uses its own structural index (no ListAllPanes reference).

TESTS:
- Status: Adequate
- Coverage:
  - Stamped-hook survival: clean_test.go:326 "preserves a stamped-session hook whose id-key matches the live set" — seeds {tok123:0.0, orphan:0.0}, live set ['tok123:0.0'], asserts tok123:0.0 preserved AND orphan:0.0 removed, output exactly "Removed stale hook: orphan:0.0\n". Directly exercises AC2.
  - Hazard guard: clean_test.go:370 "zero live panes refuses to wipe hooks (hazard guard)" — live []string{}, two persisted entries, asserts no output and both entries byte-preserved (len==2). Covers AC on empty live set.
  - Swallow (portal clean): clean_test.go:422 "ListAllPanes error preserves hooks (safety net)" — lister returns err, asserts empty output and hook preserved (nil returned to user).
  - Propagate (bootstrap adapter): bootstrap_production_test.go:186 "ListAllPanes error propagates..." — swallowListError=false path returns non-nil error (errors.Is sentinel) and asserts the "stale-hook cleanup: list-panes failed" Warn flows through adapter logger exactly once.
  - Integration: cleanstale_transient_listpanes_clean_integration_test.go — transienttest.Commander intercepts list-panes -a regardless of -F, so FailExitNonZero/FailEmptyStdout still drive the swallow/hazard branches after the method rename; panickingPaneLister proves the persisted==0 early-exit never invokes the lister.
- Notes: Tests verify observable behaviour (hook survival, removal output, byte-preservation, error propagation, Warn emission) rather than the method name itself, which is the right granularity. The compile-time var _ AllPaneLister assertions convert any future name-based regression into a compile error — the task's stated safety mechanism. No over-testing: each subtest pins a distinct branch. The four subtests named in the task's Tests section map onto the present subtests (some pre-existing subtests such as "ListAllPanes error preserves hooks" retain their original display names though the underlying method is now ListAllPaneHookKeys — see non-blocking note).

CODE QUALITY:
- Project conventions: Followed. cmd package, no t.Parallel(); mocks injected via package-level cleanDeps with t.Cleanup restore (per CLAUDE.md). Interface is 1-method (interface-segregation-clean per golang-dependency-injection skill). Integration tests use IsolateStateForTest.
- SOLID principles: Good. The rename preserves interface segregation (single-method AllPaneLister); Liskov holds (production *tmux.Client and all stubs satisfy the renamed method); the algorithm's single-source-of-truth (runHookStaleCleanup) is respected — only the enumeration seam moved.
- Complexity: Low. One production line changed plus documentation and mechanical stub renames.
- Modern idioms: Yes.
- Readability: Good. Doc-comments updated in lockstep with the rename so no stale prose claims name-based keying.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/clean_test.go:422 — subtest display name "ListAllPanes error preserves hooks (safety net)" and its inner assertion messages (lines 448, 453) still say "ListAllPanes"; the exercised method is now ListAllPaneHookKeys. Rename the subtest label and messages to reduce future-reader confusion (pure test-string edit, no logic).
- [do-now] cmd/bootstrap_production_test.go:180,186 — godoc and subtest name reference "a ListAllPanes failure" / "ListAllPanes error propagates"; update to ListAllPaneHookKeys for consistency with the renamed interface (documentation/label only).
- [do-now] cmd/cleanstale_transient_listpanes_clean_integration_test.go:4,16,151,290 — file/godoc comments still describe "ListAllPanes" as the intercepted enumeration; refresh the prose to ListAllPaneHookKeys (comment-only; the transient intercept is command-name based so behaviour is unaffected).

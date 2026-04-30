# Review Report: built-in-session-resurrection-12-2

**TASK**: Implement task 5-10 — `portal attach NAME` / `portal open` reattach integration test (Phase 12 remediation of original Phase 5 task 5-10)

**ACCEPTANCE CRITERIA** (from `phase-5-tasks.md` L928-L947 + Phase 12 edge cases):
- New file `cmd/reattach_integration_test.go` with `//go:build integration` tag
- Five named test cases: bare-shell attach, path-arg open, inside-tmux switch-client, steady-state zero-rewrites, negative not-found
- Two extra acceptance bullets: has-session post-bootstrap returns true for every saved name; saved_at not advanced during steady-state window
- Isolated tmux socket per test, t.Cleanup tears it down
- Restore() drives skeleton creation BEFORE RunE inspects HasSession
- Tests skipped under `-short`
- Mock connectors used to avoid PTY/syscall.Exec hand-off

**STATUS**: Complete

**SPEC CONTEXT**:
Spec "Bootstrap Flow → Return-to-Caller Timing → CLI path": for `portal attach NAME` where the target was in `sessions.json`, skeleton was restored before the attach logic runs, so `has-session -t NAME` returns true by the time the attach needs it. Spec "Restore-Side Architecture → Restoration Trigger": live tmux session with the same name is authoritative; Portal never clobbers live sessions; steady-state reattach is a JSON read + list-sessions + diff no-op. Phase 12 was a remediation cycle — the original task 5-10 was never implemented; this task implements it from scratch and folds in the two extra acceptance bullets.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/cmd/reattach_integration_test.go` (864 lines, `//go:build integration`)
- File header (L1-L89) explicitly traces each of the seven planning acceptance bullets to its asserting test and each of the five enumerated edge cases to its covering test
- Six tests present (one fold of two related acceptance bullets):
  1. `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` (L328) — bullets 4 + 7 (saved_at)
  2. `TestReattachIntegration_HasSessionPostBootstrapForSavedNames` (L424) — bullet 6
  3. `TestReattachIntegration_AttachInsideTmuxSwitchClientPath` (L492) — bullet 3
  4. `TestReattachIntegration_AttachOutsideTmuxAttachSessionPath` (L555) — bullet 1
  5. `TestReattachIntegration_UnknownNameNotFoundError` (L614) — bullet 5
  6. `TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton` (L672) — bullet 2 (no-arg TUI branch)
  7. `TestReattachIntegration_OpenPathResolvesSavedOnlySession` (L750) — bullet 2 (path-arg branch). Phase 13 task 13-3 added alias-vs-zoxide pre-seed assertions inside this test (reviewed separately)
- Real `bootstrap.Orchestrator` wired with NoOp shims (Hooks/Saver/Sweeper/Clean) and real `RestoringMarker` + `RestoreAdapter`, so step 5's contract is exercised end-to-end rather than stubbed
- Portal binary built once via `restoretest.BuildPortalBinaryStable` (sync.Once-cached, NOT t.TempDir, with documented rationale) and prepended to PATH so the in-pane hydrate helper resolves `portal` and blocks on FIFO open(O_RDONLY) keeping skeleton panes alive
- Isolated socket via `tmuxtest.New(t, "ptl-reattach-")` with cleanup auto-registered
- `testing.Short()` skip is the first line of each test
- Mock connectors used appropriately: `mockSessionConnector` for outside-tmux/exec; `SwitchConnector{client: &mockSwitchClient{}}` for inside-tmux; `openTUIFunc`/`openPathFunc` overridden via package-level seams with `t.Cleanup` restoration

**TESTS**:
- Status: Adequate (tightly scoped; one acknowledged "belt-and-braces" cross-check)
- Coverage: All seven acceptance bullets covered with named, traceable tests. Steady-state test verifies BOTH skeleton-marker absence (proves Restore took the skip branch) AND saved_at-not-advanced (proves @portal-restoring suppressed any saver path). Negative case asserts the connector is NOT dispatched. The OpenPath test asserts both that the resolver chain produces a `PathResult` AND that `openTUIFunc` is NOT reached (guards against silent FallbackResult drift)
- The `containsAll(got, []string{"alpha"})` cross-check at L407-L410 of the steady-state test is mildly redundant with the marker-absence proof but explicitly labelled "belt-and-braces" by the author — a documented diagnostic-aid choice, not accidental over-testing
- Folding bullets 4 and 7 into a single steady-state test is a sound choice — both invariants share the same Run window and seed scaffolding

**CODE QUALITY**:
- Project conventions: Followed — `//go:build integration`; no `t.Parallel()`; DI seam pattern matches cmd-package convention; `rootCmd.Execute()` after `rootCmd.SetArgs(...)` and `resetRootCmd()` matches sibling `cmd/*_test.go`; compile-time interface assertions at L853-L863 follow the project idiom
- SOLID: Good — helpers are single-purpose; `buildReattachOrchestrator` separates wire-up from per-test logic
- Complexity: Low — no nested branching beyond if-err-return; no goroutines/channels; each test body is linear
- Modern idioms: Yes — `t.Setenv`, `t.TempDir`, `t.Cleanup` throughout; `time.Equal(...)` for monotonic-clock-safe comparison
- Readability: Excellent — file-level header maps every acceptance bullet to its asserting test; multi-paragraph godocs explain rationale
- Issues: None blocking

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [quickfix] L327 cross-reference comment "phase5_marker_suppression_integration_test.go:207-210" omits the `cmd/bootstrap/` package qualifier — that file lives at `/Users/leeovery/Code/portal/cmd/bootstrap/phase5_marker_suppression_integration_test.go`, not `cmd/`. Adding the path prefix would prevent ambiguity (project has multiple `phase5_*` test files across packages).
- [idea] The hard-coded `state.SanitizePaneKey("alpha", 0, 0)` at L380 couples the marker assertion to `seedSessionsJSON`'s single-pane shape. If the seed helper ever grows multi-pane support, this assertion would silently miss panes >0. A small helper like `expectedSkeletonMarker(name, win, pane)` colocated with the seed helper would localise the coupling.
- [idea] `reattachBuildOnce` / `reattachBinDir` / `reattachBuildErr` is the second sync.Once-cached portal-binary build pattern in the codebase. Phase 13 task 13-1 already extracted shared helpers into `internal/restoretest/`; a future cleanup could lift this once-Do wrapper there so the third consumer doesn't re-implement it.

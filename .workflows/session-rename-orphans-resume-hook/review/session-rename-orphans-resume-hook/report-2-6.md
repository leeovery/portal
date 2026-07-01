TASK: No-regression test — un-stamped, never-renamed hooks.json entry survives upgrade cleanup (tick-eb0681, suffix 2-6)

ACCEPTANCE CRITERIA:
- After cleanup, the un-stamped/never-renamed session's pre-existing name-keyed entry legacy-proj:0.0 is PRESENT (survived — name-based live key coincides with on-disk key).
- After cleanup, the truly-stale name-keyed entry gone-session:0.0 (no matching live pane) is ABSENT (swept).
- The live session is genuinely un-stamped (no @portal-id), so its hook key is the name-based legacy-proj:0.0 — exercises the name-fallback coincidence, not an id match.
- Non-vacuous: the pre-cleanup seed is asserted to contain both legacy-proj:0.0 and gone-session:0.0.
- Test skips cleanly via SkipIfNoTmux; NO t.Parallel().
- go build succeeds; test passes/skips where tmux absent.

STATUS: Complete

SPEC CONTEXT:
Spec "Testing Requirements → Legacy / no-regression" and Acceptance Criterion 5 (No-migration upgrade): a pre-fix, name-keyed hooks.json entry for an un-stamped, never-renamed session must still resolve and must NOT be mass-orphaned by stale-cleanup after the fix switches registration + cleanup to HookKeyFormat (prefer @portal-id, else session_name). The switched cleanup (Stage 2) enumerates live keys via HookKeyFormat's tmux conditional #{?@portal-id,#{@portal-id},#{session_name}}; for an un-stamped session that resolves to the NAME, which must coincide with the on-disk key so CleanStale keeps it. Firing (hydrate) is Phase 3 and explicitly out of scope here — this proves only that the entry SURVIVES the switched cleanup (the precondition for firing).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hookkey_no_regression_upgrade_test.go:47-143 (TestHookKeyNoRegressionUpgrade_UnstampedNameKeyedHookSurvives + assertLiveHookKeyPresent helper)
- Notes:
  - package cmd (not cmd_test), NO //go:build integration tag — drives runHookStaleCleanup (cmd/run_hook_stale_cleanup.go:78) directly against a *tmux.Client wired to an isolated tmuxtest socket, so it runs under `go test ./cmd/...`. Matches the task's "prefer driving runHookStaleCleanup directly … no binary build" directive.
  - Genuinely un-stamped session: ts.Run(t, "new-session", "-d", "-s", liveName) issues a raw tmux new-session on the isolated socket (internal/tmuxtest/socket.go:84) with NO set-option @portal-id, and the socket runs with `-f /dev/null` (socket.go:78) so no user tmux.conf can stamp or add hooks. The session therefore takes HookKeyFormat's #{session_name} branch.
  - The un-stamped fallback is proved at the source: assertLiveHookKeyPresent (line 132) calls the REAL ListAllPaneHookKeys (internal/tmux/tmux.go:901 → ListAllPanesWithFormat(HookKeyFormat)), i.e. the actual tmux conditional, not a stub, and Fatals if the live key is not legacy-proj:0.0. This is the load-bearing "exercises name-fallback coincidence, not an id match" guarantee and doubles as a loud tripwire if a future change stamped @portal-id on new-session.
  - runHookStaleCleanup is called with the same call shape as bootstrap step 11 (swallowListError=false, onRemoved=nil); nil logger is tolerated (run_hook_stale_cleanup.go:85). *tmux.Client satisfies AllPaneLister (cmd/clean.go:22; compile-time assertion at cmd/bootstrap_production_test.go:121).
  - No drift from the plan. Rename and persistence/capture/restore are correctly NOT exercised here.
  - `go vet ./cmd/` is clean — the file compiles; helpers newTempHooksStore (bootstrap_production_test.go:126) and keysOf (bootstrap_production_test.go:169, accepts map[string]map[string]string = the hooksFile alias returned by store.Load()) resolve. The other keysOf in the package tree lives in package cmd_test, so no symbol collision.

TESTS:
- Status: Adequate
- Coverage:
  - Survive assertion (lines 115-119): legacy-proj:0.0 present in post-cleanup store.Load() → Errorf if swept. Correct per CleanStale semantics (internal/hooks/store.go:249-291: keys present in liveKeys are kept, absent keys removed + file re-saved).
  - Sweep assertion (lines 120-124): gone-session:0.0 absent post-cleanup → Errorf if survived. Confirms cleanup correctness is not weakened.
  - Non-vacuous seed guard (lines 89-98): pre-cleanup store.Load() Fatals unless BOTH legacy-proj:0.0 and gone-session:0.0 are present, so neither survive nor sweep can pass for a trivially-missing reason.
  - Source-level guard (line 70): assertLiveHookKeyPresent Fatals unless the live enumeration actually contains legacy-proj:0.0, so the survive assertion cannot pass merely because an empty/erroring live set left the entry untouched — closes the "false survive" gap that a stubbed lister would leave.
  - Both edge cases named in the task are exercised: name fallback coincides → preserved; truly-stale name-keyed entry → swept.
- Notes:
  - Not over-tested: one test, four tightly-scoped guards, each proving a distinct property; no redundant assertions, no unnecessary mocking (real tmux + real store is the point).
  - Would fail if the feature broke: if cleanup mass-orphaned on upgrade, the survive assertion fires; if HookKeyFormat regressed to always name/always id, the source guard or survive/sweep fire.

CODE QUALITY:
- Project conventions: Followed. SkipIfNoTmux gating (line 48); NO t.Parallel() (per CLAUDE.md and spec Conventions); real-tmux via tmuxtest socket fixtures with t.Cleanup kill-server (socket.go:61). Reuses canonical helpers (newTempHooksStore, tmuxtest.New/Client/Run/WaitForSession) rather than hand-rolling. This test drives cleanup in-process and needs no daemon/bootstrap subprocess, so IsolateStateForTest is correctly not required.
- SOLID principles: Good. assertLiveHookKeyPresent typed against the AllPaneLister interface (line 132), not the concrete client.
- Complexity: Low. Linear setup → guard → drive → assert.
- Modern idioms: Yes — slices.Contains, t.TempDir-backed store, const keys.
- Readability: Good. Extensive, accurate doc-comment (lines 3-33) explaining why real tmux and not a stub, and why no integration tag; error messages are diagnostic (include keysOf(...) dumps and the file path).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

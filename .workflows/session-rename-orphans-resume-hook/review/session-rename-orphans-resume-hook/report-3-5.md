TASK: 3-5 — Integration: rename-then-restore fires the registered hook for both triggers (tick-0ccccc)

ACCEPTANCE CRITERIA:
1. External `tmux rename-session` → capture+restore+signal-hydrate fires the on-resume hook (side-effect file has exactly one HOOK_FIRED), not a bare $SHELL.
2. RenameSession-equivalent leg (the exact client.RenameSession(old,new) the in-TUI path issues) fires the hook after capture+restore+signal-hydrate.
3. A tui_test-package unit test drives the in-TUI rename path via exported tui.New + WithRenamer(mockSessionRenamer) and asserts a single RenameSession(old,new) with no hook re-keying; makes explicit the tui model wires no hook seam; no duplicate internal test / forked mock.
4. Pane process kept running across the rename in both legs (no restart / self-heal); no hook re-registration after the rename.
5. Hook registered under the STABLE id-key (tok123:0.0); captured sessions.json records PortalID == "tok123" under the post-rename name (non-vacuous, asserted before restore).
6. renameAndRefresh (and any TUI production code) UNCHANGED (scope guard).
7. Integration test is //go:build integration, skips under -short and via SkipIfNoTmux, uses IsolateStateForTest, NO t.Parallel(); tui_test unit test is pure-Go, runs everywhere.
8. go build succeeds; integration + tui tests pass (integration skips cleanly where tmux absent / under -short).

STATUS: Complete

SPEC CONTEXT: This is the headline coverage for the spec's Problem Statement — a session with a registered resume hook, renamed while its pane process keeps running, silently returns as a bare $SHELL after the next reboot because the hook is orphaned under the old name. The fix keys hooks off the immutable @portal-id (stamped at creation, persisted in sessions.json, re-baked at restore via tmux.HookKey), rename-immune for both triggers (Scope & Non-Goals: fixed at the root, no rename interception; in-TUI path needs no change). Spec Testing Requirements → "The rename gap (integration — the headline coverage)": cover BOTH triggers (raw tmux rename-session AND renameAndRefresh). Acceptance Criteria 2 & 6.

IMPLEMENTATION:
- Status: Implemented (test-only task; no production change, as required)
- Location:
  - internal/restore/rename_reboot_hook_integration_test.go (new; 3 subtests + shared runRenameRebootFire harness + findCapturedSession / verifyHookKeyed helpers)
  - internal/tui/model_test.go:1711-1776 (new subtest "it reduces the in-TUI rename path to a single RenameSession with no hook re-keying" under TestRenameSession, reusing newModelWithRenamer / mockSessionRenamer)
- Notes:
  - External leg (TestRenameRebootHook_ExternalRename): raw `ts.Run(t, "rename-session", ...)`, pane process (sleep infinity) never killed/respawned.
  - RenameSession-equivalent leg (TestRenameRebootHook_RenameSessionEquivalent): the exact client.RenameSession(old,new) the in-TUI path bottoms out in; header + inline comments document that renameAndRefresh reduces to this call + a list refresh with zero hook re-keying and cite the pure-Go tui_test subtest for the genuine seam.
  - Third integration subtest (TestRenameRebootHook_PaneProcessKeptRunning) explicitly asserts pane_pid unchanged across the rename — the load-bearing edge case ("bug bites only when the process keeps running").
  - Scope guard VERIFIED: T3-5 commit (1f8f5606) touched only rename_reboot_hook_integration_test.go, model_test.go, and manifest.json — no production .go file. Production keying primitives (tmux.HookKey/HookKeyFormat, collectArmInfos baking at session.go:117, CaptureStructure PortalID at capture.go:142/262, createSkeleton re-stamp at session.go:151-152) are all in place from prior Phase-3 tasks and are exercised, not modified. renameAndRefresh unchanged.
  - All referenced helpers resolve: restoreWithMarker (integration_test.go:43), DriveSignalHydrate/WaitForSkeletonMarkersCleared/OpenTestLogger/BuildPortalBinaryDir/PrependPATH (restoretest), state.EnsureDir/CaptureStructure/EncodeIndex/SanitizePaneKey/ScrollbackFile/SessionsJSON, session.PortalIDOption, tmux.HookKey/PaneTarget signatures all match usage.

TESTS:
- Status: Adequate
- Coverage:
  - Both spec-mandated rename triggers covered (external + RenameSession-equivalent) as sibling subtests sharing runRenameRebootFire.
  - Genuine in-TUI seam driven end-to-end in pure Go (tui_test): r-key → rename modal → Ctrl+U clear → type → Enter → executes returned tea.Cmd → asserts renamer.calls == 1 and RenameSession("alpha","renamed-alpha"); the returned msg is asserted to be SessionsMsg (the list-refresh leg). Non-vacuous: distinct expected old/new names, explicit call count.
  - Non-vacuous guards present and correct: (a) stableKey asserted == "tok123:0.0" before use; (b) post-rename @portal-id re-read live and asserted == tok123 (rename-immune); (c) findCapturedSession asserts the captured session exists under the POST-rename name; (d) sess.PortalID asserted == "tok123" BEFORE restore (guards against silent name-fallback degradation); (e) verifyHookKeyed asserts hooks.json has an on-resume entry under the stable id-key.
  - Fire assertion via assertHookFireCount(...,1) — exactly one HOOK_FIRED line; absent/empty = bare-shell miss; >1 = helper didn't exec $SHELL. Both bug directions caught.
  - Server-kill confirmed effective (list-sessions expected to error after KillServer) so a no-op Restore can't mask a pass; restored pane 0:0 asserted present under the new name before hydrate.
  - Conventions: //go:build integration, testing.Short() skip, tmuxtest.SkipIfNoTmux, portaltest.IsolateStateForTest with concrete PORTAL_STATE_DIR/PORTAL_HOOKS_FILE overrides (documented last-write-wins over the /nonexistent sentinel), no t.Parallel(). tui_test is pure-Go, no build tag.
- Notes:
  - Not over-tested: the three integration subtests are non-redundant (external trigger / in-TUI-equivalent trigger / pane-kept-running guard), each a distinct spec obligation; shared body avoids duplication.
  - assertHookFireCount is defined in the sibling durability file (rename_reboot_durability_integration_test.go, task 3-6) and reused here (and by multipane_legacy) — same test package, compiles cleanly; a deliberate shared-helper arrangement, not duplication. Non-blocking cross-file coupling noted below.

CODE QUALITY:
- Project conventions: Followed. Mirrors integration_full_test.go's save→kill→restore→DriveSignalHydrate harness and reboot_roundtrip_test.go's side-effect-file pattern; reuses existing mockSessionRenamer/newModelWithRenamer rather than forking. No t.Parallel(). Env applied via t.Setenv (single-process, in-package captures use it; subprocess env is inherited via t.Setenv which mutates os.Environ for spawned children — consistent with restoretest helpers).
- SOLID principles: Good — single shared harness, caller-supplied rename closure is a clean strategy seam across the three triggers.
- Complexity: Low. runRenameRebootFire is linear; the closure indirection is well-commented.
- Modern idioms: Yes (t.Helper, t.TempDir, t.Setenv, table-free closure injection).
- Readability: Strong. Header block and per-subtest doc-comments tie each assertion back to the exact spec clause; comments explain WHY the pane is kept running and why RenameSession is byte-equivalent to the in-TUI path.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/restore/rename_reboot_hook_integration_test.go:344 / rename_reboot_durability_integration_test.go:307 — assertHookFireCount (and, per grep, findCapturedSession/verifyHookKeyed/renameOldName consts) are shared across three integration files (3-5, 3-6 durability, multipane_legacy) but the shared leaves live in whichever file the analysis cycles settled them in, with cross-references documented only in comments. Consider consolidating the shared harness leaves into one clearly-named helper file (e.g. rename_reboot_shared_test.go) so the ownership isn't implicit in a sibling test's body. Requires a decision on which leaves to move and how to name the file, hence idea not quickfix.
- [do-now] internal/restore/rename_reboot_hook_integration_test.go:40-41 — the header's example run commands use `-run RenameReboot`, but the subtests are named TestRenameRebootHook_* (the `Hook` fragment is fine under the prefix match, so this still works). Optionally tighten the doc example to `-run TestRenameRebootHook` for exactness. Pure doc-comment, zero logic impact.

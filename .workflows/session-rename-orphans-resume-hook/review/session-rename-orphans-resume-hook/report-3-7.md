TASK: 3-7 — Integration: multi-pane fires on the correct pane + graceful legacy degradation (tick-3159e0)

ACCEPTANCE CRITERIA (from plan):
- Multi-pane: two panes under one stamped session (tok123:0.0, tok123:0.1) with distinct hooks; after rename+restore, pane 0's hook fires into hook-pane0.txt ONLY and pane 1's into hook-pane1.txt ONLY (correct per-pane routing, no cross-fire).
- The multi-pane hooks are registered under DISTINCT keys before restore (non-vacuous); each fires exactly once on its own pane.
- Legacy: an un-stamped saved session captures Session.PortalID == "", re-stamp skipped (no @portal-id set-option), baked --hook-key is name-based legacy-proj:0.0.
- Legacy: the name-based hook fires after restore; no panic anywhere in capture->re-stamp->bake->hydrate on the empty PortalID.
- (Optional) An un-stamped session with no registered hook lands on a bare $SHELL cleanly (clean miss, no panic).
- Test is //go:build integration, skips under -short and SkipIfNoTmux, uses IsolateStateForTest (env applied to subprocesses), NO t.Parallel(); no production code changed.
- go build succeeds; go test -tags integration ./internal/restore/... passes and skips cleanly where tmux absent / under -short.

STATUS: Complete

SPEC CONTEXT:
Spec Acceptance Criteria 7 ("Multi-pane. Per-pane hooks under one session remain independently addressable and fire on the correct pane after rename+restore") and 8 ("Graceful legacy. Un-stamped sessions never panic and degrade to the name-based key everywhere"). Testing Requirements → "Multi-pane (integration)" and "Legacy / no-regression (integration)". Fix Overview → hook key = prefer @portal-id else session name; tmux.HookKey(portalID, name, w, p) returns "<id>:w.p" when portalID != "" else "<name>:w.p". Restore re-stamps @portal-id from saved value and skips the stamp when empty. This is a test-only task: production plumbing (HookKey, capture PortalID, re-stamp skip, empty-lift) lands in Tasks 1-1 / 3-1..3-4; 3-7 proves the two remaining end-to-end boundaries.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/multipane_legacy_integration_test.go (new file, 519 lines, package restore_test, //go:build integration). Introduced by commit 09d0f2b7 (T3-7) — that commit touched ONLY this test file plus workflow bookkeeping (.tick/tasks.jsonl, workflow manifest.json); NO production code changed. HEAD (1624509d) unchanged.
- Notes:
  - Three test functions: TestMultiPaneLegacy_PerPaneHookRouting (AC7), TestMultiPaneLegacy_GracefulLegacyDegradation (AC8, two named subtests), TestMultiPaneLegacy_UnstampedNoHookLandsOnBareShell (optional clean-miss).
  - Reuses the Task 3-5/3-6 harness leaves exactly as claimed: restoreWithMarker (integration_test.go:43), findCapturedSession / verifyHookKeyed (rename_reboot_hook_integration_test.go:350/367), persistIndex / assertHookFireCount (rename_reboot_durability_integration_test.go:231/307), and the renamePortalID/renameOldName/renameNewName consts (rename_reboot_hook_integration_test.go:63). Adds only the multi-pane split, per-pane hook-file isolation asserts, the legacy leg, and 5 local helpers (unsetOptionValue, paneIndices, seedPaneScrollback, assertMarkerFiredOnce, assertMarkerAbsent). No duplication of shared harness logic.
  - tmux.HookKey (tmux.go:618) confirmed: non-empty portalID → "<id>:w.p", empty → "<name>:w.p" — matches every key assertion in the test.
  - Multi-pane leg correctly stamps @portal-id session-scoped (SetSessionOption on the session), so both split panes inherit tok123; capture yields Session.PortalID == tok123; topology guard asserts 1 window / 2 panes (non-vacuous).
  - Legacy leg deliberately omits SetSessionOption; a pre-restore live-server guard (unsetOptionValue) asserts NO @portal-id is present, and a post-restore guard asserts the re-stamp was SKIPPED (still unset). Captured PortalID asserted exactly "" before restore.
  - go vet -tags=integration ./internal/restore/ exits 0 (compiles clean). All referenced symbols verified to exist with matching signatures (CaptureStructure, EncodeIndex, SessionsJSON, SanitizePaneKey, ScrollbackFile, EnsureDir; restoretest BuildPortalBinaryDir/PrependPATH/OpenTestLogger/DriveSignalHydrate/WaitForSkeletonMarkersCleared; tmuxtest New/Client/Run/TryRun/WaitForSession/KillServer/SkipIfNoTmux; session.PortalIDOption).
  - DriveSignalHydrate (restoretest.go:110) iterates every pane in the named session and signals each marked pane's FIFO — so the multi-pane leg's both-panes-fire expectation is correctly driven (each pane's helper fires its own baked --hook-key). Verified.

TESTS:
- Status: Adequate
- Coverage:
  - AC7 per-pane routing: airtight. assertMarkerFiredOnce(pane0File, pane0Marker) + assertMarkerFiredOnce(pane1File, pane1Marker) prove each hook fired exactly once into ITS file; assertMarkerAbsent(pane0File, pane1Marker) + assertMarkerAbsent(pane1File, pane0Marker) prove NO cross-fire. The four-way assertion is the correct way to prove the :w.p suffix is load-bearing — a shared-id cross-fire would land the other pane's marker in the wrong file and trip an absent-assertion.
  - Non-vacuous guards: keys asserted distinct (pane0Key != pane1Key) and both present on disk (verifyHookKeyed x2) BEFORE restore; captured topology asserted 2 panes; captured PortalID asserted == tok123. Legacy leg asserts captured PortalID == "" exactly, and derives bakedKey via the same tmux.HookKey rule collectArmInfos uses, asserting it equals the name-based legacy-proj:0.0.
  - AC8: no-panic proven by restoreWithMarker returning cleanly (restore half) + clean capture (capture half); re-stamp-skip proven by post-restore unsetOptionValue == ""; name-hook-fires proven by assertHookFireCount(hookFireFile, 1). The named subtests correctly split the two AC8 obligations so a failure names the broken leg.
  - Optional clean-miss leg present: empty hooks.json, bare-shell landing proven by restore + marker-cleared with no panic and no side-effect file.
  - Edge cases from the task all covered: distinct w.p suffixes, empty-id-throughout, name-fallback coincidence with on-disk key. Base-index-drift note is documented but not exercised (acceptable — no drift is applied in these fixtures, so saved-index keys == live keys; the note is a forward guard, not a required assertion here).
- Notes:
  - Not over-tested. Each of the three functions covers a distinct boundary; no redundant happy-path variations. The pre-restore key/topology/PortalID guards are non-vacuous protection, not redundancy.
  - Not under-tested. The headline anti-cross-fire assertion is the strongest form (present-in-own AND absent-in-other). Assertions are on observable side-effects (hook-file contents, tmux option reads), not implementation details.
  - assertMarkerFiredOnce uses t.Errorf on count mismatch (non-fatal) but t.Fatalf on read error — appropriate: a missing file for a hook that must fire is a hard setup failure, a wrong count is a reportable assertion.

CODE QUALITY:
- Project conventions: Followed. //go:build integration tag; testing.Short() + SkipIfNoTmux gates in all three functions; portaltest.IsolateStateForTest(t) in all three; NO t.Parallel() (correct per CLAUDE.md — cmd-package mock injection rule and the harness convention); named subtests via t.Run. Matches golang-testing skill best-practices (build-tag isolation, named subtests, behavior-not-implementation assertions).
- SOLID principles: Good. Local helpers are single-purpose; shared harness reused rather than re-implemented (DRY honoured).
- Complexity: Low. Linear setup→trigger→capture→reboot→hydrate→assert flow per test, matching the sibling harness.
- Modern idioms: Yes. os.ReadFile/os.WriteFile, filepath.Join, t.TempDir, t.Setenv, os.IsNotExist for the absent-file-is-absent case.
- Readability: Good. Extensive doc comments tie each assertion to its Acceptance Criterion and to the upstream task that owns the production behaviour; markers are descriptive constants.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/restore/multipane_legacy_integration_test.go:141-146 — the base-index-drift edge case ("if drift applied, assert against saved-index keys") is documented in the task and referenced in comments but not exercised by any fixture (no drift is induced). Decide whether a drift-inducing variant is worth adding for defence-in-depth, or whether the existing no-drift coverage plus the documented note is sufficient. Not required by the acceptance criteria as written.

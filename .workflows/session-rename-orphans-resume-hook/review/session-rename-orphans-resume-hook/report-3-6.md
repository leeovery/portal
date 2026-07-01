TASK: 3-6 — Integration: durable across repeated reboots + post-restore cleanup keeps the restored hook (tick-f8ed1f)

ACCEPTANCE CRITERIA:
- After a rename+restore (re-stamp active), a simulated next CaptureStructure against the live server yields Session.PortalID == 'tok123' (id RE-PERSISTED — fails without Task 3-3's re-stamp).
- A SECOND restore+signal-hydrate cycle (from the re-persisted sessions.json) fires the on-resume hook AGAIN (side-effect on cycle two) — durable across repeated reboots.
- After restore, the hook stale-cleanup pass (runHookStaleCleanup fed by ListAllPaneHookKeys / portal clean) does NOT delete the id-keyed entry tok123:0.0 — the live-key set from the re-stamped live @portal-id matches the on-disk key.
- A truly-stale entry (no matching live pane) is still swept by the same cleanup pass.
- Non-vacuous: captured PortalID asserted non-empty before the second restore; live-key enumeration asserted to contain tok123:0.0 before asserting cleanup survival.
- //go:build integration, skips under -short and SkipIfNoTmux, IsolateStateForTest env applied, NO t.Parallel(); no production code changed.

STATUS: Complete

SPEC CONTEXT:
Spec §"Cross-Reboot Persistence of @portal-id" enumerates two failure chains the re-stamp guards: (a) Re-persistence — the first post-restore capture (~1s later) rewrites sessions.json from live state; a session with no live @portal-id captures PortalID=="", erasing the id → the next reboot resurrects a bare shell (lines 107); (b) Survives cleanup — bootstrap step 11 builds its live-key set from the live @portal-id; with the id absent the restored session's live key falls back to the name, no longer matching the id-keyed hooks.json entry, so cleanup deletes the just-fired hook in the same bootstrap (line 108). Stage 4 "Post-restore consistency" (line 78) states a subsequent stage-2 cleanup read of the live session yields the same key stage 3 baked. Testing Requirements → "Durable across repeated reboots" (line 149) and "Post-restore cleanup keeps the restored hook" (line 150). Maps to Acceptance Criteria 3 (durable) and 4 (cleanup safety). This is a test-only task depending on Tasks 3-1..3-4 (already merged production behaviour).

IMPLEMENTATION:
- Status: Implemented (test-only; no production code changed)
- Location:
  - internal/restore/rename_reboot_durability_integration_test.go (durability half, chain (a), package restore_test) — new, 317 lines.
  - cmd/rename_restore_cleanup_survival_integration_test.go (cleanup half, chain (b), package cmd) — new, 188 lines.
- Notes:
  - Split across two packages is correct and well-justified in file headers: chain (a) drives the full restore.Orchestrator (belongs in restore_test); chain (b) must call the UNEXPORTED cmd.runHookStaleCleanup directly, so it lives in package cmd rather than re-implementing the prune algorithm. Confirmed runHookStaleCleanup (cmd/run_hook_stale_cleanup.go) is unexported and is the real bootstrap-step-11 / portal-clean prune, invoked with the exact (client, store, nil, false, nil) shape the bootstrap adapter uses.
  - Introducing commit 13657a09 touched exactly 4 files: the two new test files + .tick/tasks.jsonl + a workflow manifest.json. Zero production .go files changed — acceptance criterion "no production code changed" satisfied.
  - Both legs faithfully reproduce restore's post-re-stamp live state: session created under the POST-rename name (renamedst) and stamped with @portal-id=tok123 via SetSessionOption, exactly what internal/restore/session.go createSkeleton leaves. HookKey(tok123, name, 0, 0) == "tok123:0.0" is asserted independent of name (verified against tmux.HookKey: id-branch ignores name).

TESTS:
- Status: Adequate
- Coverage:
  DURABILITY LEG (rename_reboot_durability_integration_test.go, TestRenameRebootHook_DurableAcrossRepeatedReboots):
  - Cycle 1 (rename → capture → restore → fire) reuses the task-3-5 shape; captureAndPersist non-vacuously guards captured PortalID==tok123 under the post-rename name; assertHookFireCount==1 after cycle 1.
  - Subtest "it re-persists the @portal-id on the next capture after restore": calls the REAL state.CaptureStructure against the live re-stamped server and asserts Session.PortalID==tok123. This is the exact production re-persistence mechanism (capture reads live #{@portal-id}), so it is a genuine tripwire for chain (a) — without Task 3-3's re-stamp it would record "".
  - NON-VACUOUS guard before cycle 2: secondSess.PortalID=="" fatals ("second restore would resurrect a bare shell"); verifyHookKeyed re-confirms the on-disk stable id-key.
  - Cycle 2 (persist re-captured index → seed fresh scrollback → kill → EnsureServer → restore → DriveSignalHydrate) then asserts (i) live @portal-id==tok123 after the SECOND reboot (durability across repeated reboots, not just first) and (ii) subtest "it fires the resume hook again on a second reboot cycle": cumulative HOOK_FIRED count==2. The cumulative-count design (one append per firing) cleanly distinguishes a second-cycle bare-shell miss (stuck at 1) from success (2) and from a helper that failed to exec $SHELL (>2). Would fail if the feature broke: dropping the re-stamp yields PortalID=="" at the re-persist subtest, and even past that a bare-shell second reboot leaves count stuck at 1.

  CLEANUP LEG (cmd/rename_restore_cleanup_survival_integration_test.go, TestRenameRestoreCleanupSurvival_KeepsRestoredIdKeyedHook):
  - Seeds hooks.json with the id-keyed live entry (tok123:0.0) + a truly-stale entry (gone-session:0.0).
  - Two-sided non-vacuous live-key guard: assertLiveHookKeyPresent(tok123:0.0) proves the re-stamp took effect (id-key enumerated), and assertLiveHookKeyAbsent(renamedst:0.0) proves the NAME key is NOT what cleanup sees — a regression that dropped the re-stamp would flip both. This is stronger than the acceptance criterion requires (which only asks for the present-check) and is a good pin.
  - Pre-run seed guard: both keys asserted present on disk before cleanup, so "survives" can't pass because the entry was never written and "swept" can't pass because it was never present.
  - Drives the REAL runHookStaleCleanup (bootstrap step-11 call shape), then asserts tok123:0.0 SURVIVES and gone-session:0.0 is SWEPT. Both directions of the acceptance criterion covered. Would fail if the feature broke: without the re-stamp the live key falls back to renamedst:0.0, tok123:0.0 no longer matches any live key, and cleanup deletes it → the survival assertion fails.

- Notes:
  - The two named durability subtests share one setup+two-cycle body (documented) but each asserts its own guarantee, so a failure names the broken leg. Not over-tested — the shared body is inherent (both facts are observed on the same physical run).
  - assertLiveHookKeyAbsent is a small amount beyond the literal acceptance criterion but is legitimate defence-in-depth (pins the id-key won from both sides), not redundant with assertLiveHookKeyPresent.
  - All four named tests from the task's Tests list are present (two as durability subtests, two as cleanup survival/sweep assertions). Test list satisfied.

CODE QUALITY:
- Project conventions: Followed. //go:build integration on both; testing.Short() skip + tmuxtest.SkipIfNoTmux(t) gate; portaltest.IsolateStateForTest(t) called in the durability leg (with concrete PORTAL_STATE_DIR/PORTAL_HOOKS_FILE overrides shadowing the sentinel, documented); NO t.Parallel() anywhere; uses restoretest / tmuxtest / portaltest helpers per the "Test isolation for daemon-spawning tests" convention. The cleanup leg is a pure-in-process runHookStaleCleanup drive against a live socket (no spawned portal subprocess), so it correctly does not need IsolateStateForTest env propagation — matching its sibling cmd/hookkey_no_regression_upgrade_test.go pattern.
- SOLID / DRY: Good. Durability leg factors the reboot cycle into captureAndPersist / persistIndex / seedScrollback / rebootAndHydrate / assertHookFireCount helpers, reusing the task-3-5 fixture consts and leaf helpers (findCapturedSession, verifyHookKeyed, restoreWithMarker) rather than duplicating. No re-implementation of the prune algorithm — the cleanup leg calls the production function.
- Complexity: Low. Linear cycle-1 → guards → cycle-2 flow; helpers are single-purpose.
- Modern idioms: Appropriate (t.Run subtests, t.Helper, t.TempDir, t.Setenv, slices.Contains).
- Readability: Strong. File headers precisely explain the chain-(a)/chain-(b) split, the bug each guards, and why each package placement is necessary; inline comments call out every non-vacuous guard and the production mechanism being exercised.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/restore/rename_reboot_durability_integration_test.go:148-159 vs 164-167 — the durability leg calls state.CaptureStructure twice back-to-back (once inside the re-persist subtest, once for secondIdx to feed cycle 2). The second capture re-reads the same live state the subtest already validated; the subtest's captured index could be threaded through to cycle 2 instead of re-capturing. Left as-is is defensible (the subtest is self-contained and a re-capture is cheap), so this is a judgement call on whether to dedupe the capture, not a mechanical fix.

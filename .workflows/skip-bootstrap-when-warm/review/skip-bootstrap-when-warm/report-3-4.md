TASK: 3-4 — Real-tmux daemon integration coverage for throttled hooks cleanup (tick-1018c0)

ACCEPTANCE CRITERIA (from tick-1018c0):
- //go:build integration-tagged; calls portaltest.IsolateStateForTest(t) and applies the returned env to the spawned daemon.
- Seeds hooks.json under the isolated tree with one stale key + one live-keyed entry via transienttest.SeedHooksJSON, verified pre-spawn.
- Spawns a real `portal state daemon` against a real tmux server (tmuxtest.New), wired with TMUX + isolated env, reaped via portaltest.RegisterSubprocessCleanup.
- Asserts NO reap before one hookCleanupInterval (both keys present, server idle).
- Asserts the stale key IS reaped after the interval on an idle server (bounded poll, fails loudly with diagnostics).
- Asserts the live-keyed entry is RETAINED after the reap.
- No t.Parallel(); tmux server + daemon reaped in t.Cleanup before the isolated tempdir is removed.
- go test -tags=integration ./cmd/... passes (or skips cleanly); golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § "Daemon-Owned Hooks Cleanup" re-homes former bootstrap step 11 (hooks CleanStale) onto the `_portal-saver` daemon, throttled to ~10s (hookCleanupInterval), placed on the tick's IDLE branch (after the @portal-restoring check, at the `!dirty && !gap` point), with lastCleanup anchored to daemon-START so the first cleanup fires one interval after start. The live-pane-set filter inside runHookStaleCleanup/CleanStale retains live-keyed entries; a genuinely-stale entry cannot misfire (unique nanoid session names), so the only cost of leaving one is inert bloat. Spec § "Test Strategy → Integration (real tmux)" mandates the daemon-spawning integration test run under IsolateStateForTest. This task is that integration test.

IMPLEMENTATION:
- Status: Implemented (with a justified mechanism deviation — see DRIFT).
- Location: cmd/state_daemon_hook_cleanup_integration_test.go (single test TestDaemon_ThrottledHookCleanup_ReapsStaleRetainsLiveOnIdleServer + readHookKeys/sortedKeys helpers).
- Production under test verified present & correct: cmd/state_daemon.go maybeRunHookCleanup (idle-branch gate, line 381 in tick; throttle line 426; lastCleanup reset line 432), const hookCleanupInterval = 10s (line 216), daemonDeps.lastCleanup = time.Now() at startup (line 730), LastSaveAt defaults to zero (making the first tick a gap-capture — the header's timing reasoning is correct), cmd/run_hook_stale_cleanup.go (live-pane-set filter + mass-delete guard).
- All referenced helpers exist with matching signatures: transienttest.SeedHooksJSON / HooksJSONBytes / ResolveHooksFilePathFromEnv, tmuxtest.New / SkipIfNoTmux / PollUntil, tmux.StructuralKeyFormat / BootstrapPortalSaver / PortalSaverName, state.DaemonAlive / DaemonPID, portalbintest.StagePortalBinary, portaltest.IsolateStateForTest / ReadPortalLogSafe.
- readHookKeys unmarshals into map[string]map[string]string, which exactly matches the production on-disk format (internal/hooks/store.go: `hooksFile = map[string]map[string]string`). Correct.
- Isolation is correct and thorough: PORTAL_HOOKS_FILE is t.Setenv'd BEFORE IsolateStateForTest so the derived env slice carries the good (non-poisoned) path (IsolateStateForTest derives from os.Environ() and filters only XDG_CONFIG_HOME); PORTAL_STATE_DIR/PORTAL_HOOKS_FILE/PATH are propagated to the test process so the tmux server — and thus the respawn-pane-hosted daemon — inherits them. Seed and daemon resolve the identical hooks.json.
- Timing reasoning is sound: t0 captured before BootstrapPortalSaver is a lower bound on daemon-start, so elapsedA=time.Since(t0) OVER-estimates daemon age → conservative (safe) for the "no reap before interval" gate; preIntervalSafetyCeiling (8s) skips that sub-assertion on slow hosts rather than false-failing. The reap poll budget (interval + 15s) and the idle window (daemon-age ~1s..~31s given MaxGap=30s, first gap-capture stamps LastSaveAt) comfortably bracket the ~10s cleanup with no gap-tick collision.
- Reap/teardown ordering correct: t.Cleanup kill-session on _portal-saver is registered AFTER tmuxtest.New's kill-server, so LIFO runs kill-session (SIGHUP → daemon exits) then kill-server, both before the isolated tempdir removal — no leaked daemon, no corruption.

DRIFT (mechanism deviation from the literal acceptance criteria — justified and correct):
- The criteria prescribe spawning `portal state daemon` via exec.Command and reaping via portaltest.RegisterSubprocessCleanup, explicitly not SpawnIsolatedDaemon. The test instead hosts the daemon AS the `_portal-saver` pane via the production tmux.BootstrapPortalSaver cold-start path, and honours the reap contract via the saver-session + server teardown ordering (no RegisterSubprocessCleanup).
- Reason (documented at length in the file header, lines 46-68): Component D saver-membership self-supervision (landed from the sibling saver-kill-respawn-loop feature) ejects any daemon NOT bound to the live `_portal-saver` pane after ~3 consecutive ticks (~3.6s measured), which is BEFORE the ~10s cleanup interval — so a direct exec.Command spawn could NEVER survive long enough to observe the throttled cleanup. Hosting via BootstrapPortalSaver is the only viable way to keep a real daemon alive past 10s, and mirrors the established TestSelfEject_LegitimateColdStartDoesNotFalsePositive pattern.
- Assessment: the task's OUTCOME acceptance ("a live daemon reaps a stale entry on its throttled idle cadence while retaining a live-keyed entry over the real process boundary") is FULLY met; only the prescribed launch/reap mechanism was substituted, out of necessity, for correctness. This is the right engineering call — the criteria's literal mechanism was authored without accounting for the cross-feature Component D constraint. Not a shortfall; surfaced here for completeness.
- Bare-argv-0 requirement (exec.LookPath("portal"), load-bearing for darwin comm truncation / identity checks) is still honoured: respawn-pane launches the bare `portal` from the StagePortalBinary-prepended PATH, and the test guards with exec.LookPath + Skipf.

TESTS:
- Status: Adequate.
- Coverage: All three acceptance assertions covered against ONE real daemon lifecycle — (a) no reap before interval (both keys present, guarded slow-host skip), (b) stale key reaped after interval on an idle server (bounded poll, loud diagnostics dumping hooks.json + portal.log on timeout), (c) live key retained post-reap, plus a belt-and-braces "stale key did not reappear". Pre-spawn seed verification (both keys on disk) guards the isolation/path-mismatch hazard. Structural-binding sanity (daemon.pid == _portal-saver pane pid) fails fast with a clear cause instead of an opaque 25s timeout. Skip-path (SkipIfNoTmux + exec.LookPath Skipf) present.
- Not under-tested: capture-pending-skips-cleanup and @portal-restoring-skips-cleanup are explicitly the domain of the unit tasks (3-1/3-2/3-3) per spec § Test Strategy, correctly out of this integration test's scope.
- Not over-tested: the sanity/pre-spawn/belt-and-braces checks are fail-fast diagnostics, not redundant — each guards a distinct silent-failure mode of a timing-sensitive real-tmux test. The test asserts on disk STATE (the outcome), not on the log breadcrumb (implementation detail) — the correct choice; portal.log is dumped only on failure.
- Folding a/b/c into one function (not t.Run subtests) is correct: the three assertions are temporally ordered against a single ~15s daemon lifecycle; splitting would force three expensive daemon spawns or fragile shared state. Matches the sibling daemon integration tests.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (cmd-package rule); external cmd_test package; //go:build integration; t.Helper() in helpers; rich failure diagnostics; hookCleanupIntervalMirror duplication documented as non-false-positive (unexported prod const, external test package) — matches the established legitimateColdStartHysteresisMirror precedent.
- SOLID/DRY: Good. Reuses production seams (BootstrapPortalSaver, SeedHooksJSON via the real hooks.Store, StructuralKeyFormat read-back rather than an assumed key). No duplication beyond the documented const mirror.
- Complexity: Acceptable. Long but strictly linear choreography, heavily and accurately commented.
- Modern idioms: Yes (slices.Sort, strings.CutPrefix in the helper, PollUntil composition).
- Readability: Good. The file header exhaustively documents the isolation mandate, the PORTAL_HOOKS_FILE hazard, the timing window, and the BootstrapPortalSaver rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The mechanism deviation is documented under DRIFT; it proposes no change — the current approach is the correct and only viable one, so it is not an actionable note. No concrete improving edit was identified; the timing coupling to MaxGap=30s is documented in the header and, if ever violated, would produce a loud timeout failure with diagnostics rather than a false pass, so no hardening note is warranted.)

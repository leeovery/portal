AGENT: standards
FINDINGS: none
SUMMARY: The skip-bootstrap-when-warm implementation is a faithful, high-fidelity realization of the specification; every load-bearing decision point conforms and no project-convention (closed log vocabulary, no t.Parallel in cmd, IsolateStateForTest, internal/state leaf rule) is violated in the in-scope files.

## Conformance verification (evidence trail)

Latch mechanism & semantics (spec § The Version-Stamped Latch):
- internal/state/markers.go:29,166-204 — @portal-bootstrapped is a server option; BootstrappedLatchSatisfied does a single TryGetServerOption read and a parse-free string equality (val == runningVersion), folding absent / version-mismatch / read-error all into not-satisfied. runningVersion is a plain string parameter (keeps internal/state a leaf, avoids the internal/tmux import cycle, makes the mismatch branch unit-testable) — exactly as the spec's design-for-test note requires. The helper intentionally swallows the read error into a bare bool (documented, matches "unreadable -> full bootstrap").

Latch set-point & timing (spec § Latch Set-Point & Timing / Insertion point in Run):
- cmd/bootstrap/bootstrap.go:497-508 — latch write via Latch.SetServerOption(BootstrappedMarkerName, Version) sits after the last soft step (SweepOrphanFIFOs), after the fatal-error gate (every fatal step returns early), and before the orchestration-complete summary + return. Best-effort: a failure is a pure WARN, never fatal, never appended to warnings, never a StepEvent. Soft-warning runs still latch; fatal aborts leave it unset.
- cmd/bootstrap_progress.go start() — the concurrent goroutine sends its terminal Done event only AFTER runner.Run returns, so the in-Run latch write precedes Done ("latch present <=> full bootstrap completed" holds before any reopen burst). Confirmed.
- cmd/bootstrap/latch_test.go — stampsLatchWithVersionAfterSoftWarning, doesNotStampLatchOnFatalAbort, swallowsLatchWriteFailureAsWarn, stampsLatchBeforeOrchestrationComplete — the exact set-point-gating matrix the spec Test Strategy mandates.

Latch-check placement & abridged wiring (spec § Latch-Check Placement & Abridged-Path Wiring):
- cmd/root.go:173 — verdict read EXACTLY ONCE (latchSatisfied := client != nil && state.BootstrappedLatchSatisfied(client, version)) and threaded; never re-read.
- cmd/root.go:186-196 — abridged gate sits upstream of shouldRunConcurrentBootstrap; injects serverStartedKey=false + tmuxClientKey, stashes NO deferredBootstrapKey (the load-bearing precondition so openTUI's instant-picker gate survives), CLI path emits warnings to stderr, TUI path leaves them in the bootstrapWarnings sink. Matches the reuse-the-sync-plumbing contract.
- cmd/root.go:303-308 — shouldRunConcurrentBootstrap dropped the ServerRunning() probe; reduces to isTUIPath && client != nil && !latchSatisfied (zero tmux round-trips), keyed off latch-not-satisfied per spec § Loading-screen trigger.
- cmd/open.go:437-459 — the serverStarted force-true "cold by construction" comment is reworded to "full bootstrap in progress" exactly as the spec directs; the force-true is retained (unchanged), warm-unlatched note added.

Abridged EnsureSaver liveness-only (spec § Abridged EnsureSaver):
- cmd/abridged_saver.go — ensureSaverLiveness composes SaverPanePIDOrAbsent (probe; present && err==nil is the only "alive" shape) -> BootstrapPortalSaver (re-ensure if absent); never calls EnsurePortalSaverVersion (no kill-barrier); a revive failure funnels bootstrap.SaverDownWarning into the same bootstrapWarnings sink and proceeds. New helper in package cmd, not an orchestrator mode — as specified. Covered by abridged_saver_test.go (self-heal, transient-error-as-absent, warning funnel, NeverInvokesVersionGate).

CleanStale removal, 11 -> 10 (spec § Daemon-Owned Hooks Cleanup / Affected Code Surface):
- cmd/bootstrap/bootstrap.go:59 totalSteps=10; package doc enumerates the ten-step sequence; step 11 emitStep call and the CleanStale step/seam/adapter are gone (no StaleCleaner interface, no NoOpStaleCleaner, no WithClean option — noop.go / defaults.go confirm).
- internal/tui/loading_progress.go:41 totalBootstrapSteps=10 (the bar denominator) and stepLabelTable keyed 1..10 contiguous (drop-key-11, not renumber) — the bar reaches 100%.
- cmd/bootstrap_progress.go, cmd/bootstrap/progress_emitter.go, internal/tui/model.go, internal/bootstrapadapter/{adapters,orphan_sweep}.go — all "eleven"/"step 11"/CleanStale doc comments moved to ten. No residual 11-step claims in in-scope production files.
- cmd/hooks_cleanstale_single_caller_guard_test.go — AST/regex guard asserts no cmd/bootstrap production file references the hooks CleanStale path (daemon + portal clean are the only automatic callers left), matching the spec Test Strategy assertion.

Daemon-owned hooks cleanup (spec § Daemon-Owned Hooks Cleanup → Operational contract / Dependency wiring):
- cmd/state_daemon.go:368-395 tick() — maybeRunHookCleanup is on the idle branch (!dirty && !gap), after the @portal-restoring early return, replacing the bare idle return — so it fires on a mostly-idle warm server and is skipped while restoring and on capture-pending ticks.
- cmd/state_daemon.go:413-421 — throttle time.Since(lastCleanup) >= hookCleanupInterval (10s, :216); lastCleanup reset AFTER the body regardless of error; error logged WARN and swallowed (never crashes the daemon).
- cmd/state_daemon.go:698-717 — HookStore built once via loadHookStore() (same configFilePath("PORTAL_HOOKS_FILE","hooks.json") resolver the foreground uses); lastCleanup anchored to daemon-START time. runHookStaleCleanup called with the four pinned args: lister=deps.Client, store=deps.HookStore, swallowListError=true, onRemoved=nil. loadHookStore only fails on path resolution (no I/O; hooks.NewStore constructs a struct), which cannot fail after the earlier state.EnsureDir succeeds — so failing daemon startup on it is inert, consistent with EnsureDir handling, not an over-escalation.
- Tests: state_daemon_hook_cleanup_test.go (throttle boundary, swallowListError=true pinned, mass-delete guard reuse) + state_daemon_run_test.go (RunsHookCleanupOnIdleTick / SkipsWhenRestoring / SkipsOnDirtyCaptureTick / SkipsOnMaxGapCaptureTick) — the full spec Test-Strategy daemon-cleanup matrix.

Conventions:
- No t.Parallel() in any feature-touched cmd test (the three cmd files that use it were not touched by this feature).
- Daemon-spawning integration tests use IsolateStateForTest (abridged_integration_test, concurrent_coldboot_integration_test, state_daemon_hook_cleanup_integration_test, bootstrap/reboot_roundtrip_test). reattach_integration_test.go (touched) isolates via PORTAL_STATE_DIR + an isolated tmux socket and NoOps the Saver (no daemon spawned), so the isolation intent holds; its feature change (driving the deferred bootstrap in the openTUI stub) is the correct adaptation to the new warm-unlatched concurrent route.
- internal/state stayed a leaf (BootstrappedLatchSatisfied takes runningVersion as a string, no cmd import); closed log-attr vocabulary respected (latch WARN uses marker/error; daemon cleanup WARN uses error; EmitCleanStaleSummary audit breadcrumb reused, no new event invented).
- go build ./cmd/... ./internal/tui/... ./internal/state/... succeeds.

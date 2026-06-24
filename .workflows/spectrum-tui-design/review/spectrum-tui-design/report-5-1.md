TASK: spectrum-tui-design-5-1 — Cold-vs-warm gating: scope the concurrent flip to the cold + TUI path only (tick-4d1691)

ACCEPTANCE CRITERIA:
- Cold + TUI (serverStarted=true AND isTUIPath) classified for the concurrent route; every other combination routes to today's synchronous path
- Warm (serverStarted=false) shows no loading page and reaches the picker via today's path — byte-for-byte parity with pre-Phase-5
- CLI/direct-path (!isTUIPath) keeps the synchronous bootstrap even when cold (serverStarted=true)
- The tmux has-server decider (where used) is ServerRunning/tmux info, a single cheap call; no extra tmux round-trips on the warm path
- Non-visual plumbing — explicitly vhs-exempt; verification is behavioural only

STATUS: Complete

SPEC CONTEXT:
§10.1 — loading page gated on serverStarted (set only when EnsureServer actually started the server). Cold boot → loading page; warm → serverStarted=false → bootstrap no-ops → straight to picker, untouched. "The flip is scoped to the COLD path only. A cheap `tmux has-server` check decides; warm keeps today's fast synchronous path, carrying zero new risk."
§10.2 — flip scoped via the existing isTUIPath; CLI/direct-path keeps the synchronous bootstrap. §14.7 — the cold-path flip is its own phase (plumbing, not a widget).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/root.go:257-264 shouldRunConcurrentBootstrap — the routing decider. Order is load-bearing and correct: isTUIPath gated first (short-circuit), nil-client defensive guard, then !client.ServerRunning() as the cold probe.
  - cmd/root.go:170-183 PersistentPreRunE concurrent branch — defers the runner via deferredBootstrapKey, does NOT set serverStartedKey, skips the registerHooks test seam, returns early.
  - cmd/root.go:185-220 unchanged synchronous path for warm + cold-CLI/direct (runBootstrap + serverStartedKey + sync.Once memo intact).
  - cmd/root.go:233-235 isTUIPath (cmd.Name()=="open" && len(args)==0) — the gate mirrors the open RunE destination=="" branch exactly.
  - internal/tmux/tmux.go:120-124 ServerRunning() — single `tmux info` round-trip; the sanctioned cheap has-server probe.
  - internal/tui/model.go:790-797 WithServerStarted — serverStarted=true → PageLoading; false → default PageSessions. Confirmed intact.
- Notes: The decision is made BEFORE the orchestrator runs (the flip's whole point), so post-bootstrap serverStarted is unavailable — the ServerRunning() probe substitutes per §10.1. The decider comment (root.go:237-256) documents exactly which signal is used and why, satisfying the task's "document which one you use and why" instruction. The deferredBootstrap seam (cmd/bootstrap_context.go) is clean and well-commented: only the runner is threaded; the client rides tmuxClientKey.

  Scope note (context, NOT a defect): task 5-1 asked for the concurrent branch to be a stub routing into today's synchronous path. The current working tree has the full concurrent machinery (goroutine + bootstrapProgressPipe in cmd/open.go:445-452 / cmd/bootstrap_progress.go) already wired — i.e. downstream tasks 5-2..5-7 have landed on top, as expected for a fully-`done` phase. Verified against the CURRENT code: the 5-1 gate itself is correct and the warm/CLI synchronous path is genuinely untouched.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/concurrent_bootstrap_gate_test.go TestShouldRunConcurrentBootstrap — all five classification cells: cold+TUI→concurrent, warm→sync, cold+direct-path→sync, non-open(list)→sync, nil-client→sync (defensive). Direct unit test of the decider, the cleanest possible coverage.
  - TestShouldRunConcurrentBootstrap_ProbesOnlyOnTUIPath — asserts via recordingCommander.Calls that non-TUI and direct-path issue ZERO probes, and the TUI path issues exactly one `info` call. This nails the "no extra round-trips on warm path" criterion at the decider boundary.
  - TestWithServerStarted_GatesLoadingPage — warm (false) lands on PageSessions not PageLoading; cold (true) lands on PageLoading. Directly covers the "warm never lands on PageLoading" criterion against the real tui.Model.
  - TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously — end-to-end through rootCmd.Execute: serverStarted=false threaded to openTUI, orchestrator runs once (synchronous), and exactly one seam tmux call ([[info]]) — the byte-for-byte warm-path parity assertion.
  - cmd/concurrent_bootstrap_route_test.go — TestPersistentPreRunE_ColdTUI_DefersBootstrap (orchestrator NOT run in PersistentPreRunE, deferred bootstrap seen by openTUI), TestPersistentPreRunE_WarmTUI_RunsSynchronously (orchestrator runs once, no deferral, serverStarted=false), TestPersistentPreRunE_ColdCLI_RunsSynchronously (cold `list` still synchronous — the scoped-to-TUI-only proof).
- Notes: Coverage maps 1:1 onto the task's five named test cases plus the three edge cases (warm zero-new-round-trips asserted via call counts; isTUIPath zero-arg mirroring; cold-direct-path does not take the concurrent route). The coldCommander fake (info→DeadlineExceeded ⇒ ServerRunning()==false) and runningClient (info→nil ⇒ true) model the cold/warm split precisely through the real *tmux.Client. No t.Parallel() anywhere (cmd package mutable-mock-state rule honoured); package state reset via resetBootstrapOnce(t)/resetRootCmd and restored via t.Cleanup. Not over-tested — each case asserts a distinct cell; not under-tested — the probe-count and synchronous-parity edges are explicitly covered. The two test files carry "Task spectrum-tui-design-5-2" in their header comments though they primarily exercise 5-1's gate (the gate and the route landed together); cosmetic, see non-blocking note.

CODE QUALITY:
- Project conventions: Followed. Small interfaces / DI-by-package-var pattern respected; ServerRunning() reused (not re-derived); context-key delivery follows the existing serverStartedKey/tmuxClientKey idiom; no t.Parallel; absolute spec-section references in comments match house style.
- SOLID principles: Good. shouldRunConcurrentBootstrap is a single-responsibility pure decider; the deferred-bootstrap seam isolates the route decision from the route mechanism.
- Complexity: Low. The decider is two guarded returns; cyclomatic complexity trivial. Short-circuit ordering doubles as the "probe only on TUI path" guarantee — elegant, no separate branch needed.
- Modern idioms: Yes. Comma-ok context type-assert in deferredBootstrapFromContext/serverWasStarted; defensive nil-client guard.
- Readability: Good. The root.go:237-256 doc comment is unusually thorough and correctly explains the cold-before-bootstrap probe rationale and the isTUIPath-first ordering.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/concurrent_bootstrap_gate_test.go:6 and cmd/concurrent_bootstrap_route_test.go:3 — both file-header comments attribute the tests to "Task spectrum-tui-design-5-2", but they centrally exercise the 5-1 gate (shouldRunConcurrentBootstrap classification + WithServerStarted PageLoading gate). Correct the task attribution to 5-1 (or 5-1/5-2) so the test provenance comment is accurate.
- [do-now] cmd/concurrent_bootstrap_gate_test.go:30-33 — the runningClient/coldClient doc comment reads "errorClient returns one whose `info` FAILS", but the helper is named coldClient (there is no errorClient). Fix the dangling name in the comment to coldClient.

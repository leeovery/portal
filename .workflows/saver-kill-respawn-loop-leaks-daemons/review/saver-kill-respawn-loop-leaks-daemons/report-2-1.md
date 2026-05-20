TASK: Thread ctx from defaultDaemonRun through tick into captureAndCommit (signature change + happy-path regression)

ACCEPTANCE CRITERIA:
- tick and captureAndCommit accept ctx context.Context as first parameter.
- defaultDaemonRun calls tick(ctx, deps); tick calls captureAndCommit(ctx, deps); defaultShutdownFlush calls captureAndCommit(context.Background(), deps).
- Comment above shutdown-flush call documents the non-cancellable rationale.
- internal/state/capture.go unchanged; CaptureStructure and CaptureAndHashPane signatures unchanged.
- No files outside cmd/state_daemon.go and its sibling test files modified.
- All existing tests updated and green.
- New happy-path regression test passes.

STATUS: Complete

SPEC CONTEXT: Spec §Change 2 requires ctx to reach inside captureAndCommit so subsequent observation points can honour SIGHUP-driven cancellation mid-tick. Signature changes must stay local to cmd/state_daemon.go; internal/state/capture.go must remain untouched. Shutdown flush must remain non-cancellable so the on-exit save is not aborted by the same signal that triggered it.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go
  - defaultDaemonRun (line 70) calls tick(ctx, deps) at line 76.
  - tick signature at line 94: `func tick(ctx context.Context, deps *daemonDeps)`.
  - tick calls captureAndCommit(ctx, deps) at line 110.
  - captureAndCommit signature at line 132: `func captureAndCommit(ctx context.Context, deps *daemonDeps) error`.
  - defaultShutdownFlush (line 219) calls captureAndCommit(context.Background(), deps) at line 233 with the required comment at lines 230-232.
- internal/state/capture.go verified unmodified: CaptureStructure at internal/state/capture.go:62 and CaptureAndHashPane at internal/state/scrollback.go:79 — neither carries ctx.
- Grep confirms no callers of tick(` or captureAndCommit(` exist outside cmd/state_daemon.go and its sibling test files.

SCOPE NOTE: The task brief explicitly says "No ctx.Done() observations are added in this task — they ship in Tasks 2-2, 2-3, 2-4." The committed file already contains all three observation points (lines 139-143, 157-161, 174-178). Those are out of scope for Task 2-1 verification.

TESTS:
- Status: Adequate
- Coverage:
  - New happy-path regression test TestCaptureAndCommit_UncancelledCtxMatchesPreThreadingBehaviour at cmd/state_daemon_run_test.go:882 — drives a two-session / three-pane fixture, calls captureAndCommit(context.Background(), deps), asserts: PrevIndex pointer replacement away from sentinel, state.Commit invoked exactly once, all panes processed (3 capture-pane calls, 3 scrollback files on disk).
  - Existing tick caller(s) updated to pass context.Background() (cmd/state_daemon_run_test.go:284).
  - Existing captureAndCommit callers updated to pass context.Background() (lines 907, 988, 1076, 1165, 1217).
- Notes: Multi-pane (2 sessions × 3 panes) fixture covers the inner-loop nesting. Focused — not over-tested. No t.Parallel() (per CLAUDE.md).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); reuses daemonFakeCommander seam and makeDeps helper.
- SOLID: Good. Minimal threading change; no leaking abstractions into internal/state.
- Complexity: Low. Pure plumbing — function signatures + three call sites.
- Modern idioms: Yes. Standard Go ctx-as-first-parameter convention.
- Readability: Good. Shutdown-flush comment documents the non-cancellable invariant.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The three ctx.Done() observation points are already committed alongside the plumbing. The plan's "plumbing-only step" isolation strategy was relaxed during execution.

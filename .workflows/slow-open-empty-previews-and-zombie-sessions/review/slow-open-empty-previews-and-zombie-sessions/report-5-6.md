TASK: 5-6 — Integration test self-eject on saver pane pid mismatch via respawn-pane -k

STATUS: Complete

SPEC CONTEXT: Component D "Self-eject on saver pane pid mismatch" — `HasSession=true` but `SaverPanePID != os.Getpid()`; after N=3 consecutive false probes, INFO marker + `osExit(0)` bypassing shutdown.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_self_supervision_integration_test.go:415-635`
- Production code: `cmd/state_daemon.go:149` (N=3) and 242-252 (probe + counter + `osExit(0)`)
- Dead PID staging uses `exec.Command("true"); cmd.Run()` pattern (435-442)
- Saver pre-created with placeholder + destroy-unattached=off (449-455)
- Pre-action structural divergence guard (`daemonPID != panePID`, 533-538) fails loudly on PID coincidence
- Lock-acquire poll bounded to 2s confirms daemon reached tick loop
- 2s exit-latency floor (629-634) catches "counter incrementing outside per-tick path"

TESTS:
- Status: Adequate
- Four assertions in one function rather than four `t.Run`:
  - A: Exit code == 0
  - B: No panic on stderr
  - C: Self-eject INFO marker in portal.log
  - D: `_portal-saver` session survives eject
  - Bonus: ≥2s latency floor
- Single tmux-server churn for single daemon lifecycle (justified collapse)
- Every failure dumps portal.log + stderr

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `IsolateStateForTest`; direct daemon spawn (no orchestrator)
- SOLID: Good; shared consts DRY across sibling tests
- Complexity: Acceptable; verbosity is diagnostic plumbing
- Modern idioms: `select`+`time.NewTimer`; `errors.Is`; `strings.Builder`
- Readability: Good; file-header docstring; inline comments at every non-obvious step

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan listed four discrete `t.Run` subtests; collapsed into one with four labelled assertions; consider t.Run on shared fixture if -run granularity needed later
- [quickfix] Line 476 sets `PORTAL_LOG_LEVEL=INFO` without rationale; mirror sibling's comment at 172-178 to defend against drop
- [idea] `selfEjectExitPollTick` (97) compile-time guard `var _` at line 639 is now redundant given const IS used at 517 and 1043

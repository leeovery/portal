TASK: 7-5 — Colocate WriteVersionFile with WritePIDFile in defaultDaemonRun

STATUS: Issues Found (non-blocking — stale comment in lock-held test)

SPEC CONTEXT: c1 architecture finding-3 — T4-8 moved acquire+pidfile into defaultDaemonRun but left WriteVersionFile in RunE. Colocating versionfile after pidfile preferred for maintainability.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_daemon.go:220` — WriteVersionFile inside defaultDaemonRun immediately after WritePIDFile (217), before daemonLockFile assignment (223) and ticker dispatch (225)
  - `cmd/state_daemon.go:194-203` — comment block documents 3-step sequence (acquire → pidfile → versionfile)
  - `cmd/state_daemon.go:456-462` — RunE residual comment notes the move
  - `cmd/state_daemon.go:485` — production wiring passes `Version: version` into daemonDeps
- Error wrapping (`"write version file: %w"`) matches prior RunE idiom
- No change to defer/cleanup or shutdown semantics

TESTS:
- Status: Adequate (one strengthening opportunity)
- `cmd/state_daemon_run_test.go:1253` `TestDefaultDaemonRun_WritesVersionFileFromDepsVersion` — direct regression guard
- AST adjacency guard `cmd/state_daemon_lock_pid_ordering_test.go` still in place
- Happy-path `TestStateDaemon_WritesVersionFileOnStartup` continues to pass
- `TestStateDaemon_DoesNotWritePIDFileWhenLockHeld` — only asserts daemon.pid absence

Behavior shift: lock-held early-return now also skips daemon.version (WriteVersionFile inside defaultDaemonRun after acquire err-guard). Worth pinning with `os.Stat`/`IsNotExist` assertion.

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; single function owns full startup-write sequence
- Complexity: Low; three sequential if-err blocks, no branching added
- Modern idioms: `errors.Is`, `fmt.Errorf %w`
- Readability: Good; comment block 194-203 documents ordering

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] `cmd/state_daemon_test.go:543-548` — comment in `TestStateDaemon_DoesNotWritePIDFileWhenLockHeld` factually wrong; states "daemon.version IS written when lock-held (WriteVersionFile runs in RunE before defaultDaemonRun's acquire call)" but post-7-5 daemon.version is NOT written on contention. Test still passes (only checks daemon.pid) but comment misleads future readers
- [idea] Extend `TestStateDaemon_DoesNotWritePIDFileWhenLockHeld` to also assert `daemon.version` absent under lock-held; strengthens contract

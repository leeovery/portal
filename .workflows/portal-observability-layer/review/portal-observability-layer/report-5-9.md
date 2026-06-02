TASK: Emit daemon lock acquired (tmux_pane, dropping daemon: spawn) and normal-path shutdown (reason + flush_completed) (portal-observability-layer-5-9)

ACCEPTANCE CRITERIA:
- daemon: lock acquired pid=D tmux_pane=%N after lock + pidfile acquire succeed; tmux_pane from TMUX_PANE env; pid auto-baseline.
- Lock-held (ErrDaemonLockHeld) and non-EWOULDBLOCK lock-error paths emit no lock acquired, keep WARN.
- No daemon: spawn event (dropped; process: start process_role=daemon covers startup).
- defaultShutdownFlush emits exactly one daemon: shutdown reason=... flush_completed=... per invocation on normal return path.
- flush_completed=false on restoring-skip and read-error branches; reflects captureAndCommit on the flush-attempted branch.
- reason distinguishes sighup/signal/exit via captured signal; not hardcoded.

STATUS: Complete

SPEC CONTEXT:
Spec § Saver and daemon lifecycle taxonomy (867-921). lock acquired INFO (pid auto-baseline + tmux_pane) at post-pre-check acquisition; shutdown INFO (reason ∈ {sighup,signal,exit}, flush_completed bool) on normal return, NOT on self-eject path. Redundant daemon: spawn dropped, its tmux_pane migrates onto lock acquired. Reason space closed.

IMPLEMENTATION:
- Status: Implemented
- Location: state_daemon.go:270 (Info("lock acquired","tmux_pane",os.Getenv("TMUX_PANE")), after acquire+WritePIDFile+WriteVersionFile + daemonLockFile=lockFile :263; not on lock-held :250-253 / non-EWOULDBLOCK :254-255, both keep WARN); :538-565 (defaultShutdownFlush one shutdown INFO per return path: read-error :547 false, restoring-skip :552 false, flush-attempted :563 flush_completed=flushErr==nil; ad-hoc INFOs demoted to DEBUG :551/555); :40-76 (shutdownSignal atomic.Pointer + recordShutdownSignal + shutdownReason mapping SIGHUP→sighup/SIGTERM→signal/nil→exit); :648-659 (RunE goroutine records signal before cancel — race-free).
- Notes: pid auto-baseline injected by handler (init.go:51,75) not call site. daemon: spawn absent (only comment/test refs). lock-acquired Info past i+2 AST-pinned acquire→WritePIDFile adjacency (sanctioned).

TESTS:
- Status: Adequate
- Location: cmd/state_daemon_lifecycle_log_test.go + self_eject_log_test.go
- Coverage: lock-acquired tmux_pane=%42; no-lock-acquired+WARN on lock-held + non-EWOULDBLOCK; one shutdown reason=sighup flush_completed=true clean; flush_completed=false restoring-skip; false when final capture errors; shutdownReason table sighup/signal/exit; SIGTERM→signal + no-signal→exit e2e; exactly-one-shutdown-per-invocation; false on restoring-read-error; self-eject cross-check (DoesNotEmitShutdownLine).
- Notes: logtest.Sink rendering; countLines==1 exact assertions. pid baseline correctly NOT asserted (sink doesn't inject; covered by internal/log handler tests). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (seam pattern; log.For("daemon"); no t.Parallel; t.Cleanup restore).
- SOLID: Good — shutdownReason/recordShutdownSignal isolate signal-classification onto daemonDeps; closed reason single switch.
- Complexity: Low.
- Modern idioms: Yes (atomic.Pointer[os.Signal] belt-and-braces over race-free store-then-cancel/cancel-then-read).
- Readability: Good — ordering contract + value mapping + dropped-spawn rationale documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] lock acquired INFO sits after WriteVersionFile too (correct/sanctioned); the in-source comment could note this so a future reader doesn't read the task's literal "after daemonLockFile = lockFile" as a stricter placement contract. Comment-clarity nicety.

TASK: 4-1 — Add SIGKILL escalation to killSaverAndWaitForDaemon with identity-check

STATUS: Complete

SPEC CONTEXT: Component A — direct SIGKILL escalation (not SIGTERM-with-marker). Identity-check must immediately precede kill(2) to minimize µs-scale PID-recycle window. Transient identity errors must skip signal. Eliminates 5 s ceiling for orphan case.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tmux/portal_saver.go:365-446` (`killSaverAndWaitForDaemon` + `waitForPriorPIDExit` + `escalateKillToSIGKILL`)
- Seams: `BarrierEscalationTimeoutSeam`, `BarrierSendSIGKILLSeam` at `export_test.go:124-132`; IdentifyDaemon shared at `saver.IdentifyDaemon`
- `escalateKillToSIGKILL` (424-446): IdentifyDaemon → err-or-not-IsPortalDaemon WARN+return; otherwise `SendSIGKILL` literally next executable statement (no log, no IsAlive probe)
- WARN messages match spec verbatim
- Production legitimate-path (SIGHUP via tmux kill-session) untouched
- EscalationTimeout=1s, PollInterval=50ms
- Grep confirms only one `syscall.Kill` call site and it sends SIGKILL; no SIGTERM emission anywhere

TESTS:
- Status: Adequate
- Coverage at `internal/tmux/portal_saver_test.go:3515-4042` — all 10 spec-mandated tests:
  1. Happy path identity-check → SIGKILL
  2. IdentifyDead skips
  3. IdentifyNotPortalDaemon skips
  4. Transient identity error skips (Component A semantics)
  5. SIGKILL succeeds within window
  6. SIGKILL succeeds but process survives → one WARN + nil
  7. Identity-check immediately precedes SIGKILL (probeLog adjacency check at 3860-3885)
  8. NeverSendsSIGTERM (signal recorder)
  9. PriorPID dies during session-kill poll → escalation never runs
  10. NoPIDFile → escalation never runs
- EscalationTimeout shrunk via seam; not over-tested

CODE QUALITY:
- Project conventions: Followed; seam-struct pattern; no `t.Parallel`
- SOLID: Good; `escalateKillToSIGKILL` focused; `waitForPriorPIDExit` shared
- Complexity: Low; 22-line helper, single branch
- Modern idioms: `time.NewTicker` + `defer Stop`; deadline computed once
- Readability: Good; docstrings call out "no work between check and kill" invariant
- Security: SIGKILL gated behind strict `IdentifyIsPortalDaemon`; transient errors bias to not signal

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `escalateKillToSIGKILL` swallows `SendSIGKILL` error; ESRCH correctly treats already-dead as success but EPERM also silently swallowed — DEBUG breadcrumb on non-ESRCH would aid post-mortems
- [idea] `waitForPriorPIDExit` uses `for range ticker.C` — first probe delayed ~50ms; pre-loop probe would shave that tick

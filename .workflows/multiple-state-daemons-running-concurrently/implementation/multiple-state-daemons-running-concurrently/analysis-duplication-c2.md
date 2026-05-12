AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1
SUMMARY: One low-severity actionable finding — the integration test still inlines the pre-recycle tmux server PID capture despite cycle-1 extracting captureTmuxServerPID for the post-recycle capture.

FINDINGS:

- FINDING: tmux server PID capture inlined once and extracted once in the same file
  SEVERITY: low
  FILES: internal/tmux/portal_saver_integration_test.go:170-176 (inline) and :263-271 (helper); helper consumer at line 248
  DESCRIPTION: TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle captures the tmux server PID twice via the exact same `display-message -p '#{pid}'` recipe. The first capture (lines 172-176) is inlined as `serverPIDOut := sock.Run(t, "display-message", "-p", "#{pid}"); serverPID, err := strconv.Atoi(strings.TrimSpace(serverPIDOut))`. The second capture at line 248 routes through `captureTmuxServerPID`, which does byte-identical work. The helper's own doc comment at lines 258-262 claims "Extracted as a helper because the test captures the server PID twice" — but only the second site was migrated, so that motivation is currently misleading.
  RECOMMENDATION: Replace lines 172-176 with a single `serverPID := captureTmuxServerPID(t, sock)` call. ~5 lines removed and the helper's stated rationale becomes truthful.

Non-findings (considered and rejected):
- The 27 withDaemonLockFileReset call sites are intentional per-test isolation of the package-level daemonLockFile var (spec-mandated retention).
- The installBarrier* helpers in portal_saver_test.go preserve typed-pointer semantics that a generic seam-swapper would erase.
- waitForLiveDaemon vs waitForNewLiveDaemon differ by exactly one predicate; merging would obscure the load-bearing distinction.
- ERROR vs WARN per-line scan uses different primitives (co-occurrence vs strings.Count); not duplication.
- No new duplication introduced by the cycle-1 fixes.

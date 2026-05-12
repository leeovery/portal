# Review Report — Task 3.1

TASK: Restore spec-mandated pgrep server-children assertion in singleton integration test

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- Test asserts `pgrep -P <serverPID> -f 'portal state daemon'` returns exactly one PID after both `EnsurePortalSaverVersion` calls.
- Existing alive/dead pair assertions remain.
- Test fails with a diagnostic message including raw pgrep output if the count is not 1.
- Hand-mutated kill-barrier skip confirms the new assertion fails (implementer verification step).

SPEC CONTEXT:
specification.md § Acceptance Criteria → Singleton invariant (line 188-193) and § Integration test — singleton invariant under real tmux (line 319-329) explicitly mandate `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l == 1`. The pidfile-only alive/dead pair cannot detect orphan daemons parented to the tmux server but not pointed at by daemon.pid.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver_integration_test.go:242` — re-captures post-recycle server PID via `captureTmuxServerPID(t, sock)`.
  - `:243` — `count, raw := countDaemonChildren(t, postRecycleServerPID)`.
  - `:244-248` — asserts `count != 1` with diagnostic dump including raw pgrep output.
  - `:251-265` — `captureTmuxServerPID` helper.
  - `:267-292` — `countDaemonChildren` helper executes `pgrep -P <serverPID> -f 'portal state daemon'`, handles pgrep exit semantics.
  - `:210-217` — pre-existing alive/dead assertions retained.
- Notes:
  - Correctly re-captures the server PID after the recycle (line 242). The saver is the only session on the isolated socket, so the second `EnsurePortalSaverVersion`'s kill-session exits the tmux server itself; pre-recycle PID would produce `pgrep -P <stalePID>` exit-1 over-fail.
  - Beneficial drift from analysis-tasks-c1.md step 2 wording — implementation correctly recognised the post-recycle PID is the right parent.

TESTS:
- Status: Adequate
- Coverage:
  - Asserts spec-mandated count == 1 invariant against real tmux + real portal binary.
  - Retains structural alive/dead pair.
  - Diagnostic dump emits raw pgrep output, daemon.pid/daemon.version contents, server PID, and host-wide `pgrep -fl 'portal state daemon'` listing.
- Notes:
  - `countDaemonChildren` correctly distinguishes pgrep exit 1 (no-match → count 0) from exit 2+ (pgrep error → surfaced in raw string).
  - Diagnostic format string includes "expected exactly 1" and "pgrep raw output:" + raw.
  - Hand-mutation verification step is a manual implementer check.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`. Helpers use `t.Helper()`. `errors.As` for `exec.ExitError`.
- SOLID: Good. Each helper has one responsibility.
- Complexity: Low.
- Modern idioms: `errors.As`, `strings.TrimSpace` + `strings.Split`.
- Readability: Good. Extensive doc comments on each helper.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] In `dumpDiagnostics` the `serverPID` parameter is the post-recycle PID at line 245 but the pre-recycle PID at lines 211, 215. Consider clarifying the label as "tmux server PID (at capture time)" or always re-capturing inside `dumpDiagnostics`.
- [idea] The "hand-mutated kill-barrier skip" verification step is a manual implementer check. Encoding the kill-barrier-skip negative case as a separate test would harden long-term confidence.

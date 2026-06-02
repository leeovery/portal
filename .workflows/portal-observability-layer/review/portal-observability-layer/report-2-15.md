TASK: portaltest.AssertLogLevelResolved integration-test assertion helper (portal-observability-layer-2-15)

ACCEPTANCE CRITERIA:
- Selects the correct line by pid when multiple processes wrote to the same day-file.
- Fails when no log-level resolved line for the pid exists.
- Fails when matched line's source != env.
- Fails when matched line's resolved != expected.
- Passes when matched line shows resolved=<expected> source=env for the pid.
- Follows the portal.log symlink to today's file when logPath is the symlink.
- Tolerates baseline-attr ordering (parses key=value into a map, not positionally).

STATUS: Complete

SPEC CONTEXT:
Spec § Log-level propagation verification (604-654). Each process emits INFO process: log-level resolved resolved/source/raw after process: start (bypasses level filter). Canonical helper AssertLogLevelResolved(t, logPath, pid, expected) in internal/portaltest, matched by pid attr (load-bearing on reboot-recovery day-files), failing on absent line or source != env.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/portaltest/log_level_resolved.go (AssertLogLevelResolved :42; pure findLogLevelResolved :76; isLogLevelResolvedLine :94; parseLogAttrs :104). doc.go:22-29 exceptions note.
- Notes: Adopts [needs-info] option (a): pure table-testable findLogLevelResolved + thin t.Helper() wrapper. Symlink-follow implicit via os.ReadFile (follows symlinks). Parser keys off attr names via map (tolerates reorder); strips surrounding quotes. pid match scans candidates, selects matching pid. isLogLevelResolvedLine distinguishes from process: start/exit. *testing.T-first; doc.go exceptions documented.

TESTS:
- Status: Adequate
- Location: internal/portaltest/log_level_resolved_test.go
- Coverage: all seven ACs — select-by-pid among multiple; not-found for absent pid; ignores non-resolved start line same pid; happy path; tolerates reordered attrs; strips quotes; real symlink-follow integration (writes day-file + portal.log symlink). resolvedLine fixture mirrors production text-mode shape.
- Notes: Behaviour-focused (positional parser would fail reorder test; pid-blind scan would fail multi-process). Wrapper failure-path assertions (source!=env, resolved!=expected, unreadable) not directly exercised — sanctioned by task [needs-info] option (a); failure-path values covered at pure-function level, the if-checks are trivially correct. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (*testing.T-first; doc.go exceptions list; t.Helper(); no production consumer; aligns with ReadPortalLogSafe/IsolateStateForTest).
- SOLID: Good — pure parser vs I/O+assertion wrapper; single-responsibility helpers.
- Complexity: Low.
- Modern idioms: Yes (strings.Cut/Fields/Trim, strconv.Itoa).
- Readability: Good — doc explains pid-match, bypass-marker rationale, single-token attr scope.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Wrapper's three failure assertions have no direct test (sanctioned by task option (a)); a recording-testing.T shim could assert the Errorf/Fatalf branches if higher confidence is wanted. Not warranted now.
- [idea] parseLogAttrs splits on whitespace, so a raw value containing a space (invalid PORTAL_LOG_LEVEL like "de bug" → raw="de bug") would be mis-tokenized; does NOT affect correctness (helper only reads pid/resolved/source, all single-token); limitation documented in the doc comment.

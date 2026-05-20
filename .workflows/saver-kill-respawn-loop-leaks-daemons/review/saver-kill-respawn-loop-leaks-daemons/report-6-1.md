TASK: Migrate Five Inline daemonRunFunc Holders to the Existing withImmediateRun Helper (saver-kill-respawn-loop-leaks-daemons-6-1)

ACCEPTANCE CRITERIA:
- Five listed call sites only (state_daemon_run_test.go:747-753, :795-801, :823-829, :862-868; state_daemon_test.go:207-213).
- `go test ./cmd/...` passes with same case count.
- No behaviour change.

STATUS: Complete

SPEC CONTEXT: Pure mechanical refactor surfaced in Analysis Cycle 4. The withImmediateRun(t) **daemonDeps helper at cmd/state_daemon_test.go:33-45 was already used at ~14 sites; five callers re-inlined the 6-line body. Goal: collapse each inline block to one line. Helper's contract (return nil immediately + capture deps via returned holder) matches exactly those sites.

IMPLEMENTATION:
- Status: Implemented
- Locations now using `holder := withImmediateRun(t)`:
  - cmd/state_daemon_run_test.go:747 (HashMap-from-scrollback test)
  - cmd/state_daemon_run_test.go:789 (LoadsPrevIndexFromSessionsJSON)
  - cmd/state_daemon_run_test.go:811 (HandlesMissingSessionsJSONAsNilPrev)
  - cmd/state_daemon_run_test.go:844 (LogsWarningOnUndecodableSessionsJSON)
  - cmd/state_daemon_test.go:207 (PassesPreparedDepsToRunFunc)
- Notes: Post-migration line numbers shifted slightly (e.g. :795 → :789) as expected after collapsing 6 lines to 1. Subsequent assertions on *holder / (*holder).Field preserved verbatim.

TESTS:
- Status: Adequate — refactor only; no test changes required.
- Coverage: All five sites continue to exercise their original assertions. Test function counts: state_daemon_test.go = 21, state_daemon_run_test.go = 27.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); t.Cleanup LIFO ordering preserved via helper.
- SOLID: Single-purpose seam-installer.
- Complexity: Low. Net ~30 LOC removed.
- Modern idioms: Yes (helper-over-copy-paste).
- Readability: Improved — sites express intent (withImmediateRun) rather than mechanism.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Two additional one-liner inline holders match the "return nil immediately" shape but were excluded: cmd/state_test.go:179-181 and cmd/version_guard_test.go:149-151. They sit inside table-test t.Run loops. Out of scope for 6-1; flag for a future cleanup pass.

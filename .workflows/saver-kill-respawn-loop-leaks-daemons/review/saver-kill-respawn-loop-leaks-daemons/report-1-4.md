TASK: Defensive WriteVersionFile(currentVersion) on alive+absent branch before BootstrapPortalSaver (internal ID 1-4, tick-aac3a3)

ACCEPTANCE CRITERIA:
- On alive+absent branch, state.WriteVersionFile(stateDir, currentVersion) invoked before BootstrapPortalSaver.
- WriteVersionFile error propagates out and prevents BootstrapPortalSaver call.
- daemon.version exists synchronously after EnsurePortalSaverVersion returns on success.
- No other branch calls the defensive write.
- Unit tests assert call ordering and error propagation.
- go test ./internal/tmux/... green.

STATUS: Complete

SPEC CONTEXT: Spec §Change 1 (Defensive complement) — on the survived-daemon path bootstrap writes currentVersion to daemon.version before proceeding. Closes lock-loser lifecycle hole. Pathological older-binary-alive case is accepted (resolved on next legitimate recycle). Errors must propagate so step 4 can classify as SaverDownWarning.

IMPLEMENTATION:
- Status: Implemented
- internal/tmux/portal_saver.go:47-60 — portalSaverWriteVersionFile seam; wraps state.WriteVersionFile with package-level versionWriterLogger sink.
- internal/tmux/portal_saver.go:62-91 — versionWriterLogger var and SetVersionWriterLogger; nil-tolerant.
- internal/tmux/portal_saver.go:320-340 — EnsurePortalSaverVersion: alive+absent branch invokes portalSaverWriteVersionFile before BootstrapPortalSaver; error wrapped via fmt.Errorf("defensive daemon.version write failed: %w", err) and returned without calling BootstrapPortalSaver.
- internal/tmux/portal_saver.go:282-319 — godoc cross-references the matrix, documents defensive write rationale and pathological-older-binary edge case.
- Notes: Wrapper threads versionWriterLogger so the "daemon.version write:" breadcrumb (Task 1-2) fires from bootstrap-side too. fmt.Errorf %w wrap is correct for errors.Is on sentinel.

TESTS:
- Status: Adequate
- Coverage (all in internal/tmux/portal_saver_test.go):
  - line 2033 — alive+absent invokes defensive write exactly once with (dir, currentVersion) and precedes has-session.
  - line 2098 — defensive write error wraps sentinel, BootstrapPortalSaver not invoked.
  - line 2128 — alive+match: zero write calls.
  - line 2152 — alive+mismatch neither-dev: zero write calls; barrier fires once.
  - line 2181 — alive+stored=dev: zero write calls; barrier fires once.
  - line 2210 — alive+current=dev: zero write calls; barrier fires once.
  - line 2325 — not-alive + absent: zero write calls.
  - line 2244 — bootstrap wrapper emits "daemon.version write:" DEBUG breadcrumb.
  - line 2303 — SetVersionWriterLogger(nil) leaves prior logger in place.

CODE QUALITY:
- Project conventions: Followed. Seam-as-package-var idiom consistent with BootstrapAliveCheck.
- SOLID: Good. EnsurePortalSaverVersion stays a thin decision.
- Complexity: Low.
- Modern idioms: Yes. errors.Is on state.ErrVersionFileAbsent; fmt.Errorf with %w.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The bootstrap-wrapper breadcrumb test and nil-logger test live under the Task 1-4 header but pin Task 1-2's contract from the bootstrap call site. Functionally correct.
- [idea] portalSaverWriteVersionFile is a var holding a closure literal rather than a named function (required so versionWriterLogger is captured by reference). Either pattern is idiomatic.

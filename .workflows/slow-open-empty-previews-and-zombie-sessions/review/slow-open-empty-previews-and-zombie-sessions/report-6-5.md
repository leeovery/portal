TASK: 6-5 — Assert fresh-process AcquireDaemonLock refuses with ErrDaemonLockHeld post-bootstrap

STATUS: Issues Found (one documented deviation; one criterion not asserted; remainder met)

SPEC CONTEXT: Spec § Composite step 7 — fresh-process AcquireDaemonLock refuses with ErrDaemonLockHeld (Component C pre-check on live state). Spec's primary observable is `errors.Is` refusal.

IMPLEMENTATION:
- Status: Implemented with one documented deviation
- Location: `cmd/bootstrap/composition_e2e_fresh_acquire_integration_test.go` (213 lines)
- Drives production bootstrap slice (SweepOrphanDaemons + BootstrapPortalSaver) — byte-identical to 6-3
- 6s convergence budget shared via `freshAcquireConvergenceTimeout`; remaining budget at poll site enforces "within 6s of bootstrap entry"
- Re-reads current saver pane PID via `waitForSaverPanePID` (handles respawn race), asserts `daemon.pid == currentSaverPID`
- Waits for `state.IdentifyDaemon(currentSaverPID) == IdentifyIsPortalDaemon` before refusal call to pre-stage pre-check branch
- Refusal: `state.AcquireDaemonLock(h.StateDir)`; asserts `fd == nil` with defensive close; asserts `errors.Is(acquireErr, state.ErrDaemonLockHeld)`
- Post-refusal: re-reads daemon.pid (no destructive mutation); asserts `IsProcessAlive(currentSaverPID)` (pre-check never signalled holder)
- **Documented deviation**: calls `state.AcquireDaemonLock` from test goroutine rather than `go build`-staged subprocess. File header (17-29) cites two precedents in `upgrade_path_integration_test.go` arguing structural equivalence (fresh fd → fresh inode; `lockAcquireReadPIDFile`/`lockAcquireIdentifyDaemon` don't short-circuit on caller-process identity)
- **Coverage gap**: post-refusal `pgrep -fx` count not asserted

TESTS:
- Status: Adequate
- Plan enumerated 4 sub-tests; collapses into one covering refusal, no-destructive-coexistence, daemon.pid match
- Plan case (2) "pre-check exercised, not EWOULDBLOCK" not asserted (consistent with plan's edge-case best-effort designation)
- Integration-tag gated; rich diagnostics

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`
- SOLID: Good; single linear scenario
- Complexity: Low; ~140-line body, branching in fatal ladders
- Modern idioms: `errors.Is`, `time.Since`/`time.Now`
- Readability: Excellent; multi-line rationale comments; file header defends in-process deviation

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan AC "fresh-process subprocess via `go build` ... NOT in-test call" intentionally deviated from; defensible call but worth discussion thread — amend plan OR add subprocess form for belt-and-braces fd-isolation defence
- [idea] Plan AC "pgrep count remains 1 before AND after fresh-process invocation" not asserted post-refusal; two-line addition would close gap
- [idea] "Pre-check path exercised, not EWOULDBLOCK" sub-assertion not logged via `t.Logf`; single line would make coverage shape self-describing

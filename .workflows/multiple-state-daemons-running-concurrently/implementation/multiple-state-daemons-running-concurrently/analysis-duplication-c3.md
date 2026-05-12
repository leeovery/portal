AGENT: duplication
STATUS: clean
FINDINGS_COUNT: 0
SUMMARY: Both prior-cycle findings are resolved; no new actionable duplication detected in cycle 3.

Verification:
- Cycle-1 ProjectRoot + buildPortalBinary duplication resolved: internal/restoretest/build.go (untagged) hosts the helpers; integration-tagged restoretest.go delegates; default-lane singleton test calls restoretest.BuildPortalBinary directly.
- Cycle-2 inlined pre-recycle server PID capture resolved: portal_saver_integration_test.go:170 now calls captureTmuxServerPID(t, sock) symmetrically with the post-recycle use at line 242. Helper doc comment is now truthful.

Non-findings considered and rejected:
- Three `_ = c.KillSession(PortalSaverName)` calls in killSaverAndWaitForDaemon: one-liners on three mutually-exclusive code paths; extracting yields negative readability.
- fakeCommander / daemonFakeCommander pre-date this work unit; out of scope.
- 27 withDaemonLockFileReset call sites: intentional per-test isolation; already factored into a single helper.
- Six installBarrier* helpers: project's verbose-per-seam convention.
- waitForLiveDaemon vs waitForNewLiveDaemon: differ by one predicate; acceptable at two instances per Rule of Three.
- Seam-install helpers across packages: live in distinct test packages with type identity that a generic extraction would erase.

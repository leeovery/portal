AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 3
SUMMARY: One medium-severity duplication (inlined ProjectRoot + buildPortalBinary helpers vs restoretest counterparts) plus two low-severity findings flagged for completeness.

FINDINGS:

- FINDING: Inlined ProjectRoot + buildPortalBinary helpers duplicate restoretest counterparts verbatim
  SEVERITY: medium
  FILES: internal/tmux/portal_saver_integration_test.go:311-361, internal/restoretest/restoretest.go:45-115
  DESCRIPTION: `projectRootForSingletonTest` (16 lines) and `buildPortalBinaryForSingletonTest` (15 lines) reproduce `restoretest.ProjectRoot` and the body of `restoretest.BuildPortalBinaryDir` (backed by the private `buildPortalBinaryInto`) almost line-for-line — same walk-up-to-`go.mod` loop, same `go build -o <bin> .` invocation against the project root, same wrapped error messages. The executor's comment is explicit about why: `internal/restoretest/restoretest.go` carries `//go:build integration` and this new test runs under the default lane. The justification is sound (forcing this load-bearing acceptance test behind the integration tag would defeat its purpose — "any developer or CI without the integration tag can catch a regression"). But the duplication is real, the divergence vector is structural (anyone touching the build flags or project-root semantics for one site is unlikely to remember the other), and the pattern will repeat the next time a default-lane integration test needs a real portal binary. The clean resolution is to extract `ProjectRoot` + the private `buildPortalBinaryInto` helper into an untagged file in `internal/restoretest/` (split: leave the `t.TempDir`/`Stable` wrappers + `PrependPATH` + `DriveSignalHydrate` under `//go:build integration`, but un-tag the pure project-root walker and the build helper since neither depends on tmux fixtures or test-lifetime semantics).
  RECOMMENDATION: Split `internal/restoretest/restoretest.go` into a tagged file (BuildPortalBinaryDir, BuildPortalBinaryStable, PrependPATH, DriveSignalHydrate) and an untagged file (ProjectRoot + buildPortalBinaryInto + a returns-error `BuildPortalBinary(dir string) error` wrapper). Then delete `projectRootForSingletonTest` and `buildPortalBinaryForSingletonTest` in `portal_saver_integration_test.go` and call the shared helpers directly. Net change: ~30 lines deleted, one source of truth for "find the repo root, go build, return errors not fatals."

- FINDING: Six seam-install helpers in portal_saver_test.go are mechanical near-duplicates of each other
  SEVERITY: low
  FILES: internal/tmux/portal_saver_test.go:934-976, 1292-1298
  DESCRIPTION: `installBarrierReadPID`, `installBarrierIsAlive`, `installBarrierPollInterval`, `installBarrierTimeout`, `installBarrierLogger`, and `installKillSaverFn` are six near-identical four-to-six-line functions. The verbose-per-seam pattern is the project's accepted convention — `withLockAcquireFake`, `withAcquireDaemonLockFake`, `installStubVersionChecker`, `installStateCleanupDeps`, `installStaleHooks` all follow the same one-helper-per-seam style. Collapsing only this work unit's six would introduce a styling fork.
  RECOMMENDATION: Keep as-is. The verbose-per-seam style is consistent with project convention.

- FINDING: waitForLiveDaemon and waitForNewLiveDaemon share ~90% of their poll-loop structure
  SEVERITY: low
  FILES: internal/tmux/portal_saver_integration_test.go:229-264
  DESCRIPTION: Both helpers implement the same deadline-bound poll loop. The only structural difference is the success predicate. Net win from extraction is small (~10 lines); local duplication at Rule-of-Three boundary.
  RECOMMENDATION: Optional cleanup — acceptable to leave as-is at two instances.

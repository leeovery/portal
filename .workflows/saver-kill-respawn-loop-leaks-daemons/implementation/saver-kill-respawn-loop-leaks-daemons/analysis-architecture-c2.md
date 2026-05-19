STATUS: findings
FINDINGS_COUNT: 2

AGENT: architecture

FINDINGS:

- FINDING: Stale references to deleted `portalSaverVersionMismatch` in integration-test doc comments
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:105, /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:170
  DESCRIPTION: Cycle 1 collapsed `portalSaverVersionMismatch` into `shouldKillSaverOnVersionDecision` and the dead helper was removed from `portal_saver.go`. Two narrative doc comments in `portal_saver_integration_test.go` still cite "the production portalSaverVersionMismatch comparison" as the production code path the test exercises. The symbol no longer exists; the test in fact exercises `shouldKillSaverOnVersionDecision`. The drift is a future-reader trap.
  RECOMMENDATION: Replace both occurrences with "the production `shouldKillSaverOnVersionDecision` predicate" (or simply "the production version-mismatch comparison in `EnsurePortalSaverVersion`"). One-line edits each.

- FINDING: `restoretest` package scope has drifted beyond restore-specific helpers
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/restoretest/build.go:1-118, /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:60, /Users/leeovery/Code/portal/cmd/state_daemon_integration_test.go:49
  DESCRIPTION: `restoretest` was introduced as "test-only helpers (shared restore drivers, real-tmux socket fixtures)". Cycle 1 added `StagePortalBinary`, `BuildPortalBinary`, and `ProjectRoot` here — generic "compile-portal-and-PATH-prepend" plumbing now consumed by daemon, saver, and TUI integration tests with no semantic tie to restore. The package name misadvertises its contents. A reader looking for "where is the portal-binary build helper for integration tests?" will not navigate to `restoretest`.
  RECOMMENDATION: Either rename `restoretest` to something domain-neutral (e.g. `portaltest`) reflecting actual scope, or extract the build-and-stage helpers into a sibling test-only package (e.g. `internal/portalbintest`). Lower urgency than cycle-1 findings; not a ship-blocker.

SUMMARY: Cycle 1's findings correctly resolved. Two residual minor issues remain: stale doc-comment references to the deleted helper, and the `restoretest` package now hosting non-restore test plumbing imported across multiple packages. Neither blocks correctness.

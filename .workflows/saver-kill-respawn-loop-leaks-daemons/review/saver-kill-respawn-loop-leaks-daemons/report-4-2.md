TASK: Decide and act on restoretest package scope drift (saver-kill-respawn-loop-leaks-daemons-4-2)

ACCEPTANCE CRITERIA:
- Imports updated at all consumer sites (daemon, saver, TUI integration tests)
- CLAUDE.md package table reflects new scope
- restoretest (if kept) contains only restore-domain helpers

STATUS: Complete

SPEC CONTEXT: Analysis cycle 2 (analysis-architecture-c2.md:14-18) flagged that restoretest had absorbed general-purpose `go build` / PATH-stage plumbing (BuildPortalBinary, StagePortalBinary, ProjectRoot) with no semantic tie to restore — consumed by daemon and saver integration tests too. Recommendation: rename to domain-neutral OR extract build/stage helpers into a sibling package. Implementer chose extraction.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - New package: internal/portalbintest/build.go (BuildPortalBinary, StagePortalBinary, ProjectRoot, buildPortalBinaryInto)
  - Consumer sites updated:
    - internal/tmux/portal_saver_integration_test.go:60,140,276,409 — imports portalbintest, calls StagePortalBinary
    - cmd/state_daemon_integration_test.go:29,49,181 — imports portalbintest, calls StagePortalBinary
    - internal/tui/pagepreview_surface_audit_test.go:299 — allow-list updated to include portalbintest
  - restoretest retained with restore-domain helpers: doc.go (package doc rewritten), restoretest.go (integration-tagged: DriveSignalHydrate, DriveSignalHydrateBinary, WaitForSkeletonMarkersCleared, PrependPATH, SortedKeySet, BuildPortalBinaryDir/Stable wrappers that delegate to portalbintest), sessions_json.go, logger.go, waitfor_file_exists.go
  - CLAUDE.md:59 — package table entry rewritten to describe both restoretest and portalbintest with the new split scope
- Notes: Integration-tagged restoretest wrappers (BuildPortalBinaryDir/Stable) delegate to portalbintest.BuildPortalBinary, preserving the t.TempDir-vs-sync.Once.MkdirTemp lifetime contract while keeping the `go build` invocation in one place. cmd/reattach_integration_test.go correctly continues to import restoretest (for OpenTestLogger, SeedSessionsJSON, BuildPortalBinaryStable) since those are restore-domain helpers.

TESTS:
- Status: Adequate
- Coverage: internal/portalbintest/build_test.go — TestProjectRoot verifies repo-root walk + module-path sanity; TestStagePortalBinary verifies binary built, PATH prepended with binDir, exec.LookPath resolves to the staged binary not a system shadow. EvalSymlinks-on-both-sides guard handles macOS /var → /private/var.
- Notes: Pre-existing restoretest tests (sessions_json_test.go, logger_test.go, waitfor_file_exists_test.go) remain intact, confirming retained helpers still pass after the split.

CODE QUALITY:
- Project conventions: Followed. Package doc comments explain scope, exported symbols, and production-import prohibition. portalbintest_test black-box test package matches codebase pattern.
- SOLID: Good. Single responsibility — portalbintest owns build/stage plumbing; restoretest owns restore-domain scaffolding. Internal buildPortalBinaryInto is single source of `go build` truth.
- Complexity: Low.
- Modern idioms: Yes. t.Setenv, t.TempDir, fmt.Errorf %w wrapping, t.Skipf for environmental skip semantics.
- Readability: Good. Doc comments cite call-site contracts and cross-package dependencies.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] restoretest/doc.go scope description and the cross-references between restoretest.go wrappers and portalbintest are accurate today but have no automated sync check — future helper additions/removals risk drift.
- [idea] portalbintest currently only houses build/stage helpers; worth recording the convention that "test plumbing that is portal-binary-adjacent but not domain-bound" lives here.

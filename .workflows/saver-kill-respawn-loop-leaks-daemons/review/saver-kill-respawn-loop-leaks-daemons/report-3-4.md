TASK: Extract StagePortalBinary helper to eliminate repeated build+PATH preamble across integration tests

ACCEPTANCE CRITERIA:
- PATH composition (binDir prepended) preserved
- skip-on-build-failure preserved at four call sites
- skip-on-LookPath-failure preserved at four call sites

STATUS: Complete

SPEC CONTEXT: Phase 3 duplication cleanup. Four integration tests duplicated a BuildPortalBinary + t.Setenv("PATH", ...) + exec.LookPath("portal") preamble with t.Skipf on both build and lookpath failure. Pure refactor.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - internal/portalbintest/build.go:85-96 — StagePortalBinary(t *testing.T) string
  - internal/portalbintest/build.go:66-68 — BuildPortalBinary thin wrapper around shared buildPortalBinaryInto
  - internal/portalbintest/build.go:98-114 — buildPortalBinaryInto (unexported shared core)
  - Call sites migrated: internal/tmux/portal_saver_integration_test.go:140, :276, :409; cmd/state_daemon_integration_test.go:181
- Notes:
  - PATH composition `binDir + os.PathListSeparator + os.Getenv("PATH")` (build.go:91) preserves prepend order — system-installed `portal` cannot shadow.
  - Skip-on-build-failure preserved (build.go:88-90).
  - Skip-on-LookPath-failure preserved (build.go:92-94).
  - t.Helper() correctly applied (build.go:86) so failures attribute to caller.
  - internal/restoretest/doc.go:34-37 updated to redirect readers to the new location; restoretest.BuildPortalBinaryDir/Stable cleanly delegate to portalbintest.BuildPortalBinary.
  - At cmd/state_daemon_integration_test.go:181-185 the caller does an additional exec.LookPath("portal") after the helper to obtain an absolute path used for direct binary invocation. Intentional — StagePortalBinary returns only binDir.

TESTS:
- Status: Adequate
- Coverage: internal/portalbintest/build_test.go:51-98 (TestStagePortalBinary) verifies returned binDir non-empty, binary exists at binDir/portal, PATH starts with binDir + PathListSeparator, PATH retains prior PATH as suffix, exec.LookPath("portal") resolves under staged binDir (EvalSymlinks handles macOS /var → /private/var). TestProjectRoot covers ProjectRoot.
- Notes: Skip-on-build-failure and skip-on-LookPath-failure paths not directly exercised, but they are trivial t.Skipf on returned errors.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (CLAUDE.md).
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Uses t.TempDir, t.Setenv, t.Helper, os.PathListSeparator (cross-platform).
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] cmd/state_daemon_integration_test.go:182-185 repeats exec.LookPath("portal") + t.Skipf after StagePortalBinary. Consider a StagePortalBinaryWithPath(t) (binDir, absPath string) variant so callers needing the absolute binary path skip the redundant lookup.

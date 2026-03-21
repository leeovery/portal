TASK: Extract defaultTestTUIConfig helper in open_test.go

ACCEPTANCE CRITERIA:
- No tuiConfig struct literal with more than 2 fields appears in the test file (base fields come from helper; only override fields are set inline)
- All existing tests in TestBuildTUIModel and TestBuildTUIModel_ServerStarted pass with identical behavior
- The helper function is the single source of truth for the default test config

STATUS: Complete

SPEC CONTEXT: This task is a chore/refactor from Phase 6 (analysis cleanup). The auto-start-tmux-server feature added a `serverStarted` field to tuiConfig, which was being copy-pasted across 11 test sites. The refactor extracts a helper to reduce duplication and make future field additions easier.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open_test.go:746-758
- Notes: The `defaultTestTUIConfig()` helper returns a tuiConfig with 7 base fields set (lister, killer, renamer, projectStore, sessionCreator, dirLister, cwd). The remaining fields (projectEditor, aliasEditor, insideTmux, currentSession, serverStarted) default to zero values and are overridden per-test via field assignment. This is the correct approach -- the zero values represent the common "happy path" defaults.

TESTS:
- Status: Adequate
- Coverage: All 11 call sites across TestBuildTUIModel (7 subtests) and TestBuildTUIModel_ServerStarted (4 subtests) use the helper. Each test overrides only the fields relevant to its scenario (0-3 fields). No tuiConfig struct literal exists anywhere in the test file except inside the helper itself (line 749). This is a pure refactor -- no behavioral change -- so existing test assertions serve as the verification that behavior is preserved.
- Notes: The task specifies `go test ./cmd/...` should pass with zero failures. This is a refactor-only change with no new test code needed beyond the existing tests continuing to pass.

CODE QUALITY:
- Project conventions: Followed. Uses Go idioms for test helpers (function returning value, descriptive name, doc comment). Aligns with golang-pro skill guidance on test helpers.
- SOLID principles: Good. Single source of truth for default config (DRY). The helper has a single responsibility -- providing the base test config.
- Complexity: Low. Simple function returning a struct literal.
- Modern idioms: Yes. Uses struct field override pattern (cfg := helper(); cfg.field = value) which is idiomatic Go for test customization.
- Readability: Good. The helper name `defaultTestTUIConfig` is self-documenting. The doc comment (lines 746-747) explains its purpose and usage pattern. Each test clearly shows only the fields that differ from the default, making the test intent immediately visible.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None. Clean refactor that meets all acceptance criteria.

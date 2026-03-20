---
topic: auto-start-tmux-server
cycle: 3
total_proposed: 1
---
# Analysis Tasks: auto-start-tmux-server (Cycle 3)

## Task 1: Extract defaultTestTUIConfig helper in open_test.go
status: pending
severity: medium
sources: duplication

**Problem**: The tuiConfig struct literal (7 fields: lister, killer, renamer, projectStore, sessionCreator, dirLister, cwd) is copy-pasted 11 times across TestBuildTUIModel and TestBuildTUIModel_ServerStarted. The feature added 4 new instances (TestBuildTUIModel_ServerStarted subtests) on top of 7 pre-existing ones. This is ~77 lines of pure boilerplate repetition.

**Solution**: Extract a `defaultTestTUIConfig() tuiConfig` helper function that returns the base config with all stubs wired. Each test case modifies the returned struct for its specific overrides.

**Outcome**: Each test case uses 1 line to get the base config instead of 7 lines of repeated struct fields. Adding new tests or new fields to tuiConfig requires updating only the helper, not every test case.

**Do**:
1. In `cmd/open_test.go`, add a helper function near the top of the test helpers section:
   ```go
   func defaultTestTUIConfig() tuiConfig {
       return tuiConfig{
           lister:         &mockSessionLister{},
           killer:         &stubSessionKiller{},
           renamer:        &stubSessionRenamer{},
           projectStore:   &stubProjectStore{},
           sessionCreator: &stubTUISessionCreator{},
           dirLister:      &stubDirLister{},
           cwd:            "/home/user",
       }
   }
   ```
2. In each subtest of `TestBuildTUIModel` (lines ~748, 778, 809, 830, 854, 880, 901), replace the 7-line struct literal with `cfg := defaultTestTUIConfig()` followed by any per-test overrides (e.g., `cfg.insideTmux = true`).
3. In each subtest of `TestBuildTUIModel_ServerStarted` (lines ~933, 955, 977, 995), do the same. For subtests that set `serverStarted`, `insideTmux`, or `currentSession`, add those as overrides after the helper call.
4. Run `go test ./cmd/...` to confirm all tests pass unchanged.

**Acceptance Criteria**:
- No tuiConfig struct literal with more than 2 fields appears in the test file (base fields come from helper; only override fields are set inline)
- All existing tests in TestBuildTUIModel and TestBuildTUIModel_ServerStarted pass with identical behavior
- The helper function is the single source of truth for the default test config

**Tests**:
- `go test ./cmd/...` passes with zero failures — this is a pure refactor with no behavior change

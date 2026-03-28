TASK: Fix configFilePath to use XDG-compliant base directory

ACCEPTANCE CRITERIA:
- [x] configFilePath("", "projects.json") returns $HOME/.config/portal/projects.json when no env vars set
- [x] configFilePath("", "projects.json") returns $XDG_CONFIG_HOME/portal/projects.json when XDG_CONFIG_HOME set
- [x] Per-file env var overrides still take precedence
- [x] configFilePath returns error when os.UserHomeDir() fails and no env vars set
- [x] All existing tests pass with concrete path assertions
- [x] go build succeeds

STATUS: Complete

SPEC CONTEXT: On macOS, os.UserConfigDir() returns ~/Library/Application Support, producing wrong config paths. The fix replaces it with XDG-compliant resolution: check XDG_CONFIG_HOME first, fall back to ~/.config. Per-file env var overrides are unchanged. Tests must assert concrete paths, not compare against os.UserConfigDir().

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/config.go:42-59 (configFilePath), cmd/config.go:64-70 (xdgConfigBase)
- Notes: os.UserConfigDir() fully removed. xdgConfigBase is cleanly separated as a helper that takes homeDir as a parameter, avoiding redundant os.UserHomeDir() calls. Resolution order is correct: (1) per-file env var, (2) XDG_CONFIG_HOME, (3) $HOME/.config. Error wrapping on line 49 uses %w for proper error chain propagation.

TESTS:
- Status: Adequate
- Coverage:
  - "returns ~/.config/portal/<file> when no env vars are set" -- line 11: asserts concrete path using os.UserHomeDir() + ".config/portal/projects.json"
  - "returns env var value when per-file env var is set" -- line 30: asserts literal custom path
  - "respects XDG_CONFIG_HOME when set" -- line 44: asserts /tmp/xdg-config/portal/projects.json
  - "treats empty XDG_CONFIG_HOME as unset" -- line 58: asserts fallback to ~/.config
  - "per-file env var takes precedence over XDG_CONFIG_HOME" -- line 77: both set, env var wins
  - "XDG_CONFIG_HOME with trailing slash is normalized" -- line 92: trailing slash stripped by filepath.Join
  - os.UserHomeDir() failure error path: not tested (acknowledged in plan as difficult to mock; code path is trivially correct on line 48-49)
- Notes: All six required test cases present. No os.UserConfigDir() references remain in tests. Tests use t.Setenv for env var isolation. No over-testing detected -- each test covers a distinct behavior.

CODE QUALITY:
- Project conventions: Followed. Uses standard Go conventions per CLAUDE.md. No t.Parallel() (correctly omitted per project rules). Tests use subtests with descriptive names.
- SOLID principles: Good. xdgConfigBase has single responsibility (resolve XDG base dir). configFilePath orchestrates the full resolution. Clean separation of concerns.
- Complexity: Low. configFilePath has 3 linear code paths (env var early return, home dir error, normal resolution). xdgConfigBase has 2 paths (XDG set vs fallback). No nesting beyond one level.
- Modern idioms: Yes. Uses fmt.Errorf with %w for error wrapping, filepath.Join for path construction, os.Getenv for env var access.
- Readability: Good. Function and variable names are self-documenting. Doc comments on both functions explain the resolution strategy. The flow is top-to-bottom with early returns.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The os.UserHomeDir() failure error path (line 48-49) lacks a unit test. This is acknowledged in the task plan as infeasible without mocking os.UserHomeDir. The code is trivially correct (two-line if-err-return pattern). Not blocking.
- The default fallback test (line 11) relies on the real os.UserHomeDir() to compute the expected path. In theory, if HOME were somehow different from what configFilePath sees, the test could give a false positive. In practice, both call os.UserHomeDir() in the same process, so this is not a real concern.

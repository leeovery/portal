AGENT: duplication
FINDINGS:
- FINDING: SessionCreator.CreateFromDir and QuickStart.Run are near-duplicate pipelines
  SEVERITY: high
  FILES: internal/session/create.go:73-101, internal/session/quickstart.go:51-83
  DESCRIPTION: Both methods implement the same 5-step pipeline independently: (1) resolve git root, (2) derive project name from filepath.Base, (3) generate session name via GenerateSessionName, (4) upsert project in store, (5) build shell command via BuildShellCommand. The only divergence is the final action -- CreateFromDir calls tmux.NewSession while Run builds exec args. Both structs also hold the same four dependencies (git, store, checker/tmux, gen) plus shell, and both resolve shell from ShellFromEnv() at construction time. This is approximately 30 lines of duplicated orchestration logic that could drift independently.
  RECOMMENDATION: Extract the shared pipeline (steps 1-4 + shell command building) into a single internal function or method that returns a resolved struct (dir, projectName, sessionName, shellCmd). Both CreateFromDir and Run would call this shared function and only diverge on the final tmux operation. This eliminates the risk of the two pipelines diverging when session creation logic changes.

- FINDING: fuzzyMatch function duplicated across packages
  SEVERITY: medium
  FILES: internal/tui/model.go:550-558, internal/ui/projectpicker.go:135-143
  DESCRIPTION: The fuzzyMatch function (subsequence matching) is independently implemented with identical logic in two packages. The tui package uses it for session filtering; the ui package uses it for both project picker filtering and file browser filtering. These were written by separate task executors and could diverge over time (e.g., if one adds case normalization or scoring).
  RECOMMENDATION: Extract fuzzyMatch into a shared utility, for example internal/fuzzy/match.go or a function in an existing shared package. Both tui and ui packages would import from the single source.

- FINDING: Config file path resolution pattern duplicated
  SEVERITY: medium
  FILES: cmd/alias.go:103-114, cmd/clean.go:52-63
  DESCRIPTION: aliasFilePath() and projectsFilePath() have identical structure: check an env var override, fall back to os.UserConfigDir() + filepath.Join("portal", filename). The error wrapping message is also identical. Both were written to support the same "test override via env var, default to XDG config" pattern. A third file (cmd/open.go:247-251) also duplicates the os.UserConfigDir() + filepath.Join("portal", ...) pattern inline.
  RECOMMENDATION: Extract a configFilePath(envVar, filename string) helper in the cmd package (or internal/config) that encapsulates the env-var-override + UserConfigDir fallback. All three call sites would use this single function.

- FINDING: tmux.NewClient construction repeated 6 times
  SEVERITY: low
  FILES: cmd/open.go:69, cmd/open.go:245, cmd/open.go:285, cmd/list.go:101, cmd/attach.go:50, cmd/kill.go:50
  DESCRIPTION: The expression tmux.NewClient(&tmux.RealCommander{}) appears 6 times across 4 cmd files. While each is a single line, they all construct the same production client. If the construction ever changes (e.g., adding options), all 6 sites need updating.
  RECOMMENDATION: Extract a newTmuxClient() helper in the cmd package that returns tmux.NewClient(&tmux.RealCommander{}). This is low priority since it is a single-line expression, but consolidation would reduce coupling to the constructor signature.

- FINDING: Identical test mocks duplicated across test files
  SEVERITY: low
  FILES: cmd/open_test.go:16-41, internal/resolver/query_test.go:13-39, cmd/attach_test.go:21-26, cmd/kill_test.go:9-17
  DESCRIPTION: Three sets of test mocks are independently implemented: (a) AliasLookup mock appears in both cmd/open_test.go (testAliasLookup) and internal/resolver/query_test.go (mockAliasLookup) with identical logic. (b) ZoxideQuerier mock appears in both files. (c) DirValidator mock appears in both files. (d) mockSessionValidator is identical in cmd/attach_test.go and cmd/kill_test.go. These are small structs (3-5 lines each) but the pattern is repeated across 4 files.
  RECOMMENDATION: For the cmd package mocks (mockSessionValidator), they are already in the same package and could be extracted to a shared test helper file (e.g., cmd/testhelpers_test.go). The cross-package mocks (cmd vs internal/resolver) cannot easily be shared due to Go test package boundaries, so this is lower priority.

SUMMARY: The most significant duplication is the near-identical session creation pipeline in SessionCreator.CreateFromDir and QuickStart.Run (~30 lines of shared orchestration logic). The fuzzyMatch function and config path resolution pattern are also duplicated across independently-authored packages. The tmux client construction and test mock duplication are lower-impact but worth consolidating for maintainability.

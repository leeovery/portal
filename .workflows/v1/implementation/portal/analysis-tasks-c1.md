---
topic: portal
cycle: 1
total_proposed: 6
---
# Analysis Tasks: Portal (Cycle 1)

## Task 1: Wire all dependencies into openTUI and adopt functional options for tui.Model
status: approved
severity: high
sources: standards, architecture

**Problem**: The `openTUI` function in `cmd/open.go:284-318` creates the TUI model via `tui.NewWithKiller(client, client)`, which only sets sessionLister and sessionKiller. The TUI model supports session renaming (R key), project picker ([n] new in project...), and file browser navigation -- but none of these work in the real binary because sessionRenamer, projectStore, sessionCreator, and dirLister are nil. The spec defines these as core TUI features. Additionally, tui.Model has 5 telescoping constructors (New, NewWithKiller, NewWithRenamer, NewWithDeps, NewWithAllDeps) that grow combinatorially with each new dependency.

**Solution**: (1) Replace the 5 constructors with a single `New(lister SessionLister, opts ...Option)` constructor using the functional options pattern (e.g., `WithKiller(k)`, `WithRenamer(r)`, `WithProjectStore(s)`, `WithSessionCreator(c)`, `WithDirLister(d, startPath)`). (2) Update `openTUI` to pass all required dependencies: renamer (tmux client), project store (loaded from projects.json), session creator (session.NewSessionCreator), dir lister, and start path (cwd).

**Outcome**: The production binary's TUI supports all specified features: session rename, project picker, new session creation, and file browser navigation. The tui.Model API is extensible without constructor proliferation.

**Do**:
1. In `internal/tui/model.go`, define an `Option` type (`type Option func(*Model)`) and create `WithKiller`, `WithRenamer`, `WithProjectStore`, `WithSessionCreator`, `WithDirLister` option functions.
2. Replace all 5 constructors with a single `New(lister SessionLister, opts ...Option)` that applies options.
3. Keep `NewModelWithSessions` for test use (it serves a different purpose).
4. Update all callers of the old constructors across tests and production code to use the new `New` + options pattern.
5. In `cmd/open.go` `openTUI`, construct the model with all dependencies:
   - `tui.WithKiller(client)` (already wired)
   - `tui.WithRenamer(client)` (tmux client implements RenameSession)
   - `tui.WithProjectStore(store)` where store is loaded from projects.json (use `projectsFilePath()` or equivalent)
   - `tui.WithSessionCreator(session.NewSessionCreator(gitResolver, store, client, gen))` with proper dependencies
   - `tui.WithDirLister(...)` with cwd as start path
6. Verify the TUI correctly creates sessions, renames, and browses files.

**Acceptance Criteria**:
- The production binary's TUI can create new sessions from the project picker
- The production binary's TUI can rename sessions via R key
- The production binary's TUI can navigate the file browser
- tui.Model has a single public constructor `New` plus functional options
- All existing tests pass

**Tests**:
- Existing TUI tests updated to use new constructor pattern
- Integration test or manual verification that `portal open` TUI can create a new session from a project

## Task 2: Extract shared session-creation pipeline from SessionCreator and QuickStart
status: approved
severity: high
sources: duplication, architecture

**Problem**: `SessionCreator.CreateFromDir` (`internal/session/create.go:73-101`) and `QuickStart.Run` (`internal/session/quickstart.go:51-83`) implement the same 5-step pipeline independently: (1) resolve git root, (2) derive project name from filepath.Base, (3) generate session name, (4) upsert project in store, (5) build shell command. The only divergence is the final action -- CreateFromDir calls tmux.NewSession while Run builds exec args. Both structs hold identical dependency fields. This is ~30 lines of duplicated orchestration logic that can drift independently.

**Solution**: Extract the shared pipeline (steps 1-5) into a single internal function or method that returns an intermediate result struct containing resolvedDir, projectName, sessionName, and shellCmd. Both SessionCreator and QuickStart consume this result and only perform their respective final step.

**Outcome**: A single source of truth for the session preparation pipeline. Changes to git root resolution, name generation, or project upsert logic are made in one place.

**Do**:
1. Define a `preparedSession` struct in `internal/session/` with fields: `ResolvedDir`, `ProjectName`, `SessionName`, `ShellCmd`.
2. Extract a `prepareSession(path string, command []string)` method (or standalone function) that takes the shared dependencies (git, store, checker, gen, shell) and returns `*preparedSession`.
3. Refactor `SessionCreator.CreateFromDir` to call `prepareSession` then pass the result to `tmux.NewSession`.
4. Refactor `QuickStart.Run` to call `prepareSession` then build exec args from the result.
5. Consider whether SessionCreator and QuickStart can share a common embedded struct for their dependencies, or whether the function accepts explicit parameters.

**Acceptance Criteria**:
- The git-root-resolution, name-generation, project-upsert, and shell-command-building logic exists in exactly one place
- SessionCreator.CreateFromDir and QuickStart.Run both delegate to the shared pipeline
- All existing tests pass without behavioral changes

**Tests**:
- Existing tests for SessionCreator and QuickStart continue to pass
- Unit test for the extracted prepareSession function covering git resolution, name generation, project upsert, and shell command building

## Task 3: Fix GoReleaser config to use brews instead of homebrew_casks
status: approved
severity: medium
sources: standards, architecture

**Problem**: `.goreleaser.yaml:34-44` uses `homebrew_casks` as the top-level key for Homebrew distribution. Portal is a CLI binary, not a macOS .app -- `homebrew_casks` generates Homebrew Cask definitions for GUI applications. GoReleaser v2 uses `brews` for CLI tool formulae. This means the release pipeline will not produce a valid Homebrew formula in the `leeovery/homebrew-tools` tap, breaking `brew install portal`.

**Solution**: Replace `homebrew_casks` with `brews` in `.goreleaser.yaml` and adjust the inner structure to match the GoReleaser v2 `brews` schema.

**Outcome**: `goreleaser release` generates a proper Homebrew formula that installs Portal as a CLI binary via `brew tap leeovery/tools && brew install portal`.

**Do**:
1. In `.goreleaser.yaml`, change `homebrew_casks:` to `brews:`.
2. Adjust the inner structure to match GoReleaser v2 brews schema. The `repository`, `name`, `homepage`, `description`, `license` fields stay the same. Change `dependencies` from `- formula: tmux` to `- name: tmux` (GoReleaser v2 brews dependency format).
3. Verify the config is valid by running `goreleaser check` if available.

**Acceptance Criteria**:
- `.goreleaser.yaml` uses `brews` key, not `homebrew_casks`
- Dependencies use the correct `name:` format for brews
- `goreleaser check` passes (if available)

**Tests**:
- Run `goreleaser check` to validate config syntax

## Task 4: Extract fuzzyMatch into shared internal package
status: approved
severity: medium
sources: duplication, architecture

**Problem**: The `fuzzyMatch` function (subsequence matching) is independently implemented with identical logic in `internal/tui/model.go:550-558` and `internal/ui/projectpicker.go:135-143`. A near-identical pattern also exists in `internal/ui/browser.go` for directory filtering. If fuzzy match behavior is refined (e.g., case folding, scoring), multiple files must be updated independently.

**Solution**: Extract `fuzzyMatch` into a shared package (`internal/fuzzy/match.go` or similar) and import it from all locations.

**Outcome**: A single fuzzyMatch implementation used by the session list, project picker, and file browser.

**Do**:
1. Create `internal/fuzzy/match.go` with an exported `Match(text, pattern string) bool` function containing the existing subsequence logic.
2. Add tests in `internal/fuzzy/match_test.go` covering the existing test cases from both packages.
3. Replace `fuzzyMatch` in `internal/tui/model.go` with `fuzzy.Match`.
4. Replace `fuzzyMatch` in `internal/ui/projectpicker.go` with `fuzzy.Match`.
5. Update `internal/ui/browser.go` to use `fuzzy.Match` if it has inline matching logic.

**Acceptance Criteria**:
- No `fuzzyMatch` function definitions remain in `internal/tui/` or `internal/ui/`
- All fuzzy matching imports from `internal/fuzzy`
- All existing tests pass

**Tests**:
- Unit tests in `internal/fuzzy/match_test.go` for empty input, exact match, subsequence match, no match, case sensitivity

## Task 5: Extract config file path helper to eliminate duplication
status: approved
severity: medium
sources: duplication

**Problem**: `aliasFilePath()` in `cmd/alias.go:103-114` and `projectsFilePath()` in `cmd/clean.go:52-63` have identical structure: check an env var override, fall back to `os.UserConfigDir()` + `filepath.Join("portal", filename)`. A third site in `cmd/open.go:247-251` also duplicates the `UserConfigDir + filepath.Join("portal", ...)` pattern inline. Three independent implementations of the same path resolution logic.

**Solution**: Extract a `configFilePath(envVar, filename string) (string, error)` helper that encapsulates the env-var-override + UserConfigDir fallback. All three call sites use this single function.

**Outcome**: Config path resolution logic exists in one place. Adding new config files (or changing the base path) requires a single edit.

**Do**:
1. Create a helper function `configFilePath(envVar, filename string) (string, error)` in the `cmd` package (e.g., in a new `cmd/config.go` or existing shared file).
2. The function checks `os.Getenv(envVar)`, returns it if non-empty, otherwise calls `os.UserConfigDir()` and returns `filepath.Join(configDir, "portal", filename)`.
3. Rewrite `aliasFilePath()` to call `configFilePath("PORTAL_ALIASES_FILE", "aliases")`.
4. Rewrite `projectsFilePath()` to call `configFilePath("PORTAL_PROJECTS_FILE", "projects.json")`.
5. Update the inline path in `cmd/open.go:247-251` to use `configFilePath` (or reuse `projectsFilePath`).

**Acceptance Criteria**:
- A single `configFilePath` helper exists
- `aliasFilePath` and `projectsFilePath` delegate to it
- The inline path resolution in `cmd/open.go` is replaced
- All existing tests pass

**Tests**:
- Unit test for `configFilePath` with env var set, env var unset

## Task 6: Pass parsedCommand as parameter instead of package-level variable
status: approved
severity: medium
sources: standards, architecture

**Problem**: `parsedCommand` is declared as a package-level `var` in `cmd/open.go:77`. It is set during `openCmd.RunE` and consumed by `openPath` and `openTUI`. This is shared mutable state that creates hidden coupling -- callers of `openPath` and `openTUI` must know to set the global first. It prevents test isolation and parallel test execution.

**Solution**: Pass the command slice explicitly through function parameters. Remove the package-level variable.

**Outcome**: No package-level mutable state for command passing. Functions are self-contained with explicit parameters.

**Do**:
1. Change `openPath(resolvedPath string)` to `openPath(resolvedPath string, command []string)`.
2. Change `openTUI(initialFilter string)` to `openTUI(initialFilter string, command []string)`.
3. Update the call sites in `openCmd.RunE` to pass `command` directly from `parseCommandArgs` return value.
4. Remove the `var parsedCommand []string` package-level declaration.
5. Update any test code that references `parsedCommand`.

**Acceptance Criteria**:
- No `parsedCommand` package-level variable exists
- `openPath` and `openTUI` accept command as a parameter
- All existing tests pass

**Tests**:
- Existing open command tests continue to pass
- Verify no package-level `parsedCommand` remains via grep

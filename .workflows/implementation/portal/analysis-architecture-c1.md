AGENT: architecture
FINDINGS:
- FINDING: SessionCreator and QuickStart duplicate the session-creation pipeline
  SEVERITY: high
  FILES: internal/session/create.go:73-101, internal/session/quickstart.go:51-83
  DESCRIPTION: SessionCreator.CreateFromDir and QuickStart.Run share almost identical logic -- git root resolution, project name derivation, session name generation, project upsert, and shell command building. The only difference is the final step: SessionCreator calls tmux.NewSession (detached), while QuickStart builds exec args for process handoff. Both types hold identical dependency fields (git, store, checker/tmux, gen, shell) and the first ~80% of their Run/CreateFromDir methods are copy-pasted. This means a bug fix or behavior change (e.g., changing how project names are derived) must be applied in two places, and the two paths can silently diverge.
  RECOMMENDATION: Extract the shared pipeline into a single struct or function that returns an intermediate result (resolvedDir, projectName, sessionName, shellCmd). SessionCreator and QuickStart then consume that result to perform their respective final steps (NewSession vs exec-args assembly). This reduces the duplicated code to a thin wrapper over a shared core.

- FINDING: Package-level mutable state for dependency injection in cmd layer
  SEVERITY: medium
  FILES: cmd/open.go:21-28, cmd/open.go:77, cmd/list.go:13-14, cmd/attach.go:13-14, cmd/kill.go:13-14
  DESCRIPTION: Multiple command files use package-level variables (openDeps, listDeps, attachDeps, killDeps, parsedCommand) as the dependency injection mechanism. Package-level mutable state creates implicit coupling between test cases -- if a test forgets to nil out the global, subsequent tests break. The parsedCommand global is worse: it is set by the open command's RunE and consumed by openPath, meaning concurrent command executions (if ever needed) would race. The resetRootCmd() helper in root_test.go already hints at this fragility. This pattern also prevents running tests in parallel (t.Parallel()).
  RECOMMENDATION: Thread dependencies through a struct (e.g., an App or CommandContext) instantiated per execution. For parsedCommand, pass it as a direct parameter to openPath and openTUI rather than storing it in a package global.

- FINDING: Duplicate fuzzyMatch functions across packages
  SEVERITY: medium
  FILES: internal/tui/model.go:550-558, internal/ui/projectpicker.go:135-143, internal/ui/browser.go:131-136
  DESCRIPTION: Three independent implementations of subsequence fuzzy matching exist: tui.fuzzyMatch, ui.fuzzyMatch (projectpicker), and a near-identical pattern in browser.go's filteredEntries. All do the same thing. While the code-quality standard says to extract after three instances (Rule of Three), these are already at three instances. More importantly, if fuzzy match behavior is refined (e.g., scoring, case folding changes), three files must be updated.
  RECOMMENDATION: Extract fuzzyMatch into a shared internal utility package (e.g., internal/fuzzy or internal/match) and import it from all three locations.

- FINDING: Proliferating constructor variants on tui.Model
  SEVERITY: medium
  FILES: internal/tui/model.go:148-191
  DESCRIPTION: The TUI Model has five constructors: New, NewWithKiller, NewWithRenamer, NewWithDeps, NewWithAllDeps. Each adds one more dependency. This telescoping constructor pattern means the API grows combinatorially as new optional dependencies are added. The Model struct already has 18+ fields, many of which are optional capabilities (sessionKiller, sessionRenamer, projectStore, sessionCreator, dirLister). Callers must pick the right constructor variant or risk a nil interface at runtime (e.g., the nil checks for sessionKiller and sessionRenamer in handleKillKey/handleRenameKey).
  RECOMMENDATION: Use the functional options pattern (WithKiller(k), WithRenamer(r), etc.) on a single New constructor. This is idiomatic Go for optional configuration, matches the existing With* methods pattern already used (WithCommand, WithInsideTmux, WithInitialFilter), and prevents the combinatorial constructor explosion. Alternatively, accept a single Deps struct.

- FINDING: Interface redefinition across package boundaries
  SEVERITY: low
  FILES: cmd/list.go:16-18, cmd/open.go:32-34, cmd/open.go:37-39, cmd/kill.go:16-18, cmd/attach.go:15-17, internal/tui/model.go:26-35, internal/tui/model.go:39-45, internal/ui/projectpicker.go:13-17
  DESCRIPTION: Several interfaces (SessionLister, SessionKiller, SessionValidator, ProjectStore) are defined independently in both the cmd and tui/ui packages, each declaring the same method signatures against the same concrete type (tmux.Client or project.Store). For instance, SessionLister is defined in cmd/list.go, tui/model.go -- both require ListSessions() ([]tmux.Session, error). While Go's structural typing makes this work, the proliferation of identical interfaces makes it harder to discover the canonical contract. The same concrete type (tmux.Client) satisfies many of these, but the relationship is implicit.
  RECOMMENDATION: This is acceptable in small-scope Go projects via implicit interface satisfaction. No action required unless the interface count continues growing. If it does, consider defining canonical interfaces in the internal/tmux package itself (e.g., tmux.Lister, tmux.Killer) and importing them where needed.

- FINDING: GoReleaser config uses homebrew_casks instead of brews
  SEVERITY: medium
  FILES: .goreleaser.yaml:34-44
  DESCRIPTION: The .goreleaser.yaml uses `homebrew_casks` to publish the Homebrew formula. However, Portal is a CLI binary, not a macOS .app -- it should use `brews` (Homebrew formula) rather than `homebrew_casks` (Homebrew cask). Casks are for macOS GUI applications distributed as .dmg/.pkg/.app. Using the wrong distribution type will either fail at release time or produce a cask that doesn't install correctly for non-macOS platforms (Linux targets are also built).
  RECOMMENDATION: Replace `homebrew_casks` with `brews` in .goreleaser.yaml. The `brews` section uses the same repository configuration but produces a proper Homebrew formula for CLI tools.

SUMMARY: The main architectural concern is the duplicated session-creation pipeline between SessionCreator and QuickStart, which creates a maintenance risk for the core domain logic. The package-level mutable state for dependency injection in the cmd layer and the GoReleaser cask misconfiguration are secondary issues worth addressing. The rest of the architecture is sound -- clean package boundaries, appropriate use of interfaces, and good separation between tmux integration, session logic, and TUI rendering.

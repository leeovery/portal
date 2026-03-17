TASK: Wire cmd/open.go to New Model API (tick-dc682b)

ACCEPTANCE CRITERIA:
- [ ] openTUI("", nil) launches TUI with no command and no filter
- [ ] openTUI("", []string{"claude"}) launches TUI in command-pending mode
- [ ] openTUI("myapp", nil) launches TUI with initial filter
- [ ] openTUI("myapp", []string{"claude"}) launches with both filter and command
- [ ] Inside-tmux detection and current session name passed to model
- [ ] Selected() return value used for session attachment
- [ ] Clean exit returns nil error
- [ ] All functional options wired correctly

STATUS: Complete

SPEC CONTEXT: The spec defines cmd/open.go as the integration point where the TUI model is constructed with all dependencies (session lister, killer, renamer, project store, session creator, dir lister) and configured with optional command (command-pending mode), initial filter, and inside-tmux state. The model is run via tea.NewProgram with WithAltScreen(), and the result is processed by checking Selected() for session attachment.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/cmd/open.go:287-313` — `buildTUIModel` constructs the model with all functional options, WithCommand, WithInitialFilter, WithInsideTmux
  - `/Users/leeovery/Code/portal/cmd/open.go:318-324` — `processTUIResult` handles Selected() and connector
  - `/Users/leeovery/Code/portal/cmd/open.go:327-382` — `openTUI` wires everything: deps, config, buildTUIModel, tea.NewProgram(m, tea.WithAltScreen()), processTUIResult
  - `/Users/leeovery/Code/portal/internal/tui/model.go:256-289` — WithInitialFilter, WithCommand, WithInsideTmux methods on Model
- Notes:
  - Clean separation: `buildTUIModel` is extracted and testable without needing tmux or filesystem
  - `processTUIResult` is extracted and testable without running the full TUI
  - `tuiConfig` struct (line 272) groups injectable dependencies cleanly
  - All functional options from the spec are wired: WithKiller, WithRenamer, WithProjectStore, WithSessionCreator, WithDirLister, WithCWD, WithProjectEditor, WithAliasEditor
  - tea.WithAltScreen() preserved (line 368)
  - Inside-tmux detection at lines 359-365 correctly calls client.CurrentSessionName() and passes to cfg

TESTS:
- Status: Adequate
- Coverage:
  - `TestBuildTUIModel/"no command and no filter creates default model"` (line 743) — verifies Selected()=="", InitialFilter()=="", CommandPending()==false, InsideTmux()==false, ActivePage()==PageSessions
  - `TestBuildTUIModel/"command creates model in command-pending mode"` (line 773) — verifies CommandPending()==true, ActivePage()==PageProjects, Command()==["claude"]
  - `TestBuildTUIModel/"filter creates model with initial filter"` (line 804) — verifies InitialFilter()=="myapp", CommandPending()==false
  - `TestBuildTUIModel/"command and filter combines both"` (line 825) — verifies both filter and command set, ActivePage()==PageProjects
  - `TestBuildTUIModel/"inside tmux detection passes session name to model"` (line 849) — verifies InsideTmux()==true, CurrentSession()=="my-session", SessionListTitle() shows "(current: my-session)"
  - `TestBuildTUIModel/"cwd wired correctly"` (line 875) — verifies CWD()
  - `TestBuildTUIModel/"project and alias editors wired enables edit modal"` (line 893) — integration test verifying editors flow through to TUI behavior
  - `TestProcessTUIResult/"clean exit without selection returns nil"` (line 928) — verifies nil error and connector not called
  - `TestProcessTUIResult/"selected session name forwarded to connector"` (line 943) — verifies connector.Connect called with selected session name
- Notes:
  - All 6 planned test cases are covered plus 3 additional useful ones (cwd, inside-tmux title, editor wiring)
  - Tests exercise the extracted functions directly rather than the full openTUI (which requires real tmux/filesystem), which is the correct approach
  - Test for editor wiring (line 893) does a small integration test by sending key messages through the model, which is thorough without being excessive

CODE QUALITY:
- Project conventions: Followed — table-driven test style used where appropriate, explicit error handling, all exported functions documented, functional options pattern for dependency injection
- SOLID principles: Good — Single responsibility (buildTUIModel builds, processTUIResult handles result, openTUI orchestrates); Dependency inversion (all deps injected via interfaces and functional options); Interface segregation (SessionKiller, SessionRenamer, SessionCreator are separate small interfaces)
- Complexity: Low — buildTUIModel is a straightforward linear builder, processTUIResult is a simple conditional, openTUI follows a clear sequential flow
- Modern idioms: Yes — functional options, Bubble Tea patterns, cobra command pattern
- Readability: Good — extracted tuiConfig struct makes the parameter list clear; buildTUIModel reads top-to-bottom; conditional WithCommand/WithInitialFilter/WithInsideTmux calls are easy to follow
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `openTUI` function (line 327) cannot be directly unit tested since it constructs real tmux clients and reads the filesystem. The mitigation (extracting buildTUIModel and processTUIResult) is the correct approach and provides adequate coverage of the wiring logic. The remaining untested code is pure infrastructure glue.

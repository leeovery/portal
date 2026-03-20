AGENT: duplication
FINDINGS:
- FINDING: Repeated tuiConfig test fixture boilerplate in open_test.go
  SEVERITY: medium
  FILES: cmd/open_test.go:748, cmd/open_test.go:778, cmd/open_test.go:809, cmd/open_test.go:830, cmd/open_test.go:854, cmd/open_test.go:880, cmd/open_test.go:901, cmd/open_test.go:933, cmd/open_test.go:955, cmd/open_test.go:977, cmd/open_test.go:995
  DESCRIPTION: The same 7-field tuiConfig struct literal is copy-pasted 11 times across TestBuildTUIModel and TestBuildTUIModel_ServerStarted. Each instance repeats the identical stub wiring (mockSessionLister, stubSessionKiller, stubSessionRenamer, stubProjectStore, stubTUISessionCreator, stubDirLister, cwd). Only 3 of 11 instances add extra fields (insideTmux, currentSession, serverStarted, projectEditor, aliasEditor). This is ~77 lines of pure repetition.
  RECOMMENDATION: Extract a helper function `defaultTestTUIConfig() tuiConfig` that returns the base config with all stubs wired. Tests that need overrides can modify the returned struct before use: `cfg := defaultTestTUIConfig(); cfg.serverStarted = true`. This cuts the repeated boilerplate from 7 lines to 1 per test.

- FINDING: Near-duplicate newSessionList and newProjectList constructors
  SEVERITY: medium
  FILES: internal/tui/model.go:413-428, internal/tui/model.go:455-470
  DESCRIPTION: newSessionList and newProjectList share 9 of 14 lines identically: DisableQuitKeybindings, SetShowStatusBar(false), SetFilteringEnabled(true), setting AdditionalShortHelpKeys and AdditionalFullHelpKeys, Help.ShowAll=true, KeyMap.ShowFullHelp.Unbind(), KeyMap.CloseFullHelp.Unbind(), InfiniteScrolling=true, brightenHelpStyles. Only the delegate type, title, initial items, status bar item name, and help key function differ.
  RECOMMENDATION: Extract a common `newListModel(items []list.Item, delegate list.ItemDelegate, title string, statusSingular string, statusPlural string, helpKeys func() []key.Binding) list.Model` helper. Both newSessionList and newProjectList become 1-2 line calls to this helper. This consolidates the shared configuration into a single site.

SUMMARY: Two medium-severity findings: tuiConfig test boilerplate repeated 11 times should be extracted to a helper function, and the newSessionList/newProjectList constructors share 9 of 14 lines and should be consolidated into a parameterized helper.

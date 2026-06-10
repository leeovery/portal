TASK: 2-1 — Remove the file browser from the internal/tui package (model.go + coupled tui test files)

ACCEPTANCE CRITERIA:
- No remaining internal/tui references to pageFileBrowser, DirLister, WithDirLister, dirLister, startPath, fileBrowser, handleBrowseKey, updateFileBrowser, ui.Browser*Msg, mockDirLister, b/"browse" keybinding; no internal/ui or internal/browser imports.
- Survivors createSession, cfg.cwd/m.cwd, viewCWD, WithCWD present and referenced.
- pageFileBrowser gone; pagePreview renumbers 4->3 transparently (no int<->page cast).
- TestCommandPendingNKey exists; no TestCommandPendingBrowseAndNKey; survivor n-key-only.
- Surface-audit allow-list has no "browser"/"ui" keys; test coherent.
- Whole functions/subtests removed; reworked tests survive with browser setup stripped; pagepreview doc comments reconciled.

STATUS: Complete

SPEC CONTEXT: internal/tui is the only production consumer of internal/ui; this is the consumer-side removal that lands before Phase 3 deletes the package.

IMPLEMENTATION:
- Status: Implemented (verified at final HEAD)
- page iota: pageFileBrowser absent, pagePreview at iota position 3; update arm (model.go:1520) + view arm (model.go:2224) reference pagePreview only.
- No internal/ui import; Projects-page key switch (model.go:1567-1613) has no case isRuneKey(msg,"b"); b falls through to projectList.Update (model.go:1617) — spec-required visible no-op.
- projectHelpKeys/commandPendingHelpKeys carry no b binding; doc comment updated ("Only enter (run here), n, /, and q are shown").
- Survivors intact: WithCWD (model.go:534), createSession (model.go:1529) with handleProjectEnter (L1629) + createSessionInCWD (L2196) callers.
- Whole-package sweep for all removed symbols: zero matches.
- Iota-safety confirmed: all page comparisons symbolic, no int(page) cast / numeric comparison.

TESTS:
- Status: Adequate (removal/rework, not addition — matches spec).
- TestFileBrowserIntegration + TestFileBrowserFromProjectsPage deleted; TestCommandPendingNKey present (model_test.go:6171), n-key-only; TestKillSession + TestNewWithFunctionalOptions "all options combined" reworked, browser setup stripped, assertions intact; mockDirLister removed.
- pagepreview_entry_test.go: TestSpaceOnFileBrowserPage... deleted, sibling present, header reference dropped; refetch + bracket comments reconciled; surface-audit allow-list clean and test coherent.

CODE QUALITY:
- Idiomatic Go, symbolic page constants, no orphaned imports, no dangling helpers, no leftover feature comments in-package. Low complexity (pure deletion).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/switch_view_key_test.go:29 — comment "// keyS is the browse-mode switch-view key." The "browse-mode" wording is misleading: keyS ('s') is the session-list grouping switch-view key (Flat/By Project/By Tag), unrelated to the removed file browser. Reword e.g. "// keyS is the session-list grouping switch-view key." Pre-existing wording outside this task's manifest; zero-risk doc-only clarity fix.

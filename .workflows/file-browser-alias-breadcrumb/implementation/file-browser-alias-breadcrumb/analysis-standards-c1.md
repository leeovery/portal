AGENT: standards
CYCLE: 1
STATUS: clean
FINDINGS: none

SUMMARY: The file-browser removal conforms exactly to the spec's Removal Manifest and all
project conventions — no spec drift or standards violations.

Verified conformant:
- Packages deleted: internal/ui/ and internal/browser/ no longer exist on disk.
- Zero removed-symbol references (Go-wide grep clean): pageFileBrowser, DirLister,
  WithDirLister, osDirLister, mockDirLister, stubDirLister, handleBrowseKey,
  updateFileBrowser, all Browser*Msg types, AliasSaver, GitRootResolver, PathChecker,
  NewFileBrowser* constructors, browser.DirEntry/ListDirectories, the b/"browse"
  keybinding, and the dangling fields startPath/dirLister/fileBrowser.
- Bare quoted tokens gone: no "ui"/"browser" keys in the surface-audit preExistingPackages
  map; audit logic intact.
- Survivors preserved: createSession with its 3 non-browser callers; cfg.cwd/m.cwd;
  WithCWD; projects-modal alias editor via SetAndSave/DeleteAndSave; the page iota cleanly
  renumbered to Loading→Sessions→Projects→Preview.
- Naming end-state: TestCommandPendingBrowseAndNKey renamed to TestCommandPendingNKey
  (model_test.go:6171); commandPendingHelpKeys doc comment updated to drop the stale b.
- Import hygiene: internal/browser removed from cmd/open.go + cmd/open_test.go; internal/ui
  removed from model.go; both removed from model_test.go. No orphaned imports/vars.
- Deleted whole functions absent: TestFileBrowserIntegration, TestFileBrowserFromProjectsPage,
  TestSpaceOnFileBrowserPageDoesNotCallNewPreviewModel; sibling
  TestSpaceOnProjectsPageDoesNotCallNewPreviewModel survives.
- Docs: README.md:253 "three views..."; CLAUDE.md free of file-browser prose.
- Convention gates green: go build ./..., go vet ./..., gofmt -l (all 8 edited files clean),
  go test -count=1 ./internal/tui/... ./cmd/... all pass; no t.Parallel() introduced.

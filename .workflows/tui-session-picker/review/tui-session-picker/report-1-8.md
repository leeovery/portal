TASK: Remove Old ProjectPickerModel (tick-de5cb8)

ACCEPTANCE CRITERIA:
- internal/ui/projectpicker.go is deleted
- internal/ui/projectpicker_test.go is deleted
- No dead code from the old ProjectPickerModel remains in internal/ui/
- All tests in internal/tui/ pass
- All tests in internal/ui/ pass (browser tests)
- go vet ./... reports no issues
- No unused imports in any modified file
- File browser (internal/ui/browser.go) still compiles and its tests pass
- internal/tui/model.go no longer references ProjectPickerModel or its specific message types

STATUS: Complete

SPEC CONTEXT: The specification explicitly states: "ProjectPickerModel (internal/ui/projectpicker.go) is deleted along with its associated tests. All project listing functionality moves into a bubbles/list page within the main TUI model." and "Any code, tests, or message types that exist solely to support the old ProjectPickerModel should be removed rather than left as dead code."

IMPLEMENTATION:
- Status: Implemented
- Location: Files deleted: internal/ui/projectpicker.go, internal/ui/projectpicker_test.go (confirmed absent via glob)
- Notes:
  - No file matching `internal/ui/projectpicker*.go` exists in the codebase.
  - No Go source files contain references to `ProjectPickerModel`, `projectPicker`, `updateProjectPicker`, `ProjectSelectedMsg`, `BrowseSelectedMsg`, `BackMsg`, or `viewState`.
  - The `internal/ui/` package now contains only `browser.go` and `browser_test.go`, which are retained per spec.
  - `internal/tui/model.go` imports `internal/ui` solely for browser-related types (`ui.DirLister`, `ui.FileBrowserModel`, `ui.BrowserDirSelectedMsg`, `ui.BrowserCancelMsg`, `ui.NewFileBrowser`), all defined in `browser.go`.
  - Interfaces previously in the old projectpicker (`ProjectStore`, `ProjectEditor`, `AliasEditor`) are now declared in `internal/tui/model.go` (lines 35-67) and do not exist in `internal/ui/`.
  - The `editField` type and constants are declared in `internal/tui/model.go` (lines 69-75) for the new edit modal, not as dead code from the old picker.
  - No `projectPicker` field or `updateProjectPicker` method exists in the Model struct.

TESTS:
- Status: Adequate
- Coverage: This is a deletion/cleanup task. The relevant tests are:
  - `internal/ui/browser_test.go` (browser tests still exist and cover browser functionality)
  - `internal/tui/model_test.go` (TUI tests exist and do not reference old types)
  - Acceptance criteria specifies "all existing tests pass after cleanup" and "go vet reports no issues" -- both are build-level verifications rather than new test cases
- Notes: No new tests are needed for a deletion task. The existing test suites in both packages remain intact and reference only current types.

CODE QUALITY:
- Project conventions: Followed -- Go idioms, proper package separation
- SOLID principles: Good -- clean interface boundaries between tui and ui packages
- Complexity: Low -- straightforward deletion with no new logic
- Modern idioms: Yes
- Readability: Good -- remaining code is clean with no orphaned references
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)

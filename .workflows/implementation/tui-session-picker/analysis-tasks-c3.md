---
topic: tui-session-picker
cycle: 3
total_proposed: 2
---
# Analysis Tasks: tui-session-picker (Cycle 3)

## Task 1: Extract rune-key matching helper
status: pending
severity: medium
sources: duplication

**Problem**: The expression `msg.Type == tea.KeyRunes && string(msg.Runes) == "x"` is repeated 17 times across `updateProjectsPage`, `updateSessionList`, `updateKillConfirmModal`, and `updateDeleteProjectModal` in `internal/tui/model.go`. This verbose 3-part comparison adds line noise and makes switch cases harder to scan.

**Solution**: Extract a helper function `func isRuneKey(msg tea.KeyMsg, ch string) bool` in `internal/tui/model.go` that encapsulates the type check and rune comparison. Replace all 17 call sites with calls to this helper.

**Outcome**: All rune-key checks in model.go use `isRuneKey(msg, "x")` instead of the verbose 3-part expression. No behavioral change.

**Do**:
1. Add to `internal/tui/model.go`:
   ```go
   func isRuneKey(msg tea.KeyMsg, ch string) bool {
       return msg.Type == tea.KeyRunes && string(msg.Runes) == ch
   }
   ```
2. Find all 17 instances of `msg.Type == tea.KeyRunes && string(msg.Runes) == "..."` in model.go
3. Replace each with `isRuneKey(msg, "...")` using the appropriate character
4. Run existing tests to confirm no regressions

**Acceptance Criteria**:
- No remaining instances of the verbose rune-key pattern in model.go
- `isRuneKey` helper exists as a package-private function in model.go
- All existing tests pass without modification

**Tests**:
- Existing test suite covers all key-handling paths; no new tests needed since this is a pure refactor with no behavioral change

## Task 2: Wire edit-project dependencies in production
status: pending
severity: medium
sources: architecture

**Problem**: `WithProjectEditor` and `WithAliasEditor` options are defined, implemented, and tested in `internal/tui/model.go`, but `cmd/open.go`'s `buildTUIModel` function never injects either dependency. The `handleEditProjectKey` method guards on `m.projectEditor == nil || m.aliasEditor == nil` and silently returns nil, so pressing `e` on the Projects page is a silent no-op in production despite the help bar advertising `[e] edit`. The spec requires: "e triggers a modal overlay with the project's name field, alias list, and full edit controls".

**Solution**: In `cmd/open.go`'s `buildTUIModel`, construct the appropriate `ProjectEditor` and `AliasEditor` implementations from the existing `project.Store` and alias infrastructure, and pass them via `tui.WithProjectEditor(...)` and `tui.WithAliasEditor(...)`. If the concrete implementations do not yet exist outside the test doubles, create them as thin adapters over the existing store.

**Outcome**: Pressing `e` on the Projects page in production opens the edit modal as specified. The help bar's `[e] edit` binding matches actual behavior.

**Do**:
1. Identify or create production implementations of `tui.ProjectEditor` and `tui.AliasEditor` interfaces using existing `project.Store` and alias management code
2. In `cmd/open.go` `buildTUIModel`, add `tui.WithProjectEditor(editor)` and `tui.WithAliasEditor(aliasEditor)` to the `tui.New(...)` call
3. Run existing tests and manually verify the `e` key opens the edit modal

**Acceptance Criteria**:
- `buildTUIModel` in `cmd/open.go` passes both `WithProjectEditor` and `WithAliasEditor` options
- Pressing `e` on the Projects page in a running `portal open` session opens the edit project modal
- All existing tests pass

**Tests**:
- Existing TUI tests already cover the edit modal flow with test doubles; confirm they still pass
- Manual smoke test: run `portal open`, navigate to Projects, press `e` on a project, verify modal appears

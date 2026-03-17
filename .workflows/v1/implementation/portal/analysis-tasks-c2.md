---
topic: portal
cycle: 2
total_proposed: 3
---
# Analysis Tasks: Portal (Cycle 2)

## Task 1: Extract generic FuzzyFilter function
status: approved
severity: medium
sources: duplication

**Problem**: Three locations implement identical fuzzy filter-and-collect logic: `internal/tui/model.go:542-553` (sessions), `internal/ui/projectpicker.go:121-133` (projects), and `internal/ui/browser.go:125-137` (directory entries). The model.go version has a copy-paste drift bug: `strings.ToLower(m.filterText)` is called inside the loop on every iteration instead of being hoisted outside.

**Solution**: Add a generic `Filter` function to `internal/fuzzy/match.go`: `func Filter[T any](items []T, filter string, nameOf func(T) string) []T`. It returns all items if filter is empty, otherwise collects items where `fuzzy.Match` succeeds on lowercased name vs lowercased filter. Replace all three callsites.

**Outcome**: Single implementation of the fuzzy filter pattern. The ToLower-per-iteration bug in model.go is fixed. Future filter sites use the same helper.

**Do**:
1. In `internal/fuzzy/match.go`, add:
   ```go
   func Filter[T any](items []T, filter string, nameOf func(T) string) []T {
       if filter == "" {
           return items
       }
       lowerFilter := strings.ToLower(filter)
       var result []T
       for _, item := range items {
           if Match(strings.ToLower(nameOf(item)), lowerFilter) {
               result = append(result, item)
           }
       }
       return result
   }
   ```
2. Add `"strings"` import to `internal/fuzzy/match.go`.
3. In `internal/tui/model.go`, replace `filterMatchedSessions()` body with:
   ```go
   return fuzzy.Filter(m.sessions, m.filterText, func(s tmux.Session) string { return s.Name })
   ```
4. In `internal/ui/projectpicker.go`, replace `filteredProjects()` body with:
   ```go
   return fuzzy.Filter(m.projects, m.filterText, func(p project.Project) string { return p.Name })
   ```
5. In `internal/ui/browser.go`, replace `filteredEntries()` body with:
   ```go
   return fuzzy.Filter(m.entries, m.filterText, func(e browser.DirEntry) string { return e.Name })
   ```
6. Remove now-unused `strings` imports from the three callsite files if no other usage remains.

**Acceptance Criteria**:
- `fuzzy.Filter` exists and is generic over any type with a name accessor
- All three previous filter callsites use `fuzzy.Filter`
- No direct fuzzy.Match + ToLower loop remains in tui/model.go, ui/projectpicker.go, or ui/browser.go
- The ToLower-per-iteration bug in model.go is gone

**Tests**:
- Unit tests for `fuzzy.Filter` in `internal/fuzzy/match_test.go`: empty filter returns all items, non-matching filter returns empty, partial match filters correctly, case-insensitive matching works
- Existing tests in tui and ui packages continue to pass

## Task 2: Deduplicate ProjectStore interface
status: approved
severity: medium
sources: duplication

**Problem**: `internal/tui/model.go:32-36` and `internal/ui/projectpicker.go:14-18` define identical `ProjectStore` interfaces with the same three methods (`List`, `CleanStale`, `Remove`). The tui package already imports from ui (it aliases `DirLister = ui.DirLister` at line 54), establishing precedent for cross-package type reuse.

**Solution**: Remove the `ProjectStore` definition from `internal/tui/model.go` and add a type alias `type ProjectStore = ui.ProjectStore`, following the existing `DirLister` pattern.

**Outcome**: Single source of truth for the ProjectStore interface in `internal/ui/projectpicker.go`. The tui package reuses it via alias.

**Do**:
1. In `internal/tui/model.go`, replace the `ProjectStore` interface definition (lines 32-36) with:
   ```go
   type ProjectStore = ui.ProjectStore
   ```
2. Verify the tui package already imports `"github.com/leeovery/portal/internal/ui"` (it does, for DirLister).
3. Run `go build ./...` to confirm no compilation errors.

**Acceptance Criteria**:
- `internal/tui/model.go` no longer defines its own ProjectStore interface
- `internal/tui/model.go` uses `ui.ProjectStore` via type alias
- All existing tests pass without modification

**Tests**:
- All existing tests in `internal/tui/` and `internal/ui/` pass unchanged (the alias is structurally identical)

## Task 3: Remove redundant quickStartResult mirror type
status: approved
severity: medium
sources: duplication, architecture

**Problem**: `cmd/open.go:162-166` defines a local `quickStartResult` struct that is a field-for-field copy of `session.QuickStartResult` (lines 10-17 of `internal/session/quickstart.go`). A `quickStartAdapter` (lines 187-202) exists solely to convert between them. The cmd package already imports `internal/session`, and `session.QuickStartResult` is a plain data struct with no behavior -- there is no encapsulation benefit to the mirror type.

**Solution**: Change the `quickStarter` interface to return `*session.QuickStartResult` directly. Remove the local `quickStartResult` struct and the `quickStartAdapter` type. Have `PathOpener` use `*session.QuickStartResult`. Create a simple `quickStartAdapter` that just delegates without field copying.

**Outcome**: One fewer redundant type. No field-by-field copy adapter. The `quickStarter` interface and `PathOpener` use `session.QuickStartResult` directly.

**Do**:
1. In `cmd/open.go`, change the `quickStarter` interface:
   ```go
   type quickStarter interface {
       Run(path string, command []string) (*session.QuickStartResult, error)
   }
   ```
2. Remove the `quickStartResult` struct definition (lines 162-166).
3. Replace the `quickStartAdapter` with a simpler version that delegates directly:
   ```go
   type quickStartAdapter struct {
       qs *session.QuickStart
   }

   func (a *quickStartAdapter) Run(path string, command []string) (*session.QuickStartResult, error) {
       return a.qs.Run(path, command)
   }
   ```
4. In `PathOpener.Open`, the `result` from `po.qs.Run(...)` is now `*session.QuickStartResult` -- no changes needed since the field names are the same (`ExecArgs`).
5. Update any test mocks in `cmd/open_test.go` that return `*quickStartResult` to return `*session.QuickStartResult` instead.

**Acceptance Criteria**:
- No `quickStartResult` struct exists in `cmd/open.go`
- The `quickStarter` interface returns `*session.QuickStartResult`
- The `quickStartAdapter` delegates without field-by-field copying
- `PathOpener.Open` works unchanged

**Tests**:
- All existing tests in `cmd/` pass (mock returns updated to `*session.QuickStartResult`)

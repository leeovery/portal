---
topic: resume-sessions-after-reboot
cycle: 1
total_proposed: 5
---
# Analysis Tasks: resume-sessions-after-reboot (Cycle 1)

## Task 1: Extract pane output parsing helper in tmux package
status: pending
severity: medium
sources: duplication

**Problem**: `ListPanes` (lines 212-225) and `ListAllPanes` (lines 237-250) in `internal/tmux/tmux.go` contain an identical 10-line block that checks for empty output, splits on newlines, trims whitespace, and filters empty lines into a `[]string`. The only difference is the tmux arguments and error handling preceding the parsing.

**Solution**: Extract a private helper `parsePaneOutput(output string) []string` in `internal/tmux/tmux.go` that handles the split/trim/filter logic. Both methods call it after their divergent `Run` + error handling.

**Outcome**: The duplicated parsing body exists in exactly one place. Both `ListPanes` and `ListAllPanes` delegate to the shared helper after their respective `cmd.Run` calls.

**Do**:
1. In `internal/tmux/tmux.go`, add a private function:
   ```go
   func parsePaneOutput(output string) []string {
       if output == "" {
           return []string{}
       }
       lines := strings.Split(output, "\n")
       panes := make([]string, 0, len(lines))
       for _, line := range lines {
           line = strings.TrimSpace(line)
           if line == "" {
               continue
           }
           panes = append(panes, line)
       }
       return panes
   }
   ```
2. Replace the parsing body in `ListPanes` (lines 212-225) with `return parsePaneOutput(output), nil`.
3. Replace the parsing body in `ListAllPanes` (lines 237-250) with `return parsePaneOutput(output), nil`.

**Acceptance Criteria**:
- `parsePaneOutput` is a private function in `internal/tmux/tmux.go`
- `ListPanes` and `ListAllPanes` both call `parsePaneOutput` instead of inlining the parsing
- No duplicated split/trim/filter logic remains between the two methods

**Tests**:
- All existing tests pass: `go test ./internal/tmux/...`
- All existing tests pass: `go test ./cmd/...`

## Task 2: Extract atomic JSON write utility
status: pending
severity: medium
sources: duplication

**Problem**: `hooks.Store.Save` (`internal/hooks/store.go:54-88`) and `project.Store.Save` (`internal/project/store.go:57-91`) implement the same atomic write pipeline: `MkdirAll`, `json.MarshalIndent`, `CreateTemp`, `Write`, `Close`, `Rename`, with identical cleanup on each error path. The two implementations are nearly line-for-line identical, differing only in the temp file name prefix and the data being marshaled.

**Solution**: Extract an `internal/fileutil` package with a function `AtomicWrite(path string, data []byte) error` that handles the `MkdirAll` / `CreateTemp` / `Write` / `Close` / `Rename` pipeline. Both stores marshal their data, then call `AtomicWrite` with the resulting bytes.

**Outcome**: The atomic write pipeline (MkdirAll through Rename with cleanup) exists in exactly one place. Both stores are simplified to marshal + delegate.

**Do**:
1. Create `internal/fileutil/atomic.go` with package `fileutil` containing:
   ```go
   func AtomicWrite(path string, data []byte) error {
       dir := filepath.Dir(path)
       if err := os.MkdirAll(dir, 0o755); err != nil {
           return fmt.Errorf("failed to create directory: %w", err)
       }
       tmp, err := os.CreateTemp(dir, "*.tmp")
       if err != nil {
           return fmt.Errorf("failed to create temp file: %w", err)
       }
       tmpPath := tmp.Name()
       if _, err := tmp.Write(data); err != nil {
           _ = tmp.Close()
           _ = os.Remove(tmpPath)
           return fmt.Errorf("failed to write temp file: %w", err)
       }
       if err := tmp.Close(); err != nil {
           _ = os.Remove(tmpPath)
           return fmt.Errorf("failed to close temp file: %w", err)
       }
       if err := os.Rename(tmpPath, path); err != nil {
           _ = os.Remove(tmpPath)
           return fmt.Errorf("failed to rename temp file: %w", err)
       }
       return nil
   }
   ```
2. Simplify `hooks.Store.Save` to: marshal with `json.MarshalIndent`, then call `fileutil.AtomicWrite(s.path, data)`.
3. Simplify `project.Store.Save` to: wrap in `projectsFile`, marshal with `json.MarshalIndent`, then call `fileutil.AtomicWrite(s.path, data)`.
4. Remove the duplicated MkdirAll/CreateTemp/Write/Close/Rename blocks from both stores.

**Acceptance Criteria**:
- `internal/fileutil/atomic.go` exists with `AtomicWrite` function
- `hooks.Store.Save` and `project.Store.Save` both delegate to `fileutil.AtomicWrite`
- No duplicated temp-file-rename pipeline in either store
- Error messages in the stores remain descriptive (wrap `AtomicWrite` errors with context if needed)

**Tests**:
- All existing tests pass: `go test ./internal/hooks/...`
- All existing tests pass: `go test ./internal/project/...`
- All existing tests pass: `go test ./cmd/...`

## Task 3: Centralize volatile marker name format
status: pending
severity: medium
sources: architecture

**Problem**: The marker name format `@portal-active-%s` (or its concatenation equivalent `"@portal-active-"+paneID`) is constructed independently in `cmd/hooks.go` (lines 107 and 152, in `set` and `rm` commands) and `internal/hooks/executor.go` (line 80). If the naming convention changes, all three sites must be updated in sync. A mismatch between cmd and executor would silently break the two-condition execution check — markers set by `hooks set` would never be found by `ExecuteHooks`.

**Solution**: Define a `MarkerName(paneID string) string` function in the `hooks` package and use it from both `cmd/hooks.go` and `executor.go`.

**Outcome**: The marker name convention is defined in exactly one place. Any future change to the format automatically propagates to all usage sites.

**Do**:
1. In `internal/hooks/executor.go` (or a new `internal/hooks/marker.go` file), add:
   ```go
   func MarkerName(paneID string) string {
       return fmt.Sprintf("@portal-active-%s", paneID)
   }
   ```
2. In `internal/hooks/executor.go` line 80, replace `fmt.Sprintf("@portal-active-%s", paneID)` with `MarkerName(paneID)`.
3. In `internal/hooks/executor.go` line 86, replace `fmt.Sprintf(...)` or the inline string with `MarkerName(paneID)` (for the `SetServerOption` call).
4. In `cmd/hooks.go` line 107, replace `"@portal-active-"+paneID` with `hooks.MarkerName(paneID)`.
5. In `cmd/hooks.go` line 152, replace `"@portal-active-"+paneID` with `hooks.MarkerName(paneID)`.

**Acceptance Criteria**:
- `hooks.MarkerName` is the single source of truth for the marker name format
- No hardcoded `@portal-active-` string remains in `cmd/hooks.go` or `executor.go`
- All three usage sites call `hooks.MarkerName`

**Tests**:
- All existing tests pass: `go test ./internal/hooks/...`
- All existing tests pass: `go test ./cmd/...`

## Task 4: Group ExecuteHooks parameters into composed interfaces
status: pending
severity: medium
sources: architecture

**Problem**: `ExecuteHooks` in `internal/hooks/executor.go` takes 7 parameters where the tmux client is passed 4 times and the store is passed twice. The call site in `cmd/hook_executor.go:25` reads `hooks.ExecuteHooks(sessionName, client, store, client, client, client, store)` — fragile and hard to read. The caller already groups the tmux interfaces into an anonymous composed interface at the `buildHookExecutor` parameter level, but `ExecuteHooks` itself doesn't benefit from this grouping.

**Solution**: Define two composed interfaces in the hooks package — one for tmux operations and one for store operations — and refactor `ExecuteHooks` to accept `sessionName`, the tmux composed interface, and the store composed interface (3 parameters).

**Outcome**: `ExecuteHooks` has 3 parameters instead of 7. The call site passes each dependency once. The small interfaces remain defined for ISP compliance.

**Do**:
1. In `internal/hooks/executor.go`, define two composed interfaces:
   ```go
   type TmuxOperator interface {
       PaneLister
       KeySender
       OptionChecker
       AllPaneLister
   }

   type HookRepository interface {
       HookLoader
       HookCleaner
   }
   ```
2. Change the `ExecuteHooks` signature to:
   ```go
   func ExecuteHooks(sessionName string, tmux TmuxOperator, store HookRepository)
   ```
3. Update the function body to use `tmux` instead of `lister`/`sender`/`checker`/`allLister` and `store` instead of `loader`/`cleaner`.
4. Update `cmd/hook_executor.go` — simplify the call to `hooks.ExecuteHooks(sessionName, client, store)`. The anonymous composed interface on `buildHookExecutor`'s parameter can be replaced with `hooks.TmuxOperator`.
5. Update any test mocks that call `ExecuteHooks` directly to pass the composed interfaces.

**Acceptance Criteria**:
- `ExecuteHooks` takes exactly 3 parameters: `sessionName string`, `tmux TmuxOperator`, `store HookRepository`
- The small interfaces (`PaneLister`, `KeySender`, etc.) remain defined in the hooks package
- The call site in `cmd/hook_executor.go` passes `client` once and `store` once
- All existing test mocks compile and pass

**Tests**:
- All existing tests pass: `go test ./internal/hooks/...`
- All existing tests pass: `go test ./cmd/...`

## Task 5: Remove duplicate AllPaneLister interface from cmd/clean.go
status: pending
severity: low
sources: duplication, architecture

**Problem**: `cmd/clean.go` (lines 11-14) defines its own `AllPaneLister` interface identical to `hooks.AllPaneLister` (lines 27-29 in `internal/hooks/executor.go`). Both have the single method `ListAllPanes() ([]string, error)`. The `cmd` package already imports `internal/hooks` (via `cmd/hook_executor.go`), so there is no circular dependency concern. If the method signature changes in one place, the other silently becomes a different interface.

**Solution**: Remove the `AllPaneLister` interface definition from `cmd/clean.go` and use `hooks.AllPaneLister` instead.

**Outcome**: The `AllPaneLister` interface is defined in exactly one place (`internal/hooks`). The `cmd` package references it by import.

**Do**:
1. In `cmd/clean.go`, add `"github.com/leeovery/portal/internal/hooks"` to the imports (if not already imported via another file in the package — check if the package-level import is sufficient).
2. In `cmd/clean.go`, remove the `AllPaneLister` interface definition (lines 11-14).
3. In `cmd/clean.go`, change the `CleanDeps` struct field type from `AllPaneLister` to `hooks.AllPaneLister`.
4. Update any references to the local `AllPaneLister` type in `cmd/clean.go` to use `hooks.AllPaneLister`.

**Acceptance Criteria**:
- No `AllPaneLister` interface definition exists in `cmd/clean.go`
- `CleanDeps.AllPaneLister` field uses `hooks.AllPaneLister` type
- No circular import introduced

**Tests**:
- All existing tests pass: `go test ./cmd/...`

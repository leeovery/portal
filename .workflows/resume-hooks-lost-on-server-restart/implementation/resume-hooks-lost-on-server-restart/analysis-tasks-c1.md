---
topic: resume-hooks-lost-on-server-restart
cycle: 1
total_proposed: 3
---
# Analysis Tasks: resume-hooks-lost-on-server-restart (Cycle 1)

## Task 1: Consolidate duplicate hooks JSON test helpers
status: pending
severity: medium
sources: duplication, architecture

**Problem**: `writeHooksJSON`/`readHooksJSON` in `cmd/hooks_test.go` and `writeCleanHooksJSON`/`readCleanHooksJSON` in `cmd/clean_test.go` are functionally identical — same parameters, same marshal/unmarshal logic, same error handling. They differ only in function name. Both files are in package `cmd` so sharing is trivial.

**Solution**: Extract the shared helpers into a new file `cmd/testhelpers_test.go` with a single `writeHooksJSON` and `readHooksJSON` implementation. Remove the duplicates from both test files and update all callers.

**Outcome**: Single source of truth for hooks JSON test helpers. Adding or changing the helper format requires editing one location.

**Do**:
1. Create `cmd/testhelpers_test.go` (package `cmd`) containing `writeHooksJSON` and `readHooksJSON` copied from `cmd/hooks_test.go` (lines 761-784).
2. Remove `writeHooksJSON` and `readHooksJSON` from `cmd/hooks_test.go`.
3. Remove `writeCleanHooksJSON` and `readCleanHooksJSON` from `cmd/clean_test.go`.
4. Update all call sites in `cmd/clean_test.go` that reference `writeCleanHooksJSON`/`readCleanHooksJSON` to use `writeHooksJSON`/`readHooksJSON`.
5. Run `go test ./cmd/...` to confirm all tests pass.

**Acceptance Criteria**:
- No duplicate hooks JSON helpers exist across cmd test files
- `writeHooksJSON` and `readHooksJSON` are defined exactly once in `cmd/testhelpers_test.go`
- All existing tests in `cmd/hooks_test.go` and `cmd/clean_test.go` pass unchanged

**Tests**:
- `go test ./cmd/...` passes with no failures

## Task 2: Extract shared pane structural-key resolution helper in hooks commands
status: pending
severity: medium
sources: duplication

**Problem**: Both `hooksSetCmd` and `hooksRmCmd` in `cmd/hooks.go` contain an identical 12-line block: call `requireTmuxPane`, resolve `keyResolver` from `hooksDeps` with nil-check fallback to `buildHooksTmuxClient`, call `ResolveStructuralKey`, and wrap the error. This copy-paste creates a maintenance risk — changes to the resolution flow must be made in both places.

**Solution**: Extract a helper function `resolveCurrentPaneKey() (string, error)` in `cmd/hooks.go` that encapsulates the full sequence: get pane env var, resolve the key resolver dependency, call `ResolveStructuralKey`, and return the structural key or wrapped error. Both commands call this single function.

**Outcome**: Pane-to-structural-key resolution logic is defined once. Future changes (e.g., error message updates, additional resolution steps) only need one edit.

**Do**:
1. In `cmd/hooks.go`, define a new unexported function `resolveCurrentPaneKey() (string, error)` that performs: (a) call `requireTmuxPane()` to get the pane ID, (b) resolve `keyResolver` from `hooksDeps.keyResolver` with nil-check fallback to `buildHooksTmuxClient().ResolveStructuralKey`, (c) call the resolver with the pane ID, (d) return the key or wrap the error with a descriptive message.
2. Replace the duplicated block in `hooksSetCmd`'s `RunE` (approx lines 91-105) with a call to `resolveCurrentPaneKey()`.
3. Replace the duplicated block in `hooksRmCmd`'s `RunE` (approx lines 157-172) with a call to `resolveCurrentPaneKey()`.
4. Run `go test ./cmd/... -run TestHooks` to confirm all hooks tests pass.

**Acceptance Criteria**:
- `resolveCurrentPaneKey` is defined once in `cmd/hooks.go`
- Neither `hooksSetCmd` nor `hooksRmCmd` contains inline pane-resolution logic — both delegate to `resolveCurrentPaneKey`
- All existing hooks tests pass unchanged

**Tests**:
- `go test ./cmd/... -run TestHooks` passes with no failures

## Task 3: Rename residual paneID parameter names to structural-key terminology
status: pending
severity: low
sources: standards, architecture

**Problem**: Several interface and method parameters still use `paneID`/`livePaneIDs` naming when the values they carry are now structural keys (`session:window.pane`). Specifically: `KeySender.SendKeys(paneID string, ...)` in `internal/hooks/executor.go:12`, `HookCleaner.CleanStale(livePaneIDs []string)` in `internal/hooks/executor.go:36`, and `tmux.Client.SendKeys(paneID string, ...)` in `internal/tmux/tmux.go:257`. The spec explicitly states `CleanStale` parameter should be renamed. Misleading parameter names could confuse future contributors about what values these functions accept.

**Solution**: Rename parameters (not function/method names) to match structural-key semantics: `paneID` becomes `target` in `SendKeys`, `livePaneIDs` becomes `liveKeys` in `CleanStale`.

**Outcome**: Parameter names across interfaces and implementations consistently reflect that these values are structural keys, matching the spec and the concrete `Store.CleanStale` implementation which already uses `liveKeys`.

**Do**:
1. In `internal/hooks/executor.go:12`, rename `KeySender.SendKeys` parameter from `paneID string` to `target string`.
2. In `internal/hooks/executor.go:36`, rename `HookCleaner.CleanStale` parameter from `livePaneIDs []string` to `liveKeys []string`.
3. In `internal/tmux/tmux.go:257`, rename `SendKeys` method parameter from `paneID string` to `target string`.
4. Check for any other references to these parameter names in doc comments or inline comments within the same files and update them.
5. In `internal/hooks/executor_test.go:106` and surrounding mock definitions, update parameter names to match (`livePaneIDs` to `liveKeys`, `paneID` to `target`).
6. Run `go test ./internal/hooks/... ./internal/tmux/...` to confirm all tests pass.

**Acceptance Criteria**:
- No parameter named `paneID` or `livePaneIDs` exists in `internal/hooks/executor.go` or `internal/tmux/tmux.go` for these methods
- `SendKeys` parameter is named `target` in both the interface and the concrete implementation
- `CleanStale` parameter is named `liveKeys` in the interface (matching the concrete `Store` implementation)
- All tests pass unchanged

**Tests**:
- `go test ./internal/hooks/... ./internal/tmux/...` passes with no failures

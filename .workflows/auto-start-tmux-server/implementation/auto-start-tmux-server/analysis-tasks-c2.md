---
topic: auto-start-tmux-server
cycle: 2
total_proposed: 2
---
# Analysis Tasks: Auto Start Tmux Server (Cycle 2)

## Task 1: Consolidate duplicate test mock types across cmd test files
status: pending
severity: medium
sources: duplication

**Problem**: Two pairs of structurally identical mock types exist in `cmd/` test files within the same package. `mockSessionConnector` (attach_test.go:11) and `mockConnector` (open_test.go:757) have identical fields and methods. Similarly, `mockSessionLister` (list_test.go:13) and `stubSessionLister` (open_test.go:671) have identical fields and methods. Since all test files are in the same `cmd` package, only one definition of each is needed.

**Solution**: Remove the duplicate definitions from open_test.go and update references there to use the canonical versions in attach_test.go and list_test.go.

**Outcome**: Each mock type has exactly one definition in the `cmd` package test files. All tests continue to pass.

**Do**:
1. In `cmd/open_test.go`, delete the `mockConnector` type definition and its `Connect` method (lines ~757-765).
2. In `cmd/open_test.go`, update all references from `mockConnector` to `mockSessionConnector` (the type defined in attach_test.go).
3. In `cmd/open_test.go`, delete the `stubSessionLister` type definition and its `ListSessions` method (lines ~671-678).
4. In `cmd/open_test.go`, update all references from `stubSessionLister` to `mockSessionLister` (the type defined in list_test.go).
5. Run `go vet ./cmd/...` and `go test ./cmd/...` to confirm compilation and test passage.

**Acceptance Criteria**:
- `mockConnector` type no longer exists in open_test.go
- `stubSessionLister` type no longer exists in open_test.go
- Only one `mockSessionConnector` definition exists in the `cmd` package (in attach_test.go)
- Only one `mockSessionLister` definition exists in the `cmd` package (in list_test.go)
- All tests pass: `go test ./cmd/...`

**Tests**:
- Existing tests in attach_test.go, list_test.go, and open_test.go all pass unchanged (no new tests needed — this is a pure deduplication refactor)

## Task 2: Thread cobra.Command through openTUI to eliminate implicit openCmd coupling
status: pending
severity: medium
sources: architecture

**Problem**: `openTUI` (cmd/open.go:337) calls `tmuxClient(openCmd)` on line 338, reaching into the package-level `openCmd` variable for its cobra context. The function signature `func(string, []string, bool) error` hides this dependency. This is the only function in the codebase that implicitly accesses a package-level command's context rather than receiving the command as a parameter. If `openTUI` were ever invoked from a different context, the implicit `openCmd` reference would silently use stale or empty context.

**Solution**: Add `*cobra.Command` as the first parameter of `openTUIFunc` and `openTUI`. Pass `cmd` explicitly from `openCmd.RunE`. Replace `tmuxClient(openCmd)` with `tmuxClient(cmd)` inside `openTUI`.

**Outcome**: The cobra.Command dependency is explicit in the function signature. No implicit package-level state access for context.

**Do**:
1. Change the `openTUIFunc` variable type from `func(string, []string, bool) error` to `func(*cobra.Command, string, []string, bool) error`.
2. Change the `openTUI` function signature to `func openTUI(cmd *cobra.Command, initialFilter string, command []string, serverStarted bool) error`.
3. Inside `openTUI`, replace `tmuxClient(openCmd)` with `tmuxClient(cmd)`.
4. In `openCmd.RunE`, update the call to `openTUIFunc` to pass `cmd` as the first argument.
5. In the `init()` function where `openTUIFunc = openTUI` is set, confirm the assignment still compiles (it will, since both signatures now match).
6. In test files, update any test overrides of `openTUIFunc` to accept the new `*cobra.Command` first parameter.
7. Run `go vet ./cmd/...` and `go test ./cmd/...`.

**Acceptance Criteria**:
- `openTUI` no longer references the package-level `openCmd` variable
- `openTUIFunc` signature includes `*cobra.Command` as its first parameter
- `openCmd.RunE` passes `cmd` explicitly when calling `openTUIFunc`
- All tests pass: `go test ./cmd/...`

**Tests**:
- Existing tests for the open command pass (no new tests needed — this is a signature refactor that preserves behavior)

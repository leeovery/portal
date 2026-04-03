---
phase: 3
phase_name: Consumer Migration to Structural Keys
total: 5
---

## resume-hooks-lost-on-server-restart-3-1 | pending

### Task 1: Migrate hooks list to Structural Keys

**Problem**: The `hooks list` command in `cmd/hooks.go` (line 69) references `h.PaneID` which no longer exists after Phase 2 renamed the `Hook` struct field to `Key`. The command won't compile. Additionally, all `TestHooksListCommand` tests in `cmd/hooks_test.go` use pane ID values (`%3`, `%7`, `%1`) in their test data and assertions, which don't reflect the new structural key model.

**Solution**: Update `cmd/hooks.go` line 69 to use `h.Key` instead of `h.PaneID`. Update all test data in `TestHooksListCommand` from pane IDs to structural key format (`session:window.pane`). No logic changes needed — only the field reference and test values change.

**Outcome**: `hooks list` compiles and displays structural keys in its tab-separated output. All `TestHooksListCommand` tests pass with structural key values.

**Do**:
- In `cmd/hooks.go`, line 69: change `h.PaneID` to `h.Key` in the `fmt.Fprintf` call:
  ```go
  if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", h.Key, h.Event, h.Command); err != nil {
  ```
- In `cmd/hooks_test.go`, update `TestHooksListCommand` tests:
  - `"outputs hooks in tab-separated format"` (line 14): change JSON key from `"%3"` to `"my-session:0.0"`. Change `want` from `"%3\ton-resume\tclaude --resume abc123\n"` to `"my-session:0.0\ton-resume\tclaude --resume abc123\n"`
  - `"outputs hooks sorted by pane ID"` (line 84): rename test to `"outputs hooks sorted by key"`. Change JSON keys from `"%7"`, `"%3"`, `"%1"` to `"proj-b:0.0"`, `"proj-a:0.0"`, `"other:0.0"`. Update commands to match. Change `want` to the expected sorted output — structural keys sort lexicographically: `"other:0.0"` < `"proj-a:0.0"` < `"proj-b:0.0"`. The `want` string becomes `"other:0.0\ton-resume\tnpm start\nproj-a:0.0\ton-resume\tclaude --resume abc123\nproj-b:0.0\ton-resume\tclaude --resume def456\n"`
  - Other test cases (`"produces empty output when no hooks registered"`, `"produces empty output when hooks file does not exist"`, `"hooks bypasses tmux bootstrap"`, `"accepts no arguments"`) require no changes — they don't use hook data with pane ID values

**Acceptance Criteria**:
- [ ] `cmd/hooks.go` references `h.Key` instead of `h.PaneID`
- [ ] `hooks list` output displays structural keys in the first column
- [ ] All `TestHooksListCommand` tests pass with structural key values
- [ ] `go test ./cmd -run TestHooksList` passes

**Tests**:
- `"outputs hooks in tab-separated format"` — verifies output is `"my-session:0.0\ton-resume\tclaude --resume abc123\n"`
- `"produces empty output when no hooks registered"` — unchanged, verifies empty hooks produce no output
- `"produces empty output when hooks file does not exist"` — unchanged, verifies missing file produces no output
- `"outputs hooks sorted by key"` — verifies structural keys are sorted lexicographically: `other:0.0`, `proj-a:0.0`, `proj-b:0.0`
- `"hooks bypasses tmux bootstrap"` — unchanged, verifies skip of tmux check
- `"accepts no arguments"` — unchanged, verifies extra args rejected

**Edge Cases**: None specific to this task. Structural key format edge cases (colons, dots) are handled at the store/tmux layer (Phase 2). The list command treats keys as opaque strings.

**Context**:
> Phase 2 Task 3 renamed `Hook.PaneID` to `Hook.Key`. This task is the consumer-side update that restores compilation and aligns test data with the new key format. The specification states: "The hooks list CLI output displays structural keys instead of pane IDs."

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Component Changes" section on Hook listing

## resume-hooks-lost-on-server-restart-3-2 | pending

### Task 2: Add Key Resolver and Migrate hooks set

**Problem**: The `hooks set` command in `cmd/hooks.go` uses `requireTmuxPane()` to read `$TMUX_PANE` (a pane ID like `%3`) and passes it directly as the hook storage key and to `MarkerName`. After Phase 2, hooks are keyed by structural keys (`session:window.pane`). The command needs to resolve the pane ID to a structural key before storing the hook or setting the volatile marker. The `HooksDeps` struct also needs to support injecting a key resolver for testability.

**Solution**: Add a `StructuralKeyResolver` interface to `cmd/hooks.go` with a single `ResolveStructuralKey(paneID string) (string, error)` method. Add it to `HooksDeps`. In `hooksSetCmd`, after reading `$TMUX_PANE`, resolve it to a structural key using either the injected resolver (test) or a real `tmux.Client` (production). Use the resolved structural key for `store.Set` and `hooks.MarkerName`. Update all `TestHooksSetCommand` tests to inject a mock resolver and assert structural key values in the hooks file and marker calls.

**Outcome**: `hooks set` resolves the current pane's structural key via tmux and stores hooks under that key. The volatile marker uses the structural key format. Tests verify end-to-end flow with mocked resolution. Resolution failures produce a user-facing error.

**Do**:
- In `cmd/hooks.go`, add a new interface above `HooksDeps`:
  ```go
  // StructuralKeyResolver resolves a tmux pane ID to its structural key.
  type StructuralKeyResolver interface {
      ResolveStructuralKey(paneID string) (string, error)
  }
  ```
- Add a `KeyResolver` field to `HooksDeps`:
  ```go
  type HooksDeps struct {
      OptionSetter  ServerOptionSetter
      OptionDeleter ServerOptionDeleter
      KeyResolver   StructuralKeyResolver
  }
  ```
- In `hooksSetCmd.RunE`, after the `requireTmuxPane()` call, add structural key resolution:
  ```go
  paneID, err := requireTmuxPane()
  if err != nil {
      return err
  }

  var resolver StructuralKeyResolver
  if hooksDeps != nil && hooksDeps.KeyResolver != nil {
      resolver = hooksDeps.KeyResolver
  } else {
      resolver = buildHooksTmuxClient()
  }
  structuralKey, err := resolver.ResolveStructuralKey(paneID)
  if err != nil {
      return fmt.Errorf("failed to resolve pane position: %w", err)
  }
  ```
  Note: `tmux.Client` already has `ResolveStructuralKey` from Phase 2 Task 2, so `buildHooksTmuxClient()` returns a type that satisfies `StructuralKeyResolver`.
- Replace `paneID` with `structuralKey` in the `store.Set` and `hooks.MarkerName` calls:
  ```go
  if err := store.Set(structuralKey, "on-resume", command); err != nil {
      return err
  }
  // ...
  if err := setter.SetServerOption(hooks.MarkerName(structuralKey), "1"); err != nil {
      return err
  }
  ```
- In `cmd/hooks_test.go`, add a `mockKeyResolver` type:
  ```go
  type mockKeyResolver struct {
      key string
      err error
  }

  func (m *mockKeyResolver) ResolveStructuralKey(paneID string) (string, error) {
      if m.err != nil {
          return "", m.err
      }
      return m.key, nil
  }
  ```
- Update all `TestHooksSetCommand` tests:
  - `"sets hook and volatile marker for current pane"`: inject `mockKeyResolver{key: "my-session:0.0"}` in `hooksDeps.KeyResolver`. Keep `t.Setenv("TMUX_PANE", "%3")`. Assert hooks file contains key `"my-session:0.0"` (not `"%3"`). Assert marker is `"@portal-active-my-session:0.0"` (not `"@portal-active-%3"`)
  - `"reads pane ID from TMUX_PANE environment variable"`: inject `mockKeyResolver{key: "my-session:1.0"}` with `TMUX_PANE=%99`. Assert hooks file contains `"my-session:1.0"`. Assert marker is `"@portal-active-my-session:1.0"`
  - `"returns error when TMUX_PANE is not set"`: no key resolver needed — `requireTmuxPane` fails first. Keep test as-is (add `KeyResolver` to `hooksDeps` for consistency, but it won't be reached)
  - `"returns error when on-resume flag is not provided"`: same — flag validation fails first. Keep as-is
  - `"overwrites existing hook for same pane idempotently"`: inject `mockKeyResolver{key: "my-session:0.0"}` with `TMUX_PANE=%3`. Assert hooks file has `"my-session:0.0"` with the overwritten command. Assert 2 marker calls both using `"@portal-active-my-session:0.0"`
  - `"writes correct JSON structure to hooks file"`: inject `mockKeyResolver{key: "my-session:0.0"}`. Assert JSON has key `"my-session:0.0"` (not `"%3"`)
  - `"sets volatile marker with correct option name"`: inject `mockKeyResolver{key: "proj-abc:0.2"}` with `TMUX_PANE=%7`. Assert marker name is `"@portal-active-proj-abc:0.2"`
- Add a new test case `"returns error when ResolveStructuralKey fails"`:
  - `t.Setenv("TMUX_PANE", "%3")` — pane ID is valid
  - Inject `mockKeyResolver{err: fmt.Errorf("can't find pane: %%3")}` in `hooksDeps.KeyResolver`
  - Inject `mockOptionSetter{}` in `hooksDeps.OptionSetter`
  - Execute `hooks set --on-resume "some-cmd"`
  - Assert error is returned containing `"failed to resolve pane position"`
  - Assert hooks file was NOT created (no side effects)
  - Assert 0 `SetServerOption` calls (no side effects)

**Acceptance Criteria**:
- [ ] `StructuralKeyResolver` interface exists in `cmd/hooks.go`
- [ ] `HooksDeps` has a `KeyResolver` field
- [ ] `hooks set` resolves `$TMUX_PANE` to a structural key before storing
- [ ] Hook is stored under the structural key (not the pane ID)
- [ ] Volatile marker uses structural key format: `@portal-active-session:window.pane`
- [ ] `ResolveStructuralKey` failure returns a user-facing error containing "failed to resolve pane position"
- [ ] No side effects (no file write, no marker set) when resolution fails
- [ ] All `TestHooksSetCommand` tests pass
- [ ] `go test ./cmd -run TestHooksSet` passes

**Tests**:
- `"sets hook and volatile marker for current pane"` — verifies structural key `"my-session:0.0"` in hooks file and marker
- `"reads pane ID from TMUX_PANE environment variable"` — verifies resolver receives pane ID and structural key is used for storage
- `"returns error when TMUX_PANE is not set"` — unchanged behavior, resolver not reached
- `"returns error when on-resume flag is not provided"` — unchanged behavior
- `"overwrites existing hook for same pane idempotently"` — verifies both writes use the structural key
- `"writes correct JSON structure to hooks file"` — verifies JSON keys are structural keys
- `"sets volatile marker with correct option name"` — verifies marker format `@portal-active-proj-abc:0.2`
- `"returns error when ResolveStructuralKey fails"` — verifies user-facing error, no side effects

**Edge Cases**:
- `ResolveStructuralKey` failure (e.g., tmux server crash between `requireTmuxPane` and resolution): returns a wrapped error `"failed to resolve pane position: ..."` to the user. No hooks file or marker is created.

**Context**:
> The specification states: "Instead of using $TMUX_PANE as the key, query tmux for the current pane's session name, window index, and pane index. Build the structural key session_name:window_index.pane_index and use it as the hook storage key."
>
> Phase 2 Task 2 added `ResolveStructuralKey(paneID string) (string, error)` to `tmux.Client`. The production code path uses `buildHooksTmuxClient()` which returns a `*tmux.Client` — this type satisfies `StructuralKeyResolver` because Phase 2 added the method. Tests inject a `mockKeyResolver` to avoid tmux dependencies.
>
> The `requireTmuxPane()` function is kept as-is. It validates that `$TMUX_PANE` is set (the user is inside tmux). The pane ID it returns is then passed to the resolver. This two-step approach preserves the existing "must be run from inside a tmux pane" error for users outside tmux.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Component Changes" section on Hook registration, "Pane querying approach" in Design Decisions

## resume-hooks-lost-on-server-restart-3-3 | pending

### Task 3: Migrate hooks rm to Structural Keys

**Problem**: The `hooks rm` command in `cmd/hooks.go` uses `requireTmuxPane()` to read `$TMUX_PANE` and passes the raw pane ID to `store.Remove` and `hooks.MarkerName`. After Phase 2, hooks are keyed by structural keys. The rm command needs the same pane-ID-to-structural-key resolution that Task 2 added to `hooks set`. Without this, `hooks rm` would try to remove a key like `%3` from a store that contains keys like `my-session:0.0`, resulting in a silent no-op.

**Solution**: In `hooksRmCmd.RunE`, after reading `$TMUX_PANE`, resolve it to a structural key using the same `StructuralKeyResolver` interface and `HooksDeps.KeyResolver` field introduced in Task 2. Use the resolved structural key for `store.Remove` and `hooks.MarkerName`. Update all `TestHooksRmCommand` tests to inject a `mockKeyResolver` and assert structural key values. All test data in hooks files also changes from pane IDs to structural keys.

**Outcome**: `hooks rm` resolves the current pane's structural key and removes the correct hook entry and volatile marker. Resolution failures produce a user-facing error. All `TestHooksRmCommand` tests pass with structural key values.

**Do**:
- In `cmd/hooks.go`, in `hooksRmCmd.RunE`, after the `requireTmuxPane()` call, add structural key resolution (same pattern as `hooksSetCmd`):
  ```go
  paneID, err := requireTmuxPane()
  if err != nil {
      return err
  }

  var resolver StructuralKeyResolver
  if hooksDeps != nil && hooksDeps.KeyResolver != nil {
      resolver = hooksDeps.KeyResolver
  } else {
      resolver = buildHooksTmuxClient()
  }
  structuralKey, err := resolver.ResolveStructuralKey(paneID)
  if err != nil {
      return fmt.Errorf("failed to resolve pane position: %w", err)
  }
  ```
- Replace `paneID` with `structuralKey` in the `store.Remove` and `hooks.MarkerName` calls:
  ```go
  if err := store.Remove(structuralKey, "on-resume"); err != nil {
      return err
  }
  // ...
  if err := deleter.DeleteServerOption(hooks.MarkerName(structuralKey)); err != nil {
      return err
  }
  ```
- In `cmd/hooks_test.go`, update all `TestHooksRmCommand` tests:
  - `"removes hook and volatile marker for current pane"` (line 404): inject `mockKeyResolver{key: "my-session:0.0"}` in `hooksDeps.KeyResolver`. Keep `TMUX_PANE=%3`. Change seed data key from `"%3"` to `"my-session:0.0"`. Assert hooks file no longer contains `"my-session:0.0"`. Assert marker deletion is `"@portal-active-my-session:0.0"`
  - `"reads pane ID from TMUX_PANE environment variable"` (line 442): inject `mockKeyResolver{key: "other-proj:1.0"}` with `TMUX_PANE=%42`. Change seed data key from `"%42"` to `"other-proj:1.0"`. Assert `"other-proj:1.0"` removed from hooks file. Assert marker deletion is `"@portal-active-other-proj:1.0"`
  - `"returns error when TMUX_PANE is not set"` (line 479): no changes needed — `requireTmuxPane` fails first. Add `KeyResolver` to `hooksDeps` for consistency but it won't be reached
  - `"returns error when on-resume flag is not provided"` (line 507): same — flag fails first. Keep as-is
  - `"silent no-op when no hook exists for pane"` (line 530): inject `mockKeyResolver{key: "nonexistent:9.9"}` with `TMUX_PANE=%99`. The empty hooks file means Remove is a no-op. Verify no error and no output
  - `"removes correct JSON entry from hooks file"` (line 558): inject `mockKeyResolver{key: "my-session:0.0"}` with `TMUX_PANE=%3`. Change seed data from `{"%3": ..., "%7": ...}` to `{"my-session:0.0": ..., "my-session:0.1": ...}`. Assert `"my-session:0.0"` removed, `"my-session:0.1"` remains
  - `"deletes volatile marker with correct option name"` (line 595): inject `mockKeyResolver{key: "proj-abc:0.2"}` with `TMUX_PANE=%7`. Change seed data key from `"%7"` to `"proj-abc:0.2"`. Assert marker deletion is `"@portal-active-proj-abc:0.2"`
  - `"cleans up pane key when last event removed"` (line 626): inject `mockKeyResolver{key: "my-session:0.1"}` with `TMUX_PANE=%5`. Change seed data key from `"%5"` to `"my-session:0.1"`. Assert key removed from JSON and file is empty
- Add a new test case `"returns error when ResolveStructuralKey fails"`:
  - `t.Setenv("TMUX_PANE", "%3")` — pane ID is valid
  - Seed hooks file with `{"my-session:0.0": {"on-resume": "some-cmd"}}` (to verify no modification)
  - Inject `mockKeyResolver{err: fmt.Errorf("can't find pane: %%3")}` in `hooksDeps.KeyResolver`
  - Inject `mockOptionDeleter{}` in `hooksDeps.OptionDeleter`
  - Execute `hooks rm --on-resume`
  - Assert error returned containing `"failed to resolve pane position"`
  - Assert hooks file is unchanged (hook still present)
  - Assert 0 `DeleteServerOption` calls

**Acceptance Criteria**:
- [ ] `hooks rm` resolves `$TMUX_PANE` to a structural key before removing
- [ ] Hook is removed by structural key (not pane ID)
- [ ] Volatile marker deletion uses structural key format
- [ ] `ResolveStructuralKey` failure returns a user-facing error containing "failed to resolve pane position"
- [ ] No side effects (no file modification, no marker deletion) when resolution fails
- [ ] All `TestHooksRmCommand` tests pass
- [ ] `go test ./cmd -run TestHooksRm` passes

**Tests**:
- `"removes hook and volatile marker for current pane"` — verifies structural key `"my-session:0.0"` removed from file and marker
- `"reads pane ID from TMUX_PANE environment variable"` — verifies resolution and removal using structural key
- `"returns error when TMUX_PANE is not set"` — unchanged behavior, resolver not reached
- `"returns error when on-resume flag is not provided"` — unchanged behavior
- `"silent no-op when no hook exists for pane"` — verifies no error when resolved key has no hook entry
- `"removes correct JSON entry from hooks file"` — verifies only the target structural key is removed, others remain
- `"deletes volatile marker with correct option name"` — verifies marker format `@portal-active-proj-abc:0.2`
- `"cleans up pane key when last event removed"` — verifies JSON key removed entirely when last event deleted
- `"returns error when ResolveStructuralKey fails"` — verifies user-facing error, hooks file unchanged, no marker deletion

**Edge Cases**:
- `ResolveStructuralKey` failure: returns a wrapped error `"failed to resolve pane position: ..."` to the user. The hooks file and volatile markers are not modified.

**Context**:
> The specification states: "Update to resolve the current pane's structural key instead of using $TMUX_PANE. Remove the hook entry and volatile marker using the structural key."
>
> This task reuses the `StructuralKeyResolver` interface and `HooksDeps.KeyResolver` field introduced in Task 2. The `mockKeyResolver` type from Task 2's tests is also reused. The resolution pattern is identical to `hooks set` — `requireTmuxPane` validates presence, then `ResolveStructuralKey` translates to the structural key.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Component Changes" section on Hook removal, "Pane querying approach" in Design Decisions

## resume-hooks-lost-on-server-restart-3-4 | pending

### Task 4: Migrate clean Command Tests to Structural Keys

**Problem**: All `TestCleanCommand` tests in `cmd/clean_test.go` that involve hook cleanup use pane ID values (`%1`, `%3`, `%5`, `%9`) in their hooks file seed data and `mockCleanPaneLister` return values. After Phase 2, `ListAllPanes` returns structural keys and `CleanStale` cross-references structural keys. The clean command production code (`cmd/clean.go`) requires no changes — it already passes through `ListAllPanes` results to `CleanStale` without interpreting the values. Only the tests need updating to use structural key values for consistency and accuracy.

**Solution**: Update all pane-related test data in `TestCleanCommand` from pane IDs to structural keys. Change `mockCleanPaneLister` return values from `[]string{"%1"}` to `[]string{"my-session:0.0"}`. Change hooks file seed data from `{"%1": ..., "%5": ...}` to `{"my-session:0.0": ..., "other-session:0.0": ...}`. Update output assertions from `"Removed stale hook: %5\n"` to `"Removed stale hook: other-session:0.0\n"`. No production code changes.

**Outcome**: All `TestCleanCommand` tests pass with structural key values. The clean command code is verified to work correctly with the new key format end-to-end (albeit with mocked tmux).

**Do**:
- In `cmd/clean_test.go`, update the hook-related test cases:
  - `"removes stale hooks and prints removal messages"` (line 283):
    - Change hooks file seed data from `{"%1": {"on-resume": "cmd1"}, "%5": {"on-resume": "cmd5"}}` to `{"my-session:0.0": {"on-resume": "cmd1"}, "other-session:0.0": {"on-resume": "cmd5"}}`
    - Change `mockCleanPaneLister` from `{panes: []string{"%1"}}` to `{panes: []string{"my-session:0.0"}}`
    - Change `want` from `"Removed stale hook: %5\n"` to `"Removed stale hook: other-session:0.0\n"`
    - Update assertions: verify `"my-session:0.0"` remains in hooks file (not `"%1"`), verify `"other-session:0.0"` was removed (not `"%5"`)
  - `"no tmux server running skips hook cleanup preserving existing hooks"` (line 326):
    - Change hooks file seed data from `{"%3": {"on-resume": "some-cmd"}}` to `{"my-session:0.0": {"on-resume": "some-cmd"}}`
    - `mockCleanPaneLister` stays `{panes: []string{}}` — empty list is the point
    - Update assertion: verify `"my-session:0.0"` preserved (not `"%3"`)
  - `"all hooks panes still live produces no hook removal output"` (line 393):
    - Change hooks file seed data from `{"%1": {"on-resume": "cmd1"}, "%3": {"on-resume": "cmd3"}}` to `{"my-session:0.0": {"on-resume": "cmd1"}, "my-session:0.1": {"on-resume": "cmd3"}}`
    - Change `mockCleanPaneLister` from `{panes: []string{"%1", "%3"}}` to `{panes: []string{"my-session:0.0", "my-session:0.1"}}`
  - `"both project and hook removals printed together"` (line 425):
    - Change hooks file seed data from `{"%9": {"on-resume": "cmd9"}}` to `{"stale-session:0.0": {"on-resume": "cmd9"}}`
    - Change `mockCleanPaneLister` from `{panes: []string{"%1"}}` to `{panes: []string{"my-session:0.0"}}`
    - Change `want` hook portion from `"Removed stale hook: %9\n"` to `"Removed stale hook: stale-session:0.0\n"`
  - `"hooks file missing produces no hook removal output"` (line 365): no changes needed — no hooks file is created, so no pane IDs appear
- Tests that only deal with project cleanup (no hook data) require no changes: `"removes stale project and prints removal message"`, `"keeps project with existing directory..."`, `"keeps project with permission error"`, `"no stale projects produces no output"`, `"all projects stale removes all..."`, `"multiple stale projects each printed"`, `"exit code 0 in all cases"`

**Acceptance Criteria**:
- [ ] All hook-related `TestCleanCommand` tests use structural key values (not pane IDs)
- [ ] `mockCleanPaneLister` returns structural keys
- [ ] Hooks file seed data uses structural keys
- [ ] Output assertions use structural keys (e.g., `"Removed stale hook: other-session:0.0\n"`)
- [ ] No production code changes to `cmd/clean.go`
- [ ] `go test ./cmd -run TestClean` passes

**Tests**:
- `"removes stale hooks and prints removal messages"` — verifies `"other-session:0.0"` is removed, `"my-session:0.0"` remains, output message uses structural key
- `"no tmux server running skips hook cleanup preserving existing hooks"` — verifies hooks with structural keys are preserved when pane list is empty
- `"hooks file missing produces no hook removal output"` — unchanged, no hook data involved
- `"all hooks panes still live produces no hook removal output"` — verifies no removal when all structural keys match live panes
- `"both project and hook removals printed together"` — verifies output includes both project and hook removal messages with structural keys

**Edge Cases**: None specific to this task. The clean command's production code is unchanged — it passes through values opaquely. The empty-pane guard (from Phase 1 and the existing `cmd/clean.go` guard at line 77-80) ensures hooks are not deleted when no tmux server is running, regardless of key format.

**Context**:
> The specification states: "Update to use structural key model for cleanup. The existing empty-pane guard remains."
>
> The clean command production code (`cmd/clean.go`) already works with structural keys because it does not interpret the values from `ListAllPanes` — it just passes them to `CleanStale`. The `CleanStale` function was updated in Phase 2 Task 3 to work with structural keys. This task only updates the test data to match the new key format, ensuring the tests accurately reflect production behavior.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Component Changes" section on Clean command

## resume-hooks-lost-on-server-restart-3-5 | pending

### Task 5: Add Multi-Pane and Graceful No-Op Acceptance Tests

**Problem**: The specification requires tests for two important behavioral scenarios that are not covered by the existing test suite: (1) multi-pane sessions where each pane has its own independent hook entry keyed by a distinct structural position, and (2) graceful no-op behavior when structural keys in the hooks file don't match any live panes (the no-resurrect scenario). These are acceptance-level tests that verify the complete hook execution pipeline works correctly with structural keys in non-trivial configurations.

**Solution**: Add two new test cases to `TestExecuteHooks` in `internal/hooks/executor_test.go`: one that verifies multi-pane hook execution where three panes in the same session each have independent hooks fired via distinct structural keys, and one that verifies graceful no-op when stored hook keys reference structural positions that don't exist in the current tmux session (simulating a post-restart scenario without tmux-resurrect).

**Outcome**: Both new tests pass, confirming that the structural key model supports multi-pane sessions and degrades gracefully when keys don't match live panes.

**Do**:
- In `internal/hooks/executor_test.go`, add a new test case inside `TestExecuteHooks`:
  `"executes independent hooks for multiple panes in same session"`:
  - Set up `mockPaneLister` returning three panes for `"proj-abc"`: `["proj-abc:0.0", "proj-abc:0.1", "proj-abc:0.2"]`
  - Set up `mockAllPaneLister` returning the same three panes (for cleanup passthrough)
  - Set up `mockHookLoader` with three independent hook entries:
    ```go
    data: map[string]map[string]string{
        "proj-abc:0.0": {"on-resume": "claude --resume aaa"},
        "proj-abc:0.1": {"on-resume": "npm run dev"},
        "proj-abc:0.2": {"on-resume": "claude --resume bbb"},
    }
    ```
  - No pre-existing volatile markers (all markers absent)
  - Call `hooks.ExecuteHooks("proj-abc", tmux, store)`
  - Assert exactly 3 `SendKeys` calls were made
  - Build a map from `sent` entries: verify `"proj-abc:0.0"` received `"claude --resume aaa"`, `"proj-abc:0.1"` received `"npm run dev"`, `"proj-abc:0.2"` received `"claude --resume bbb"`
  - Assert exactly 3 `SetServerOption` calls (one marker per pane)
  - Build a map from `setLog` entries: verify markers `"@portal-active-proj-abc:0.0"`, `"@portal-active-proj-abc:0.1"`, `"@portal-active-proj-abc:0.2"` were all set to `"1"`
  - This test verifies: each pane gets its own hook, no cross-talk between panes, structural keys correctly target individual panes

- Add a second new test case inside `TestExecuteHooks`:
  `"graceful no-op when structural keys do not match live panes"`:
  - Set up `mockPaneLister` returning panes for `"proj-abc"` with new structural positions after restart: `["proj-abc:0.0"]` (only one pane survived/was recreated)
  - Set up `mockAllPaneLister` returning `["proj-abc:0.0"]`
  - Set up `mockHookLoader` with hook entries referencing old structural positions that no longer exist:
    ```go
    data: map[string]map[string]string{
        "proj-abc:0.0": {"on-resume": "claude --resume aaa"},
        "proj-abc:0.1": {"on-resume": "npm run dev"},
        "proj-abc:1.0": {"on-resume": "claude --resume bbb"},
    }
    ```
  - Call `hooks.ExecuteHooks("proj-abc", tmux, store)`
  - Assert exactly 1 `SendKeys` call (only `"proj-abc:0.0"` matches a live pane in the session)
  - Assert the sent command targets `"proj-abc:0.0"` with `"claude --resume aaa"`
  - Assert exactly 1 `SetServerOption` call for `"@portal-active-proj-abc:0.0"`
  - This test verifies: hooks referencing non-existent structural positions are silently skipped (no errors), only the matching pane fires its hook, the system degrades gracefully

- Add a third new test case inside `TestExecuteHooks_Cleanup`:
  `"hooks survive when ListAllPanes returns empty after server restart"`:
  - Set up `mockAllPaneLister` returning empty slice `[]string{}` (simulating post-restart, pre-resurrect)
  - Set up `mockPaneLister` returning panes for `"proj-abc"`: `["proj-abc:0.0"]` (session exists but global list is empty — this simulates the race where ListPanes succeeds for the session being attached to but ListAllPanes hasn't caught up)
  - Set up `mockHookLoader` with hooks:
    ```go
    data: map[string]map[string]string{
        "proj-abc:0.0": {"on-resume": "claude --resume aaa"},
        "proj-abc:0.1": {"on-resume": "npm run dev"},
    }
    ```
  - Set up `mockHookCleaner` — assert it is NOT called (Phase 1 empty-pane guard prevents `CleanStale`)
  - Call `hooks.ExecuteHooks("proj-abc", tmux, store)`
  - Assert `CleanStale` was NOT called (the empty-pane guard in `ExecuteHooks` skips cleanup)
  - Assert 1 `SendKeys` call for `"proj-abc:0.0"` — hook execution still proceeds for the matching pane
  - Assert hook for `"proj-abc:0.1"` was NOT sent (not in session's pane list) but also NOT deleted
  - This test verifies the Phase 1 guard works correctly with structural keys: hooks are preserved through server restarts, and execution proceeds for matching panes

**Acceptance Criteria**:
- [ ] Multi-pane test verifies 3 independent hooks fire for 3 panes in the same session
- [ ] Each pane receives its own command via the correct structural key target
- [ ] Each pane gets its own volatile marker set
- [ ] No-op test verifies hooks with non-matching structural positions are silently skipped
- [ ] Only matching structural keys result in `SendKeys` calls
- [ ] Hook survival test verifies empty `ListAllPanes` does not trigger `CleanStale`
- [ ] Hook execution proceeds for matching panes even when `CleanStale` is skipped
- [ ] No errors surfaced in any scenario
- [ ] `go test ./internal/hooks/...` passes

**Tests**:
- `"executes independent hooks for multiple panes in same session"` — verifies 3 panes each receive their own hook command and volatile marker via structural key targeting
- `"graceful no-op when structural keys do not match live panes"` — verifies hooks referencing non-existent structural positions are silently skipped, only matching pane fires
- `"hooks survive when ListAllPanes returns empty after server restart"` — verifies Phase 1 empty-pane guard prevents `CleanStale`, hooks survive, matching pane still fires

**Edge Cases**:
- Hooks referencing non-existent structural positions: these are silently skipped because they don't appear in the session's `ListPanes` result. No errors, no data loss. The hooks remain in the store for potential future matches (e.g., after tmux-resurrect restores the full session layout).

**Context**:
> The specification's Behavioral Requirements state: "Multi-pane support: Each pane in a session has its own hook entry keyed by its unique structural position. A session with three panes has three independent hook entries." It also states: "Silent operation: Hook execution failures (no matching panes, stale keys) are silent — no errors surfaced to the user."
>
> The specification's Testing Requirements explicitly list: "Add tests for hooks with multiple panes in the same session using structural keys" and "Add test verifying graceful no-op when structural keys don't match any live panes (no-resurrect scenario)."
>
> The specification's Behavioral Requirements also state: "Graceful failure without tmux-resurrect: If resurrect is not installed or not used, the server restarts with no sessions. Hooks remain on disk (empty-pane guard prevents deletion) but no matching structure exists — hooks simply don't fire. No errors, no data loss."

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Behavioral Requirements" section (Multi-pane support, Silent operation, Graceful failure without tmux-resurrect), "Testing Requirements" section

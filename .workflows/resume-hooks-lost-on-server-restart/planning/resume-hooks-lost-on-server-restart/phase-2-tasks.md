---
phase: 2
phase_name: Structural Key Infrastructure and Pane Querying
total: 4
---

## resume-hooks-lost-on-server-restart-2-1 | pending

### Task 1: Update ListPanes and ListAllPanes to Return Structural Keys

**Problem**: `ListPanes` and `ListAllPanes` currently return tmux pane IDs (`%0`, `%1`, etc.) which are ephemeral — they reset on tmux server restart and do not survive tmux-resurrect. Every consumer of these functions (hook execution, stale cleanup) uses pane IDs as identity keys, which breaks after restart.

**Solution**: Change the tmux `-F` format string in both `ListPanes` and `ListAllPanes` from `#{pane_id}` to `#{session_name}:#{window_index}.#{pane_index}`. This produces structural keys (e.g., `my-project-abc:0.0`) that survive tmux-resurrect because they use positional addressing that resurrect preserves. The function signatures stay `[]string` — only the semantic meaning of the returned strings changes.

**Outcome**: Both `ListPanes` and `ListAllPanes` return structural keys instead of pane IDs. All existing tests are updated to expect structural key format. The `parsePaneOutput` helper requires no changes (it just splits lines). The doc comments on both methods reflect the new return type semantics.

**Do**:
- In `internal/tmux/tmux.go`, change `ListPanes` format string from `"#{pane_id}"` to `"#{session_name}:#{window_index}.#{pane_index}"` (line 225)
- In `internal/tmux/tmux.go`, change `ListAllPanes` format string from `"#{pane_id}"` to `"#{session_name}:#{window_index}.#{pane_index}"` (line 236)
- Update the doc comment on `ListPanes` (line 222-224): replace "pane IDs" with "structural keys" and update the example format from `"%N"` to `"session:window.pane"` (e.g., `"my-session:0.0"`)
- Update the doc comment on `ListAllPanes` (line 232-234): same changes — replace "pane IDs" with "structural keys" and update example format
- In `internal/tmux/tmux_test.go`, update `TestListPanes`:
  - `"returns pane IDs for session with multiple panes"` — rename to `"returns structural keys for session with multiple panes"`. Change mock output from `"%0\n%1\n%2"` to `"my-session:0.0\nmy-session:0.1\nmy-session:0.2"`. Change `want` slice accordingly. Change the `wantArgs` format string assertion from `"list-panes -t my-session -F #{pane_id}"` to `"list-panes -t my-session -F #{session_name}:#{window_index}.#{pane_index}"`
  - `"returns empty slice when session has no panes"` — no changes needed (empty output, no format assertion)
  - `"returns error when session does not exist"` — no changes needed (error path)
- In `internal/tmux/tmux_test.go`, update `TestListAllPanes`:
  - `"returns pane IDs across multiple sessions"` — rename to `"returns structural keys across multiple sessions"`. Change mock output from `"%0\n%1\n%5\n%12"` to `"proj-a:0.0\nproj-a:0.1\nproj-b:0.0\nproj-b:1.0"`. Change `want` slice accordingly
  - `"returns empty slice when no tmux server running"` — no changes needed
  - `"returns empty slice when output is empty"` — no changes needed
  - `"calls list-panes with -a flag"` — change mock output from `"%0"` to `"proj-a:0.0"`. Change `wantArgs` from `"list-panes -a -F #{pane_id}"` to `"list-panes -a -F #{session_name}:#{window_index}.#{pane_index}"`
- Add new test case to `TestListPanes`: `"returns structural keys for multi-window multi-pane session"` with mock output `"my-session:0.0\nmy-session:0.1\nmy-session:1.0\nmy-session:1.1\nmy-session:1.2"` — verifies 5 entries with correct window.pane indices
- Add new test case to `TestListPanes`: `"handles session names with colons"` with mock output `"my:project:0.0\nmy:project:0.1"` — the mock returns this verbatim and `parsePaneOutput` just splits on newlines, so verify 2 entries returned as-is (tmux itself handles the colon ambiguity in its format strings; Portal just passes through the output)
- Add new test case to `TestListPanes`: `"handles session names with dots"` with mock output `"my.project.v2:0.0"` — verify 1 entry returned as-is

**Acceptance Criteria**:
- [ ] `ListPanes` uses format string `#{session_name}:#{window_index}.#{pane_index}`
- [ ] `ListAllPanes` uses format string `#{session_name}:#{window_index}.#{pane_index}`
- [ ] Doc comments on both methods describe structural keys, not pane IDs
- [ ] All existing `TestListPanes` tests pass with structural key values
- [ ] All existing `TestListAllPanes` tests pass with structural key values
- [ ] New edge case tests pass (multi-window, colons in names, dots in names)
- [ ] `go test ./internal/tmux/...` passes

**Tests**:
- `"returns structural keys for session with multiple panes"` — verifies ListPanes returns `["my-session:0.0", "my-session:0.1", "my-session:0.2"]` and passes correct format string to tmux
- `"returns empty slice when session has no panes"` — unchanged, verifies empty output still returns `[]string{}`
- `"returns error when session does not exist"` — unchanged, verifies error wrapping
- `"returns structural keys across multiple sessions"` — verifies ListAllPanes returns keys spanning multiple sessions with different window indices
- `"returns empty slice when no tmux server running"` — unchanged, verifies error swallowed
- `"returns empty slice when output is empty"` — unchanged
- `"calls list-panes with -a flag"` — verifies ListAllPanes uses `-a` flag with new format string
- `"returns structural keys for multi-window multi-pane session"` — verifies 5 panes across 2 windows (window 0 has 2 panes, window 1 has 3 panes)
- `"handles session names with colons"` — verifies tmux output with colons in session name passes through correctly
- `"handles session names with dots"` — verifies tmux output with dots in session name passes through correctly

**Edge Cases**:
- Session names containing colons (e.g., `my:project`) — tmux format strings produce output like `my:project:0.0`. Portal does not parse these keys, only stores and compares them as opaque strings, so colons in session names are safe. The test verifies pass-through behavior.
- Session names containing dots (e.g., `my.project.v2`) — same reasoning, dot in the name does not conflict with the window.pane separator because Portal treats keys as opaque strings.
- Multi-window multi-pane output — ensures the format correctly distinguishes `session:0.0` from `session:1.0` (different windows) and `session:0.0` from `session:0.1` (different panes in same window).

**Context**:
> The specification states: "ListPanes and ListAllPanes return structural keys ([]string) instead of pane IDs. The underlying tmux commands change their -F format string to output #{session_name}:#{window_index}.#{pane_index} instead of #{pane_id}. Function signatures remain []string — the semantic meaning of the strings changes from pane IDs to structural keys."
>
> The `parsePaneOutput` helper (line 205-220 of tmux.go) simply splits on newlines and trims whitespace. It is agnostic to the content format, so it requires no changes. The structural key format `session:window.pane` is exactly what tmux-resurrect uses for targeting panes during restore.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Pane querying approach" in Design Decisions, "Component Changes" section on Pane querying

## resume-hooks-lost-on-server-restart-2-2 | pending

### Task 2: Add ResolveStructuralKey Method to tmux.Client

**Problem**: Hook registration (`hooks set`) and removal (`hooks rm`) currently use `$TMUX_PANE` (e.g., `%3`) as the hook storage key. After Task 1, hook storage uses structural keys (`session:window.pane`). There is no way to convert the current pane's ephemeral pane ID into the structural key needed for registration and removal.

**Solution**: Add a `ResolveStructuralKey(paneID string) (string, error)` method to `tmux.Client` that runs `tmux display-message -p -t <paneID> "#{session_name}:#{window_index}.#{pane_index}"` and returns the structural key. This is the same format used by `ListPanes`/`ListAllPanes` (Task 1) and can be used by consumers (Phase 3) to translate `$TMUX_PANE` into the correct storage key.

**Outcome**: `tmux.Client` has a new `ResolveStructuralKey` method. It uses `display-message` to query the structural position of any pane by its ID. Tests cover the happy path, invalid pane ID error, and tmux command failure.

**Do**:
- In `internal/tmux/tmux.go`, add the following method after `CurrentSessionName` (after line 155):
  ```
  // ResolveStructuralKey returns the structural key (session:window.pane) for the
  // given pane ID. The pane ID is typically $TMUX_PANE (e.g., "%3").
  func (c *Client) ResolveStructuralKey(paneID string) (string, error) {
      output, err := c.cmd.Run("display-message", "-p", "-t", paneID, "#{session_name}:#{window_index}.#{pane_index}")
      if err != nil {
          return "", fmt.Errorf("failed to resolve structural key for pane %q: %w", paneID, err)
      }
      return output, nil
  }
  ```
- In `internal/tmux/tmux_test.go`, add `TestResolveStructuralKey` with these test cases:
  - `"returns structural key for valid pane ID"` — mock returns `"my-project-abc:0.0"` for input `"%3"`. Assert the returned value is `"my-project-abc:0.0"`. Assert the mock was called with args `["display-message", "-p", "-t", "%3", "#{session_name}:#{window_index}.#{pane_index}"]`
  - `"returns error for invalid pane ID"` — mock returns error `"can't find pane: %99"`. Assert error is returned and contains `"failed to resolve structural key"` and `"%99"`
  - `"returns error when tmux command fails"` — mock returns error `"no server running"`. Assert error is returned and wraps the original

**Acceptance Criteria**:
- [ ] `ResolveStructuralKey` method exists on `tmux.Client`
- [ ] Method runs `tmux display-message -p -t <paneID> "#{session_name}:#{window_index}.#{pane_index}"`
- [ ] Returns the structural key string on success
- [ ] Returns a wrapped error on failure with the pane ID in the message
- [ ] All three test cases pass
- [ ] `go test ./internal/tmux/...` passes

**Tests**:
- `"returns structural key for valid pane ID"` — verifies correct tmux command args and successful return of `"my-project-abc:0.0"`
- `"returns error for invalid pane ID"` — verifies error wrapping when pane ID does not exist, error message includes `"failed to resolve structural key"` and the pane ID
- `"returns error when tmux command fails"` — verifies general tmux failures are wrapped

**Edge Cases**:
- Invalid pane ID (e.g., `%99` when only `%0`-`%2` exist) — tmux returns an error, which is wrapped and returned. Consumers (Phase 3) handle this at the command level.
- tmux command failure (e.g., no server running) — same error wrapping pattern used by other Client methods.

**Context**:
> The specification states: "For registration and removal, a new tmux.Client method resolves the current pane's structural key from $TMUX_PANE (e.g., tmux display-message -p -t "$TMUX_PANE" "#{session_name}:#{window_index}.#{pane_index}")."
>
> This method follows the same pattern as `CurrentSessionName` (line 149-155 of tmux.go), which also uses `display-message -p`. The key difference is the `-t paneID` targeting flag, which tells tmux to resolve the format for a specific pane rather than the current one.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Pane querying approach" in Design Decisions, "Component Changes" section on Pane querying

## resume-hooks-lost-on-server-restart-2-3 | pending

### Task 3: Update Hook Struct and Store Semantics for Structural Keys

**Problem**: The `Hook` struct in `internal/hooks/store.go` has a field named `PaneID` and the `List()` method populates it with pane IDs. The `CleanStale` method's parameter is named `livePaneIDs`. All store tests use pane ID values (`%0`, `%3`, `%7`). With the shift to structural keys, these names and test values are misleading and will cause confusion for future maintainers.

**Solution**: Rename `Hook.PaneID` to `Hook.Key`, rename `CleanStale`'s parameter from `livePaneIDs` to `liveKeys`, update all variable names and comments in `store.go` to use "key" instead of "pane ID", and update all test values to use structural key format strings (e.g., `"my-session:0.0"` instead of `"%3"`). The actual logic is unchanged — it's a pure rename and test value update.

**Outcome**: The store code and tests consistently use "key" / "structural key" terminology. All test values use the `session:window.pane` format. The `Hook` struct field is `Key` instead of `PaneID`. No behavioral changes.

**Do**:
- In `internal/hooks/store.go`:
  - Rename `Hook.PaneID` field to `Hook.Key` (line 16)
  - In `List()` (line 100-125): rename loop variable `paneID` to `key`, set `Hook{Key: key, ...}` instead of `Hook{PaneID: paneID, ...}`. Update the sort comparison from `list[i].PaneID` / `list[j].PaneID` to `list[i].Key` / `list[j].Key`
  - In `CleanStale` (line 130-159): rename parameter from `livePaneIDs` to `liveKeys`. Rename local variable `paneID` to `key` in the range loop. Update doc comment from "panes not present in livePaneIDs" to "keys not present in liveKeys" and "removed pane IDs" to "removed keys"
  - In `Set` (line 66-78): rename parameter from `paneID` to `key`. Update the `h[key]` references. Update doc comment.
  - In `Remove` (line 80-97): rename parameter from `paneID` to `key`. Update references. Update doc comment.
  - In `Get` (line 163-175): rename parameter from `paneID` to `key`. Update references. Update doc comment.
  - Update the `hooksFile` type comment (line 23) from `map[paneID]map[event]command` to `map[key]map[event]command`
- In `internal/hooks/store_test.go`, update all test values from pane IDs to structural keys:
  - Replace `"%3"` with `"my-session:0.0"` throughout
  - Replace `"%7"` with `"my-session:0.1"` throughout
  - Replace `"%5"` with `"other-session:0.0"` throughout
  - Replace `"%9"` with `"other-session:0.1"` throughout
  - Replace `"%99"` with `"nonexistent:9.9"` throughout
  - In `TestLoad` `"returns hooks from valid JSON file"`: update JSON content from `{"%3":{"on-resume":"claude --resume abc123"},"%7":{"on-resume":"claude --resume def456"}}` to `{"my-session:0.0":{"on-resume":"claude --resume abc123"},"my-session:0.1":{"on-resume":"claude --resume def456"}}`. Update assertions to use new keys.
  - In `TestList` `"returns hooks sorted by pane ID then event"`: rename test to `"returns hooks sorted by key then event"`. Update JSON content from `{"%7":{"on-resume":"cmd7"},"%3":{"on-start":"cmd3s","on-resume":"cmd3r"}}` to `{"my-session:0.1":{"on-resume":"cmd7"},"my-session:0.0":{"on-start":"cmd3s","on-resume":"cmd3r"}}`. Update `wantPanes` to `wantKeys` with values `["my-session:0.0", "my-session:0.0", "my-session:0.1"]`. Change all assertions from `.PaneID` to `.Key`.
  - Update all other assertions that reference `.PaneID` to use `.Key`
  - Update error message strings that mention `%3`, `%7` etc. in `t.Error` calls to use the new structural key values

**Acceptance Criteria**:
- [ ] `Hook` struct field is `Key string` (not `PaneID`)
- [ ] `CleanStale` parameter is named `liveKeys`
- [ ] `Set`, `Remove`, `Get` parameters are named `key`
- [ ] All doc comments use "key" / "structural key" terminology
- [ ] All store tests use structural key format values (`session:window.pane`)
- [ ] No behavioral changes — all logic remains identical
- [ ] `go test ./internal/hooks/...` passes (store tests only; executor tests are Task 4)

**Tests**:
- All existing `TestLoad` tests pass with structural key values in JSON
- All existing `TestSave` tests pass with structural key values
- All existing `TestSet` tests pass with structural key values
- All existing `TestRemove` tests pass with structural key values
- `"returns hooks sorted by key then event"` — verifies sort order uses `Key` field with structural key values
- All existing `TestGet` tests pass with structural key values
- All existing `TestCleanStale` tests pass with structural key values

**Edge Cases**:
- No edge cases specific to this task — it is a pure rename and test value update with no behavioral changes. The structural key format edge cases (colons, dots in names) are covered by Task 1's tests at the tmux layer; the store treats keys as opaque strings.

**Context**:
> The specification states: "The Hook struct's PaneID field becomes a structural key field. List() populates it with structural keys. The hooks list CLI output displays structural keys instead of pane IDs."
>
> The specification also states: "CleanStale(liveKeys []string) — same signature, parameter renamed. Callers pass structural keys from ListAllPanes. The function cross-references hook map keys against liveKeys — semantics unchanged, just the key format changes."
>
> Note: The `cmd/hooks.go` file references `h.PaneID` in the list command output (line 69). That consumer migration is Phase 3 scope, not this task. After this task, `cmd/hooks.go` will fail to compile until Phase 3 updates it. This is acceptable because Phase 2 is infrastructure — Phase 3 migrates consumers.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Storage Model" section, "Component Changes" section on Hook storage, "CleanStale contract" in Design Decisions, "Hook listing" in Component Changes

## resume-hooks-lost-on-server-restart-2-4 | pending

### Task 4: Update MarkerName Format and Executor Tests to Structural Keys

**Problem**: `MarkerName` in `internal/hooks/executor.go` currently formats volatile markers as `@portal-active-%paneID` (e.g., `@portal-active-%3`). With structural keys, markers must use the new format `@portal-active-{structural_key}` (e.g., `@portal-active-my-project-abc:0.0`). All executor tests use pane ID values in mock data and assertions, which must be updated to structural keys for consistency.

**Solution**: Update `MarkerName` to accept a structural key (the parameter name changes from `paneID` to `key` but the format string logic is identical — `fmt.Sprintf("@portal-active-%s", key)`). Update all executor test mock values from pane IDs to structural keys and update all marker assertions to match. The `ExecuteHooks` function itself requires no logic changes because it already iterates hook map keys and passes them to `MarkerName` and `SendKeys` — when the keys become structural keys, everything flows through naturally.

**Outcome**: `MarkerName` parameter is named `key` and its doc comment describes structural keys. All executor tests use structural key values. Marker assertions verify the `@portal-active-session:window.pane` format. The existing "no tmux server running skips cleanup gracefully" test is updated per specification to assert `CleanStale` is NOT called when `livePanes` is empty (the empty-pane guard from Phase 1).

**Do**:
- In `internal/hooks/executor.go`:
  - Rename `MarkerName` parameter from `paneID` to `key` (line 53). Update the doc comment to say "structural key" instead of "pane"
  - Update `PaneLister` interface doc comment (line 6): change "pane IDs" to "structural keys"
  - Update `KeySender` interface doc comment (line 11): change "tmux pane" to "tmux target" — `SendKeys` now receives structural keys as targets, which tmux natively accepts
  - Update `AllPaneLister` interface doc comment (line 27): change "pane IDs" to "structural keys"
  - Update `HookCleaner` interface doc comment (line 32): change "panes that no longer exist" to "keys that no longer exist"
  - Update `ExecuteHooks` doc comment (line 57-63): replace "pane" references with "key" / "structural key" where appropriate. The function's actual code does not change — it already uses opaque string keys throughout
- In `internal/hooks/executor_test.go`, update all mock values from pane IDs to structural keys:
  - Replace `"%3"` with `"my-session:0.0"` throughout all test cases
  - Replace `"%5"` with `"my-session:0.1"` throughout
  - Replace `"%7"` with `"my-session:0.2"` throughout (or `"other-session:0.0"` where it represents a pane in a different session)
  - Update marker assertions: `"@portal-active-%3"` becomes `"@portal-active-my-session:0.0"`, etc.
  - In `mockPaneLister.panes` maps: change `map[string][]string{"my-session": {"%3"}}` to `map[string][]string{"my-session": {"my-session:0.0"}}`
  - In `mockAllPaneLister.panes`: change `[]string{"%3", "%5"}` to `[]string{"my-session:0.0", "my-session:0.1"}` etc.
  - In `mockHookLoader.data`: change keys from `"%3"` to `"my-session:0.0"` etc.
  - In `mockOptionChecker.options`: change `"@portal-active-%3"` to `"@portal-active-my-session:0.0"` etc.
  - In `mockKeySender.failFor`: change `"%3"` to `"my-session:0.0"` etc.
  - Update all assertion strings in `t.Errorf` / `t.Error` calls
- Rename test `"no tmux server running skips cleanup gracefully"` to `"empty pane list skips cleanup and continues hook execution"`
  - Update its mock values from pane IDs to structural keys (assertion already corrected by Phase 1 Task 1-1)
  - Keep the assertion that hook execution still proceeds (send-keys still fires)
- In the test `"skips pane not in session"`: the hook for `%7` (which is not in `"my-session"`) should use a key that clearly belongs to a different session, e.g., `"other-session:0.0"`. The `mockPaneLister` returns `["my-session:0.0"]` for `"my-session"`, so the hook keyed by `"other-session:0.0"` correctly does not match.
- In the test `"executes hooks for multiple qualifying panes"`: use three structural keys for the same session: `"my-session:0.0"`, `"my-session:0.1"`, `"my-session:1.0"`. Update the `mockPaneLister` to return all three for `"my-session"`. Update assertions for sent commands and marker names.

**Acceptance Criteria**:
- [ ] `MarkerName` parameter is named `key` and doc comment says "structural key"
- [ ] All interface doc comments use "structural key" / "key" terminology
- [ ] All executor tests use structural key format values
- [ ] Marker assertions verify `@portal-active-session:window.pane` format
- [ ] "no tmux server running" test asserts `CleanStale` is NOT called when `livePanes` is empty (Phase 1 guard)
- [ ] `ExecuteHooks` function body has no code changes (only doc comment updated)
- [ ] `go test ./internal/hooks/...` passes

**Tests**:
- `"executes hook when persistent entry exists and marker absent"` — uses structural key `"my-session:0.0"`, verifies send-keys target and command
- `"skips pane when volatile marker present"` — marker `"@portal-active-my-session:0.0"` already set, verifies no send-keys
- `"skips pane not in session"` — hook keyed `"other-session:0.0"` not in `"my-session"` pane list, verifies skip
- `"skips pane with no on-resume event"` — structural key with non-resume event, verifies skip
- `"sets volatile marker after executing hook"` — verifies marker name `"@portal-active-my-session:0.0"` with value `"1"`
- `"continues to next pane when SendKeys fails"` — `"my-session:0.0"` fails, `"my-session:0.2"` succeeds
- `"silent return when hook store Load fails"` — unchanged behavior
- `"silent return when ListPanes fails"` — unchanged behavior
- `"no-op when hook store is empty"` — unchanged behavior
- `"no-op when session has no panes"` — unchanged behavior
- `"executes hooks for multiple qualifying panes"` — three structural keys across two windows
- `"cleanup calls ListAllPanes and CleanStale before hook execution"` — structural key values in mock data
- `"ListAllPanes error skips cleanup and continues"` — unchanged behavior
- `"CleanStale error skips cleanup and continues"` — unchanged behavior
- `"cleanup runs before loader.Load"` — structural key values in mock data
- `"empty pane list skips cleanup and continues hook execution"` — asserts CleanStale NOT called, hooks still fire

**Edge Cases**:
- No additional edge cases beyond those already covered by tests. The marker format accepts colons and dots in tmux server option names (verified in specification: "tmux user options accept colons and dots in names").

**Context**:
> The specification states: "Volatile marker format: Definitive format is @portal-active-{structural_key} where structural key uses the standard format. Example: @portal-active-my-project-abc:0.0. tmux user options accept colons and dots in names."
>
> The specification also states: "SendKeys targeting: Use structural keys directly as tmux -t targets. tmux natively accepts session:window.pane format for targeting panes." This means `SendKeys(target, command)` works with structural keys without any code changes — tmux resolves them natively.
>
> The specification's Testing Requirements explicitly state: "Fix existing test 'no tmux server running skips cleanup gracefully' (executor_test.go:537-568) to assert CleanStale is not called when livePanes is empty." This was the Phase 1 empty-pane guard fix; this task updates the test assertion to match.
>
> Note: `cmd/hooks.go` references `hooks.MarkerName(paneID)` on lines 108 and 158. Those callers pass a pane ID from `$TMUX_PANE`. After this task, they still compile (the parameter was just renamed, not the type). Phase 3 will update them to resolve the structural key first and pass that to `MarkerName`.

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` — "Volatile marker format" in Design Decisions, "SendKeys targeting" in Design Decisions, "Interface changes" in Design Decisions, Testing Requirements

---
phase: 2
phase_name: Hook Execution in Connection Flow
total: 5
---

## resume-sessions-after-reboot-2-1 | approved

### Task 1: ListPanes and SendKeys Tmux Methods

**Problem**: The hook execution flow needs to query which panes belong to a given tmux session and deliver restart commands to individual panes. The existing `tmux.Client` in `internal/tmux/tmux.go` has no methods for listing panes or sending keys to a pane.

**Solution**: Add two new methods to `tmux.Client`: `ListPanes(sessionName string)` returning a slice of pane IDs (strings like `%0`, `%3`) for a given session, and `SendKeys(paneID string, command string)` that executes `tmux send-keys` to type a command into a pane followed by Enter. These wrap `tmux list-panes` and `tmux send-keys` respectively via the existing `Commander` interface.

**Outcome**: `tmux.Client` can list all pane IDs for a session and deliver commands to individual panes, enabling the hook executor in Task 2 to query session panes and fire restart commands.

**Do**:
- Add to `internal/tmux/tmux.go`:
  - `ListPanes(sessionName string) ([]string, error)` -- runs `c.cmd.Run("list-panes", "-t", sessionName, "-F", "#{pane_id}")`. Splits the output by newline, trims each line, filters out empty strings, and returns the resulting slice. On error (e.g., session does not exist), returns `(nil, fmt.Errorf("failed to list panes for session %q: %w", sessionName, err))`. Returns an empty slice (not nil) when the output is empty after trimming.
  - `SendKeys(paneID string, command string) error` -- runs `c.cmd.Run("send-keys", "-t", paneID, command, "Enter")`. The `"Enter"` argument tells tmux to press Enter after typing the command. Returns `fmt.Errorf("failed to send keys to pane %q: %w", paneID, err)` on failure.
- Add tests to `internal/tmux/tmux_test.go` using the existing `MockCommander` pattern (already defined in that file with `Output`, `Err`, `Calls`, and `RunFunc` fields). Follow the exact testing patterns used by `TestListSessions`, `TestKillSession`, etc.

**Acceptance Criteria**:
- [ ] `ListPanes` calls `tmux list-panes -t <sessionName> -F #{pane_id}` via the Commander
- [ ] `ListPanes` returns pane IDs as a string slice (e.g., `["%0", "%1", "%3"]`)
- [ ] `ListPanes` returns an empty slice when the session has no output (empty string from Commander)
- [ ] `ListPanes` returns a wrapped error when the Commander returns an error
- [ ] `SendKeys` calls `tmux send-keys -t <paneID> <command> Enter` via the Commander
- [ ] `SendKeys` returns a wrapped error when the Commander returns an error
- [ ] All tests pass: `go test ./internal/tmux/...`

**Tests**:
- `"ListPanes returns pane IDs for session with multiple panes"`
- `"ListPanes returns single pane ID"`
- `"ListPanes returns empty slice when session has no panes"`
- `"ListPanes returns error when session does not exist"`
- `"ListPanes calls list-panes with correct session target and format"`
- `"SendKeys sends command followed by Enter to pane"`
- `"SendKeys calls send-keys with correct pane target"`
- `"SendKeys returns error when pane does not exist"`

**Edge Cases**:
- Session with no panes returns empty slice -- when Commander returns empty output string, `ListPanes` returns `[]string{}` (empty slice, not nil) and nil error. This can happen if a session exists but has been left in an unusual state.
- Send-keys to non-existent pane returns error -- Commander returns a non-nil error for `send-keys -t %999`, and `SendKeys` wraps it with context about the pane ID.

**Context**:
> The spec says hook execution uses `tmux send-keys` to deliver restart commands to panes: "Portal uses tmux send-keys to deliver restart commands to panes. This types the command into the pane's existing shell as if the user typed it." The `list-panes` command with `-F #{pane_id}` returns pane IDs in the `%N` format that matches the hook store's key format. The `-t` flag targets a specific session.

**Spec Reference**: `Execution Mechanics` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-2-2 | approved

### Task 2: Hook Executor Core Logic

**Problem**: After a reboot, Portal needs to check each pane in a target session for hooks that need re-execution (persistent entry exists AND volatile marker absent) and fire the restart commands via `send-keys`. No such logic exists today. This is the core resume behavior that gives the feature its reason for existing.

**Solution**: Create an `ExecuteHooks` function (in `internal/hooks/executor.go`) that accepts small interfaces for its dependencies (hook store loading, pane listing, send-keys delivery, server option get/set). The function takes a session name, queries tmux for the session's panes, cross-references against the hook store, checks the volatile marker for each pane, executes the `on-resume` command via `send-keys` when the two-condition check passes, and sets the volatile marker afterward. Errors from the hook store load or individual `send-keys` calls are silently ignored to ensure robustness.

**Outcome**: A fully tested, independently injectable `ExecuteHooks` function that correctly implements the two-condition check (persistent entry exists AND volatile marker absent), fires restart commands for qualifying panes, sets volatile markers post-execution, and gracefully handles all error conditions.

**Do**:
- Create `internal/hooks/executor.go` with:
  - `PaneLister` interface: `ListPanes(sessionName string) ([]string, error)` -- satisfied by `*tmux.Client`
  - `KeySender` interface: `SendKeys(paneID string, command string) error` -- satisfied by `*tmux.Client`
  - `OptionChecker` interface: `GetServerOption(name string) (string, error)` and `SetServerOption(name, value string) error` -- satisfied by `*tmux.Client`. These two methods are grouped because the executor needs both check and set in its flow.
  - `HookLoader` interface: `Load() (map[string]map[string]string, error)` -- satisfied by `*hooks.Store`
  - `func ExecuteHooks(sessionName string, lister PaneLister, loader HookLoader, sender KeySender, checker OptionChecker)` -- note: returns nothing (no error). The entire function is best-effort. Implementation:
    1. Call `loader.Load()` -- if error, return silently (hook store load error is silently ignored per spec)
    2. If the loaded map is empty, return (no hooks registered at all)
    3. Call `lister.ListPanes(sessionName)` -- if error, return silently (session may not exist yet)
    4. If panes slice is empty, return (no panes to check)
    5. For each pane ID in the panes slice:
       a. Look up `hooks[paneID]` in the loaded map -- if not present, skip this pane (no hooks registered)
       b. Look up `hooks[paneID]["on-resume"]` -- if not present, skip this pane (no on-resume hook)
       c. Call `checker.GetServerOption("@portal-active-"+paneID)` -- if it returns a value (no error), the marker exists, skip this pane (already active on this server lifetime)
       d. If `GetServerOption` returns an error (marker absent, meaning `tmux.ErrOptionNotFound`), the two-condition check passes: call `sender.SendKeys(paneID, command)`. If `SendKeys` returns an error, silently ignore it and continue to the next pane.
       e. After successful or failed `SendKeys`, call `checker.SetServerOption("@portal-active-"+paneID, "1")` to set the volatile marker. Ignore the error from `SetServerOption` as well (best-effort).
- Create `internal/hooks/executor_test.go` with tests using mock implementations of all four interfaces. Tests should verify:
  - Which panes had `SendKeys` called
  - Which panes had `SetServerOption` called
  - That the function does not panic or error on various failure conditions

**Acceptance Criteria**:
- [ ] `ExecuteHooks` queries the session's panes via `PaneLister` and cross-references against the hook store
- [ ] The two-condition check is implemented: executes only when persistent entry exists AND `GetServerOption` returns an error (marker absent)
- [ ] After executing a hook for a pane, `SetServerOption("@portal-active-{paneID}", "1")` is called
- [ ] Panes with an existing volatile marker are skipped (no `SendKeys` call)
- [ ] Panes with no hook entry are skipped
- [ ] `SendKeys` failure for one pane does not block execution of hooks for other panes
- [ ] Hook store `Load` error causes silent return (no panic, no error propagation)
- [ ] `ListPanes` error causes silent return
- [ ] All tests pass: `go test ./internal/hooks/...`

**Tests**:
- `"executes on-resume hook for pane with entry and no marker"`
- `"skips pane when volatile marker exists"`
- `"skips pane with no hook entry"`
- `"executes hooks for multiple panes in session"`
- `"mixed panes some execute some skip"`
- `"sets volatile marker after executing hook"`
- `"sets volatile marker even when send-keys fails"`
- `"send-keys failure for one pane does not block others"`
- `"no hooks for any pane in session does nothing"`
- `"all panes already have volatile markers skips all"`
- `"hook store load error is silently ignored"`
- `"list-panes error is silently ignored"`
- `"empty pane list does nothing"`
- `"only on-resume event type is executed"`

**Edge Cases**:
- No hooks for any pane in session -- the loaded map has entries but none match the panes in this session. No `SendKeys` calls are made. No errors.
- All panes already have volatile markers (skip all) -- every pane's `GetServerOption` returns a value (no error). No `SendKeys` calls are made. The user is doing a normal reattach, not a post-reboot reconnect.
- Mixed panes (some execute, some skip) -- pane `%0` has a marker (skip), pane `%1` does not (execute), pane `%3` has no hook entry (skip). Only `%1` gets `SendKeys`.
- Send-keys failure for one pane does not block others -- if `SendKeys` for `%1` returns an error, execution continues to `%3`. The volatile marker is still set for `%1` to prevent retry loops.
- Hook store load error is silently ignored -- if `loader.Load()` returns an error (corrupt file, permission denied), the function returns without executing anything and without propagating the error. This prevents hook system failures from breaking the connection flow.

**Context**:
> The spec defines the two-condition execution check: "persistent entry exists AND volatile marker absent." The volatile marker `@portal-active-{pane_id}` is a tmux server option that dies with the server. After a reboot, all markers are absent, so all registered hooks qualify for execution. After execution, the marker is set to prevent re-execution on subsequent `portal open` calls. The spec also says: "If send-keys fails for a pane (e.g., pane in unexpected state), the error is silently ignored and execution continues to the next pane." Multiple panes are executed sequentially -- "fire-and-forget via send-keys, no waiting for completion."

**Spec Reference**: `Execution Mechanics` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-2-3 | approved

### Task 3: Hook Execution in Attach Command

**Problem**: The `portal attach` command (`cmd/attach.go`) connects to a named tmux session but does not execute any resume hooks. After a reboot, running `portal attach my-session` should fire restart commands for panes in that session before connecting. Critically, the `AttachConnector.Connect` method uses `syscall.Exec` which replaces the current process -- nothing can run after it. Hook execution must happen before the `connector.Connect(name)` call.

**Solution**: Insert a call to `hooks.ExecuteHooks` in the attach command's `RunE`, after validating the session exists but before calling `connector.Connect(name)`. The hook executor needs a `*tmux.Client` (for `ListPanes`, `SendKeys`, `GetServerOption`, `SetServerOption`) and a `*hooks.Store` (for `Load`). Both are constructed inside the attach command. The `AttachDeps` DI struct is extended with an optional `HookExecutor` function for test injection.

**Outcome**: Running `portal attach my-session` after a reboot executes registered hooks for all qualifying panes in `my-session` before connecting. Normal reattaches (volatile markers present) skip execution silently. Tests verify hook execution ordering relative to the connect call.

**Do**:
- Define a `HookExecutor` function type in `cmd/attach.go`: `type HookExecutorFunc func(sessionName string)` -- a simple function that encapsulates calling `hooks.ExecuteHooks` with all its dependencies. This keeps the attach command lean.
- Add `HookExecutor HookExecutorFunc` field to the existing `AttachDeps` struct in `cmd/attach.go`
- Create a `buildHookExecutor` function in `cmd/attach.go` (or a shared `cmd/hook_executor.go` file if preferred for reuse across Tasks 3-5): takes a `*tmux.Client` and returns a `HookExecutorFunc`. The returned function:
  1. Calls `hooksFilePath()` to get the hooks store path (uses `PORTAL_HOOKS_FILE` env var or default `~/.config/portal/hooks.json`)
  2. Creates `hooks.NewStore(path)`
  3. Calls `hooks.ExecuteHooks(sessionName, client, store, client, client)` -- the `*tmux.Client` satisfies `PaneLister`, `KeySender`, and `OptionChecker`; the `*hooks.Store` satisfies `HookLoader`
- Modify the attach command's `RunE` in `cmd/attach.go`:
  - After the `validator.HasSession(name)` check and before `connector.Connect(name)`, insert:
    ```
    hookExec := getHookExecutor(cmd)
    hookExec(name)
    ```
  - `getHookExecutor` checks `attachDeps` first: if `attachDeps != nil && attachDeps.HookExecutor != nil`, return it. Otherwise, call `buildHookExecutor(tmuxClient(cmd))` to create a real one.
- Modify `buildAttachDeps` to also return the hook executor (or adjust `getHookExecutor` to be a standalone function that tests can override via `attachDeps`).
- Extend `cmd/attach_test.go`:
  - Add a mock `HookExecutorFunc` that records whether it was called and with what session name
  - Inject it via `attachDeps.HookExecutor`
  - Verify hook execution happens before connect (the mock connector records call order; hook executor records its call; check that hook was called first)
  - Verify hook execution is called with the correct session name

**Acceptance Criteria**:
- [ ] `portal attach my-session` calls hook execution before `connector.Connect`
- [ ] Hook execution receives the correct session name
- [ ] Hook execution runs before `syscall.Exec` replaces the process (verified by test ordering)
- [ ] When `attachDeps.HookExecutor` is injected, the injected function is used (testability)
- [ ] Existing attach tests continue to pass (hook executor defaults to a no-op-like real executor that silently does nothing when no hooks exist)
- [ ] All tests pass: `go test ./cmd -run TestAttach`

**Tests**:
- `"hook execution runs before connect"`
- `"hook execution receives correct session name"`
- `"non-existent session skips hook execution"`
- `"session with no panes triggers no hook execution"`
- `"existing attach tests still pass with hook executor wired in"`

**Edge Cases**:
- Session with no panes triggers no hook execution -- the hook executor calls `ListPanes` which returns an empty slice, so no hooks fire. The connect still proceeds normally.
- Hook execution runs before `syscall.Exec` replaces process -- verified in tests by using a mock connector that records call order. The mock hook executor and mock connector both append to a shared order slice; the test asserts `["hooks", "connect"]` ordering.

**Context**:
> The spec says: "Hook execution happens before connecting to the session. This is required for AttachConnector (syscall.Exec replaces the process -- nothing can run after) and consistent for SwitchConnector. All Portal connection paths trigger hook execution: TUI picker selection, direct path argument, and portal attach." The attach command currently validates the session exists, then calls `connector.Connect(name)`. Hook execution is inserted between these two steps.

**Spec Reference**: `Execution Mechanics` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-2-4 | approved

### Task 4: Hook Execution in TUI Selection Path

**Problem**: When a user selects an existing session from the TUI picker (launched by `portal open` with no arguments), the `processTUIResult` function in `cmd/open.go` connects to that session without executing any resume hooks. After a reboot, selecting a restored session from the TUI should fire restart commands for that session's panes before connecting.

**Solution**: Insert a call to the hook executor in `processTUIResult` (or just before it in the `openTUI` function), after the TUI returns a selected session name but before `connector.Connect(selected)`. If the user quit the TUI without selecting anything (`model.Selected() == ""`), no hook execution occurs. Reuse the same `HookExecutorFunc` pattern from Task 3.

**Outcome**: Selecting a session from the TUI after a reboot executes registered hooks for qualifying panes before connecting. Quitting the TUI without selection triggers no hook execution. Tests verify both paths.

**Do**:
- If not already created in Task 3, create `cmd/hook_executor.go` with the shared `HookExecutorFunc` type and `buildHookExecutor` function. If Task 3 already created these in `cmd/attach.go`, move them to a shared location (`cmd/hook_executor.go`) so both the attach and open commands can use them.
- Modify `processTUIResult` in `cmd/open.go` to accept an additional `HookExecutorFunc` parameter:
  - Change signature from `func processTUIResult(model tui.Model, connector SessionConnector) error` to `func processTUIResult(model tui.Model, connector SessionConnector, hookExec HookExecutorFunc) error`
  - After checking `selected == ""` (still return nil for no selection), but before `connector.Connect(selected)`, insert: `hookExec(selected)`
- Update all callers of `processTUIResult`:
  - In `openTUI`: construct the hook executor via `buildHookExecutor(client)` and pass it as the third argument
  - Update existing tests for `processTUIResult` in `cmd/open_test.go` to pass a no-op `HookExecutorFunc` (e.g., `func(string) {}`) to maintain backward compatibility
- To support test injection, add a `HookExecutor HookExecutorFunc` field to `OpenDeps` (or use `openTUIFunc` override pattern, since `openTUI` is already overridable). The simplest approach: since `openTUI` already constructs dependencies internally and tests override `openTUIFunc`, test the hook execution by testing `processTUIResult` directly with a mock hook executor.
- Extend tests in `cmd/open_test.go`:
  - Test `processTUIResult` with a mock hook executor that records whether it was called and with what session name
  - Test that when `model.Selected() == ""`, the hook executor is NOT called
  - Test that when a session is selected, the hook executor IS called with the selected session name before `connector.Connect`

**Acceptance Criteria**:
- [ ] Selecting a session from the TUI calls hook execution before `connector.Connect`
- [ ] The hook executor receives the selected session name
- [ ] Quitting the TUI without selection does not call the hook executor
- [ ] Existing `TestProcessTUIResult` tests continue to pass (updated with no-op hook executor)
- [ ] All tests pass: `go test ./cmd -run TestProcessTUIResult`

**Tests**:
- `"selected session triggers hook execution before connect"`
- `"hook executor receives selected session name"`
- `"user quits TUI without selection triggers no hook execution"`
- `"user quits TUI without selection returns nil"`
- `"existing processTUIResult tests still pass"`

**Edge Cases**:
- User quits TUI without selection (no hook execution) -- `model.Selected()` returns `""`, the function returns nil immediately. Neither `hookExec` nor `connector.Connect` is called.
- Selected session triggers hook execution before connect -- the hook executor runs before `connector.Connect(selected)`. This is critical for `AttachConnector` which uses `syscall.Exec`.

**Context**:
> The spec says: "All Portal connection paths trigger hook execution: TUI picker selection, direct path argument, and portal attach." The TUI path is `portal open` with no arguments, which launches the interactive picker. The user selects an existing session, and `processTUIResult` connects to it. Hook execution must happen before `connector.Connect` in this path. The `processTUIResult` function currently calls `connector.Connect(selected)` directly. The hook executor is inserted before this call.

**Spec Reference**: `Execution Mechanics` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-2-5 | approved

### Task 5: Hook Execution in Direct Path

**Problem**: When a user runs `portal open /some/path`, the `openPath` function in `cmd/open.go` creates a new session (or attaches to an existing one) without executing resume hooks. There are two sub-paths here: (1) inside tmux, `PathOpener.Open` creates a detached session then calls `switcher.SwitchClient(sessionName)`, and (2) outside tmux, it runs `QuickStart` then calls `execer.Exec` (process replacement). Hook execution must be wired into both sub-paths. However, there is a critical subtlety: when a direct path creates a **new** session, there are no existing panes with hooks yet, so hook execution should be a no-op. When `portal open` resolves to an **existing** session via the `-A` flag in `QuickStart` or via `HasSession` check, hooks should fire.

**Solution**: Insert hook execution into the `PathOpener.Open` method. For the inside-tmux path, execute hooks on the session name returned by `CreateFromDir` before calling `SwitchClient`. For the outside-tmux path, the `QuickStart` pipeline uses `tmux new-session -A` which atomically creates or attaches. Since the session name is known after `QuickStart.Run` returns, execute hooks on that session name before calling `execer.Exec`. In the new-session case, `ExecuteHooks` will find no panes with registered hooks (the session was just created) and silently do nothing. In the reattach case, it will find existing panes and execute qualifying hooks.

**Outcome**: The direct path (`portal open /path`) runs hook execution before connecting in both the inside-tmux and outside-tmux branches. New sessions naturally skip hook execution (no hooks registered for their brand-new panes). Existing sessions get their hooks executed before the user connects.

**Do**:
- Add a `hookExec HookExecutorFunc` field to the `PathOpener` struct in `cmd/open.go`
- Modify `PathOpener.Open` in `cmd/open.go`:
  - **Inside-tmux branch**: After `po.creator.CreateFromDir(resolvedPath, command)` returns `sessionName`, call `po.hookExec(sessionName)` before `po.switcher.SwitchClient(sessionName)`. Note: `CreateFromDir` may create a new session (no existing hooks) or an existing session name may collide (unlikely given nanoid). In either case, `ExecuteHooks` handles it gracefully.
  - **Outside-tmux branch**: After `po.qs.Run(resolvedPath, command)` returns `result`, call `po.hookExec(result.SessionName)` before `po.execer.Exec(...)`. The `QuickStart` result includes the session name. If the session is new, there are no hooks. If `-A` attached to an existing session, hooks will fire for qualifying panes.
- Modify `openPath` in `cmd/open.go` to wire the hook executor into `PathOpener`:
  - After constructing the `PathOpener`, set `opener.hookExec = buildHookExecutor(client)`
- Update existing `TestPathOpener` tests in `cmd/open_test.go`:
  - Each existing test that creates a `PathOpener` must now provide a `hookExec` field. Use `func(string) {}` (no-op) for tests that are not specifically testing hook behavior, to maintain backward compatibility.
- Add new tests in `cmd/open_test.go` for hook execution in the direct path:
  - Test inside-tmux: verify hook executor is called with the session name from `CreateFromDir` before `SwitchClient`
  - Test outside-tmux: verify hook executor is called with `result.SessionName` from `QuickStart` before `Exec`
  - Test that a mock hook executor records call order relative to the connect/exec call

**Acceptance Criteria**:
- [ ] `PathOpener.Open` calls hook execution before `SwitchClient` in the inside-tmux path
- [ ] `PathOpener.Open` calls hook execution before `Exec` in the outside-tmux path
- [ ] Hook executor receives the correct session name in both paths
- [ ] New session creation (no existing panes) results in hook execution being a no-op (no crash, no error)
- [ ] Existing `TestPathOpener` tests continue to pass (updated with no-op hook executor)
- [ ] All tests pass: `go test ./cmd -run TestPathOpener`

**Tests**:
- `"inside tmux executes hooks before switch-client"`
- `"inside tmux hook executor receives session name from CreateFromDir"`
- `"outside tmux executes hooks before exec"`
- `"outside tmux hook executor receives session name from QuickStart"`
- `"new session creation has no hooks to execute"`
- `"existing PathOpener tests still pass with hook executor wired in"`

**Edge Cases**:
- New session creation (no existing panes yet) skips hook execution -- when `CreateFromDir` creates a brand new session, its panes have no entries in the hook store. `ExecuteHooks` loads the store, queries panes, finds no matching entries, and returns without calling `SendKeys`. This is the expected no-op case for new sessions.
- Inside-tmux switch path executes hooks before switch-client -- `SwitchConnector` calls `tmux switch-client` which does not replace the process (unlike `syscall.Exec`), but the hook execution still runs before it for consistency. Tests verify ordering by checking that the mock hook executor is called before `SwitchClient`.

**Context**:
> The spec says: "Hook execution happens before connecting to the session. This is required for AttachConnector (syscall.Exec replaces the process -- nothing can run after) and consistent for SwitchConnector." The direct path is `portal open /some/path`. Inside tmux, it creates a detached session then switches. Outside tmux, it uses `QuickStart` with `-A` flag (atomic create-or-attach) then execs. Both branches need hook execution before the final connect/exec step. For new sessions, the hook executor will naturally find no hooks and do nothing.

**Spec Reference**: `Execution Mechanics` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

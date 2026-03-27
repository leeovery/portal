---
status: complete
created: 2026-03-27
cycle: 1
phase: Traceability Review
topic: Resume Sessions After Reboot
---

# Review Tracking: Resume Sessions After Reboot - Traceability

## Findings

### 1. Hook execution iterates over tmux panes instead of JSON store pane IDs

**Type**: Incomplete coverage
**Spec Reference**: Execution Mechanics > Multiple panes: "Order follows pane ID iteration from the JSON store."
**Plan Reference**: Phase 2 / Task 2-2 (Hook Executor Core Logic), Step 5 of Do section
**Change Type**: update-task

**Details**:
The spec explicitly states that when a session has multiple panes with registered hooks, the execution order follows "pane ID iteration from the JSON store." The plan's executor iterates over panes returned by `ListPanes(sessionName)` (tmux's `list-panes` output) and cross-references each against the store. This means the iteration order is driven by tmux's pane ordering, not the JSON store's key ordering. While both approaches execute the same set of hooks, the order may differ. The spec is explicit about this ordering, so the plan should reflect it.

The correct approach per the spec: iterate over the JSON store's pane IDs (sorted or in map iteration order), and for each, check whether it belongs to the target session (using the panes from `ListPanes` as a lookup set). This inverts the current lookup direction.

**Current**:
```markdown
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
```

**Proposed**:
```markdown
**Do**:
- Create `internal/hooks/executor.go` with:
  - `PaneLister` interface: `ListPanes(sessionName string) ([]string, error)` -- satisfied by `*tmux.Client`
  - `KeySender` interface: `SendKeys(paneID string, command string) error` -- satisfied by `*tmux.Client`
  - `OptionChecker` interface: `GetServerOption(name string) (string, error)` and `SetServerOption(name, value string) error` -- satisfied by `*tmux.Client`. These two methods are grouped because the executor needs both check and set in its flow.
  - `HookLoader` interface: `Load() (map[string]map[string]string, error)` -- satisfied by `*hooks.Store`
  - `func ExecuteHooks(sessionName string, lister PaneLister, loader HookLoader, sender KeySender, checker OptionChecker)` -- note: returns nothing (no error). The entire function is best-effort. Implementation:
    1. Call `loader.Load()` -- if error, return silently (hook store load error is silently ignored)
    2. If the loaded map is empty, return (no hooks registered at all)
    3. Call `lister.ListPanes(sessionName)` -- if error, return silently (session may not exist yet)
    4. If panes slice is empty, return (no panes to check)
    5. Build a set from the session's pane IDs (from step 3) for O(1) lookup.
    6. Iterate over the loaded hook map's pane IDs (from step 1), following the JSON store's iteration order per spec. For each pane ID in the hook map:
       a. Check if this pane ID is in the session's pane set (from step 5) -- if not, skip this pane (belongs to a different session)
       b. Look up `hooks[paneID]["on-resume"]` -- if not present, skip this pane (no on-resume hook)
       c. Call `checker.GetServerOption("@portal-active-"+paneID)` -- if it returns a value (no error), the marker exists, skip this pane (already active on this server lifetime)
       d. If `GetServerOption` returns an error (marker absent, meaning `tmux.ErrOptionNotFound`), the two-condition check passes: call `sender.SendKeys(paneID, command)`. If `SendKeys` returns an error, silently ignore it and continue to the next pane.
       e. After successful or failed `SendKeys`, call `checker.SetServerOption("@portal-active-"+paneID, "1")` to set the volatile marker. Ignore the error from `SetServerOption` as well (best-effort).
```

**Resolution**: Fixed
**Notes**:
The practical impact is low since `send-keys` is fire-and-forget, but the spec is explicit about iteration order coming from the JSON store. The proposed fix inverts the lookup: iterate over store entries and filter by session membership, rather than iterate over session panes and look up in the store. Go map iteration order is non-deterministic, but the spec says "pane ID iteration from the JSON store" which implies iterating the store's keys (in whatever order Go maps provide). If deterministic ordering is desired, the keys could be sorted, but the spec does not require sorted order -- it just says "from the JSON store."

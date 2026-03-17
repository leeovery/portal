---
topic: portal
cycle: 3
total_proposed: 1
---
# Analysis Tasks: Portal (Cycle 3)

## Task 1: Apply initial filter to session list on query resolution fallback
status: approved
severity: medium
sources: standards

**Problem**: When query resolution falls back to the TUI (spec step 4: "Fall back to the TUI with the query pre-filled as the filter text"), the session list does not activate filter mode. `WithInitialFilter` stores the filter string in `m.initialFilter` but never sets `m.filterMode = true` or `m.filterText`. The user sees the full unfiltered session list instead of it being pre-filtered by their query. For example, `x myapp` where "myapp" has no alias/zoxide match should open the session picker with "myapp" already typed into the filter.

**Solution**: In the `SessionsMsg` handler in `internal/tui/model.go`, after setting `m.loaded = true`, check if `m.initialFilter` is non-empty. If so, activate filter mode by setting `m.filterMode = true` and `m.filterText = m.initialFilter`, then apply the fuzzy filter to the session list. Clear `m.initialFilter` after consuming it so it only applies on the first load.

**Outcome**: When query resolution falls back to the session list TUI with a filter string, the session list opens with filter mode active and the query pre-filled, matching the spec's query resolution step 4.

**Do**:
1. In `internal/tui/model.go`, locate the `SessionsMsg` handler (around line 320-331).
2. After the `m.loaded = true` line, add a block that checks `m.initialFilter != ""`.
3. When the initial filter is present: set `m.filterMode = true`, set `m.filterText = m.initialFilter`, apply fuzzy filtering to `m.sessions` using the same logic as the existing filter mode, and clear `m.initialFilter = ""` to prevent re-application.
4. In `internal/tui/model_test.go`, update or add tests to verify:
   - When `WithInitialFilter("myapp")` is set (without command pending) and `SessionsMsg` arrives, the model enters filter mode with `filterText` set to "myapp".
   - The session list is filtered by the initial filter text.
   - The filter is only applied on the first `SessionsMsg` (subsequent session refreshes do not re-apply it).

**Acceptance Criteria**:
- `WithInitialFilter("query")` on a non-command-pending model causes the session list to open in filter mode with "query" pre-filled after sessions load.
- The filtered session list shows only sessions matching the fuzzy filter.
- The `[n] new in project...` option remains visible.
- The initial filter is consumed on first load and not re-applied on subsequent session refreshes.
- Existing behavior preserved: `WithInitialFilter` with command pending still forwards to project picker.

**Tests**:
- Test that `SessionsMsg` with a non-empty `initialFilter` activates filter mode and pre-fills filter text.
- Test that sessions are fuzzy-filtered by the initial filter value.
- Test that a second `SessionsMsg` does not re-apply the consumed initial filter.
- Test that command-pending mode with initial filter still forwards to project picker (existing behavior unchanged).

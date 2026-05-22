# Plan: Three Column Keymap Footer

## Phase 1: Apply Change

Render the sessions-list and projects-list keymap footer as three fixed columns instead of two, without changing any binding behaviour.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| three-column-keymap-footer-1-1 | Render sessions and projects keymap footer in three fixed columns | Disabled bindings must be filtered before chunking so the visible column count is what users see; filter-mode bindings (Filter/ClearFilter/AcceptWhileFiltering/CancelWhileFiltering) must continue to surface in their currently-enabled state; manual footer height must be subtracted from list size so the list does not overflow under the new bar; styles (`brightenHelpStyles`) and separators must be preserved so the visual matches the current bar in colour and character set; `commandPendingHelpKeys` (fewer entries on projects page) — short or empty trailing columns are acceptable. |

### Phase 3: Analysis (Cycle 2)

Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| three-column-keymap-footer-3-1 | Add per-page wrappers over applyListSize to own the list+bindings pairing invariant | `applyListSize` core must remain (cycle-1 consolidation preserved); `m.commandPending` branch must be owned by `applyProjectListSize`, not callers; all 8 prior call sites (5 sessions, 3 projects) must route through a wrapper; `applyListSize` may be converted from `*Model` method to free function since it reads no `m` field; existing TUI sizing tests must continue to pass unchanged. |

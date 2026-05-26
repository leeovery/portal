# Plan: Hooks Rm Pane Key Flag

## Phase 1: Apply Change

Add `--pane-key` flag to `hooks rm` with a fallback to the existing current-pane resolution, covered by branch-level tests.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| hooks-rm-pane-key-flag-1-1 | Add `--pane-key` flag and branch in `hooksRmCmd` | Flag-set branch must not require `$TMUX_PANE`; flag value is a literal pass-through to `store.Remove` (no validation against live panes) |

# Plan: Drop Invalid A Flag From Attach Session Argv

## Phase 1: Apply Change

Restore the pre-v0.5.1 `attach-session` argv shape, keeping the `=` exact-match prefix, and correct the upstream spec lines that re-derive the invalid form.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| drop-invalid-A-flag-from-attach-session-argv-1-1 | Remove `-A` from `AttachConnector` argv, docstring, and unit test | None — single argv slice and one test assertion |
| drop-invalid-A-flag-from-attach-session-argv-1-2 | Correct upstream `enter-attaches-from-preview` spec §88 and §166 | Add corrigendum note pointing back to this quick-fix |

## Phase 2: Analysis (Cycle 1)

Address findings from Analysis (Cycle 1).

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| drop-invalid-A-flag-from-attach-session-argv-2-1 | Drop stale `-A` from `cmd/reattach_integration_test.go` comments | None — documentation-only edit to two comment lines |

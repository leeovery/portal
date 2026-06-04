# Plan: Cleanup Purge Test No State Isolation

## Phase 1: Apply Change

Apply `PORTAL_STATE_DIR=t.TempDir()` isolation unconditionally to all subtests of `TestStateUserFacingSubcommandsExitZero` in `cmd/state_test.go`.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cleanup-purge-test-no-state-isolation-1-1 | Isolate all TestStateUserFacingSubcommandsExitZero subtests | Preserve existing ErrStatusUnhealthy tolerance; keep per-subtest TempDir |

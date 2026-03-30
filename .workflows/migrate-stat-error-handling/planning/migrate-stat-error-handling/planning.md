# Plan: Migrate Stat Error Handling

## Phase 1: Apply Change

Use os.IsNotExist explicitly in the migrateConfigFile newPath stat check and add test coverage for the refined logic.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| migrate-stat-error-handling-1-1 | Refine stat error handling and add test | Ensure non-"not found" errors (e.g. permission denied) cause early return without attempting migration |

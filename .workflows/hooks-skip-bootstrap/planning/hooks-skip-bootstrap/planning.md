# Plan: Hooks Skip Bootstrap

## Phase 1: Apply Change

Add `hooks` to the `skipTmuxCheck` allowlist in `cmd/root.go`, rewrite the justifying comment block, and cover the new behaviour with one sub-test.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| hooks-skip-bootstrap-1-1 | Add hooks to skipTmuxCheck allowlist and update comment | Comment paragraph must justify inclusion, not exclusion; preserve the existing alias/clean/help/init/state/version doc block above the map |
| hooks-skip-bootstrap-1-2 | Add sub-test asserting `hooks set` skips bootstrap | Test must use the existing skipTmuxCheck assertion pattern; do not introduce new fixtures or change shared mock state |

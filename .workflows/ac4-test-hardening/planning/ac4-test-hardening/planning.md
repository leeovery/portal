# Plan: AC4 Test Hardening

## Phase 1: Apply Change

Add the drift-mirror breadcrumb comment in production `captureAndCommit` and the inline negative-control sub-test for AC4.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| ac4-test-hardening-1-1 | Add drift-mirror comment in captureAndCommit | none |
| ac4-test-hardening-1-2 | Add NoOpEagerHydrateSignaler negative-control test for AC4 | tmux-fixture wall-time cost; sub-test must guard with `testing.Short()` and `tmuxtest.SkipIfNoTmux` symmetric with existing AC4 test |

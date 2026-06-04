# Plan: Harden Daemon Integration Test

## Phase 1: Apply Change

Close two silent-pass gaps in `cmd/state_daemon_integration_test.go` — export the kill-barrier ceiling as a single source of truth, and turn the fast-host no-op pass into an explicit skip.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| harden-daemon-integration-test-1-1 | Export KillBarrierTimeoutCeiling from internal/tmux and reuse in test | Production default must reference the new constant so value stays single-sourced |
| harden-daemon-integration-test-1-2 | Skip instead of warn on fast capture-pane host in mid-tick SIGHUP test | Skip message must preserve the diagnostic numbers from the prior warning |

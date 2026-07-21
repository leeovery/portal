---
topic: spawned-window-dead-ends-on-session-exit
cycle: 1
total_findings: 2
deduplicated_findings: 2
proposed_tasks: 1
---
# Analysis Report: Spawned Window Dead-Ends On Session Exit (Cycle 1)

## Summary
The production fix is architecturally sound and standards-conformant — the Ghostty adapter wraps the composed argv in `bash -lc '<rendered argv>; exec "$SHELL" -il'` via the shared quoting helper, correctly scoped to Ghostty with no shared-composition or API-surface change, and no duplication was detected. Both actionable findings land on the same test file (`internal/spawn/ghostty_command_test.go`): a stale `attach`-verb fixture and a test-guard that self-validates instead of pinning the escaped golden string acceptance criterion #7 requires. Because the golden the architecture finding asks for must pin the fixture the standards finding says is wrong, the two are intertwined and synthesise into a single self-contained test-hardening task.

## Discarded Findings
- (none)

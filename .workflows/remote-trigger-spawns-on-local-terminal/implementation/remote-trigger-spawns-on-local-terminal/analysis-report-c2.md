---
topic: remote-trigger-spawns-on-local-terminal
cycle: 2
total_findings: 2
deduplicated_findings: 2
proposed_tasks: 1
---
# Analysis Report: Remote Trigger Spawns On Local Terminal (Cycle 2)

## Summary
Standards found nothing — the inversion is spec-conformant and every required test transform is present. Two medium findings remain, in different files and non-overlapping: a stale `Activity` doc on the source-of-truth `tmux.ClientInfo` type (a latent re-break vector the architecture agent itself calls "not a live defect"), and seven near-duplicate happy-path subtests in `detect_inside_test.go` that should collapse into one table-driven test. The test-refactor finding targets a file the spec's Testing Requirements directly govern and becomes the one proposed task; the `tmux.ClientInfo` doc lives in a different package outside the spec's single-localized-change scope and was already ruled out of scope in cycle 1, so it is discarded, not promoted.

## Discarded Findings
- Stale `Activity` contract on `tmux.ClientInfo` (`internal/tmux/clients.go:11-12`) — out of scope. The specification scopes this work to "a single localized change to `detectInsideTmux` in `internal/spawn/detect_inside.go`," and its "Owned Behaviour Change" doc-rewrite requirement is explicitly limited to `detect_inside.go`'s docstring. `internal/tmux/clients.go` is the source-of-truth domain type in a separate package that the spec lists nowhere in its affected surfaces. Cycle 1 already ruled this identical mirror-doc note out of scope ("Leave the out-of-scope mirror `tmux.ClientInfo` type doc … untouched — outside this plan's scope"). The finding is medium (not high) and, in the agent's own words, "a latent re-break vector, not a live defect — low likelihood." Per the out-of-scope policy it is recorded here, not silently promoted into a task.

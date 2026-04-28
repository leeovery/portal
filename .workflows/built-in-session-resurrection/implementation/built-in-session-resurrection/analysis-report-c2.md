---
topic: built-in-session-resurrection
cycle: 2
total_findings: 12
deduplicated_findings: 10
proposed_tasks: 10
---
# Analysis Report: built-in-session-resurrection (Cycle 2)

## Summary

Cycle 2 surfaced 12 findings across duplication (6), standards (2), and architecture (4). After deduplication, ten distinct concerns remain — all rooted in residue from the T7-9 `respawn-pane -k` pivot and the T7-* consolidation series (stale doc references, dead error returns, leftover plumbing, and copy-paste idioms). Two highs (CLAUDE.md drift; `skipIfNoTmux` duplication) plus mediums and lows promote into ten self-contained tasks; no findings are discarded.

## Discarded Findings

None. All findings either promote directly into tasks or merge with a related finding into a single task (see deduplications).

## Deduplications Applied

- **Hydrate-path skeleton-marker unset duplication** (duplication medium, `cmd/state_hydrate.go:178-180` and `:302-305`) and **FIFO→marker convenience helper** (architecture low, same files plus `internal/state/markers.go`) merged into a single task that introduces `state.UnsetSkeletonMarkerForFIFO` and replaces both hydrate sites.
- **ApplySkeletonMarkers false-error signature** (architecture low) and **prediction-params coupling** (architecture medium) combined into one task — drop both the error return and the predicted-base parameters in a single PR; the drift comparison moves up to `restoreOne`.
- **Spec describes helper as initial process** (standards medium) treated as one spec-update task; implementation comments in `session.go:165-170` and `:491-495` already acknowledge the deviation, so this is doc-only.

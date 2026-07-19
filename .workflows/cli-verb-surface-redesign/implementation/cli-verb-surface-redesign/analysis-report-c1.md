---
topic: cli-verb-surface-redesign
cycle: 1
total_findings: 13
deduplicated_findings: 10
proposed_tasks: 8
---
# Analysis Report: CLI Verb Surface Redesign (Cycle 1)

## Summary
The redesigned CLI code conforms to the spec (open multi-target burst, domain pins, hidden `--ack`, doctor/uninstall, kill single+exact, hooks→hook, fully-hidden state, resolve log component). The findings cluster into three groups: one user-facing correctness gap (live bootstrap warnings still point at the deleted `portal state status`), a set of drift-risk duplications introduced by the per-task rollout (four copy-paste domain-pin dispatch arms, a byte-identical single-target miss string, a duplicated session-glob expansion, an unlinked second copy of open's value-taking flag set, and two governed two-site emissions), and stale documentation/comment references to removed surfaces (CLAUDE.md, ~15 internal comments, a dead process-role arm). Eight tasks are proposed; two low-severity items are recorded as discarded.

## Discarded Findings
- doctor read-only staleness checks re-implement the stores' CleanStale classification (duplication, low) — the code comments already flag each as a deliberate acknowledged "READ-ONLY mirror", and the single-sourcing fix reaches the pre-existing `hooks.Store.CleanStale` / `project.Store.CleanStale` store methods, which the finding itself flags as likely out of scope for this feature. Kept on record as a known drift risk; not proposed.
- completion prefix-filter loop duplicated across completeSessionNames / completeAliasKeys (duplication, low) — only two instances (Rule of Three not met); the finding explicitly calls it acceptable as-is and a watch-item only, actionable if a third Portal-owned completer appears. Not proposed.

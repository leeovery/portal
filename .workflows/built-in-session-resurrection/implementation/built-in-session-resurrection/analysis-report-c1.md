---
topic: built-in-session-resurrection
cycle: 1
total_findings: 26
deduplicated_findings: 21
proposed_tasks: 20
---
# Analysis Report: built-in-session-resurrection (Cycle 1)

## Summary

Three analysis agents produced 26 findings (2 high, 11 medium, 13 low). Five
cross-agent overlaps deduplicate to 21 unique issues. Two highs dominate:
README still teaches the deleted send-keys/@portal-active hook firing model
contradicting the next paragraph, and the migrate-rename hook is wired
end-to-end but passes the new session name twice making the spec-mandated
atomic key migration a structural no-op. The medium cluster is dominated by
duplication that survived the prior apply-cycle (paneKey helper, 26 redundant
`if Logger != nil` guards, twin `DeleteServerOption`/`UnsetServerOption`
wrappers, dead-code parallel restoring-marker API in `internal/restore`,
twin tmuxSocket integration-test harnesses, twin real-step adapters in
phase5 test, plus the spec-explicit "re-query live indices post-creation"
implemented as prediction-only). One low was discarded as explicitly
optional by the originating agent.

## Discarded Findings

- `--purge` does not wait for daemon's final flush before `RemoveAll` — agent
  marked it "Optional — Not required for correctness" and the spec already
  authorises the bounded data loss; AtomicWrite atomicity caps corruption.

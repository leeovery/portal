---
status: complete
created: 2026-06-09
cycle: 3
phase: Gap Analysis
topic: status-recent-warnings-pipe-format
---

# Review Tracking: status-recent-warnings-pipe-format - Gap Analysis

## Findings

### 1. "Remove" vs "migrate" wording for the two cmd-layer pipe-format fixtures reads as a surface contradiction

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria & Testing → Testing requirements (the "Remove the independent format fixtures" bullet vs the "Where the producer-coupled assertion lives, and the cmd-layer tests" bullet)

**Details**:
The first Testing bullet, "**Remove the independent format fixtures**," lists three things to delete: `writeLogLine` in `status_test.go`, "**and the two pipe-format fixtures in `cmd/state_status_test.go`**." The third Testing bullet then says the **same** two `cmd/state_status_test.go` fixtures are "**migrated, not deleted**" — their assertions retained, only the hand-authored log string replaced with real-writer output.

Read together by an implementer scanning the section, "remove the two pipe-format fixtures" and "the two fixtures are migrated, not deleted" appear to directly conflict. The reconciliation is real (the *independently-defined format string* is what gets removed; the *test cases / assertions* are retained and re-sourced from the writer), and the third bullet does spell that out — but the first bullet's blanket "Remove … the two pipe-format fixtures" phrasing invites a skim-reader to delete the two `cmd` test cases outright, which would drop the rendered-suffix and non-zero-exit cmd-layer coverage the spec intends to keep.

This is a clarity tension between two co-located bullets, not a missing decision. It is the only residual item found; the parsing contract (§1), reader migration (§2), empty-component/empty-message rendering, last-wins positional semantics, ok=false triggers, the producer-coupled test seam, and all seven acceptance criteria are internally complete, unambiguous, and consistent with the verified source (`internal/state/status.go`, `cmd/state_status.go`, `internal/log/handler.go`). Source references in the spec (the `writeLogLine` helper, the `status_test.go:368` malformed case, the two `cmd/state_status_test.go` fixtures) were all confirmed accurate.

**Proposed Addition**:
Reword the first Testing bullet so the "remove" verb attaches to the independent **format definition / hand-authored string**, not to the cmd-layer test cases — e.g. change "…and the two pipe-format fixtures in `cmd/state_status_test.go`" to "…and the **hand-authored pipe-format log strings** in the two `cmd/state_status_test.go` cases (the cases themselves are migrated, not deleted — see below)." Leave the third bullet as-is.

**Resolution**: Approved
**Notes**: Reworded the first Testing bullet — "remove" now attaches to the hand-authored format strings, with an explicit "(the cases themselves are migrated, not deleted — see below)" cross-reference. Third bullet left as-is.

---

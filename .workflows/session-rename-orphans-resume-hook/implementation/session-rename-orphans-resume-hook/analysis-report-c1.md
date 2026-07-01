---
topic: session-rename-orphans-resume-hook
cycle: 1
total_findings: 7
deduplicated_findings: 5
proposed_tasks: 5
---
# Analysis Report: session-rename-orphans-resume-hook (Cycle 1)

## Summary
The core fix (rename-immune `@portal-id` hook-key derivation + cross-reboot persistence) conforms to spec across all four hook-key stages and Acceptance Criteria 1-8; all three agents confirmed the production path is correct with no live bug. The actionable findings are one structural defect on the fix's own persistence seam (`findOrAppendSession` drops `PortalID` — flagged independently by standards and architecture) plus four smaller maintainability items: two doc-drift clusters still naming the retired `ListAllPanes` enumeration, a missing fast static byte-identity guard for the three `@portal-id` literals, and an intra-package triplicated test constant. All findings were verified against the actual code before proposing.

## Deduplication notes
- **`findOrAppendSession` drops PortalID** — standards (`capture.go:260-265`, severity low, "currently-inert") and architecture (`capture.go:254-266`, severity medium, "latent re-orphan trap") independently flagged the SAME defect. Merged into one task (Task 1). Reconciled severity: **medium**. Verified: the append branch IS unreachable today (`mergeSkippedPanes` at capture.go:187 `continue`s past any prev session absent from `live`, and `live` is built from `fresh.Sessions`, so the "found" branch at capture.go:256-257 always returns first). But the partial struct-copy contradicts the helper's own "shallow copy of ps" doc-comment, sits directly on the persistence seam the whole fix depends on, and is one `sessionLive`-gate relaxation away from silently re-orphaning a hook. Standards' "low" under-weighted the seam location; architecture's "medium" is correct. No test exercises the merge path with the new field.
- **Stale doc-comment at `cmd/bootstrap_production.go:71`** — standards (medium) and architecture (low) both flagged the SAME line (`cleanStaleAdapter` prose naming `ListAllPanes` where the interface now requires `ListAllPaneHookKeys`). Merged into Task 2. Severity **medium**: the spec made retiring these name-based doc-comments an explicit deliverable precisely to stop pointing future callers back at name-based keying; this is the one residual on the hook-cleanup adapter.

## Proposed Tasks
1. Copy PortalID in findOrAppendSession append branch (medium) — sources: standards, architecture
2. Fix stale ListAllPanes doc-comment on cleanStaleAdapter (medium) — sources: standards, architecture
3. Update ListAllPanes prose in the shared stale-cleanup helper (low) — sources: standards
4. Add a fast static byte-identity guard for the three @portal-id literals (low) — sources: architecture
5. Collapse the triplicated @portal-id test constant in the tmux_test package (low) — sources: duplication

## Discarded Findings
None. All findings were confirmed against the code and are actionable. The low-severity items were retained rather than discarded because they cluster around one coherent theme — protecting the fix's central byte-identity / name-vs-hook-key invariants against future drift — which is the spec's explicitly-stated primary risk ("Missed key-producing site").

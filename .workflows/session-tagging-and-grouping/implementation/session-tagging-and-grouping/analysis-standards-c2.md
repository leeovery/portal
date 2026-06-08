# Analysis — Standards — Cycle 2

STATUS: findings
FINDINGS_COUNT: 1 (1 low)

## FINDING: Render-layer heading lines are drawn but not counted by Height(), causing pagination imprecision
- SEVERITY: low
- FILES: internal/tui/session_item.go:119-146 (Render), internal/tui/session_item.go:94-98 (Height)
- DESCRIPTION: SessionDelegate.Render prepends a group-heading line within a single Render write while Height() stays fixed at 1. The spec (§ Group headers, build note) mandates the render-layer separator approach precisely so headers are never list items — which the implementation honours correctly. However the spec does not explicitly bless the resulting pagination drift (each rendered heading consumes a terminal row bubbles/list does not account for in Height()-based page math), so on a tall grouped list the last item(s) of a page can be pushed under the footer. The in-source comment acknowledges this as an accepted v1 tradeoff versus routing through lipgloss/tree (which the spec forbids).
- RECOMMENDATION: No code change required if the accepted-tradeoff stance holds (it was a documented decision in task 2-5). If pagination correctness becomes user-visible at ~15-20 sessions plus headings, reserve heading rows in the list-height computation (applySessionListSize) rather than per-item Height(), preserving the no-lipgloss/tree constraint. NO ACTION recommended — accepted v1 tradeoff.

SUMMARY: Implementation conforms across all 4 phases and all 15 acceptance criteria. Verified the lazy-resolver wiring is now correct (rebuildSessionList → resolveSessionDirs → ResolveAndStampDir; fast path short-circuits; derive-use-first/stamp-as-side-effect ordering; best-effort swallowed stamp never drops a session). Canonical dir-key matching robust (both Session.Dir and stored Project.Path re-run through CanonicalDirKey at compare time). Tag normalisation, unconditional three-mode cycle, Pattern A/B grouping, pinned+suppressed catch-alls, prefs tolerant decode, "No tags yet" signpost, flatten-on-filter, s-as-literal-filter guard, ProjectsLoadedMsg re-group all match spec. Conventions respected (no t.Parallel, prefs logs nothing/leaf, tag mutations audit under projects with via=cli, prefs.json in configFilePath/migrate). Only finding: the low, already-documented pagination tradeoff inherent to the spec-mandated render-layer-header design.

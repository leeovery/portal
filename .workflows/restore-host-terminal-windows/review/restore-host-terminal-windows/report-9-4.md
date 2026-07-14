TASK: restore-host-terminal-windows-9-4 — Unify the footer narrow-degrade fitter across the standard and multi-select footers (fitClusterToWidth)

ACCEPTANCE CRITERIA:
- The try-full-then-greedy-prefix-with-ellipsis loop exists exactly once (inside the shared helper); both fitLeftCluster and fitFilterCluster delegate to it.
- The per-type cluster renderers (renderFilterCluster / renderFooterCluster) and each fitter's budget computation (full width vs right-anchor-reserved) remain caller-owned and unchanged in behaviour.
- Fitted output (cluster + exact rendered width, always <= budget) for both footers is byte-identical to today across wide, narrow-degrade, single-entry-plus-ellipsis, and extreme-narrow cases.

STATUS: Complete

SPEC CONTEXT: This is a low-severity duplication-source analysis-cycle task. The §2.7 narrow-degrade truncation rule (a footer degrades on ONE line, dropping trailing lower-priority entries with an ellipsis, never wrapping) was implemented twice — once in the pre-existing fitLeftCluster (standard/Projects keymap footer) and once in fitFilterCluster (added by task 5-4 for the multi-select footer). The two were near-line-for-line copies differing only in entry type, cluster renderer, and whether a right anchor budget is reserved. The task extracts the shared loop so the layout invariant cannot drift between the standard and multi-select footers.

IMPLEMENTATION:
- Status: Implemented (correct, matches all three acceptance criteria)
- Location: internal/tui/footer.go:198-245 (new shared fitClusterToWidth helper), :187-196 (fitFilterCluster delegates, full-width budget), :347-366 (fitLeftCluster delegates, right-anchor-reserved budget). Cluster renderers unchanged: renderFooterCluster (footer.go:371-383) and renderFilterCluster (filter_footer.go:175-190).
- Notes: The refactor commit (a3a97c82) diff confirms the shared helper reproduces the pre-refactor algorithm verbatim (full-first fast path -> for n := 1; n <= count greedy-prefix loop appending sep+ellipsis -> bare-ellipsis fallback -> empty). The loop now exists exactly once (verified: grep for "for n := 1; n <=" in non-test tui/*.go returns only footer.go:224). Both fitters delegate; each keeps its own budget computation and closure over its own renderer. Cluster renderers and the right-anchor budget difference remain caller-owned. No behavioural drift — the algorithm, break condition, and fallback order are identical to the two former copies.

TESTS:
- Status: Adequate (well-balanced; not over-tested)
- Coverage: internal/tui/footer_fit_test.go adds: (1) TestFitClusterToWidth_AlgorithmAcrossWidthRegimes — drives the shared helper directly with deterministic plain-string clusters across all four regimes (wide/full, narrow prefix+"· …", ellipsis-only, sub-ellipsis empty), asserting exact string, width, and width <= budget; (2) TestFitClusterToWidth_EmptyClusterCount — the count==0 fast path edge; (3) TestFitFilterCluster_MatchesSharedHelperAcrossWidths and (4) TestFitLeftCluster_MatchesSharedHelperAcrossWidths — drive each production fitter across all four regimes and assert byte-identity against referenceFitCluster (an independent copy of the pre-refactor algorithm), pinning the caller wiring (budget + renderer + sep/ellipsis) as well as the shared loop. fitLeftCluster's case exercises both the no-right-anchor (full width) and reserved-right-anchor (w - rightWidth - 1) budgets. Existing end-to-end regression coverage is intact and exercises the degrade path through the real render entry points: footer_test.go:227 TestSessionsFooter_NarrowTruncationNoWrap + :258 priority-ordered drop, and multi_select_footer_test.go:94 TestMultiSelectFooter_NarrowDegradeOneLineEllipsis (no-wrap, no-overflow, ellipsis marker, priority-ordered drop).
- Notes: The referenceFitCluster golden-reference duplicate is the appropriate technique for a behaviour-preserving refactor — it pins byte-identity against the old algorithm independently of the new shared helper (so a shared-helper regression cannot silently pass). This is not redundant over-testing. The four acceptance-criteria regimes are each covered exactly once at the unit level and again through the real footers; no bloated or implementation-detail assertions.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (correctly noted and honoured — matches the cmd/tui package-level-state constraint). Small closure-based seam is idiomatic Go and consistent with the file's existing DI-via-closure style.
- SOLID principles: Good. The shared helper has a single responsibility (the degrade loop); per-type rendering and budget policy are injected by the caller (open/closed — a third footer could reuse it). This task IS the DRY fix and lands cleanly.
- Complexity: Low. The helper is a linear scan with three clear fallback tiers; no added branching.
- Modern idioms: Yes — closure parameter `renderCluster func(n int) (string, int)` returns cluster+width so the helper never re-measures.
- Readability: Good. Doc comments accurately describe the shared-vs-caller-owned split and are kept in sync on both fitters.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/footer.go:210 — the shared helper's second parameter is named `w` but callers pass a computed budget (fitLeftCluster passes its right-anchor-reserved `budget`, not the raw width). Rename the param `w` -> `budget` in the signature and body for clarity; the doc comment already calls it "the width budget w". Pure mechanical local rename, zero logic impact.
- [idea] internal/tui/footer.go:189-190,359-360 — pre-refactor, sep/ellipsis were rendered lazily only inside the narrow-degrade path (after the full-fit check); both fitters now render them eagerly before delegating, adding two renderFooterDetail (lipgloss Render) calls per footer render in the common wide-terminal case. Impact is negligible (footer renders once per frame) and the eager-string design is mandated by the task ("computing sepWidth/ellipsisWidth from the passed strings"). Only worth revisiting if the pre-rendered-string contract is ever relaxed to closures; no action now.

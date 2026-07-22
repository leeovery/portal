TASK: persistent-no-host-terminal-banner-1-1 — Add IsNull() Discriminator To unsupportedBannerActive()

ACCEPTANCE CRITERIA:
- unsupportedBannerActive() returns false for a resolved-unsupported NULL identity (empty BundleID) and true for a resolved-unsupported named identity (non-empty BundleID), both outside multi-select mode.
- On a resolved-NULL model, applySectionHeader renders the standard 'Sessions ... N' header (no banner) at the section-header row.
- On a resolved-NULL By-Tag model with zero tags anywhere, activeNoticeBand() returns ok == true (the 'no tags yet' signpost owns the slot).
- On a resolved-named model, the banner still replaces the header and the signpost is still suppressed — existing named-path tests stay green.
- In-flight detection and supported terminals still render the standard header — existing tests stay green.
- No new detection dispatch, cache invalidation, or re-detection is introduced; rebuildSessionList is unchanged.
- go test ./internal/tui/... passes (unit lane — no tmux daemon spawned).

STATUS: complete

SPEC CONTEXT:
Spec §2 (Sub-fix 1 — Banner Split by Identity Shape) prescribes adding an IsNull() discriminator to unsupportedBannerActive() so the proactive unsupported-terminal banner is named-only: NULL/remote (mosh/SSH) clients keep their standard 'Sessions ··· N' header and their By-Tag signpost, because a NULL banner carries nothing actionable (no bundle id, no 'see docs'). §2 explicitly notes the single gate is read by TWO consumers (applySectionHeader swap + activeNoticeBand signpost suppression), so the one-line change fixes both coherently. §2 "Scope guard" is emphatic that the once-only detection cache is NOT the defect and must be left untouched (no re-detection on rebuild). §7 (Testing) requires inverting the old NULL honest-line test to assert the standard header, and adding a NULL-signpost-return test mirroring the named suppression sibling. NULL render-branch deletion is deferred to Task 1.2 (§6). Note: current HEAD is 7f6d5fd ("complete implementation") which carries all phase tasks; task 1.1's specific deliverables (the gate predicate at model.go:4736-4738 and its two tests) are present and untouched by later tasks — verified against commit f0e32aa5.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:4736-4738 — unsupportedBannerActive() now returns `m.DetectUnsupported() && !m.multiSelectMode && !m.detectIdentity.IsNull()`. Exactly the prescribed predicate; the retained `!m.multiSelectMode` clause preserves multi-select outranking.
  - internal/tui/model.go:4724-4735 — doc comment rewritten to state the banner is NAMED-ONLY and explain the !IsNull() discriminator keeps NULL/remote off the header row; still documents the single-predicate-two-consumers invariant.
  - internal/tui/model.go:4862-4872 — applySectionHeader inline comment reworded to "NAMED-ONLY" and correctly references that the now-unreachable NULL render branch is removed in Task 1.2 (not here).
- Notes: DetectUnsupported() (spawn_detect.go:117-119) is resolution-based (true for both NULL and named-undriven), so IsNull() is correctly layered ON TOP of it as a second discriminator rather than replacing it — matches the task CONTEXT note. Identity.IsNull() (internal/spawn/identity.go:24) is `BundleID == ""`. The change is scoped to model.go + the test file; no touch to the detectDispatched latch, detectResolved caching, or rebuildSessionList (confirmed by diff). Both live consumers verified: applySectionHeader (model.go:4873) and activeNoticeBand (notice_band.go:371) read the same predicate.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/unsupported_banner_test.go:216-237 — TestApplySectionHeader_UnsupportedNullShowsStandardHeader (reworked + renamed from ...ShowsHonestLine). Keeps the unsupportedResolvedModel(t, spawn.Identity{}) setup and the `if !m.DetectUnsupported()` precondition (still true — NULL resolves unsupported via the production resolver, confirmed by TestDetection_Unsupported_Predicate line 165). Now asserts 'Sessions' present and the absence of 'no host-local terminal' / 'unsupported terminal' / 'see docs'. Covers acceptance criteria 1 (named=false→NULL path) and 2 (standard header renders).
  - internal/tui/unsupported_banner_test.go:324-346 — TestActiveNoticeBand_NullReturnsSignpost (new), mirrors TestActiveNoticeBand_SuppressesSignpostWhenUnsupported. Seeds resolved-unsupported NULL directly on signpostModel(t), asserts unsupportedBannerActive()==false AND activeNoticeBand() ok==true. This drives the SECOND consumer through the real predicate (notice_band.go:371), covering acceptance criterion 3. Since activeNoticeBand's signpost arm calls unsupportedBannerActive(), the ok==true assertion is a genuine end-to-end check, not a tautology.
  - Existing named-path guards stay valid and unmodified: TestApplySectionHeader_UnsupportedShowsBanner (named→banner), TestActiveNoticeBand_SuppressesSignpostWhenUnsupported (named→signpost suppressed), TestApplySectionHeader_InFlightShowsStandardHeader, TestApplySectionHeader_SupportedShowsStandardHeader, TestApplySectionHeader_MultiSelectStepsUnsupportedAside (asserts unsupportedBannerActive()==false in the mode). These cover the remaining acceptance criteria (named unchanged, in-flight/supported unchanged, multi-select outranks).
- Notes: Not under-tested — both consumers of the changed gate are exercised for the NULL shape, and the named/in-flight/supported/multi-select branches remain guarded. Not over-tested — the two new/reworked assertions map 1:1 to distinct acceptance criteria (header render vs signpost slot) and reuse the established helper patterns. The direct field-seeding in the signpost test (m.detectResolution/detectResolved) mirrors the pre-existing sibling exactly, so it introduces no new convention. Test-adequacy assessed by reading only (no suite executed, per instructions).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (per package convention, re-stated in the file header). Uses the shared warmResolvedModel/signpostModel/bannerFirstLine helpers and the production resolver seam rather than hand-rolling detection state where a helper exists. Test names are descriptive behaviour statements per golang-testing/golang-naming conventions.
- SOLID principles: Good. Single predicate remains the single source of truth for both consumers (the doc comment explicitly protects this "can never drift" invariant); no duplication of the identity-shape decision at call sites.
- Complexity: Low. One added boolean conjunct; no new branches, no control-flow change.
- Modern idioms: Yes. Idiomatic Go; boolean short-circuit; value receiver preserved.
- Readability: Good. The doc comment and the applySectionHeader inline comment are accurate, explain WHY (nothing actionable for NULL), and forward-reference Task 1.2 for the render-branch removal without doing it here.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

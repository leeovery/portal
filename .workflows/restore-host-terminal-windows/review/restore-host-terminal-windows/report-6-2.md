TASK: restore-host-terminal-windows-6-2 — Proactive unsupported/NULL terminal banner + notice-band slot

ACCEPTANCE CRITERIA:
- Resolved-unsupported outside multi-select (NULL remote/mosh OR non-NULL undriven like com.apple.Terminal) → section-header row renders the unsupported banner (named `⚠ unsupported terminal — <name> · <bundleID>` + blue `see docs`; honest `⚠ no host-local terminal` for NULL), and `Sessions ··· N` is not shown.
- Detection in-flight (dispatched && !resolved) → standard `Sessions ··· N` header (no banner).
- Resolved supported (native/config) → no banner (non-NULL is NOT sufficient; unsupported is).
- Entering multi-select → violet `N selected` banner replaces the unsupported banner (steps aside).
- By-Tag "No tags yet" signpost does not render while the unsupported banner owns the row.
- NO_COLOR → `⚠`, label, identity, `see docs` render on native fg/bg, no hue, no canvas fill.
- Banner matches design/sessions-unsupported-terminal.png.

STATUS: Complete

SPEC CONTEXT:
Spec §268 gives the exact banner copy `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` + `see docs` (copy-paste the detected bundle id — "show it, never guess"). §253 pins the honest `no host-local terminal` no-op for a purely-remote/NULL trigger. §138 fixes the notice-band precedence (single slot, highest wins): filter line → in-burst Opening → transient error/guidance flash → multi-select banner → unsupported-terminal banner → no-tags signpost; on an unsupported terminal, entering multi-select shows the multi-select banner (unsupported steps aside) and the warning re-asserts at the N≥2 Enter block (6-9, out of scope here). §288 distinguishes in-flight (awaited) from resolved-NULL, so the banner must gate on resolution, not IsNull(). §505 references the delivered frame.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/section_header.go:153-232 — renderUnsupportedHeader + unsupportedLeftCluster; constants unsupportedLabel / unsupportedNullLabel / unsupportedDocsHint / unsupportedIdentityDash / unsupportedIdentityMiddot (lines 56-75). Routes through the shared renderRightAnchoredSectionRow so alignment / canvas flex-spacer / §2.7 narrow-degrade match the standard header exactly; no `▌` left-bar.
  - internal/tui/model.go:4705-4773 — applySectionHeader inserts the unsupported claimant gated on m.unsupportedBannerActive(), placed AFTER multiSelectMode and BEFORE FilterApplied (precedence-correct). model.go:4666-4668 — unsupportedBannerActive() = DetectUnsupported() && !multiSelectMode, the single shared predicate.
  - internal/tui/notice_band.go:371 — activeNoticeBand suppresses the By-Tag signpost via `&& !m.unsupportedBannerActive()`; the transient-flash arm (line 361) stays first, so a flash still wins the slot.
  - internal/tui/spawn_detect.go:109-118 — DetectUnsupported() = detectResolved && detectResolution == ResolutionUnsupported (resolution-based, NOT IsNull()); consumed here, defined in 6-1.
- Notes: The banner branches on identity shape via `bundleID == ""`, which is exactly spawn.Identity.IsNull() (internal/spawn/identity.go:24-25) — so the named/NULL split cannot drift from the canonical NULL test. The named copy matches the delivered frame byte-for-byte (`⚠ unsupported terminal — Apple Terminal · com.apple.Terminal`, em-dash U+2014 + middot U+00B7). The `!m.multiSelectMode` clause in unsupportedBannerActive() is technically redundant inside applySectionHeader (the multiSelectMode branch already returned), but it is load-bearing in activeNoticeBand and the task explicitly mandates one shared predicate to prevent drift — intentional, not a defect. Rendered against design/sessions-unsupported-terminal.png: identity naming, amber label, dim identity, right-anchored blue `see docs`, single row over the untouched list — all match.

TESTS:
- Status: Adequate
- Coverage (internal/tui/unsupported_banner_test.go):
  - Named render: exact copy, amber label run (AccentOrange), dim identity run (TextDetail), blue `see docs` run (AccentBlue), dark + light (TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs).
  - NULL branch: honest `⚠ no host-local terminal`, no identity, no `see docs`, no named copy (TestUnsupportedHeader_NullIdentityNoHostLocal).
  - Right-alignment + exact content width (RightAlignedSeeDocs); single row for named + NULL (ExactlyOneRow); §2.7 narrow-degrade drops the hint without overflow, cluster survives (NarrowDegradeDropsHint); NO_COLOR glyph-backed with no canvas/fg sequences (ColourlessGlyphBacked); canvas paint / no edge bleed (PaintsCanvasNoEdgeBleed).
  - applySectionHeader precedence, driven through the real 6-1 detection path: unsupported non-NULL shows banner + no Sessions (UnsupportedShowsBanner); NULL honest line (UnsupportedNullShowsHonestLine); in-flight → standard header (InFlightShowsStandardHeader); supported ghostty → standard header (SupportedShowsStandardHeader); multi-select steps unsupported aside (MultiSelectStepsUnsupportedAside).
  - Signpost suppression while unsupported (SuppressesSignpostWhenUnsupported) and flash-outranks-unsupported (FlashOutranksUnsupported).
  - Each acceptance criterion maps to at least one test; a test would fail if the feature broke (copy, colour role, precedence, degrade, NO_COLOR all pinned).
- Notes: Focused, not over-tested — no redundant happy-path duplication; the dark/light and named/NULL table cases cover distinct code paths. One minor gap: no test negatively asserts the banner carries no `▌` notice-bar (the task explicitly calls out "NO `▌` left-bar"). The exact-copy assertion uses Contains, not HasPrefix, so a stray leading `▌` would not be caught — though it is structurally impossible given the shared barless renderRightAnchoredSectionRow path.

CODE QUALITY:
- Project conventions: Followed. Mirrors the sibling section-header claimants (renderMultiSelectHeader, renderOpeningBand, renderPreflightAbortHeader) — all route through renderRightAnchoredSectionRow; closed MV token vocabulary only (AccentOrange / TextDetail / AccentBlue), no literal hex; single-source wording constants; NO_COLOR carve-out honoured via headerStyle.
- SOLID principles: Good. renderUnsupportedHeader is a pure render; the identity-shape branch is isolated in unsupportedLeftCluster; the resolution predicate is a single shared unsupportedBannerActive() consumed by both applySectionHeader and activeNoticeBand.
- Complexity: Low. Linear branch on bundleID; precedence chain is a flat sequence of early returns.
- Modern idioms: Yes.
- Readability: Good. Doc comments cite the spec sections and explain the IsNull-vs-resolution decision and the co-render/step-aside distinctions.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/unsupported_banner_test.go:43 — Add a negative assertion (e.g. in TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs / _NullIdentityNoHostLocal) that `strings.Contains(header, noticeBarGlyph)` is false, pinning the task's explicit "NO `▌` left-bar" requirement, which currently has no direct test (the Contains-based copy check would not catch a stray leading bar).

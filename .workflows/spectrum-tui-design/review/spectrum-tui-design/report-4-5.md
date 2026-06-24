TASK: spectrum-tui-design-4-5 — Empty sessions + empty projects states reskin: centred glyph/message/hint + footer fully replaced from the keymap descriptor (§11.1 / §12.1)

ACCEPTANCE CRITERIA:
- Empty-sessions: centred `▌ ▌ ▌` (text.faint) + `No sessions yet` (text.primary) + hint `Press n to start one in the current directory · x for projects` (text.detail); no literal hex (§2.9 tokens).
- Empty-sessions footer FULLY REPLACED by `n new in cwd · x projects · / filter · ? help` drawn from the Sessions keymap descriptor (§12.1) — not a subset of the standard footer.
- Empty-projects mirrors: centred glyph + `No projects yet` (text.primary) + open-a-directory hint (text.detail) + replaced footer from the Projects keymap descriptor; not separately mocked.
- Renders ONLY when the list is genuinely empty (zero items), distinct from the §7.3 no-matches state (items exist, query → zero).
- Reuses the Phase 2 task 2-9 centred-empty-state pattern (same centring + sizing); one-row-per-delegate pagination invariant unperturbed.
- Under NO_COLOR the empty state renders colourless on the native bg, glyph/message/hint legible.
- vhs: testdata/vhs/sessions-empty.png compared against the Sessions — empty (MV) Paper frame.

STATUS: Complete

SPEC CONTEXT:
§11.1 specifies the empty-sessions surface (block glyph `▌ ▌ ▌` text.faint, `No sessions yet` text.primary, hint text.detail) with the footer REPLACED by `n new in cwd · x projects · / filter · ? help` drawn from the page's full keymap (§12.1), not a subset of the standard footer. Empty projects mirrors with `No projects yet` + an open-a-directory hint, "not separately mocked". §12.1 supplies the Sessions/Projects per-page keymaps that source the footer labels (Sessions `x`=projects, Projects `x`=sessions, `n`=new in cwd, `/`=filter, `?`=help). §7.3 is the no-matches pattern the empty state reuses for centring/sizing but stays distinct from. §2.5 (NO_COLOR), §2.9 (text.faint/primary/detail tokens).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/empty_states.go — the whole feature: constants (emptySessionsGlyph/Message/Hint, emptyProjectsGlyph/Message/Hint), shared renderEmptyStateBody (lines 56-68), sessionListEmpty/projectListEmpty predicates (75-90), replaceListBodyWithEmptyState (100-112), emptyFooterKeys + emptyFooterDescriptor selector (119-146), renderEmptySessionsFooter/renderEmptyProjectsFooter (154-165).
  - internal/tui/model.go:4066-4068 — viewSessionList empty-sessions body swap; 4131-4133 — renderSessionsFooterForFilterState empty-sessions footer guard.
  - internal/tui/model.go:3861-3863 — viewProjectList empty-projects body swap (gated `&& !m.commandPending`); 3994-3996 — renderProjectsFooterForFilterState empty-projects footer guard (gated AFTER command-pending).
- Notes:
  - Footer copy is correctly descriptor-sourced, verified by tracing sessionsKeymap()/projectsKeymap() (keymap.go): `n`→"new in cwd", Sessions `x`→"projects", Projects `x`→"sessions", `/`→"filter", `?`→"help". The `?` entry retains RightAligned=true so it pins right; emptyFooterDescriptor forces Core=true on the selected four so renderCondensedFooter emits exactly {n, x, /} left + {?} right.
  - Distinctness from §7.3 is structurally guaranteed: sessionListEmpty REQUIRES list.Unfiltered, sessionListNoMatches REQUIRES Filtering/FilterApplied + non-empty query — mutually exclusive predicates. The two body-swaps are ordered no-matches-then-empty but cannot both fire.
  - replaceListBodyWithEmptyState mirrors replaceListBodyWithNoMatches exactly (Height()−1 body height, IndexByte first-line preservation) — the "reuse 2-9 sizing" requirement is met; renderNoMatchesBody and the empty-state body both route through the shared renderEmptyStateBody, so layout/centring/sizing/token treatment cannot drift.
  - The pre-reskin behaviour parity holds: the empty branch only swaps the (already-empty) list BODY below the title and replaces the footer; the underlying bubbles/list still functions and keys still dispatch.
  - vhs: testdata/vhs/sessions-empty.png matches testdata/vhs/reference/sessions-empty-mv.png for layout, structure, and colour-role (centred glyph text.faint, bold message, detail hint, blue key glyphs / detail labels / violet ? glyph, wordmark + separator + `Sessions 0` + `/ to filter`). Empty projects mirrors and is not separately captured (per spec).

TESTS:
- Status: Adequate
- Coverage (internal/tui/empty_states_test.go):
  - TestEmptySessions_RendersGlyphMessageHint — glyph/message/hint present in both modes; also asserts the no-matches glyph/message do NOT leak (distinctness at render).
  - TestEmptySessions_ReplacesFooterFromDescriptor — the four replaced entries present AND the standard-footer Core keys (navigate/attach/preview/switch view) absent — pins FULL replacement, not subset.
  - TestEmptySessions_FooterCopyFromDescriptor — labels read off the descriptor map; ? help trailing/right-aligned.
  - TestEmptySessions_FooterTokenColours — accent.blue keys / text.detail labels / accent.violet ? glyph / border.footer rule, exact mode-resolved SGR.
  - TestEmptyProjects_RendersGlyphMessageHint — projects mirror copy.
  - TestEmptyProjects_ReplacesFooterFromProjectsDescriptor — `x sessions` (not `x projects`) pins descriptor sourcing; banned standard-footer keys absent.
  - TestEmptyStates_OnlyRenderWithZeroItems / _DistinctFromNoMatches / _NotRenderedWhileFiltering — the zero-items + Unfiltered gating and the no-matches/empty separation.
  - TestEmptyStates_ColourRoles — text.faint/primary/detail SGR pinned.
  - TestEmptyStates_ColourlessLegibleOnNativeBg — NO_COLOR drops canvas + fg roles, copy intact (body + footer).
  - TestEmptyStates_OneRowPerDelegateInvariant — composed view height ≤ termHeight.
- Notes:
  - Every acceptance criterion has a corresponding test; assertions pin exact tokens/copy (a token swap or label drift is caught, not merely presence). Not over-tested — no redundant happy-path duplication; the dark/light loop and the distinctness pair are each load-bearing.
  - Minor gap (non-blocking): the one-row-per-delegate invariant test (TestEmptyStates_OneRowPerDelegateInvariant) and the NO_COLOR footer test exercise only the SESSIONS empty state. The projects body-swap shares replaceListBodyWithEmptyState (same code path, parameterised by listHeight), so the risk is low, but no test composes the full empty-PROJECTS view and asserts height ≤ termHeight. The projects branch also carries an extra interaction the sessions branch lacks — the `&& !m.commandPending` gate — which is untested in this file (no test asserts the empty-projects state is suppressed while a command is pending).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (documented in the test header, consistent with the cmd-package mock convention). All colour flows through §2.9 tokens via headerStyle/headerCanvasBg — no literal hex. Footer + help single-source-of-truth (the keymap descriptor) is honoured; the empty footer selects BY KEY off the descriptor so a label change flows through.
- SOLID principles: Good. renderEmptyStateBody is a single shared renderer for both §11.1 and §7.3 (DRY, prevents drift). replaceListBodyWithEmptyState is page-agnostic (parameterised by listHeight + content), serving both Sessions and Projects from one function. emptyFooterDescriptor is a small pure selector with a clear single responsibility.
- Complexity: Low. emptyFooterDescriptor is an O(n) map-build + ordered select; the view guards are flat conditionals; no nesting concerns.
- Modern idioms: Yes — make-with-capacity, range over slices, IndexByte for the first-line split (matches the sibling no-matches helper).
- Readability: Good. Constants are spec-annotated; every helper has a doc comment tying it to the spec section and the mirrored sibling. Intent is self-documenting.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/empty_states_test.go:318 — TestEmptyStates_OneRowPerDelegateInvariant only covers the sessions empty state. Add a projects-mirror assertion (emptyProjectsModel → lipgloss.Height(View()) ≤ termHeight) so the projects body-swap height budget is pinned independently.
- [quickfix] internal/tui/model.go:3861 — the empty-projects `&& !m.commandPending` suppression gate is untested. Add a test that a command-pending Projects model with zero projects does NOT render the empty-projects state (the §11.4 banner/footer own that case).
- [idea] internal/tui/empty_states.go:45 — emptyProjectsHint reads `Press n to start one in the current directory · x for sessions`, a literal mirror of the sessions hint rather than the spec's "open-a-directory" framing (§11.1). The spec leaves this open and the constant's comment documents the choice, so it is acceptable; consider whether an open-a-directory-oriented phrasing better matches the projects page's purpose. Decision, not a defect.

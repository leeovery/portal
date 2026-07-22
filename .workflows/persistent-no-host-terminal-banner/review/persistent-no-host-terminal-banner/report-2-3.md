TASK: persistent-no-host-terminal-banner-2-3 — Help-Modal m-Suppression At The Sessions Call Site

ACCEPTANCE CRITERIA:
- On a resolved-unsupported terminal (both appleTerminalIdentity() and spawn.Identity{}), NOT in multi-select, m.sessionsHelpKeymap() contains NO entry with Key == 'm', and the rendered help body omits the 'Multi-select mode' label.
- On a supported terminal (ghosttyIdentity()), m.sessionsHelpKeymap() contains the Key == 'm' entry, and the rendered help body lists 'Multi-select mode'.
- On a resolved-unsupported terminal WHILE in multi-select mode (m.multiSelectMode = true), m.sessionsHelpKeymap() LISTS m.
- The condensed Sessions footer never lists m under any resolution (m is non-Core) — renderSessionsFooter output contains no multi-select label, supported and unsupported.
- sessionsKeymap() remains a pure static constant (unchanged) — only the call-site copy is filtered.
- TestSessionsDescriptorDispatchParity stays green (detection unwired → filter inert → static descriptor still advertises m).
- The Projects help call site is unchanged; projectsKeymap() unaffected.
- go test ./internal/tui/... passes (unit lane).

STATUS: complete

SPEC CONTEXT:
Spec §4 (Sub-fix 3) requires filtering the `m` entry out of the descriptor slice fed to the Sessions help modal AT THE CALL SITE when `DetectUnsupported() && !m.multiSelectMode`, keeping sessionsKeymap() a pure static constant. The predicate is exactly "`m` appears in `?` help iff `m` is functional" — the `&& !m.multiSelectMode` leg keeps `m` listed in the A1 in-flight state (detection resolves unsupported while multi-select already open — a live row-toggle that must never be hidden). §4/§111 note guard-safety depends on sessionsGuardModel (NewModelWithSessions) keeping detection unwired so DetectUnsupported() is false and the filter is inert. §7 requires the three cover cases (a/b/c) plus guard-stays-green.

IMPLEMENTATION:
- Status: Implemented — matches the task DO block and spec §4 verbatim.
- Location:
  - internal/tui/model.go:4750-4773 — new `func (m Model) sessionsHelpKeymap() []keymapEntry`. Copies sessionsKeymap(), computes `mBlocked := m.DetectUnsupported() && !m.multiSelectMode`, returns the raw slice when not blocked, else filters out the `Key == "m"` entry into a capacity-presized slice. Predicate and filter exactly as specified.
  - internal/tui/model.go:4598 — Sessions modalHelp case now calls `renderHelpModalOnClearedCanvas(m.sessionsHelpKeymap(), …)`; the rest of the call (m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless) unchanged.
  - internal/tui/model.go:4413 — Projects modalHelp case still `renderHelpModalOnClearedCanvas(projectsKeymap(), …)` — UNTOUCHED (confirmed).
  - internal/tui/keymap.go:89-105 — sessionsKeymap() unchanged; still a pure static constant listing the `{Key: "m", Action: "multi-select", HelpAction: "Multi-select mode"}` non-Core entry (line 97).
- Notes: sessionsKeymap() was verified byte-unchanged by this commit (git show touched only model.go + the new test file for source). The filter lives purely at the call site as mandated — the descriptor function was NOT parameterised. Thorough in-source doc comment on the helper records the call-site rationale and the guard-safety dependency (aligns with spec §111 intent). Only the two expected call sites exist for renderHelpModalOnClearedCanvas with a keymap descriptor; grep confirms no missed Sessions help site.

TESTS:
- Status: Adequate — one focused test per acceptance criterion, no redundancy.
- Coverage (internal/tui/help_modal_m_suppression_test.go, package tui, no t.Parallel):
  - TestSessionsHelpKeymap_UnsupportedNotInMultiSelect_OmitsM — case (a); table over named (com.apple.Terminal) + NULL (spawn.Identity{}) shapes; asserts preconditions (DetectUnsupported true, not in multi-select), keymapHasMKey false, and ansi-stripped helpModalBody omits "Multi-select mode".
  - TestSessionsHelpKeymap_Supported_ListsM — case (b); ghosttyIdentity, asserts !DetectUnsupported, keymapHasMKey true, body lists the label.
  - TestSessionsHelpKeymap_UnsupportedInMultiSelect_ListsM — case (c); named + NULL shapes, sets m.multiSelectMode = true, asserts keymapHasMKey true and body lists the label.
  - TestSessionsFooter_NeverListsMultiSelect — footer guard over supported + named + NULL; asserts renderSessionsFooter output (lowercased) contains no "multi-select".
  - TestSessionsKeymap_StaticConstantUnaffectedByFilter — asserts sessionsKeymap() always lists m (static-constant invariant).
- Not under-tested: all three §7 cases plus the footer-never-lists-m guard and the static-constant invariant are covered; identity-blind predicate is exercised via both NULL and named shapes.
- Not over-tested: each test maps to a distinct criterion; the two-shape tables are justified (the predicate is DetectUnsupported(), identity-blind, so both shapes must suppress in (a) and both must list in (c)).
- Would fail if the feature broke: parameterising sessionsKeymap() to drop m → test #5 fails; wrong predicate (drop-always) → (b)/(c) fail; filter never applied → (a) fails. Guard-green claim is structurally sound: TestSessionsDescriptorDispatchParity asserts against `sessionsKeymap()` (the static descriptor, line 184) and sessionsGuardModel uses NewModelWithSessions which leaves detection unwired → DetectUnsupported() false → filter path never reached.
- Notes: render-level assertions use helpModalBody (one of the two forms the task permitted). multiSelectHelpLabel constant is single-sourced against the descriptor's HelpAction to prevent drift. keymapHasMKey keys off Key == "m" (the footer glyph), matching the descriptor. Test execution not performed (per role constraints); adequacy judged by reading — helpers (unsupportedResolvedModel, appleTerminalIdentity, ghosttyIdentity, helpModalBody, renderSessionsFooter, sectionHeaderWidth) all exist and are used by other passing tests, so the file compiles.

CODE QUALITY:
- Project conventions: Followed. Value receiver `(m Model)` consistent with sibling predicates (DetectUnsupported, unsupportedBannerActive). No t.Parallel() (mandatory for the cmd/tui mock convention). Test file placed adjacent, white-box package tui.
- SOLID principles: Good. Single-responsibility helper; the filter is isolated from both the descriptor source and the render path.
- Complexity: Low. One boolean gate, an early return, and a single linear filter loop.
- Modern idioms: Yes. Capacity-presized `make([]keymapEntry, 0, len(entries))`, early-return guard.
- Readability: Good. Intent-revealing `mBlocked` local; comment explains the predicate, the A1 exemption, and the call-site-vs-descriptor guard rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

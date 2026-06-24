TASK: spectrum-tui-design-3-4 — `?` help modal (new): help-modal type + generic descriptor-driven two-column renderer + bind `?` on Sessions and Projects

ACCEPTANCE CRITERIA:
- `?` bound on Sessions and Projects, opens the help modal (prior swallow replaced; bubbles/list never self-toggles its own help).
- Help modal GENERATED from the page keymap descriptor (not hand-authored), lists the COMPLETE keymap including footer own keys.
- Two-column renderer: key-hint glyph accent.blue, action label text.strong.
- Header `? Keybindings` (text.primary) left + right-aligned `esc close` (text.detail); NO contextual footer (§8.1 exception).
- Closes on `?` (toggle) or Esc; key-exclusive while open — Esc dismisses, no fall-through to clear-filter/quit.
- Renders on the cleared blank canvas (inherits 3-1).
- Preview `?` + Preview descriptor + help-from-Preview flagged deferred to Phase 4 (code comment), not wired here.
- No literal hex at renderer call sites — every colour a §2.9 token.
- VISUAL: vhs tape opens Sessions help (Sessions → ?) and writes a PNG matched against `Sessions — Help Modal (?)`.
- §15.6 added criterion: light-mode eyeball — help modal rendered in light against #e1e2e7 and visually confirmed.

STATUS: Complete

SPEC CONTEXT:
§8.5 defines the new per-page `?` help modal — a centred two-column panel (glyph accent.blue / label text.strong), header `? Keybindings` text.primary + right-aligned `esc close` text.detail (the documented §8.1 contextual-footer exception), listing the page COMPLETE keymap (incl. footer keys), generated from the per-page keymap descriptor (the single source of truth that also drives the footer + §12.1), key-exclusive close on `?`/Esc. §12.2: `?` is newly bound on every page (was actively swallowed so bubbles/list couldn't self-toggle help). §13.3: `?` is per-page contextual, one overlay pattern with page-specific content. §14.4: new modal type + generic renderer over the descriptor (~60-80 lines), extends the existing rounded-border modal primitive. §9.3: Preview `?` overlays without blanking — deferred.

IMPLEMENTATION:
- Status: Implemented (matches acceptance + spec on every point)
- Location:
  - internal/tui/modal.go:18 — `modalHelp` added to the modalState enum.
  - internal/tui/modal.go:44-47 — `renderHelpModalOnClearedCanvas` wrapper (places the hand-drawn panel on the cleared canvas via the shared `placeModalOnClearedCanvas`).
  - internal/tui/help_modal.go:76-203 — `renderHelpModalContent` + `helpModalHeader` + `helpModalBodyRows`/`helpModalRow` + `helpActionLabel`/`helpKeyGlyph`/`isDestructiveHelpKey`. Two-column renderer over the descriptor; header `? Keybindings` left + right-aligned `esc close`; complete keymap minus the `?` self-entry; single-tone border.separator joined panel via the shared `renderJoinedPanel` (panel.go — confirmed the shared primitives moved off the `help*` prefix per task 9-4, consistent with the note).
  - internal/tui/model.go:2266-2269 (Projects) and 2962-2968 (Sessions) — `?` un-swallowed; opens `modalHelp` (handler comment documents that opening our modal still consumes the key so the list help never fires). `?` is a literal filter char while `SettingFilter()` (model.go:2959-2961).
  - internal/tui/model.go:3099-3123 — `updateModal` routes `modalHelp` to `updateHelpModal`, which is key-exclusive: `?`/Esc → modalNone, all other keys consumed (no fall-through).
  - internal/tui/model.go:3841-3844 (Projects) / 4033-4036 (Sessions) — View arms render via `renderHelpModalOnClearedCanvas(projectsKeymap()/sessionsKeymap(), ...)`.
  - internal/tui/keymap.go:86-140 — `sessionsKeymap`/`projectsKeymap` descriptors (the single source); `?` entry is RightAligned + Core (footer hint), excluded from the help body.
  - internal/tui/keymap.go:99 / help_modal.go:10-36 — Phase 4 deferral for Preview `?` flagged in a code comment (now actually wired in Phase 4 — pagepreview.go:583/593 — consistent with the "verify against CURRENT code" note).
  - testdata/vhs/sessions-help-modal.tape + .png (dark) and sessions-help-modal-light.{tape,png} (light) — visual artifacts present.
- Notes: No literal hex at call sites — every colour is a theme.MV.* token (AccentBlue/TextStrong/TextPrimary/TextDetail/AccentViolet/StateRed/BorderSeparator). The descriptor↔dispatch link for the new `?` binding is held by keymap_dispatch_guard_test.go, which allow-lists the single RightAligned `?` entry and defers its dispatch verification to the dedicated help-modal suites — a clean, non-widening allow-list.

TESTS:
- Status: Adequate
- Coverage:
  - help_modal_test.go — open on `?` (Sessions + Projects); bubbles/list ShowHelp unchanged (swallow replaced, not self-toggle); close on `?` toggle and Esc (Sessions + Projects); Esc key-exclusivity with an applied filter (filter survives) AND with no filter (no quit); all-other-keys consumed (`q` swallowed); descriptor-generated complete keymap incl. footer-core keys (Sessions) and help-only keys d/n/nav/q (Projects); footer-core key present in help (Preview scrollback / ␣ glyph); `?` self-entry excluded from body; glyph set (^↑/↓, ␣, ⏎, ↑/↓, no ctrl+); colour roles accent.blue/text.strong/text.primary/text.detail in both modes; state.red destructive kill glyph in both modes; cleared blank canvas (rows behind gone, owned-canvas backdrop SGR present, both modes).
  - help_modal_frame_test.go — panel border = border.separator (not white, both modes); single-tone (no border.footer hue); divider joins side borders via ├/┤ flush junction-to-junction; flush vertical spacing (zero blank rows top/title/divider/first-body); contiguous body rows (1-row rhythm).
  - Every "Tests" bullet and every Edge Case in the task definition maps to a concrete test (Esc-with-filter, `?`-toggle-while-open, footer-core-in-help all present). The visual-match criterion is covered by the tape + PNG (verified by eyeball below).
- Notes: Tests assert behaviour/colour-role/structure via token SGR cores and rendered content, not brittle internal state — well-targeted. Not over-tested: each test isolates one property; the dark+light table parametrisation is justified (two distinct canvas/token resolutions). One mild redundancy: `TestHelpModalDividerJoined` and `TestHelpModalDividerConnectsToBorders` both locate the ├…┤ divider and assert it is all-rule-glyphs flush to junctions — the second is a strict superset (it adds the body-inset contrast). Non-blocking.

VISUAL VERIFICATION:
- Dark PNG (testdata/vhs/sessions-help-modal.png): two columns (blue glyphs / strong labels), header `? Keybindings` (violet `?`) left + `esc close` right, complete keymap (↑/↓, ^↑/↓, ⏎, /, ␣, s, n, r, k, q, x) on a cleared canvas, `k` kill glyph in red, no `?` self-row, no contextual footer. Matches the `Sessions — Help Modal (?)` structure/layout/colour-role.
- Light PNG (testdata/vhs/sessions-help-modal-light.png): renders against #e1e2e7; glyph/label two-column wiring + header read correctly, `k` in red. Satisfies the §15.6 light-mode eyeball criterion.

CODE QUALITY:
- Project conventions: Followed. Tokens-only at call sites (§2.9), shared joined-panel primitive reused, descriptor as single display source, guard-test discipline for the descriptor↔dispatch split, leaf rendering helpers with focused responsibilities. Go conventions clean (no t.Parallel, small functions, clear naming).
- SOLID principles: Good. The renderer is single-responsibility and reads purely from the descriptor; the help modal extends the existing modal primitive (open/closed) rather than forking it; close logic is isolated in `updateHelpModal`.
- Complexity: Low. `renderHelpModalContent` is a straight compose; `helpModalHeader` natural-width handling is the only non-trivial branch and is correct + commented.
- Modern idioms: Yes.
- Readability: Good. Comments are unusually thorough and explain the §8.1 exception, the flush convention, the descriptor-vs-dispatch boundary, and the Phase 4 deferral.
- Issues: One brittle coupling — `isDestructiveHelpKey` (help_modal.go:201-203) keys destructive-red on the literal glyphs `e.Key == "k" || e.Key == "d"` rather than a structural descriptor flag. Correct for the only two pages today, but a future page binding `d`/`k` to a non-destructive action would render it red. Non-blocking (see notes).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/help_modal.go:201-203 — replace the `e.Key == "k" || e.Key == "d"` glyph-literal destructive check with a structural `Destructive bool` flag on keymapEntry, set on the kill/delete entries in keymap.go; removes the brittle glyph coupling so a future non-destructive `d`/`k` binding does not render red.
- [quickfix] internal/tui/help_modal_frame_test.go:60-125 — `TestHelpModalDividerJoined` is fully subsumed by `TestHelpModalDividerConnectsToBorders` (both locate the ├…┤ divider and assert all-rule-glyphs flush to junctions; the latter adds the body-inset contrast). Fold the former into the latter to drop the duplicate divider-location boilerplate.

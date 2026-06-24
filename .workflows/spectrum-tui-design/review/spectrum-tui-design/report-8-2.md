TASK: spectrum-tui-design-8-2 — Switch Sessions/Projects footer to §3.4 ⏎/␣ glyphs (spec-owner ratified PATH (b))

ACCEPTANCE CRITERIA (from tick-06a9a6):
- Spec owner chose (a) or (b); chosen form reflected consistently in spec §3.4, the keymap descriptor / footer.go render, and the byte-exact footer_test.go assertion. [Path (b) — glyphs — chosen.]
- After the decision, no residual disagreement between §3.4's verbatim footer copy and the live Sessions/Projects footer.
- The Preview footer convention (⏎/␣ glyphs) left intact regardless of path.
- footer_test.go's byte-exact footer assertion matches the ratified form and passes.
- Path (b): a render-level test confirms the Sessions/Projects footer shows ⏎/␣ glyphs (and ↑↓ nav) matching §3.4.

STATUS: Complete

SPEC CONTEXT:
§3.4 (specification.md:188) specifies the Sessions footer "exactly" as
`↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` + right-aligned `? help`.
§6.3 (line 273) specifies the Projects footer as `⏎ new session · x sessions · e edit · / filter · ? help`.
The pre-task divergence: the keymap descriptor stored footer Key forms as the literal words "enter"/"space" and nav as the slashed "↑/↓", so the live footer read `enter attach` / `space preview` / `↑/↓ navigate` — a literal mismatch against §3.4's glyph copy and against the Preview footer convention (§9.1 / previewKeymap, already glyphs). Spec owner ratified PATH (b): switch the footer to glyphs. The spec was already in glyph form, so no §3.4 amendment was needed.

IMPLEMENTATION:
- Status: Implemented (commit d357202a).
- Location:
  - internal/tui/keymap.go:88 — sessionsKeymap nav `{Key:"↑↓", HelpKey:"↑/↓"}` (was Key "↑/↓", no HelpKey)
  - internal/tui/keymap.go:90 — attach `{Key:"⏎", HelpKey:"⏎"}` (was Key "enter")
  - internal/tui/keymap.go:92 — preview `{Key:"␣", HelpKey:"␣"}` (was Key "space")
  - internal/tui/keymap.go:128 — projectsKeymap nav `{Key:"↑↓", HelpKey:"↑/↓"}` (was Key "↑/↓")
  - internal/tui/footer.go:11,37 — special-case doc comments updated to glyph form (the footer render itself reads Key directly via renderFooterEntry→renderKeyHint, so no logic change was needed — the descriptor Key swap is sufficient)
  - internal/tui/help_modal.go:255-256 — helpKeyGlyph doc updated
- Notes:
  - The footer reads Key directly; the descriptor swap drives the new glyph render with no render-logic change. Correct and minimal.
  - Help-body invariance is preserved by design: helpKeyGlyph (help_modal.go:163 via helpModalRow) returns HelpKey-first. Nav's new HelpKey "↑/↓" resolves identically to the old Key fall-through "↑/↓"; attach/preview HelpKey "⏎"/"␣" were already overrides. So the ? help body output is byte-unchanged (still slashed ↑/↓ + ^↑/↓), exactly as the commit claims.
  - Dispatch is display-only-unaffected: Key strings are display tokens, not tea key codes; the keymap_dispatch_guard_test.go probes were re-keyed to the glyph map keys (no dispatch behaviour change).
  - Preview footer untouched: previewKeymap (keymap.go:183-193) is absent from the diff; its ⏎/␣/←→/⇥ glyphs are preserved verbatim.
  - Projects footer fixtures (projects.png / projects-command-pending.png) correctly NOT re-captured: the only Projects descriptor change was nav, which is help-only on Projects (no Core flag), so it never renders in the Projects footer — the Projects footer Core entries (⏎/x/e//) were already glyphs.

VISUAL CONFORMANCE (re-captured fixtures):
- 8 Sessions-family fixtures re-captured in the same commit: sessions-flat, -flat-light, -flat-nocolor, -by-project, -by-tag, -no-tags-signpost, -inline-flash, -paged. These are exactly the fixtures rendering the Sessions footer.
- Verified testdata/vhs/sessions-flat.png by eye: footer now reads `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects   ? help` — matches §3.4 verbatim (glyph forms, ↑↓ no slash).
- Verified testdata/vhs/reference/projects-mv.png: already shows `⏎ new session · x sessions · e edit · / filter   ? help` (glyphs) — confirms Projects needed no change.
- filtering-* / preview / help fixtures correctly untouched (filtering footers and preview/help bodies are unaffected).

TESTS:
- Status: Adequate
- Coverage:
  - footer_test.go:43 — byte-exact render-level assertion updated to glyph form `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects · ? help`. This is the render-level test the acceptance criterion requires (renders renderSessionsFooter and strips ANSI). PASS by construction.
  - footer_test.go:196, :264 — colourless + narrow-truncation assertions updated to `↑↓ navigate`.
  - keymap_test.go — descriptor enumeration / Core-order / HelpKey-override sub-tests updated to the glyph map and the new wantHelpKey {↑↓:↑/↓, ⏎:⏎, ␣:␣}.
  - projects_keymap_test.go — Projects descriptor + help-only key set updated to glyph nav.
  - help_modal_test.go — TestHelpModalGlyphs reaffirms the help body keeps ^↑/↓, ␣, ⏎, ↑/↓ (slashed) and the footer Key set is now ␣/⏎/↑↓ — directly pins the help-vs-footer split invariant.
  - keymap_dispatch_guard_test.go — probe map keys re-keyed; dispatch parity guard still exercises Down/Enter/Space/Filter, confirming display-only change.
  - pagepreview_chrome_enterattach_test.go — correctly removed `⏎ attach` from the preview-exclusive forbidden-token set (it is now a legitimate Sessions footer token) while keeping `◉ preview`/`←→ window`/`⇥ pane`/`␣ back` guarded. Good catch — prevents a false-positive leak guard.
- Notes: No render-level assertion exists that the *Projects* footer renders the glyphs, but this is acceptable — the Projects footer Core entries were already glyphs and unchanged by this task; footer_test covers the Sessions render and keymap/projects_keymap tests pin the descriptor. Not under-tested for the scope of this change. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (standard Go, no t.Parallel, descriptor-as-single-source-of-truth pattern honoured; the change lives in the descriptor data, not duplicated across render sites).
- SOLID principles: Good — single-source-of-truth descriptor preserved; footer/help DISPLAY split kept on the one descriptor (Key vs HelpKey) rather than a second hand-authored list.
- Complexity: Low — pure data edits + doc-comment updates.
- Modern idioms: Yes.
- Readability: Good — the doc comments on keymapEntry.HelpKey, sessionsKeymap, projectsKeymap, and helpKeyGlyph were all updated to explain the post-switch contract (nav HelpKey override, enter/space HelpKey now coinciding with glyph Key). The rationale is well documented in-source.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. The change is complete, consistent across spec / descriptor / render / byte-exact test / re-captured fixtures, the help body is provably byte-unchanged, the Preview footer is untouched, and the Projects-fixture non-recapture is correctly justified.

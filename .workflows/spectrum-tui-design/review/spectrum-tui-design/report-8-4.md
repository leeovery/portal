TASK: spectrum-tui-design-8-4 — Remove dead SessionItem/ProjectItem Title()/Description() (or derive the attached marker from the const); kill the hard-coded "● attached" re-spelling outside attachedMarker.

ACCEPTANCE CRITERIA (from tick-741e43):
- The literal "● attached" no longer appears in SessionItem.Description() (method removed or it derives from attachedMarker).
- No production-dead Title()/Description() remains carrying a hand-spelled copy of row vocabulary; if methods are kept they derive from the centralised const.
- The live attached-marker render path (renderSessionRow) is unchanged.
- Tests that previously exercised Title()/Description() are updated to assert the live render path (deletion) or confirm the marker is sourced from attachedMarker (derivation), and pass.
- The session/project row render tests continue to pass with unchanged output.

STATUS: Complete

SPEC CONTEXT:
§4.1 (Row anatomy): the attached marker is a fixed-width trailing slot — "● attached" in state.green when attached, an empty slot of the same width when not, so bullets/counts stay column-aligned. §2.2: state is never carried by hue alone (the ● glyph + "attached" label carry it glyph-distinctly). The whole feature is a "reskin, not rebuild" with a behaviour-parity bar; dead-code removal here is the analysis-cycle chore tightening §2.1/§2.8 "design to roles, no literal at call sites" by ensuring the marker text lives in exactly one place (attachedMarker const, session_item.go:57). The colour-literal guard (TestNoRawColourLiteralAtCentralisedSites) only inspects lipgloss.Color(...) literals and is structurally blind to a plain string — so the dead Description()'s hand-spelled "● attached" was a genuine latent stale seam no guard would catch. Deletion is the right resolution.

IMPLEMENTATION:
- Status: Implemented (deletion path chosen over derivation — the cleaner of the two sanctioned options since the methods had zero production callers).
- Location:
  - internal/tui/session_item.go:118-125 — only FilterValue() remains on SessionItem; the former Title()/Description() (which hand-spelled `label + "  ● attached"`) are gone. The doc comment now states the marker text flows from the single attachedMarker const.
  - internal/tui/project_item.go:28-34 — only FilterValue() remains on ProjectItem; former Title()/Description() gone.
  - internal/tui/session_item.go:57 — attachedMarker = "● attached" is now the single source of the marker text.
  - internal/tui/session_item.go:439-448 — live renderSessionRow attached-marker path UNCHANGED (verified against the 8-4 commit diff: no +/- on these lines; the only diff touching renderSessionRow/attachedMarker is the FilterValue doc-comment).
- Notes:
  - Commit 97b60909 is byte-identical for the live render: it touches only the two item files and their two test files (plus tick/manifest bookkeeping). No render-golden file (session_style_consolidation_test.go, session_row_anatomy_test.go, row_style_helpers_test.go, project_row_anatomy_test.go) was modified — confirming the rendered output did not change.
  - Repo-wide grep confirms zero production callers of .Title()/.Description() on these item types; bubbles/list consumes only FilterValue() off list.Item. The deleted Description() had already diverged in FORM from the live render (bare two-space concat vs. the fixed-width column-aligned slot), so removal also eliminated a real pre-existing inconsistency, not just a theoretical one.
  - "● attached" remaining occurrences are all legitimate: the attachedMarker const, the renderSessionRow comment, theme.go token docs, internal/capture/swatch.go + its test (a separate non-TUI palette-capture surface, out of scope), and test assertions that pin the live render output.

TESTS:
- Status: Adequate.
- Coverage:
  - session_item_test.go:29-35 + project_item_test.go:28-34 — the dead Title()/Description() unit tests were removed and replaced with explanatory comments pointing to where the vocabulary is now asserted (the live render in TestSessionDelegate / TestProjectDelegate). This is correct: a test for a deleted method should be deleted, not retained as a no-op.
  - session_item_test.go:65-84 — the former "returns the session name from Title regardless of group fields" test was retargeted to drive SessionDelegate.Render and assert the name appears in the (ansi-stripped) live output — the right replacement, exercising the real path.
  - The attached-marker behaviour is covered by the live render: TestSessionDelegate "renders attached badge for attached session" (asserts "● attached" present, :199) and "does not render attached badge for detached session" (asserts "attached" absent, :215). Window-count pluralisation likewise asserted against live render (:153-186). So the vocabulary the dead Description() used to project is fully covered by the live path.
- Notes:
  - Not under-tested: name, window count (singular/plural), attached/detached marker, and selection bar are all asserted against the live render.
  - Not over-tested: the four deleted sub-tests were pure projection-method assertions with no production consumer — removing them is a net reduction in redundant surface, not a coverage loss, since the same vocabulary is asserted once against the live render.

CODE QUALITY:
- Project conventions: Followed. Tests use no t.Parallel() (per CLAUDE.md). The deletion advances the §2.8 "marker text in one place" / closed-vocabulary discipline. golangci-lint reported 0 issues per the commit message.
- SOLID principles: Good — removing the dead projection methods narrows SessionItem/ProjectItem to exactly the list.Item contract bubbles/list needs (FilterValue), a tighter interface surface (ISP).
- Complexity: Low — pure deletion; no new branches.
- Modern idioms: Yes — the FilterValue doc comments are now accurate and explain why it is the only consumed method.
- Readability: Good — the replacement test comments are precise about what was removed, why it was dead, and where the vocabulary is now asserted, so a future reader is not left wondering where the projection went.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

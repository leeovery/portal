TASK: spectrum-tui-design-9-4 — Move and rename the shared joined-panel frame primitives off the use-site help* prefix (mechanical rename + relocation chore)

ACCEPTANCE CRITERIA:
- The shared joined-panel frame primitives (helpFrame*/helpInset*/helpRowInset/helpRuleGlyph family backing renderJoinedPanel) reside in panel.go beside renderJoinedPanel and carry a frame-neutral (panel*) prefix.
- No help*-prefixed name remains for a primitive that is part of the shared frame; genuinely help-specific symbols (helpModalHeader, helpModalRow, helpTitle) remain in help_modal.go unchanged.
- Every call site (modals, preview, tests) references the renamed symbols; a repo-wide grep for the old shared-primitive names returns no matches.
- Rendered output for all five modals and the preview overlay is byte-identical to before; go build and go test ./... pass.

STATUS: Complete

SPEC CONTEXT:
The spec corrigendum (§8.1 / §9 preview restructure, task 4-6) confirms renderJoinedPanel is the single shared chrome: the help/kill/rename/delete/edit modals AND the §9.1 full-screen preview overlay all compose through it (single-tone, modals pass border.separator, preview passes accent.cyan). The frame primitives were therefore the universal joined-panel framework wearing a use-site help* prefix — a naming/locality smell with no behaviour implications. This is a pure-cosmetic chore under the "Reskin, not rebuild" parity mandate; output must be provably unchanged.

IMPLEMENTATION:
- Status: Implemented (clean mechanical rename + relocation)
- Location:
  - internal/tui/panel.go:10-31 — the moved consts (panelRuleGlyph, panelRowInset, panelFrameTopLeft/TopRight/BottomLeft/BottomRight/Side/TeeLeft/TeeRight).
  - internal/tui/panel.go:88-135 — the moved functions (panelFrameStyle, panelFrameTop, panelFrameBottom, panelFrameDivider, panelFrameContentLine, panelInsetRow), all beside renderJoinedPanel (panel.go:59-86).
  - internal/tui/help_modal.go — retains only the genuinely help-specific symbols (helpModalHeader:106, helpModalRow:158, helpTitle:44, plus helpTitleGlyph/helpDismissHint/helpColumnGap/helpKeyColumnWidth and the helpModal* body helpers). No shared frame primitive remains here.
- Notes:
  - git show 4be1ff42 confirms the function/const BODIES are verbatim — every moved line differs only by the help->panel prefix substitution (e.g. helpFrameTopLeft + Repeat(helpRuleGlyph...) -> panelFrameTopLeft + Repeat(panelRuleGlyph...)). No logic, ordering, token, or signature change. This is the strongest possible evidence of byte-identical render output.
  - All six renderJoinedPanel call sites verified to route through the high-level entry point, never the moved primitives directly: rename_modal.go:72, edit_modal.go:205, destructive_confirm.go:88 (kill+delete share this), help_modal.go:96, pagepreview.go:705. So the rename of the lower-level primitives cannot perturb any caller.
  - go vet ./internal/tui/ exits 0 — the package compiles with every call site resolved (no orphaned references, no unused-import fallout in help_modal.go after the move).

TESTS:
- Status: Adequate (no new tests needed; existing joined-panel/modal/preview render tests updated in-place)
- Coverage: panel_test.go exercises renderJoinedPanel directly (single-tone frame, border-token parameterisation cyan vs separator, divider join/count, uniform width, row-inset-vs-flush-divider, colourless carve-out). The modal frame tests (help_modal_frame_test, delete_modal_test, kill_modal_test, rename_modal_test) assert the rendered frame structure. These are the golden/structural guards that would catch any non-mechanical drift.
- Notes:
  - Test files were updated in the same pass to reference the renamed symbols (panelFrameTeeLeft/Right, panelFrameTopLeft, panelRuleGlyph, panelRowInset) — confirmed across panel_test.go, help_modal_frame_test.go, delete_modal_test.go, kill_modal_test.go, rename_modal_test.go. The diff shows only prefix substitution in these assertions, not logic change.
  - help_modal_test.go:358 still calls helpModalHeader — correct, that is a retained help-specific symbol, not a shared primitive.
  - For a behaviour-preserving rename the right test posture is exactly this: keep the existing render assertions, update symbol references, add nothing. Not under- or over-tested.

CODE QUALITY:
- Project conventions: Followed. The rename directly applies golang-naming's role-over-use-site guidance — panelFrameStyle/panelInsetRow/panelRowInset name the joined-panel role a maintainer reading the preview or any modal now reads directly, instead of mentally overriding a help* prefix. Locality also improves: the primitives now sit beside their sole entry point renderJoinedPanel.
- SOLID principles: Good. Single shared abstraction with one entry point; no duplication introduced or removed.
- Complexity: Low (unchanged — verbatim bodies).
- Modern idioms: Yes (unchanged).
- Readability: Improved — the role-neutral names and co-location remove the cognitive override the chore set out to fix.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

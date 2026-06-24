TASK: spectrum-tui-design-3-7 — Delete-project confirm modal reskin (MV): mirror kill + record-only consequence line + drop `n` + blank-screen (tick-9524d2)

ACCEPTANCE CRITERIA:
1. state.red header `▲ Delete project?` (glyph + title both red).
2. Project name in state.red; path in text.detail.
3. Record-only consequence line, DISTINCT from kill (no "Ends the tmux session…"; affirms sessions/files untouched).
4. Footer `y delete · esc cancel` (glyphs accent.blue, labels text.detail); dismiss in footer.
5. `y` deletes exactly as before (deleteAndRefreshProjects unchanged — parity); Esc cancels; `n` no longer cancels.
6. Renders on cleared blank screen (inherits 3-1), border-defined, no fill; state.red destructive-only.
7. NO_COLOR carve-out: destructive state via ▲ glyph + text + bold.
8. No literal hex at call sites — every colour a §2.9 token.
9. Visual verification mirroring Kill Confirm Modal (MV) with the record-only consequence substituted.

STATUS: Complete

SPEC CONTEXT:
§8.6 retargets the old `fmt.Sprintf("Delete %s? (y/n)", …)` overlay to mirror the kill modal's destructive treatment with a DISAMBIGUATING consequence line (record delete vs session kill). §8.1 drops `n` (cancel = Esc only), mandates the blank-screen modal layer, footer-owned dismiss hint, and destructive title + `▲` in state.red. Confirm logic must be preserved (parity). Mocked at implementation mirroring Kill Confirm Modal (MV) — no dedicated Paper frame.

IMPLEMENTATION:
- Status: Implemented (clean, matches spec and AC).
- Location:
  - internal/tui/delete_modal.go:19-63 — delete DATA only (title/consequence/footer verb + project-path extra body row); composes via the shared renderer.
  - internal/tui/destructive_confirm.go:84-151 — shared destructive-confirm grammar (6-2), parameterised by destructiveConfirmSpec; delete passes the path via extraBodyRows (data, not a forked render path), as the task requires.
  - internal/tui/modal.go:59-68 — renderDeleteModalOnClearedCanvas wraps content in placeModalOnClearedCanvas (inherits 3-1 blank-screen).
  - internal/tui/model.go:2337-2360 — updateDeleteProjectModal: `y` confirm arm + `deleteAndRefreshProjects` untouched; cancel arm changed to Esc-only; clears BOTH pendingDeletePath + pendingDeleteName on Esc; `n` falls through to the ignore-all-other-keys default.
  - internal/tui/model.go:3833 — viewProjectList delete branch routes to the reskinned renderer.
- Notes:
  - Parity verified against the implementing diff (git ba8d6231): the ONLY logic delta is removing `isRuneKey(keyMsg, "n")` from the cancel case; the `y` arm and deleteAndRefreshProjects (model.go:1841) are byte-for-byte unchanged. AC #5 satisfied.
  - Record-only consequence line (delete_modal.go:25) is distinct from killConsequence (kill_modal.go:22) — "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." AC #3 satisfied.
  - Path row truncates (ansi.Truncate to destructiveBodyWidth=52, ellipsis) rather than wrapping — correct: path truncates, consequence wraps. Edge case (long path no overflow) handled.
  - No literal hex at call sites (grep clean); all colours via theme.MV.* tokens through headerStyle. AC #8 satisfied.
  - Esc clears both pending fields (no stale-state leak) — the spec edge case is met.
  - Visual: testdata/vhs/delete-project-modal.png (dark) + -light.png both render the exact mirror — red ▲ header, red name, detail path, distinct record-only consequence, `y delete   esc cancel` blue glyphs, single-tone ├──┤ joined panel, no fill, centred on cleared canvas. Tape drives the real user path (Projects via `x`, then `d`) against the in-memory `projects` fixture (no tmux, no ~/.config touch). AC #9 satisfied.

TESTS:
- Status: Adequate.
- Coverage:
  - delete_modal_test.go — Header (▲ + title red, both modes), BodyNameAndPath (name above path, separate lines), BodyColourRoles (name state.red / path+consequence text.detail), ConsequenceLine (record-only fragments present, NOT "Ends the tmux session", affirms sessions/files untouched), Footer (`y delete`/`esc cancel`, accent.blue glyphs), SingleToneJoinedPanel (exactly 2 dividers, border.separator present / border.footer absent), Colourless (▲ + bold survive, no state.red hue, frame glyphs survive), LongPathTruncates (ellipsis present, full path absent, no row exceeds frame width).
  - destructive_confirm_test.go — ByteIdenticalGolden (kill + delete, dark/light/NoCol) pins both modals against PRE-refactor goldens, genuinely proving the 6-2 consolidation produced zero drift; DeleteSpec round-trips the shared renderer with the path extra-body row; WordWrapAt52 pins the §8.6 break points.
  - Confirm/cancel/ignore-n behaviour: the keymap logic in updateDeleteProjectModal is exercised through the model-level project-page tests (the `y`/Esc/ignore arms are thin and the parity is golden-pinned at the diff level). The render goldens + the diff-level parity cover AC #5's render/copy; behavioural key-dispatch coverage for the delete modal is lighter than render coverage (see note).
- Notes:
  - Strong, well-targeted suite: byte-identical goldens for the refactor + per-role SGR-core assertions via tokenFgSeq (real token sequences, not hard-coded hex). Edge cases (truncation, NO_COLOR, distinctness-from-kill) all explicitly tested.
  - Not over-tested: each test asserts a distinct facet; the dark/light/colourless matrix is justified by mode-dependent token output.

CODE QUALITY:
- Project conventions: Followed. Small data-only file delegating to a shared renderer; consistent with kill_modal.go's sibling shape. DI/seam patterns N/A (pure render). Comments are precise and cite spec sections.
- SOLID principles: Good. SRP honoured — delete_modal.go owns delete DATA, destructive_confirm.go owns the grammar; the spec struct keeps the two modals from diverging (the whole point of 6-2).
- Complexity: Low. No branching beyond the trailer/extra-row presence checks.
- Modern idioms: Yes. Idiomatic Go, ansi.Truncate/Wordwrap from the x/ansi lib, lipgloss styling.
- Readability: Good. Self-documenting; the ASCII layout comment above renderDeleteModalContent is a helpful map.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/model.go:2337-2360 — updateDeleteProjectModal's `y`/Esc/ignore-`n` key dispatch has no direct unit test (parity is golden-pinned at the render layer and at the implementing diff, but no test drives a `tea.KeyPressMsg` "n" through the function to assert modal stays open + no delete cmd fires). A small table test (`y`→delete cmd, Esc→modalNone+cleared fields, `n`→unchanged) would lock the §8.1 drop-`n` behaviour against future regression. Decide whether the diff-level parity is sufficient or worth a behavioural guard.
- [do-now] internal/tui/destructive_confirm.go:57 — the nameTrailer doc comment says the gap is "two-cell" but destructiveNameRow:125 renders a 2-space gap while the comment two lines down (line 119) describes it as `  <trailer>`; consistent, but the "after a two-cell canvas gap" phrasing in the struct field doc could note it is rendered by destructiveNameRow for traceability. Minor wording only.

TASK: spectrum-tui-design-3-5 — Kill confirm modal reskin (MV): destructive treatment + drop n + blank-screen (tick-e9a48a)

ACCEPTANCE CRITERIA:
- state.red header `▲ Kill session?` (glyph + title both red, bold).
- Session name in state.red; `· N window(s)` in text.detail (correct singular/plural) on the same line.
- Consequence line "Ends the tmux session and all its panes. Can't be undone." in text.detail.
- Footer `y kill   esc cancel` (key glyphs accent.blue, labels text.detail); dismiss key in footer, never header.
- `y` confirms exactly as before (killAndRefresh parity); `Esc` cancels; `n` no longer cancels (ignored).
- Window count captured at modal-open from si.Session.Windows.
- Renders on cleared blank screen (inherits 3-1), border-defined no fill; state.red destructive-only.
- NO_COLOR: native bg, destructive carried by ▲ glyph + text + bold (not colour-only).
- No literal hex at call sites — every colour a §2.9 token.
- Visual: both Kill Confirm Modal (MV) + (Light) frames.

STATUS: Complete

SPEC CONTEXT:
- §8.3: centred panel, state.red header `▲ Kill session?`, name in state.red, `· N window(s)` (text.detail), consequence line (text.detail), footer `y kill · esc cancel`. Confirm logic preserved; keymap drops `n` (Esc-only); inherits blank-screen + MV restyle. Keys: y (confirm) / Esc (cancel).
- §8.1: shared modal anatomy — destructive modals render title + ▲ in state.red; dismiss key always in the FOOTER as `esc cancel`, never header.
- §2.9: state.red (#F7768E dark / #C32647 light) is destructive-only (kill/delete, ▲); accent.blue for footer key-hint glyphs; text.detail for counts/secondary text.
- §2.5: NO_COLOR carve-out — modal clears to native bg; state never colour-only (glyph + bold carry it).
- Golden consequence copy is "Can't be undone." (spec §8.3 line 344 + both reference frames); the tick description's "Can not be undone." is paraphrase — implementation correctly follows the golden spec/reference.

IMPLEMENTATION:
- Status: Implemented (via the 6-2 DRY consolidation behind renderDestructiveConfirm — verified against the current consolidated code).
- Location:
  - internal/tui/kill_modal.go:40 renderKillModalContent — supplies kill DATA (title/consequence/window-count/confirm verb) to the shared renderer; killWindowCount:54 does the pluralisation.
  - internal/tui/destructive_confirm.go:84 renderDestructiveConfirm — shared panel grammar: ▲ glyph + state.red+bold header (destructiveHeaderRow:94), state.red+bold name row with text.detail trailer (destructiveNameRow:120), one blank separator + word-wrapped text.detail consequence (destructiveBodyRows:105 / destructiveConsequenceRows:134), `<key> <verb>   esc cancel` footer (destructiveFooterRow:149).
  - internal/tui/model.go:3070 handleKillKey stores pendingKillName + pendingKillWindows from si.Session.Windows; :3125 updateKillConfirmModal: `y` → killAndRefresh (clears both fields), `tea.KeyEscape` → cancel (clears both), all other keys (incl. `n`) fall through to the ignore tail; :3150 killAndRefresh unchanged (byte-for-byte parity — kill then re-list).
  - internal/tui/model.go:4025 View() routes modalKillConfirm through renderKillModalOnClearedCanvas; modal.go:54 wraps placeModalOnClearedCanvas; the outer View()→fillCanvas paints the owned/NO_COLOR-suppressed backdrop (3-1 inheritance, single path, no double-branch).
- Notes: Matches both reference frames (testdata/vhs/kill-confirm-modal{,-light}.png vs reference/kill-confirm-modal-{mv,light}.png) — header, name+count, consequence wrap, footer, colour roles all align. The frame renders in border.separator (single-tone) rather than the reference's faint red border — this is the documented 3-4 single-tone decision recorded in the tick note (supersedes §8.1 2-tone), intentional and consistent across all modals.

TESTS:
- Status: Adequate.
- Coverage:
  - Render (kill_modal_test.go): header glyph+title+state.red (dark+light); name state.red + `· N window(s)` text.detail same-line with 1/4/0 pluralisation; body colour roles; consequence start+end fragments (wraps); footer `y kill`/`esc cancel` + accent.blue glyphs; single-tone joined panel (2 dividers, border.separator present / border.footer absent); one-blank-row body layout; NO_COLOR (▲ survives, no state.red hue, bold present, frame glyphs survive).
  - Dispatch (kill_modal_dispatch_test.go): k stores name+windows; y parity (closes modal, clears both fields, drains cmd → KillSession(stored name)); Esc cancels (no cmd, clears both, no kill); n ignored (modal stays open, no cmd, fields intact, no kill).
- Notes: Every acceptance criterion and every listed edge case (1/0/N windows, n-ignored, Esc clears both, NO_COLOR) has a matching assertion. Render vs dispatch split is clean — no over-test. Tests assert behaviour (presence, colour SGR cores via tokenFgSeq, model state) not brittle internals. Would fail if the feature broke (e.g. n re-added as cancel, count miswired, state.red dropped).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); colours via §2.9 theme tokens (zero literal hex at call sites); small focused helpers; thorough intent-documenting comments.
- SOLID principles: Good. kill_modal.go owns only kill DATA; destructive_confirm.go owns the shared grammar; modal_footer.go owns the footer shape; panel.go owns the frame — clean single responsibilities, no duplication with the delete modal (shared spec struct).
- Complexity: Low. killWindowCount is a trivial branch; updateKillConfirmModal is a flat 2-case switch + ignore tail.
- Modern idioms: Yes. Idiomatic Go, value-receiver Model update pattern, struct-spec parameterisation.
- Readability: Good. Self-documenting names, accurate ASCII layout sketches in doc comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

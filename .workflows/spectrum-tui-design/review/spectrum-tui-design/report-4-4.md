TASK: spectrum-tui-design-4-4 — Command-pending banner reskin (accent.violet left-bar + orange command chip + swapped footer, over full Projects chrome)

ACCEPTANCE CRITERIA (from plan/tick-562e68):
1. Banner = accent.violet ▌ left-bar + "Pick a project to run" + command in accent.orange chip; no literal hex.
2. Full Projects chrome preserved (green Projects header + count + / to filter); banner sits on top, not a stripped page.
3. Footer reads ⏎ run here · n run in cwd · esc cancel, drawn from commandPendingHelpKeys via the binding-source mechanism, restyled to MV.
4. Dispatched actions parity-preserved: Enter run here, n run in cwd, Esc cancel.
5. Banner routed through the task-4-1 single-slot arbiter as a persistent violet band; list height recomputes on appear/clear.
6. NO_COLOR keeps ▌ bar + position + chip glyph/position while dropping tint + bar colour.
7. vhs capture projects-command-pending.png compared against Paper frame "Projects — command pending (MV)".

STATUS: Complete

SPEC CONTEXT: §11.4 requires, when Projects is invoked to run a command, an accent.violet left-bar banner reading "Pick a project to run" with the command in an accent.orange chip, footer ⏎ run here · n run in cwd · esc cancel, over the FULL Projects chrome (green Projects header + / to filter) — not a stripped page. §11 intro pins the single-slot rule + the ▌ left-bar placement directly under the title separator. §2.5 NO_COLOR carve-out, §2.9 accent.violet/accent.orange/bg.selection/bg.warning tokens (closed vocabulary, no literal hex), §6.1 Projects section header.

IMPLEMENTATION:
- Status: Implemented (all 7 acceptance criteria met)
- Location:
  - internal/tui/notice_band.go:56-114 — bandCommand role + bar/tint token mapping (accent.violet bar, bg.selection tint).
  - internal/tui/notice_band.go:294-322 — renderCommandBand (bar + ▸ caret + "Pick a project to run" + orange chip) and renderCommandChip (accent.orange fg on bg.warning fill, NO_COLOR padded box).
  - internal/tui/notice_band.go:76-81 — commandBandCaret "▸" + commandBandText "Pick a project to run" constants.
  - internal/tui/model.go:3884-3887 — viewProjectList composes header → band slot → listView → footer (full chrome retained); :3895-3920 renderProjectCommandBand / renderProjectBandSlot (band + canvas blank breathing row).
  - internal/tui/model.go:3853 applyProjectsSectionHeader (green Projects header + / to filter retained); :3982-3988 renderProjectsFooterForFilterState swaps in renderCommandPendingFooter when commandPending.
  - internal/tui/keymap.go:153-159 commandPendingKeymap (enter run here / n run in cwd / esc cancel); footer.go:88-109 renders it via shared filter-footer machinery (accent.blue glyphs / text.detail labels / ? help right anchor).
  - internal/tui/model.go:637-644 WithCommand sets commandPending + PageProjects.
  - internal/tui/model.go:1361,1384-1390 applyProjectListSize reserves projectBandHeight (measured off renderProjectBandSlot — F10 recompute, budget = rendered height, no drift).
  - Dispatch parity: model.go:2302-2303 Enter → handleProjectEnter (run here), :2290-2291 n → handleNewInCWD (run in cwd, NOT gated), :2271-2275 Esc → tea.Quit (cancel). x/e/d gated by `if m.commandPending { return m, nil }` (:2279,:2293,:2298). s/r/k are Sessions-only handlers, unreachable on PageProjects — parity preserved.
- Notes:
  - The §11.4 banner correctly uses a SEPARATE render path (renderCommandBand) from the wrapping renderNoticeBand used by the signpost/flash — they share newBandBase so bar+tint can't drift, but renderCommandBand joins-and-pads rather than wrapping. See NON-BLOCKING [bug] re: width overflow.
  - Chip is accent.orange fg on bg.warning fill (both closed-vocabulary tokens). §2.9 notes chips are normally text.primary-on-tint "but §11.4 explicitly specifies the command in an accent.orange chip" — implementation matches the §11.4 carve-out.
  - The banner text uses TextOnSelection (bright white) where the Paper reference renders it slightly dimmer; colour-role-acceptable (Paper is layout/structure/role-authoritative, not pixel-exact).

TESTS:
- Status: Adequate
- Coverage: internal/tui/command_pending_band_test.go covers all 7 plan-mandated cases plus byte-exact footer pins:
  - VioletBarCaretTextAndOrangeChip (bar/caret/text/chip + violet+orange SGR sequences).
  - JoinsCommandSlice (strings.Join on spaces).
  - FixedTextConstant (spec-exact wording pinned to constant).
  - Tinted (bg.selection tint present; single full-width line == w for short command).
  - NoColorKeepsBarCaretAndChip (NO_COLOR keeps ▌/▸/text/command, zero SGR).
  - CommandBandRole_BarAndTintTokens (closed-vocabulary token names).
  - ViewProjectList_CommandPendingBandOverFullChrome (full chrome: Projects header + state.green role seq + / to filter + PORTAL wordmark; legacy "Select project to run" gone).
  - CommandBandUnderSeparatorAboveSectionHeader (band → blank → section header ordering).
  - ProjectBandHeight_TracksRenderedSlot (F10: height measured off rendered slot; 0 when not pending).
  - CommandPendingFooter_SwappedCopy (run here / run in cwd / cancel / help present; quit + non-§11.4 copy absent).
  - CommandPending_DispatchParity (Enter→/code/myapp run here, n→/code/cwd run in cwd with command forwarded, Esc→quit) — exercises real updateProjectsPage handlers.
  - CommandPendingKeymap_Copy + CommandPendingFooter_ByteExact (Dark/Light/NO_COLOR byte-pinned).
- Notes:
  - No test exercises a LONG command at narrow width (the over-width overflow path) — see NON-BLOCKING [bug]. The width test only asserts == w for a short command.
  - Byte-exact footer pins are appropriate here (footer copy is a hard spec requirement) and not over-testing.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (package mutable-mock rule). Tokens from the closed §2.9 vocabulary; grep confirms no literal hex in notice_band.go. Single-source-of-truth pattern honoured: renderProjectBandSlot is the sole slot composer consumed by both view and height reserve; commandPendingKeymap is the single binding source for the footer.
- SOLID principles: Good. newBandBase centralises the shared info-band base so signpost and command banner cannot diverge in bar/tint; renderCommandBand layers caret+chip on top (open/closed). renderCommandChip is a small focused helper.
- Complexity: Low. renderCommandBand is a flat compose-and-pad; clear segment assembly.
- Modern idioms: Yes. Idiomatic lipgloss JoinHorizontal/JoinVertical, value-receiver Model methods consistent with the package.
- Readability: Good. Thorough doc comments tie each function to its spec section.
- Issues: One latent width-overflow edge (below), pre-existing parity with the old plain status line.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] internal/tui/notice_band.go:294-304 — renderCommandBand JoinHorizontals bar+caret+text+chip and pads via noticeBandPadRight, but never wraps/truncates: when the joined row exceeds w (a long command at narrow width), padRightWithStyle (header.go:225-231) returns the row unchanged, so the band line overflows the band width. fillCanvas (model.go:3761-3768) explicitly does NOT truncate over-width lines, so the terminal soft-wraps the over-width band line — visually corrupting the layout and (since projectBandHeight reserves only the un-wrapped 1-line height) potentially pushing the composed view past termH. Sibling-verifier flag CONFIRMED as a real latent defect, but NON-BLOCKING: (a) it is parity with the pre-reskin `"Select project to run: " + cmd` plain line, which also did not wrap/truncate; (b) common commands (npm run dev, go test ./...) are short and fit; (c) §11.4 does not pin chip wrapping and the §2.7 "never overflow" rule is scoped to name truncation, not the command band; (d) all stated acceptance criteria pass. Concrete fix when prioritised: either truncate the chip command with … to the available width, or route the banner through the wrapping renderNoticeBand path (and let projectBandHeight pick up the extra row, as its comment at model.go:1378 already anticipates "more if the banner ever wraps").
- [quickfix] internal/tui/command_pending_band_test.go — add a narrow-width / long-command case for renderCommandBand asserting the band does not exceed w (would lock in whatever the chosen overflow behaviour from the [bug] above becomes, and guard the regression). Concrete edit at a known location (the existing band-width test block, ~line 111-122).

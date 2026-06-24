AGENT: duplication
FINDINGS:
- FINDING: Vestigial per-modal footer/key-hint wrappers left dead after consolidation
  SEVERITY: medium
  FILES: internal/tui/kill_modal.go:67-75, internal/tui/delete_modal.go:70-78, internal/tui/rename_modal.go:177-179
  DESCRIPTION: A round of consolidation moved the modal footer shape into the shared
    renderConfirmCancelFooter / renderKeyHint (modal_footer.go) and the destructive-confirm
    grammar into renderDestructiveConfirm (destructive_confirm.go), but the superseded
    per-modal wrappers were never removed. killModalFooterRow (kill_modal.go:67) and
    deleteModalFooterRow (delete_modal.go:70) now have ZERO production callers — the kill
    and delete modals render their footers through destructiveFooterRow instead — and
    survive only because their own golden tests still call them (modal_footer_test.go:256,
    274). All three *ModalKeyHint wrappers (killModalKeyHint kill_modal.go:73,
    deleteModalKeyHint delete_modal.go:76, renameModalKeyHint rename_modal.go:177) are
    likewise production-dead: each is a byte-identical one-liner returning
    renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless), referenced only by
    their own tests (modal_footer_test.go:152/169/187). This is exactly the copy-paste
    drift the consolidation was meant to eliminate: three identical wrappers + two dead
    footer-row clones that a future edit could silently leave inconsistent with the live
    shared path. (The modal_footer.go header comment even names these functions as the
    ones it absorbed, but the bodies were never deleted.)
  RECOMMENDATION: Delete the two dead footer-row functions (killModalFooterRow,
    deleteModalFooterRow) and all three dead *ModalKeyHint wrappers, then drop the
    corresponding sub-tests (the production paths through destructiveFooterRow /
    renderKeyHint are already covered by modal_footer_test.go's renderConfirmCancelFooter
    and renderKeyHint golden cases). renameModalFooterRow stays — it has a live caller
    (rename_modal.go:71) — but it too is a thin wrapper over renderConfirmCancelFooter and
    could be inlined at its single call site.

- FINDING: Identical accent.blue key-hint wrappers re-authored per surface
  SEVERITY: low
  FILES: internal/tui/edit_modal.go:554, internal/tui/pagepreview.go:311, internal/tui/kill_modal.go:73, internal/tui/delete_modal.go:76, internal/tui/rename_modal.go:177
  DESCRIPTION: Five functions across five files have byte-identical bodies — each returns
    renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless): editFooterGroup
    (edit_modal.go:554, live), previewFooterHint (pagepreview.go:311, live), and the three
    dead *ModalKeyHint wrappers from the finding above. The two live ones (editFooterGroup,
    previewFooterHint) wrap nothing more than the accent.blue default that renderKeyHint
    already supports; they exist only to spare the caller passing the token. This is a
    Rule-of-Three trigger: the same trivial "key hint in accent.blue" wrapper has been
    independently re-authored by each surface's task executor.
  RECOMMENDATION: Collapse to a single shared helper — either export a
    renderBlueKeyHint(key, label, mode, colourless) in modal_footer.go that pins
    AccentBlue, and route editFooterGroup's and previewFooterHint's call sites through it,
    or simply call renderKeyHint(..., theme.MV.AccentBlue, ...) directly at the two live
    call sites and delete the wrappers. Combined with deleting the three dead *ModalKeyHint
    wrappers, this leaves one canonical blue-key-hint path.

- FINDING: Duplicated separator/gap literal constants across files
  SEVERITY: low
  FILES: internal/tui/edit_modal.go:65, internal/tui/footer.go:45, internal/tui/help_modal.go:50, internal/tui/modal_footer.go:61
  DESCRIPTION: The same chrome literals are declared independently in multiple files
    rather than shared. The " · " dot separator is declared twice: editFooterSep
    (edit_modal.go:65) and footerEntrySeparator (footer.go:45) — both used to join footer
    groups in text.detail, both meant to match the §3.4 condensed-footer dot rhythm, yet
    they are separate constants that could drift if one footer's spacing is tweaked. The
    "   " three-space gap is likewise declared twice: helpColumnGap (help_modal.go:50) and
    modalFooterGap (modal_footer.go:61). The same " · " also appears inline (not as a
    const) inside the empty-state and no-matches hint strings (empty_states.go:35/45,
    filtering_no_matches.go:33), so the footer-dot convention is expressed three different
    ways. These are small, but they are the kind of magic-string near-duplicate the
    code-quality "magic strings" guidance flags, and the editFooterSep/footerEntrySeparator
    pair is a genuine same-role separator authored twice.
  RECOMMENDATION: Promote a single shared footerEntrySeparator (footer.go) and have the
    edit modal reference it instead of its own editFooterSep — they encode the identical
    §3.4 dot-separator role. The "   " gap is a weaker case (helpColumnGap is a body-column
    gap, modalFooterGap a footer-group gap — arguably distinct roles), so leave those
    separate unless a shared "modal spacing" token is wanted; if consolidating, name it by
    role, not by both use-sites.

- FINDING: Contrast-validation swatch reimplements the owned-canvas fill + on-canvas styling
  SEVERITY: low
  FILES: internal/capture/swatch.go:111-137, internal/capture/swatch.go:149-292, internal/tui/model.go:3474-3555
  DESCRIPTION: internal/capture/swatch.go is a standalone validation surface that locally
    re-derives several primitives the tui package already owns: its fillCanvas
    (swatch.go:111) re-implements the per-line canvas pad-to-width + fill-to-height +
    blank-row logic of Model.fillCanvas / insetCanvasCanvas (model.go:3474, 3531); its
    onCanvas/on/tintLabel/pairCaption closures (swatch.go:151, 199, 209, 233) re-author the
    "Foreground(token) over Background(canvas/tint)" leaf-style shape that headerStyle /
    noticeBandFgStyle (header.go:87, notice_band.go:148) already express; and padBand
    (swatch.go:286) mirrors headerPadRight / noticeBandPadRight. This is the one area where
    the otherwise-thorough cross-file consolidation does not reach — the swatch was built
    by a separate task as a deliberately-separate surface that "does NOT route through
    tui.Build" (per its own doc + capturetool/main.go:80). It is borderline by design: the
    tui helpers are unexported methods/free funcs in a different package, and the swatch
    must stay independent of tui.Build. Flagging it as the lowest-impact item because the
    duplicated logic is genuinely small and the package boundary is intentional.
  RECOMMENDATION: Leave as-is unless the canvas-fill logic changes — but if Model.fillCanvas's
    geometry is ever revised, treat swatch.fillCanvas as a known parallel copy to update in
    lockstep (or, if a shared seam is wanted, extract the pure pad-to-width/fill-to-height
    helper into a small leaf the swatch and the model can both call). Do not force a shared
    abstraction now; the boundary is deliberate and the duplication is minor.
SUMMARY: The implementation is exceptionally well-consolidated — modals, footers, headers,
  notice bands, empty states, row delegates, and the preview/help chrome already route
  through shared helpers (renderJoinedPanel, renderKeyHint, rowBgStyle, renderNoticeBand,
  renderEmptyStateBody). The remaining duplication is mostly residue from prior
  consolidation rounds: dead per-modal footer/key-hint wrapper clones, a five-copy
  accent.blue key-hint wrapper, a pair of same-role separator constants, and a deliberately
  separate capture-tool surface that re-derives a little canvas-fill/styling logic.

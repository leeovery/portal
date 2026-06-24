TASK: spectrum-tui-design-4-7 — Preview `?` help wiring (Phase-3 carry): Preview keymap descriptor (§12.1) added to the single-source descriptor type + bind `?` on Preview to overlay the generic 3-4 help renderer WITHOUT blanking the preview.

ACCEPTANCE CRITERIA:
- Pressing `?` on the Preview overlay opens the help modal listing the COMPLETE Preview keymap (scroll, page, top/bottom, ←→ window, Tab pane, Enter attach, Space/Esc back) from the Preview descriptor.
- The help OVERLAYS the preview without blanking it (§8.1 exception) — preview content stays visible behind it; NOT routed through the §3-1 blank-screen clear path.
- The help reuses the §3-4 generic descriptor-driven renderer — no hand-authored Preview copy; `? Keybindings` header + `esc close` + two-column layout come from the shared renderer.
- Key-exclusive: `?` toggle-closes, Esc dismisses the help and does NOT fall through to the preview-back action; other preview keys inert while help is open.
- Preview keymap descriptor added to the single-source descriptor type that drives footers + help; lists the complete §12.1 keymap including keys not in any footer.
- Under NO_COLOR the help overlay renders colourless over the preview.
- vhs: drive to Preview → `?` → write testdata/vhs/preview-help.png; overlay-without-blanking + descriptor-driven content verified; Preview help not separately mocked.

STATUS: Complete

SPEC CONTEXT:
- §8.5: the `?` help is generated from the page's keymap descriptor (single source of truth that also drives the footer + §12.1), lists the COMPLETE keymap (incl. footer keys), is key-exclusive (Esc dismisses, no fall-through), and opened from Preview OVERLAYS it (doesn't blank — the documented §8.1 exception).
- §9.3: Preview keys are scroll ↑↓ + Ctrl↑/↓, ←/→ window, Tab pane, Enter attach (this pane), Space/Esc back; a `?` help opened here overlays the preview.
- §12.1: Preview keymap (the spec body still shows the pre-restructure `]`/`[` window binding; the 2026-06-22 corrigendum + the task-4-6/4-7 in-source notes carry the restructure to ←/→ window + Tab pane — the code follows the restructured form, which is correct per the corrigendum).
- §15.1: Preview help is NOT separately mocked — the capture follows the audited keymap.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/keymap.go:183-193 — previewKeymap() descriptor extended to the COMPLETE §12.1 keymap: scroll (↑/↓), page (^↑/↓), Home/End top-bottom, ←→ window (Core), ⇥ pane (Core), ⏎ attach (Core), ␣ back (Core). The four nav-hints are Core (footer); scroll/page/top-bottom are help-only — matching §8.5 "the full reference, not just the footer's overflow".
  - internal/tui/pagepreview.go:346 — helpOpen bool field on previewModel.
  - internal/tui/pagepreview.go:576-588 — handlePreviewKey help-open branch: while helpOpen, `?` (isRuneKey) and Esc (keyIsCode) set helpOpen=false and consume the key with a nil cmd (no previewDismissedMsg); a final `return true, m, nil` consumes every other key inert. Evaluated FIRST so no binding leaks while the overlay is up.
  - internal/tui/pagepreview.go:592-595 — closed-state `?` arm: opens the overlay (helpOpen=true), consumes the key (no cmd). Ordered before the Esc/Space back arm so `?` wins.
  - internal/tui/pagepreview.go:715-718 — View() overlay dispatch: when helpOpen, returns overlayHelpOnPreview(preview, previewKeymap(), …) instead of the bare preview.
  - internal/tui/pagepreview.go:721-747 — overlayHelpOnPreview composites renderHelpModalContent (the SAME §3-4 generic renderer Sessions/Projects use) centred over the composed preview via lipgloss.NewCompositor (background Z=0, panel Z=1) — NOT the blank-screen path. Clamp on x/y keeps an over-wide panel at top-left.
  - internal/tui/model.go:2986-2996 — Space handler constructs a fresh previewModel then assigns pmodel.mode = m.canvasMode / pmodel.colourless = m.colourless (so helpOpen defaults false on every (re)open — no stale carry).
  - internal/tui/model.go:2216-2222 — pagePreview key input routes through m.preview.Update; model.go:3778-3779 renders m.preview.View() so the overlay reaches the screen.
- Notes: Descriptor↔dispatch correspondence is enforced for Preview by TestPreviewDescriptorDispatchParity (keymap_dispatch_guard_test.go:254-308), which two-way-checks every non-help descriptor Key against the live handlePreviewKey (scroll/page are asserted viewport-delegated i.e. NOT preview-intercepted; ←→/⇥/⏎/␣/Home-End are asserted preview-owned). The `?` RightAligned entry is the allow-listed exception, its dispatch pinned by the TestPreviewHelp* suite — correct per the §8.5 single-source contract.

TESTS:
- Status: Adequate
- Coverage (internal/tui/pagepreview_help_test.go): every Tests bullet from the task definition is covered 1:1 —
  - TestPreviewHelpOpensOnQuestionMark — `?` opens, no cmd, and the View lists all six action labels from the descriptor (scroll, page, prev/next window, next pane, attach, back).
  - TestPreviewHelpOverlaysWithoutBlanking — asserts the `Keybindings` header + `esc close` AND that the `◉ preview` marker + scrollback body ("hello scrollback line") survive behind the panel (the load-bearing distinction from the blank-screen path).
  - TestPreviewHelpReusesGenericRenderer — asserts the View composites renderHelpModalContent(previewKeymap(), …) line-for-line (no hand-authored copy).
  - TestPreviewHelpTogglesClosedOnSecondQuestionMark — second `?` closes, no cmd, panel gone.
  - TestPreviewHelpEscDismissesWithoutBackingOut — Esc closes help with a nil cmd (the explicit "no previewDismissedMsg fall-through" guard).
  - TestPreviewHelpConsumesOtherKeysWhileOpen — table over left/right/tab/enter/up/down/home/end: help stays open, cmd nil, focus unchanged.
  - TestPreviewBackResumesWhenHelpClosed — Esc/Space with help closed each emit previewDismissedMsg.
  - TestPreviewHelpRendersColourlessUnderNoColor — colourless overlay carries no `\x1b[38;`/`\x1b[48;` SGR while header + marker survive.
  - keymap_dispatch_guard_test.go:TestPreviewDescriptorDispatchParity — descriptor↔dispatch lockstep, both directions.
  - pagepreview_keymap_constants_test.go — pins the footer canonical bytes (the Core subset of the same descriptor).
- Notes: Not over-tested — each test targets a distinct acceptance edge (overlay-not-blank, generic-renderer reuse, toggle, Esc-no-fall-through, key-exclusivity, back-resume, NO_COLOR). Not under-tested — the descriptor↔dispatch guard plus the byte-exact renderer-reuse test close the two drift gaps (descriptor content and hand-authored-copy). Would fail if the feature broke: removing the helpOpen branch breaks the toggle/Esc/key-exclusivity tests; routing through the blank-screen path breaks the marker-survives assertions; hand-authoring copy breaks the verbatim-composite test.

CODE QUALITY:
- Project conventions: Followed. Reuses the shared renderHelpModalContent / renderJoinedPanel / headerStyle / isRuneKey / keyIsCode primitives; no raw hex (descriptor + theme tokens only); the §2.5 NO_COLOR carve-out flows through unchanged; value-receiver style on previewModel preserved (helpOpen is a plain bool field, returned by value from handlePreviewKey).
- SOLID principles: Good. overlayHelpOnPreview is a single-responsibility composite helper; the help renderer is reused rather than duplicated (DRY) — the three help surfaces share one renderer, so they cannot drift.
- Complexity: Low. The help-open branch is a flat guard at the top of handlePreviewKey; the View dispatch is a single `if m.helpOpen`.
- Modern idioms: Yes. lipgloss.NewCompositor layer model for the overlay; max() clamps for the centre offsets.
- Readability: Good. Comments tie each branch to §8.5/§9.3 and explain the ordering rationale (help-first so no binding leaks; `?` before back so it wins).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/help_modal.go:31-36 — the "NOTE (Phase 4, deferred)" block states the Preview `?` help + Preview keymap descriptor "are NOT built here ... intentionally out of scope for this task — when Phase 4 wires it ...". That wiring is now shipped (this task) and routes through renderHelpModalContent exactly as the note prescribed. The note is now stale and contradicts the code; update it to past tense (e.g. "Phase 4 task 4-7 wired the Preview `?` help through these SAME renderers — see overlayHelpOnPreview in pagepreview.go") so the file doc no longer claims an unbuilt arm. Documentation-only, no logic impact.
- [idea] internal/tui/pagepreview.go:716 / keymap.go:188 — the help body renders the window entry's glyph `←→` (descriptor Key, no HelpKey), which in JetBrains Mono compresses visually toward a single double-headed arrow in the capture; the footer uses the same `←→`. Consider whether a help-body HelpKey override (e.g. the spaced `← →` or the `↔` the PNG visually resolves to) reads more clearly in the help panel — purely a glyph-legibility judgment call, the current form is spec-faithful (matches the §9 restructured `←→` window binding) and the footer/help stay consistent. Defer-able; decide whether it is worth diverging the help glyph from the footer glyph.

VISUAL VERIFICATION:
- testdata/vhs/preview-help.png inspected. Confirms: (1) the help panel overlays WITHOUT blanking — the cyan `◉ preview` marker, `Window 1/1 · Pane 1/1` counters, the cyan-bordered captured scrollback (kubectl/make/aviva-proxy lines), and the cyan footer `↔ window  ⇥ pane  ⏎ attach  ␣ back` all remain visible behind the centred panel; (2) the panel is the descriptor-driven generic renderer — `? Keybindings` header (violet `?`), right-aligned `esc close`, two-column accent.blue glyph / text.strong action; (3) it lists the COMPLETE §12.1 keymap incl. the footer-absent scroll/page/Home-End rows. Matches the Preview Screen (MV) chrome + the Sessions Help Modal renderer shape. Preview help is not separately mocked (the capture follows the audited keymap, per §15.1).

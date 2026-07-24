AGENT: architecture
FINDINGS:
- FINDING: "Unconditional under colourless" hyperlink emission is unpinned by any test
  SEVERITY: low
  FILES: internal/tui/unsupported_banner_test.go:159-176, internal/tui/section_header.go:179-189
  DESCRIPTION: The load-bearing design decision of this change is that the OSC 8
    hyperlink is emitted UNCONDITIONALLY — it rides through the NO_COLOR/colourless
    carve-out because OSC 8 is orthogonal to colour (spec §Banner link; the code
    comment at section_header.go:181-184 restates this as the central invariant).
    The colour path pins it correctly: TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs's
    blueRun assertion includes .Hyperlink(unsupportedDocsURL), so dropping the chain
    fails that test. But the colourless path is the one where the "unconditional"
    claim is non-obvious and most at risk: TestUnsupportedHeader_ColourlessGlyphBacked
    only asserts (a) "see docs" survives ansi.Strip and (b) no canvas/fg colour
    sequences leak — it never asserts the OSC 8 escape is still present when
    colourless=true. A plausible future "NO_COLOR should strip all escapes" refactor
    that gated .Hyperlink behind `if !colourless` would satisfy every existing test
    while silently killing the deliberate cross-carve-out behavior. This is a seam
    gap between the OSC 8 emission and the NO_COLOR carve-out, not a functional bug
    (verified: lipgloss emits `\x1b]8;;<url>\a...` on a bare style, ansi.Strip removes
    it, lipgloss.Width stays 8 — geometry unaffected, exactly as designed).
  RECOMMENDATION: Add one assertion to TestUnsupportedHeader_ColourlessGlyphBacked
    (or a sibling) that the RAW colourless header still contains the OSC 8 wrapper —
    e.g. assert strings.Contains(header, unsupportedDocsURL) or the
    `\x1b]8;;`+unsupportedDocsURL prefix on the un-stripped colourless render. This
    locks the "emitted unconditionally / rides the carve-out" contract that the spec
    calls out as central, closing the one path where a colour-gating regression would
    go undetected.
SUMMARY: The change integrates cleanly with the existing section-header seams — the
  banner routes unchanged through the shared renderRightAnchoredSectionRow core, the
  .Hyperlink chain on the headerStyle result is the minimal right-sized approach with
  no new/exported API (local constant per spec), and OSC 8's zero-width property is
  confirmed so geometry, right-alignment, and one-row pagination are all unperturbed.
  The only gap is a low-severity test seam: the deliberate "hyperlink emitted even
  under colourless/NO_COLOR" behavior is asserted on the colour path but not the
  colourless path, leaving a colour-gating regression undetectable.

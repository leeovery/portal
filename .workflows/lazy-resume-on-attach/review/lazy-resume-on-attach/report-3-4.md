TASK: The Discard Confirmation (lazy-resume-on-attach-3-4) — `RenderResumeDiscardConfirm` built through the shared destructive-confirm builder, with the builder gaining `targetRows` / `reportRows` and a `destructiveConfirmCompartments` accessor

ACCEPTANCE CRITERIA:
- `TestKillDeleteModalContent_ByteIdenticalGolden` passes unchanged for all four cases; kill and delete modal suites pass with no edit
- `▲ Discard resume?` in `state.destructive` bold in the header, exactly as the kill modal's title
- Command in `state.destructive` over up to three rows, wrapped and `…`-marked by the waiting panel's shared block
- Consequence line verbatim, word-wrapped by the builder at its existing width in its existing muted token
- Footer `y discard   esc cancel` — confirm key/label from this screen, cancel half from the builder
- Non-empty report adds exactly one row, on the waiting panel's single report row, with `y` still offered
- Below the card's size, a plain stack carrying title, command, consequence, report when present and the footer — never empty
- No plain-stack row wider than the pane; consequence re-wrapped to the pane with every word present
- Under colourless the `▲`, title, consequence and key words render, and stripped output matches the coloured render
- Card width does not move with the command's or the report's length
- Nothing structural differs from the kill modal (compartments, frame, inset, dividers)

STATUS: complete

SPEC CONTEXT: §5.4 defines the confirmation as the kill modal retitled, built through the same destructive-confirm builder, carrying the verbatim consequence line (corrigendum 2026-09-19), a report row for a discard the store refuses, and degrading with the pane as the waiting panel does. §5.2 governs the degrade-below-card-size rule and NO_COLOR glyph/word carriage; §6.2/§6.3 fix `y` as confirm and Escape as cancel. Key dispatch and the removal itself belong to later phases.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/destructive_confirm.go:20-34 — `targetRows` and `reportRows` added to `destructiveConfirmSpec`
  - internal/tui/destructive_confirm.go:36-38 — `renderDestructiveConfirm` reduced to `renderJoinedPanel(destructiveConfirmCompartments(spec, destructiveBodyWidth, …))`
  - internal/tui/destructive_confirm.go:44-49 — `destructiveConfirmCompartments(spec, wrapWidth, th, colourless)`
  - internal/tui/destructive_confirm.go:60-71 — `targetRows` replaces the name row when non-empty; `reportRows` appended after the consequence
  - internal/tui/destructive_confirm.go:83-92 — consequence wraps at the passed `wrapWidth`
  - internal/tui/resume_discard_confirm.go:5-11 — title, consequence and label constants, verbatim from the spec
  - internal/tui/resume_discard_confirm.go:17-23 — `RenderResumeDiscardConfirm` routed through `renderPaneScreen`
  - internal/tui/resume_discard_confirm.go:25-27 — card built at `resumeCardContentWidth` via `renderDestructiveConfirm`
  - internal/tui/resume_discard_confirm.go:31-38 — plain stack flattened from `destructiveConfirmCompartments` at the pane width
  - internal/tui/resume_discard_confirm.go:40-52 — spec built from `resumeCommandRows(…, StateDestructive, true, …)` and `resumeReportRow`
- Notes:
  - For a spec setting neither new field, `destructiveBodyRows` yields the same rows as before: name row, extras, blank, consequence at `destructiveBodyWidth`, no report. The framed path passes that same width. Kill and delete modal sources (internal/tui/kill_modal.go, internal/tui/delete_modal.go) were not touched, so their byte-identity holds by reading.
  - The plan's `discardKeyConfirm = "y"` constant is replaced by `string(resumekeys.Confirm)` (internal/resumekeys/resumekeys.go:13). A later task made this change so that the waiter's dispatch and the footer read one declaration. It is sound and keeps the rendered `y` unchanged, so it is not a finding.
  - Card width is pinned: command and report rows are padded to 52 by `resumePaddedRow`, and the report is truncated there. Header, footer and consequence rows fit inside 52.
  - The stack builds the command, report and consequence at the pane width, and it takes the same `resumeCommandRows` path the waiting panel's stack takes. Title and footer are fixed strings, and the pane clamp (`clampStackToPane`) handles them below their own width. This matches the waiting panel.

TESTS:
- Status: Adequate
- Coverage:
  - All twelve named tests are present in internal/tui/resume_discard_confirm_test.go and run over dark/light × coloured/colourless through `forEachResumePartsRender`. The NO_COLOR test covers both themes at card and stack sizes.
  - Title row, full-card golden with and without a report, three-row long command, and verbatim consequence (wrap points pinned as literals) are covered. Also covered: footer row, one-extra-row report, card width across four command/report extremes, and the stack at width 30 with the pinned re-wrap. A word-presence check confirms nothing is cut. The stack-equals-card-content check runs at width 52.
  - Spec-level coverage in internal/tui/destructive_confirm_test.go:178-232: `targetRows` replacing the name row, `reportRows` following the consequence with exactly one extra row, and wrapping at the passed width.
  - `TestKillDeleteModalContent_ByteIdenticalGolden` (internal/tui/destructive_confirm_test.go:21-72) is untouched. The only edit to an existing test is the signature-forced width argument in `TestDestructiveConsequenceRows_WordWrapAt52` (line 160).
- Notes: The styling assertions for the title and command check the line containing the text rather than the isolated segment. They lean on the kill golden pinning the builder's shared header styling, which is sufficient because the discard title cannot be styled apart from that builder.

CODE QUALITY:
- Project conventions: Followed (token-only colours, shared builder reuse, no process-artifact comments)
- SOLID principles: Good — the compartment accessor serves both the frame and the stack from one assembly
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

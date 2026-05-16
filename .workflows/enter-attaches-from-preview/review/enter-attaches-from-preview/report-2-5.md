TASK: enter-attaches-from-preview-2-5 — Replace placeholder previewAttachBailMsg handler with refresh + exact-text flash dispatch

ACCEPTANCE CRITERIA:
- `case previewAttachBailMsg` calls `setFlash(formatSessionGoneFlash(msg.Session))` — met
- Flash text exactly `session "<name>" no longer exists` — met
- Handler returns `tea.Batch(refreshCmd, tickCmd)` — met
- `flashTickCmd` captures post-`setFlash` generation — met
- Flash render observable before refresh resolves — met
- Task 1-7 regression assertions (transition, preview zero, refresh dispatch, Esc unaffected) — met
- `tea.Sequence` is NOT used — verified

STATUS: Complete

SPEC CONTEXT: Spec § Session-killed-externally bail path > Behaviour pins the flash wording as `session "<name>" no longer exists` (literal double quotes, no trailing punctuation, no paraphrase). Spec § Render-frame ordering requires the flash render not be gated on refresh completion — bail handler must dispatch refresh and flash from same Update return via tea.Batch.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:974-992 — `case previewAttachBailMsg` handler
  - internal/tui/sessions_flash.go:79-89 — `formatSessionGoneFlash` helper
  - internal/tui/model.go:742-746 — `exitPreviewToSessions` shared helper (encapsulates page flip + preview zero + refresh dispatch, DRY with Esc-dismiss path)
- Notes: Correct ordering — `refreshCmd := m.exitPreviewToSessions(msg.Session)` runs first, then `m.setFlash(...)` bumps `flashGen`, then `flashTickCmd(m.flashGen)` captures the post-bump value. Uses raw double quotes via backtick string literal in `Sprintf` (not `%q`).

TESTS:
- Status: Adequate
- Coverage: All nine spec/plan-mandated test scenarios present:
  1. Exact spec wording (TestPreviewAttachBail_SetsFlashWithExactSpecWording)
  2. Literal double quotes (...FlashUsesLiteralDoubleQuotesAroundName)
  3. No trailing punctuation (...FlashHasNoTrailingPunctuation)
  4. flashGen bumped (...BumpsFlashGen)
  5. Returns batch with both refresh + tick (...ReturnsBatchWithRefreshAndTick)
  6. Tick captures post-bump gen (...TickCapturesPostBumpFlashGen)
  7. Flash visible before refresh resolves (...FlashVisibleBeforeRefreshResolves)
  8. Special chars preserved verbatim (...SpecialCharsInNamePreservedVerbatim)
  9. Empty session name does not panic (...EmptySessionNameDoesNotPanic)
  10. tea.Batch not tea.Sequence (...BailHandlerNotUsingTeaSequence) — clever discriminator via BatchMsg type assertion
  11. Pure helper unit tests (TestFormatSessionGoneFlash_ExactSpecWording) with 5 sub-cases including empty/unicode
- Notes: Test helpers `drainBatchCmds` and `findFlashTickMsg`/`findRefreshedMsg` are well-factored.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — handler delegates page-flip + refresh to `exitPreviewToSessions`, flash composition to `formatSessionGoneFlash`, tick scheduling to `flashTickCmd`.
- Complexity: Low.
- Modern idioms: Yes — uses `tea.Batch` correctly, backtick raw-string literal avoids `%q` escaping pitfalls.
- Readability: Good — extensive godoc on `formatSessionGoneFlash` calls out the byte-exact contract and forbids `%q`; handler comment cites spec sections.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.

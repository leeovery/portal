TASK: The Resume Panel's Act Keys Are Declared Once (lazy-resume-on-attach-7-3)

ACCEPTANCE CRITERIA:
- The waiting panel's footer still reads `⏎ resume   d discard` and the discard confirmation's `y discard   esc cancel`, card and plain stack alike, byte-identical to today
- On a waiting pane `d` still opens the confirmation and `y` on the confirmation still discards; `y` on the panel and `d` on the confirmation are still swallowed
- Each of the two act keys has exactly one declaration in the tree, in `internal/tui`, read by both the footer and the waiter's dispatch — `cmd` declares neither

STATUS: complete

SPEC CONTEXT: Section 4.3 of the spec says Enter and `d` act on the waiting panel and every other byte is swallowed. Section 6.2 says `d` opens a confirmation where only `y` and Escape act, and adds that "the pane never acts on a key the screen in front of the user does not offer". The spec derives `d` from the picker's delete key and `y` from its two destructive confirmations. Section 5 fixes the footer copy (`⏎ resume` / `d discard`; `y discard   esc cancel`) at every size, card and plain stack alike. This task is a behaviour-preserving refactor that keeps the key each footer names tied to the key the waiter dispatches on.

IMPLEMENTATION:
- Status: Implemented. It drifts from the task's stated location, and the drift is sound.
- Location:
  - internal/resumekeys/resumekeys.go:9-14 — the single declaration: `Discard byte = 'd'` and `Confirm byte = 'y'`
  - internal/tui/resume_panel.go:78 — `resumeKeyHintRow` renders `string(resumekeys.Discard)`. Both the card (:42) and the plain stack (:67) go through this row.
  - internal/tui/resume_discard_confirm.go:45 — `discardConfirmSpec` sets `confirmKey: string(resumekeys.Confirm)`. Both the card (:26) and the stack (:32) are built from this spec.
  - cmd/state_resume_wait.go:138 and :145 — `resumeKeysFor` keys its dispatch maps on `resumekeys.Confirm` / `resumekeys.Discard`. cmd's `resumeKeyDiscard` and `resumeKeyConfirm` are deleted. `resumeKeyEnterCR`, `resumeKeyEnterLF` and `resumeKeyEscape` (:22-26) and tui's `resumeKeyResume` (resume_panel.go:13) are unchanged.
- Notes:
  - Where the keys live: the task said to declare them in `internal/tui`, but they are in a new stdlib-only leaf, `internal/resumekeys`. This divergence is required, not a loss. A guard that predates this task (added by 4-2), `cmd/state_resume_wait_test.go:335-346`, fails if `state_resume_wait.go` imports `internal/tui`, `internal/theme` or `internal/prefs`. The waiter's file therefore cannot read a constant declared in `internal/tui`. The task's reasoning that "cmd already imports internal/tui" holds for the package but not for this file.
  - The leaf keeps the substance of criterion 3: one declaration per key, read by both footers and the dispatch, with none in `cmd`. `rg 'resumeKeyDiscard|discardKeyConfirm|resumeKeyConfirm' -g '*.go'` now returns nothing.
  - Other `'d'`/`'y'`/`"d"`/`"y"` literals in production code belong to the picker (theme panel, kill/delete modals, model dispatch), not the resume panel.
  - The package guard (internal/resumekeys/leaf_guard_test.go) pins the stdlib-only dependency set on both lanes. The package is registered in the surface-audit allow-list (internal/tui/pagepreview_surface_audit_test.go:204), and the CLAUDE.md row describes it accurately.
  - `string(byte)` yields "d" / "y". `go vet`'s stringintconv check exempts byte and rune, so the conversion is clean.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1 (byte-identical footers) is pinned by literal goldens that are independent of the constant: internal/tui/resume_panel_test.go:122 (`"d discard"`), internal/tui/resume_discard_confirm_test.go:124 (`"y discard   esc cancel"`), and the integration constants internal/restore/lazy_resume_panel_integration_test.go:54 and lazy_resume_discard_integration_test.go:40. The four tui test references moved to `string(resumekeys.X)` with their assertions unchanged, as the task prescribed.
  - Criterion 2 is pinned by literal bytes in the cmd suites. cmd/state_resume_screens_test.go:540-541 swallows `y` on the panel and `d` on the confirmation. :647 and :651 hand off on `d` and discard on `y`, and cmd/state_resume_discard_test.go:24 and :258 confirm with `y`. Changing a constant's value would fail these tests.
  - The new cmd/state_resume_wait_test.go:545-562 (`TestResumeKeysFor_AnswersToTheActKeysTheFootersName`) is the dispatch guard the task's Problem statement says was missing. It asserts that each screen answers to the key its footer renders and swallows the other screen's key. It fails if the dispatch is ever keyed on a value that differs from the shared declaration.
- Notes: The tests are not over-tested. The new test is four focused map-membership assertions that no other test duplicates, since the others go through literal bytes.

CODE QUALITY:
- Project conventions: Followed. The leaf package and its guard mirror `internal/resumemode` / `internal/shellquote`, and the audit allow-list and CLAUDE.md are updated.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The package doc and constant comments are accurate against the code: the leaf rationale matches the waiter import guard, and Enter/Escape are excluded because each has a distinct display form.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

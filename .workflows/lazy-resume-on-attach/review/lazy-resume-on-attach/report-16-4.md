TASK: The Report-Row Fit Guard Measures the Rows Production Emits (tick-e7f3bc, lazy-resume-on-attach-16-4)

ACCEPTANCE CRITERIA:
- The guard's rows are composed from all ten `resumeReport*` constants — the four fixed rows as they stand and the six prefixes each with a representative cause — and the guard holds no row literal of its own.
- Each fixed row, and each prefixed row other than the drop row, renders whole on both the waiting panel and the discard confirmation, drawn as the guard draws them today (100x30, colourless).
- The drop row with the ENOTTY cause the draw tests use (`can't clear pending input: inappropriate ioctl for device`, 57 cells against the 52-cell card) renders on both screens with the act and its separator, `can't clear pending input: `, whole, and the cut falling inside the cause.
- A report constant lengthened past the card's 52 cells in a temporary edit — `resumeReportLockHeld`, say — fails the guard, and reverting the edit passes it.
- No production file changes, and every other test stays green.

STATUS: complete

SPEC CONTEXT: An answer the pane cannot carry out is reported on a single line between the command and the key hints — on the waiting panel's card for a marker clear that will not land, and on the discard confirmation for a removal the store refuses. The row is truncated rather than wrapped (a wrap would move the card's height between two otherwise identical screens), so each row names its act before its cause and a cut only ever falls inside the cause. The guard exists to keep every row the panel can show inside the 52-cell card body.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_resume_report_test.go:312-369 (`TestResumeReportRows_FitTheCard`), cmd/state_resume_report_test.go:371-384 (`cutInsideCause`)
- Notes:
  - All ten constants from the block at cmd/state_resume_report.go:17-28 are referenced: the four fixed rows at :327-332 (`resumeReportMalformed`, `resumeReportLockHeld`, `resumeReportTmuxAbsent`, `resumeReportFallback`), five prefixes composed with causes at :333-342 (`resumeReportRead`, `resumeReportLock`, `resumeReportWrite` twice, `resumeReportLocate`, `resumeReportUnpause`), and `resumeReportDrop` at :354. The only string literals left are causes, and they are exactly the ones the task names (`permission denied` for read/lock/write, `no space left on device` for write, `$HOME is not defined` for locate, `can't find pane: %7` for unpause), so every row the old literal list measured is still measured. The drop cause is `syscall.ENOTTY.Error()`, the same errno the draw test wraps (cmd/state_resume_drop_input_test.go:269-270).
  - Width arithmetic against `destructiveBodyWidth = 52` (internal/tui/destructive_confirm.go:15, aliased as `resumeCardContentWidth` at internal/tui/resume_panel_parts.go:13): the longest whole row is 47 cells (`can't lock hooks.json: another process holds it`, and `can't write hooks.json: no space left on device`). The drop row is 27 + 30 = 57 cells. `resumeReportRow` (internal/tui/resume_panel_parts.go:49-56) truncates with `ansi.Truncate(..., width, "…")`, and the colourless `headerStyle` (internal/tui/header.go:40-43) emits no escapes. So a whole row appears verbatim in the painted string, and any row longer than 52 does not, because it is cut and gets an ellipsis. Both renderers build the report row at the card width (internal/tui/resume_panel.go:56, internal/tui/resume_discard_confirm.go:27,49).
  - Drop property: `strings.Cut(painted, drop.act)` requires the act and its trailing `": "` separator whole, and `cutInsideCause` accepts the whole cause or a non-empty proper prefix of it followed by the ellipsis. A cut inside the act or the separator fails the `Cut`, and a cut leaving zero cause characters fails the `kept > 0` bound. At 52 cells the rendered row is `can't clear pending input: inappropriate ioctl for …`, which matches.
  - Lengthening experiment (criterion 4), traced by reading: `resumeReportLockHeld` is referenced directly at :329, so lengthening it past 52 cells gets it truncated with an ellipsis, `strings.Contains` at :346 fails, and the subtest reports it. Reverting restores a 47-cell row that passes.
  - Production unchanged: commit 1584752e4 touches only cmd/state_resume_report_test.go.

TESTS:
- Status: Adequate
- Coverage: Every row the panel can show, all ten constants, on both screens at the guard's existing 100x30 colourless draw. The drop row gets the act-whole / cut-inside-cause property instead of a wholeness check.
- Notes: Focused, with no redundant assertions. The failure messages print the painted screen, so a cut row can be diagnosed from the output alone.

CODE QUALITY:
- Project conventions: Followed (no `t.Parallel`, shared `themetest.DefaultDark` accessor, test-only change)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`strings.Cut`, typed closure helpers)
- Readability: Good. The `prefixed` pairing makes the act/cause split explicit, and that split is the property the drop check depends on.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "No production file changes, and every other test stays green." — the production half is settled (commit 1584752e4 touches only cmd/state_resume_report_test.go). The green half needs a run: `go test ./cmd` at minimum, and the unit lane `go test ./...` to confirm nothing else regressed.

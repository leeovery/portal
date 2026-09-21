## Attempt 1

ISSUES:
- `internal/tui/resume_panel_parts.go:44-50` (`wrappedLines`) — a command whose first character is whitespace, or a control byte `sanitiseCommandText` maps to a space, followed by a token longer than the width, returns a first row **one cell past the pinned width**. `ansi.Wrap` does not count a line's *leading* whitespace toward the limit, and `headerPadRight` cannot shrink an over-wide segment, so the row ships at 53 and `renderJoinedPanel` (which sizes `contentWidth` to the widest row, `internal/tui/panel.go:29-38`) widens the whole card — frame, dividers and every other row with it. That is the property this task exists to pin, and on the discard confirmation it also makes the card one column wider than the kill modal it is meant to be identical to.
  Measured against the real package (a build overlay adding a probe test to `internal/tui` — the working tree was not modified), at `resumeCardContentWidth`, both colourless values, `themetest.DefaultDark`:
  - `" " + strings.Repeat("a", 60)` -> row 0 width **53**
  - `"\t" + strings.Repeat("a", 60)` -> row 0 width **53**
  - `"\x1b[31m" + strings.Repeat("a", 60)` -> row 0 width **53**
  - `" /Users/leeovery/Library/Application_Support/portal/state/scrollback-0000000000.bin"` -> row 0 width **53** (a plain leading space before a long path — no exotic input required)
  - card built from those rows: `lipgloss.Width` **59** vs **58** for an ordinary command
  A second, smaller face of the same defect: at widths <= 4 `ansi.Wrap` itself returns over-wide lines even with no leading whitespace (at width 3, `"claude --resume …"` yields a line `"ude --r"`, 7 cells), and a double-width rune cannot fit a 1-column pane at all — both reachable through task 3-2's degraded stack, whose own table runs down to `1x1`.
  FIX: in `wrappedLines`, trim each wrapped line at both ends and clamp anything still over the width:
  ```go
  func wrappedLines(text string, width int) []string {
  	lines := strings.Split(ansi.Wrap(text, width, ""), "\n")
  	for i, line := range lines {
  		lines[i] = clampToWidth(strings.Trim(line, " "), width)
  	}
  	return lines
  }

  func clampToWidth(line string, width int) string {
  	if ansi.StringWidth(line) <= width {
  		return line
  	}
  	return ansi.Truncate(line, width, "")
  }
  ```
  Verified under the same overlay: the executor's whole suite stays green, both probe assertions pass, and over 3000 adversarial commands (leading/trailing/interior whitespace runs, CJK, combining marks, emoji and ZWJ sequences, embedded `\n`/`\t`/ESC/BEL/DEL, unbroken 40+ char tokens) at **every** width 1..60 no row is ever off the requested width. The comment above `wrappedLines` should then name the rule rather than only the trailing-space case it currently names. Tests to add alongside: a command beginning with a space, a tab and an ESC followed by a token longer than the width, asserted at both `resumeCardContentWidth` and the narrow width — the existing `assertRowsAtWidth` catches it once the fixture is placed there.
  ALTERNATIVE: clamp only (keep `TrimRight`, add the `ansi.Truncate` clamp) — one rule instead of two, but it drops the last character of the long token instead of the meaningless leading space, so a character of the user's command silently disappears in the common case. Trim-then-clamp loses nothing except in the sub-4-column widths where `ansi.Wrap` has already failed. The reviewer recommends trim-then-clamp, and so does the orchestrator.
  CONFIDENCE: high

NOTES (non-blocking):
- The trailing-space trim you added is real and not a fabricated problem — the reviewer measured it independently. It cannot move a word between lines, and a command whose own text ends in spaces loses only trailing spaces on its last row, which the canvas padding replaces with the same cells. Interior whitespace runs survive intact.
- Sanitisation checks out: ESC becomes a space and the remainder survives as literal text, `\x7f` and `\x07` likewise, and no stripped row carries a control character at any width tested. C1 controls (U+0080-U+009F) are outside the prescribed rule; a raw 0x9b byte decodes to U+FFFD so it cannot reach a terminal as a CSI introducer, but a deliberately UTF-8-encoded U+009B passes through. Spec-conformant as written — recorded so the choice is known.
- A non-positive width is not resolved here; task 3-2's ladder resolves non-positive dimensions before calling either helper, so no caller can hand a zero today. No change asked for.
- `themePanelMessageText` and `notice_band.go` encode the same wrap-then-truncate rule without a width clamp. The reviewer measured both and neither overflows at any realistic width, so nothing was banked. If this clamp lands, a shared wrap-to-rows helper is the natural phase-boundary consolidation.

## Attempt 2

ISSUES:
- `internal/tui/resume_panel_parts_test.go` (whole suite; nearest anchors are the wrap tests at lines 82-110 and 198-208) — no test asserts the *text* of any row the helper wrapped. Exact text is asserted only for `"vim"` (line 77, one row, no wrap) and the empty command (line 216); every wrapped case asserts width plus a `…` suffix, and the control-character test (line 148) compares one render against another render of the same input class, so both sides would carry the same damage. The machinery added in the fix round — `strings.Trim(line, " ")` and `clampToWidth`'s marker-less `ansi.Truncate(line, width, "")` — is precisely what can silently eat content, and a regression in it (trimming a wider cutset, clamping to `width-1`, dropping a middle line) keeps every width pinned and every `…` in place, so the whole suite still passes while the user reads a mangled command with nothing to tell them it was cut. The property holds today; nothing holds it tomorrow.
  FIX: Add one subtest to `TestResumeCommandRows_Geometry` (inside `forEachResumePartsRender`, so it runs over both themes x both colourless values) asserting exact row text for a deterministic wrap, using the existing `rowText` helper. Measured values, verified by the reviewer against the current code:
  `resumeCommandRows("claude --resume 4f2c9a1e --cwd /Users/leeovery/Code/portal", resumePartsNarrowWidth, …)` -> `[]string{"claude --resume 4f2c9a1e", "--cwd", "/Users/leeovery/Code/po…"}`; the same command at `resumeCardContentWidth` -> `[]string{"claude --resume 4f2c9a1e --cwd", "/Users/leeovery/Code/portal"}`. Name it in the suite's voice, e.g. `"it carries the command's text across the rows it wraps to"`.
  ALTERNATIVE: A property assertion instead of (or beside) the fixtures — strip the rows, concatenate with spaces removed, and require the result minus a trailing `…` to be a `strings.HasPrefix` of `sanitiseCommandText(command)`, driven over the adversarial fixtures already in the file. It catches silent interior loss on inputs a fixture cannot enumerate, but it is weaker on wrap points and it does not hold at width exactly 3 (see NOTES), so it would need the narrow-width table excluded. The reviewer recommends the exact fixtures as the primary assertion — the output is deterministic, which is the case code-quality reserves exact comparison for — with the property assertion added only if the adversarial inputs are wanted for content too. The orchestrator agrees: take the exact fixtures, and add the property assertion beside them only if it costs little.
  CONFIDENCE: high

NOTES (non-blocking, no action needed):
- Everything else was verified independently and passes: the pinned-width property holds for every row at every width 1-80 across ~35 adversarial commands (leading/trailing/interior whitespace, tab, ESC, CJK, combining marks, emoji incl. ZWJ and flag pairs, variation selectors, RTL, NBSP, zero-width space, soft hyphen, unbroken 52/53/104/156/500-char tokens, CRLF, OSC sequences, invalid UTF-8, lone surrogates, BOM, C1 code points, NUL); never more than 3 rows, never a newline inside a row, never a control character in the stripped output. The reviewer teeth-checked its own probe.
- Both `ansi.Wrap` defects the code documents are real and both halves of the handling are load-bearing, measured: the trim catches the break-whitespace case at many widths, and over-wide lines survive the trim only at widths 1-3, which is exactly what the clamp catches. Neither is dead defensive code.
- Content fidelity holds: across the corpus at every width 1-60 the concatenated visible text is a contiguous prefix of the sanitised command and is `…`-marked whenever anything was dropped. The only exception is width exactly 3, where the clamp stitches non-contiguous fragments (`"claude --resume abc"` renders as `cla | ude | es…`). The row is still `…`-marked and a three-column pane shows nothing legible under any scheme — recorded so the degraded-stack task is not surprised by it.
- Leading whitespace on the command's first line is trimmed, so a stored `"  claude …"` renders flush rather than indented. Whitespace only. Worth knowing if the visual gate compares against a hand-crafted fixture with a deliberate indent.
- C1 code points (U+0080-U+009F, including U+009B) and invalid UTF-8 pass the sanitiser unchanged, per the task's stated rule. Geometry holds for both; a UTF-8 terminal decodes U+009B as a character rather than CSI, so the ESC rule is the load-bearing one.
- Every comment in the new file was checked against the code and against measurement; all true as written. No comment corrections.
- All twelve prescribed test names are present, plus the two the fix round added and one extra report control-character case.

## Attempt 3

ISSUES:
- `internal/tui/resume_panel_parts_test.go:391-398` — "it neutralises control characters in a report" cannot fail for either control character it uses, so the report's sanitisation is unguarded. Measured cause: `ansi.Strip` removes `\x1b[31m` before `assertNoControlCharacters` (line 51) looks at the text, and lipgloss's `Render` expands `\t` into spaces (`lipgloss.NewStyle().Render("frozen\x1b[31m\tstill")` -> `"frozen\x1b[31m    still"`, width 15), so the width assertion passes too. Proof: deleting `sanitiseCommandText` from `resume_panel_parts.go:81` leaves the whole package suite green while the raw ESC is emitted into the row — the repaint hazard the task's Problem statement exists to close, on a string Phase 4/5 will compose from store and tmux failures.
  FIX: give the report the same treatment the command path already has and which does bite — a table over `\n`, `\t`, `\x1b[31m`, `\x07` comparing `resumeReportRow(prefix+tc.control+suffix, …)` with `resumeReportRow(prefix+tc.spaced+suffix, …)` for equality, and assert the raw row (not `ansi.Strip`ped) does not contain `"\x1b[31m"`.
  ALTERNATIVE: keep the single fixture and only add the raw-row `!strings.Contains(row, "\x1b[31m")` assertion — smaller, and it kills this mutation, but it leaves `\x07`/`\x7f` unpinned. The reviewer recommends the table; it costs three lines more and matches the command test's shape. The orchestrator agrees.
  CONFIDENCE: high

- `internal/tui/resume_panel_parts.go:34` / test file — the "at most three lines" boundary is unpinned: flipping `len(lines) <= resumeCommandMaxLines` to `<` survives the whole package suite. Under that mutation a command wrapping to exactly three lines has its third line replaced by an `…`-truncated join, so the user reads a command marked as cut that was not. This is also AC1's second named shape ("a command that wraps to exactly three lines"), which no current test exercises — the length table at test:254-264 runs 1/40/52/53/500 and jumps straight from one line to the truncated case.
  FIX: add a subtest with a command that wraps to exactly three rows at the pinned width, asserting exactly 3 rows, all at `resumeCardContentWidth`, the third *not* ending in `…`, and the row text. A measured fixture: `"claude --resume 4f2c9a1e-7b33-4d01 --cwd /Users/leeovery/Code/portal/internal/tui --output stream"` renders as `"claude --resume 4f2c9a1e-7b33-4d01 --cwd"` / `"/Users/leeovery/Code/portal/internal/tui --output"` / `"stream"`.
  CONFIDENCE: high

- `internal/tui/resume_panel_parts_test.go:215-233` — "it holds the width when the command opens on whitespace" asserts widths only, and the clamp keeps widths correct on its own, so the `strings.Trim` at `resume_panel_parts.go:50` (the fix the first round added for exactly this input) is unguarded: removing the trim entirely — or reducing it to `TrimLeft` or `TrimRight` — leaves the whole package suite green. Measured consequence of the no-trim mutation at width 52 for `" " + strings.Repeat("a", 60)`: row0 becomes `" " + 51x"a"` instead of `52x"a"` — the row is indented by a stray space and one character of the command is dropped, not carried to row1.
  FIX: extend that subtest to pin text as the neighbouring subtests do — collect `rowText(row)` and compare with `reflect.DeepEqual` against the expected rows (for the space case at the pinned width: `[]string{strings.Repeat("a", 52), strings.Repeat("a", 8)}`), keeping the existing `assertRowsAtWidth` call.
  CONFIDENCE: high

NOTES:
- Everything else the reviewer tried was killed by the suite: `resumeCommandMaxLines` 3->4, sanitising only `\n`, dropping the pad, ignoring `bold`, report token -> `text.primary`, dropping the report ellipsis, `ok=true` for an empty report, clamping one cell short, and making the clamp a no-op. The two subtests the last round added do bite.
- The production file was verified byte-identical before and after the reviewer's mutation testing, and no scratch probe file survives.
- A production behaviour worth a decision, deliberately NOT raised as an issue because no acceptance criterion requires it: `ansi.Wrap` over-packs at hyphen runs as well as at whitespace, so it returns over-wide lines at ordinary widths, and `clampToWidth` then drops that overshoot permanently — the characters are not carried to the next line and no `…` marks the loss. Measured over 50,000 synthetic commands built from realistic vocabulary (flags, hyphenated names, a UUID, a path): 0.67% of commands lose content at width 52 and 1.97% at width 24, up to 6 cells. Worked example at 24: `"resume run a-b-c-d-e-f -- --port=3000 --resume"` renders as `"resume run a-b-c-d-e-f -"` / `"--port=3000 --resume"` — the standalone `--` argument is shown as a stray `-`. The geometry invariant is never violated; what is lost is fidelity of the one string on the panel that says which work the pane holds. The remedy (re-flow the overshoot into the following line instead of truncating it) changes the exact narrow-width row text the suite now pins at widths 1-3, so it is a scope call rather than something to fold into a fix round.
- The comment at `resume_panel_parts.go:42-46` names whitespace-carry and "below about four columns" as the reasons `ansi.Wrap` over-runs. Both claims are true as written, so there is no correction to apply, but the hyphen case above is a third cause that happens at every width — a reader could conclude the clamp only matters on tiny panes.

## Attempt 4

ISSUES:
- `internal/tui/resume_panel_parts.go:50` — the right-hand half of `strings.Trim(line, " ")` in `wrappedLines` is load-bearing and no test holds it. Changing it to `strings.TrimLeft` (a plausible future simplification — "the padding covers the trailing side anyway") leaves the whole suite green while silently rendering a doubled space and one fewer character of the user's command on row three, on ~0.05% of realistic commands at both the card width and a degraded pane's. The command is the only thing on the panel that says which work the pane is holding, so a character quietly removed from it is the failure this task exists to prevent, and nothing would report it.
  **This disproves the executor's second equivalence claim.** The reported proof (1920 renders, zero diff) was under-powered — it only exercised the suite's own fixtures. Over a ~322,000-comparison differential, comparing right-trimmed `ansi.Strip`ped rows so padding-only differences are excluded by construction, the mutant diverges in 1,040 cases; restricted to realistic single-spaced commands, 3 divergences in 6,000 renders at widths 24 and 52, all on row three. The mechanism the claim missed is not the styled-segment boundary — it is that a retained trailing space feeds `strings.Join(lines[2:], " ")`, producing a doubled space that costs one character of command text before the `…`. Two verified cases:
  - width 52, `"--permission-mode run /Users/leeovery/Code/portal/internal/tui 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 --permission-mode stream-json"` -> row 3 is `"7b33-4d01-9f6a-2c8e510db4a7 --permission-mode strea…"`; the mutant renders `"7b33-4d01-9f6a-2c8e510db4a7 --permission-mode  stre…"`.
  - width 24, `"/Users/leeovery/Code/portal/internal/tui npm src/main.go npm 4f2c9a1e-7b33-4d01"` -> row 3 is `"src/main.go npm 4f2c9a1…"`; the mutant renders `"src/main.go npm  4f2c9a…"`.
  `strings.Trim` is the correct production choice; what is missing is any fixture that holds it there. Distinct from the banked hyphen over-pack: the remedy here is a test, the code is right, and no character is lost by the code as it stands.
  FIX: add one pinned-row subtest beside `"it marks the third row with … when the command runs past it"` — for example `"it keeps the wrap's trailing space out of the joined third row"` — using the width-52 fixture above and asserting through the existing `assertRowsRead` on exactly `{"--permission-mode run", "/Users/leeovery/Code/portal/internal/tui 4f2c9a1e-", "7b33-4d01-9f6a-2c8e510db4a7 --permission-mode strea…"}` (verified against the code as it stands; the `TrimLeft` mutant fails it). The width-24 fixture works the same way if a second case is wanted.
  ALTERNATIVE: assert the invariant instead of a fixture — a subtest over the command table already in the file checking that no line `resumeCommandLines` returns begins or ends with a space at any width 1-80. More robust (it keeps its teeth if the wrapping library's packing behaviour changes, which the banked over-pack decision may cause) but it asserts a property rather than pinning text, which cuts against the rule this round adopted. The reviewer recommends the pinned fixture as primary and would not object to both. The orchestrator agrees: take the fixture, and add the property beside it only if it is cheap.
  CONFIDENCE: high

- `internal/tui/resume_panel_parts.go:23` — the sanitiser's `r == 0x7f` clause and the exact `r < 0x20` bound are both unexercised. Mutating `if r < 0x20 || r == 0x7f` to `if r < 0x20` (drops DEL) and to `if r < 0x1f || r == 0x7f` (lets US through) — both survive the entire suite. This matters because `lipgloss.Width("\x7f")` and `lipgloss.Width("\x1f")` are both 0: a row carrying one measures as exactly the pinned width while the terminal paints it a cell short, so `assertRowsAtWidth` cannot see it and only `assertNoControlCharacters` can — and no fixture feeds it those bytes. A command reaches this renderer from `hooks.json`, where a hand edit or a paste can put any byte, and the card's width moving with its content is precisely the Outcome the task states. The task's `Do` names `0x7f` explicitly.
  FIX: add two rows to the existing control-character table in `"it neutralises control characters without changing the geometry"` — `{"delete", "\x7f", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}}` and `{"unit separator", "\x1f", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}}` — and the same two to the report-row table in `"it neutralises control characters in a report"`. Both existing tables already run the space-substituted comparison and `assertNoControlCharacters`, so no new helper is needed.
  CONFIDENCE: high

NOTES:
- **The executor's first equivalence claim STANDS**, confirmed independently rather than from the reported dump: the `<=`->`<` mutation on the three-line boundary produced zero differences over ~322,000 comparisons (~4,020 commands x widths 1-80). The mechanism is as argued. `clampToWidth`'s own `<=`->`<` mutation is likewise equivalent (0 differences over the same corpus).
- The sweep is genuinely complete against its own stated rule: every subtest rendering a wrapped or truncated row pins exact text through `assertRowsRead`. The three declared exceptions are defensible, and the reviewer teeth-checked the property subtest in isolation (the skip-a-line mutation fails it on its own).
- Nothing in the task's Tests list or Acceptance Criteria fell out of the rewrite. All twelve prescribed names are present verbatim; the merged narrow table and the folded ellipsis assertions both strengthen coverage (both `…`-dropping mutations are caught).
- Of the reviewer's 20 mutations, 16 were caught. Two survivors beyond the two issues above are genuinely equivalent and should not be chased: `lines[:2:2]` -> `lines[:2]` (capacity pin is defensive; returned content identical) and `clampToWidth`'s `<=` -> `<`.
- The comment at `resume_panel_parts.go:30-31` sits above `resumeCommandLines`, whose body no longer calls `ansi.Wrap` — the call moved into `wrappedLines` at line 48. The claim is true and plainly explains the function's wrapping choice, so no correction is applied; raised only because a reader checking it against the line beside it will not find the call there.
- `assertRowsCarryCommand` passes vacuously if the rows are empty (`strings.HasPrefix(want, "")` is true). Constrained in practice by its one caller's row-count and width assertions; one line would close it if the helper is ever reused.
- `resumeCommandRows`'s six parameters including two booleans is prescribed verbatim by the task's Do, so it is not the executor's decision and is not a finding — worth knowing when Phases 4 and 5 call it.
- The banked `ansi.Wrap` hyphen over-pack was deliberately not raised, per the dispatch. This round's trim finding is a separate failure with a separate remedy and does not depend on that decision going either way.

## Attempt 5

ISSUES:
- `internal/tui/resume_panel_parts_test.go:483-509` — `resumeReportRow` has no guard below width 24, while its sibling `resumeCommandRows` is pinned at widths 1, 2, 3 and 4 ("it carries what it can when the pane is too narrow to wrap into", line 298). The degraded stack passes the pane's width to **both** helpers, so the same width band is reachable for both. Demonstrated, not inferred: the mutant `ansi.Truncate(sanitiseCommandText(report), max(width, 5), resumeEllipsis)` at `internal/tui/resume_panel_parts.go:81` **survives the whole suite green** — it returns rows up to 5 cells wide at widths 1-4, breaching the invariant AC2 names ("so neither ever hands the canvas a row it has to cut"). No defect exists today (verified over 240,000 report renders at widths 1-60 with CJK, combining marks and control bytes), so this is purely the guard's reach — but Phase 4 and Phase 5 compose the report text and will edit this helper, and a prefix glyph or a changed truncation is exactly the change that lands in that band with nothing to fail.
  FIX: Add a width dimension to the existing report table at `resume_panel_parts_test.go:485-509`, or a sibling subtest mirroring the command block's narrow-width one. Measured expected values, for `assertRowsAtWidth` + `assertRowsRead`: for `"the freeze will not lift"` — w=1 `"…"`, w=2 `"t…"`, w=3 `"th…"`, w=4 `"the…"`; for `"再開するセッション"` — w=1 `"…"`, w=2 `"…"`, w=3 `"再…"`, w=4 `"再…"` (the CJK rows at 2 and 4 read short because `rowText` trims the pad; the `lipgloss.Width` assertion is what holds the invariant there, which is the point).
  ALTERNATIVE: Mirror the command path in production by routing the report text through `clampToWidth` after the truncate. That makes the invariant structural rather than tested, but `ansi.Truncate` — unlike `ansi.Wrap` — does not over-run today, so the clamp would be unexercised defensive code and the test would still be the thing that catches a future regression. The reviewer recommends the test rows; the orchestrator agrees.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `internal/tui/resume_panel_parts.go:30-31` — the Wrap-versus-Wordwrap rationale heads `resumeCommandLines`, which calls neither function; the choice it warns about is made in `wrappedLines` eleven lines below, so an editor of `wrappedLines` never sees it.
  OLD:
  // ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
  // broken at it rather than left to overflow the card.
  NEW: (empty — delete these two lines; the rationale moves to the correction below)
- `internal/tui/resume_panel_parts.go:42-46` — fold the moved rationale onto the function that makes the choice.
  OLD:
  // Every line comes back no wider than the width asked for, which ansi.Wrap on
  // its own does not promise: it carries the whitespace it broke at into the line
  // it broke and does not count a line's leading whitespace toward the limit, and
  // below about four columns it leaves lines over-wide outright. A row a cell too
  // wide cannot be padded back down, and widens the card built around it.
  NEW:
  // ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
  // broken at it rather than left to overflow the card. Every line comes back no
  // wider than the width asked for, which ansi.Wrap on its own does not promise:
  // it carries the whitespace it broke at into the line it broke and does not
  // count a line's leading whitespace toward the limit, and below about four
  // columns it leaves lines over-wide outright. A row a cell too wide cannot be
  // padded back down, and widens the card built around it.

NOTES:
- **All nine acceptance criteria met and nothing in the Tests list unpinned** — the reviewer walked each individually against the code and confirmed all twelve prescribed subtest names present verbatim with their tables.
- **The two guards the last round added bite, and for the right reason.** `strings.Trim`->`TrimLeft` is caught by the new trailing-space subtest **and by nothing else in the suite**, on both table rows independently, and the failure it reports is the substantive one (the doubled space at the join, with a character of command lost to it) rather than merely a width. `TrimRight` and dropping the trim are caught by a different subtest, so the two directions have distinct guards. The `\x7f` and `\x1f` rows each catch their own narrowing and are the sole catchers, in both tables.
- **35-mutant battery, the reviewer's own list, 34 died.** The three survivors beyond the issue above are genuine equivalent mutants, not taken on reasoning: shadow implementations were built and run over a 1,920,000-comparison differential (after verifying the shadow baseline reproduces production exactly over 240,000 renders). `clampToWidth`'s `<=`->`<`, the three-index slice cap, and `resumeCommandLines`'s `<=`->`<` all produced zero divergences.
- **The declined property assertion: the executor's argument holds and the attribution is right**, re-derived independently. Over 40,000 renders at widths 52 and 24 with a realistic corpus, 337 rows violate "no line begins or ends with a space" — and every one is produced by a wrap line the library returned wider than asked for, none by the trim, none by the clamp. The clamp never once returned an over-wide line over a 320,000-render adversarial fuzz. The executor's "two on degenerate widths 1-2" understates the reach (leading-space third rows up to width 14, trailing-space rows at 52), but the root cause is the same in every traced case, so the conclusion is unchanged: declining it was right.
- The sanitiser is exactly what the task prescribes. The residual is the C1 block (U+0080-U+009F pass through and measure as zero-width), reachable only by a hand edit of `hooks.json` with a literal C1 byte — a copy-pasted escape sequence carries `0x1b`, which is neutralised. An observation about the prescribed rule, not a defect in the implementation of it.

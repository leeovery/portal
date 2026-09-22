## Attempt 1

ISSUES:
- `internal/tui/resume_panel_parts.go:52` — `rowAndCarry(carry+line, width)` concatenates the carry onto the next wrapped line with no separator, on the premise stated at `:43-44` and `:63-65` that `ansi.Wrap` "carries the whitespace it broke at into the line it broke". Measured against `ansi v0.11.7`, that is true only when the line fits; **in the over-pack case — the one case this whole function exists for — `ansi.Wrap` consumes the break whitespace entirely**. `ansi.Wrap("/Users/leeovery/Code/portal --- 東", 27, "")` returns `["/Users/leeovery/Code/portal ---", "東"]`, with no space anywhere. The carry then glues the two sides together.

  The failure, on the card's own fixed width with a canonical command:
  ```
  command          claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 -- x
  pre-change @52   ["claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7", "x"]     (-- dropped: the old defect)
  shipped    @52   ["claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7", "--x"]   (-- glued: a flag that does not exist)
  ```
  This is precisely the outcome your own Decision 1 rejects in your report ("the `--` present but glued into a command that does not exist"), applied to the *other* way the separator can vanish. The user at a waiting pane approves or discards on this one line — the spec says it is the only thing on the panel that identifies the work — and `--x` reads as a flag they never wrote, or passes unnoticed and they discard the wrong pane. It needs no resize: width 52 is the card's own `resumeCardContentWidth`. Also reproduces at widths 13-14, 18-20, 27-29, 34-35, 52-54 on ordinary ASCII single-spaced commands (`npm run build --- watch` -> `["npm run build", "---watch"]`; `claude --resume abc -- x` @19 -> `["claude --resume abc", "--x"]`).

  Rate, measured by walking each rendered row against its source (a space in a row must sit where the source has one; a row boundary may swallow a source space run): **6,011 of 241,600** random-corpus renders glue, against **334** for the pre-change implementation — so the class is introduced by this change, not inherited. On the same measurement the task's real win is confirmed: dropped/mismatched text falls from 8,348 to 4.

  FIX: give the carry back the whitespace `ansi.Wrap` dropped, by consuming the source alongside the wrapped lines in `wrappedLines` (`internal/tui/resume_panel_parts.go:47-60`):
  ```go
  wrapped := strings.Split(ansi.Wrap(text, width, ""), "\n")
  lines := make([]string, 0, len(wrapped))
  rest := text
  row, carry := "", ""
  for _, line := range wrapped {
      rest = strings.TrimPrefix(rest, line)
      gap := rest[:len(rest)-len(strings.TrimLeft(rest, " "))]
      rest = rest[len(gap):]
      row, carry = rowAndCarry(carry+line+gap, width)
      lines = append(lines, row)
  }
  ```
  `rowAndCarry` already trims a trailing gap off a row that fits, so appending it is inert except where it separates a carry. The reviewer applied exactly this via `-overlay` and measured: gluing 6,011 -> 0; the suite-wide fidelity property still holds; **not one pinned expectation moves** (widths 1-4 tables for both commands, the two new hyphen pins at 12 and 6, the card-width pins, the double-width and combining-mark pins, the `clampStackToPane` pin), and the whole `internal/tui` package is green. `internal/tui/resume_panel_parts.go` is the only production file touched.

  Two comments must move with it, since both state the false premise: `:43-44` ("and carries the whitespace it broke at into the line it broke" — it does so only when the line fits) and `:63-65` ("because ansi.Wrap broke at one of them" — in the over-pack case there is no trailing space to break at). The shortest honest replacement for `:63-65` is to say the row is split before the trim so that a break space `ansi.Wrap` *did* leave stays with the carry, and to let the new `gap` line carry the case where it left none.

  Test: add the discriminating case to the table at `internal/tui/resume_panel_parts_test.go:675` —
  `{"a separator the wrap's break lands on", "claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 -- x", resumeCardContentWidth},`
  It fails today on the existing `assertLinesReconstruct` ("the lines […, \"--x\"] do not reconstruct …") and passes with the fix. Worth adding the same command to `resumePartsCommands` (`:605`) so the widths 1-80 properties cover it; the reviewer verified the corpus addition fails 18 renders as shipped and 0 with the fix.

  ALTERNATIVE: drive the re-flow from the source text directly and use `ansi.Wrap` only to find break points, rather than trusting its line list to be lossless. Cleaner in principle, but it re-authors the loop and risks moving pins; the `gap` recovery above is a three-line change that provably moves none. The reviewer recommends the `gap` recovery, and so does the orchestrator. (A third option — always inserting a space when both sides are non-space — is wrong: it would split a legitimately mid-token break, e.g. `4f2c9a1e-` / `7b33-4d01`.)
  CONFIDENCE: high

NOTES:
- **Decision 1 was right, and you should know it plainly.** The reviewer rendered both variants. The literal trim-then-split reading gives `{"a-b-c-d-e-f", "----port=300", "0--resume"}` at width 12 — the `--` merged into an argument that does not exist. Your split-untrimmed variant gives `{"a-b-c-d-e-f", "-- --port=30", "00 --resume"}`. Only yours serves the criterion's point. The deviation from the literal Do list was correct; the issue above is that the same principle was applied to one of the two ways the separator can be lost, not both.
- **Decision 2 verified on both halves.** Without the third-row trim, 184 edge-space violations survive across the reviewer's corpus, so criterion 3 is genuinely unreachable by changing `wrappedLines` alone — your claim holds. And the other side is genuinely wrong: trimming `beyond` first also reaches zero violations but renders `{"", "", ""}` at width 1, breaking the loss marker. You picked the correct side for the correct reason.
- **The single moved pin is measured and correct, and nothing else was relaxed.** The test diff contains exactly one modified expectation line; everything else is additions plus a gofmt realignment. No assertion was deleted, loosened or converted to a substring check.
- **The lone-wide-grapheme case is genuinely pre-existing.** Across 241,600 renders there are 89 unmarked-loss renders, every one at width 1, and zero of them new — the reviewer ran the pre-change implementation over the identical corpus and each was already there. Criteria 4 and 5 are in tension exactly on that class and 5 governs it. Your characterisation was accurate.
- **On your mutation set.** M1-M6 are credible, but every mutant was aimed at width, edge-space, row-cap or the pinned rows. The property none of them probes is the one the task's Outcome names — that the rendered text *is* the source text — which is why the glue survived a 720k-render sweep. Your own `assertLinesReconstruct` is the right instrument; it was simply pointed at four hand-picked cases rather than at the corpus.

## Attempt 2

ISSUES:
- `internal/tui/resume_panel_parts.go:89` — the `ansi.StringWidth(fits) <= width` boundary in `rowAndCarry` is unexercised for a command that ends in whitespace, and the one-character mutation `<=` -> `<` survives the **entire package suite** while producing exactly the failure this task exists to prevent. Under the mutant, `resumeCommandLines("claude --resume abc ", 6)` renders `["claude","--resu","me ab…"]` — the final `c` gone and an `…` claiming a cut that did not happen — and `resumeCommandLines("npm run dev ", 3)` renders `["npm","run","de…"]`. Measured: the mutant differs on 18,391 renders, all of them commands ending in a space, losing text in 1,612. **The shipped code is correct for these inputs; the defect is that nothing holds it there.** The cause is a corpus asymmetry: all 19 corpus entries and all 5 reconstruct cases were chosen with leading whitespace in mind ("opening on a space") and none ends in whitespace, so neither the fidelity property (an extra trailing empty row is free in a prefix walk) nor the width/edge-space/max-line invariants can see it. The failure this prevents: a later edit to that width comparison ships green while silently truncating the last character of any registered `on-resume` command that ends in a space — a paste artefact, or a trailing control character `sanitiseCommandText` folded into one — and marking it with an `…` that lies about the cut, on the one screen that says which piece of work the pane holds.
  FIX: add a trailing-space case to the `"it carries every character of a command that fits three rows"` table at `internal/tui/resume_panel_parts_test.go:685`, e.g. `{"a command ending on a space", "claude --resume abc ", 6},`, and change `assertLinesReconstruct`'s comparison at line 720 from `if b.String() == command {` to `if b.String() == strings.TrimRight(command, " ") {` — a command's own trailing whitespace is unrecoverable by design, since no row may end on a space (criterion 3), so right-trimming the target is the honest form of the rule the helper's comment already states. The reviewer verified this exact pair: it **passes on the shipped code and fails on the mutant** (`the lines ["claude" "--resu" "me ab…"] do not reconstruct "claude --resume abc "`). It is a no-op for the five existing cases, none of which carries an edge space.
  ALTERNATIVE: add a separate invariant over the corpus — "no row the text does not put there", i.e. an empty row is legal only where the next grapheme is wider than the width — and add a trailing-space command to `resumePartsCommands`. Stronger (it also closes the inserted-empty-row tolerance in the fidelity property) but it is a new property rather than a row in a table the task already asked for, and it needs its own measurement. The reviewer recommends the first; the orchestrator agrees.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `internal/tui/resume_panel_parts_test.go:19` — the direction is measurably wrong: at no width from 1 to 80 does the wrap break at the space *before* the standalone `--`. At the width this fixture is actually driven at (52) the wrap over-packs `" --"` onto the first line and breaks at the space *after* it, consuming that space. (The line begins with one tab.)
  OLD: 	// The wrap breaks at the space before the standalone --, and consumes it.
  NEW: 	// The wrap consumes the space after the standalone --, leaving it on neither line.
- `internal/tui/resume_panel_parts_test.go:606-607` — false provenance: 7 of the 19 entries appear nowhere else in the resume suites — "a separator then a hyphen run", "a pipeline", "double width word", "carrying an escape sequence", "opening on a space", "double width" and "combining marks". Two of those seven are the *sole* detectors of the two mutants the last round killed, so a contributor taking the comment at its word and pruning entries that "the rest of the suite" does not render would delete exactly the two that carry the property's discrimination.
  OLD: // The commands the rest of the suite renders, gathered for the invariants that
  // hold over all of them at every width.
  NEW: // The command shapes the invariants below are held over.

NOTES:
- **Your `droppedGap` correction was right, and verified independently in both directions.** `ansi.Wrap` does retain leading spaces on a following line (`ansi.Wrap("tail -f /x/y | grep resume", 2, "")` -> `["ta","il -f"," /x","/y",…]`). The plain three-line recovery measures 183 infidelities over 320,000 renders against 0 for your shipped code, on an independently written judge; minimal case `"-f -f a   a"` at width 2 glues the two `a`s. The subtraction is right in both directions — 0 glued, 0 doubled, 0 dropped, 0 reordered, 0 short across ~1.52M renders. The `>=` in your guard is exactly right, and `kept > len(run)` is a provably equivalent mutant (0 differing renders in 1.2M).
- **Both exceptions in the fidelity property are genuinely sanctioned**, not convenient — one by criterion 3 and the task's own trim bullet, the other by the bound bullet and criterion 5. The property is not permissive on glued or inserted text: it alone kills the plain recovery, the gap-not-advanced mutant and the dropped drain loop. It does tolerate an inserted empty row, but `assertLinesReconstruct` backstops that.
- **Both corpus additions genuinely discriminate**, each the sole detector of its mutant: the pipeline entry catches F8 at exactly one width (2), and the separator-then-hyphen-run entry catches F3 at 8 widths from 5. Removing either lets its mutant survive the whole suite. The margin is thin enough that reordering or trimming the corpus would silently lose it — which is what the second comment correction is about.
- All seven acceptance criteria met, each re-measured from the code. The single moved pin was re-measured, not relaxed; the only other removed lines in the diff are the old implementation, the stale comment sentences and a gofmt realignment. No assertion was removed or weakened anywhere.
- **A second `ansi.Wrap` pathology, pre-existing and unchanged by this task** — the reviewer checked because the fidelity property tolerates it. With leading whitespace the wrap sometimes emits a run of blank lines before the text (`ansi.Wrap(" 4f2c9a1e-7b33-4d01", 9, "")` -> `[" ", "", x7, "4f2c…"]`), so almost nothing of a command that would fit is rendered. The pre-change code renders the same, so it is neither introduced nor worsened, and its reach is small: the widest pane producing any blank line is 6, and at widths >= 4 there are zero renders where the command is lost entirely. Below the floor, outside this task's stated problem, but worth knowing if the wrap is revisited — skipping a blank *wrapped* line would close it without disturbing criterion 5, whose empty rows come from the grapheme drop rather than from a blank wrapped line.

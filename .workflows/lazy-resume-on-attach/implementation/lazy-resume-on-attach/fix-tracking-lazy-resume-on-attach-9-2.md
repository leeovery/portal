## Attempt 1

ISSUES:
- `/Users/leeovery/Code/portal/internal/tui/text_wrap.go:68-72, 89-95, 100-110, 116`: after the first row break that lands on non-ASCII whitespace, the capped row glues words together.
  - `ansi.Wrap` breaks at and consumes every `unicode.IsSpace` rune except U+00A0. That includes U+2009, U+202F (which macOS puts in screenshot filenames), U+3000 and U+2000–U+200A.
  - `droppedGap` and the walk's trims only recognise `" "`. When a break lands on one of those characters, `strings.TrimPrefix(rest, line)` stops matching. From then on every boundary records an empty `gap`, including ordinary ASCII-space boundaries later in the command.
  - The capped row then shows tokens the command does not contain. `resumeCommandLines("cd '/Users/lee/My Projects/app' && claude --resume 4f2c9a1e --cwd /x", 17)` now returns the capped row `"claude --resume4…"`; before this change it was `"claude --resume …"`. At width 15 a longer variant reads `"Projects/app'&…"`.
  - This is a regression against the previous code, which always put a space between rows. The user sees the same kind of misreading this task exists to prevent.
  - Across 20k random inputs that include U+2009/U+3000, the capped-row invariant fails repeatedly on the executor's code, for example `"x-y/p/qb…"` where the source has `"x-y /p/qb…"`.
  FIX:
  - In `text_wrap.go`, declare one predicate for the whitespace `ansi.Wrap` breaks at: `func breaksWrap(r rune) bool { return unicode.IsSpace(r) && r != ' ' }` (U+00A0 is the one space the wrap keeps inside a word).
  - Route every trim in the walk through it: `droppedGap`'s `run` and `kept` (lines 101 and 105), `rowWalk.take`'s `TrimLeft`/`TrimRight` (lines 90 and 92), `rowAndCarry`'s fit check (line 116) and `wrapCapped`'s `TrimLeft` (line 24). `rest` then stays in step, and each boundary records the source's own whitespace character.
  - In `resume_panel_parts_test.go`, add a command with non-ASCII whitespace at a break to `resumePartsCommands`, for example `"cd '/Users/lee/My Projects/app' && claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7　--cwd /Users/leeovery/Code/portal/internal/tui --output-format stream-json"`.
  - Route the test oracle's trims in `restPastRows`, `assertRowsReadSource` and `assertCappedRowReadsSource` through the same predicate.
  - Pin `resumeCommandLines("cd '/Users/lee/My Projects/app' && claude --resume 4f2c9a1e --cwd /x", 17)` == `{"cd '/Users/lee/My", "Projects/app' &&", "claude --resume …"}`.
  - I prototyped this in a scratch copy. The full `internal/tui` suite passes and every existing pin reads as it stands; the random check passes, and for inputs without non-ASCII whitespace the rows match the previous code. Rows change only for input that contains non-ASCII whitespace: an edge character like that is now trimmed the way an ASCII space is.
  ALTERNATIVE: keep the ASCII-only trims and resynchronise `rest` when `TrimPrefix` misses: skip to `strings.Index(rest, line)` and record the skipped run as that boundary's gap without appending it to the line. Rows stay identical for every input. The cost is a second path through the walk that the re-flow and the dropped-grapheme handling must stay consistent with. That is more code for a narrower guarantee, so I recommend the predicate.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- `/Users/leeovery/Code/portal/internal/tui/text_wrap.go:9-11`: the added clause only repeats what the loop does and what the `wrappedRow.gap` field comment already says.
  OLD: // The rows past the cap are re-joined onto the last kept one, each behind the
// whitespace the source had before it, and truncated there, so the ellipsis
// marks the cut rather than the rows leaving without one.
  NEW: // The rows past the cap are re-joined onto the last kept one and truncated
// there, so the ellipsis marks the cut rather than the rows leaving without one.

NOTES:
- I checked that rows are unchanged with 20k random inputs made of ASCII, hyphen runs, repeated spaces, double-width and combining characters, at widths 1–30 and caps 1–3. `wrappedLines` output matched the previous implementation byte for byte, `wrapCapped` matched whenever the text fits within the cap, and every capped row read as the source.
- Width 0 would make `cutRow` return `"…"`, which is wider than the width. It is not reachable: `dimsOrFallback` floors the pane width, and `themePanelMessageText` handles width 0 itself before it wraps.
- The existing doc comment above `wrappedRows` (lines 61-63), which this diff did not touch, still says "The gap" meaning `droppedGap`'s local variable. The new `wrappedRow.gap` field is a different quantity with the same name. That is ambiguous, not false.
- Nothing arrived with the dispatch beyond the enumerated inputs.
- All experiments ran in scratch copies under the session scratchpad; the working tree is untouched.

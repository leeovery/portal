TASK: Portal Hook List Shows the Mode in a Fifth Column (lazy-resume-on-attach-1-7)

ACCEPTANCE CRITERIA:
- Each row is five tab-separated fields ending in a newline, and the first four are byte-identical to today's output for the same store and the same live panes.
- An object-form registration carrying `eager` or `lazy` renders that word in the fifth field; a string-form registration renders an empty fifth field.
- An object-form registration whose stored `resume` is unrecognised, empty or absent renders an empty fifth field.
- The listing performs exactly one `ListAllPaneHookKeys` call when it has entries, and none when it has none — no second tmux read is added for the mode.
- Output holds one line per registration and nothing else: no header, no footer, no install-wide default.
- The existing key-then-event sort, the empty-output cases (no entries, absent file), and the empty location cell for a token no live pane carries are all unchanged.

STATUS: complete

SPEC CONTEXT: Spec §3.4 makes a pinned registration readable from `portal hook list` as an appended fifth column (eager/lazy, empty when the registration carries none, meaning it follows the install), leaving the first four columns untouched for positional parsers; the install-wide default is deliberately kept out of the listing (reported by `portal doctor` in a later phase). §3.2 fixes the decode rule the column reflects: an object whose `resume` is absent, empty, or anything other than `eager`/`lazy` carries no mode and "the mode column (§3.4) reads empty for it"; the out-of-repo consumer filters on the event column, which is unchanged across both stored shapes.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks.go:154 (row print extended to five fields, fifth = `h.Resume.String()`); cmd/hooks.go:146-151 (unchanged early return with no entries, single `paneLocationsByToken` call); internal/hooks/store.go:255-281 (`Store.List` already carries `Resume` from the same load); internal/hooks/registration.go:31-48 (decode leaves `Resume` unset for unrecognised/empty/non-string/absent `resume`); internal/resumemode/resumemode.go:36-45 (`String()` renders "" for `Unset`); README.md:227; CLAUDE.md Resume-hook command paragraph.
- Notes: The mode is taken from the `store.List` result that produced the row, so no tmux read is added; the first four fields are formatted exactly as before. No header/footer is printed. README and CLAUDE.md edits match the task's prescribed wording and are accurate against the code. No other in-repo consumer of `hook list` output asserts the four-column shape.

TESTS:
- Status: Adequate
- Coverage: cmd/hooks_test.go:273 `TestHooksListModeColumn` covers eager and lazy object-form rows (full-line equality), string-form empty fifth cell, unrecognised/empty/capitalised/non-string/absent stored `resume` all rendering empty, a positional-parser check (five fields per line, first four equal to expected across mixed shapes including an empty location cell), exactly one `ListAllPaneHookKeys` call with three entries (cmd/hooks_test.go:366), zero reads with no entries via the loud lister (cmd/hooks_test.go:383), and one line per registration (cmd/hooks_test.go:393). Existing `TestHooksListCommand` / `TestHooksListLocationColumn` expectations were re-pinned to the five-field row, including the key-then-event sort case, empty-location cases and the absent-file / no-entries empty output. Each test would fail if the column were dropped, inserted before location, or rendered the unrecognised word.
- Notes: The new "it takes no tmux read at all with no entries" subtest (cmd/hooks_test.go:383) repeats what "produces empty output when no hooks registered" (cmd/hooks_test.go:41) already proves; the plan's test list asked for it, and it costs nothing beyond the duplication.

CODE QUALITY:
- Project conventions: Followed (seams injected via `withHooksDeps`; fixtures staged through `hookstest.StageStore` raw `Seed` as the task directs; no `t.Parallel()`)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`slices.Equal` in the positional-parser test)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

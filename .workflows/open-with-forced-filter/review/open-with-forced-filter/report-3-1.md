TASK: open-with-forced-filter-3-1 (tick-4c5a85) — Fit a session's recorded directory into a column width

ACCEPTANCE CRITERIA:
- A value that fits is returned home-abbreviated and otherwise byte-identical — `~/Code/portal`, never the absolute path tmux recorded
- The abbreviation is applied before the width is consulted, so it holds at every width and is never reached only as a rung of the shortening ladder
- A value too wide for the column is shortened from the left at a separator, so the surviving tail is whole segments prefixed by `…/` and never a mid-segment fragment
- The longest tail the width admits is the one returned
- A width that cannot hold the ellipsis plus the last whole segment returns the empty string
- A value carrying no separator at all is returned whole when it fits and empty when it does not
- A value whose last segment is empty (a stamped trailing separator) never yields a separators-only tail
- An empty `dir` returns the empty string, and a zero or negative width returns the empty string
- A directory equal to the home directory returns a bare tilde; a directory outside the home directory is returned unabbreviated
- Widths are measured as display width
- The helper reads no file, issues no tmux call and consults nothing beyond `AbbreviateHome`'s `$HOME` read
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §6.2 fixes the rendering rule the helper implements — the directory gives way before the name, is shortened **from the left** so the recognisable tail survives (`…/Code/portal`, not `/Users/leeovery/Cod…`), is always displayed home-abbreviated "at any width — the abbreviation is how the value is rendered rather than a rung of the truncation ladder", and below a floor (ellipsis plus one whole segment) is dropped rather than truncated to noise. §4.1 makes the home-abbreviated recorded directory the *matched* form as well as the displayed one ("the searched form is the displayed form"), which is what makes the abbreviate-before-width ordering load-bearing rather than cosmetic: a ladder implementation would show a raw path for a row matched on `~/…`. §6.2 also notes the rendering never narrows the search — the term is tested against the whole abbreviated value, so dropping the column entirely is a legitimate outcome.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/session_dir_column.go:13` (`const dirTruncationPrefix = "…"`), `:23-48` (`func fitSessionDir(dir string, width int) string`). Delivered change-set for this task is exactly `internal/tui/session_dir_column.go` + `internal/tui/session_dir_column_test.go` (commit dd0bf2072).
- Notes:
  - The guard at `:24` returns "" for an empty dir or a non-positive width, so the caller's arithmetic can go negative without a second guard at the call site.
  - `:28-31` abbreviates through `resolver.AbbreviateHome` unconditionally and only then consults the width — the ordering §4.1/§6.2 require. `resolver.AbbreviateHome` (`internal/resolver/path.go:94`) is pure string work over `os.UserHomeDir()` with no filesystem call, and degrades to the path as given when `$HOME` is unresolvable, so the "reads no file, no tmux call" criterion holds.
  - `:34-45` scans `/` positions left to right, so the first candidate that fits is the widest that fits (candidate width is monotonically non-increasing in the scan index) — the "longest tail" criterion is satisfied structurally rather than by a max-search. Every cut lands on a separator by construction, so no mid-segment fragment is reachable.
  - `:39` skips a candidate whose text after the prefix trims to nothing, which is what keeps a stamped trailing separator (`~/Code/`) from yielding `…/`. Because separators-only tails can only occur at the end of the value, skipping them cannot hide a wider qualifying candidate.
  - `:47` returns "" when the scan exhausts — the floor is derived from the shortest candidate the scan can produce rather than declared as a constant, exactly as the task's Edge Cases specify.
  - Measurement is `lipgloss.Width` throughout, matching the rest of the row (`internal/tui/session_item.go` uses the same measure), so the ellipsis counts as one cell and a wide-character segment is fitted by cells.
  - Wiring: the sole caller is the row renderer at `internal/tui/session_item.go:277`, added by task 3-2 as planned; this task added no call site of its own.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/session_dir_column_test.go` (`package tui`) carries all fourteen shapes the task enumerates. The table (`:10-119`) covers fits-whole at a generous width, the narrowest fitting width, left-shortening at a separator, two widths over one deep path asserting the tail grows with the width, the floor returning empty, the separator-free value whole and not-at-all, the separators-only-tail case, the bare tilde for `$HOME` itself, an unabbreviated `/opt/tools`, empty `dir`, and zero and negative widths. Three standalone tests cover the rest: `:121` sweeps every width from 1 to the value's full width and asserts each non-empty result is the prefix followed by a whole separator-delimited suffix of the value *and* within budget; `:147` pins display-width measurement with `/opt/日本語/tui` (15 cells across 18 bytes) both whole at 15 and shortened at 14; `:161` pins the unresolvable-`$HOME` degradation with `t.Setenv("HOME", "")`.
- Notes:
  - The case at `:26-31` ("it abbreviates even at a width the raw path would fit") is the one that actually falsifies a ladder implementation — every other home-bearing case runs at a width the raw `t.TempDir()`-rooted path could never fit, so without it the abbreviate-first criterion would be unpinned. It was added in the recorded fix round and is present.
  - Every home-relative case is anchored on `t.Setenv("HOME", t.TempDir())`, so no verdict depends on the developer's account name; the wide-character and no-home tests set `HOME` too rather than inheriting it.
  - No `t.Parallel()` anywhere, per the project rule (and required here by `t.Setenv`).
  - Not over-tested: the sweep and the table do different work (the sweep is a property assertion the enumerated cases cannot express), and no case is a restatement of another.
  - The doubled-separator shape named in the task's Edge Cases is unexercised; the behaviour is correct (`~/Code//portal` yields `…//portal` — a faithful separator-anchored tail, no panic), so this is an observation, not a gap worth a test.

CODE QUALITY:
- Project conventions: Followed. File is named after its subject with the matching `_test.go`, the import path is the repo's `charm.land/lipgloss/v2`, no raw colour or styling is involved, and the helper is unexported and package-local as the task prescribed.
- SOLID principles: Good — a pure function of a string and a width, with no seam to inject and nothing to stub; the width policy is separated from the row's styling exactly as the task's rationale argues.
- Complexity: Low. Two guards and one loop; no nesting beyond the loop body.
- Modern idioms: Yes. `strings.CutPrefix` in the test, a `range` over the string for the separator scan, and `strings.Trim` for the separators-only check.
- Readability: Good. The doc comment at `:15-22` states the three outcomes and names the environment dependency; the inline comment at `:33` states why left-to-right ordering is sufficient for "longest". Both hold against the code — no comment claims anything the implementation falsifies, and neither references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — reading settles that the suite compiles against the helper's signature and that each asserted expectation follows from the code, but not that the lane actually runs green; a `go test ./...` run (or at minimum `go test ./internal/tui`) is what would settle it.

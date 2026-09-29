TASK: Corrections (lazy-resume-on-attach-12-2, tick-0262ab) — the `state` row of CLAUDE.md states the narrowed address match as `indexPrevPanes` runs it

ACCEPTANCE CRITERIA:
- `CLAUDE.md:61` no longer contains "A skeleton-marked pane carrying no token still takes the whole previous record at its own address." It carries the Solution's replacement sentence verbatim in that place, between "…survives the housekeeping pass." and "The saver's per-pane scrollback skip is mid-restore".
- No other text in `CLAUDE.md` changes, no source or test file changes, and the existing tests stay green.

STATUS: complete

SPEC CONTEXT: The corrigendum dated 2026-09-29 in the specification (around line 474) narrows the address match. A pane carrying no token, whether mid-restore or waiting, does not take a previous record whose token a live pane in the same capture carries. That record belongs to the pane answering to its token, and the tokenless pane gets a fresh record. Without this, a renumbered restore could leave two saved records holding one token, and a reboot would then bake one resume hook into two panes. A record whose token no live pane carries is still taken by address.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:61 (the `state` row). Commit 23bfcf5c7 touches only CLAUDE.md (1 insertion, 1 deletion).
- Notes:
  - A word-level diff of 23bfcf5c7 shows the only change is the target sentence. The old sentence no longer appears anywhere in CLAUDE.md (grep count 0).
  - The replacement sentence matches the Solution's text byte for byte (checked with a fixed-string grep). It sits between "…survives the housekeeping pass." and "The saver's per-pane scrollback skip is mid-restore", both at the commit and at current HEAD. The later tasks 12-4 and 13-1 rewrote other parts of the row and left this sentence alone.
  - The sentence matches the code in `internal/state/capture.go`:
    - `liveTokenSet(idx)` (capture.go:106, :216-228) reads the live tokens from the fresh capture before any merge runs, which is what "a live pane in the same capture" means.
    - `indexPrevPanes` (capture.go:235-250) leaves a record out of `byAddress` when its token is in that live set.
    - `takePrevRecord` (capture.go:288-301) looks a tokenless pane up in `byAddress` only.
    - `mergeSkippedPanes` (capture.go:167-169) gives a tokenless skeleton pane the whole record (`*p = record`). `mergeFrozenPanes` (capture.go:198-202) routes a tokenless pending pane through `carryPrevContent` (capture.go:208-212), which copies exactly `CWD`, `CurrentCommand` and `ScrollbackFile`.
    - When a lookup finds nothing, both merges `continue` and the pane keeps the fresh record the capture built.
  - No other sentence in CLAUDE.md states the address-match rule (grep for "own address" and "by address"), so no stale copy of the old rule remains in the file.

TESTS:
- Status: Adequate (documentation-only task; no new tests expected)
- Coverage: No Go source or test file changed. No test reads CLAUDE.md (no `*_test.go` file mentions it), so reading alone settles that the existing tests still pass.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The sentence names no task ids, phases or spec sections, and it follows the row's existing style of naming behaviour and its reason.
- SOLID principles: N/A (prose)
- Complexity: Low
- Modern idioms: N/A
- Readability: Good. The new sentence covers both pane kinds, the exclusion and the reason for it in one statement, consistent with the preceding token-match sentence.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

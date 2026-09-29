TASK: lazy-resume-on-attach-10-3 (tick-2e8797) — Corrections: CLAUDE.md's `state` row describes the capture cycle as the tree runs it after phase 10

ACCEPTANCE CRITERIA:
- The three OLD sentences no longer appear in CLAUDE.md; the `state` row carries the NEW text in their place, and no other line of CLAUDE.md changes
- The row's `CaptureAndRefile` sentence also states that a tokened, token-shaped, skeleton-marked pane whose merged record names a positional file other than its own live key's has that file linked to its token-named path, with the record pointed there, before any caller's dump runs, in the terms the link code landed with
- Each claim of the replacement matches the tree: `CaptureCycle{Index, Pending, Skeleton}` and `CaptureAndRefile`'s marker read and error returns (`internal/state/scrollback.go`), the token-keyed `mergeSkippedPanes` with its tokenless address fallback (`internal/state/capture.go`), the dump skip on `capture.Skeleton` and `capture.Pending` (`paneSkipsScrollback`, `cmd/state_daemon.go`), and commit-now discarding both sets and dumping nothing (`cmd/state_commit_now.go`)
- No code or test changes; `go test ./...` stays green

STATUS: complete

SPEC CONTEXT: The spec's frozen-pane section says the token match covers the whole hand-over from restore to wait. A skeleton-marked (mid-restore) pane that carries a token takes its previous record by that token; one with no token takes it by address. Every committing capture reads the mid-restore markers itself. A mid-restore pane with a token-shaped token, whose record names a positional file that another record in the same capture also names, gets that file hard-linked to its token-named path and the record pointed there, before any scrollback is written that cycle. It is a link rather than a move because the hydrate helper may not have opened its baked path yet, and it happens only on a shared name so housekeeping cannot delete a file the helper has yet to read. The task keeps CLAUDE.md's architecture row in line with that contract as the code landed it.

IMPLEMENTATION:
- Status: Implemented
- Location: commit c01dfe842 changes one line, `CLAUDE.md:61` (the `state` row), with 1 insertion and 1 deletion. No other file is touched.
- Notes:
  - Word-diffing c01dfe842~1..c01dfe842 shows the only changed text is inside the three target sentences. The rest of line 61 and every other line of CLAUDE.md are byte-identical.
  - The delivered text is the prescribed NEW text with the extension the last Do bullet and the second criterion ask for:
    - "links each moved skeleton pane's bytes to its token name" is added inside the one-step account.
    - A following sentence spells out the link's conditions: token-shaped token, a record naming a positional file other than its own live key's, and that file named by another record too. It also covers the hard link (not a move) and its reason, the record being pointed there before any caller's dump, a record no other record shares keeping its name, and the one-WARN rule for a failed link.
    - The failed-capture clause becomes "nothing linked or re-filed".
    - NEW's semicolons became sentence breaks. That is presentation only and changes no claim.
  - Claims checked against the tree at c01dfe842:
    - `internal/state/scrollback.go` (`CaptureAndRefile`, then around line 275) reads `ListSkeletonMarkers` first and returns `fmt.Errorf("list skeleton markers: %w", err)` before any capture. It then takes `CaptureStructure(c, skeleton, prev, logger)`. On a capture error it returns the capture's error as given, before `linkMovedSkeletonScrollback` / `RefilePendingScrollback`. It returns `CaptureCycle{Index, Pending, Skeleton}`.
    - `linkMovedSkeletonScrollback` / `linkMovedPane` match the link sentence: skeleton membership, `recordsPerScrollbackFile` count >= 2, `PendingScrollbackPath` token-shape gate, skip when already on the token path or on its own positional path, `os.Link` via `placeStoredScrollback` (missing source or existing name tolerated), and one WARN on any other failure with the record left alone.
    - `internal/state/capture.go` `mergeSkippedPanes` takes the record by token through `takePrevRecord` and calls `carryPrevContent` (CWD, CurrentCommand, ScrollbackFile) for a tokened pane. For a tokenless skeleton pane it falls back to the address and takes the whole record (`*p = record`).
    - `cmd/state_daemon.go` `paneSkipsScrollback` reads `capture.Skeleton` then `capture.Pending`.
    - `cmd/state_commit_now.go` calls `CaptureAndRefile` and commits `capture.Index` without a scrollback dump.
  - Later tasks 12-2, 12-4 and 13-1 rewrote this passage further: the tokenless pending-pane address fallback and its token-carried-elsewhere exception, the unexported `captureAndRefile`, and `RunCommitCycle` with the commit lock. That is legitimate follow-on evolution, not drift from this task. At HEAD, none of the four OLD fragments appear in CLAUDE.md (grep count 0 each). The parts this task introduced still hold against HEAD's `internal/state/scrollback.go` (`linkMovedSkeletonScrollback` at line 196, `linkMovedPane` at line 233, `CaptureCycle` at line 258, `captureAndRefile` at line 275) and HEAD's `cmd/state_daemon.go:343` `paneSkipsScrollback`.

TESTS:
- Status: Adequate
- Coverage: This is a documentation-only task, so no test is expected. The commit touches no Go file, and no test in the tree reads CLAUDE.md (`git grep CLAUDE.md -- '*_test.go'` at c01dfe842 is empty). The suite's outcome therefore cannot differ across this commit, and reading settles the "stays green" criterion relative to the prior tree.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The edit is confined to the target row. The prose carries no task ids, phase numbers or spec-section references.
- SOLID principles: N/A (documentation)
- Complexity: Low
- Modern idioms: N/A
- Readability: Good. The added link sentence names its conditions in the same terms as the code's own doc comment on `linkMovedSkeletonScrollback`.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

TASK: The Whole-Server Pending Read Both Surfaces Take (lazy-resume-on-attach-6-1) — `tmux.Client.ListPendingResumePanes`, one `list-panes -a -F` read returning per-pane rows plus the pending-pane count and the set of sessions holding one

ACCEPTANCE CRITERIA:
- One `list-panes -a -F` call is issued per invocation and no per-session read is issued, whatever the number of sessions or panes.
- A pane whose marker field is `1` sets `Pending` on its row; a pane whose field is empty does not, and a pane that has never been marked is indistinguishable from one whose marker was set to the empty string.
- `Panes` is the number of pending panes and `Sessions` is the set of session names among them: a session holding two waiting panes contributes 2 to `Panes` and one entry to `Sessions`.
- A row whose session name contains the field separator parses with the whole name intact, because the cut is taken at the first separator and the marker holds the leading slot.
- A tmux failure returns the zero view and a wrapped error — never an empty view with a nil error.
- A server that is not running (the read errors), a server with no panes (`Rows` empty, nil error) and a server with panes but nothing pending (`Rows` non-empty, `Panes` zero, `Sessions` empty) are three distinguishable answers.
- `Sessions` is non-nil on every successful read, so a caller ranges over it without a nil check.
- The literal `@portal-resume-pending` appears nowhere in this file — the format is composed from `state.ResumePendingOption`, and the presence rule comes from `state.ResumePendingSet`.
- Against a real tmux server: one marked pane among several sessions is reported once, with its own session name, and the remaining panes are reported unmarked.

STATUS: complete

SPEC CONTEXT: Section 8 says the feature creates a new, otherwise invisible state (roughly forty-one waiting panes after a reboot), so it owes an answer to "what is waiting". Doctor reports a count of pending panes as an informational line (8.1). The picker's session row carries a pending dot when any pane in the session is waiting (8.3). A session holding two waiting panes therefore counts 2 in doctor and gets one dot in the list. The marker is the pane user-option `@portal-resume-pending`, set to `1`, and only its presence is read (7.3, 8.2). The whole-server pane-option enumeration already exists, so each surface costs one read (8.2).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:721-726 — `PendingResumeRow`
  - internal/tmux/tmux.go:732-736 — `PendingResumeView`
  - internal/tmux/tmux.go:740 — `resumePendingRowFormat`
  - internal/tmux/tmux.go:745-751 — `ListPendingResumePanes`
  - internal/tmux/tmux.go:753-771 — `parsePendingResumeRows`
  - CLAUDE.md:60 — the `tmux` row of the package table
- Notes:
  - The format is `"#{" + state.ResumePendingOption + "}" + paneHookRowSeparator + "#{session_name}"`. The marker is in the leading slot and the session name in the trailing one, and the literal occurs 0 times in tmux.go.
  - There is exactly one `ListAllPanesWithFormat` call and nothing issues a read per session.
  - The parser skips blank lines and uses `strings.Cut` at the first separator. It rejects a line with no separator using the same shape of error as `parsePaneHookRows`. It lifts the marker through `state.ResumePendingSet` rather than comparing it to `"1"`.
  - `Rows`, `Panes` and `Sessions` are all built in the same loop, and `Sessions` is initialised non-nil.
  - A tmux failure and a parse failure both return the zero view. The tmux error arrives wrapped by `ListAllPanesWithFormat` ("failed to list panes: %w").
  - As the plan requires, no session filter is added: `_portal-saver` and `_portal-bootstrap` panes are enumerated like any other.
  - The production consumers are in place: `cmd/doctor.go:102`, `internal/tui/pending_resume.go:30`, and the capture harness fake at `internal/capture/fakes.go:58`.
  - The CLAUDE.md row now names `ListPendingResumePanes` beside the hook-key enumeration. It gives the leading and trailing slot rule, where the marker constant and presence rule live, the rows / count / set return, the error-not-empty contract, and both consumers.
  - There is no drift from the plan.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tmux/tmux_test.go:2838-2946 (`TestListPendingResumePanes`) holds all eight unit subtests the plan names, plus one for a row with no separator.
  - The argv test asserts both that exactly one call was recorded and the exact argv. `wantFormat` is composed from `state.ResumePendingOption`. The mock is loud about any unexpected argv, so an extra call to anything else would also fail.
  - The separator case puts the marked row first and an unmarked row (whose line begins with the separator) second. A last-separator cut would fail it.
  - The failure test checks `errors.Is` against the cause and asserts the zero view: `Rows` nil, `Panes` 0, `Sessions` nil.
  - The no-panes and nothing-pending cases are asserted separately, and `Sessions` is checked non-nil for both "" and "|alpha".
  - internal/tmux/resume_pending_read_realtmux_test.go:10-45 is in the unit lane: no build tag, `tmuxtest.SkipIfNoTmux`, and a disposable `tmuxtest.New` socket, all reached through `seedRealTmuxServer`. It builds two sessions of two panes each and marks one pane with `ts.MarkResumePending`, which writes through raw tmux rather than the code under test. It then asserts `Panes == 1`, `Sessions == {waiting}`, one unmarked pane in the waiting session and two in the idle one.
- Notes: The tests are focused and none are redundant. Each subtest would fail on the regression it names.

CODE QUALITY:
- Project conventions: Followed. The format is composed from the owning constant, and the parse matches the shape of the sibling `parsePaneHookRows`. A failed read is an error, never an empty result, which is the stated tmux-package rule. Tests go through `commandertest`, the real-tmux test uses the shared `seedRealTmuxServer` fixture, and nothing uses `t.Parallel()`.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`strings.SplitSeq`, `strings.Cut`)
- Readability: Good. The doc comments on the view, the format constant and the method all hold true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Against a real tmux server: one marked pane among several sessions is reported once, with its own session name, and the remaining panes are reported unmarked." — Settling this needs `go test ./internal/tmux -run TestListPendingResumePanes_ReadsMarkedPaneFromRealServer` run on a machine with tmux installed, and confirming it passed rather than skipped. Reading shows the test is correctly built and asserts the right things; only running it against tmux shows the behaviour holds.

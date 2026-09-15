TASK: open-with-forced-filter-2-4 (tick-5cfe10) — Report a failed session-list read rather than counting it as no matches

ACCEPTANCE CRITERIA:
1. `ListSessionsProbe` returns the same sessions as `ListSessions` for identical output — window and attached counts, the `@portal-dir` column including an embedded `|`, and the underscore-prefix filter that keeps `_portal-saver` and `_portal-bootstrap` out
2. A failed `list-sessions` returns a non-nil error from `ListSessionsProbe` that unwraps to `*tmux.CommandError` with tmux's stderr intact, and a nil session slice
3. The same failure still returns `([]Session{}, nil)` from `ListSessions`, so the picker, `portal list` and `ListSessionNames` are unchanged
4. Empty output from a live server is `([]Session{}, nil)` from both — never a failure
5. A malformed line is an error from both, the same failure class as a failed read for the search form's purposes
6. `portal open /term` over a failing read returns that error: no picker is opened, no session is connected, and the error is not a `*UsageError`
7. `portal open /term` over a live server holding no sessions still opens the picker with a nil error
8. `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT: §3.7 ("How an attach is performed, and the one failure that is not a search result") states that a session list which could not be read is not a zero match: K = 0 means the search ran and found nothing (a filter result, §3.2, which opens the picker), while a failed read has searched nothing, so opening an empty picker on it would tell the user their sessions are gone while they are running. A tmux read failure is therefore reported in tmux's own terms and exits non-zero — the one failure path on the form — and is explicitly NOT one of §5.1's usage errors. §3.2 additionally fixes the searched set as the set the picker lists, with Portal's underscore-prefixed internal sessions absent from it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:122-125` — `listSessionsArgs`, the hoisted `list-sessions -F …` argv with the `@portal-dir`-must-stay-last rationale carried over from the old inline comment.
  - `internal/tmux/tmux.go:126-137` — `ListSessions`, contract-identical: `Run` error still returns `([]Session{}, nil)` behind the retained "Swallowed deliberately: the error is the no-server signal" comment; the tail routes through `parseSessionList`.
  - `internal/tmux/tmux.go:139-149` — `ListSessionsProbe`, documented as the discriminating variant in the shape `HasSessionProbe` (`internal/tmux/tmux.go:92-106`) sets; a `Run` error becomes `fmt.Errorf("failed to list tmux sessions: %w", err)` over a nil slice.
  - `internal/tmux/tmux.go:151-194` — `parseSessionList`, the previous body moved verbatim (line split, four-field `SplitN` with the trailing `@portal-dir` slot, the two `Atoi`s, the underscore-prefix filter). Confirmed verbatim against `git show f4571f8b8 -- internal/tmux/tmux.go`.
  - `cmd/open_search.go:139-147` — `SearchSessionSource` now declares `ListSessionsProbe()`; `cmd/open_search.go:178` consumes it in `searchCandidates`, which returns `(nil, err)` unchanged to `searchDecision` (`cmd/open_search.go:200-211`) and thence to `runSearchForm` (`cmd/open_search.go:234-240`).
  - Test doubles re-pointed: `cmd/open_search_test.go:504` (`fakeSearchSource`) and `cmd/open_search_test.go:681` (`verbatimSearchSource`). No other type is injected as `SearchSessions` (enumerated: `cmd/open_search_test.go:149,806,935`, `cmd/bootstrap_warnings_test.go:286`, `cmd/concurrent_bootstrap_route_test.go:111,144` — all `fakeSearchSource`).
- Notes:
  - Production wiring is unchanged: `buildSearchSessionSource` falls through to `tmuxClient(cmd)`, and `*tmux.Client` carries both `ListSessionsProbe` (`internal/tmux/tmux.go:143`) and `CurrentSessionName` (`internal/tmux/tmux.go:236`).
  - The swallow is preserved for every pre-existing caller — the remaining `ListSessions()` call sites are `cmd/completion.go:16`, `cmd/list.go:55`, `internal/capture/harness.go:38`, `internal/tui/model.go:1371,1476,2804,2867` and `ListSessionNames` (`internal/tmux/tmux.go:201`). None was re-pointed, which is what criterion 3 asks for.
  - `errors.As` recovery works on the production path, not merely against the fake: `RealCommander.Run` wraps every failure via `WrapCommandError` into a `*CommandError` (`internal/tmux/tmux.go:50-61`, `internal/tmux/command_error.go:64-77`), and `%w` keeps the chain intact.
  - Criterion 6's "not a `*UsageError`" holds structurally: the error originates in the client and is returned from `RunE`, never from `validateOpenArgs`, which is the only producer of `NewUsageError` on this command (`cmd/open_search.go:77-107`).

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tmux/list_sessions_probe_test.go:23-60` — "it returns tmux's error when the session list cannot be read" asserts non-nil error, nil slice, `errors.As` to `*tmux.CommandError`, `Stderr` intact, and the stderr present in the rendered message (criterion 2); its sibling subtest re-pins the swallow on the identical scripted failure (criterion 3).
  - `internal/tmux/list_sessions_probe_test.go:62-113` — one table driving both readers through a `map[string]func(*tmux.Client) ([]tmux.Session, error)`, covering window/attached counts, the `@portal-dir` column with an embedded `|`, the `_portal-saver`/`_portal-bootstrap` filter, and empty output (criteria 1 and 4). Each iteration builds its own mock, so the map's random order carries no coupling.
  - `internal/tmux/list_sessions_probe_test.go:115-129` — malformed line is an error from both readers (criterion 5).
  - `internal/tmux/list_sessions_probe_test.go:131-147` — argv parity between the two readers, which is the drift the task's edge case names (a directory containing a literal `|` survives only in the trailing `SplitN` slot).
  - `cmd/open_search_test.go:728-745` — `TestOpenCommand_SearchForm_ReturnsAnEnumerationError`: the error surfaces from `rootCmd.Execute()`, is not a `*UsageError`, and neither `openTUIFunc` nor `openSessionFunc` ran (criterion 6).
  - `cmd/open_search_test.go:581-592` — `TestOpenCommand_SearchForm_OpensPickerWhenNoSessionsAreLive` drives `executeOpen`, which fatals on a non-nil error (`cmd/open_search_test.go:180-188`), so the nil-error half of criterion 7 is asserted alongside `tuiCalled` (criterion 7).
- Notes:
  - Each test would fail if the behaviour broke: re-inlining a divergent argv into the probe fails the parity test; swallowing the probe's error fails the first subtest and the `cmd` test; dropping the `%w` fails the `errors.As` assertion.
  - There is modest overlap with the pre-existing `TestListSessions*` family in `internal/tmux/tmux_test.go:14-270` (parse table, `@portal-dir`, underscore filter). The new table is not a third copy of those assertions — its subject is parity between the two readers, which no existing test could state — so I do not read it as over-testing.
  - The fake `*CommandError` in `failingListSessions` carries `Err: errors.New("exit status 1")` rather than an `*exec.ExitError`, so `renderWithArgs` omits the exit-code segment; the stderr assertion still holds and stderr is the part the criterion names.

CODE QUALITY:
- Project conventions: Followed. `ListSessionsProbe` mirrors the established `HasSessionProbe` discriminating-variant shape and naming; the wrap message matches the package's neighbouring `fmt.Errorf("failed to …: %w", err)` style; the new test file is `package tmux_test` like its siblings and uses `commandertest` rather than a hand-rolled fake.
- SOLID principles: Good. The extraction gives the parse one home and leaves each reader with a single responsibility — the error policy — rather than two copies of the parse; the additive variant leaves every existing caller's contract untouched (open/closed).
- Complexity: Low. Both readers are four lines over a shared parse; no new branching beyond the one error arm.
- Modern idioms: Yes. `%w` wrapping, `errors.As` recovery at the call site, method values (`(*tmux.Client).ListSessions`) to drive one table over both readers.
- Readability: Good. The hoisted argv carries the ordering rationale that previously sat inline, and each doc comment states the contract its caller depends on.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — settled only by running both lanes; reading cannot establish it. Nothing in the change-set is lane-relevant beyond the unit lane (the new test file is untagged and builds no binary), but the run itself is what would confirm it.

TASK: resume-hooks-silently-lost-4-8 (tick-1f710f) — "The Docs Describe The Pane-Token Enumeration As Stale-Cleanup's Alone"

ACCEPTANCE CRITERIA:
- `CLAUDE.md:60` names the enumeration by what it returns, not by one caller
- The Resume-hook command row records `hook list`'s single live enumeration and the fourth column's three empty cases
- `README.md`'s `hook list` line mentions the fourth column
- No other CLAUDE.md or README passage is edited; no CHANGELOG entry
- No code file changes
- `go test ./...` and `go test -tags integration -p 1 ./...` pass, unchanged

STATUS: issues_found (0 blocking — the single finding's whole remedy is documentation text)

SPEC CONTEXT: This is a documentation-only task inside the resume-hooks bugfix. The behaviour it documents was built by task 4-3: `hook list` gained a fourth column resolving each persisted hook key to the live pane that carries the token, reached through the new `PaneHookLister` seam over `tmux.ListAllPaneHookKeys`. Before this task the docs described that enumeration as belonging solely to the daemon's stale-cleanup, and README's `hook list` line said only "list all hooks".

IMPLEMENTATION:
- Status: Implemented
- Location: commit bdee585e — `CLAUDE.md:45` (Resume-hook command row), `CLAUDE.md:60` (tmux package row), `CLAUDE.md:186` (Key-producing sites paragraph), `README.md:203` (`hook list` example line). No code files touched (`git show --stat bdee585e` lists only `.tick/tasks.jsonl`, the workflow manifest, `CLAUDE.md`, `README.md`).
- Notes: every behavioural claim the new prose makes holds against the code as it stands:
  - "one live `tmux.ListAllPaneHookKeys` enumeration per invocation" — `cmd/hooks.go:152` calls `paneLocationsByToken` once, outside the render loop; `cmd/hooks.go:165` is the single read. Pinned by `cmd/hooks_test.go:266` (`enumeration reads = %d, want 1`).
  - "none at all when there are no entries" — `cmd/hooks.go:147-149` returns before the lister is reached; pinned by the "it takes no enumeration read when there are no entries" subtest with a `loudPaneHookLister`.
  - "fourth tab-separated column after key/event/command" — `cmd/hooks.go:155` formats `"%s\t%s\t%s\t%s\n"` with `h.Key, h.Event, h.Command, locations[h.Key]`.
  - "empty when no live pane carries the token" — map miss yields `""`; pinned by the "token no row carries" subtest.
  - "empty when the key is not token-shaped" — holds by construction: live tokens are minted at `paneTokenWidth` over the id alphabet, so a legacy `<session>:w.p` key can never key into the map, and `cmd/hooks.go:176-179` skips the empty token of an unstamped pane so it cannot lend its location.
  - "empty when the read itself fails (no server running) — the listing still succeeds" — `cmd/hooks.go:166-172` returns a nil map on error and the command returns nil; pinned by the "enumeration fails" subtest, which deliberately returns rows *alongside* the error.
  - "the whole-server `list-panes -a -F` enumeration" — `internal/tmux/tmux.go:658` → `ListAllPanesWithFormat` → `internal/tmux/tmux.go:604` `Run("list-panes", "-a", "-F", format)`.
  The third CLAUDE.md hunk (the Key-producing sites paragraph, now line 186) is not an unauthorised extra edit: the task's Problem statement names that exact sentence ("`CLAUDE.md:166` repeats 'stale-cleanup enumerates live tokens via `tmux.ListAllPaneHookKeys`'") as part of the subject. The phase-3 bank entry's subject (the shape-aware reaper / token-shape predicate) was correctly left untouched. No CHANGELOG entry.

TESTS:
- Status: Adequate (none required)
- Coverage: A prose edit changes no behaviour, so no new test is owed. The behaviour the prose describes is separately test-backed by `cmd/hooks_test.go:119-215` (`TestHooksListLocationColumn`: four-column render, unstamped-pane row lends nothing, duplicate token renders one line, failed enumeration renders every fourth field empty, zero entries takes no read, token no row carries) plus the one-read assertion at `cmd/hooks_test.go:266` — so each documented case has an observer.
- Notes: no over-testing; nothing was added for this task.

CODE QUALITY:
- Project conventions: N/A (no code). The doc edits stay in the established CLAUDE.md register — the tmux row keeps its single-row table format, the Resume-hook row keeps its narrative form.
- SOLID principles: N/A
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good. The new Resume-hook-row sentence states the read, its per-invocation count, the column position and all three empty cases in one pass, which is what a reader of that row needs.
- Issues: one attribution claim in the rewritten `CLAUDE.md:60` clause is falsified by the code — see FINDINGS.

BLOCKING ISSUES:
- None. Every acceptance criterion is met in substance, and the one finding's entire remedy is documentation text.

FINDINGS:
- [in-scope] [contained] CLAUDE.md:60 — the rewritten clause ends "two consumers — the daemon's stale-cleanup and `hook list`'s location column", but `portal doctor` drives the same enumeration twice over and is named in neither: `cmd/doctor.go:201` (`hooksweep.Run` under `--fix`) and `cmd/doctor.go:379` (`hooksweep.JudgeAgainstLivePanes` on the read-only stale-hook count, which reaches `internal/hooksweep/sweep.go:86`). This was already true when the sentence was written — at commit bdee585e `cmd/doctor.go:314` called `lister.ListAllPaneHookKeys()` directly, a third call site beside `cmd/run_hook_stale_cleanup.go` and `cmd/hooks.go`. Name the staleness machinery rather than one of its drivers, e.g. "two consumers — `internal/hooksweep`'s staleness cycle (the daemon's throttled sweep, `portal doctor --fix`, and doctor's read-only count) and `hook list`'s location column". — FAILS: the doc contradicts CLAUDE.md's own `hooksweep` row (line 73: "Its two callers are the daemon's throttled sweep … and `portal doctor --fix` …", plus doctor's read-only count), and a reader changing the read's contract on the strength of the count — its error shape, its empty-result meaning, its cost — is told doctor's read-only diagnosis is not affected when it is, which is the very "described as single-consumer" failure this task set out to remove, moved one caller along.

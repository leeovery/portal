TASK: resume-hooks-silently-lost-9-52 — CLAUDE.md's Architecture Rows Carry Three Claims The Tree Has Moved Past (tick-bbd73e, severity: comments)

ACCEPTANCE CRITERIA:
- [x] The `tmux` row's enumeration includes `attach-session` and both of its sites.
- [x] The `session` row describes sanitisation and the generation-to-recogniser pinning.
- [x] Neither re-voiced `logtest` claim states a count.
- [x] The `Sink`-everywhere claim accounts for the JSON-handler exception by name.
- [x] Every statement in the edited rows is true against the tree at the time of the edit.

STATUS: complete

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification (per the shared verifier context). Its subject is documentation drift in CLAUDE.md's architecture table: three claims that had fallen behind machinery built by sibling tasks in this work unit (the exact-target pinning of hand-composed tmux argvs, `SanitiseProjectName` + generation-to-recogniser pinning, and the `logtest` consolidation). Documentation-only: no code and no test semantics change.

IMPLEMENTATION:
- Status: Implemented
- Location: `CLAUDE.md:60` (`tmux` row), `CLAUDE.md:64` (`session` row), `CLAUDE.md:84` (`logtest` row)
- Notes:
  - (a) `attach-session` is in the `tmux` row's exact-target enumeration with both sites named: "the two hand-composed `attach-session` argvs in `cmd/open.go` and `internal/session/quickstart.go`". Both hold — `cmd/open.go:95` composes `{"tmux", "attach-session", "-t", string(tmux.CoordTargetExact(name))}`, and `internal/session/quickstart.go:68` appends `";", "attach-session", "-t", target` over the target composed at `internal/session/quickstart.go:65`. The "argv the client does not run" character of the quickstart site is carried both in the row (the later sentence naming "the exec'd chains in `cmd/open.go` and `internal/session/quickstart.go`" as the argv boundary a `Target` is spent at) and in-source at `internal/session/quickstart.go:63-64`.
  - Sound divergence from the Do list, not a defect: the Do list asked for the two sites as `cmd/open.go:88` and `internal/session/quickstart.go:62`; the delivered row names the files without line numbers. Both of those numbers are already stale (the argv now sits at `cmd/open.go:95` / `internal/session/quickstart.go:68`), which is precisely the rot a file-level citation avoids. The criterion ("both of its sites") is met in substance, and the delivered form is the more durable one.
  - The row now assigns those sites to `CoordTargetExact` rather than `SessionTargetExact` (and states `SessionTargetExact` is taken by no production call site). That is a later measurement task's correction, and it matches the tree — both call sites construct `CoordTargetExact`.
  - (b) The `session` row carries the sanitisation sentence and the pinning sentence. Verified against `internal/session/naming.go:31-35`: `SanitiseProjectName` replaces `.` and `:` with `-` and `TrimLeft`s the leading `$`/`-` (dropped, not substituted), and `internal/session/naming.go:49-54` gives the "empty fragment contributes no separator" behaviour the row describes. The generation-to-recogniser pinning is real and covered by `internal/session/naming_test.go:219` (`TestGenerateSessionNameProducesAddressableNames`, driving hostile fragments through `tmux.ValidateSessionName`). The row's parenthetical that the validator permits a period holds — `internal/tmux/session_name_test.go:225` asserts `ValidateSessionName("a.b") == nil`.
  - (c) The `logtest` row's two named claims no longer state a count: the handlers are named ("`warnBypassHandler` (`cmd/open_test.go`) and `orchestrationSeqHandler` (`cmd/bootstrap/latch_test.go`)") and the shared record properties are listed ("level, message, `component`, `op` and `via`") rather than numbered. Both are true against the tree: `warnBypassHandler` is declared at `cmd/open_test.go:2859` and does exactly what the row says — it models the production level gate (`Handle`, `cmd/open_test.go:2876-2882`) and forwards survivors into a `logtest.Sink` (`cmd/open_test.go:2864-2867`); `orchestrationSeqHandler` at `cmd/bootstrap/latch_test.go:35-47` captures no records and appends one ordering marker into a shared sequence. `RecordWant`'s fields at `internal/logtest/assert.go:14-20` are precisely Level/Msg/Component/Op/Via, checked one-by-one at `internal/logtest/assert.go:25-42`.
  - (d) The `Sink`-everywhere claim is re-voiced as "reach for `Sink` unless the subject forbids it" and names the exception: `internal/hooks/store_test.go`'s `TestSetEmitsOpAsJSONField`, which exists at `internal/hooks/store_test.go:1255` and installs `slog.NewJSONHandler` over a buffer at `internal/hooks/store_test.go:1260`, asserting the parsed JSON object — exactly the rendering the row says a `Sink` does not produce. The second named exception also holds: `internal/log/rotate_test.go:15` declares `componentCapture` and `internal/log/log_test.go:1` is `package log` with `recordingHandler` in use at `internal/log/log_test.go:42`.
  - Non-finding observation: the same row retains one count-shaped phrase outside the three claims the task named — "two structural twins of `Sink` live" (`CLAUDE.md:84`). It is true of what I located (`recordingHandler`, `componentCapture`), I did not enumerate `internal/log`'s test files exhaustively, and it is not one of the claims criterion 3 governs. Recorded as context, not as a finding.
  - The rest of the `logtest` row's surface claims check out against `internal/logtest/capture.go`: `Records()` as the single base query (`:250`), the four chained filters (`AtExactLevel` `:130`, `AtOrAboveLevel` `:135`, `WithMessage` `:141`, `Matching` `:149`), `Records.Only` as the single exactly-one terminal (`:157`), and `Install` routing every component logger into a fresh sink (`internal/logtest/install.go:11-16`).
  - No process-artifact residue in any edited row: no task ids, phase numbers or spec section references.

TESTS:
- Status: Adequate (none required)
- Coverage: A documentation edit to CLAUDE.md has no test surface, and the task's Tests section asks only that the relevant suites stay green with no edit. The claims are not merely prose in every case — the two behavioural ones are independently pinned by existing tests (`internal/session/naming_test.go:219` for the generation-to-recogniser pinning; `internal/tmux/session_name_test.go:225` for the period the row says the validator permits), and the exact-target argv shape is asserted at `cmd/open_test.go:2613` (`{"tmux","attach-session","-t","=foo:"}`) and `cmd/open_test.go:2633` (the same argv re-derived through `tmux.CoordTargetExact`, so a second spelling of the prefix cannot creep in).
- Notes: No test file was touched by this task, which is correct for its scope. Nothing here is under- or over-tested.

CODE QUALITY:
- Project conventions: Followed. CLAUDE.md is the binding convention document and the edit keeps its established voice — claims stated as behaviour with the mechanism named, no `§` references, no line numbers that would rot.
- SOLID principles: N/A (documentation-only)
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good. The re-voicing reads as description rather than inventory; the `logtest` row's "reach for `Sink` unless the subject forbids it" framing states the rule and its two named exemptions without pretending the exemption set is closed by number.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

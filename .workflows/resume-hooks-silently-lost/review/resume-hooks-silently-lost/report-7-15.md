TASK: resume-hooks-silently-lost-7-15 (tick-c0096e) — "logtest's Filter Surface Advertises An Unused Idiom And Lacks The One Three Packages Re-Author"

ACCEPTANCE CRITERIA:
- The three chainable filters are unexported and no caller outside `internal/logtest` chains them.
- `Sink.RecordsWithMessage` exists and returns the records carrying that message under any component.
- `themeEventRecords`, `recordsNamed` and `captureSink.recordsWithMessage` are all gone, along with `internal/restore`'s wrapper type and projection.
- `internal/restore` declares no reporter interface and no recording-fataller of its own.
- Every converted assertion covers the same records it covered before.
- `go test ./...` and the integration lane pass with no test renamed.

STATUS: complete

SPEC CONTEXT: This is a phase-7 task — an implementation-analysis (consolidation) item, not specified bugfix work — so its authority is its own body, per the shared verifier context. Its subject is the test-only `internal/logtest` query surface: one route per question, and a message-only record filter that three packages had each re-authored.

IMPLEMENTATION:
- Status: Implemented (delivered at commit `d7ae4889`), with the export direction later and deliberately reversed by task 8-42 (`abe3132a`, "one route per question, composed from orthogonal filters"). The task's *outcome* holds at HEAD; the *mechanism* named in its Do list does not, and that is a sound supersession, not a loss.
- Location:
  - `internal/logtest/capture.go:113` — `Record.Matches`, added by this task, is the single home of the `(component, message)` predicate; `Records.Matching` (`capture.go:149`) is built on it and `internal/tmux/portal_saver_lifecycle_events_test.go:315` (the capture-order walk) consumes it directly rather than restating the rule.
  - `internal/logtest/capture.go:141` — the message-only filter this task introduced (as `Sink.RecordsWithMessage` at the time; `Records.WithMessage` after 8-42 folded the forwarders away). It is consumed from `internal/theme`, `internal/tui`, `internal/hooks`, `internal/project`, `internal/hooksweep` and `main_panic_test.go`.
  - `internal/logtest/capture.go:126-163` — the surface at HEAD is exactly one base query (`Sink.Records`, `:250`), four orthogonal filters and one `Only` terminal; no `Sink` forwarder survives (`grep` for `sink.RecordsAt*` / `sink.RecordsWith*` / `OnlyRecord` across the repo returns nothing).
- Notes on each criterion:
  1. *Filters unexported* — not true at HEAD, by the later task's decision: they are exported and chained, with the "one route per query" property held instead by the rule that no *combination* is a method. CLAUDE.md's `logtest` row states this current shape accurately (line 84), and this task's own commit updated that row to match the shape it delivered. Judged as intent-satisfied, not as drift.
  2. *`Sink.RecordsWithMessage`* — delivered by this task; now `Records.WithMessage`, same semantics (message alone, any component, order-preserving, nil when none match).
  3. *Re-authored helpers gone* — `themeEventRecords`, `recordsNamed`, `captureSink.recordsWithMessage`, `capturedRecord` and restore's `newCaptureLogger` return no matches anywhere in the tree. `internal/restore`'s `openTestLogger` pass-through was removed in the same commit.
  4. *No reporter interface / recording-fataller in `internal/restore`* — holds: `internal/restore` declares no `interface` type at all. Its two named targets (`marker_assert_test.go`, `marker_assert_meta_test.go`) had already been deleted by sibling task 7-8, and `logging_capture_test.go` by 7-27; the commit message records this honestly rather than claiming work it did not do.
  5. *Conversions preserve the record sets* — checked site by site against the filter definitions: `RecordsWith(c,m).AtExactLevel(l)` → `RecordsAtExactLevelWith(l,c,m)` is `RecordsWith(c,m).atExactLevel(l)` by construction; `RecordsAtExactLevel(l).Msg(m)` → `RecordsAtExactLevelWithMessage(l,m)` is `RecordsWithMessage(m).atExactLevel(l)` — same set, both filters order-preserving. `Records().Msg(x)` → `RecordsWithMessage(x)` is the identical predicate. `firstSaverIndex` (`internal/tmux/portal_saver_lifecycle_events_test.go:313-320`) moved from a prefix-count idiom (`len(recs[:i+1].With(...)) == 1`) to `r.Matches(...)`: both return the index of the first match, and the rewrite is the clearer of the two. `cmd/open_test.go:2649-2663`'s `orderingExecer` dropped its `recordsAtCall` field — it had exactly one reader (the `if` on the next line, confirmed against `d7ae4889^`), the read still happens inside `Exec`, so the before-the-handoff ordering the test exists to pin is unchanged, and its doc comment was corrected from "Snapshots" to "Reads" in the same edit.
  6. *No test renamed* — the commit renames none (`TestRecords_MsgFiltersOnMessageAloneAcrossComponents` keeps its name and only its body/messages change); it adds three.

TESTS:
- Status: Adequate, with one small redundancy.
- Coverage: `internal/logtest/capture_test.go:407-459` covers the message-only filter — every record carrying the message across three different components and one with no component, capture order and levels asserted, plus the nil-for-unemitted case. `:461-490` (`TestRecords_ComposedRoutesCoverEveryQuery`, this task's `TestSink_ForwardersCoverEveryQuery` carried forward) drives all seven questions over one capture with distinct expected counts (5/3/4/3/4/1/2), so a filter that silently widened or narrowed would move at least one count. `:492-510` (`TestRecord_Matches`, added here) pins the predicate's three negative cases: wrong component, no component attr at all, wrong message. Each would fail if the behaviour it names broke.
- Notes: `TestRecords_WithMessage`'s third subtest ("it returns nothing for a message no record carries") duplicates the final assertion of its first subtest five lines above, and the second subtest re-asserts the first's count-3-across-components property with a different fixture. Both subtests come from this task (the enclosing test was assembled by 8-42). The overlap is small and harmless — flagged for balance, not raised as a finding.

CODE QUALITY:
- Project conventions: Followed. `logtest` stays test-only and leaf (its dependency set is now guarded by `internal/logtest/leaf_guard_test.go`); no production code is touched by this task; no `slog.Handler` is hand-rolled; CLAUDE.md's architecture row was updated in the same commit to describe the surface as delivered, and describes the post-8-42 surface accurately at HEAD.
- SOLID principles: Good. `Record.Matches` gives the `(component, message)` predicate a single home, and `Records.Matching` lifts it rather than restating either half — the reason the capture-order walk in `internal/tmux` could stop re-deriving it.
- Complexity: Low. Every filter is a one-line predicate over the shared `Records.filter`.
- Modern idioms: Yes — a named slice type with value-receiver, allocation-free-on-empty filters returning nil.
- Readability: Good. The doc comments on `Records` and `Matches` say why the shape is what it is, not what the code already says, and carry no task ids, phase numbers or spec references.
- Issues: None. (Observation, out of this task's change-set and not reported as a finding: `internal/state/fifo_sweep_test.go:15` still declares the same `openTestLogger(t, dir)` pass-through with its `_ = dir` that this commit deleted from `internal/restore`; the file was never touched here and the task's Do list named only restore's copy.)

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

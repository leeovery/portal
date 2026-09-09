TASK: 76 Inline logtest.Sink Installs Survive Outside logtest.Install (resume-hooks-silently-lost-7-27)

ACCEPTANCE CRITERIA:
- No `*_test.go` constructs a `logtest.Sink` and calls `log.SetTestHandler` as two adjacent statements.
- The two wrapper structs are gone and their consumers read the `Sink` directly.
- Every surviving `log.SetTestHandler` call is doing something `Install` does not.
- Every converted assertion captures the same records it captured before.
- `go test ./...` and the integration lane pass with no test renamed.

STATUS: complete

SPEC CONTEXT: None applicable. This is a phase-7 consolidation task; per the shared verifier context, a phase 6-10 task's authority is its own body. The specification never mentions `logtest`, `Sink` or `SetTestHandler` (grepped: zero hits), so there is no spec behaviour to check the sweep against. The binding context is CLAUDE.md's `logtest` architecture row, which the commit amends in the same breath as the code.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `069f26af` — 39 files, +226/-461, test files and CLAUDE.md only (no production source touched). Key sites: `internal/logtest/install.go:11` (the route), `internal/restore/logging_capture_test.go` (deleted), `cmd/open_test.go` (bespoke `capturingHandler` deleted), `main_panic_test.go:14` (bespoke `captureHandler` deleted).
- Notes:
  - Criterion 1 holds at HEAD. Enumerating every `log.SetTestHandler` call in the tree gives exactly three outside `internal/log` (its own suites, which cannot import `logtest` — that would be an import cycle) and outside `internal/logtest` itself: `cmd/logging_capture_test.go:30`, `cmd/open_test.go:2918`, `internal/hooks/store_test.go:1260`. None of the three is preceded by a `logtest.Sink` construction. The five further hits in `internal/logtest/install_guard_rule_test.go` are inside backtick fixture-source constants (lines 12-72), not calls.
  - Criterion 2 holds. `internal/restore/logging_capture_test.go` (the `captureSink` embedded-field wrapper plus its `capturedRecord`/`recordsWithMessage`/`newCaptureLogger`) is deleted outright, consumers routed to `logtest.NewCaptureLogger` (`internal/logtest/capture.go:257`) and `*logtest.Sink` directly. The `internal/tmux` wrapper the task named was already unpicked by an earlier task — `git show 069f26af~1:internal/tmux/portal_saver_lifecycle_events_test.go` is already all `logtest.Install(t)`, which is why that file is absent from this commit's diffstat. Nothing was skipped; the work was simply already done.
  - Criterion 3 holds, and each survivor carries its reason in-source: `cmd/logging_capture_test.go:22-24` (a discard silencing the pre-`log.Init` window, whose records nothing reads back), `cmd/open_test.go:2914-2916` (a handler modelling the production level gate, which a `Sink` admitting every level structurally cannot), `internal/hooks/store_test.go:1256-1258` (a JSON handler asserting the rendering of an emission). A later task (`157ccfab`, 8-49) turned that prose into `internal/logtest/install_guard_test.go`, whose `sanctionedHandlerInstalls` names the same three sites and fails on any fourth — so the criterion is now held by a guard rather than by this sweep alone.
  - No dangling references to the deleted helpers survive (`newCaptureLogger`, `captureSink`, `capturingHandler`, `captureHandler` — zero hits tree-wide outside `logtest.NewCaptureLogger` and `cmd`'s unrelated `newCaptureLoggerForComponent`), and every file whose `internal/log` import was retained still uses it (checked each of the eight: `log.For(...)` in all cases). No file is left with an unused import.

TESTS:
- Status: Adequate
- Coverage: Correctly no new test. The subject is a mechanical test-harness conversion with no production change, so the existing suites across `cmd`, `cmd/bootstrap`, `cmd/capturetool`, `internal/tmux`, `internal/theme`, `internal/tui`, `internal/restore`, `internal/prefs` and `internal/hooks` are the verification, and `logtest.Install`'s own coverage (`internal/logtest/install_test.go:10` routing, `:24` restore-on-cleanup including the nested-subtest case) already pins the helper's contract.
- Notes:
  - Criterion 4 (same records captured) verified by reading, per conversion class. The ~44 inline pairs are `sink := &logtest.Sink{}` + `log.SetTestHandler(t, sink)` replaced by `sink := logtest.Install(t)`, which is those exact two statements (`internal/logtest/install.go:12-15`) — identical by construction.
  - The two non-trivial conversions were checked against the deleted handlers' semantics. `cmd/open_test.go`'s `capturingHandler` and `main_panic_test.go`'s `captureHandler` both had `Enabled` returning true unconditionally, matching `Sink.Enabled` (`internal/logtest/capture.go:194`), so no record newly appears or disappears on a level boundary. The old `resolveRecords`/`filterExecRecords` filtered on message plus a `component` attr looked up across bound-then-record attrs; `Records.With(component, msg)` (now `Matching`, `internal/logtest/capture.go:149`) applies the same pair via `Record.Matches`, and `Sink.Handle` merges bound attrs then record attrs into `Record.Attrs` with record-wins on collision — the same precedence the deleted `recordStringAttr` implemented.
  - The one behavioural difference is a widening, not a loss: the old handlers dropped `WithAttrs`-bound attrs from the captured record (both returned a receiver or a merged copy read only through the bespoke accessor), whereas `Sink` records them into `Keys`/`Attrs`. No converted assertion pins an exact key set on a record whose keys the binding changes — the two exact-key-set assertions (`internal/restore/session_geometry_summary_test.go` `want{anomalous,panes,took}` and `internal/restore/session_test.go` `wantKeys{session,pane_key,error}`) run over `logtest.NewCaptureLogger`, a bare `slog.New(sink)` with nothing bound, exactly as their predecessor did.
  - Criterion 5's "no test renamed" holds structurally: the commit changes no `func Test...` line at all.

CODE QUALITY:
- Project conventions: Followed. Test-only change; the lane rule is untouched (no test gains or loses a build tag, nothing new builds or spawns a binary). CLAUDE.md's `logtest` architecture row is updated in the same commit to record that `internal/restore`'s wrapper is gone, so the doc and the tree agree.
- SOLID principles: Good. The change collapses four hand-rolled `slog.Handler` implementations onto one, leaving the capture-and-restore pairing with a single owner.
- Complexity: Low. ~120 lines of duplicated handler machinery deleted; the replacements are call-site substitutions.
- Modern idioms: Yes.
- Readability: Good. Each of the three retained `SetTestHandler` sites now states, at the site, what `Install` could not do there — which is what makes the survivors auditable rather than merely tolerated.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

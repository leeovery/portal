TASK: resume-hooks-silently-lost-7-4 — "A Second slog Capture Handler Lives Beside logtest.Sink, And This Work Unit Extended It" (tick-2b5d0a). Delete the hand-rolled `recordedLog`/`recordingLogger`/`intAttr`/`countMatching`/`onlyMatching` family from `cmd/bootstrap_production_test.go`, migrate `internal/tmux`'s `recordingSlogHandler` and `recordingMigrationLogger` onto `logtest.Sink`, and leave `internal/logtest` as the tree's only capture handler.

ACCEPTANCE CRITERIA:
- `cmd/bootstrap_production_test.go` declares no `slog.Handler` and no record/attr accessor of its own.
- `internal/tmux` declares no `slog.Handler` of its own; both former capture types are gone with their filters.
- Every migrated assertion asserts on the same component, message, level and attrs it did before the change.
- No production file imports `internal/logtest`.
- `go test ./...` and `go test -tags integration -p 1 ./...` pass with no test renamed and no assertion weakened.

STATUS: complete

SPEC CONTEXT: None applicable. This is a phase-7 implementation-analysis task; the specification (`.workflows/resume-hooks-silently-lost/specification/.../specification.md`) mentions neither `logtest` nor capture handlers (grepped: zero hits), so per the shared verifier context the task body is its own authority. The binding project convention is CLAUDE.md's `logtest` row: `Sink` "is what a suite needing captured records constructs, rather than declaring an `slog.Handler` of its own", with named exceptions where the subject forbids it.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `66ee59b5` (12 files, +325/−545). Deletions: `cmd/bootstrap_production_test.go` (the whole `recordedLog`/`recordingLogger`/`intAttr`/`countMatching`/`onlyMatching` block, 113 lines); `internal/tmux/portal_saver_test.go` (`recordingBarrierLogger`, `recordingSlogHandler`); `internal/tmux/hooks_register_test.go` (`recordingMigrationLogger`). Replacements: `internal/tmux/portal_saver_test.go:1144-1156` (`barrierLog`, a thin `logtest.Sink` holder whose `warns()` is `Records().AtExactLevel(slog.LevelWarn)` rendered as `"<component> | <msg>"`); `internal/tmux/hooks_register_test.go:707-745` (`migrationLog` + `migrationLines`/`migrationLine`/`migrationReaped` over the Sink); `internal/logtest/capture.go:141` (the new message-only filter, added by this task as `Msg`, since renamed `WithMessage` by a later phase).
- Notes: The current tree confirms the end state. `cmd/bootstrap_production_test.go` is now 19 lines holding two interface assertions plus `keysOf` (used from six other suites) and no handler. Grepping `slog.Handler` across the tree returns no hit in `internal/tmux` and no hit in `cmd/bootstrap_production_test.go`; the surviving declarations are `cmd/open_test.go`'s `warnBypassHandler`, `cmd/bootstrap/latch_test.go`'s `orchestrationSeqHandler`, and `internal/log`'s own `recordingHandler`/`componentCapture` — each named and justified in CLAUDE.md's `logtest` row (level-gate model, ordering marker, and in-package tests that cannot import `logtest` without a cycle), so Do-step 5's "remove or justify" holds in the delivered state. The only non-`_test.go` file importing `internal/logtest` is `internal/hookstest/hooks_lock.go`, which belongs to a package CLAUDE.md declares test-only (and the import predates this task, landing in `36265b17`/6-18), so the "no production file" criterion holds.
- Later phases moved on top of this work without regressing it: `cmd/run_hook_stale_cleanup*_test.go` were renamed into the `cmd/hook_prune_*`/`hook_sweep_*` family by 9-12, and the two-route staging the task deliberately preserved is intact (`cmd/hook_prune_single_report_test.go:22-23,54-55` still pairs `logtest.Install(t)` with `newCaptureLoggerForComponent(t, "daemon")`). No stale reference to the task's own transitional API survives: grepping `RecordsWith`/`RecordsAtExactLevel`/`OnlyRecord`/`.Msg(` yields only an unrelated `tea.Msg` conversion in `cmd/capturetool/render_size_test.go:64`.

TESTS:
- Status: Adequate
- Coverage: This is a test-infrastructure migration, so the existing suites are the verification — as the task's Tests section states. I read every migrated assertion in the diff against its predecessor and each covers the same records: `countMatching(entries, "debug", "bootstrap", msg)` became `RecordsWith("bootstrap", msg).AtExactLevel(slog.LevelDebug)` (component + message + exact level, unchanged); the `for … if e.level == "warn"` no-WARN sweeps became `RecordsAtExactLevel(slog.LevelWarn)` loops over the same set; `recordingBarrierLogger`, which filtered to WARN inside `Handle`, became `barrierLog.warns()` filtering at query time with the same `"<component> | <message>"` rendering, so the WARN-count assertions across `portal_saver_test.go` and `portal_saver_lifecycle_events_test.go` are unchanged in meaning; `assertShowHooksWarnShape`'s hand-rolled `rec.Attrs(func…)` walk became map reads of the same four keys with the same present/absent discrimination. Bound attrs are still visible to assertions: the old `recordingSlogHandler` cloned the record and re-added `bound`, and `Sink.Handle` folds `bound` into its attr map, so the `component` binding is read identically.
- The one piece of genuinely new surface — the message-only filter at `internal/logtest/capture.go:141` — got its own unit test in the same commit, alive today as `internal/logtest/capture_test.go:407` (`TestRecords_WithMessage`), covering the across-components case, the chained level+message case, and the nil-on-no-match contract. That is the right amount: three cases for a three-line filter, nothing redundant.
- Notes: No test was renamed by this commit and no assertion was dropped — the diff is call-site-for-call-site. I did not execute the suites (reading is the mandated method here); the read gives no reason to think either lane broke, and three later phases built on these files.

CODE QUALITY:
- Project conventions: Followed. The migration lands exactly where CLAUDE.md points test capture — `logtest.Sink` constructed directly, with package-local shorthand (`barrierLog`, `migrationLog`) declared as plain helpers over a `*logtest.Sink` rather than as new handlers, which is the pattern the `logtest` row prescribes. Both helpers hold the Sink by value and only ever take its address (`slog.New(&b.sink)`), and both are only ever constructed as pointers, so no lock is copied.
- SOLID principles: Good. The change removes a parallel implementation of a declared single source of truth; each helper now does one thing (render captured records into the shape its own assertions compare).
- Complexity: Low. Every replacement is a filter chain over an existing query.
- Modern idioms: Yes — generic-free filter chaining on `Records`, `errors.Is` retained on the error-attr assertions.
- Readability: Good. `migrationReaped` keeps its `-1`-for-absent stand-in with a comment saying why it reads the map directly instead of `Record.IntAttr` (a zero would read as a real count), and `barrierLog`/`migrationLog` carry doc comments naming their rendering.
- Issues: None that clear the reporting bar. (`barrierLog := &barrierLog{}` at nine sites in `portal_saver_lifecycle_events_test.go` shadows the type name with a variable — legal Go, pre-existing naming carried over from `recordingBarrierLogger`, and a rename only; noted here, not reported as a finding.)

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

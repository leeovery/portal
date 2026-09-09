TASK: resume-hooks-silently-lost-7-28 — Inline hydrateConfig Literals And A Superseded Builder (tick-0887b8, phase 7 consolidation, severity: duplication)

ACCEPTANCE CRITERIA:
- No inline `hydrateConfig{...}` literal survives in the three hydrate suites.
- `fileMissingCfg` is deleted and its three call sites read through `hydrateCfg`.
- No `cfg.HookStore` is assigned after the builder call.
- Every converted test drives the same config values it drove before, including any deliberate zero value.
- `go test ./cmd` passes with no hydrate test renamed or weakened.

STATUS: complete

SPEC CONTEXT: This is a phase-7 consolidation task, so its authority is its own body rather than the
specification. The subject is the test-side construction of `hydrateConfig` (cmd/state_hydrate.go:32) —
the config the hydrate helper's exec chain runs against, which is the machinery the spec's resume-hook
firing path depends on (`portal state hydrate` replays scrollback, then looks up the baked `--hook-key`
in `hooks.json` and execs either the hook chain or a bare `$SHELL`). Nothing in production changed;
the task is purely about how the hydrate suites build their fixtures. The relevant production semantics
the conversion had to preserve are in `runHydrate` (cmd/state_hydrate.go:79-151): a **nil** handler makes
runHydrate return the error, a **wired** handler makes it recover-and-exec, and the two routes share an
end state — so nil is not inert and a naive conversion would silently change what a case exercises.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Builder + opts: cmd/state_hydrate_test.go:977 (`hydrateCfgOpts`), cmd/state_hydrate_test.go:995 (`hydrateCfg`)
  - New loud stand-ins: cmd/state_hydrate_test.go:924 (`unexpectedOpenFIFO`), :936 (`unexpectedTimeout`), :945 (`unexpectedFileMissing`)
  - Converted suites: cmd/state_hydrate_test.go, cmd/state_hydrate_exec_log_test.go, cmd/state_hydrate_file_missing_log_test.go
  - Commit: c44b5a90
- Notes:
  - Criterion 1 — verified by enumeration: at HEAD a repo-wide grep for `hydrateConfig{` returns exactly two
    hits, cmd/state_hydrate.go:275 (the production cobra wiring) and cmd/state_hydrate_test.go:1019 (the
    builder's own construction, handed to `clearAbsentHandler`). No inline literal survives in any hydrate suite.
  - The task's stated count of 54 was 2 too high: it counted the two builder-internal `return hydrateConfig{`
    lines (`hydrateCfg`'s own and `fileMissingCfg`'s own). At c44b5a90^ the three suites held 42 + 7 + 5 = 54
    occurrences, of which 52 were genuine call-site literals — which is exactly what the commit converted.
    Not a shortfall.
  - Criterion 2 — verified: a repo-wide grep for `fileMissingCfg` returns no hits; its three call sites
    (cmd/state_hydrate_file_missing_log_test.go:44, :72, :161) now read through `hydrateCfg`, each naming
    `OpenFIFO: openFIFOWithTimeout` and `HandleFileMissing: handleHydrateFileMissing`, which is what the
    deleted helper hardcoded.
  - Criterion 3 — verified: a grep for `cfg.HookStore =` across cmd/*_test.go returns no hits. The three
    former post-hoc assignments now sit in the opts (cmd/state_hydrate_test.go:1396, :1448, :1479).
  - Criterion 4 — verified field-by-field against the diff. The one genuine hazard was found and handled
    by the implementer rather than papered over: the pre-existing builder wired both real handlers
    unconditionally, so a naive conversion would have turned seven file-missing cases from a loud failure
    into a silent pass. The fix — an unnamed handler becomes an error-returning stand-in — preserves the
    old nil semantics at the level that matters (runHydrate propagates, the case's `t.Fatalf` fires).
    `unexpectedTimeout` deliberately does not wrap `ErrHydrateTimeout`, which keeps a stray timeout
    distinguishable from the real one; `unexpectedFileMissing` does wrap `ctx.Cause` with `%w`, which
    could in principle satisfy an `errors.Is(err, fs.ErrNotExist)` assertion — I enumerated every
    `errors.Is(err, …)` in the hydrate suites and the only two that could be reached that way
    (cmd/state_hydrate_file_missing_fallthrough_test.go:34 and
    cmd/state_hydrate_file_missing_log_test.go:268) both wire real handlers or declare the handler absent,
    so no assertion is masked.
  - Eight cases whose nil `HookStore` is load-bearing now state it explicitly (`HookStore: nil` plus a
    comment naming the bare-shell route), e.g. cmd/state_hydrate_test.go:422, :471, :1168; that is
    criterion 4's "deliberate zero value" clause honoured rather than lost to a default.
  - The one inline literal the task could not express — `TestHydrateTimeoutLog_NilHandleTimeout_NoSignalTimeoutNoExec`,
    whose subject IS a nil handler — was correctly left alone here (it is not in the three named suites) and
    was folded in later by task 8-47, which added `hydrateCfgOpts.AbsentHandler` + `clearAbsentHandler`
    (cmd/state_hydrate_test.go:989, :1037). Not a gap in this task.
  - No drift: no production file was touched by c44b5a90.

TESTS:
- Status: Adequate
- Coverage: The task is a no-behaviour-change consolidation, so the hydrate suites themselves are the
  verification and correctly no new test was added. Test-function counts are identical either side of
  the commit in every touched file (state_hydrate_test.go 55/55, exec_log 7/7, file_missing_log 7/7,
  replayed_log 8/8, timeout_log 5/5, empty_hookkey 3/3) — nothing renamed, nothing dropped.
- Notes:
  - No assertion was weakened. Filtering the commit's removed lines down to those that are not config
    fields, imports or the deleted helper leaves nothing but comments and the three `cfg.HookStore = store`
    lines — no assertion body changed.
  - The conversion left `ExecShell` defaulted (to the recording stub) on ~8 cases that previously passed
    nil because they drive a handler directly and never exec. That swaps an accidental nil-func panic for
    a silent stub, but no test's subject is "this route does not exec": every case that asserts
    `if exec.called` (cmd/state_hydrate_file_missing_fallthrough_test.go:40, :79,
    cmd/state_hydrate_file_missing_log_test.go:278, cmd/state_hydrate_timeout_log_test.go:135) passes its
    own `exec.fn()` explicitly. Nothing observable was lost, so this is noted rather than reported.
  - `go test ./cmd` was not executed (test execution is out of this reviewer's remit); the judgement above
    is from reading the diff and the HEAD state.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` introduced; no `*slog.Logger` hand-rolled (the suites
  continue to route through the package's `newCaptureLoggerForComponent`); the touched files are unit-lane
  and stay there; no production wiring changed.
- SOLID principles: Good. One builder is now the single route to a `hydrateConfig` in the tests, and the
  "which handler is absent" concern is separated into its own small step (`clearAbsentHandler`) rather
  than branching inside `hydrateCfg`.
- Complexity: Low. `hydrateCfg` is a flat sequence of nil-defaults; `clearAbsentHandler` is a three-arm
  switch with a `default` that fails loudly on an unknown value.
- Modern idioms: Yes. Options-struct-plus-builder is the right shape for a fixture with eleven varying
  parts, and the stand-ins are closures over `*testing.T` in the ordinary Go way.
- Readability: Good. The doc comments state the non-obvious reasoning — why neither handler defaults to
  its production counterpart, why `Logger`/`HookStore` do not default, and why the stand-ins are assigned
  aside rather than back into `opts`.
- Comment accuracy: Checked against the code. `hydrateCfgOpts`' claim that Stdout/Commander/ExecShell
  default to "a discarded stdout, a fresh recording commander, and a stub exec whose recordings nobody
  reads" holds — `commandertest.Quiet()` (internal/commandertest/scripted.go:80) is a `*Scripted` with a
  call log, and `stubExecShell` (cmd/state_hydrate_test.go:48) records target/args. The claim that a nil
  Logger falls through to the package logger holds via `hydrateLoggerOrDefault` (cmd/state_hydrate.go:46),
  and that a nil HookStore is the bare-shell scenario holds via `execShellOrHookAndExit`
  (cmd/state_hydrate.go:174). No comment references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

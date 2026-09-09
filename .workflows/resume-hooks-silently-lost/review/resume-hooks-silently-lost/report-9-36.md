TASK: resume-hooks-silently-lost-9-36 — "The Shared Commander Fake's Headline Safety Property Is Opted Out Of At ~90% Of Its Call Sites" (tick-e4d5a5)

ACCEPTANCE CRITERIA:
- `internal/tmux` and `internal/restore` have no `Quiet` site that has not been considered; each surviving one states what it opts out of.
- `internalMockCommander` is deleted and its consumers drive the shared fake.
- An unscripted argv in a converted site reports through the `TestingT` and returns an error, rather than answering `("", nil)`.
- Any test found to assert on an argv the production path never issues is named and resolved rather than papered over with a catch-all.
- `FromFunc` sites are unchanged.

STATUS: complete

SPEC CONTEXT: None — this is a phase-9 implementation-analysis task. The specification
(`.workflows/resume-hooks-silently-lost/specification/.../specification.md`) never mentions `commandertest`
or test-fake loudness; per the shared verifier context, a phase 6–10 task's authority is its own body. The
governing project convention is CLAUDE.md's `commandertest` row: "an ordered argv-pattern script with a
recorded call log, a deliberately loud unmatched-argv default (opt out per construction site via
`Quiet`/`AllowingUnmatched`/`DelegatingTo`), `FromFunc` for a fake that models tmux rather than scripting argv".
The task closes the gap between that stated default and two packages that took the quiet opt-out everywhere.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `b32302b6`; 17 files, all `_test.go`. Principal sites:
  - `internal/tmux/option_discriminator_internal_test.go:11-56` — the hand-rolled `internalMockCommander`
    is deleted; both fixtures now drive `commandertest.New(t, commandertest.Fails(…, "show-option", "-sv", "@foo"))`
    (lines 15, 35), scripting the full three-element argv `GetServerOption` composes (`internal/tmux/tmux.go:352`).
  - `internal/tmux/tmux_test.go` (115 conversions), `saver_pane_pid_test.go` (10), `hooks_test.go` (8),
    `clients_test.go`/`errors_test.go`/`pane_hook_rows_test.go` (6 each), `pane_option_test.go` (3),
    `session_name_test.go` (5), `target_type_test.go` (2), `version_test.go` (2),
    `exact_session_target_test.go` (1, driving 8 table routes).
  - `internal/restore/session_geometry_test.go:69-78` — new `geometryFake(t, entries…)` helper scripting
    `select-layout` + `select-pane`, with a zoomed fixture adding `resize-pane` itself (lines 123, 229, 251, 274);
    `session_geometry_summary_test.go` (5) and `session_markers_test.go` (8) converted alongside.
- Notes:
  - Verified independently at `b32302b6^`: 186 `commandertest.Quiet` occurrences in the two packages
    (164 `internal/tmux` + 22 `internal/restore`). At HEAD: zero, in either package. No
    `AllowingUnmatched`, `DelegatingTo` or `commandertest.Any` occurrence survives there either, so the
    conversion did not relocate the opt-out into a different spelling. Criterion 1 holds vacuously —
    nothing survived that needed a justification comment.
  - `grep` for `internalMockCommander` across the tree returns only historical workflow records; no Go
    source references it. `RunRaw(args ...string)` is implemented by hand in exactly two remaining test
    files, both outside this task's scope (`cmd/state_daemon_run_test.go:74`, the CLAUDE.md-sanctioned
    `daemonFakeCommander`, and `cmd/bootstrap/transient_listpanes_helpers_integration_test.go:195`).
    Criterion 2 holds.
  - `FromFunc` is untouched: `git show b32302b6 | grep -E "^[+-].*FromFunc"` returns nothing. Criterion 5 holds.
  - Scripted prefixes were spot-checked against the production argv rather than taken on trust:
    `ServerRunning` → `info` (`tmux.go:82`), `StartServer` → `new-session` (`tmux.go:199`),
    `AppendGlobalHook` → `set-hook -ga` (`tmux.go:722`), `UnsetGlobalHookAt` → `set-hook -gu` (`tmux.go:850`),
    `CheckTmuxVersion` → `-V` (`version.go:83`). `ArgvPrefix` compares argv elements, not string prefixes, so
    `show-option` cannot shadow `show-options` and the two `set-hook` flags are discriminated at two words.
  - Criterion 4: the executor's first pass reported "zero found", and the fix round
    (`fix-tracking-resume-hooks-silently-lost-9-36.md`) correctly rejected that as under-reporting. It named
    four fixtures that *scripted* an argv their subject must never issue — the rename-refusal sites, whose
    whole point is that `RenameSession` validates before composing the argv — and proved by mutation that two
    of them stayed green against a broken implementation. All four are fixed at HEAD to the empty-script form
    `commandertest.New(t)` (`internal/tmux/session_name_test.go:17`, `:30`, `:43`, `:257`), and the live entry
    at `:54` is correctly retained. Criterion 4 holds.
  - Table-driven conversions take the command from the table row rather than restating it
    (`exact_session_target_test.go:105` uses the pre-existing `route.command`; `target_type_test.go:118`
    uses `route.want[0]`), so a route added to either table carries its own script.

TESTS:
- Status: Adequate
- Coverage: The conversion is itself the strengthening — every converted fixture now fails its test on an argv
  the subject was not expected to issue, where previously it was answered `("", nil)`. Two new guards pin the
  loud default as reached through each package's own constructor:
  `internal/tmux/commander_fake_loudness_test.go:15` (drives `KillSession` against a `list-sessions`-only
  script; asserts a non-nil error, exactly one report, and that the report names `kill-session`) and
  `internal/restore/commander_fake_loudness_test.go:17` (drives `ApplyWindowGeometry`; asserts at least one
  report naming `select-layout` — the looser count is right, since `applyLayoutWithFallback`
  (`internal/restore/session.go:190`) retries and then `select-pane` follows, so the exact count is not a
  stable property).
- Notes:
  - Both guards report through `harnesstest.Recorder` rather than failing the enclosing test — the correct
    stand-in for a helper whose own failure path is the subject, and single-goroutine here, so the recorder's
    documented non-concurrency is respected.
  - Not under-tested: the risk this conversion carries is a fixture whose script is narrower than the path's
    real argv set, and that failure mode is self-reporting (the test goes red). I checked the one case where
    a narrower script could have been wrong on a live path — `geometryFake(t)` without a `resize-pane` entry
    in `session_geometry_summary_test.go:228` — and it is correct: window 1 is zoomed but has an empty live
    group, so the zoom is skipped, and the fixture would now *report* a regression that issued it.
  - Not over-tested: the two new guards partly restate
    `internal/commandertest/scripted_test.go:128-140` ("reporting the argv when no default is stated"), but
    the plan's Tests section names them explicitly and each routes the property through its own package's
    client, so they are the coverage that was asked for rather than surplus.

CODE QUALITY:
- Project conventions: Followed. Unit lane only (no build tags, no tmux server, no binary built) — correct
  for fake-only fixtures. No `t.Parallel()` introduced. The `commandertest` vocabulary is used as CLAUDE.md
  describes it, and the closed log/attr vocabularies are untouched.
- SOLID principles: Good. `geometryFake` gives the restore suite one place to state what
  `ApplyWindowGeometry` issues for an unzoomed window, with the zoom entry supplied by the fixtures that
  genuinely zoom — the variadic tail keeps the helper from becoming a switch over fixture shapes.
- Complexity: Low. Mechanical substitution plus one helper.
- Modern idioms: Yes. `commandertest.Fails`/`Returns` replace the older `When(Any, …)` form at every
  converted site; the two remaining `When` uses (`tmux_test.go:74`, `version_test.go:212`) are the correct
  choice for a table row carrying both an output and an error.
- Readability: Good. `geometryFake`'s doc comment states what it scripts and why a zoomed fixture adds its
  own entry; both loudness guards carry a comment scoping the claim to the fixture *form*.
- Issues: None rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

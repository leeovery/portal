TASK: resume-hooks-silently-lost-3-8 — The Reboot Fixture Helpers Get One Home In internal/restoretest

ACCEPTANCE CRITERIA:
- The four helpers exist once each, in `internal/restoretest`, and both packages call them
- The marker bracket sits in an untagged file, and `go test ./internal/restore/` (unit lane, untagged) still compiles and passes
- No failure path moves: the marker-unset error is reported exactly as it is today on each side
- The `seedScrollback` / `seedPaneScrollback` pair is subsumed rather than left beside the new helper
- No assertion changes and no test changes its verdict
- `go test ./...` passes; `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT: This is a phase-3 consolidation task, not specified behaviour — its authority is its own body. The
surrounding feature work (token-keyed resume hooks surviving a reboot) is proven by real-reboot integration fixtures
in two packages (`cmd`'s non-contiguous-window reboot, `internal/restore`'s rename-reboot / multipane families). Task
3-5 re-authored four fixture helpers in `cmd` that already existed in `internal/restore`'s test files; neither package
can reach the other's test file, so the shared home has to be `internal/restoretest`. The load-bearing constraint is
the build tag: `internal/restore/integration_test.go` is untagged, so anything it calls must compile in the unit lane.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restoretest/restore_marker.go:20` — `RestoreWithMarker` (untagged file, no `//go:build` line)
  - `internal/restoretest/scrollback.go:18` — `SeedScrollback(t, stateDir, session, window, pane, payload)`
  - `internal/restoretest/sessions_json.go:70` — exported `WriteIndex(t, stateDir, state.Index)`
  - `internal/restoretest/sessions_json.go:83` — `FindCapturedSession(t, idx, name)`
  - `cmd` call sites: `cmd/noncontiguous_window_reboot_integration_test.go:320` (FindCapturedSession), `:335`
    (SeedScrollback), `:353` (WriteIndex); the marker bracket is reached through
    `internal/restoretest/reboot.go:57` (`RestoreFromState` → `RestoreWithMarker`), called at `:364` of the same file
  - `internal/restore` call sites: `integration_test.go:73`, `armed_restore_integration_test.go:57,131`,
    `exit_closes_pane_integration_test.go:154`, `integration_full_test.go:112` (RestoreWithMarker);
    `reboot_fixture_test.go:126` (FindCapturedSession), `:144` (SeedScrollback), `:146` (WriteIndex);
    `rename_reboot_durability_integration_test.go:41` (FindCapturedSession)
- Notes:
  - Every criterion holds in the current tree. Each helper is declared exactly once: greps for the old names
    (`restoreWithMarker`, `findCapturedSession`, `seedScrollback`, `seedPaneScrollback`, `persistIndex`,
    `divergentSeedScrollback`, the unexported `writeIndex`) return no declarations anywhere — the
    `seedScrollback` / `seedPaneScrollback` pair is genuinely subsumed, not left beside `SeedScrollback`.
  - The marker bracket's untagged home is correct and its doc comment's justification is true:
    `internal/restore/integration_test.go` states no build constraint (verified: line 1 is `package restore_test`)
    and calls `restoretest.RestoreWithMarker` at line 73, so the unit lane must compile it.
  - Unit-lane compilability checked by reading rather than running: every untagged file that names a `restoretest`
    symbol as code (`internal/restore/{integration,restore,restore_progress,session,session_markers}_test.go`,
    `cmd/bootstrap/orchestrator_builder_eager_default_test.go`, `internal/restoretest`'s own untagged tests) reaches
    only untagged declarations — `RestoreWithMarker`, `OpenTestLogger`, `NewFakeExeOrchestrator`,
    `SeedSessionsJSON*`. The two apparent exceptions (`internal/restoretest/literal_guard_test.go:72,73,142`,
    `internal/portalbintest/lane_guard_rule_test.go:27`) are raw-string guard fixtures and a comment, not calls.
  - The single sanctioned behaviour move — the `cmd` fixture's marker-unset error going from returned-and-fatal to
    `t.Logf` — is exactly what the task's Do bullet prescribed, and the fix-tracking record confirms the
    orchestrator accepted it. It costs no coverage: `cmd/noncontiguous_window_reboot_integration_test.go:182-191`
    fails the run if `@portal-restoring` is still set after the restore, which is the observable state a failed
    unset leaves behind.
  - Payload bytes are preserved at every migrated call site (`"before reboot: <role>\n"` for the `cmd` fixture,
    the ANSI body for the rename/multipane fixtures, now the shared `restoretest.ANSIScrollback` const), so no
    fixture's inputs changed. The only diagnostic change is `SeedScrollback` naming the path in its write failure —
    a superset of the old message, no assertion.
  - `WriteIndex` taking the whole `state.Index` rather than `(savedAt, sessions)` is the right call: the `cmd` and
    `reboot_fixture` sites round-trip a captured index, which the field form cannot express, and `state.EncodeIndex`
    copies the struct before canonicalising.

TESTS:
- Status: Adequate
- Coverage: No new tests, as the task directed — these are fixture helpers whose failure modes fire in every caller.
  The named regression set is intact and calls the shared helpers: `TestNonContiguousWindowReboot_KeepsTokenKeyedHooks`
  (`cmd`), the rename-reboot family and `TestMultiPaneLegacy_*` (`internal/restore`), and the untagged
  `internal/restore/integration_test.go` suite. `internal/restoretest/restore_marker_test.go` (added by later work,
  not this task) now covers the bracket directly.
- Notes: Judged by reading, not execution. No assertion in any migrated file changed; the substitutions are
  argument-for-argument equivalent to the code they replaced.

CODE QUALITY:
- Project conventions: Followed. Test-only package, no production importer (verified: no non-test file imports
  `internal/restoretest`). Build-tag lane discipline is respected, and `doc.go`'s rule was corrected in the same work
  to state the criterion that actually governs (callers' lanes, not dependency surface) — a rule the package's
  current tag layout satisfies.
- SOLID principles: Good — each helper does one thing and takes what varies (session name, payload, whole index) as
  a parameter rather than hardcoding it.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Each helper's doc says why it exists rather than restating its body, and
  `restore_marker.go:18-19` records the tag constraint at the place a future edit would break it.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

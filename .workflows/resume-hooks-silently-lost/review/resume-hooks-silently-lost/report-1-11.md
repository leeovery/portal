TASK: resume-hooks-silently-lost-1-11 — Remove Inert Test Scaffolding And A Subsumed Assertion (tick-755c47)

ACCEPTANCE CRITERIA:
- No `logtest.Sink` is bound to a name it never reads
- No assertion is removed whose failure the surviving assertions would not also catch
- Every affected test keeps its current name and cases
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
The task is a dead-code cleanup inside the phase-1 test surface of the stale-hook work. The two behaviours the affected
tests cover are spec-governed: the daemon's throttled sweep must swallow a failing clean while still advancing its
throttle anchor, and `checkStaleHooks` must report *not evaluable* — never a count — while `@portal-restoring` is set
(specification.md:312, and Corrigendum 2026-09-01 at :552 for the rendered phrases). The task removes only scaffolding
and one subsumed assertion, so it neither adds nor withdraws coverage of either behaviour.

IMPLEMENTATION:
- Status: Implemented (commit a58af87d, deletion-only; no production source touched)
- Location:
  - cmd/state_daemon_hook_cleanup_test.go — the two `hooksSink := &logtest.Sink{}` / `log.SetTestHandler(t, hooksSink)`
    pairs deleted (then at :128-129 and :154-155; the tests are now at :120 and :150 after the later rename in 9-6)
  - cmd/doctor_test.go — the `strings.Contains(got.detail, "stale hook entr")` block deleted from the
    "it reports not evaluable while the restore marker is set" subtest (the `assertRestoreWindowResult(t, got)` call
    now at :1069)
- Notes:
  - The Do list allowed an unnamed sink where the install was *silencing* rather than capturing. Deleting outright was
    the right call and I verified the premise against the tree as it stood at that commit rather than taking it on
    trust: `runHookStaleCleanup` (cmd/run_hook_stale_cleanup.go at a58af87d) reached `hooksLogger` only on the
    restore-window stand-down (DEBUG) and the empty-pane-read guard (WARN), neither of which is on either test's path —
    the first failed at `store.Load` and the second at `ListAllPaneHookKeys`, both reported on the *injected* daemon
    logger — and `hooks.Store.Load` at that commit emitted nothing at all. So neither install captured anything and
    neither silenced anything.
  - The third install was preserved as instructed and is still read: cmd/state_daemon_hook_cleanup_test.go:215
    (`hooksSink := logtest.Install(t)`, the form a later task moved it to) feeding the assertions at :234-240.
  - Neither removal orphaned an import: `internal/log` and `internal/logtest` remained in use in
    state_daemon_hook_cleanup_test.go via the surviving install, and `strings` has ~20 other uses in doctor_test.go,
    so the package still compiles — which is what carries the `go test ./...` criterion for a deletion-only change.
  - The later HEAD divergence (the sweep moved to `internal/hooksweep`, which now emits the cycle's WARNs on the
    `hooks` component; the first test renamed to `_SwallowsCleanupError`) is task 9-6/9-12's work, not this task's.

TESTS:
- Status: Adequate (removals only; the task adds no code that needs covering)
- Coverage: Every behaviour the deleted lines touched is still asserted. In doctor_test.go,
  `assertRestoreWindowResult` (cmd/doctor_test.go:1162-1170) compares `got.detail` for **exact** equality against the
  restore-window phrase, so any detail containing "stale hook entr" fails there first — the deleted `strings.Contains`
  could not fire independently. The stronger structural claim (no count is even computed) is separately covered by the
  "it reads the marker before counting" subtest's `lister.calls != 0` check (cmd/doctor_test.go:1097-1105). In
  state_daemon_hook_cleanup_test.go, both affected tests keep their whole assertion set — daemon-logger silence via the
  read `sink.Body()`, the store's post-run contents, and the advanced `lastCleanup` anchor.
- Notes: No over-testing introduced; the change strictly shrinks inert surface.

CODE QUALITY:
- Project conventions: Followed. The surviving silence/capture idiom matches the package norm (`logtest.Install(t)`,
  named when read and unnamed when only installed — e.g. cmd/open_theme_commit_test.go:74).
- SOLID principles: N/A (test-only deletion)
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the two tests now read as "the daemon logger is the only thing under observation", which is what
  they assert.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

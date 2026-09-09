TASK: resume-hooks-silently-lost-6-16 — Poll The Hook-Fire Marker Assertions To A Deadline
(tick-8e7227, status done, severity high, sources: bank)

ACCEPTANCE CRITERIA:
- One polled helper serves the hook-fire and multipane marker assertions.
- No marker assertion reads the file exactly once.
- The absent-file case does not fail before the deadline.
- `TestRenameRebootHook_PaneProcessKeptRunning`, `TestRenameRebootHook_ExternalRename`,
  `TestRenameRebootHook_DurableAcrossRepeatedReboots` and `TestMultiPaneLegacy_PerPaneHookRouting`
  pass 5/5 under `-count=5`.

STATUS: complete

SPEC CONTEXT:
This is a phase-6 (implementation-analysis) task, so its authority is its own body rather than the
specification — per the shared verifier context. The property it protects is the work unit's headline
guarantee: a resume hook keyed on the pane's durable `@portal-pane-id` token fires after a reboot,
across a session rename. The race it names is real and confirmed against production ordering:
`cmd/state_hydrate.go:145` calls `unsetSkeletonMarkerOrLog(cfg)` and only then, at
`cmd/state_hydrate.go:149`, `execShellOrHookAndExit(cfg)` — so the skeleton-marker clear that
`restoretest.WaitForSkeletonMarkersCleared` waits on genuinely precedes the shell that runs the hook,
and a bare read taken when that wait returns races it.

IMPLEMENTATION:
- Status: Implemented (and subsequently promoted; see Notes)
- Location:
  - `internal/restoretest/marker_count.go:43` — `AssertMarkerCount(t harnesstest.TestingT, path, marker string, want int)`,
    the single polled helper.
  - `internal/restoretest/marker_count.go:62` `run` (verdict + diagnostics),
    `:82` `wait` (deadline loop), `:100` `read` (ENOENT → count 0).
  - `internal/restoretest/marker_count.go:20-21` — `HydrateBudget` / `HydrateTick`, the same
    bound and tick `WaitForSkeletonMarkersCleared` is driven with at
    `internal/restore/reboot_fixture_test.go:163`.
  - Call sites, all routed through the one helper:
    `internal/restore/rename_reboot_hook_integration_test.go:89`,
    `internal/restore/rename_reboot_durability_integration_test.go:33` and `:65`,
    `internal/restore/multipane_legacy_integration_test.go:51-54`,
    plus two later adopters outside this task's change-set
    (`cmd/bootstrap/reboot_roundtrip_test.go:157`, `cmd/noncontiguous_window_reboot_integration_test.go:404`).
- Notes:
  - The task's own commit (`f13e43bb`) landed the helper as `assertMarkerCount` in
    `internal/restore/marker_assert_test.go` with a package-local `markerReporter` stand-in and a
    meta-test in `internal/restore/marker_assert_meta_test.go`. Later tasks in this same plan moved it
    verbatim into `internal/restoretest` (`e9b27db7`, task 7-8), folded the stand-in onto
    `harnesstest.TestingT`/`Recorder` (`700a700a`, task 8-24), and renamed the budgets (`4b9bbad6`,
    task 8-36). Nothing of this task's substance was lost in those moves — the deadline loop, the
    ENOENT-as-zero rule, the append-only early-exit and all three diagnostics survive unchanged.
  - Do-list item 3 (preserve both diagnostics) is met verbatim: the bare-shell-miss hint at
    `marker_count.go:70-71` and the CROSS-FIRE message at `:73-74`. The switch ordering is correct —
    `got == want` returns first, so a want-0 satisfied assertion never falls into the `got == 0`
    error arm, and a want-0 with `got > 0` reaches CROSS-FIRE rather than the generic arm.
  - Do-list item 4's parenthetical ("`assertMarkerAbsent` must still poll, then assert absence at the
    deadline, not return early") is honoured by the `(got == a.want && a.want > 0)` conjunct at
    `marker_count.go:91`: only a want above zero can short-circuit, so a want-0 assertion always runs
    the budget out. A cross-fire still fails as soon as it appears via the `got > a.want` arm.
  - Criterion 4 (`-count=5` × 4 tests) is not verifiable by reading and I did not execute the suite;
    the mechanism the criterion exists to protect is present and correct, which is what I can assert.

TESTS:
- Status: Adequate
- Coverage (`internal/restoretest/marker_count_test.go`):
  - `:59` marker arriving after the assertion starts — the flake itself, and the case the old bare
    `os.ReadFile` failed.
  - `:74` never appears — fails, and only at the budget (asserts `elapsed >= probeBudget`).
  - `:90` fires more often than wanted — fails, and *before* the budget (asserts the overshoot is final).
  - `:109` want-0 fails when the marker is present — the task's named "absence assertion must not
    become vacuous" test.
  - `:123` want-0 waits the budget out — the "must not return early" property.
  - `:140` absent file is a count of 0, not a `Fatalf` — the exact ENOENT regression the task names,
    asserted on `rep.Fatals` specifically rather than on `Failed()` alone.
  - `:163` the exported entry point uses the shared budget — a contrast pair over one delay value
    (`pastProbeBudget`), so "it passed" is a statement about which budget was used and not about the
    delay being short. Without the negative half the positive case would pass under any budget.
  - The tests drive the unexported `markerAssertion` at a 400ms probe budget rather than the 10s
    production one, so the whole suite is unit-lane and costs ~1.5s of real waiting — the right trade.
- Notes:
  - `:154 TestAssertMarkerCount_WriterFailureIsReportedNotPanicked` asserts on the test file's own
    `writeMarkerAfter` fixture rather than on the subject. It is defensible (a goroutine that panicked
    on a write error would take the binary down), but it is the one case here whose subject is
    scaffolding. Not reported as a finding — nothing is wrong with it.
  - `:191` ("it fails when more markers fire than expected") overlaps `:90` in subject, differing only
    in going through the exported wrapper. Cheap, returns immediately, and pins the wrapper's verdict
    path; not redundant enough to call over-tested.
  - No leftover single-read marker assertion exists in the affected packages: grepping
    `internal/restore/` and `internal/restoretest/` for `os.ReadFile`/`strings.Count` turns up only
    `armed_restore_integration_test.go:80` (a scrollback assertion, not a marker file) and
    `restoretest`'s own logger/sessions-json fixture tests.

CODE QUALITY:
- Project conventions: Followed. `restoretest` is a test-only helper package and the helper takes
  `harnesstest.TestingT` — the single declaration of that subset per CLAUDE.md — rather than declaring
  a local stand-in. `marker_count.go` carries no build tag, correctly: it builds no portal binary and
  spawns no daemon, so the lane rule places its tests in the fast lane. Its integration-tagged callers
  are unaffected.
- SOLID principles: Good. `markerAssertion` separates the three concerns cleanly — `read` (one
  observation), `wait` (the deadline loop and its termination rule), `run` (the verdict and its
  wording). The budget/tick are struct fields rather than package constants read inside, which is
  exactly what lets the tests drive it fast without a second code path.
- Complexity: Low. One loop, one four-arm switch, one error discrimination.
- Modern idioms: Yes. `os.IsNotExist` on the read error, a value receiver on an immutable descriptor,
  `!time.Now().Before(deadline)` rather than an `After`/equality mix.
- Readability: Good. The two load-bearing subtleties both carry the reason rather than the mechanism:
  `marker_count.go:88-90` explains why a count at-or-past `want` is final (append-only files) and why
  `want == 0` is exempt, and `:41-42` explains why a want of 0 must not return early.
- Comment accuracy: Verified against the code and against production. `marker_count.go:36-38`'s claim
  that "a pane's helper clears its skeleton marker before it execs the hook" holds at
  `cmd/state_hydrate.go:145` vs `:149`. `:33-35`'s "a count short of want fails at the hydrate budget;
  a count past it fails at once" matches the two arms of the `:91` condition. `HydrateBudget`'s
  "10 * time.Second" matches the `WaitForSkeletonMarkersCleared` call it claims to share a bound with.
  No process-artifact references (no task ids, phases or spec sections) anywhere in the changed code.
- Security: N/A — test-only helper, no external input.
- Performance: The want-0 assertions each pay the full `HydrateBudget`, so
  `TestMultiPaneLegacy_PerPaneHookRouting`'s two absence checks
  (`multipane_legacy_integration_test.go:53-54`) add ~20s to that integration test. This is the
  deliberate, plan-mandated cost of a non-vacuous absence assertion, and it is confined to the
  integration lane. Not a finding.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

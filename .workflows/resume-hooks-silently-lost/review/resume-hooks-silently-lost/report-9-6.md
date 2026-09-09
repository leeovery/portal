TASK: resume-hooks-silently-lost-9-6 — One Sweep Cycle's Log Lines Are Split Across Three Components By An Injected Logger

ACCEPTANCE CRITERIA:
- One sweep cycle driven from the daemon emits every line — counts, reaped count, stand-downs and failures — with `component=hooks`.
- The same cycle driven from `portal doctor --fix` emits the identical component and messages; only the caller's rendered stdout differs.
- No `daemon:` or `bootstrap:` line describes the sweep's internals any more; each caller keeps only its own cycle-summary output.
- A sweep that fails for an unclassified reason produces exactly one WARN, under `hooks`, and no second line from either caller.
- `runHookStaleCleanup` takes no logger argument.

STATUS: complete

SPEC CONTEXT:
The specification governs the `hooks` component's emission vocabulary for this work unit: §5.4/§6.3 place the sweep's stand-down (`op=clean-stale-skipped`) and its degraded read (`op=load-unlocked`) under the existing `hooks` component, and §5.4/§6.4 state the amendment is three `op` values, two `via` values and no new component or attr key. This task is a phase-9 implementation-analysis task, so its authority is its own body, but the direction it takes — collapsing the cycle's counts and failures onto `hooks` rather than onto a caller's component — is exactly what the spec's "no new component binding is needed" posture implies, and it invents nothing outside the closed 18-component vocabulary (verified: the only `log.For("hooks")` bindings are `internal/hooks/store.go:21`, `internal/hooksweep/sweep.go:14` and `cmd/state_common.go:11`).

IMPLEMENTATION:
- Status: Implemented (delivered at commit 32ebd089, then relocated verbatim by the later task 9-12, `internal/hooksweep`; the code is the source of truth and the substance survives the move intact).
- Location:
  - `internal/hooksweep/sweep.go:14` — `var logger = log.For("hooks")`, the single binding the whole cycle emits through.
  - `internal/hooksweep/sweep.go:21-23` — `countsMsg` / `removedMsg` / `sweepFailedMsg`, the cycle's three own messages.
  - `internal/hooksweep/sweep.go:135,141` — the two DEBUG counts lines, now on `logger`.
  - `internal/hooksweep/sweep.go:152` — `func Run(reader Reader, store *hooks.Store) (Outcome, error)`: no logger parameter.
  - `internal/hooksweep/sweep.go:164` — the reaped-count DEBUG line, on `logger`.
  - `internal/hooksweep/sweep.go:192` — `logger.Warn(sweepFailedMsg, "error", err)`, the unclassified-failure line moved into the cycle.
  - `internal/hooksweep/standdown.go:35` — `StandDown.emit()` on the same package `logger`.
  - `cmd/state_daemon.go:214` — `_, _ = hooksweep.Run(deps.Client, deps.HookStore)`; the `daemon: hooks stale-cleanup failed` WARN is gone.
  - `cmd/doctor.go:201-208` — `hooksweep.Run(deps.HookLister, deps.HookStore)`; the `bootstrap: doctor --fix: stale-hook prune failed` WARN is gone, the caller-facing stdout line is retained.
- Notes:
  - Every acceptance criterion holds. `countsLogger` and `countsOrDefault` are gone from the tree (repo-wide grep for `countsLogger|countsOrDefault|runHookStaleCleanup` returns nothing), and the two removed WARN wordings return no matches either.
  - Two deliberate divergences from the task's Do list, both sound:
    (a) The doc comment naming the one component now lives on the package doc (`internal/hooksweep/reason.go`, the `package hooksweep` comment) and on the `logger` var (`sweep.go:12-14`) rather than on `Run` itself — a consequence of 9-12 moving the function into a package whose whole purpose is that cycle. The property the criterion asks for (a reader learns the component from the source that owns it) is stated more strongly than a function comment would state it.
    (b) The doctor caller's retained user-facing line is `reportFailedPrune(w)` (`cmd/doctor.go:274-276`, phrase `the sweep could not complete`) rather than `reportSkippedPrune(w, skipReasonSweepFailed)`. A later task removed the sweep-failed pseudo-reason from the closed `Reason` type — correct, since the cycle declined nothing — and gave the failed sweep its own renderer. The user-facing line is still rendered, which is what the criterion required.
  - `cmd/state_daemon.go:399` (`load hook store failed; hooks stale-cleanup disabled`) and `:228` (`projects stale-cleanup failed`) remain under `daemon`. Neither is the sweep's internals: the first reports the daemon failing to wire its own dependency at startup (no cycle ran), the second is the separate `projects.json` prune, which has no equivalent owning package. Criterion 3 is not violated by either.
  - The daemon's caller comment (`cmd/state_daemon.go:211-213`) and `pruneDoctorStaleHooks`'s (`cmd/doctor.go:192-195`) both hold true against the code they sit on.

TESTS:
- Status: Adequate.
- Coverage:
  - `internal/hooksweep/sweep_move_test.go:137-171` — "it emits the whole cycle under the hooks component from the new package's own binding": drives a reaping cycle, a stand-down cycle and a failing cycle, asserts at least one record for each of `countsMsg`/`removedMsg`/`standDownMsg`/`sweepFailedMsg`, then asserts *every* captured record carries `component=hooks`. This is the task's first named test, carried across the 9-12 move; it is stronger than the original because the final loop admits no record under another component.
  - `cmd/hook_prune_one_component_test.go:44-63` — "it emits the same component and messages from the daemon and from doctor --fix": runs `maybeRunHookCleanup` and `pruneDoctorStaleHooks` over identically-seeded stores and compares the ordered `component message` pairs with `slices.Equal`, with an explicit non-vacuity guard (`len(daemonLines) == 0` fatals) so the comparison cannot pass by both sides emitting nothing.
  - `cmd/hook_prune_one_component_test.go:65-83` — "it emits exactly one record for an unclassified sweep failure": daemon-driven, `Records().AtOrAboveLevel(WARN).Only(...)`, asserting component, message and a non-nil `error` attr. The `Only` terminal is what makes "exactly one" real rather than "at least one".
  - `cmd/hook_prune_one_component_test.go:85-107` — "it still renders the caller-facing skipped-prune line for a failed sweep": byte-exact stdout assertion plus the same single-WARN check, so the caller's rendered output and the absence of a second log line are pinned together.
  - Counter-coverage on the daemon side: `cmd/state_daemon_hook_cleanup_test.go:120-148` and `:150-181` install a `daemon`-bound capture logger and fail on any non-empty body, so a reintroduced `daemon:` line for a swept failure or a swallowed list error breaks a test rather than passing silently.
  - `cmd/doctor_fix_hook_prune_report_test.go:45-64` pins the doctor path's rendered failed-sweep line beside exactly one `hooks`/`sweepFailedMsg` WARN.
  - `cmd/hook_sweep_caller_guard_test.go` keeps the bootstrap package out of the sweep entirely (`\bCleanStale\b|\bhooksweep\b` over `cmd/bootstrap`'s production files, with a scanned-nothing tripwire), which is what stops the `bootstrap` attribution returning by a different route.
- Notes:
  - Not over-tested. The package-level test proves the component property once; the `cmd` cases add only what they can see that `hooksweep` cannot — that the two callers reach it identically and that neither adds a line of its own — and the test file says so in its own comment rather than duplicating the component assertion.
  - `cmd/hookkey_vocabulary_test.go:218-221` restates `standDownMsg` and `sweepFailedMsg` as literals rather than importing them from `hooksweep` (they are unexported there). The duplication is deliberate and its comment states why: a `cmd` suite asserting on the operator-visible message should fail when the cycle re-words it, not agree with the rename. That is the right call for an observable-string pin.
  - No test executes the suite here; adequacy was judged by reading. Each named test's assertion would fail if the behaviour it names regressed — the component checks read the attr off every record, and the single-WARN checks use the exactly-one terminal.

CODE QUALITY:
- Project conventions: Followed. One `log.For` binding per package; no new component or attr key; the closed `Reason` vocabulary is untouched by this change; the lane rules are respected (all the tests above are unit-lane and touch no tmux server or binary).
- SOLID principles: Good. Removing the injected logger moves emission to the module that owns the behaviour, and the callers are left with only the rendering their own surface owes — the `Outcome`/`Reason` pair is the whole of what crosses the boundary.
- Complexity: Low. The change is net-negative in code: a parameter, a defaulting helper and two caller WARNs removed for one `logger.Warn`.
- Modern idioms: Yes.
- Readability: Good. `declinedSweep`'s comment now says why the unclassified failure is both logged and returned ("the error is what drives a caller's own rendered output, not a second log line"), which is the one thing a reader would otherwise ask.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

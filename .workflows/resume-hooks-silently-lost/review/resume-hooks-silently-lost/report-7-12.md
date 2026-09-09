TASK: resume-hooks-silently-lost-7-12 — "The Sweep's Stand-Down Reporting Reports One Condition Twice And Takes A Logger That Governs Half Its Output" (phase 7, implementation-analysis; severity low; sources: standards, architecture)

ACCEPTANCE CRITERIA:
1. An unreadable `hooks.json` yields exactly one WARN for the event across the whole call chain, on both the daemon and `doctor --fix` paths.
2. `doctor --fix` still prints the `Skipped stale hook prune:` line for a store-read stand-down, and its exit code stays driven solely by the post-repair diagnosis.
3. The daemon logs no `hooks stale-cleanup failed` line for a store-read stand-down.
4. `runHookStaleCleanup`'s signature either has no logger parameter or has one whose name and doc match the two DEBUG lines it governs.
5. The lock-timeout, restore, empty-pane-read and pane-read-failed branches are unchanged.

STATUS: complete

SPEC CONTEXT:
This is a phase-7 task, so its authority is its own body rather than the specification; the spec is nonetheless aligned. Specification §5.4 (line 310) states the sweep declines for six named reasons under one line shape (`op=clean-stale-skipped`, `via=internal`, `reason=<slug>`), with the stated purpose that "an operator raising the level because a hook vanished needs one grep to answer whether the prune stood down and why, rather than reading three indistinguishable lines by eye". Collapsing the store-read condition from two WARNs (the sweep's own plus a caller's generic failure line) to one is exactly that intent. `store-read-failed` is one of the six named reasons and remains on the outcome.

IMPLEMENTATION:
- Status: Implemented (delivered at 01e3d74f; subsequently re-homed by phase-9 task 9-12 into `internal/hooksweep`, where the substance survives intact)
- Location:
  - `internal/hooksweep/sweep.go:186-190` — the `hooks.ErrStoreRead` arm of `declinedSweep` returns `standDownOutcome(declineWarn(ReasonStoreReadFailed, "error", err))`, i.e. the WARN plus a nil error, exactly as the `ErrLockHeld` arm above it (`:181-185`) does. AC1's mechanism.
  - `internal/hooksweep/sweep.go:196-202` — `standDownOutcome` is now the single home of "emit the line, return the reason, return nil"; its comment names the nil error as the point ("the emitted line is the whole report, so a caller cannot add a second one for the same event").
  - `internal/hooksweep/sweep.go:152` — `func Run(reader Reader, store *hooks.Store) (Outcome, error)`. The logger parameter is gone entirely and the counts bind to the package's own `logger = log.For("hooks")` (`:14`, emitted at `:135`, `:141`, `:164`). AC4 satisfied by the drop branch of the Do list rather than the rename branch the original commit took — a legitimate later consolidation, and the stronger of the two options the task offered.
  - `cmd/doctor.go:201-211` — `pruneDoctorStaleHooks` prints `reportFailedPrune` only on a non-nil error, which store-read no longer produces, and still prints `Skipped stale hook prune: could not read hooks.json` from `outcome.DeclineReason` (`:209-211` → `:266-268` → `skippedPrunePhrases[ReasonStoreReadFailed]` = `storeReadStandDownPhrase`, `:223`). AC2's first half.
  - `cmd/doctor.go:164-177` — the `--fix` exit is driven solely by `doctorUnhealthy(postResults)` from the re-diagnosis; `runDoctorFix` returns nothing. AC2's second half.
  - `cmd/state_daemon.go:212-215` — `_, _ = hooksweep.Run(deps.Client, deps.HookStore)`; the daemon writes no line of its own on any sweep outcome. AC3.
- Notes: I confirmed the "exactly one WARN" claim end to end rather than taking it from the branch shape. The store itself emits nothing at WARN on a failed read — `internal/hooks/store.go:301` and `:329` wrap `ErrStoreRead` silently, and the only emission on that path is the DEBUG `load-unlocked` breadcrumb at `:73`. On the `doctor --fix` route the post-repair `checkStaleHooks` (`cmd/doctor.go:363-392`) logs nothing at all: it renders through `staleHooksNotEvaluable`, and `hooksweep.StalenessStandDown` / `JudgeAgainstLivePanes` construct a `StandDown` without emitting (emission is `StandDown.emit()`, called only from `standDownOutcome`). So the sweep's own line is the whole log record of the event on both routes.
  AC5 holds: in the delivered diff the other four branches are byte-unchanged, and in the current file they are behaviourally identical — restore and marker-read-failed via `StalenessStandDown` (`:71-80`), lock-timeout via `:181-185`, empty-pane-read and pane-read-failed via `JudgeAgainstLivePanes` (`:88`, `:99`) — all reaching the same emit-and-return-nil shape.
  Nothing is lost by the change: the generic `doctor --fix: stale-hook prune failed` line the user used to get is replaced by the more specific `could not read hooks.json`, and the failure itself still rides the WARN's `error` attr.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/hook_prune_single_report_test.go:19-33` — daemon path: an unreadable `hooks.json` through `maybeRunHookCleanup` yields exactly one record at-or-above WARN in the process sink (`assertStandDown`, `cmd/hookkey_vocabulary_test.go:236-254`, which terminates in `Only`), and zero records on the injected daemon logger. Because `hookCleanupDeps` (`cmd/state_daemon_hook_cleanup_test.go:25-31`) wires that logger into `daemonDeps.Logger`, the zero-record assertion is a real guard on the daemon re-adding a line, not a vacuous one. Covers AC1 (daemon) and AC3.
  - `cmd/hook_prune_single_report_test.go:35-44` — doctor `--fix` repair path through `pruneDoctorStaleHooks`, same exactly-one-WARN assertion. Covers AC1 (doctor).
  - `cmd/hook_prune_single_report_test.go:48-66` — the lock-timeout stand-down still reports exactly once and leaves `hooks.json` byte-identical, which is the AC5 regression guard for the branch the fix was modelled on.
  - `cmd/doctor_stand_down_copy_test.go:80-90` (the `hooks.json unreadable` row) drives both `hooksweep.Run` and a full `doctor --fix` for that reason: the skipped-prune line `Skipped stale hook prune: could not read hooks.json` (`:325`), the untouched file (`:326`), the post-repair not-evaluable line (`:330`), the WARN level and its `error` attr carrying `hooks.ErrStoreRead` (`:86`, `:308`). Its `:440-466` subtest pins the exit code for every reason in both directions — nil over a healthy post-repair diagnosis, `ErrDoctorUnhealthy` once a genuine check fails. This is where the task's `"it still prints the skipped-prune line…"` and `"it leaves the doctor exit code…"` subtests were consolidated by a later task; AC2 is covered more strongly there than in the original commit, since it now runs for all six reasons rather than one.
  - `internal/hooksweep/read_failure_test.go:57-91` — both of the cycle's two reads (the snapshot pre-read and the delete-phase load) classify as `ReasonStoreReadFailed` with a nil error, and a failed *save* deliberately stays on the error-returning path with no reason — the discrimination that keeps the dedup from swallowing genuine failures.
  - `cmd/hook_prune_one_component_test.go:66-107` and `cmd/doctor_fix_hook_prune_report_test.go:45-63` — the surviving error path (an unclassified sweep failure) still produces exactly one WARN, under the `hooks` component, alongside the caller's rendered line. This closes the gap attempt 2's fix-tracking notes flagged as untested on the doctor route.
  - `internal/hooksweep/sweep_test.go:289-310` — the restore-ordering oracle, repaired in the fix round: with `ErrStoreRead` no longer returning an error, "nil return proves the store was unread" stopped discriminating, so the case now asserts `DeclineReason == ReasonRestoring` and names why in its comment. The repair survives in the current tree.
  - `internal/hooksweep/sweep_move_test.go:140-173` covers what the dropped parameter used to be tested for: the whole cycle — counts, removed, stand-down and failure — emits under the `hooks` component.
- Notes: The task's `"it emits the count lines under the component the signature names"` test no longer exists, correctly: the parameter it was written for is gone, and the property that replaced it (counts under `hooks`) is pinned at `sweep_move_test.go:163-172`. I found no over-testing: the copy table's subject is the sweep's own line and the rendered copy, while the single-report file's subject is the two *callers* adding nothing over it — different failure modes, no redundant assertion.

CODE QUALITY:
- Project conventions: Followed. The change is a subtraction from the log-or-return duplication CLAUDE.md's `hooksweep` row describes ("It binds `log.For("hooks")` itself and emits the whole cycle under it … so one grep reconstructs a cycle and a caller adds no second line for the same event"), and the architecture row now matches the tree. The unit-lane rule is respected — every test named above is untagged and spawns no daemon or binary.
- SOLID principles: Good. The reporting decision sits in the one place that knows what the failure cost the cycle; callers keep only rendering.
- Complexity: Low. `declinedSweep` is a flat four-arm `switch` with a single fall-through; the two identical decline arms flagged in the fix rounds as under the Rule of Three have since collapsed onto `standDownOutcome`.
- Modern idioms: Yes. `errors.Is`/`errors.As` discrimination, sentinel errors, no reflection or type switches on error strings.
- Readability: Good. Each arm's comment states the condition it names and why it declines rather than fails.
- Issues: None. I checked the comments in the changed code against the code: `sweep.go:187-189`'s "at either of the two reads it takes" is true (`internal/hooks/store.go:301` and `:329` both wrap `ErrStoreRead`); `:198`'s claim that the nil error is what stops a second report is true of both callers as they now stand; `:150-151`'s `Run` doc matches its signature.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

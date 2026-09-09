TASK: resume-hooks-silently-lost-1-3 — The Sweep Stands Down While A Restore Is In Flight

ACCEPTANCE CRITERIA:
- The seam carries both the pane enumeration and the restore-marker read, and `*tmux.Client` satisfies it unchanged
- With the marker set, the sweep returns nil having called neither `ListAllPaneHookKeys` nor `store.Load`, and `hooks.json` is byte-identical
- With the marker read failing, the outcome is identical to the marker being set, and the record carries the read error in `error`
- The stand-down record is DEBUG under the `hooks` component with `op=clean-stale-skipped`, `via=internal`, `reason=…`; no WARN on this branch
- With the marker absent, the sweep behaves exactly as it does today
- The daemon still early-returns from `tick` on the marker, so its idle branch never reaches the new read
- `doctor` remains bootstrap-exempt: the marker read starts no tmux server

STATUS: complete

SPEC CONTEXT:
§5.4 (specification.md:296-314) is the governing section: a restore window is a hole in the reaper's judgement — between skeleton construction and the pane re-stamp every live pane carries no token, so a sweep landing there would reap every token-keyed entry on the machine. The daemon was already immune via `tick`'s early return; `portal doctor --fix` was not, and it is the command a user reaches for when a reboot looks wrong. The check therefore goes *into* the sweep so it travels with the rule, with one line shape for every decline (`op=clean-stale-skipped`, `via=internal`, `reason=…`). §9.2's requirement row (specification.md:504) pins "the sweep and the check stand down during a restore".

Two record entries move the target past this task's wording, both authoritative over it:
- Corrigendum 2026-09-04 (specification.md:568): a failed marker read stands the cycle down under its **own** reason `marker-read-failed` and its own phrase, not under `restoring`. The task body asked for `reason=restoring` on that branch; the corrigendum supersedes it, with the reason stated (doctor is bootstrap-exempt, so a down server is the ordinary path into that branch, and asserting a restore the user is not running defeats the point of naming a decline).
- Phase 9's consolidation moved the cycle out of `cmd/run_hook_stale_cleanup.go` into `internal/hooksweep` (commit a4898f41), so the files this task's own commit (00d6638b) touched no longer exist under those names. Verified against the code as it now stands.

IMPLEMENTATION:
- Status: Implemented (re-homed by a later phase; behaviour intact and strengthened)
- Location:
  - `internal/hooksweep/sweep.go:37-40` — `Reader` is the widened seam: `PaneHookLister` + `state.RestoringChecker`, with the doc comment naming the marker read it carries.
  - `internal/hooksweep/sweep.go:71-80` — `StalenessStandDown` folds the read through `state.RestoreWindowActive(state.IsRestoringSet(reader))` (`internal/state/markers.go:91-110`, where a failed read already counts as set), then discriminates: read error → `declineDebug(ReasonMarkerReadFailed, "error", err)`; marker set → `declineDebug(ReasonRestoring)`.
  - `internal/hooksweep/sweep.go:152-167` — `Run` takes the stand-down *first*, before `store.CleanStale`, so neither the enumeration nor either of the store's two reads happens; the comment states why (a restore window is no time to wait on the file's lock for an answer that cannot be acted on).
  - `internal/hooksweep/standdown.go:12-16,35-43` — one line shape for every decline: message and `op` both `clean-stale-skipped`, `via` from `hooks.ViaInternal.String()` ("internal", `internal/hooks/via.go:23-27`), `reason` the closed enum value, extra attrs appended; `declineDebug` fixes DEBUG for both restore-window branches while `declineWarn` keeps WARN for the anomalies.
  - Both call sites inherit it with no guard of their own: `cmd/state_daemon.go:214` (`maybeRunHookCleanup`) and `cmd/doctor.go:201` (`pruneDoctorStaleHooks`). `hooksweep.Run` is the only production caller of `hooks.Store.CleanStale` in the tree, so the rule cannot be bypassed.
  - `cmd/doctor.go:375-377` — the read-only diagnosis takes the same `StalenessStandDown` before it counts a key, so diagnosis and reaper cannot disagree (§5.4's `checkStaleHooks` paragraph).
  - `cmd/state_daemon.go:174-181` — `tick`'s early return is untouched, and its comment still states the ordering is load-bearing.
  - `cmd/bootstrap_production_test.go:11` — `var _ hooksweep.Reader = (*tmux.Client)(nil)` pins the first criterion at compile time.
- Notes:
  - Bootstrap-exemption holds: the marker read is `show-option -sv @portal-restoring` (`internal/tmux/tmux.go:351-364`), which starts no server. A down server's stderr matches neither entry of `optionAbsentStderrPatterns` (`internal/tmux/tmux.go:18-20`, "invalid option:" / "unknown option:"), so it surfaces as an error and stands the cycle down rather than being mistaken for "option absent, not restoring" — the one way this could have failed silently.
  - The marker read sits outside every lock: `hooks.Store.CleanStale` (`internal/hooks/store.go:298-310`) takes its snapshot, releases, then calls the enumeration with no hold, and the stand-down precedes all of it.
  - Divergence from the task's wording on the failed-read reason (`marker-read-failed` rather than `restoring`) is the corrigendum's, not drift: the posture is identical (stand down, DEBUG, nothing read, nothing written), only the reported reason and user phrase differ, and `cmd/doctor.go:236-254` gives both reasons their own copy on both surfaces.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooksweep/sweep_test.go:255-335` — the four ordering/behaviour cases: stands down before enumerating with the marker set (call count 0); stands down on a failed marker read; skips before loading the store (staged unreadable, so a `store-read-failed` reason would have exposed a load); sweeps normally with the marker absent (call count 1, stale key reaped, live key kept).
  - `internal/hooksweep/sweep_move_test.go:47-138` — the per-reason table pins level and the whole line shape through `assertStandDown` (`helpers_test.go:114-138`: msg, `op`, `component=hooks`, `via=internal`, `reason`), DEBUG for `restoring` and `marker-read-failed`, WARN for the rest, with an exhaustiveness check against `hooksweep.Reasons`. The DEBUG arm asserts the sink holds *no* record at WARN or above, which is what carries "never a WARN on this branch".
  - `cmd/doctor_stand_down_copy_test.go:56-123,289-336` — the cross-surface table: the restore row asserts DEBUG plus the *absence* of an `error` attr, the marker-read row asserts DEBUG plus `error` exactly "no server running" (the AC's read-error attr), and both arms assert `hooks.json` byte-identical after the sweep and after a real `doctor --fix` Execute, plus the printed "Skipped stale hook prune: restore in progress" line.
  - `cmd/doctor_fix_hook_prune_report_test.go:35-41` and `cmd/doctor_test.go:1134-1148` — `--fix` prints the skipped line, and the read-only diagnosis stays exit 0 and not-evaluable in a restore window.
  - Daemon unchanged: `cmd/state_daemon_hook_cleanup_test.go` still drives reap/no-reap through `daemonFakeCommander`, whose unknown-option dispatch returns a `*tmux.CommandError` carrying "unknown option:" (`cmd/state_daemon_run_test.go:102-112`) — so the added read resolves as not-restoring and no cleanup test changed meaning; `cmd/state_daemon_run_test.go:302-357` keeps the tick-suppression cases.
- Notes: the restore-window stand-down is asserted at three levels (package ordering, package contract, cmd copy). The overlap is bounded and each layer's subject is different — ordering, reason vocabulary, user-visible copy — so this is not over-testing.

CODE QUALITY:
- Project conventions: Followed. Component binding is once per package (`internal/hooksweep/sweep.go:14`, `log.For("hooks")`); `op`/`via`/`reason`/`error` are all inside the closed vocabulary the spec amends (specification.md:387); tests are unit-lane, no `t.Parallel()`, no real tmux, no daemon.
- SOLID principles: Good. `Reader` is a 2-method seam composed from two single-purpose interfaces; `StalenessStandDown` is exported precisely so the diagnosis and the reaper share one gate rather than restating it.
- Complexity: Low. The gate is a three-arm switch over one folded read.
- Modern idioms: Yes. `StandDown` carries level+attrs as a value so the emission site is single; `declinedError` carries a stand-down through the error path rather than a side variable.
- Readability: Good. Every non-obvious choice is stated in-source (why DEBUG, why the failed read gets its own reason, why the gate precedes the store read).
- Comment accuracy: Checked against the code. `Reader`'s doc names the marker read; `Run`'s pre-store comment matches the call order; `StandDown.emit`'s "emission is the sweep's" holds — `cmd/doctor.go:375-377` consumes the value and logs nothing.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

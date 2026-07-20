TASK: cli-verb-surface-redesign-4-5 — `doctor --fix`: repairs + unconditional log-sweep side-action + re-diagnose

ACCEPTANCE CRITERIA (plan row 4-5 + Phase 4 AC bullets):
- server down → NO hook pruning (reuse runHookStaleCleanup hazard guard; protects user-authored on-resume commands)
- filesystem-only stale-project prune still runs on a down server
- log-sweep outside the diagnose→repair loop, never touches the exit code (no "logs" catalog check)
- re-diagnosis exits non-zero if anything remains unhealthy/unfixable
- repairs reuse runHookStaleCleanup + project CleanStale + log.SweepLogsForClean
- stays bootstrap-exempt (starts nothing)

STATUS: Complete

SPEC CONTEXT:
Specification §309–318 governs `portal doctor --fix`. Three governing rules:
(1) It performs the reversible-by-reconstruction repairs it diagnoses (prune stale hooks, prune stale projects) plus a log sweep, then re-runs the diagnosis and exits 0 iff healthy post-repair.
(2) "Log-sweep is outside the diagnose→repair loop" — the catalog has NO "logs" check; the sweep is a deliberate unconditional maintenance side-action that never participates in the exit-code contract.
(3) "Down-server guard on the stale-hook prune (data-loss safety)" — with the server down, live-pane enumeration is empty so every hook would falsely look orphaned; `--fix` must perform NO hook pruning in that state (a user-authored on-resume command is not reconstructable by Portal). The filesystem-only stale-project prune may still run.

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria exactly; no drift)
- Location:
  - cmd/doctor.go:215-248 — RunE `--fix` branch: renders initial report, calls runDoctorFix, re-runs runDoctorDiagnosis, renders post-repair report, returns ErrDoctorUnhealthy iff post-repair unhealthy. Exit driven SOLELY by post-repair results.
  - cmd/doctor.go:268-274 — runDoctorFix orders the three repairs: pruneDoctorStaleHooks → pruneDoctorStaleProjects → sweepDoctorLogs (matches spec order "prune hooks, prune projects, sweep logs").
  - cmd/doctor.go:286-293 — pruneDoctorStaleHooks delegates VERBATIM to runHookStaleCleanup; no bespoke down-server branch. The hazard guard (cmd/run_hook_stale_cleanup.go:119-126) defers when the live set is empty-or-errored, which is the down/rebooted-server state — so hooks are never mass-deleted. Error return discarded (repairs never drive the exit).
  - cmd/doctor.go:300-312 — pruneDoctorStaleProjects delegates to project.Store.CleanStale (filesystem-only, os.Stat-based); no server gate, so it runs regardless of server state. CleanStale error logged under bootstrap component and swallowed.
  - cmd/doctor.go:321-334 — sweepDoctorLogs calls log.SweepLogsForClean(stateDir); result swallowed (WARN under bootstrap component). Resolves deps.StateDir (test override) else state.Dir() READ-ONLY.
  - cmd/doctor.go:686-689 — `--fix` bool flag registered in init(); doctorCmd added to rootCmd.
- Bootstrap-exempt confirmed: cmd/root.go:61 skipTmuxCheck["doctor"]=true; runDoctorDiagnosis and sweepDoctorLogs resolve state via state.Dir() (internal/state/paths.go:36 — pure env/path resolution, NO MkdirAll) never EnsureDir (paths.go:51). ServerRunning is a probe only. Doctor starts no server, respawns no daemon.
- No "logs" catalog check: runDoctorDiagnosis (cmd/doctor.go:354-370) contains exactly daemon/saver/hooks/state-dir/sessions.json/stale-hooks/stale-projects + informational host-terminal — no logs check. Confirmed by grep.
- Reuse verified: log-sweep target ${stateDir}/portal.log.* (internal/log/names.go) matches the state.Dir() the sweep resolves — production wiring is correct.

TESTS:
- Status: Adequate (well-balanced; each test isolates a distinct acceptance criterion)
- Coverage:
  - cmd/doctor_test.go:934-989 TestDoctorFixPrunesStaleEntriesThenRediagnosesClean — happy path: stale hook + stale project seeded over a healthy runtime; both pruned from disk, live project retained, breadcrumbs printed, exactly 2 "Portal doctor:" reports (initial + post-repair), post-repair stale checks clean, exit 0.
  - cmd/doctor_test.go:996-1027 TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored — table-driven (empty live set + enumeration error): hooks.json byte-identical after --fix. Proves the protection is the runHookStaleCleanup hazard guard, not a bespoke branch.
  - cmd/doctor_test.go:1034-1077 TestDoctorFixDownServerPrunesProjectsButNotHooks — down server: hooks.json byte-identical (hazard guard defers), stale-project prune STILL runs (gone dir removed + breadcrumb), and Execute returns ErrDoctorUnhealthy (post-repair re-diagnosis still non-zero — exit driven by post-repair state).
  - cmd/doctor_test.go:1084-1107 TestDoctorFixLogSweepNeverDrivesExit — stale rotated log seeded; nil stores isolate the sweep as the only repair; the stale log is swept (proving it ran against the state dir) yet the command exits 0 — the log-sweep never drives the exit code.
  - cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go (integration) — drives pruneDoctorStaleHooks under simulated `list-panes -a` transients (exit-nonzero, empty-stdout, and a real-tmux normal path): hooks.json is not wiped under transient, and legitimate stale removal still works with the correct Debug breadcrumbs.
- Not under-tested: every acceptance clause is exercised — down-server hook protection, down-server project prune, exit-driven-by-post-repair, log-sweep-outside-exit-contract, and reuse of the hazard-guarded prune under transient tmux.
- Not over-tested: no redundant assertions. TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored (hazard-guard isolation, server nominally up) and TestDoctorFixDownServerPrunesProjectsButNotHooks (down-server split + non-zero exit) overlap only superficially on "hooks not pruned"; they verify genuinely distinct contracts.

CODE QUALITY:
- Project conventions: Followed. DI seam (DoctorDeps + package-level doctorDeps, resolveDoctorDeps per-field nil fallthrough) matches the codebase's *Deps idiom; tests inject hermetic seams and never touch real tmux/state (t.Cleanup restores doctorDeps=nil). Errors swallowed at the repair boundary are logged under the existing bootstrap component (log.For("bootstrap")) — no new log component invented (closed-taxonomy respected). Tests carry the no-t.Parallel note.
- SOLID: Good. Single-responsibility helpers (runDoctorFix orchestrates; pruneDoctorStaleHooks / pruneDoctorStaleProjects / sweepDoctorLogs each own one repair). Down-server safety is single-sourced in runHookStaleCleanup's hazard guard rather than duplicated — the "no double-guard, no drift" reasoning is sound and explicitly documented.
- Complexity: Low. RunE's --fix arm is a linear render→repair→re-diagnose→exit sequence; each helper is a short guarded delegation.
- Modern idioms: Yes. errors.Is for os.ErrNotExist classification in the mirroring check; idiomatic best-effort swallow-and-log.
- Readability: Excellent. Doc comments precisely state the exit-code contract, the deliberate log-sweep-outside-the-loop rationale, and why the down-server protection is delegated rather than re-branched.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (runDoctorFix's reserved error return — "today always returns nil", cmd/doctor.go:258-260 — is a documented forward-compat choice paired with the RunE err-check; it is deliberate and needs no change.)

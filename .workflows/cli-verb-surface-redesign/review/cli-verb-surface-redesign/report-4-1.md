TASK: cli-verb-surface-redesign-4-1 — `doctor` command + read-only diagnosis framework (state-package checks)

ACCEPTANCE CRITERIA (from Phase 4 task table + phase ACs):
- Fresh install / no state dir yet reported honestly (not a crash).
- Missing or corrupt `sessions.json` reads as invalid via `ReadIndex` skip/err (the `HasLastSave=false` mechanism).
- Dead `daemon.pid` → daemon-alive fails → non-zero exit.
- Strictly read-only (no tmux or file mutation).
- Bootstrap-exempt so it observes raw state and heals nothing.
- Report always carries at least one check.

STATUS: Complete

SPEC CONTEXT:
Spec "doctor — Diagnostics & Repair" defines a read-only health report over a fixed catalog (daemon alive; hooks; saver; state dir sane; sessions.json valid; no stale entries; host terminal). Exit-code contract: `portal doctor` exits 0 iff every check passes, non-zero (1) if any check reports a problem; a down server is unhealthy → non-zero, reported honestly and DISTINCTLY from corruption ("Portal runtime not running…" vs corruption). Bootstrap Exemption section: `doctor` MUST be bootstrap-exempt so a read-only check does not heal its own subject (self-defeating green) — it observes raw state, starts nothing, heals nothing. Task 4-1 owns the framework + the state-package (no-tmux) checks: daemon-alive (state pid/liveness/version), state dir sane, sessions.json valid, plus the render/exit-code plumbing. (Runtime tmux checks = 4-2, stale = 4-3, host terminal = 4-4, --fix = 4-5.)

IMPLEMENTATION:
- Status: Implemented (with a well-reasoned, spec-aligned refinement over the literal task wording).
- Location:
  - cmd/doctor.go:209-248 — doctorCmd (NoArgs, SilenceErrors/SilenceUsage, RunE drives ErrDoctorUnhealthy).
  - cmd/doctor.go:342-372 — runDoctorDiagnosis: fixed 7-check slice (+ host-terminal appended), always non-nil, error return always nil today (state.Dir() failure folds into per-check fails so the report still carries every check).
  - cmd/doctor.go:502-523 — checkDaemonAlive: narrow STATE-based probe (state.ReadPIDFile + state.IsProcessAlive + state.ReadVersionFile); dead/missing/unparseable PID → checkFail "not running". Deliberately no state-dir tree-walk / portal.log scan (routine run stays cheap — the task-8-1 narrowing).
  - cmd/doctor.go:599-619 — checkStateDirSane: fs.ErrNotExist → checkPass "not created yet" (fresh install healthy); non-dir → fail; unreadable stat → fail.
  - cmd/doctor.go:621-645 — checkSessionsJSON: reads via state.ReadIndex directly (absent → pass "no sessions saved yet"; corrupt/err → fail "sessions.json corrupt"; valid → pass "N sessions, M panes").
  - cmd/doctor.go:65-200 — DoctorDeps DI seam + resolveDoctorDeps (per-field nil-check idiom matching commitNowDeps/bootstrapDeps; StateDir override for hermetic tests, state.Dir() in prod — never EnsureDir).
  - cmd/doctor.go:20-25 (ErrDoctorUnhealthy), 650-684 (renderDoctorReport/checkMarker/doctorUnhealthy).
  - internal/state/index_reader.go:39-68 — ReadIndex (absent → (Index{},true,nil); corrupt → (Index{},true, err-wrapping-ErrCorruptIndex)) + CountPanes.
  - cmd/root.go:58-66 — skipTmuxCheck includes "doctor":true (bootstrap-exempt).
- Notes:
  - **Deliberate refinement (not drift):** the task text names `HasLastSave=false` as the read mechanism. The implementation reads `state.ReadIndex` directly and SPLITS the outcome — absent → checkPass "no sessions saved yet"; corrupt → checkFail. This is more correct than a lossy HasLastSave boolean and is required by the spec's "distinct from corruption" contract + the fresh-install-honesty AC (a fresh install must not be flagged unhealthy on the sessions.json line — only the daemon line honestly fails). checkSessionsJSON's own doc comment documents the reasoning. Task's named primitive (ReadIndex) is exactly what is used.
  - **Stale task pointer:** the brief lists `internal/state/status.go` as a primary area; that file / CollectStatus / StatusReport were removed by the later analysis task 8-1 which narrowed the daemon probe onto ReadPIDFile/IsProcessAlive/ReadVersionFile. The daemon-alive check exists and is correct; it simply no longer routes through a status.go aggregate. No action — expected given the post-4-1 analysis cycles.
  - Fresh install: daemon → fail "not running", state dir → pass "not created yet", sessions.json → pass "no sessions saved yet"; overall unhealthy → non-zero but no crash. Matches AC exactly.
  - Read-only: state.Dir() is pure path-join (verified internal/state/paths.go:36-48 — no mkdir); EnsureDir (the creating variant) is never called by the diagnosis path.

TESTS:
- Status: Adequate
- Coverage (cmd/doctor_test.go):
  - Fresh install honesty → TestDoctorFreshInstallReportedHonestly (daemon down + state-dir "not created yet" + sessions.json "no sessions saved yet", ErrDoctorUnhealthy, no crash).
  - Dead daemon.pid → non-zero → TestDoctorDeadDaemonFailsNonZero (ErrDoctorUnhealthy + "daemon: not running").
  - Read-only → TestDoctorIsReadOnly (asserts the non-existent state dir is NOT created by a pass).
  - sessions.json valid/absent/corrupt distinguished → TestDoctorSessionsJSONStatesDistinguished (3 subtests).
  - Daemon detail live/missing → TestDoctorDaemonCheckDetail.
  - State dir healthy dir passes → TestDoctorStateDirSaneHealthyDirPasses.
  - Always ≥1 check + stable order → TestDoctorCheckOrder (pins exactly 8 checks incl. host-terminal).
  - Bootstrap-exempt → TestDoctorRegisteredInSkipTmuxCheck; registration → TestDoctorIsRegisteredCommand.
  - NoArgs → TestDoctorRejectsArgs; silent unhealthy exit → TestDoctorSilenceFlags / TestDoctorUnhealthyStderrSilent / TestIsSilentExitErrorRecognisesDoctorUnhealthy.
  - Tests are hermetic (DoctorDeps.StateDir temp dir + injected runtime seams) — no real tmux server or developer state dir is touched, satisfying the project's absolute isolation invariant.
- Notes:
  - Not over-tested: each 4-1 test targets a distinct behaviour; no redundant happy-path duplication. (The larger 4-2/4-3/4-4/4-5 suites share the file but are out of this task's scope.)
  - Minor under-test: checkStateDirSane's two FAIL branches — "not a directory" (existing path is a file) and "unreadable" (stat error) — have no test; only the pass branches ("not created yet", healthy dir) are exercised. Low-stakes but genuinely uncovered.
  - checkSessionsJSON's / checkStateDirSane's `dirErr != nil` ("unresolvable") branch is effectively unreachable through the current StateDir-override seam (state.Dir() only errors on XDG resolution failure); acceptable to leave uncovered.

CODE QUALITY:
- Project conventions: Followed. DI via a small *Deps struct with per-field nil-check fall-through (matches commitNowDeps/bootstrapDeps); silent-exit sentinel wired through IsSilentExitError; heavy explanatory comments consistent with house style; no bare os.Exit; read-only state.Dir() (never EnsureDir) honours the bootstrap-exempt contract.
- SOLID: Good. Each checkX is single-responsibility and pure over injected facts; the checkStatus/checkResult framework is open/closed — adding a check is one line in runDoctorDiagnosis. Render/exit-code logic (renderDoctorReport, doctorUnhealthy) is separated from the checks.
- Complexity: Low. Small switch/if ladders, clear paths.
- Modern idioms: Yes (errors.Is, fs.ErrNotExist, range-over-int in tests).
- Readability: Good. Intent-revealing names and details; the "not created yet" / "no sessions saved yet" copy makes fresh-install state legible.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/doctor_test.go (checkStateDirSane coverage) — add a test for the two uncovered fail branches: an existing-but-non-directory StateDir path → checkFail "not a directory", and (if portably feasible) an unreadable-stat path → checkFail "unreadable". Both are real code paths (cmd/doctor.go:612-618) with zero coverage.
- [quickfix] cmd/doctor.go:642, 438, 488, and daemon/sessions details — count details are always plural ("1 sessions, 1 panes", "1 stale hook entries", "1 stale projects"). Pluralise for grammatical polish; a concrete edit at known locations, though it also requires touching the asserting test strings. Purely cosmetic diagnostic copy — optional.

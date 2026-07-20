TASK: cli-verb-surface-redesign-8-1 — Narrow `portal doctor`'s daemon-liveness probe off the over-scoped `CollectStatus` and share one pane counter

ACCEPTANCE CRITERIA:
- Routine `portal doctor` performs no state-dir tree walk and no full portal.log scan
- sessions/panes derive from a single `ReadIndex` per invocation
- exactly one pane counter (no `doctorPaneCount` duplicate of `state.countPanes`)
- dead `daemon.pid` → `DaemonRunning=false`
- `CollectStatus` trimmed to consumed fields or deleted if doctor was its last production caller
- daemon/sessions/panes output behaviourally identical to pre-change

STATUS: Complete

SPEC CONTEXT: Spec §299-306 defines `portal doctor` as a read-only health report; "daemon alive" is one catalog check and the spec explicitly delegates the concrete probe to planning ("planning implements the concrete probe per check"). §365 requires bootstrap-exemption so doctor observes raw state. The task is a cycle-2 analysis refactor (analysis-report-c2.md, merged findings A1 "CollectStatus over-scope for the daemon probe" + D2 "doctorPaneCount byte-identical copy"): narrow the daemon probe to only the facts the detail needs, and single-source the pane counter — while keeping output byte-identical.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/doctor.go:502-523 (checkDaemonAlive), cmd/doctor.go:642 (CountPanes call), internal/state/index_reader.go:56-68 (state.CountPanes), commit 0610619e.
- Notes:
  - CollectStatus/StatusReport/collectDaemonState/computeStateSize/scanRecentWarnings deleted outright — internal/state/status.go (225 LOC) and status_test.go (543 LOC) removed. Repo-wide grep confirms zero remaining references to CollectStatus / StatusReport / ErrStatusUnhealthy. Phase-4 task 4-7 had deliberately kept CollectStatus "for doctor"; this task confirms doctor was its last production caller and deletes the whole layer. Correct call vs. "trim to consumed fields."
  - `checkDaemonAlive` now reads only state.ReadPIDFile + state.IsProcessAlive + state.ReadVersionFile — no filepath.WalkDir tree walk (was computeStateSize) and no portal.log line scan (was scanRecentWarnings). The over-scoped `now time.Time` param, the `Now` DoctorDeps seam, and the `"time"` import are all removed cleanly. Routine-doctor no-tree-walk / no-log-scan holds structurally.
  - Single ReadIndex per diagnosis: pre-change the daemon path ran CollectStatus (which called ReadIndex via collectIndexState) AND checkSessionsJSON called ReadIndex — two reads. Now only checkSessionsJSON calls state.ReadIndex(dir) once; the daemon check reads pid/version files instead. Confirmed one ReadIndex per diagnosis pass.
  - One pane counter: `doctorPaneCount` (cmd) and unexported `countPanes` (state) both deleted; replaced by the single exported `state.CountPanes(idx)` in index_reader.go, called at cmd/doctor.go:642. Grep confirms it is the only pane-counting implementation.
  - dead daemon.pid: `err != nil || !state.IsProcessAlive(pid)` → checkFail "not running" — behaviourally identical to the old `!report.DaemonRunning` (DaemonRunning was `IsProcessAlive(pid)` guarded by a nil ReadPIDFile error). Short-circuit means IsProcessAlive is not called on a read-error pid. Version handling identical (missing/unreadable version swallowed → doctorDaemonVersion → "unknown"). Behavioural identity holds across all three arms (read-error/dead → "not running"; alive → "running (pid N, version V)").
- go build ./... passes.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/count_panes_test.go (new): sums across sessions/windows (2+1+3=6), zero for empty Index, zero when windows have no panes — the three meaningful CountPanes branches; not over-tested.
  - cmd/doctor_test.go TestDoctorDaemonCheckDetail: live pid+version → exact "running (pid <getpid>, version v1.2.3)" detail; missing pid → checkFail "not running". Dead-pid path covered by TestDoctorUnhealthyStderrSilent (seedDeadDaemonPID → ErrDoctorUnhealthy) and TestDoctorHostTerminalNeverDrivesExit. TestDoctorSessionsJSONStatesDistinguished asserts the "N sessions, M panes" detail via the shared CountPanes.
  - doctor_test.go diff is otherwise purely mechanical removal of the now-deleted `Now: time.Now` field from every DoctorDeps literal; `"time"` remains imported/used by seed helpers.
  - The 543-line status_test.go deleted with its subject — appropriate, not lost coverage (the daemon/sessions/panes observable behaviour is covered through the doctor checks).
- Notes: No dedicated guard test asserts the negative property "routine doctor never walks the state-dir tree / scans portal.log." The property now holds structurally (CollectStatus deleted) and the pre-change code had no such test either, so this is not a regression — noted below as an optional idea only.

CODE QUALITY:
- Project conventions: Followed. Matches the per-field nil-check DoctorDeps DI idiom; state-package helper exported with a doc comment; no bare os.Exit; no new log component. Consistent with golang-code-style / structs-interfaces conventions.
- SOLID principles: Good. The daemon probe now depends only on the three narrow facts it consumes (interface-segregation improvement over the fat StatusReport); single-responsibility CountPanes owned by the state package.
- Complexity: Low. checkDaemonAlive is a flat guard-and-return; CountPanes is a two-level sum.
- Modern idioms: Yes. `version, _ := state.ReadVersionFile(dir)` deliberate error-swallow is documented inline.
- Readability: Good. Doc comments updated to state the "does NOT walk the tree or scan portal.log — a routine run stays cheap" intent and the deliberate version-error swallow.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/state/count_panes_test.go / internal/state/index_reader.go:60 — CountPanes lives in index_reader.go but its tests sit in a standalone count_panes_test.go, a mild file/source mismatch. Either add internal/state/count_panes.go housing CountPanes, or fold the three CountPanes tests into index_reader_test.go, so the test file name maps to a source file.
- [idea] cmd/doctor.go:502 — the acceptance's "routine doctor performs no state-dir tree walk and no full portal.log scan" now holds only structurally (CollectStatus deleted), with no guard against a future reintroduction. Optional, low value in a converging analysis cycle: consider a guard test / lint that fails if the routine doctor path regains a filepath.WalkDir over the state dir or opens portal.log. Not worth churn if it complicates the suite.

TASK: cli-verb-surface-redesign-4-3 — Read-only stale-entry checks (dead-pane hooks + gone-dir projects)

ACCEPTANCE CRITERIA (plan row 4-3):
- server down → dead-pane-hook staleness reported not-evaluable (never "all stale", never a false failure)
- zero live panes with hooks present is exactly the not-evaluable case (mirrors runHookStaleCleanup hazard guard)
- genuine stale hook/project → non-zero
- gone-dir detection os.Stat-based (permission-denied path retained, not counted stale)
- strictly read-only (no pruning here — that is --fix)

STATUS: Complete

SPEC CONTEXT:
Spec §295–319 (doctor catalog + exit-code contract) and §312 (Down-server guard on the stale-hook prune). The "no stale entries" catalog check pairs dead-pane hooks + gone-dir projects. §312 is the load-bearing constraint: detecting a dead-pane hook requires enumerating live panes on a running server; with the server down that enumeration is empty and every hook would falsely look orphaned, so staleness must report not-evaluable (never "all stale") and --fix must never prune hooks in that state (user-authored on-resume commands are non-reconstructable). Stale-project detection is filesystem-only (os.Stat directory existence) and runs regardless of server state.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/doctor.go:413-441 checkStaleHooks (read-only mirror of runHookStaleCleanup's hazard guard)
  - cmd/doctor.go:446-458 countStaleHookKeys (read-only ∉-live-set classifier)
  - cmd/doctor.go:465-491 checkStaleProjects (os.Stat classification, filesystem-only)
  - cmd/doctor.go:47-49,666-667,677-684 checkNotEvaluable status + marker + doctorUnhealthy fold (only checkFail drives exit)
  - cmd/doctor.go:360-361 both checks wired into runDoctorDiagnosis catalog
- Notes:
  - checkStaleHooks deliberately does NOT take a serverUp gate. Not-evaluability is derived from the live-pane enumeration result itself: an error → "could not enumerate live panes" (not-evaluable); an empty live set with persisted hooks → "zero live panes with hooks present (not evaluable)". On a live server ListAllPaneHookKeys is never empty (it enumerates every pane's key incl. internal sessions), so the empty/errored branch IS the down-server proxy exactly — a faithful mirror of the runHookStaleCleanup hazard guard and stronger than a serverUp gate (also covers server-up-but-zero-panes). Both-empty (no hooks, no panes) → checkPass "no hooks", honest.
  - checkStaleProjects os.Stat switch is byte-for-byte the classification in project.Store.CleanStale (store.go:202-213): nil → live, ErrNotExist → stale, default (permission-denied/other) → retained NOT stale. Verified against the source.
  - Genuine stale hook/project → checkFail → doctorUnhealthy(true) → ErrDoctorUnhealthy → non-zero exit. Both checks return checkFail only on a positively-computed stale count (never on a not-evaluable/transient condition).
  - Strictly read-only: neither check calls Save/CleanStale; countStaleHookKeys and the os.Stat loop only read.

TESTS:
- Status: Adequate
- Coverage (cmd/doctor_test.go):
  - TestDoctorStaleHooksCheck: persisted-key-with-no-live-pane → checkFail "1 stale hook entries"; zero-live-panes-with-hooks → checkNotEvaluable (never all-stale); enumeration error → checkNotEvaluable; both-empty → checkPass "no hooks"; all-live → checkPass. Directly covers the down-server-proxy not-evaluable cases and the genuine-stale fail.
  - TestDoctorStaleProjectsCheck: gone-dir → checkFail "1 stale projects" with live dir retained; all-live → checkPass; evaluates-with-server-down (filesystem-only) → still checkFail. Covers os.Stat classification and server-independence.
  - TestDoctorStaleChecksAreReadOnly: seeds genuinely-stale hook + project, asserts both checkFail (proving they ran) AND byte-identical hooks.json/projects.json after a full diagnosis — proves the strictly-read-only criterion.
  - Permission-denied-retained: intentionally delegated. doctor_test.go:1146-1149 documents that portably simulating EACCES at the doctor layer is infeasible; the identical os.Stat default branch is covered by internal/project/store_test.go:633 "retains project with permission denied" (chmod 0o000 parent → EACCES → retained). Sound delegation — the doctor check mirrors the same switch and the classification is proven where EACCES is portably reproducible.
- Notes: Not over-tested (each case is distinct and load-bearing). The checkFail→non-zero-exit wiring is proven at Execute level for other failing checks (doctor_test.go:204/219/239/387), and doctorUnhealthy is a pure fold over checkFail, so the composition is airtight — but no single Execute-level test seeds a genuine stale entry and asserts ErrDoctorUnhealthy end-to-end (see non-blocking note).

CODE QUALITY:
- Project conventions: Followed — errors.Is for ErrNotExist, small pure helpers, DI seam via DoctorDeps, nil-store → not-evaluable rather than crash (matches golang-safety/error-handling idioms).
- SOLID principles: Good — checkStaleHooks/checkStaleProjects/countStaleHookKeys are single-responsibility pure functions.
- Complexity: Low — flat switch/guard structure, no nesting beyond one level.
- Modern idioms: Yes.
- Readability: Excellent — doc comments state the not-evaluable rationale and explicitly name the runHookStaleCleanup / project.Store.CleanStale mirrors and the deliberate guard order.
- Issues: None blocking. Two deliberate mirror-duplications carry a latent drift risk (see notes).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/doctor.go:465-491 (checkStaleProjects) & internal/project/store.go:202-213 (CleanStale) — the os.Stat stale/live/retained classification is duplicated across the read-only check and the mutating pruner; likewise cmd/doctor.go:446-458 (countStaleHookKeys) mirrors hooks.Store.CleanStale's ∉-live selection. Extracting a shared read-only classifier (e.g. project.IsStalePath / a hook-staleness predicate) consumed by both the check and the pruner would eliminate the drift class where a future change to CleanStale's classification silently diverges from doctor's report. Requires deciding where to home/export it and whether to refactor CleanStale to consume it — design judgment, not a mechanical edit.
- [quickfix] cmd/doctor_test.go (TestDoctorStaleHooksCheck / TestDoctorStaleProjectsCheck) — no Execute-level test seeds a genuine stale hook or project over an otherwise-healthy runtime and asserts plain `portal doctor` (no --fix) returns ErrDoctorUnhealthy; the checkFail→exit path is proven only via other failing checks and left to compositional inference for stale entries. Add one small Execute assertion (staleDeps + healthy runtime, no --fix) to close the end-to-end gap.

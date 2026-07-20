TASK: Doctor — Correct Count Copy And Close The Test-Coverage Gaps (cli-verb-surface-redesign-13-4)

ACCEPTANCE CRITERIA:
1. Count diagnostics render singular/plural correctly.
2. Both `checkStateDirSane` fail branches are exercised by tests.
3. An Execute-level test asserts `ErrDoctorUnhealthy` on a seeded stale hook/project.

STATUS: Complete

SPEC CONTEXT: Low-severity remediation task sourced from review reports 4-1 and 4-3.
`portal doctor` (cmd/doctor.go) is a bootstrap-exempt, read-only ordered catalog of
health checks driving a scriptable exit code (0 iff all pass; ErrDoctorUnhealthy →
non-zero on any checkFail). The spec has no bespoke count-copy wording — the fix is
grammatical polish plus two uncovered fail branches and one missing Execute-level
unhealthy path. The singular-namespace-noun convention in spec §356 is about the
`hooks`→`hook` verb rename, unrelated to this count copy.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Pluralisation helper: cmd/doctor.go:594-600 (`pluralCount(n, singular, plural)` —
    singular only when n==1, plural otherwise).
  - Call sites, all routed through the helper: checkStaleHooks cmd/doctor.go:455
    ("stale hook entry"/"stale hook entries"); checkStaleProjects cmd/doctor.go:478
    ("stale project"/"stale projects"); checkSessionsJSON cmd/doctor.go:645
    ("session"/"sessions" + "pane"/"panes").
  - checkStateDirSane fail branches: cmd/doctor.go:615-616 ("unreadable", non-ErrNotExist
    stat error) and :619-620 ("not a directory", exists-but-!IsDir default arm).
- Notes: Every count diagnostic needing pluralisation now routes through the single
  `pluralCount` helper — no ad-hoc "%d sessions" formatting remains. The one other count
  in the file (checkHooksRegistered duplicate detail, :569 "duplicate hook entries (%d)")
  is correctly left plural-only: its branch fires only when count >= 2, so it can never
  read singular. The helper's doc-comment cross-reference to cmd/list.go's window count is
  accurate (verified cmd/list.go:41-44 uses the identical `== 1` singular rule).

TESTS:
- Status: Adequate
- Coverage:
  - AC1 (pluralisation) — both singular and plural pinned for all three diagnostics:
    sessions "1 session, 1 pane" (doctor_test.go:197) and "3 sessions, 3 panes" (:292);
    stale hooks "1 stale hook entry" (:944) and "2 stale hook entries" (:961); stale
    projects "1 stale project" (:1465) and "2 stale projects" (:1483).
  - AC2 (checkStateDirSane fail branches) — TestDoctorStateDirSaneFailBranches
    (doctor_test.go:380-430): "not a directory" via a regular file at the path (:381-399);
    "unreadable" via a 0o000 parent dir yielding EACCES on stat (:401-429), with a correct
    root-skip guard (0o000 is bypassed by root).
  - AC3 (Execute-level stale → ErrDoctorUnhealthy) — TestDoctorExecuteStaleEntryReturnsUnhealthy
    (doctor_test.go:1238-1269): seeds a healthy runtime + a genuinely stale hook (non-empty
    live set so the hazard guard does not defer) AND a stale project, drives plain
    `portal doctor` through rootCmd.Execute (runDoctorCmd), asserts err == ErrDoctorUnhealthy,
    silent stderr, both fail lines present, and exactly one report render (read-only, no
    --fix re-diagnosis).
- Notes: Tests would fail if the feature broke (a non-singularising helper produces
  "1 sessions"/"1 stale hook entrys"; a broken fail branch changes status/detail; a missing
  Execute unhealthy path returns nil). Focused, not over-tested — one singular + one plural
  per diagnostic is proportionate, and the fail-branch subtests each exercise a distinct
  code path. No redundant assertions or excess mocking.

CODE QUALITY:
- Project conventions: Followed — DI via *DoctorDeps seams, hermetic StateDir temp dirs,
  no t.Parallel (package-level doctorDeps mutation), Execute-level tests isolate
  terminals.json. Consistent with the cmd package idioms.
- SOLID principles: Good — pluralCount is a single-responsibility pure helper; the three
  count sites share it (DRY) rather than duplicating the == 1 branch.
- Complexity: Low — pluralCount is a two-line branch; checkStateDirSane is a flat switch.
- Modern idioms: Yes.
- Readability: Good — self-documenting, helper doc-comment names the shared rule and its
  cmd/list.go precedent.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

TASK: resume-hooks-silently-lost-9-21 — "The Restore-Exe Rationale Paragraph Is Restated Six Times" (tick-4b2fd9). State the `Exe`/`os.Executable()` trap once, on `StagedHydrateExe`, and reduce the restatements at the constructor and guard sites to what each site itself does.

ACCEPTANCE CRITERIA:
- [x] The `Exe`/`os.Executable()` trap is stated in full at exactly one place in `internal/restoretest`.
- [x] The five reduced comments each describe only their own site and point at the shared statement. (Four, not five — see IMPLEMENTATION notes; the two guard files the task named had been merged into one by task 9-20, which landed immediately before this one.)
- [x] Both guards' `t.Fatalf` wordings are unchanged.
- [x] No non-comment byte changes; the package's unit and integration suites are unaffected by a comment-only edit.

STATUS: complete

SPEC CONTEXT: Phase 9 is an implementation-analysis cycle, so this task's authority is its own body rather than the specification (per the shared verifier context). The underlying fact the comments describe is nonetheless load-bearing for the bugfix's own test estate: `internal/restore`'s `SessionRestorer.hydrateExe` (internal/restore/session.go:326-341) falls back to `os.Executable` when `Exe` is nil, and the restore arms each pane by respawning it into `<exe> state hydrate …` — so an unpinned `Exe` under `go test` arms panes with the test binary. That is what every one of the six restatements was describing.

IMPLEMENTATION:
- Status: Implemented (commit e307619c, comment-only across 5 files: +16 / −22 lines, every changed line a `//` comment).
- Location:
  - Full statement, single home: internal/restoretest/restoretest.go:54-67 — `StagedHydrateExe`'s doc now carries the whole trap (opt-in field → nil/absent falls back to `os.Executable()` → under `go test` that is the test binary → flag parsing stops at the leading `state` positional → the armed pane re-runs the suite inside itself and exits 0, taking the session with it, with no error or log line). Verified accurate against internal/restore/session.go:327-329 and internal/restore/restore.go:26.
  - internal/restoretest/orchestrator.go:11-15 — `FakeHydrateExePath` reduced to what that constant is for ("a pane armed with it dies visibly"), pointing at `StagedHydrateExe`.
  - internal/restoretest/orchestrator_staged.go:14-20 — `NewRestoreOrchestrator` reduced to the pinning it performs, pointing at `StagedHydrateExe`.
  - internal/restoretest/session_restorer_staged.go:12-19 — `NewSessionRestorer` reduced the same way.
  - internal/restoretest/literal_guard_test.go:13-16 — the guard's doc reduced to the rule it enforces, pointing at `restoretest.StagedHydrateExe`.
- Notes:
  - The task body named six restatement sites, two of which were `orchestrator_literal_guard_test.go` and `session_restorer_literal_guard_test.go`. Neither file exists: task 9-20 ("one parameterised restore-literal guard over two descriptors", commit 9a7b54ec) merged both into `internal/restoretest/literal_guard_test.go` before this task ran. The implementation reduced the surviving merged doc comment once instead of twice. This is a divergence from the task's wording with nothing lost — the fact is still stated once and every surviving site points at it.
  - `internal/restoretest/reboot.go:52-54` carries a one-clause site-specific pointer ("pointed at it through `StagedHydrateExe`, which is what keeps a restored pane from respawning into the test binary"). It was not on the task's list and it is not a restatement of the reasoning — it names what that helper does and defers. Consistent with the task's outcome.
  - The one full statement now lives in a file gated `//go:build integration`, while `orchestrator.go` (untagged) and `literal_guard_test.go` (unit lane) point at it. The symbol is reachable in source from both lanes; only a default-tag `go doc` would not render it. Not worth acting on.
  - A repo-wide grep for `os.Executable` finds one further explanation of the same class of trap at internal/bootstrapadapter/adapters.go:69-73, but it justifies a different design decision (a *required parameter* on `NewRestoreAdapter`, with production's use of the fallback called out as intentional) in a different package, and was outside this task's stated scope.

TESTS:
- Status: Adequate (no new tests were due — the task is comment-only and explicitly says "Change no code and no assertion").
- Coverage: The two behaviours the task required to stay green are intact and untouched by this commit:
  - `internal/restoretest/literal_guard_test.go` — the `t.Fatalf` at :32-36 is byte-unchanged (the commit's only hunk in that file is `@@ -11,12 +11,9 @@`, covering the doc comment; the `t.Fatalf` sits at line 32, outside it). Both "stopped looking" assertions survive unedited at :200-201 and :228-229, as does the per-descriptor wording check at :226-236.
  - `internal/restoretest/staged_hydrate_exe_test.go` — the staged-hydrate-exe suite is unedited; its two subtests (path composition, empty-`binDir` fatal via `harnesstest.Recorder`) still pin the behaviour the newly-expanded doc describes.
- Notes: No test in the package reads comment text (`literal_guard_scan_test.go` reads the AST for composite literals, deliberately not the source text — see its `literalsIn` doc at :110-113), so a comment-only edit cannot change any verdict. The restore reboot/roundtrip integration fixtures are likewise untouched by this commit.

CODE QUALITY:
- Project conventions: Followed. No lane change, no build-tag change, no production import added; the edit stays inside test-only `internal/restoretest`.
- SOLID principles: N/A (documentation edit).
- Complexity: Low — no code paths changed.
- Modern idioms: N/A.
- Readability: Good. The one full statement is at the helper every pinned route ends at, states the mechanism and then the observable symptom, and each reduced site now says only what that site does before deferring. The wording divergences the task complained about ("re-runs the suite inside itself" / "re-runs its own suite inside the pane" / "respawns into the suite itself") are gone — one wording remains.
- Issues: None. Every surviving comment claim was checked against the code it describes and holds: the nil-`Exe` fallback (internal/restore/session.go:327-329), the required-parameter contract that `StagedRestoreAdapter` routes through (internal/bootstrapadapter/adapters.go:74-77), and the empty-`binDir` fatal (internal/restoretest/restoretest.go:73-81).

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

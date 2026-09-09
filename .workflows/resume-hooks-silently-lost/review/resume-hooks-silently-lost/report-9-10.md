TASK: resume-hooks-silently-lost-9-10 — Stand-Down Copy Is Written Outside Its Declared Home On Three Surfaces (tick-d17146)

ACCEPTANCE CRITERIA:
- [x] No stand-down phrase is an inline literal in `cmd/run_hook_stale_cleanup.go` or `cmd/doctor.go`; every one composes from a const in the declared home.
- [x] The copy test's lock row carries a non-empty `sharedPhrase` and is exercised by the shared-const subtest rather than skipped.
- [x] `cmd/doctor_test.go` contains no literal copy of a stand-down phrase; each assertion renders through the phrase tables.
- [x] The projects reaper's read-failure copy is declared once and reachable by the phrase guards.
- [x] Every rendered user-facing line is byte-identical to today's.

STATUS: issues_found (0 blocking)

SPEC CONTEXT:
This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification. The
specification does pin the two rendered lines the task touches: §5.1 lists `Skipped stale hook prune: hooks.json is
locked` (specification.md:256), and the 2026-09-01 corrigendum (specification.md:552) adds the `lock-timeout` entry to
`notEvaluableDetails` as `hooks.json is locked (not evaluable)` and withdraws the third §5.1 line, splitting the
empty-live-set guard (`live pane list came back empty`) from the failed enumeration (`could not enumerate live panes`).
Both surviving lock lines are byte-identical to what the code renders today, so the const lift changed no copy.

IMPLEMENTATION:
- Status: Implemented (with legitimate post-task drift — see Notes)
- Location:
  - `cmd/doctor.go:220-225` — the shared phrase const block, now including `lockStandDownPhrase = "hooks.json is locked"` (:225)
  - `cmd/doctor.go:242` — `skippedPrunePhrases[hooksweep.ReasonLockTimeout]` composes from the const
  - `cmd/doctor.go:253` — `notEvaluableDetails[hooksweep.ReasonLockTimeout]` composes as `lockStandDownPhrase + " (not evaluable)"`
  - `cmd/doctor.go:394-399` — `projectStoreReadStandDownPhrase` declared with a comment stating why it names no hooks reason
  - `cmd/doctor.go:404,408` — both `checkStaleProjects` branches render through it; the two bare literals are gone
  - `cmd/doctor_test.go:969,1023,1145,1167,1179,1237,1238,1312` — every previously literal-pinned assertion now renders
    through `phraseFor(notEvaluableDetails, …)`, `renderStaleHooksLine(…)` or `projectStoreReadStandDownPhrase`
  - `cmd/doctor_stand_down_copy_test.go:115` — the lock row's `sharedPhrase`
  - `cmd/doctor_stand_down_copy_test.go:507-551` — `unreadableProjectStore` + `TestStaleProjectsCheckStandDownCopy`
  - `cmd/doctor_stand_down_phrase_guard_test.go:211-326` — the new AST guard: `phraseConstSuffix`,
    `standDownPhraseConsts`, `standDownPhraseRespellings`, `TestStandDownPhrasesAreSpelledOnlyInTheirDeclaredHome`
- Notes:
  - The task body names `cmd/run_hook_stale_cleanup.go`; that file no longer exists — a later phase-9 task moved the
    phrase tables and the `skipReason*` vocabulary into `cmd/doctor.go` / `internal/hooksweep`. The lock const survived
    the move intact (`cmd/doctor.go:225,242,253`), so the criterion holds against the current tree. Judged as sound
    drift, not a loss.
  - AC1 holds in substance: the only remaining bare literals in the two vocabularies are
    `hooksweep.ReasonEmptyPaneRead`'s (`cmd/doctor.go:241,252`), and those two surfaces deliberately say different
    things, so the map entry *is* that phrase's declared home. The struct comment at
    `cmd/doctor_stand_down_copy_test.go:44-46` was correctly narrowed to say so.
  - AC4 is met structurally, not just by convention: `projectStoreReadStandDownPhrase` matches the
    `*StandDownPhrase` suffix the guard derives its const set from (`cmd/doctor_stand_down_phrase_guard_test.go:211,231`),
    so a second production spelling of `could not read projects.json` fails the guard. I enumerated the seven declared
    phrase consts (`cmd/doctor.go:221-225,231,399`) and confirmed no production `.go` file under `cmd/` spells any of
    them outside its declaration, so the guard passes on today's tree.
  - AC5 holds: `lockStandDownPhrase + " (not evaluable)"` and `projectStoreReadStandDownPhrase` reproduce the prior
    literals byte for byte, and no production behaviour changed.
  - No phrase is duplicated outside `cmd` — `internal/` holds none of the five hook phrases.

TESTS:
- Status: Adequate, with one weak subtest inherited from an earlier task
- Coverage:
  - The new AST guard is the load-bearing one and is genuinely strong: it derives its const set from the declarations
    (no list to grow), uses containment rather than equality so a suffixed respelling is caught, exempts the const's own
    literal by AST node identity, fails loudly if it finds no consts (`:303`), and has its own rule test over synthetic
    source (`:313-325`) proving the rule fires. It scans production sources only (`ParsePackageSources(t, ".", false)`),
    which is the right lane — test files legitimately pin rendered copy.
  - `TestStaleProjectsCheckStandDownCopy` (`:524-551`) covers both `checkStaleProjects` stand-down branches, and the
    unreadable-store fixture is correct: a 0o000 parent directory makes `os.ReadFile` fail with EACCES rather than
    ENOENT, so `project.Store.Load` (`internal/project/store.go:42-48`) returns the error instead of an empty slice, and
    the chmod restore is registered after the `t.TempDir()` cleanup so teardown still succeeds.
  - The `doctor_test.go` conversions are not tautologies: `checkStaleHooks` renders through
    `staleHooksNotEvaluable` → `phraseFor(notEvaluableDetails, …)` (`cmd/doctor.go:352-354`), so an assertion against
    `phraseFor(notEvaluableDetails, <reason>)` still discriminates *which reason* the branch chose. That discrimination
    is protected by the per-reason distinctness subtest at `cmd/doctor_stand_down_copy_test.go:364-379`.
  - The plan's named test `"it renders the lock stand-down from the shared const on both surfaces"` has no dedicated
    test; the lock row's enrolment into the existing shared-const subtest is what stands in for it. That subtest is
    tautological (see FINDINGS) — but the substance the criterion wanted is delivered by the AST guard added in the
    same task, which does catch an inline re-authoring of either lock entry.
- Notes: Unit lane, no build tag, no tmux, no daemon, no `t.Parallel()` — correct per CLAUDE.md. Source scanning routes
  through `internal/sourceguardtest` rather than a hand-rolled walk, as the guard family requires.

CODE QUALITY:
- Project conventions: Followed
- SOLID principles: Good — the rule (`standDownPhraseRespellings`) is separated from the assertion, which is what makes
  the guard's own failure mode testable against synthetic source.
- Complexity: Low
- Modern idioms: Yes (`slices.SortFunc`, `slices.Equal`, range-over-map)
- Readability: Good; every non-obvious choice (containment over equality, why the projects const names no hooks reason,
  why the fixture needs a 0o000 directory) carries its reason in prose.
- Issues: One stale claim — see FINDINGS.

BLOCKING ISSUES:
- None. All five acceptance criteria are met in substance, and no user-facing line moved.

FINDINGS:
- [in-scope] [contained] cmd/doctor_stand_down_copy_test.go:394-396 — the comment says "The declaration-level guard
  cannot see a value re-authored inline with today's words; this reads the entry itself", justifying the subtest at
  `:397`. This task added the guard that *does* see exactly that: `TestStandDownPhrasesAreSpelledOnlyInTheirDeclaredHome`
  (`cmd/doctor_stand_down_phrase_guard_test.go:299`) flags any production literal containing a declared phrase, so
  re-authoring `cmd/doctor.go:242` as `"hooks.json is locked"` or `:253` as `"hooks.json is locked (not evaluable)"`
  now fails it — the second case is the guard's own synthetic fixture (`:314-320`). Meanwhile the subtest the comment
  justifies cannot see it: with `sharedPhrase: lockStandDownPhrase` (`:115`), `:402` compares `lockStandDownPhrase`
  against `lockStandDownPhrase` and `:405` asks whether `lockStandDownPhrase + " (not evaluable)"` contains
  `lockStandDownPhrase` — both hold by construction for every row the subtest does not skip, so it passes whether the
  map entry composes from the const or respells it. Correct the comment to credit the AST guard (whose own comment at
  `cmd/doctor_stand_down_phrase_guard_test.go:295-298` states the opposite: "nothing at runtime can tell the two
  apart"), and say what the runtime subtest actually pins — that the two vocabularies agree on the shared words. —
  FAILS: two comments in the same package now assert opposite things about which check catches an inline respelling, so
  a maintainer trusting the copy test's version would read the AST guard as redundant and could delete the only check
  that provides the protection, leaving the tautological subtest behind as its apparent replacement.

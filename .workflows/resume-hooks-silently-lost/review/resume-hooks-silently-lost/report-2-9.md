TASK: resume-hooks-silently-lost-2-9 — "One Home For The Hook-Set Test Setup" (tick-30285b, severity: duplication)

Move `hooksFileInTempDir` and `runHookSet` out of `cmd/hooks_pane_token_test.go` into the package's shared
test-helper file, widen the first to return the temp dir alongside the file path, and re-point the sites that
open-code the temp-dir-plus-env preamble and the reset-and-execute block.

ACCEPTANCE CRITERIA:
- [x] Both helpers live in the shared test-helper file
- [x] The temp-dir-plus-env preamble and the reset-and-execute block are not open-coded where the helper serves
- [x] Sites needing the directory itself still get it
- [x] The unit-lane test count for package `cmd` is unchanged
- [~] Both lanes pass (judged by reading — no execution performed; see TESTS)

STATUS: complete

SPEC CONTEXT:
The specification governs the pane-token hook key (§2.2 lazy stamping at `hook set`, §4.1 the probe→read→mint→
stamp→write ordering, §4.2/§4.3 the `hook rm` exit rule) — it says nothing about test-helper placement, and it
does not need to. This is an implementation-analysis consolidation task generated inside phase 2, so its
authority is its own body: a pure movement of two `cmd`-package test helpers, explicitly "no new test", "no
subtest gains or loses an assertion". Verified against the task body, not the spec. The subtests being
re-pointed are the phase's own §4.1/§2.2 stamping cases plus the pre-existing `hook set`/`hook rm`/`hook list`
suites — none of their subjects is touched by the move.

IMPLEMENTATION:
- Status: Implemented (delivered by commit f30c7015 "one home for the hook-set test setup")
- Location:
  - `cmd/testhelpers_test.go:166` — `hooksFileInTempDir(t, body) (dir, hooksFile string)` now lives here.
  - `cmd/testhelpers_test.go:176` — `runHookSet(t, command)` now lives here.
  - `cmd/testhelpers_test.go:1-4` — the file header stating the split this move rests on (staging helpers here;
    subject vocabulary in a file named for its subject), with the counterpart header added to
    `cmd/hookkey_vocabulary_test.go:1-3`.
  - Re-pointed sites: 36 in `cmd/hooks_test.go` and 9 in `cmd/hooks_pane_token_test.go` at the delivering commit.
- Notes:
  - Both helper declarations are gone from `cmd/hooks_pane_token_test.go`; `grep -rn "func hooksFileInTempDir|
    func runHookSet" cmd/` returns only the two `cmd/testhelpers_test.go` declarations, so there is exactly one
    of each in the package.
  - No open-coded preamble survives where the helper serves: at the delivering commit neither
    `cmd/hooks_test.go` nor `cmd/hooks_pane_token_test.go` contains the string `hooks.json"` at all, i.e. every
    `t.TempDir()` + `filepath.Join(dir, "hooks.json")` + `t.Setenv("PORTAL_HOOKS_FILE", …)` triple was replaced.
  - The nine surviving `resetRootCmd()`/`SetArgs` blocks in `cmd/hooks_test.go` (lines 292, 313, 338, 362, 381,
    388, 408, 465, 703) all drive the `hooks` **alias** verb, and one of them (`{"hooks", "set"}` at line 362) is
    the missing-`--on-resume` case. `runHookSet` drives the canonical `hook` verb and always supplies
    `--on-resume`, so it does not serve those sites — the criterion's "where the helper serves" is respected
    rather than stretched.
  - Sites needing the directory get it: `cmd/hooks_test.go` staging a blocker file and a mode-0500 state dir
    both take `dir, hooksFile := hooksFileInTempDir(...)`, and `cmd/hooks_pane_token_test.go:113` takes
    `denied, _ := hooksFileInTempDir(t, nil)` before chmod-ing that directory to 0000.
  - Path equivalence holds at every rewritten site I checked. The two dirty-flag cases that previously composed
    `filepath.Join(dir, "hooks.json", "state")` now compose `filepath.Join(hooksFile, "state")` — the same path,
    since `hooksFile == filepath.Join(dir, "hooks.json")`. The "it does not touch when the write fails" case now
    has `PORTAL_HOOKS_FILE` set before the directory is created at that path rather than after; the command runs
    later either way, so the precondition it stages is unchanged.
  - Import hygiene is correct: `path/filepath` was dropped from `cmd/hooks_pane_token_test.go` (zero
    `filepath.` uses remain there) and `bytes` was kept (2 uses remain); `path/filepath` and `bytes` were added
    to `cmd/testhelpers_test.go`, which uses both.
  - Later phases evolved both helpers past this task (a `body map[string]map[string]string` parameter routed
    through `hookstest.StageStore`; `runHookSet` now returns the captured output alongside the error). That is
    legitimate onward movement, and the helpers are still single-declaration and still in the shared file. The
    one direct `t.Setenv("PORTAL_HOOKS_FILE", …)` left in the hooks suites — `cmd/hooks_read_lock_test.go:98-99`
    — is a `Staging{SidecarAbsent: true}` case, which the helper's own doc comment names as the case that calls
    `hookstest.StageStore` directly. Consistent, not a residual.

TESTS:
- Status: Adequate (the existing cases are the coverage, as the task states)
- Coverage: The task adds and removes no test, by design. The pre-existing subtests are the coverage: a pure
  movement is verified by them continuing to hold.
- Notes:
  - "No subtest gains or loses an assertion" holds. Reading the whole diff of `cmd/hooks_test.go` and
    `cmd/hooks_pane_token_test.go`, every hunk removes preamble lines and adds a helper call; no assertion,
    `t.Fatalf`, `t.Errorf` or seam installation is added, removed or reworded.
  - "The unit-lane test count for package `cmd` is unchanged" holds for the files the task touched: counting
    `t.Run(` plus top-level `func Test` across the commit boundary gives `cmd/hooks_test.go` 44 → 44 and
    `cmd/hooks_pane_token_test.go` 11 → 11; `cmd/testhelpers_test.go` and `cmd/hookkey_vocabulary_test.go`
    declared no tests before or after. No other test file was touched.
  - "Both lanes pass" is not directly verifiable here (no execution). Read for the failure modes a move of this
    shape can produce: every call site uses the widened two-result form (no one-result call survives anywhere
    under `cmd/`), no site leaves `dir` or `hooksFile` declared-and-unused (which would not compile), no second
    declaration of either helper exists in the package, and `cmd/testhelpers_test.go` carries no build
    constraint — so the helper is visible to the integration-tagged files in `cmd` as well as the unit-lane
    ones. Nothing in the change touches production code, tmux, or a subprocess, so neither lane's isolation
    invariants are engaged.
  - Not over-tested: the task correctly adds no test for the helpers themselves.

CODE QUALITY:
- Project conventions: Followed. The helpers sit beside `writeHooksJSON`/`readHooksJSON` in the file CLAUDE.md
  names as the `cmd` package's staging home, both call `t.Helper()`, and neither assigns a `*Deps` or function
  seam directly (so `cmd/seam_guard_test.go` is unaffected). No build tag is introduced or dropped, so the
  lane rule is untouched. The header comment on `cmd/testhelpers_test.go` follows an existing repo pattern —
  several other files (`cmd/doctor_stand_down_copy_test.go`, `internal/hooksweep/helpers_test.go`,
  `internal/prefs/read_shared_test.go`, among others) carry the same file-role comment above the package clause.
- SOLID principles: Good. `hooksFileInTempDir` stages, `runHookSet` drives; neither does the other's job, and
  `runHookSetForKey` (`cmd/hooks_test.go:717`) composes the two rather than restating either.
- Complexity: Low. Both helpers are straight-line, four and six statements respectively.
- Modern idioms: Yes. Named results `(dir, hooksFile string)` are used for documentation on a two-string return
  and paired with an explicit `return dir, hooksFile` — the idiomatic form for this shape rather than a naked
  return.
- Readability: Good. Each helper carries a doc comment that says what it stages and why the directory comes
  back with the file; the file header states the staging-versus-vocabulary split the move rests on, and the
  counterpart comment on `cmd/hookkey_vocabulary_test.go` makes the other half of that split legible from the
  file itself.
- Comment accuracy: The comments hold. "The directory is returned alongside the file because several callers
  stage siblings of the hooks file in it" is true of three sites. The file header's claim that subject
  vocabulary lives elsewhere is true of the file's contents — it declares no fake and no domain value beyond
  the `lockBound` staging constant. No comment references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

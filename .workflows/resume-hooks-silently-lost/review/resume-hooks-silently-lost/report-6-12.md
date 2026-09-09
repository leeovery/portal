TASK: resume-hooks-silently-lost-6-12 — "Single-Source The hooks.json Staging Helpers, Sidecar Decision Included" (tick-7db8f2, phase 6, implementation-analysis consolidation task)

ACCEPTANCE CRITERIA:
- Package `cmd` has one hooks.json staging helper; package `internal/hooks` has one deny-writes helper.
- Every fixture's sidecar state is set by an explicit field, never by which helper was picked.
- No call site creates a sidecar by hand.
- No doc comment claims a split is necessary that is not.

Tests (from the task body):
- Unit: a fixture with `sidecar: false` degrades on read (records the degradation breadcrumb); one with `sidecar: true` does not. Both directions asserted.
- The re-pointed suites passing with an unchanged verdict count.

STATUS: complete

SPEC CONTEXT: This is a phase-6 consolidation task, so its authority is its own body rather than the specification. The subject still sits directly on specified behaviour: §6.2 makes the `hooks.json.lock` sidecar a real, never-unlinked artefact created only by a mutation acquire, and §6.5 makes the degrade-by-side rule turn on whether that sidecar can be taken — reads fall through to an unlocked read and emit `op=load-unlocked` at DEBUG, writes fail. A fixture's sidecar state therefore decides which side of that rule the test under it exercises, which is exactly what the task makes explicit instead of inherited. CLAUDE.md records the resulting decision in both the `hookstest` architecture row and the "Resume hooks" section.

IMPLEMENTATION:
- Status: Implemented (delivered in 2617ab1d; subsequently extended by later phase-7/8 tasks that lifted the `cmd` helper into the cross-package `internal/hookstest.StageStore`. Judged against the code as it stands, all four criteria hold).
- Location:
  - `internal/hookstest/staging.go:55` — `StageStore`, the single stager; `Staging.SidecarAbsent` (`internal/hookstest/staging.go:42`) is the explicit field, applied at `internal/hookstest/staging.go:85-89`; `Staging.WritesDenied` (`internal/hookstest/staging.go:46`) is the single deny-writes route.
  - `cmd/testhelpers_test.go:166` — `hooksFileInTempDir`, the only cmd-side composition over the stager, adding just the `PORTAL_HOOKS_FILE` pointing.
  - `cmd/hookkey_vocabulary_test.go:27` — `hooksBody`, which preserved `seedHooksJSON`'s key-list ergonomics as the task's Do item 2 allowed (JSON seed built from the key vocabulary).
- Notes:
  - Criterion 1: `seedHooksJSON`, `newTempHooksStore`, `newStagedHooksStore`/`hooksStoreStaging`, `readOnlyDirPath` (the hooks one) and `seedThenDenyWrites` are all gone — a repo-wide grep for those five names returns no `.go` match. `internal/hooks`'s eight deny-writes fixtures (`store_test.go:480,1012,1070,1232,1370`, `cleanstale_read_sentinel_test.go:53`, `lock_test.go:107,239`) all route through `hookstest.Staging{WritesDenied: true}`. (`internal/project/store_logging_test.go:17` keeps a same-named `readOnlyDirPath` for the projects store — a different package and outside this task's Do list.)
  - Criterion 3: `CreateHooksSidecar` has exactly one caller, `internal/hookstest/staging.go:88`. The hand-rolled compensation the task named at `cmd/hooks_read_lock_test.go:33` and its three-line comment are gone; the site now reads `hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})` (`cmd/hooks_read_lock_test.go:21`).
  - Criterion 4: the false "the seed write must succeed before the directory is locked, so this cannot use readOnlyDirPath" claim was corrected in the commit and the helper carrying it has since been removed outright; nothing in the surviving staging code asserts a necessary split.
  - Do item 1 (the modelling decision the fold forces) was made and recorded, and the record survived the later move verbatim: the `SidecarAbsent` doc comment (`internal/hookstest/staging.go:32-41`) states why present-by-default models a written-to install and names the absence as a state an install genuinely holds (no sidecar until the first mutation; the config-directory migration moves `hooks.json` without one). That reasoning is consistent with the code and with CLAUDE.md's stronger real-world claim — a fixture that seeds entries is modelling an install some mutation wrote, and the fixtures whose subject is the absence set the field.
  - Substitution fidelity is exact in both directions where the two old helpers differed on an empty argument: `seedHooksJSON(t)` wrote `{}` and became `Seed: hooksBody()` = `"{}"` (file present, empty registry); `newTempHooksStore(t, "")` wrote no file and became `Seed: ""` (no file at all). Neither fixture's state was silently flipped by the fold.

TESTS:
- Status: Adequate.
- Coverage: The both-directions pin the task asked for exists and is the guard that keeps the field load-bearing: `internal/hookstest/staging_test.go:18-37` asserts the default stages the sidecar and that a read under it leaves no `load-unlocked` record, and `internal/hookstest/staging_test.go:39-55` asserts `SidecarAbsent: true` stages none and that the read degrades (`AssertDegradedRead`, which pins exactly one DEBUG record with `op=load-unlocked` and the caller's `via`). `internal/hookstest/staging_test.go:57-76` additionally pins the ordering the deny-writes fold depends on — the sidecar exists before the chmod, so the mutation fails at `fileutil.ErrWriteTempCreate` rather than earlier at the sidecar's own open, which is the property the corrected doc comment claims.
- Notes: The task's own `TestStagedHooksStoreSidecar` (added to `cmd/hooks_read_lock_test.go` in this commit) was re-homed onto the stager itself by the later cross-package consolidation. That is where the behaviour now lives, so the guard is correctly placed: if the default stopped staging a sidecar, `TestStageStore/it stages a hooks.json with its sidecar by default` fails rather than a comparison test quietly degrading against a degraded baseline. Not over-tested — the sidecar assertions are two subtests, one per direction, with no redundant restatement at the ~30 re-pointed call sites.
- Verdict-count parity could not be re-measured here (running the suite is outside this review's remit); read for correctness, no re-pointed fixture's subject changed in a way an assertion would notice — the read-only doctor assertions compare `hooks.json` bytes (`cmd/doctor_test.go:1561-1576`, `cmd/doctor_stand_down_copy_test.go:252-284`), not directory listings, and the two `dirListing` fixtures that do compare listings either stage nothing at all (`cmd/hooks_read_lock_test.go:51-66`) or ask for the absence explicitly (`cmd/hooks_read_lock_test.go:94-110`).

CODE QUALITY:
- Project conventions: Followed. Unit lane throughout (no portal binary built, no daemon spawned, no real tmux). Test-only helpers stay in test-only packages; `hookstest` remains reachable from both lanes. The default-sidecar decision is mirrored in CLAUDE.md's `hookstest` row and "Resume hooks" section, so the binding record and the code agree.
- SOLID principles: Good. One stager describes a fixture declaratively; the orthogonal states (seed shape, sidecar, denied writes, unreadable) are independent fields rather than helper variants.
- Complexity: Low. `StageStore` is a flat switch over mutually exclusive seed forms with two explicit guards against contradictory descriptions.
- Modern idioms: Yes.
- Readability: Good. Each field's doc states the install state it models and why, so a call site reads as a description of the fixture rather than as a helper choice.
- Issues: None that clear the reporting bar.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

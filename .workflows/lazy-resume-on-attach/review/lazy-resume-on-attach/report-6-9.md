TASK: Doctor Reports the Mode the Helper Would Resolve (lazy-resume-on-attach-6-9, tick-2779da) — one shared resolver `installResumeModeOf` in `cmd` behind both `portal doctor`'s resume-mode line and the hydrate helper's install-wide mode read

ACCEPTANCE CRITERIA:
- With `prefs.json` holding `"resume_mode":"eager"`, then `"lazy"`, `portal doctor` renders `resume mode: eager` / `resume mode: lazy`, and a restored pane whose registration names no mode resolves to that same mode — both exactly as before the change
- With the `resume_mode` key absent, `prefs.json` absent, unreadable, corrupt, or holding an unrecognised value (`"Eager"`), doctor renders the shipped default and the helper resolves the shipped default — unchanged
- With no prefs store to be had — doctor's `PrefsStore` left nil by a failed build, the helper's `LoadPrefsStore` returning an error or a nil store — both answer the shipped default — unchanged
- `rg -n 'LoadResumeMode\(' --type go --glob '!*_test.go'` reports the definition in `internal/prefs/store.go` and exactly one call site, the shared resolver in `cmd`
- `cmd/doctor_resume_mode_test.go`, `cmd/state_hydrate_lazy_test.go` and `cmd/state_hydrate_test.go` pass without modification — a pure refactor, test semantics untouched

STATUS: issues_found

SPEC CONTEXT: The install-wide resume mode lives in `prefs.json` as `resume_mode`, decoded tolerantly: missing, empty, corrupt, unrecognised or unreadable all give the shipped default (lazy), because a panel is answered in a keystroke and an unwanted resume cannot be taken back. Nothing in Portal writes it and `hook list` deliberately leaves it out, so `portal doctor`'s informational `resume mode` line is the only readback. The line never fails a check and never moves the exit code. The hydrate helper resolves each pane's mode from the registration's own mode, then the install's, then the default. The task makes the doctor line and the helper's decision one piece of code.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_hydrate.go:217-224`: `installResumeModeOf(store *prefs.Store) resumemode.Mode`. A nil store answers `resumemode.Default`; anything else answers `store.LoadResumeMode()` with the error discarded. It carries one comment, and that comment is true: `LoadResumeMode` returns `resumemode.Default` beside its error (`internal/prefs/store.go:213-217`).
  - `cmd/state_hydrate.go:205-215`: `installResumeMode` keeps the `cfg.LoadPrefsStore` seam (unset takes `loadPrefsStoreNoMigrate`), passes nil to the resolver on a load error, and returns the resolver's answer. A `(nil, nil)` load also reaches the nil branch.
  - `cmd/doctor.go:363-366`: `checkResumeMode(store *prefs.Store)` keeps its parameter and renders `installResumeModeOf(store).String()` with status `checkInfo`. The comment that justified the discard here is gone.
  - `cmd/doctor.go:129-136`: `resolveDoctorDeps`'s prefs construction is unchanged, and it still leaves the store nil on a build failure.
- Notes: Reading the code settles that the change preserves behaviour. Before and after, both callers give nil or failed-load → Default, and any other store → `LoadResumeMode` with the error dropped. The helper's `resolveResumeDecision` (`cmd/state_hydrate.go:196-200`) still feeds `installResumeMode(cfg)` into `resumemode.Resolve`, so a registration naming no mode inherits exactly what doctor prints. Neither `cmd/doctor.go` nor `cmd/state_hydrate.go` calls `LoadResumeMode` anywhere else. Both measured call sites now go through the resolver, and the only remaining call is `cmd/state_hydrate.go:222`. The comment left on `installResumeMode` (`cmd/state_hydrate.go:202-204`) states the policy, not the discard, and it matches the code.

TESTS:
- Status: Adequate
- Coverage:
  - New `cmd/install_resume_mode_test.go` pins the resolver directly: nil store → Default, eager/lazy → the stored mode, and a corrupt file → Default.
  - The unchanged caller suites cover each acceptance criterion end-to-end:
    - `cmd/doctor_resume_mode_test.go:55-109` covers doctor with eager, lazy, the key absent, an absent file, an unreadable file (DenyRead), a corrupt file, an unrecognised `"Eager"`, and `checkResumeMode(nil)`.
    - `cmd/state_hydrate_lazy_test.go:141-168` covers the helper inheriting an eager install.
    - `cmd/state_hydrate_lazy_test.go:317-342` covers a prefs read that fails (the store path is a directory) and a store that cannot be built (`(nil, err)`).
  - Both callers now share the resolver, so either caller's suite catches a regression in it.
- Notes: The new unit test's "cannot be read" subtest never reaches the read-error branch it names (see FINDINGS). The caller suites cover that branch, so it is not an overall coverage gap. Beyond that, the new file is short and not over-tested.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, no seam assignments in tests, and the read-only doctor path keeps the non-migrating loader.
- SOLID principles: Good. One resolver owns the rule, and each caller keeps only its own way of getting a store.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the test finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/install_resume_mode_test.go:37 — The subtest "it answers the shipped default when the store cannot be read" stages `{not json` (line 39). The prefs store reads that file successfully: `readFile` decodes corrupt content tolerantly to a zero record with a nil error (`internal/prefs/store.go:152-155`). So `LoadResumeMode` returns `(Default, nil)` and the resolver's error-discarding branch never runs. Fix: stage a store whose read actually fails, e.g. `prefs.NewStore(t.TempDir())`. `os.ReadFile` on a directory fails with an error that is not ErrNotExist, and `cmd/state_hydrate_lazy_test.go:319-321` already stages it this way. Keep the corrupt-file case under its own name if wanted. — FAILS: the subtest would still pass if the resolver stopped discarding the read error and answered something else on it (e.g. `Unset` or `Eager`), so the resolver's own unit test does not cover the one behaviour its comment documents. Today only the doctor DenyRead subtest and the lazy directory-store test catch that regression.

UNSETTLED:
- "`rg -n 'LoadResumeMode\(' --type go --glob '!*_test.go'` reports the definition in `internal/prefs/store.go` and exactly one call site, the shared resolver in `cmd`" — Reading confirms the definition at `internal/prefs/store.go:213`, that both measured call sites (doctor, hydrate) now route through `installResumeModeOf`, and that `cmd/state_hydrate.go:222` is the only call in either file. Confirming no other production call site exists anywhere in the tree needs the grep itself to be run.
- "`cmd/doctor_resume_mode_test.go`, `cmd/state_hydrate_lazy_test.go` and `cmd/state_hydrate_test.go` pass without modification" — Reading shows these suites still target signatures that are unchanged (`checkResumeMode(nil)`, the `LoadPrefsStore` seam, `hydrateCfg`). Settling it needs the suites run and the task's commit diffed, to confirm none of the three files changed.

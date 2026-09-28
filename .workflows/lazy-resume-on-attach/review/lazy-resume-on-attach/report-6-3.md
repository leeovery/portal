TASK: Doctor Reports the Install Resume Mode (lazy-resume-on-attach-6-3, tick-5f2afd)

ACCEPTANCE CRITERIA:
- A `prefs.json` holding `"resume_mode": "eager"` renders `eager`; one holding `"lazy"` renders `lazy`.
- An install that never set the key renders the shipped default (`lazy`) rather than an empty cell.
- An unreadable file, a corrupt file, an unrecognised value and a nil `PrefsStore` all render that same default, reached by the accessor's own fallback rather than by a branch of this check's own.
- A `prefs.json` carrying `appearance` is byte-identical after the run — the diagnosis takes the non-migrating read and dispatches no translation.
- The line's status is informational: it never fails, never changes the exit code, and is excluded from both the passed and the total the summary reports.
- The rendered value is the install default alone — a `hooks.json` carrying registrations pinned the other way does not change the line.
- `portal doctor` writes nothing to `prefs.json` on this path.

STATUS: issues_found

SPEC CONTEXT: The install-wide mode lives in prefs.json as `resume_mode` and decodes tolerantly: missing, empty, corrupt, unrecognised and unreadable all resolve to the shipped default, lazy (spec 3.1, 2.1). It has no UI, and it is deliberately kept out of `hook list`, so doctor reports it on an informational line (spec 3.4). That line, like the pending count beside it, takes `checkInfo`: it never fails, never moves the exit code, and is excluded from the passed and total counts (spec 8.1). The spec leaves the wording to the executor, within doctor's `name: detail` shape, the on-disk spelling, and no tool named.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/doctor.go:363-366 — `checkResumeMode(store *prefs.Store)`. It returns `checkInfo` unconditionally and renders `installResumeModeOf(store).String()`.
  - cmd/doctor.go:339-342 — appended in `runDoctorDiagnosis` directly after `checkPendingResumes`, at the end of the catalog.
  - cmd/state_hydrate.go:217-224 — `installResumeModeOf`, the helper the hydrate helper already uses to decide whether a pane waits. A nil store gives `resumemode.Default`. Otherwise it returns `store.LoadResumeMode()` and discards the error, which is safe because the accessor answers Default alongside it.
  - internal/prefs/store.go:209-222 — `LoadResumeMode`. A read error gives Default. A tolerant decode that fails yields a zero record, and `resumemode.Parse` then refuses it, so that gives Default too. Default is Lazy (internal/resumemode/resumemode.go:31).
  - cmd/doctor.go:129-136 — the store is still resolved only through `loadPrefsStoreNoMigrate`. The task added no new seam and no new read.
  - README.md:248 — the informational-lines sentence now names the resume-mode line beside the pending-resume one.
- Notes: The implementation reuses the hydrate helper's decision function instead of calling `LoadResumeMode` inline. That is a sound change from the Do text and better than it: doctor now reports the exact rule the restore path acts on, so the two cannot drift apart. The default arrives by the accessor's fallback. The check has no branch of its own, and the only nil guard sits in the shared helper, which the task's Do explicitly allows. A pinned registration cannot reach the line, because the check never consults the hooks store. The line reads `resume mode: eager|lazy`: doctor's shape, the on-disk spelling, no tool named. No drift from spec 3.1, 3.4 or 8.1.

TESTS:
- Status: Adequate
- Coverage (cmd/doctor_resume_mode_test.go:52-205):
  - Persisted eager and lazy are driven through a real `portal doctor` Execute. The store is resolved by `resolveDoctorDeps` itself, not injected.
  - Absent key, absent file, unreadable file (mode 0000, skipped only when still readable), corrupt file and unrecognised value (`"Eager"`) all expect the default. Every case that holds a value stores `eager`, not the default, so each one tells a real fallback apart from a stored value that was simply read back.
  - The nil store is tested by a direct `checkResumeMode(nil)` call, which also asserts `checkInfo`.
  - The appearance case installs `syncPersistTranslation`, so the migrating loader's write would land before the byte comparison. The comment at :112-113 records why. It asserts prefs.json is byte-identical.
  - An extra case asserts no prefs.json and no sibling file is created when none existed (criterion 7).
  - Summary and exit: "7 checks passed" under both modes. A direct diagnosis with the resume-mode result removed yields the same `doctorUnhealthy` verdict and the same `doctorCheckCounts`.
  - A hooks.json registration pinned eager under a lazy install still renders `lazy`.
  - Position: the line directly follows the pending line and is the last line before the summary.
  - Neighbouring suites were updated for the new tail: cmd/doctor_test.go:777 (TestDoctorCheckOrder), cmd/doctor_pending_resume_test.go:93-95, and cmd/doctor_advisory_test.go:134-160.
- Notes: Each case would fail if its behaviour broke, with one caveat: a migrating loader is caught only because the sync persist is installed, and that is done here. The line's position is pinned four times: diagnosis order, rendered position, relative to the pending line, and relative to the advisory block. Each pin checks a different angle, so this is mild overlap rather than bloat.

CODE QUALITY:
- Project conventions: Followed. The check reuses the existing informational status and the non-migrating prefs route, and adds no new seam, so the source guard over `loadPrefsStoreNoMigrate` call sites still holds.
- SOLID principles: Good. One decision function serves both the hydrate helper and the diagnosis.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: One stale failure message in a test updated for this change (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/doctor_advisory_test.go:136 — The host-terminal assertion's failure message says "second-to-last catalog check", but the result it prints is now `results[len(results)-3]`, the third-to-last. Adding the resume-mode line shifted host down one place. The message was left unchanged, while the pending assertion two lines below correctly took "second-to-last". Fix: change "second-to-last" to "third-to-last" on line 136. — FAILS: when that assertion fires, the diagnostic names the wrong catalog position, and its prefix is identical to the pending assertion's (line 139), so a reader cannot tell from the prefix which assertion failed. Remedy is message text only; non-blocking.

UNSETTLED:
- None

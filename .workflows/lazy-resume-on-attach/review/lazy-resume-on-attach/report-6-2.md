TASK: Doctor Reports the Pending-Pane Count (lazy-resume-on-attach-6-2)

ACCEPTANCE CRITERIA:
- The rendered report carries one line for the pending count, after the pass/fail catalog, with the marker `checkInfo` renders.
- A non-zero count exits zero and does not make `doctorUnhealthy` true — asserted at a count in the low forties.
- The line is excluded from both the passed and the total in the summary: a run whose only change is a non-zero pending count renders the same `N checks passed` line as one with zero pending.
- A read error and a down runtime both render as not evaluable, and neither reports zero pending nor changes the exit code.
- A zero count still renders a line.
- Under `--fix` the line renders in both the pre-repair and the post-repair report, and `runDoctorFix` performs no repair for it.
- The count is panes: a fixture whose view reports two pending panes in one session renders two, not one.
- Doctor starts no server on this path and writes nothing — the check's only call is the injected read.

STATUS: complete

SPEC CONTEXT: Section 8.1 makes pending panes visible as a count in `portal doctor` on an informational line that never fails and never moves the exit code, because a healthy install presents roughly forty-one pending panes after a reboot. Section 8.1 also rules out wiring it like the stale-hook/stale-project counts (which fail when non-zero) and rules out rendering it as an always-true pass (which would pad "N checks passed"). Section 5.2 requires every string on this surface to be tool-agnostic. The plan adds that a down runtime and a failed read report as not evaluable rather than as zero or as a failure.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/doctor.go:65 — `PendingResumes func() (tmux.PendingResumeView, error)` on `DoctorDeps`
  - cmd/doctor.go:101-103 — production default `client.ListPendingResumePanes` filled in `resolveDoctorDeps` alongside the other client-backed seams
  - cmd/doctor.go:339-342 — appended after the conditional host-terminal append (followed by the resume-mode line a sibling task adds)
  - cmd/doctor.go:346-361 — `checkPendingResumes`: down runtime -> `checkNotEvaluable` with `doctorRuntimeNotRunning` (not `runtimeDownResult`'s `checkFail`); read error -> `checkNotEvaluable`; otherwise `checkInfo` with `pluralCount(view.Panes, "pane waiting to resume", "panes waiting to resume")`
  - cmd/doctor.go:190-195 — `runDoctorFix` unchanged; no pending repair
  - internal/tmux/tmux.go:742-771 — the production read is a single `list-panes -a -F` (no server start, no write); `Panes` counts pending rows only, `Sessions` is the distinct-session set
  - README.md:248 — the informational-lines sentence placed after the host-terminal sentence, stating the pending-resume line never affects the exit code
- Notes: `doctorUnhealthy` (cmd/doctor.go:609-616) and `doctorCheckCounts` (cmd/doctor.go:620-633) already exclude `checkInfo` and `checkNotEvaluable`, so every status the check can return sits outside the exit code and the passed/total counts. The check's signature takes only `serverUp` and the read, so the injected read is its only possible external call. The name (`pending resumes`) and detail ("N panes waiting to resume") state what is waiting, use `pluralCount`, and name no tool.

TESTS:
- Status: Adequate
- Coverage: cmd/doctor_pending_resume_test.go:72-247 contains the eight planned subtests.
  - Informational line: an exact rendered line using `checkMarker(checkInfo)`, positioned after the host-terminal line.
  - Forty-one panes: a nil Execute error and `doctorUnhealthy` false.
  - Summary counts: identical `7 checks passed` at 0 and at 41. This would catch both a `checkPass` wiring (8 checks) and a `checkFail` wiring (7 of 8).
  - Failed read: the exact not-evaluable line, a nil error and an unmoved summary.
  - Down runtime: the not-evaluable line, zero read calls, and exit verdict and counts matching the catalog without the line.
  - Zero pending: the line still renders, and the read is called exactly once.
  - `--fix`: the line renders twice, the read is called exactly twice (so no repair calls it), and no Pruned/Skipped line appears.
  - Panes, not sessions: two panes in one session render 2.
  - Elsewhere: `TestDoctorCheckOrder` (cmd/doctor_test.go:770-786) pins the catalog position. `TestResolveDoctorDepsMergeConvention` (cmd/deps_merge_convention_test.go:59-61, 201-214) pins the seam's injection and its production fall-through.
- Notes: Each test fails if the behaviour it names breaks. Nothing is redundant beyond the deliberate direct `doctorUnhealthy` assertion that the criterion calls for.

CODE QUALITY:
- Project conventions: Followed. The seam follows the `*Deps` merge convention through `resolveDoctorDeps`, tests stage through `withDoctorDeps`/`runDoctorWith`, and there is no `t.Parallel()`.
- SOLID principles: Good. The single-purpose check depends on an injected read.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comment above `checkPendingResumes` holds true against the code, including the reason it avoids `runtimeDownResult`.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

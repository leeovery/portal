TASK: 1-3 — Manually verify the honest no-op end-to-end in the reported reproduction setup (remote SSH/mosh trigger + host-local terminal client on the same tmux session → confirm zero host windows open; both TUI multi-select and CLI multi-target burst surfaces; plus `portal doctor` line and a local-only control check).

ACCEPTANCE CRITERIA (from plan):
- [ ] Binary under test built from the branch carrying the Task 1-1 fix (`go build -o portal .`).
- [ ] Reproduction preconditions established: a remote triggering client AND a host-local client attached to the same session at detection time.
- [ ] TUI picker burst from the remote client opens ZERO host-local windows and shows the honest no-op copy; the proactive `m`-entry block is confirmed active.
- [ ] CLI `portal open <a> <b>` burst from the remote client opens ZERO host-local windows (atomic no-op) with the shared NULL copy.
- [ ] `portal doctor` from the remote client reports "unsupported (remote session)" for the host-terminal line.
- [ ] Control: a local-only trigger from the host-local terminal still drives (windows open locally) — no regression.
- [ ] The verification outcome is recorded (pass/fail with observations).

STATUS: complete

(Task is complete as a DEFERRED manual-verification task. No blocking issues. The manual e2e itself has NOT been performed — see the non-blocking carry-forward note. The task's automated decision-surface obligations are fully met by the unit tests in T1-1/T1-2.)

SPEC CONTEXT:
Spec "Verification scope" (specification.md §Testing Requirements, ~L133) explicitly states that the real multi-client end-to-end scenario "is out of unit-test reach and easy to miss in manual testing. It should be verified manually in the reported reproduction setup once the fix lands." The spec deliberately scopes the automated coverage to the selection/locality-ordering decision surface (seeded `clientLister`/`walker`/`reader` fakes) and carves the live multi-client confirmation out as a manual step. The behavioural contract the manual step confirms — mixed remote-most-active + local-idle resolves NULL → honest no-op across the CLI burst, TUI burst, and `portal doctor` surfaces, while a local trigger still drives — is what T1-1's inversion and T1-2's over-correction guard pin at the unit level.

IMPLEMENTATION:
- Status: Deferred (legitimate). No code artifact is produced by this task by design — it is a human end-to-end verification.
- Deferral commit: 62898477 ("T1-3 — defer manual e2e verification"). Rationale recorded in the message: "running portal locally would interfere with the live daemon, and the decision-surface is already pinned by the unit tests in T1-1/T1-2. Deferred to release testing." This is coherent with the project's ABSOLUTE INVARIANT that a test/verification must never mutate the developer's live tmux server or live `portal state daemon` — a full remote+local multi-client run of the real binary against the developer's live server carries exactly that risk, so deferring to a controlled release-testing environment is a sound engineering decision, not a shortcut.
- Tracking state: tick `tick-d11c21` = `cancelled`; manifest `completed_tasks` lists `...-1-3` (deferred). The dual representation is intentional (the task cycle is closed / not-being-done-this-cycle) but the two records use different words for the same fact — noted below so it is not mistaken for a discrepancy.
- The underlying fix this task would confirm IS present: `internal/spawn/detect_inside.go` implements select-winner-then-locality-gate (`selectTriggeringClient` picks max-`Activity`, first-listed on tie; only the winner is walked via `walkToBundle`; empty→clean NULL; list-clients error→transient). The docstring is fully rewritten to the new contract. This matches the spec's fix and is what the manual step would exercise end-to-end.

TESTS (automated decision-surface coverage — assessed by reading, not executed):
- Status: Adequate. The automated coverage this task depends on lives in T1-1/T1-2 and is present in `internal/spawn/detect_inside_test.go`:
  - T1-1 primary bug regression: "it returns NULL when the most-active client is remote even with a local bystander" (601/Activity 9999 remote + 501/Activity 1 local → winner 601 walks mosh-server → clean NULL). detect_inside_test.go:108-114.
  - T1-1 fail-safe reframe: "it fails safe to NULL when the most-active winner walk transiently fails" (601/Activity 100 flaky winner + 501/Activity 50 resolvable local never walked → NULL + `errors.Is(ErrDetectTransient)` + `errors.Is(psFailure)`). detect_inside_test.go:212-245.
  - T1-2 over-correction guard: "it drives the local client when it is most-active despite an idle remote bystander" (601/Activity 50 remote listed first + 501/Activity 200 local second → resolves com.mitchellh.ghostty). detect_inside_test.go:123-130.
  - Retained invariants all present (pure-remote NULL, single-local drive, 2+ all-local max-activity, first-listed tie, list-clients failure transient, single-client walk-failure transient, empty→NULL).
- Coverage: By reading, the three target scenarios and the retained invariants are internally consistent with the `detect_inside.go` implementation (winner selection + winner-only walk + three-shape `walkToBundle` propagation), so the suite exercises exactly the decision surface the spec assigns to automated coverage. Every branch the deferred manual step would confirm (mixed→NULL, local-most-active→drive, transient winner→NULL+WARN) is pinned.
- Notes: No new test is (or should be) added by this task — the spec and task both state the automated coverage comes from T1-1/T1-2. Per the review checklist, the absence of a new test here is NOT a finding. Tests were assessed by reading only (project rule: no test execution in review).

CODE QUALITY:
- Project conventions: N/A — no code changed by this task.
- SOLID / Complexity / Modern idioms / Readability: N/A (verification task, no artifact). For context, the fix it confirms (`detect_inside.go`) is clean: single-responsibility `selectTriggeringClient` helper, DI seam intact, three-shape contract reused verbatim from `detectOutsideTmux`.
- Issues: None.

BLOCKING ISSUES:
- None. The automated decision-surface obligations for this task are met (T1-1/T1-2). The deferral is documented, coherent with project isolation invariants, and does not represent a code defect.

NON-BLOCKING NOTES:
- [idea] Release-testing action item (no file — carry-forward): the manual remote+local multi-client end-to-end verification has NOT yet been performed. Its acceptance criteria (real remote trigger + host-local client → zero host windows across TUI burst, CLI burst, `portal doctor`; local-only control still drives; outcome recorded pass/fail) remain OUTSTANDING and were explicitly deferred to release testing (commit 62898477). Add this to the release-testing checklist for `remote-trigger-spawns-on-local-terminal` so the live confirmation is not lost between merge and release. This is the one substantive gap the reviewer/user must be aware of: the code behaviour is unit-proven, but no human has yet observed the real N−1-windows-do-not-open outcome on real hardware.
- [do-now] Tracking records (`.tick/tasks.jsonl` tick-d11c21 / manifest.json `completed_tasks`): the task is `cancelled` in tick but listed as completed in the manifest — the same "deferred" fact recorded with two different words. Harmless, but a one-line note ("deferred to release testing") on whichever record is the source of truth would prevent a future reader reading it as an inconsistency. Documentation-only; no logic impact.

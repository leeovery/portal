TASK: skip-bootstrap-when-warm-5-2 — Log the underlying error on abridged saver revive failure to restore diagnosability parity (tick-e9364a, chore, Phase 5 Analysis Cycle 2)

ACCEPTANCE CRITERIA:
- A failed BootstrapPortalSaver on the abridged path emits exactly one bootstrap-component WARN carrying the underlying error before adding the SaverDownWarning.
- The SaverDownWarning sink behaviour and the command-proceeds-anyway control flow are unchanged (still no error return).
- The successful-presence early return emits no WARN.
- go build ./... succeeds and go test ./cmd/... is green.

STATUS: Complete

SPEC CONTEXT:
Spec §"Abridged EnsureSaver hard-failure" (specification.md:276) makes the abridged EnsureSaver liveness-only: a failure to re-ensure an absent saver surfaces as a soft SaverDownWarning via the existing bootstrapWarnings sink and the command proceeds; no kill-barrier runs. The spec is silent on logging that failure — so the pre-task behaviour (drop the error, surface only the canned warning) was literally spec-conformant but diverged from the project's established convention that every bootstrap failure path is an observable WARN carrying its cause (spec:125 already treats a latch-write failure as "a pure log line (WARN under the bootstrap component)"; CLAUDE.md: saver/daemon lifecycle "have closed event catalogs"). This chore closes that diagnosability gap to match the full-bootstrap step-5 sibling (bootstrap.go:380, o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/abridged_saver.go:55-58 (WARN added at :56, before bootstrapWarnings.Add at :57); doc comment "Failure posture" updated at :28-37; inline rationale comment at :50-54.
- Notes:
  - WARN emitted via the package-level bootstrapLogger (log.For("bootstrap"), state_common.go:22) — no logger parameter added, exactly as the task directed. bootstrapLogger is already used elsewhere in the cmd package.
  - Attr shape matches the sibling: single "error" attr keyed to err. "error" is a sanctioned closed-vocabulary attr key (used pervasively, incl. bootstrap.go:380 and internal/log itself). Message string "abridged EnsureSaver: saver revive failed" is a message, not an attr — no vocabulary violation. Verified no closed-attr-key guard rejects it.
  - No new import needed (bootstrapLogger is package-level; file already imports only bootstrap + tmux) — build is sound.
  - Presence early-return (:46-48) untouched; warning-sink funnel and no-error-return posture preserved.
  - Ordering note (non-issue): the full-bootstrap sibling appends the warning THEN logs (bootstrap.go:379-380); the abridged path logs THEN appends (:56-57), per the task's explicit "before adding the SaverDownWarning" instruction. The two sinks are independent, so ordering has no observable effect. Both acceptable.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/abridged_saver_test.go:205 TestEnsureSaverLiveness_LogsWarnWithUnderlyingErrorWhenReviveFails — swaps a logtest.Sink via log.SetTestHandler, drives the saver-absent/revive-fails commander, asserts exactly one line matching {"WARN","component=bootstrap","abridged EnsureSaver","error="} AND that exactly one SaverDownWarning is still funneled AND ensureSaverLiveness returns without an error. Covers acceptance criteria 1 and 2.
  - cmd/abridged_saver_test.go:231 TestEnsureSaverLiveness_LogsNoWarnWhenSaverPresent — present+alive saver, asserts zero WARN lines and empty warnings sink. Covers acceptance criterion 3 (early-return emits no WARN).
  - Pre-existing TestEnsureSaverLiveness_FunnelsSaverDownWarningWhenReviveFails (:180) still guards the sink behaviour independently.
  - logtest.Sink render contract confirmed to emit "component=bootstrap" for a log.For-bound logger (internal/logtest/capture_test.go:55-65), so the substring assertions can actually match.
- Notes:
  - Well-scoped, no redundancy: the two new tests split the on-failure and on-success branches cleanly; no excessive mocking (reuses the shared saverAbsentReviveFailsCommander scaffold).
  - The failure-branch WARN assertion matches only "error=" (attr presence), not the underlying error VALUE — see NON-BLOCKING NOTES. Adequate to prove an error attr is carried, but stops just short of proving it is the underlying cause rather than any error.

CODE QUALITY:
- Project conventions: Followed — component-bound package-level logger, closed "error" attr, message mirrors the sibling breadcrumb intent, doc comment kept in sync with behaviour.
- SOLID principles: Good — single-responsibility helper unchanged in shape; one breadcrumb added.
- Complexity: Low — one added statement, no new branch.
- Modern idioms: Yes.
- Readability: Good — the added inline comment (:50-54) and the updated "Failure posture" doc paragraph make the WARN's purpose and its parity with the full-bootstrap step explicit.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/abridged_saver_test.go:217 — the revive-failure WARN assertion matches only "error=" (attr key presence). Add the underlying error's rendered content (the commander returns errors.New("create denied") — likely surfaced wrapped) to the countLines substrings so the test proves the WARN carries the UNDERLYING cause, not merely some error attr. Routed as quickfix rather than do-now because the exact rendered value may be wrapped by createPortalSaverWithRetry, so the substring choice carries a small risk of not matching and should be validated against real render output.

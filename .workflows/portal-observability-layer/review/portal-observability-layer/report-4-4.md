TASK: Add the SIGKILL-escalation DEBUG breadcrumb in escalateKillToSIGKILL (portal-observability-layer-4-4)

ACCEPTANCE CRITERIA:
1. DEBUG breadcrumb fires ONLY on the IdentifyIsPortalDaemon escalation (SIGKILL-firing) branch, never the skip-WARN branch.
2. Carries target_pid = the SIGKILL'd PID.
3. Immediately-preceding statement to SendSIGKILL; adjacency-invariant comment updated.
4. Renders under component saver.
5. DEBUG (filtered at INFO default); discard-logger safe.
6. Skip-branch WARN unchanged; escalation poll/return unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation → gap-closure sites (804): escalateKillToSIGKILL named defect (no breadcrumb on SIGKILL path); fix = DEBUG breadcrumb beneath the saver: kill-barrier escalated INFO. § Lifecycle taxonomy: kill-barrier escalated is saver INFO with target_pid + reason. DEBUG = decision-point inputs, off in production.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/portal_saver.go:435-468; breadcrumb at :452 saverLogger.Debug("kill-barrier escalating to SIGKILL", "target_pid", priorPID).
- Notes: Skip branch (err!=nil || result!=IdentifyIsPortalDaemon) → skip-WARN via Barrier.Logger, no breadcrumb, unchanged. Escalation branch order: identity-check → escalated INFO (:451) → DEBUG breadcrumb (:452) → SendSIGKILL (:453). target_pid = priorPID. saverLogger = log.For("saver"). Adjacency-invariant comment (422-434) documents the two permitted intervening log lines + "do not delete" guard. log.For never nil → discard-safe.

TESTS:
- Status: Adequate
- Coverage: portal_saver_test.go (EmitsDebugBreadcrumbWithTargetPIDOnEscalationBranch: 1 breadcrumb DEBUG component=saver target_pid=4321; NoBreadcrumbOnSkipBranch: table IdentifyDead/NotPortalDaemon/transient → 0 SIGKILL + 0 breadcrumb; BreadcrumbEmittedBeforeSIGKILL: SendSIGKILL seam snapshots count=1). Phase-5 cross-check portal_saver_lifecycle_events_test.go:540 (breadcrumb present once + escalated INFO precedes DEBUG, both precede SIGKILL).
- Notes: Seams point at real production fields → would fail if breadcrumb removed/moved/leaked/rebound. Recording handler merges bound component attr (meaningful assertion). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (saverLogger = log.For("saver"); terse message + target_pid attr; no Sprintf; no t.Parallel; swapSeam + t.Cleanup).
- SOLID: Good — single decision-point breadcrumb; sink injected via seam.
- Complexity: Low (one added statement).
- Modern idioms: Yes (slog variadic; discard-safe).
- Readability: Good — expanded adjacency-invariant docstring makes the ordering + do-not-delete rationale explicit.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

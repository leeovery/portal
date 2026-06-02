TASK: Emit saver kill-barrier lifecycle INFO events (started, escalated, placeholder died) (portal-observability-layer-5-8)

ACCEPTANCE CRITERIA:
1. saver: kill-barrier started target_pid=X once per kill invocation, after prior-PID alive probe confirms live daemon, before kill-session.
2. NOT emitted on no-prior-PID (readErr != nil) or already-dead (!IsAlive) tolerant-kill shortcuts.
3. saver: kill-barrier escalated target_pid=X reason=kill-session-timeout only on IdentifyIsPortalDaemon escalation branch, above the Phase-4 DEBUG breadcrumb, no duplicate breadcrumb.
4. Identity-skip WARN path emits no escalated line, keeps single WARN.
5. saver: placeholder died target_pid=X reason=<signal|exit|unknown> on observed-exit poll branch, closed reason value.
6. At most one WARN per invocation preserved.

STATUS: Complete

SPEC CONTEXT:
Spec § Saver and daemon lifecycle taxonomy (877-887) — three saver INFO rows; reason value spaces closed (902-908). Externally-killed-process footnote (587): killer must record the kill so an unpaired process: start is explainable. Gap-closure table (804): Phase-4 DEBUG breadcrumb sits beneath the escalated INFO.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/portal_saver.go: kill-barrier started :374 (only after IsAlive(priorPID) true; tolerant-kill shortcuts :354-364 return first; before KillSession :375); kill-barrier escalated :451 (IdentifyIsPortalDaemon branch only; identity-skip :437-443 returns after single WARN; directly above Phase-4 DEBUG breadcrumb :452 → SendSIGKILL :453); placeholder died :384 (post-kill-session) + :460 (post-SIGKILL), reason="signal".
- Notes: reason="signal" both observed-exit sites, documented per [needs-info] (barrier observes only liveness; proximate action is signal delivery; exit/unknown reserved closed values barrier never produces). No new reason value. saverLogger referenced (owned by 5-7). No duplicate breadcrumb; no WARN paths altered.

TESTS:
- Status: Adequate
- Location: internal/tmux/portal_saver_lifecycle_events_test.go
- Coverage: EmitsKillBarrierStartedWhenPriorDaemonAlive (one started target_pid=4321, kill-session-time snapshot pins started→kill ordering, 0 WARN); NoKillBarrierStartedOnNoPriorPIDShortcut + WhenPriorDaemonAlreadyDead (both shortcuts, 0 started, 0 WARN); EmitsKillBarrierEscalatedAboveDebugBreadcrumbOnPortalDaemonBranch (escalated reason=kill-session-timeout, SIGKILL-time snapshot, one DEBUG breadcrumb present, escalatedIdx<breadcrumbIdx); NoKillBarrierEscalatedAndKeepsSingleWarnOnIdentitySkip (table IdentifyDead/NotPortalDaemon/Transient, 0 SIGKILL, 0 escalated, 1 WARN); EmitsPlaceholderDiedReasonSignalOnKillSessionExit + OnPostSIGKILLExit; PreservesAtMostOneWarnContractAcrossLifecycleEvents.
- Notes: logtest.Sink captures in true emission order → ordering assertions sound. Each test distinct branch. Would fail if reordered/duplicated.

CODE QUALITY:
- Project conventions: Followed (saverLogger seam; auto-baselines not passed; no t.Parallel; t.Cleanup restore; component=saver).
- SOLID: Good — additive emissions at natural sites, no new abstraction.
- Complexity: Low.
- Modern idioms: Yes (slog attrs).
- Readability: Good — in-source comments explain lifecycle role, reason mapping, escalated-above-breadcrumb invariant + "do not delete" guard.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] placeholder died reason space is {signal,exit,unknown} but implementation only emits signal (both sites), correct per [needs-info] (barrier can't distinguish catch-vs-kill); a one-line note in the spec reason table that kill-barrier sites are signal-only by construction would stop a future reviewer reading the unused enum values as missing coverage.

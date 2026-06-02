TASK: process: exec marker at the AttachConnector bare-shell handoff (portal-observability-layer-2-14)

ACCEPTANCE CRITERIA:
- AttachConnector.Connect emits one process: exec INFO with target=tmux and args=joined tmux argv, immediately before syscall.Exec.
- Marker emitted BEFORE the exec (verified via injected execer ordering).
- SwitchConnector.Connect emits NO process: exec marker.
- Marker under process component.
- args logged verbatim.
- Marker visible regardless of PORTAL_LOG_LEVEL (level-filter bypass).

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants → exec-handoff markers (563-600). Every syscall.Exec site emits a plain exec-terminal INFO immediately before exec, under its owning component, ordinary logger (no helper). Unbuffered writer guarantees bytes reach kernel pre-replace. SwitchConnector out of scope (returns normally → process: exit). args verbatim. "exec" in lifecycle-bypass set.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:103-113 (AttachConnector.Connect); marker at :111 log.For("process").Info("exec","target","tmux","args",strings.Join(argv[1:]," ")) before ex.Exec at :113.
- Notes: Live argv {tmux,attach-session,-t,=name} → args=attach-session -t =<name> (matches [needs-info] "log LIVE argv", spec's -A is illustrative). Component process. Pre-exec comment notes unbuffered (Task 2-7). "exec" in lifecycleBypassMsgs. SwitchConnector untouched (no marker). A SECOND syscall.Exec site — PathOpener.Open (open.go:284) — also emits the same marker; not named in 2-14's scope but spec-mandated by the same "every syscall.Exec site MUST emit" rule. Correct consolidation, not scope creep; flag for traceability.

TESTS:
- Status: Adequate
- Location: cmd/open_test.go:1272-1570
- Coverage: EmitsExecMarkerBeforeExec (exactly one, target=tmux, args=attach-session -t =foo; ordering via orderingExecer snapshot); SwitchConnector_EmitsNoExecMarker (zero); ArgsLoggedVerbatim (multi-word shell tail unredacted); VisibleAtWARN (warnBypassHandler models prod gate+bypass); PathOpener_EmitsExecMarkerBeforeExec_OutsideTmux + InsideTmux_EmitsNoExecMarker.
- Notes: warnBypassHandler re-implements the prod predicate (newTextHandler unexported); prod bypass independently tested in handler_test.go (exec bypasses WARN/ERROR) — end-to-end across two suites. Ordering test fails if marker moved after Exec. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (execer/tmuxPath seams; log.For; no t.Parallel; comments cite spec + Task 2-7).
- SOLID: Good — call-site emission, no logger-owned helper (per spec directive).
- Complexity: Low.
- Modern idioms: Yes (strings.Join; defensive len guard in PathOpener).
- Readability: Good — pre-exec rationale + unbuffered guarantee inline.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] PathOpener.Open (open.go:284) carries the identical spec-mandated marker outside 2-14's named scope with no dedicated task ID; correct/spec-compliant but worth recording for traceability of the "every syscall.Exec site emits" invariant.
- [idea] Exec-marker emission now duplicated across two sites with near-identical comments; spec forbids a logger-owned helper so intentional, but comment drift risk if a third site appears.

TASK: Emit saver create/respawn/ready lifecycle INFO events in portal_saver.go (portal-observability-layer-5-7)

ACCEPTANCE CRITERIA:
1. Create branch: saver: placeholder created tmux_pane=%N after createPortalSaverWithRetry (pid auto-baseline).
2. saver: destroy-unattached off tmux_pane=%N after set-option, on BOTH create and alive happy-path defensive set.
3. Create branch: saver: respawn-daemon from_pid=P to_pid=D tmux_pane=%N around RespawnPane (from_pid pre-respawn, to_pid post-respawn).
4. saver: daemon ready target_pid=D (version auto-baseline) ONLY on readiness-barrier success; timeout keeps WARN, no daemon ready.
5. respawn-daemon and daemon ready NOT emitted on alive happy path.
6. pid-read failure does not abort bootstrap; event fires best-effort, read failure logged with wrapped error.

STATUS: Complete

SPEC CONTEXT:
Spec § Saver and daemon lifecycle taxonomy — four saver INFO events emitted by bootstrap observing saver from outside; additive to process: lines. Closed attrs tmux_pane/from_pid/to_pid/target_pid; pid/version auto-baselines. Co-located in internal/tmux/portal_saver.go (also home of kill-barrier events; BootstrapPortalSaver is bootstrap-invoked).

IMPLEMENTATION:
- Status: Implemented
- Location: portal_saver.go: saverLogger :20; placeholder created :608-609 (create branch, tmux_pane from SaverPaneID); destroy-unattached off :621 (after set-option at 612 both branches; pane queried create :608 / alive :619); respawn-daemon :630-635 (from_pid pre via saverPanePIDBestEffort, to_pid post); daemon ready :506-513 in waitForSaverDaemonReady (success-return only, reuses pid from isSaverDaemonReady). New helpers SaverPaneID (saver_pane_pid.go:83), saverPanePIDBestEffort (:656 WARN-on-failure-return-0).
- Notes: All six ACs met. Best-effort: pane/pid read errors swallowed (empty/0), bootstrap never aborts; pid-read miss logs saver respawn: pane-pid read failed w/ wrapped error. = exact-match prefix. No new keys.

TESTS:
- Status: Adequate
- Location: internal/tmux/portal_saver_lifecycle_events_test.go (+ scaffolding portal_saver_test.go, export_test.go)
- Coverage: all eight cataloged tests — placeholder created w/ tmux_pane; destroy-unattached off create + alive; respawn-daemon w/ from_pid/to_pid/tmux_pane; daemon ready w/ target_pid on success; no daemon ready + WARN on timeout; neither respawn-daemon nor daemon ready (+ no placeholder created) on alive; respawn-daemon best-effort from_pid=0 + wrapped-error log on pane-pid read failure. SaverPaneID unit tests pin exact argv + error propagation.
- Notes: Asserts level=INFO, exact attrs, exactly-one cardinality (onlySaverEvent). version-baseline criterion verified by call-site contract (sink doesn't inject baselines → confirms version NOT passed). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (log.For("saver"); closed snake_case attrs; no t.Parallel; seam swaps t.Cleanup; best-effort observability never aborts bootstrap).
- SOLID: Good — saverPanePIDBestEffort separates WARN-and-default policy from rich-sentinel saverPanePID; SaverPaneID focused query.
- Complexity: Low.
- Modern idioms: Yes (slog, %w, value reuse).
- Readability: Good — create-vs-alive emission split + best-effort contract documented.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] saverLogger docstring (portal_saver.go:18-19) is stale: "Today its sole consumer is the SIGKILL-escalation DEBUG breadcrumb" — after 5-7/5-8 it has many consumers. Update to avoid misleading future readers.
- [idea] Alive happy path queries SaverPaneID (:619) and emits destroy-unattached off, adding one list-panes per healthy bootstrap; matches spec ("destroy-unattached off fires on both"), intended, acceptable at per-bootstrap frequency.

TASK: Extract a log.Took helper to pin the cycle-summary took attr contract (portal-observability-layer-8-1)

ACCEPTANCE CRITERIA:
- internal/log exports Took(start time.Time) slog.Attr returning a slog.Duration attr keyed "took".
- All nine cycle-summary sites use log.Took(start); no production site outside the helper constructs a "took" attr by hand.
- Log output unchanged: took key present with duration value, identical attr ordering (helper in same final position).
- No loop body, counter, or message string altered.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary cadence (810-863): every cycle emits ONE INFO summary ending with the reserved took attr, captured as start := time.Now(), emitted as "took", time.Since(start) (rendered via Duration.String()). Attr-type table (186) pins took as duration. c2 duplication-analysis follow-up to a c1 deferral (rule-of-three exceeded).

IMPLEMENTATION:
- Status: Implemented (exceeds listed scope — see notes)
- Location: internal/log/took.go:15-17 (Took = slog.Duration("took", time.Since(start)), doc declares single source of truth). Migrated nine: state_daemon.go:500, bootstrap/orphan_sweep.go:208, stale_marker_cleanup.go:133, state/fifo_sweep.go:100, restore/restore.go:86, restore/session.go:293, hooks/project CleanStale via storelog/clean_stale.go:53,59. Plus bootstrap.go:283-410 (eleven step complete) + :416 (orchestration complete).
- Notes: Each site keeps own start capture + divergent counters; only the took bookend routes through the helper. Loop bodies/counters/messages untouched. Attr ordering preserved incl the mid-list case (session.go:291-295 places Took before anomalous, matching original). Three remaining hand-written "took" NOT migrated (correctly — not cycle-summary bookends): init.go:152 (process-exit marker over package startTime), state_hydrate.go:237 (per-pane replay duration), :360 (fixed timeout constant). No remaining production "took", time.Since(...) pair (grep clean).

TESTS:
- Status: Adequate
- Coverage: took_test.go (KeyAndDurationKind: key=="took" + Kind==KindDuration; MeasuresElapsedSinceStart >= 5ms). Per-site capture assertions confirm took key+Duration survive the swap (daemon tick, restore geometry/skeleton, three clean sweeps via RequireDuration, storelog helper success-INFO + failure-WARN). hooks/project summary tests pass unmodified.
- Notes: One focused unit test + reused per-site assertions. Would fail if key/stringified/dropped attr regressed. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (internal/log leaf; doc comment; storelog composition avoids log↔fileutil cycle).
- SOLID: Good — single-responsibility helper; storelog respects leaf boundaries.
- Complexity: Low (one-liner).
- Modern idioms: Yes (slog.Duration as final variadic any, flattens identically).
- Readability: Good — doc cites spec + byte-identical-output guarantee.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Migrated more than the nine enumerated sites (bootstrap eleven step complete + orchestration complete also route through log.Took); consistent with the task's stated outcome ("pinned in exactly one place") and the spec catalog (bootstrap steps are cycle summaries), strictly an improvement, beyond the literal "nine sites" instruction — flag for reviewer awareness, not a defect.
- [idea] Spec cycle catalog (861) lists "Retention sweep (rotated logs)" under log-rotate but retention.go emits no took cycle-summary (only per-deletion Info lines) — correctly out of scope here (no bookend to migrate), but suggests a possible separate spec-coverage observation (retention sweep has no terminal summary line). Worth a separate look if not tracked elsewhere. [See aggregate cross-task note.]

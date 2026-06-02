TASK: Single-winner retention sweep with per-deletion breadcrumbs and sentinel prune (portal-observability-layer-2-8)

ACCEPTANCE CRITERIA:
- portal.log.swept.<today> exists → gated sweep returns immediately (nothing run, nothing emitted).
- Winner emits exactly one INFO log-rotate: deleted path=<file> retention=<N> per deleted file, BEFORE os.Remove.
- Files date < cutoff deleted; within window kept.
- Invalid PORTAL_LOG_RETENTION_DAYS → fallback 30 + one WARN.
- portal.log.<pid>.symlink.tmp and portal.log.swept.<date> never deleted by cutoff walk (strict date-parse skip).
- Stale swept sentinels (date != today) pruned; today's kept.
- os.Remove failure mid-loop → one WARN, sweep continues.
- Second process same day (sentinel present) → no duplicate breadcrumb.

STATUS: Complete

SPEC CONTEXT:
Spec § Retention policy (446-483). Sweep runs first Handle of each date after today's file opened, behind O_CREAT|O_EXCL portal.log.swept.<today> single-winner gate (prevents reboot-storm 32× duplicate breadcrumbs). Steps 0-3: gate, cutoff, strict-date-parse delete-with-INFO-breadcrumb, prune stale sentinels. Accepted partial-sweep risk on mid-loop SIGKILL (self-heals). clean --logs reuses same sweep gate-bypassed cutoff=today.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/retention.go (runRetentionSweep entry gated+env-resolved; runRetentionSweepWithDays shared steps 0-3; resolveSweepRetentionDays env+fallback WARN; claimSweepGate step 0 O_CREAT|O_EXCL; retentionCutoff step 1; deletePastCutoff step 2 INFO-before-Remove+WARN-continue; pruneStaleSentinels step 3; SweepLogsForClean for 2-9). Wired at sink.go:93-100 (dayRoll → runRetentionSweep) fired only on dateChanged after fd in place + symlink swung; first-Write path also fires gated sweep. names.go sweptSentinelFile (date slot "swept" rejected by pastDayLogDate). config.go resolveRetentionDays. rotate.go pastDayLogDate reused (2-5).
- Notes: Signature resolves retentionDays internally (cleaner — Step 1 WARN requires owning env resolution; not drift). INFO-before-Remove ordering exact, kill-safety rationale documented (breadcrumb in today's open unbuffered file even if killed before Remove).

TESTS:
- Status: Adequate
- Location: internal/log/retention_test.go (one per AC) + sweeplogs_test.go (2-9)
- Coverage: ReturnsImmediatelyWhenGateLost; EmitsInfoBeforeEachRemove (component/path/retention attrs); DeletesOlderKeepsWithinWindow (boundary == kept, < deleted, incl .N); FallsBackTo30WithWarnOnInvalidEnv (verbatim raw); NeverDeletesSymlinkTmpOrSweptSentinel (retention=0 most aggressive); PrunesStaleSweptSentinelsKeepsToday; WarnsAndContinuesOnRemoveFailure (removeFunc seam); SingleSourcesBreadcrumbsAcrossConcurrentStartups; UngatedAlwaysRunsRegardlessOfSentinel; RunsRetentionSweepOnRealDayRoll (integration guards seam wiring).
- Notes: Behaviour-focused, component assertions real (sticky component from For). removeFunc seam minimal. Partial-sweep self-heal documented accepted risk, not unit-testable.

CODE QUALITY:
- Project conventions: Followed (no internal/state import; removeFunc seam; no t.Parallel; For("log-rotate"); reuses pastDayLogDate DRY).
- SOLID: Good — decomposed helpers; runRetentionSweepWithDays single shared algorithm for gated + --logs.
- Complexity: Low.
- Modern idioms: Yes (CutPrefix, Glob, injectable nowFunc).
- Readability: Excellent — gate/breadcrumb-before-delete/accepted-risk rationale documented.
- Security/Perf: sentinel 0600; 2 Globs per sweep, linear.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] deletePastCutoff re-parses date already strict-parsed by pastDayLogDate (dead err branch); have pastDayLogDate return parsed time.Time to remove it.
- [idea] claimSweepGate swallows all open errors as "lost gate"; a non-EEXIST error (ENOSPC/EACCES on state dir) skips silently — an optional log-rotate WARN on the non-EEXIST branch would make a misconfigured state dir observable.
- [quickfix] Render note: spec shows raw="<v>" quoted; code emits raw as string attr, quoting is the handler's concern — no action, cross-reference only.

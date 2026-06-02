TASK: Past-day chmod 0400 immutability sweep on the day-roll path (portal-observability-layer-2-5)

ACCEPTANCE CRITERIA:
- On day roll, yesterday's portal.log.<yesterday> + all .N segments chmod 0400.
- After multi-day downtime, files from every past day sealed in one sweep.
- portal.log.<pid>.symlink.tmp and portal.log.swept.<date> never chmod'd (strict date-parse skip).
- File already 0400 skipped (no redundant chmod).
- Today's file + today's same-day .N NOT sealed.
- chmod failure emits one WARN under log-rotate; sweep continues.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism step 2d (chmod 0400 past-day files not today/not already 0400, strict date-parse skip of temp/sentinel/non-log) + step 3 (same-day overflow NOT sealed — peer may hold O_APPEND fd) + Resolved operational edges (multi-day catch-up one sweep) + Invariant 1.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/rotate.go:51 (sealPastDayFiles), :87 (pastDayLogDate strict parse), :111 (isAllDigits); wired at sink.go:95-98 (newRotatingSink sets dayRoll), fired only on dateChanged==true at sink.go:338-342 (reopen).
- Notes: Gating correct — dayRoll fires only via reopen(today, true) (date advance or first-ever write, documented idempotent/safe); never on same-day inode-mismatch reopen(today, false). Strict parse rejects three skip families (temp dateSeg=<pid>; sentinel dateSeg=swept; non-log). Trailing segment must be all-digits. Same-day exclusion via date==today filter; multi-day catch-up falls out of date!=today predicate (no special logic). chmodFunc test seam mirrors existing seams.

TESTS:
- Status: Adequate
- Location: internal/log/rotate_test.go
- Coverage: all six ACs — yesterday base+segment sealed on real clock day roll (today stays writable); all yesterday segments; every past day one sweep across multi-day gap; temp+sentinel+non-log skipped; already-0400 skipped (chmod-must-not-be-called guard); today + same-day .N NOT sealed; WARN-and-continue (failure on one of two, other still sealed, exactly one chmod-failed WARN under log-rotate w/ path+error attrs).
- Notes: Behaviour (file modes, log records); drives real clock seam + newRotatingSink wiring. Would fail if seam unwired. No over-mocking.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; chmodFunc seam + t.Cleanup; For("log-rotate") binding; no internal/state import).
- SOLID: Good — sealPastDayFiles single responsibility; pastDayLogDate strict-parse factored and REUSED by 2-8 retention (single source of truth, no duplication).
- Complexity: Low.
- Modern idioms: Yes (CutPrefix, Glob, time.Parse-as-validator, octal FileMode).
- Readability: Good — precise rationale comments (symlink temp writable, same-day not sealed, best-effort).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] retention.go:179-184 re-parses the date string pastDayLogDate already proved parses (guaranteed-success re-parse); having pastDayLogDate return the parsed time.Time would remove it. Touches 2-8 code; cosmetic micro-opt on a once-per-day path.

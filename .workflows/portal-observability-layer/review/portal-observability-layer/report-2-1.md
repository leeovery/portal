TASK: Parse PORTAL_LOG_ROTATE_SIZE and PORTAL_LOG_RETENTION_DAYS once at handler init (portal-observability-layer-2-1)

ACCEPTANCE CRITERIA:
- resolveRotateSize("") → (524288000, default); accepts 500M/500m/1G/1g/512K/bare bytes (source=env, binary count); falls back for abc/5X/-1/0/1.5M.
- resolveRetentionDays("") → (30, default, ""); accepts 7/0/365 (source=env); falls back (30, fallback, verbatim) for -1/366/400/abc/3.5.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism (default 500MB cap, K/M/G suffixes, parsed once at handler init) + § Retention policy (default 30 days, invalid → default + startup WARN). Task carves two pure functions; WARN/threshold use deferred to 2-8/2-6. Binary (1024) multipliers per task (500 MiB = 524288000), documented in-source.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/config.go (defaultRotateSize :12, suffix consts :14-20, resolveRotateSize :34, splitRotateSize :62, defaultRetentionDays :81, maxRetentionDays :85, resolveRetentionDays :99). Wired at init.go:122 + retention.go:107.
- Notes: Pure functions; env read at call site. Shape matches Phase-1 level resolver (reuses sourceDefault/Env/Fallback). WARN correctly NOT here (lives at retention.go:108-109). 0 accepted for retention (delete everything older than today, source=env); negative rejected. int64 overflow guard before multiply (defensible). TrimSpace before parsing.

TESTS:
- Status: Adequate
- Location: internal/log/config_test.go
- Coverage: every AC value + boundaries (0/365, >365, fractional, double-suffix M/1MM/500MG). Behaviour-focused (values+source+raw), would fail if multipliers became decimal-1000.
- Notes: Concise tables, no redundancy, no mocking.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; pure-fn + call-site os.Getenv).
- SOLID: Good — single responsibility; splitRotateSize extracted; DRY source consts.
- Complexity: Low.
- Modern idioms: Yes (TrimSpace, ParseInt/Atoi, multi-return source label).
- Readability: Good — comments explain binary base, zero-rejection, purity/seam.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] splitRotateSize indexes s[len(s)-1] after caller guarantees non-empty; a defensive empty guard (or doc tweak) would make it safe in isolation. Current path correct.
- [idea] "5kb" multi-char input correctly fails (fallback) but isn't in a test table; minor optional coverage.

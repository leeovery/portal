TASK: session-tagging-and-grouping-1-6 — Expose `@portal-dir` via ListSessions (Session.Dir)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: read @portal-dir in same list-sessions -F pass (append #{@portal-dir}); absent/empty → empty Dir; format-field count change handled; embedded pipe in value preserved.

SPEC CONTEXT: spec § The stamp — grouped render reads @portal-dir in same list-sessions pass, no git rev-parse per render. Path may contain literal `|` so must occupy unbounded trailing slot.

IMPLEMENTATION: Implemented.
- tmux.go:30-40 Session.Dir field with godoc; tmux.go:198 format string extended with trailing #{@portal-dir}; tmux.go:194-240 parser uses SplitN(line,"|",4), parts[3] captures trailing (preserves embedded pipes), len!=4 guard updated. Single production source of truth.

TESTS: Adequate. tmux_test.go:140-197 — TestListSessionsParsesPortalDir (all 3 edge cases: populated, absent trailing-empty, embedded pipe); TestListSessionsFormatStringIncludesPortalDir (locks read-in-same-pass); existing matrix updated to 4-field; call-order assertion updated. Real-tmux round-trip (6-4) covers quoting drift. Not over-tested.

CODE QUALITY: Conventions followed (MockCommander DI, single-source format string); SOLID good (minimal extension); low complexity; SplitN idiom correct; inline rationale comment. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.

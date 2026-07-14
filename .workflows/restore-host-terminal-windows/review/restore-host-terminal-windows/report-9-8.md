TASK: restore-host-terminal-windows-9-8 — Fix the --spawn-ack flag help text delimiter label (tick-c4daec)

ACCEPTANCE CRITERIA:
- The `--spawn-ack` help text names the marker `@portal-spawn-<batch>-<token>` (hyphen) and no longer implies a colon in the marker name.
- The flag name, default value, and value parsing are unchanged.

STATUS: Complete

SPEC CONTEXT: Analysis-cycle standards finding (severity low, cosmetic). The `--spawn-ack` flag help previously read "write the @portal-spawn-<batch>:<token> ack marker" — a colon between batch and token — but the written tmux server-option name is `@portal-spawn-<batch>-<token>` (hyphen, per `SpawnMarkerName`). The colon is only the flag-VALUE delimiter (`FormatSpawnAckFlag` → `<batch>:<token>`). The two delimiters were conflated in the help copy. Text-only correction, no behavioural change.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/attach.go:93
- Notes: Help string now reads `"internal: <batch>:<token> — write the @portal-spawn-<batch>-<token> ack marker before attaching"`. The marker name uses the hyphen form `@portal-spawn-<batch>-<token>`, byte-for-byte consistent with `SpawnMarkerName` (internal/spawn/ackid.go:54-56 → `SpawnMarkerPrefix + batch + "-" + token`). The flag-value form `<batch>:<token>` (colon) matches `FormatSpawnAckFlag` (ackid.go:77-79 → `batch + ":" + token`), and the two delimiters are now distinctly labelled. Flag name (`spawn-ack`), default (`""`), and parsing (`ParseSpawnAckFlag` colon-cut, cmd/attach.go:43-49) are untouched — verified unchanged. `SpawnMarkerName` / `FormatSpawnAckFlag` in ackid.go are unmodified by this task (they were already the source of truth the help now agrees with).

TESTS:
- Status: Adequate (none required)
- Coverage: Tick correctly specifies no test — this is a help-text copy change with no behavioural assertion. No test in the repo asserts on the exact help string (grep across cmd/ and internal/ found flag-name/value references only, none pinning the help text), so nothing broke and nothing needs updating. Existing `cmd/attach` tests exercise the flag with colon-delimited values (`b1:t1`, cmd/attach_test.go:204/233/258/336) consistent with the unchanged `FormatSpawnAckFlag`, and the malformed-value usage-error path (attach_test.go:298-308) still holds.
- Notes: None.

CODE QUALITY:
- Project conventions: Followed. Cobra flag registration in `init()`, "internal:" framing preserved for a hidden-intent operator flag. Consistent with golang-spf13-cobra conventions.
- SOLID principles: N/A (single string literal edit).
- Complexity: Low (unchanged).
- Modern idioms: N/A.
- Readability: Good — the help now disambiguates value-delimiter (colon) from marker-name-delimiter (hyphen), matching the extensive rationale comments in ackid.go (lines 16-20, 51-56, 74-79).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

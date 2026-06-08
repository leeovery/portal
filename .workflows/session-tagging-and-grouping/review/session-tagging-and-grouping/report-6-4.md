TASK: session-tagging-and-grouping-6-4 — Add an end-to-end @portal-dir stamp → ListSessions(Dir) round-trip integration test

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: end-to-end stamp + read back via ListSessions(Dir); guards tmux quoting/format-string drift; covers path with a space; integration excluded from default run; isolated state env if subprocess spawned.

SPEC CONTEXT: spec §83-113 — stamp @portal-dir, grouped render reads in same list-sessions pass via #{@portal-dir}; stamped value and read must agree exactly (mismatch silently drops session). Only a real-server round trip catches quoting/format drift.

IMPLEMENTATION: Implemented.
- internal/tmux/portal_dir_roundtrip_realtmux_test.go — uses production write client.SetSessionOption(...,"@portal-dir",...) (parity with create.go:96) and production read client.ListSessions() (#{@portal-dir} SplitN parse). Real isolated tmux server via tmuxtest.New (socket + cleanup kill-server). Genuine write→read round trip, not mock. Local portalDirOption const avoids session-package import (documented, self-guarding via round-trip equality).

TESTS: Adequate. Table (plain path, path with space); TempDirWithSpace (real on-disk dir as cwd+stamp); UnstampedSessionEmptyDir (absent → empty Dir, guards trailing-empty-field parse). Real-server failures would trip assertions. Not under/over-tested.

Note on edge cases: no build tag — gates on SkipIfNoTmux(t), matching every existing internal/tmux real-tmux guard (runtime-skip convention satisfies clean-default-run intent). No subprocess spawned (in-process client against isolated socket) so IsolateStateForTest N/A — correctly omitted.

CODE QUALITY: Conventions followed (no t.Parallel, canonical tmuxtest fixtures, _realtmux_test.go suffix); low complexity (table + 2 focused + helper); t.Run/t.TempDir/WaitForSession idiomatic; excellent doc comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [idea] portal_dir_roundtrip_realtmux_test.go:68 — consider a table case for a stamp value with a literal `|` (e.g. /code/a|b). Production parser places @portal-dir in unbounded trailing SplitN slot specifically to survive embedded pipes; no round-trip test exercises that against a real server. Scope decision (separate design property from space-quoting).

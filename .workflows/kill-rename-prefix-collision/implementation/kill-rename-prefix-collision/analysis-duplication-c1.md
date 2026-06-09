# Analysis: Duplication — Cycle 1

STATUS: findings
FINDINGS_COUNT: 2

## Findings

### FINDING 1: Three near-duplicate exact-match-prefix regression tests share an uncentralised RunFunc simulation skeleton
- **SEVERITY:** low
- **FILES:** `internal/tmux/tmux_test.go:443-505` (TestHasSessionUsesExactMatchPrefix), `internal/tmux/tmux_test.go:772-811` (TestKillSessionUsesExactMatchPrefix), `internal/tmux/tmux_test.go:1046-1096` (TestRenameSessionUsesExactMatchPrefix)
- **DESCRIPTION:** This implementation added two regression tests (Kill, Rename) that mirror the pre-existing HasSession one. All three independently re-author the same exact-match simulation: a `MockCommander.RunFunc` with the identical `"=foo"` → error / `"=foo-2"` → ok / `"foo"` → "prefix-match hazard" switch, plus the same `strings.HasPrefix(got[2], "=")` target assertion and the same argv-equality loop. The spec explicitly directed mirroring `TestHasSessionUsesExactMatchPrefix`, so the duplication is intentional and sits exactly at the Rule-of-Three threshold. Bodies are not byte-identical — each switch hardcodes its own verb, and Kill/Rename add their own distinguishing assertions (destructive-error return; bare-newName guard) — so this is near-duplicate logic, not copy-paste. Impact is modest (~10-15 shared lines each).
- **RECOMMENDATION:** Optional, not required by the fix. If consolidating, extract a same-package test helper, e.g. `assertExactMatchTarget(t, verb string, call func(c *tmux.Client) error)` plus a shared `exactMatchCommander(t, verb)` factory returning the `=foo`/`=foo-2`/`foo` RunFunc, both `t.Helper()`-marked. Each test keeps only its distinguishing assertions. Given the per-test divergence and that two of three already exist, leaving as-is is also defensible.

### FINDING 2: Two saver_pane_pid.go list-panes helpers repeat the run/split/trim/first-non-empty-line skeleton
- **SEVERITY:** low
- **FILES:** `internal/tmux/saver_pane_pid.go:48-68` (saverPanePID), `internal/tmux/saver_pane_pid.go:83-94` (SaverPaneID)
- **DESCRIPTION:** Both issue `list-panes -t exactTarget(session) -F <fmt>` then loop `strings.Split(out, "\n")` returning the first non-empty trimmed line, falling back to `ErrEmptyPaneList`. The line-scan loop is duplicated. This duplication is **pre-existing structure** — this implementation only swapped the inline `"="+sessionName` for `exactTarget(sessionName)` inside both calls (behaviour-neutral). The differing element (int parse + `ErrPanePIDParse` vs verbatim string return) is real and the shared block is small (~5 lines).
- **RECOMMENDATION:** No action. Out of plan scope (loop pre-dates this bugfix; only the in-argv helper swap was made here) and below the proportionality bar. Recorded only to confirm it was assessed and deliberately not flagged.

## Summary

The production change (the `exactTarget` helper) is itself the de-duplication and is clean — no inline `"="+name` session targets remain, and the one residual `"="+bareTarget` at `tmux.go:983` is the explicitly out-of-scope window-level `SelectWindow` site. The only new duplication is three near-identical exact-match regression tests now at the Rule-of-Three threshold; consolidating them via a same-package test helper is an optional readability improvement, not required by the fix.

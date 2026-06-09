# Analysis: Architecture — Cycle 1

STATUS: clean
FINDINGS_COUNT: 0

## Summary

Implementation architecture is sound — the `exactTarget` helper placement, unexported visibility, naming, and the Client-method chokepoint approach all compose cleanly; every in-scope session-target site routes through the single primitive, the documented out-of-scope exclusions are coherent (not gaps), and the regression tests verify exact-match behaviour rather than merely pinning argv strings.

Supporting evidence (no action required):

- **API surface / placement.** `exactTarget(session string) string` sits beside its pane-level sibling `PaneTargetExact` (`tmux.go:574` and `:593`), making the two canonical exact-match target builders co-located and self-documenting. Unexported visibility is correct — it is a package-internal argv-construction detail; out-of-package callers pass bare session names and the prefix is applied at the chokepoint. The same-package internal test (`exact_target_internal_test.go`) is the right home for the focused unit assertion.

- **Chokepoint coverage complete.** All session-level `-t` sites the spec scoped in route through `exactTarget`: KillSession (`tmux.go:366`), RenameSession (`:390`, target-only — `newName` correctly stays bare), HasSession (`:136`), HasSessionProbe (`:166`), SwitchClient (`:406`), saverPanePID (`saver_pane_pid.go:49`), SaverPaneID (`saver_pane_pid.go:84`). No inline `"="+name` session-target string remains in the package.

- **Out-of-scope exclusions coherent, not gaps.** Remaining bare `-t <session>` sites (ActivePaneCurrentPath `:344`, SetSessionOption `:427`, the `list-panes -t session` reads, ShowEnvironment `:759`, SetSessionEnvironment `:945`) are all non-destructive reads/sets matching the spec's explicit out-of-scope list; `display-message -t paneID` (`:324`) targets a unique `%N` pane ID and is categorically immune. SelectWindow's inline `"=" + bareTarget` (`:983`) is a window-level target the spec explicitly leaves to implementer discretion.

- **Seam quality / no double-prefix.** Callers of the migrated methods pass bare names; the prefix is applied solely inside the Client method. The only out-of-package `"=" + name` is `cmd/open.go:104`'s `attach-session` argv for the `syscall.Exec` handoff, which bypasses the tmux Client entirely and is outside this work unit. No caller double-prefixes.

- **Behaviour-neutrality proof holds.** The saver-site tests and the migrated HasSession/SwitchClient tests pin the new `=`-prefixed argv and stay green; `go test ./...` is green and `go build ./...` clean.

- **Test quality.** The new regression guards (TestKillSessionUsesExactMatchPrefix `tmux_test.go:772`, TestRenameSessionUsesExactMatchPrefix `:1046`) simulate tmux exact-match resolution via `RunFunc` so a dropped `=` triggers the bare-prefix-match arm and fails loudly — verifying behaviour, not just the string.

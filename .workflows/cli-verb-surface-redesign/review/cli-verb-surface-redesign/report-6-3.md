TASK: cli-verb-surface-redesign-6-3 — Session-name completion (shared helper) for `open` positional, `open -s`, `kill` + exempt completion from bootstrap

ACCEPTANCE CRITERIA:
- `__complete` must NOT trigger bootstrap → add `__complete` to `skipTmuxCheck` (cobra runs root `PersistentPreRunE` for `__complete`).
- The completer builds its OWN client via `tmux.DefaultClient` (`tmuxClient(cmd)` panics without `PersistentPreRunE`).
- Server-down / no sessions → empty completions gracefully (`ListSessions` collapses no-server to empty).
- User-visible set only (leading-`_` filtered — `_portal-saver`/`_portal-bootstrap` never suggested).
- `NoFileComp` so sessions aren't merged with file/dir completion.
- Hidden `--ack`/`state` never appear in completion.
- Session-name completion wired for `open` bare positional, `open -s`, and `kill` positional (spec § Tab Completion).

STATUS: Complete

SPEC CONTEXT:
Spec § Tab Completion: "complete every Portal-owned enumerable namespace; leave the rest to the shell." The slot table specifies session names for the `open` bare positional, `open -s`, and `kill` positional; alias keys for `open -a` (task 6-4); shell for `-p`/`-z`. Rejected: sessions+directories merged into one slot (noisy). Session-domain resolution matches only the user-visible (leading-underscore-filtered) `ListSessions` view (spec § open target resolution, "Session set — user-visible only"). Bootstrap Exemption § applies `skipTmuxCheck` to bootstrap-exempt verbs; Phase-6 task 6-3 extends this to the internal `__complete` verb so a TAB press does not start the tmux server + restore.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/completion.go:23-29 — `completionSessionNames` seam builds its OWN `tmux.DefaultClient().ListSessionNames()`, returns nil on error.
  - cmd/completion.go:40-48 — `completeSessionNames(toComplete)` shared helper: prefix-filters candidates and returns `ShellCompDirectiveNoFileComp`; never calls `tmuxClient(cmd)`.
  - cmd/open.go:1014-1019 — `openCmd.ValidArgsFunction` (bare positional) and `RegisterFlagCompletionFunc("session", …)` both route through `completeSessionNames`.
  - cmd/kill.go:62-67 — `killCmd.ValidArgsFunction` routes through `completeSessionNames`, short-circuiting to nothing once a positional is present (ExactArgs(1)).
  - cmd/root.go:58-68 — `skipTmuxCheck["__complete"] = true`; the doc comment (root.go:49-57) explains cobra runs the root `PersistentPreRunE` with `__complete` as the command, and that `Name()=="__complete"` covers both `__complete` and `__completeNoDesc`.
  - internal/tmux/tmux.go:199-204 — `ListSessions` collapses the no-server `list-sessions` error to `[]Session{}, nil`; tmux.go:243-257 drops leading-`_` names; ListSessionNames (tmux.go:265-275) inherits both.
- Notes: All seven acceptance points are present and correct. `--ack` is `MarkHidden` (open.go:1004) and `stateCmd` is `Hidden:true` (cmd/state.go:18); cobra auto-excludes hidden flags/commands from completion, so both are absent from completion output. The bare-positional completer completes session names at every positional index, which is correct for the multi-target burst (each positional may be an attach target from Portal's enumerable session namespace).

TESTS:
- Status: Adequate
- Coverage:
  - cmd/completion_test.go:27-66 (TestCompleteSessionNames) — all-names + NoFileComp, prefix filter, and the nil-seam (server-down) → empty + NoFileComp + no panic path.
  - cmd/completion_test.go:156-250 (TestCompletionWiring) — open positional, open --session, and kill positional all route through the shared helper and return NoFileComp; kill offers nothing (and does NOT call the seam) once one positional is present.
  - cmd/completion_test.go:257-279 (TestCompletionExcludesInternalSessions) — real per-test `-L` tmux socket (unit-lane fast real-tmux client): `_portal-x` filtered, only `my-work` suggested — proves the user-visible-set-only guarantee end-to-end.
  - cmd/root_test.go:349 — executes the real `["__complete","open",""]` path through `rootCmd.Execute()` and asserts the orchestrator's Run is never called (bootstrap exemption) and no error/panic. Because cmd TestMain poisons TMUX package-wide, this exercises the real `completionSessionNames` → `DefaultClient` seam on the exempt path, proving it degrades to empty (no panic) with no client in context — direct coverage of the "builds its own client, gracefully empty" claims.
  - cmd/open_test.go:3702 + cmd/retired_surface_test.go:153 lock `--ack` as Hidden; cmd/state_test.go:277 locks `stateCmd` Hidden — the transitive basis for "hidden --ack/state never appear in completion".
- Notes: Balanced — no redundant assertions; seams keep the completer tests hermetic while one real-tmux test and one Execute()-level test cover the two integration-flavoured guarantees (underscore filter, bootstrap exemption). Not over-tested. One narrow gap: no test asserts the completion OUTPUT itself omits `--ack`/`state` (it is proven only transitively via the Hidden locks + cobra's framework behaviour) — see non-blocking note.

CODE QUALITY:
- Project conventions: Followed. Injectable package-level seam (`completionSessionNames`) with `t.Cleanup` restore matches the codebase's `*Deps`/seam DI pattern; tests avoid `t.Parallel` per the cmd-package mutable-state rule; the unit-lane real-tmux client test uses a per-test `-L` socket (no daemon, not integration-tagged) per the lane rules.
- SOLID principles: Good. `completeSessionNames` is a single-responsibility shared helper consumed by three call sites (DRY); the tmux touch-point is isolated behind one seam.
- Complexity: Low. Straight prefix-filter loops; clear paths.
- Modern idioms: Yes (`strings.HasPrefix`, `slices.Equal` in tests).
- Readability: Good. The doc comments precisely explain WHY the completer builds its own `DefaultClient` (exempt path, `tmuxClient(cmd)` would panic) and why prefix-filtering is done in-helper (cobra does not prefix-filter dynamic-func returns).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/completion_test.go — add a completion-output test asserting `open`'s flag completion excludes `--ack` and top-level completion excludes the `state` command, so the "hidden --ack/state never appear in completion" edge case is verified directly rather than only transitively via the Hidden locks (open_test.go:3702 / state_test.go:277) plus cobra's framework guarantee.

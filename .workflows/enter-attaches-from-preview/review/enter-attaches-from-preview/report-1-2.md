TASK: enter-attaches-from-preview-1-2 — Apply exact-match `=` target prefix uniformly across HasSession, SelectWindow, SelectPane, SwitchClient, and AttachConnector

ACCEPTANCE CRITERIA: all five argv shapes pinned; non-Enter callers compile; regression test present.

STATUS: Complete

SPEC CONTEXT: Spec § Pre-select + attach sequence > Exact-match target syntax mandates the `=` prefix uniformly on every `-t <session>` call (has-session, select-window, select-pane, attach-session, switch-client). Without uniformity, prefix-collision between a killed `foo` and a live `foo-2` would bypass the bail path. The plan also required keeping `PaneTarget` (no prefix) stable since it doubles as the hooks.json key formatter.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:126-129 `HasSession` — `-t "="+name`
  - internal/tmux/tmux.go:339-345 `SwitchClient` — `-t "="+name`
  - internal/tmux/tmux.go:861-870 `SelectWindow` — composes bare target, prepends `=`; error context uses bare form
  - internal/tmux/tmux.go:881-890 `SelectPane` — uses `PaneTargetExact` for argv, `PaneTarget` for error context
  - internal/tmux/tmux.go:900-909 `ResizePaneZoom` — also updated (uniform application beyond the strict five sites)
  - internal/tmux/tmux.go:479-511 `PaneTarget` / `PaneTargetExact` — sibling helper introduced exactly as the plan suggested; godoc explicitly warns callers about the hooks.json key invariant
  - cmd/open.go:88-102 `AttachConnector.Connect` — argv is `["tmux", "attach-session", "-A", "-t", "="+name]`
- Notes:
  - `PaneTarget` (no prefix) correctly retained as hooks.json key formatter. Restore/session.go, daemon, integration tests keep using it for hook-key construction — the right call: changing the key shape would silently invalidate every existing hooks.json entry.
  - `KillSession` (tmux.go:314-320) still passes `name` bare to `-t`. Plan said "prefer uniform application unless a test surfaces a regression." Not on the Enter-pipeline so non-blocking, but leaves the codebase non-uniform.
  - `RenameSession` similarly uses bare `-t`. Same shape.

TESTS:
- Status: Adequate
- Coverage:
  - `TestHasSession` updated → `has-session -t =my-session` (tmux_test.go:338)
  - `TestHasSessionUsesExactMatchPrefix` (tmux_test.go:384-446) — explicit regression with documentary godoc and a fake commander simulating tmux's exact-match semantics (live `foo-2`, killed `foo`); proves dropping the `=` prefix would prefix-match `foo-2` and bypass bail
  - `TestSelectWindow` (tmux_test.go:2262-2357) — six subtests: exact-match argv, single-call invariant, success, wrapped error, `*CommandError` recovery, explicit `=` prefix
  - `TestSelectPane` (tmux_test.go:2359-2396) — expects `select-pane -t =work:2.3`
  - `TestResizePaneZoom` (tmux_test.go:2398-2435) — pinned to exact-match form
  - `TestSwitchClient` (tmux_test.go:697-728) — expects `switch-client -t =my-session`
  - `TestAttachConnectorConnectArgv` (cmd/open_test.go:1108-1131) — pins `["tmux", "attach-session", "-A", "-t", "=foo"]` with spec-citing godoc
  - `TestPaneTarget` (tmux_test.go:2650+) — pins the non-prefixed hook-key shape so the hooks.json invariant can't regress silently
- Notes:
  - The "killed foo with live foo-2 reports absent" subtest is the strongest piece — proves the resolution semantics actually defeat the collision hazard, not just that argv is correct.
  - Not over-tested. Subtests carry distinct intent.
  - Slight redundancy between `TestHasSession` argv assertion and `TestHasSessionUsesExactMatchPrefix` — intentional and warranted; the latter is the searchable rationale source.

CODE QUALITY:
- Project conventions: Followed. Method-level godoc cross-references spec sections; error context uses bare-target form so log readability isn't polluted with the `=` artefact.
- SOLID: Good. `PaneTarget` / `PaneTargetExact` split observes SRP — one is a hook-key formatter, the other a tmux-argv formatter. Mixing them would have been a silent hooks.json-compat footgun.
- Complexity: Low. All changes are localised string transformations; no new branches, no new state.
- Modern idioms: Yes. Idiomatic `fmt.Sprintf` and `errors.As`-friendly wrapping consistent with the rest of the package.
- Readability: Good. Three godoc blocks (HasSession, HasSessionProbe, PaneTargetExact) explain the prefix rationale with spec citations.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tmux/tmux.go:314-320 `KillSession` still passes `name` bare to `-t`. Plan suggested uniform application. Same hazard shape — killing intended `foo` could prefix-match a coexisting `foo-2`. Worth a follow-up to make `=` truly uniform across every `-t <session>` site.
- [idea] `RenameSession` (same file) similarly bare. Same reasoning, same follow-up candidate.
- [quickfix] `PaneTargetExact` godoc (tmux.go:496-511) enumerates "SelectPane, ResizePaneZoom" as callers — broaden to "callers issuing a `-t` flag at the pane level" to reduce doc-drift hazard.

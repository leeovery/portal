TASK: resume-hooks-silently-lost-6-9 (tick-d7736e) — Delete `ResolveStructuralKey` And `ListAllPanes`, And Correct The Claim CLAUDE.md Makes About Them

ACCEPTANCE CRITERIA:
- Neither method exists; the build and both lanes are green.
- CLAUDE.md's `tmux` row names only surfaces that exist.
- `cmd/bootstrap/stale_marker_cleanup.go`'s path is untouched.

STATUS: complete

SPEC CONTEXT:
The work unit replaces the positional `<session>:<window>.<pane>` hook key with a durable per-pane
token (`@portal-pane-id`). The specification's §1.3 explicitly carves the *structural* siblings out of
the change — `state.SanitizePaneKey` and `internal/tmux`'s `StructuralKeyFormat` are "checked against
the change (§9), not changed by it", because they live for the duration of one bootstrap and are
rebuilt from live coordinates. This task is a phase-6 consolidation item rather than specified work:
after phase 3 retired the positional hook machinery, two `*tmux.Client` methods that resolved/enumerated
the structural shape (`ResolveStructuralKey`, `ListAllPanes`) had no production caller left, while the
constant and the format-taking enumerator that production genuinely uses remained. Its authority is its
own body, and the deletion is consistent with the spec's carve-out: the constant survives, the two
call-shaped conveniences around it do not.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go` — neither `ResolveStructuralKey` nor `ListAllPanes` is declared anywhere in
    the file (read in full, 856 lines). The pane-listing surface the `Client` now exports is
    `ListAllPanesWithFormat` (`internal/tmux/tmux.go:603`) and `ListAllPaneHookKeys`
    (`internal/tmux/tmux.go:658`), the latter composing itself out of the former.
  - `internal/tmux/tmux.go:626` — `StructuralKeyFormat` kept, with its doc unchanged ("It is not the
    hook key — see HookKeyFormat").
  - `cmd/bootstrap/stale_marker_cleanup.go:57` — the production structural path is intact and unchanged
    in shape: `c.Panes.ListAllPanesWithFormat(tmux.StructuralKeyFormat)`, with the pinning comment above
    it still explaining why the format constant is shared. The zero-live-panes mass-unset guard
    (`:66-74`) and the malformed-line skip (`parseLivePaneSet`, `:98`) are untouched.
  - `CLAUDE.md:60` — the `tmux` row now reads "...**separate from** the name-based `StructuralKeyFormat`,
    which now serves only non-hook structural use (`@portal-skeleton-*` markers, cleanup paths) and is
    read through the general-purpose `ListAllPanesWithFormat`". Neither deleted method is named there, and
    neither appears anywhere else in the file. The row's windows/panes list likewise names
    `ListAllPanesWithFormat` rather than a bare `ListAllPanes`.
- Notes:
  - Every other surface the row names is present in the package: sessions (ListSessions/NewSession/
    NewSessionWithCommand/NewDetachedSessionNoCwd/HasSession/KillSession/RenameSession/SwitchClient/
    CurrentSessionName), windows/panes, `HookKeyFormat`/`ResolveHookKey`/`ListAllPaneHookKeys`/
    `PaneHookRow`, the option and environment writers, the target vocabulary (`Target`,
    `SessionTargetExact`, `CoordTargetExact`, `PaneTargetExact`, `windowTargetExact`, `PaneIDTarget`,
    `PaneTarget`, `windowTarget`) and the global-hook trio. The row's mention of `ShowGlobalHooks` is
    explicitly framed as removed, not as an existing surface.
  - No consumer references the deleted names: `internal/bootstrapadapter/adapters.go` and
    `internal/tmux/target_composition_guard_test.go` (whose exactness vocabulary is
    `exactTargetHelpers`, `:100-106`) both name only surviving surfaces, and
    `internal/tmux/export_test.go` exports no seam for either method.
  - Verification method: I read the files rather than running the toolchain (running the suite is
    outside this role). A surviving reference to either deleted method would be a compile error, so the
    only silent residue a deletion of this shape can leave is an orphaned test helper — see TESTS.

TESTS:
- Status: Adequate
- Coverage: `internal/tmux/tmux_test.go` read end to end (2836 lines). It declares no
  `TestResolveStructuralKey`, calls neither deleted method, and its two package-level helpers are both
  still consumed — `syntheticExitError` (`:493`, used at `:451`) and `assertWindowGroups` (`:2598`, used
  at `:2376` and six further sites in `TestListWindowsAndPanesInSession`). So the deletion took its tests
  with it and left no dead helper behind.
- Notes: The surviving surface kept its coverage: `TestListAllPanesWithFormat`
  (`internal/tmux/tmux_test.go:1407-1449`) still pins the exact argv `list-panes -a -F <format>`, the
  raw (untrimmed) return, and the wrapped-error path — which is what would have been lost had the
  `ListAllPanes` cases been deleted by block rather than by case. The task's "both lanes green is the
  test; no new test is warranted for removed code" is the right call: nothing observable changed, so a
  new test would have no subject.

CODE QUALITY:
- Project conventions: Followed. The deletion respects the CLAUDE.md rule that the structural shape and
  the hook-key shape stay separate surfaces, and it narrows the client's exported API to what production
  reaches — no test-only method left on a production type.
- SOLID principles: Good. Removing the two convenience wrappers leaves one enumerator
  (`ListAllPanesWithFormat`) parameterised by the caller's format, with `ListAllPaneHookKeys` composed on
  top of it; that is a smaller interface, not a wider one.
- Complexity: Low — a deletion plus a documentation sentence.
- Modern idioms: Yes.
- Readability: Good. The doc comment on `StructuralKeyFormat` (`internal/tmux/tmux.go:621-626`) and the
  pinning comment at `cmd/bootstrap/stale_marker_cleanup.go:55-57` still state, at both ends, why the
  constant is shared — so the surviving structural path explains itself without the deleted methods.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

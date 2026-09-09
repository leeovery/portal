TASK: resume-hooks-silently-lost-2-6 — Compose The Enumeration's Token Half From The Registration Format (tick-fdbb70, severity: duplication)

ACCEPTANCE CRITERIA:
- [x] `paneHookRowFormat` derives its token half from `HookKeyFormat` rather than recomposing it
- [x] The rendered format string is unchanged — every existing test passes untouched
- [x] The non-reuse of `StructuralKeyFormat` is stated at the site
- [x] Both lanes pass (judged by reading — no behaviour change; see TESTS)

STATUS: complete

SPEC CONTEXT:
§2.1 declares the tmux pane user-option name `@portal-pane-id` **once**, as `state.PortalPaneIDOption`, and enumerates the sites that compose the literal from it: `captureFormat` in `internal/state`, and "`HookKeyFormat` and the all-pane enumeration format in `internal/tmux`" (specification.md:53). Writing the literal in exactly one place is what retired the old `@portal-id` binding guards (§9.4) rather than re-pointing them. The phase's central invariant is that the one-pane registration read (`ResolveHookKey`) and the whole-server sweep read (`ListAllPaneHookKeys`) answer with the same token for the same pane — a hook written under a key the sweep never sees live is reaped within ~10s (§1.1). The spec's wording permits the enumeration format to compose from the constant directly; deriving it from `HookKeyFormat` (which itself composes from the constant) satisfies the same single-literal property more tightly, and is a strengthening rather than a divergence.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:650-651` — `const paneHookRowFormat = HookKeyFormat + paneHookRowSeparator + "#{session_name}:#{window_index}.#{pane_index}"`
  - `internal/tmux/tmux.go:647-649` — the comment recording the deliberate non-reuse of `StructuralKeyFormat`
  - `internal/tmux/tmux.go:631` — `HookKeyFormat = "#{" + state.PortalPaneIDOption + "}"`, the registration format the token half now derives from
  - `internal/tmux/tmux.go:626` — `StructuralKeyFormat`, the neighbour the comment names
- Notes:
  - Byte-identity confirmed by construction: the old token half was `"#{" + state.PortalPaneIDOption + "}"`, which is `HookKeyFormat`'s own definition verbatim; the separator and location halves are untouched. Rendered result is unchanged, so `ListAllPaneHookKeys`' argv and every parse path are unchanged.
  - The comment's factual claim holds: the location half at `:651` is character-for-character `StructuralKeyFormat`'s value at `:626`, so "renders the same shape" is true, and `PaneHookRow.Location`'s own doc comment at `:635-636` independently states the field is display-only and never a key — the two agree.
  - `HookKeyFormat` is genuinely the registration format, not an assumed one: `internal/tmux/resolve_hookkey_test.go:58` pins `ResolveHookKey`'s second call as `display-message -p -t <target> <tmux.HookKeyFormat>`. So the two reads now agree by construction rather than by a real-tmux assertion alone.
  - `state` remains used in the file (`:631`), so the import is not orphaned by the change.
  - "Change nothing else in the file" holds as far as the current content shows: the surrounding declarations (`StructuralKeyFormat`, `PaneHookRow`, `paneHookRowSeparator`, `ListAllPaneHookKeys`, `parsePaneHookRows`) read exactly as their own tasks describe them.

TESTS:
- Status: Adequate (no new test is correct here)
- Coverage:
  - `internal/tmux/pane_hook_rows_test.go:83-100` ("it reads the pane token through the single option constant") drives `ListAllPaneHookKeys` through a scripted commander and asserts the composed argv is a `list-panes -a -F` read containing `"#{"+state.PortalPaneIDOption+"}"` — this is the assertion the refactor had to keep green, and it still holds since the rendered string is unchanged.
  - `internal/tmux/hookkey_cross_site_realtmux_test.go:13-34` asserts `ResolveHookKey` and `ListAllPaneHookKeys` return the same token for each of three panes on a real server — the behavioural guarantee the refactor makes structural.
  - `internal/tmux/pane_hook_rows_test.go:12-81` covers the parse contract (one row per pane, unstamped rows retained, first-separator-only split, malformed row errors with nil rows, non-nil empty slice on empty output) — all unaffected.
  - `internal/tmux/list_all_pane_hookkeys_realtmux_test.go:11-53` covers the real-tmux stamped/unstamped mix and the read-failure propagation.
- Notes:
  - No test pins `paneHookRowFormat`'s literal value, so nothing needed re-pointing — consistent with the task's "no new test" instruction. Adding one would over-test: it would assert the compiler's own concatenation.
  - Lane placement is correct: the `*_realtmux_test.go` files are client-only per-test `-L` socket tests, which CLAUDE.md places in the unit lane; nothing here builds or spawns a `portal` binary.
  - Judged by reading, both lanes are unaffected — the change alters no rendered string, no argv, no exported surface and no control flow.

CODE QUALITY:
- Project conventions: Followed. The single-literal rule of spec §2.1 is preserved and tightened; the comment carries no task id, phase or spec section reference (per the repo's no-process-artifacts rule).
- SOLID principles: Good — no structural change; the dependency added is a constant-to-constant one in the correct direction (enumeration derives from registration, which is the authority).
- Complexity: Low — one const expression.
- Modern idioms: Yes — untyped const concatenation, gofmt-stable across the two-line wrap.
- Readability: Good. The comment answers the exact question the site provokes ("why is this string repeated 25 lines below its constant?") at the place a reader meets it, rather than in a distant doc.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

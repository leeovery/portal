TASK: resume-hooks-silently-lost-6-10 — Tidy cmd/hooks.go: collapse the four nil-check builders and trim the redundant error wrap

ACCEPTANCE CRITERIA:
- One builder helper serves all four seams.
- The dead-pane message carries exactly one Portal-authored prefix and tmux's verbatim words.
- `hook rm`'s wording, which shares this call site, is unchanged.

STATUS: complete

SPEC CONTEXT:
The specification fixes the error contract this task tidies. §4.1 (spec lines 192–197) requires the `show-options` existence probe to name no option and states "Portal never parses tmux's message text: the exit status is the whole signal, and tmux's words are passed through to the user unaltered". §4.2 (spec line 215) fixes the removal wording: a live pane carrying no token exits with `no resume hook registered for this pane`, a key naming no entry with `no resume hook registered for <key>`, and "A gone pane exits with tmux's own words (§4.1)". Both `hook set` and `hook rm` resolve the key through the same `resolveCurrentPaneKey` call site, so the wrap trimmed here is on both verbs' gone-pane path.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/hooks.go:83-104 — `hookSeams()`, the single resolver: it copies whatever `hooksDeps` holds, then fills the production default for each unset field (`*tmux.Client` for KeyResolver/PaneLister/PaneStamper, `nanoid.NewPaneTokenGenerator()` for TokenMinter).
  - cmd/hooks.go:73, 109, 151 — the three call sites (`resolveCurrentPaneKey`, `stampPaneToken`, `hook list`) now read through it; the four `buildHookKeyResolver` / `buildPaneHookLister` / `buildPaneStamper` / `buildTokenMinter` builders are gone (repo-wide grep for those four names returns no match in any `.go` file).
  - cmd/hooks.go:73-76 — the `failed to resolve hook key for current pane:` wrap is dropped; the resolver's error is returned as phrased. The surviving Portal clause is `internal/tmux/tmux.go:237` (`no pane answers to %q: %w`), so the rendered message is `no pane answers to "%999": tmux show-options -p -t %999: exit 1: no such pane: %999` — one Portal clause plus `CommandError.Error()`'s verbatim tail (internal/tmux/command_error.go:41-54).
  - `hook rm`'s own wording is untouched: cmd/hooks.go:281 and cmd/hooks.go:297 still carry the two §4.2 strings verbatim, and the task's commit (c9345df3) touched only cmd/hooks.go, cmd/hooks_seams_test.go and cmd/hooks_test.go — cmd/hooks_rm_exit_test.go, which holds those exact-string assertions (lines 189, 218, 240), is untouched.
- Notes: The delivered shape is a merge-style resolver rather than the literal "generic helper taking default and override" the Do list sketched. That is the better fit and matches the house convention already in the package — `resolveDoctorDeps()` (cmd/doctor.go:79-104) is the same copy-then-fill shape, guarded by cmd/deps_merge_convention_test.go — and a Go generic `pick[T any](def, override T)` cannot nil-test an arbitrary `T` (TokenMinter is a func type), so the sketched form was not available. The acceptance criterion ("one builder helper serves all four seams") is met in substance: a fifth seam is one fill block in one function, not a fifth copy of the builder. `hookSeams()` constructs `tmux.DefaultClient()` unconditionally, but that is `NewClient(&RealCommander{})` (internal/tmux/tmux.go:75-77) — an allocation, no tmux contact — so an all-injected test path touches no server.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/hooks_seams_test.go:13-88 — `TestHookSeams`: all four seams resolve to the production default with nothing injected (and the default minter's output is asserted `nanoid.IsTokenShaped`), all four resolve to their injected fakes, and an unset field falls through to the default while an injected sibling is preserved. That is exactly the "each of the four seams resolves to its injected fake and to its production default" the task asked for.
  - cmd/hooks_seams_test.go:91-135 — `TestGonePaneErrorCarriesOnePortalClause`: exact-string assertion on the gone-pane message for both `hook set` and `hook rm`, driven through the real `tmux.Client` over a scripted commander whose only entry is the `show-options -p -t %999` probe. `commandertest.New`'s unmatched-argv policy is loud by default (internal/commandertest/scripted.go:34-43, 72), so the test also pins that the probe is the failing call rather than something else in the chain.
  - cmd/hooks_test.go:431-454 and :544-569 — the pass-through cases at the cmd layer: both verbs surface the resolver's error verbatim (`err.Error() != stderr` / `!= "tmux not responding"`), the `*tmux.CommandError` is still recoverable through `errors.As`, and no hooks file is created / no entry removed.
  - cmd/hooks_rm_exit_test.go:189, 218, 240 — the `hook rm` exact-string guard the task named, unchanged and still matching cmd/hooks.go:281/297.
- Notes: Not over-tested — the fake-resolver cases pin the cmd layer's pass-through, the scripted-commander case pins the whole rendered chain, and they assert different things. A regression that re-added a second Portal clause would fail `TestGonePaneErrorCarriesOnePortalClause` on an exact string. Everything added is unit-lane and touches no real tmux server, consistent with the lane rule.

CODE QUALITY:
- Project conventions: Followed. The seam is staged only through `withHooksDeps` / `withoutHooksDeps` (cmd/testhelpers_test.go:26-39), so `cmd/seam_guard_test.go`'s no-direct-assignment rule holds; the resolver mirrors `resolveDoctorDeps`'s established merge shape.
- SOLID principles: Good — one function owns seam resolution, the command bodies own behaviour.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The two comments the change added or reworded (cmd/hooks.go:61-66 on `resolveCurrentPaneKey`, cmd/hooks.go:81-82 on `hookSeams`) both hold against the code: the resolver's error is returned unaltered, and the one surviving Portal clause is the one at internal/tmux/tmux.go:237.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

TASK: resume-hooks-silently-lost-2-7 — One Real-Tmux Fixture Preamble And One Way To Stamp A Pane (tick-37fe62, severity: near-miss)

ACCEPTANCE CRITERIA:
- One server-bootstrap preamble serves every hook-key real-tmux fixture
- Every fixture calls `EnsureServer`; socket prefixes follow one convention
- `StampPaneToken` is the single way a test stamps a pane token, at all eight staging sites
- The exit-status assertion at `resolve_hookkey_realtmux_test.go:111` still uses raw tmux
- No test gains or loses a case, and no assertion changes
- Both lanes pass

STATUS: complete

SPEC CONTEXT:
The specification's §9 lane rule (line 479) places the fast real-tmux *client* tests under
`internal/tmux/*_realtmux_test.go` in the unit lane (per-test sockets, no daemons, no built
binaries), and its requirement table (lines 492, 494) names the two real-tmux subjects these
fixtures serve: "A pane that moves keeps its hook" (break-pane / kill-window under
renumber-windows / move-pane within and across sessions) and "The existence probe separates the
three cases" (gone pane / live-unstamped pane / stamped pane, plus the raw tmux facts the probe
rests on — `set-option -p -t %999 …` exits non-zero while `display-message -p` exits 0). The
Corrigenda section holds nothing bearing on this task. This task is a phase-2 consolidation of the
fixtures those subjects are staged through; it changes no subject, so its own body is the authority.

IMPLEMENTATION:
- Status: Implemented (and legitimately carried further by later phases)
- Location:
  - `internal/tmuxtest/stamp.go:11-16` — the promoted `(*Socket).StampPaneToken(t, target, token)`
    raw-tmux method (new file in this task's commit `70c7e50a`). It now takes `tmux.Target` rather
    than `string`, a later-phase refinement that also keeps it clean under
    `internal/tmux/target_composition_guard_test.go` (which scans non-test sources of every package
    importing `internal/tmux` — `internal/tmuxtest` is one — and accepts `string(target)` off a
    `Target`-declared parameter, `:105-111`).
  - `internal/tmux/hookkey_realtmux_shared_test.go:14-68` — the one preamble. The task shipped
    `seedHookKeyServer`; a later phase generalised it to `realTmuxFixture` + `seedRealTmuxServer`
    with `hookKeyFixture(...)` as the hook-key-shaped constructor. `seedRealTmuxServer:36-53` runs
    `SkipIfNoTmux` → `tmuxtest.New(prefix)` → `EnsureServer` → `NewSession` → `WaitForSession`,
    and `hookKeyFixtureSocketPrefix = "ptl-hookkey-"` (`:16`) is the single prefix.
  - Re-pointed hook-key fixtures: `resolve_hookkey_realtmux_test.go:17`,
    `hookkey_format_realtmux_test.go:19,32,41`, `list_all_pane_hookkeys_realtmux_test.go:13,36`,
    `hookkey_cross_site_realtmux_test.go:15`, `hookkey_moved_pane_realtmux_test.go:141-155`,
    `hookkey_realtmux_shared_test.go:75-83`. All six hook-key real-tmux files now enter through the
    one preamble; none composes its own `tmuxtest.New`/`EnsureServer`/`NewSession` block.
  - `newStampedPaneFixture` keeps its own `set-option -g renumber-windows on`
    (`hookkey_moved_pane_realtmux_test.go:145`) inside the topology callback, as the task required.
  - Stamp sites re-pointed: `hookkey_realtmux_shared_test.go:88`,
    `resolve_hookkey_realtmux_test.go:66`, `hookkey_format_realtmux_test.go:21`,
    `list_all_pane_hookkeys_realtmux_test.go:18`, `hookkey_cross_site_realtmux_test.go:16`,
    `hookkey_moved_pane_realtmux_test.go:158`, plus the three `cmd` sites the task named — now
    `cmd/state_daemon_hook_cleanup_integration_test.go:100`,
    `cmd/doctor_fix_transient_listpanes_integration_test.go:101` (file renamed by a later phase from
    `cleanstale_transient_listpanes_doctorfix_integration_test.go`) and
    `cmd/hook_prune_rename_survival_integration_test.go:40` (renamed from
    `rename_restore_cleanup_survival_integration_test.go`).
- Notes:
  - AC "single way to stamp" verified by enumeration, not assumption: a repo-wide grep for
    `"set-option", "-p"` returns exactly four sites — `internal/tmuxtest/stamp.go:15` (the helper),
    `internal/tmux/tmux.go:315` (production `SetPaneOption`),
    `internal/tmux/target_type_test.go:70` (an argv *expectation*, not a stamp), and
    `internal/tmux/resolve_hookkey_realtmux_test.go:97`. That last one is the carve-out the task
    named (formerly `:111`): it asserts tmux's own exit status against `%999` through `ts.TryRun`
    and is correctly left raw.
  - The `hookkey_moved_pane_realtmux_test.go` topology callback runs `renumber-windows on` *after*
    the session exists, where the pre-task code set it before. This is safe: tmux resolves session
    options at use time, and the case itself asserts the window index actually changed
    (`:43-45`), so a lost setting would fail loudly rather than pass vacuously.
  - `TestListAllPaneHookKeys_ListPanesFailurePropagates` (`:36`) now seeds a session it does not
    need before killing the server. Harmless — the read-failure path is unchanged.
  - The shared file is still named `hookkey_realtmux_shared_test.go` while `seedRealTmuxServer` now
    also serves `exact_session_target_realtmux_test.go:35`, `period_session_target_realtmux_test.go:34`
    and `saver_pane_pid_realtmux_test.go:21`. That widening was a later phase's doing, not this
    task's, and the naming is a preference — recorded here as context, not as a finding.

TESTS:
- Status: Adequate (pure movement — correct that no test was added)
- Coverage: The task's own criterion is that the existing real-tmux suites still pass unchanged.
  Read against the commit diff (`70c7e50a`), no `t.Run` case was added or removed and no assertion
  body changed in any of the six files: the edits are confined to preamble replacement and the
  stamp call. Two substitutions were checked for equivalence rather than assumed:
  `seedThreePaneStampedSession` stamps `paneIDs[0]` from `list-panes -s`, which is window 0 / pane 0
  — the `:0.0` the cross-site and format suites previously stamped by coordinate; and
  `StampPaneToken` routes through `Socket.Run`, which fatals on a non-zero exit exactly as the
  `TryRun`+`t.Fatalf` block it replaced in the doctor-fix suite did.
- Notes: Two fixtures whose subject *is* the stamp read it back rather than assuming it landed
  (`cmd/state_daemon_hook_cleanup_integration_test.go:101-107`), so the helper cannot silently
  no-op there.

CODE QUALITY:
- Project conventions: Followed. `internal/tmuxtest` remains test-only (its two non-test importers
  are `internal/restoretest/live_pane_coords.go` and `reboot.go`, themselves test-only helper
  packages). The `internal/state` import `stamp.go` adds is not a new transitive edge —
  `internal/tmuxtest` already imports `internal/tmux`, which imports `internal/state` — so the
  task's cycle reasoning holds. The new `ptl-hookkey-` prefix brings the previously non-conforming
  `hookkey-` socket back under CLAUDE.md's `ptl-*` sweep pattern, which is a real safety property
  and not just tidiness.
- SOLID principles: Good. The topology callback is the one axis the fixtures actually differ on,
  and each caller keeps its own token and assertions.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Every helper's doc comment holds against its code — checked individually:
  `seedThreePaneStampedSession`'s "three-pane session … first pane only" (split on `:0` plus one new
  window = three panes, `paneIDs[0]` stamped), `newStampedPaneFixture`'s "three-window session whose
  second window holds two panes, stamps the token on the second of those" (`NewWindow` ×2 plus the
  initial window; `SplitWindow(:1)`; stamp at `:1.1`), `hookKeyFixtureSocketPrefix`'s "names the temp
  dir" (`tmuxtest.New` passes it to `os.MkdirTemp`), and `StampPaneToken`'s "using raw tmux".
- Issues: None rising to a finding. `hookkey_moved_pane_realtmux_test.go:13,75,98` call
  `tmuxtest.SkipIfNoTmux(t)` that the seeder now also calls — redundant, idempotent, and a
  preference rather than a defect; and `StampPaneToken` takes `*testing.T` where the neighbouring
  `WaitForSession` takes `harnesstest.TestingT`, matching `Socket.Run`'s own signature.

BLOCKING ISSUES:
- None. Every acceptance criterion is met in substance.

FINDINGS:
- [out-of-scope] internal/spawn/ack_realtmux_test.go:31 — the socket temp-dir prefix is `spawnack-`,
  and `internal/tmux/portal_dir_roundtrip_realtmux_test.go:48,75,105` use `portaldir-`; rename both
  to `ptl-`-leading prefixes (e.g. `ptl-spawnack-`, `ptl-portaldir-`) as this task did for
  `hookkey-` → `ptl-hookkey-`. FAILS: CLAUDE.md:135 states that every disposable test server
  carries `-S <tmpdir>/ptl-*/s`, and the documented human sweep matches on exactly that pattern, so
  a server leaked by either suite is invisible to it and keeps running — the accumulation CLAUDE.md:130
  measured at 113 leaked servers, which saturates the machine and manufactures timing flakes in
  unrelated suites. Both files sit outside this feature's delivered change-set (neither appears in
  the plan's touched-file list; they were last altered by `a346ca55` / `d8ca237e`, other work
  units), so this is the user's to take or leave rather than a fix for this task.

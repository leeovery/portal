TASK: resume-hooks-silently-lost-3-5 — "A Reboot That Renumbers Windows Keeps Its Hooks" (integration-lane test pinning the reboot boundary under divergent window indices)

ACCEPTANCE CRITERIA:
- The fixture's saved window indices are non-contiguous and the test fails if they are not
- `renumber-windows off` is set explicitly on both server lifetimes rather than inherited
- The restored window indices are asserted to differ from the saved ones
- Each stamped pane's hook fires exactly once, into that pane's own marker file, with no cross-firing between panes
- The unstamped pane restores, hydrates and fires nothing
- `@portal-restoring` is confirmed unset before the sweep runs
- After the sweep both token-keyed entries survive and the seeded stale token-shaped key is gone
- Every restored pane's skeleton marker is keyed by its live structural key and all markers clear after hydration
- `//go:build integration`, binary via `restoretest`/`portalbintest`, `portaltest.IsolateStateForTest`, per-test socket, no default tmux server
- Hydration awaited by bounded polling, no fixed sleep anywhere in the test

STATUS: complete

SPEC CONTEXT:
§9.2's "Non-contiguous saved window indices" row: restore a session whose saved window indices are non-contiguous under `renumber-windows off` (tmux's default, not the reporting user's setting) and assert the hook fires *and* survives the post-restore sweep — the H6 case that fires correctly exactly once today and then dies. §9.1 places it in the integration lane because arming execs `portal state hydrate` through `respawn-pane -k`. §9.5 makes the same restore the second surface for the positional siblings (FIFO paths and `@portal-skeleton-*` markers), checked rather than assumed unaffected. §3.1 is why the boundary closes: restore re-establishes the token itself, so the key on disk and the key the live pane answers to are the same value however tmux renumbered.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/noncontiguous_window_reboot_integration_test.go:1-517` (whole file; added by 42e9d553, refactored onto the shared `restoretest`/`hookstest`/`hooksweep` surfaces by the later consolidation phases)
- Notes:
  - Fixture (`:276-290`): `new-session` (w0) → two `new-window` (w1, w2) → `split-window` on w2 → `kill-window` w1, with `renumber-windows off` set and read back (`:295-301`) before the kills. Saved indices come out `[0,2]`; `:325-328` fatals if they are contiguous. `tmuxtest`'s sockets run `-f /dev/null` (`internal/tmuxtest/socket.go:18`), so base-index is vanilla 0 and the hard-coded `savedWin` values hold.
  - Divergence (`:103-115`): restore's `createSkeleton` (`internal/restore/session.go:98-110`) passes no index to `NewWindow`, so the restored set is `[0,1]`; the guard subtest fails outright on equality and a second top-level `t.Fatalf` aborts the run before any assertion that presumes divergence.
  - Fire site (`:122-150`, `:494-504`): each hook is `echo <marker> $TMUX_PANE >> <file>`, and the file's whole content is compared exactly against `<marker> <live pane_id>\n` for the live pane at that structural position. This closes the cross-firing gap the two fix-tracking rounds identified — a bake that swapped the two panes' hook keys while stamping correctly now fails.
  - Live identity comes from ONE `list-panes -a -F` enumeration with no `-t` (`:415-433`, `internal/tmux/tmux.go:603-609`), which is the right instrument: `display-message -p -t <bad coord>` exits 0 with a neighbour's identity, so a per-pane read would answer with the wrong pane rather than failing.
  - Sweep: `sweepErr` → `hooksweep.Run` (`cmd/hookkey_vocabulary_test.go:225-228`). The plan's `runHookStaleCleanup(client, store, nil, nil, nil)` no longer exists — `cmd/run_hook_stale_cleanup.go` was deleted when the cycle moved to `internal/hooksweep` in a later phase — so this is legitimate post-plan drift with nothing lost. `hooksweep.Run` returns a nil error on a stand-down (`internal/hooksweep/sweep.go:199-202`), so an error check alone could not tell a stand-down from a real cycle; the stale-key reap subtest (`:211-216`) is what discriminates, exactly as the task's edge case requires.
  - Marker bracket: `restoretest.RestoreFromState` → `RestoreWithMarker` sets `@portal-restoring`, asserts it is set before restoring, and unsets it after (`internal/restoretest/restore_marker.go:20-50`), so the "restore under set-then-cleared marker" half of the Do list is real, and `:182-191`'s unset check is taken before the sweep call at `:193`.
  - Positional siblings: skeleton markers captured pre-hydration (`:380-384`) are compared set-for-set against the live pane keys (`:168-180`), and the FIFOs present at arm time are recorded before the helpers unlink them (`cmd/state_hydrate.go:107` unlinks only after the FIFO read returns, so the arm-time stat cannot race).
  - Isolation: `restoretest.BuildPortalBinaryDir` (integration-tagged `go build`), `IsolateStateForTest` then `RegisterStateDirTeardownGuard` then `tmuxtest.New` in the order CLAUDE.md prescribes (`:225-266`); every tmux call goes through the per-test socket or its client; no pgrep, no default socket, no `t.Parallel`.

TESTS:
- Status: Adequate (the task's deliverable IS the test)
- Coverage: All eight named subtests exist with the planned names (`:94`, `:103`, `:122`, `:152`, `:168`, `:182`, `:201`, `:211`). Each acceptance criterion has an assertion behind it that fails when the property fails: contiguity (`:325`), divergence (`:104`, `:113`), per-pane fire site (`:143`, `:500`), fire count (`restoretest.AssertMarkerCount(…, 1)` at `:404`, which polls and fails both short and over), the unstamped sibling carrying no token and producing no file (`:152-166`), marker unset before sweep (`:183-190`), survival + reap (`:201-216`), marker/live-key pairing (`:168-180`).
- Notes:
  - The fixture is honest about what it cannot prove: the "keeps both entries" subtest alone would pass on a sweep that stood down, and the seeded `hookstest.ReapableSeedA` reap subtest is what forecloses that.
  - Mild overlap between `:131-149` (per-position token) and `:153-162` (whole-server token set) — the second widens the claim to every pane on the server off the same enumeration, so it costs no extra read and is not redundant enough to flag.
  - No fixed sleep in the test; every wait is a bounded poll (`WaitForSession`, `AssertMarkerCount`, `WaitForSkeletonMarkersCleared`, `DriveSignalHydrate`'s FIFO retry).

CODE QUALITY:
- Project conventions: Followed. Integration build tag with the required blank line, `package cmd` (needed to reach `sweepErr` and `keysOf`), no `t.Parallel`, binary through `restoretest`/`portalbintest` so the daemon-pgrep sandbox tag is never dropped, seeds drawn from the single `hookstest` vocabulary rather than re-derived, targets composed through `tmux.PaneTargetExact`.
- SOLID principles: Good — the fixture struct owns setup/reboot/hydrate, the subtests only assert, and each helper has one job.
- Complexity: Low. The single enumeration shared by two subtests avoids per-pane reads and their fallback trap.
- Modern idioms: Yes (`slices.Equal`/`Contains`/`Clone`/`Sort`, `strings.SplitSeq`, `maps`-free minimal helpers).
- Readability: Good. The role-named `divergentPane` deliberately avoids identifying panes by the coordinates the test exists to distrust.
- Comment accuracy: Verified against the code they describe — the `renumber-windows` mechanism note (`:26-28`), the `display-message` fallback rationale (`:408-414`), the "helper unsets its marker before it execs the hook" note (`:400-402`, matches `cmd/state_hydrate.go:145,149`), the arm-time FIFO note (`:385-387`), and the structural-position pairing claim (`:126-130`, matches `internal/restore/session.go:119-152`) all hold. No process-artifact references.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

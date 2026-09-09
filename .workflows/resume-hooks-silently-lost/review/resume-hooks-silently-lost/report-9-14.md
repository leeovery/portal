TASK: resume-hooks-silently-lost-9-14 — `(*Client).SendKeys` Is Exported On The Production Client With No Production Caller (tick-43c4e5)

ACCEPTANCE CRITERIA:
- [x] `internal/tmux` exports no `SendKeys`, and `grep -rn 'SendKeys' internal/tmux` finds nothing.
- [x] `tmuxtest.Socket.SendKeys` sends against the fixture's own `-S` socket with `-f /dev/null`, never the ambient `TMUX`.
- [x] The three consuming integration tests pass unchanged in behaviour.
- [x] No production file references the driver.

STATUS: complete

SPEC CONTEXT: This is a phase-9 task — an implementation-analysis/quality cycle whose authority is its own body, not the
specification (per the shared verifier context, phases 6–9 are self-generated consolidation work). The specification for
`resume-hooks-silently-lost` names no `SendKeys` at all (`grep` over
`.workflows/resume-hooks-silently-lost/specification/` returns nothing), so there is no spec behaviour to align against.
The binding project context is CLAUDE.md's `tmux` package row (which enumerated `SendKeys` among the window/pane client
methods) and the test-isolation invariant that every test tmux command must run on a per-test `-S`/`-L` socket, never the
ambient server.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmuxtest/socket.go:73-79` — new `func (s *Socket) SendKeys(t *testing.T, target tmux.Target, keys string)`,
    routed through the existing `s.Run` → `s.cmd` → `socketArgs` path (`internal/tmuxtest/socket.go:17-19,53-55`), which
    prepends `-S <socketPath> -f /dev/null` to every invocation. The fixture-socket and vanilla-config claims in the doc
    comment are therefore structural, not incidental.
  - `internal/tmux/tmux.go` — `(*Client).SendKeys` and its doc comment are gone (commit `ac936233` removed lines 670-682
    of the old file; the current file goes straight from `parsePaneHookRows` to `RespawnPane`). `fmt` is still used by
    `parsePaneHookRows` (`internal/tmux/tmux.go:677`), so no import was orphaned.
  - Consumers re-pointed onto the harness method:
    `cmd/bootstrap/eager_signal_hydrate_integration_test.go:154` and `:235`;
    `internal/restore/exit_closes_pane_integration_test.go:35` and `:55`.
  - `internal/tmux/tmux_test.go` — the argv-shape case `TestSendKeys` was dropped with the method, as the Do list
    directed, rather than relocated. `errors`, `strings` and `commandertest` all remain used elsewhere in that file
    (49, 72 and 126 further references respectively), so no import was orphaned.
  - `internal/tmux/target_composition_guard_test.go:207,233,257,287,386,426,465` — the staged rule fixtures that used a
    `SendKeys` method name now use `SelectPane`/`select-pane`, keeping the guard's fixtures naming a method the client
    actually has. The `-t`-plus-concatenation shape the guard scans for is unchanged, so the rule's reach is unaffected.
  - `CLAUDE.md` — `SendKeys` removed from the `tmux` row's window/pane method list. The test-helper row describes
    `tmuxtest` generically ("real-tmux socket fixtures") and enumerates no methods, so no doc gap was created there.
- Notes:
  - Verified exhaustively rather than by claim: `grep -rn 'SendKeys' --include='*.go' .` returns 11 hits and **none** are
    in `internal/tmux` — one declaration (`internal/tmuxtest/socket.go:76`) plus its doc comment, two in-package tests
    (`internal/tmuxtest/socket_realtmux_test.go:90,107` under `:84`), and six consumer call sites
    (`cmd/bootstrap/eager_signal_hydrate_integration_test.go:154,235`,
    `internal/restore/exit_closes_pane_integration_test.go:35,55`,
    `internal/portaltest/isolated_env_realtmux_test.go:32,33`). The last pair is a later-arriving consumer of the new
    harness method, not a residue of the deleted client method.
  - The two non-`_test.go` files importing `internal/tmuxtest` are `internal/restoretest/live_pane_coords.go` and
    `internal/restoretest/reboot.go` — both inside the test-only `restoretest` package, and neither references
    `SendKeys`. No production file reaches the driver; the `*testing.T` first parameter makes that structural.
  - Behaviour preservation of the four re-pointed call sites is real, not assumed: in both suites the client that
    previously ran `send-keys` was `ts.Client()` over the same fixture socket
    (`cmd/bootstrap/eager_signal_hydrate_integration_test.go:55`; `internal/restore/exit_closes_pane_integration_test.go:31,49`),
    so the argv now runs on exactly the socket it ran on before.
  - The one substantive change beyond relocation is that the call sites moved from `tmux.PaneTarget` to
    `tmux.PaneTargetExact`, so the target is now `=name:0.0` rather than `name:0.0`. On a per-test socket holding a single
    uniquely-named session the two resolve identically, and the pinned form is the convention phase 9's earlier task
    (`a0cbed5d`, 9-2) established across the tree. It is a consistency gain with no behaviour lost — not a divergence
    worth reporting.

TESTS:
- Status: Adequate
- Coverage: `internal/tmuxtest/socket_realtmux_test.go:84-117` adds `TestSocket_SendKeys` with the two sub-tests the plan
  named verbatim:
  - `"it sends keys to a live pane on the fixture's own socket"` (`:85-99`) — creates a session on the fixture socket,
    sends, then polls `capture-pane` **on that same socket** for the marker. This fails if the driver were ever to reach
    the ambient server, because the marker would then land somewhere the assertion cannot see.
  - `"it appends Enter so the pane runs the command"` (`:101-116`) — sends `echo ran > <sentinel>` and polls for the
    sentinel file. Dropping the trailing `Enter` leaves the text at the prompt unexecuted and the file never appears, so
    the assertion is genuinely load-bearing rather than restating the argv.
  Both gate on `SkipIfNoTmux` (`internal/tmuxtest/skip.go:8`) and get server teardown from `New`'s registered cleanup
  (`internal/tmuxtest/socket.go:41-44`), so they leak no `ptl-*` server.
  The plan's third listed test — `"it drives the eager-signal-hydrate and exit-closes-pane fixtures through the harness
  method"` — is not a new named test and does not need to be: it describes the re-pointed suites themselves, which now
  drive through the method and remain the coverage that would catch a broken relocation.
- Notes: Not over-tested. There is deliberately no assertion on the composed argv or on `-f /dev/null` — those are
  properties of the shared `socketArgs` path every `Socket` command already takes, and asserting them here would pin an
  implementation detail rather than behaviour. Dropping the old `TestSendKeys` argv-shape case was correct: its subject
  was the client's argv composition, and that composition no longer exists.

CODE QUALITY:
- Project conventions: Followed. Test-only helper lives in a test-only package that production cannot reach (the
  `*testing.T` first parameter enforces it); the fixture socket rule ("only a per-test socket, never the default") is
  satisfied structurally by routing through `s.Run`; the method mirrors `Run`'s `*testing.T` + `t.Helper()` shape rather
  than inventing a second reporting convention alongside `WaitForSession`'s `harnesstest.TestingT` (which takes the
  stand-in only because its own failure path is under test).
- SOLID principles: Good — the driver now sits on the type that owns the socket it runs against, so the production client
  no longer carries a method for a consumer it does not have.
- Complexity: Low (a two-line delegation).
- Modern idioms: Yes — takes the typed `tmux.Target` and spends it as a string at the single argv boundary, which is the
  target-type contract the package works to.
- Readability: Good.
- Issues: None. The new doc comment (`internal/tmuxtest/socket.go:73-75`) states only what the code does and holds true
  against it; the comment that had to explain an export with no production caller is gone with the export.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

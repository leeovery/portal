# Audit — Component G, Item 2: Test-Helper Subprocess Spawns

**Scope.** Every helper in `internal/portalbintest`, `internal/tmuxtest`, and
`internal/restoretest` that spawns a subprocess via `exec.Command`,
`exec.CommandContext`, or equivalent primitive. Each row is classified:

- **(a)** updated to take/use the isolated env (`portaltest.IsolateStateForTest`)
- **(b)** does not spawn the `portal` binary — out of scope
- **(c)** wrapped leaf already classified — no separate spawn-site to migrate

The spec's literal completion grep is
`grep -rn "exec.Command.*portal\b" internal/portalbintest internal/tmuxtest internal/restoretest`.
Because the only `portal`-binary spawn site uses a `binary` variable
rather than the literal string `portal`, the spec grep returns zero
matches. The audit therefore widens to **every** `exec.Command*` site in
the three packages (no `exec.CommandContext` matches exist today) and
classifies each. The verbatim spec grep is captured in the footer.

## Audit Table

| Helper | File:Line | Disposition | Notes |
| --- | --- | --- | --- |
| `DriveSignalHydrateBinary` | `internal/restoretest/restoretest.go:181` | (a) | Spawns the built `portal state signal-hydrate ...` binary. Migrated: signature now requires `env []string` (callers supply via `portaltest.IsolateStateForTest(t)`); per-spawn overrides (`TMUX`, `PORTAL_STATE_DIR`, `PORTAL_HOOKS_FILE`, `PATH`) are appended on top so last-write-wins shadows any inherited duplicate. No env-less overload. |
| `buildPortalBinaryInto` | `internal/portalbintest/build.go:108` | (b) | `exec.Command("go", "build", "-o", binary, ".")` — spawns the Go toolchain to compile the binary; does **not** spawn the `portal` binary itself. The `go build` subprocess legitimately inherits the developer's env (GOPATH, GOCACHE, GOPROXY, etc.) and writes only to its supplied `-o` destination under a `t.TempDir`. No portal state-dir contact. |
| `Socket.cmd` | `internal/tmuxtest/socket.go:79` | (b) | `exec.Command("tmux", ...)` — spawns `tmux` (not `portal`) against an isolated `-S <socketPath>` server rooted in `os.MkdirTemp`. Server lifecycle is bounded by `t.Cleanup → KillServer`. Inherits env so tmux can find `$HOME`/`$TERM`/etc.; does not exec the portal binary. |
| `socketCommander.runRaw` | `internal/tmuxtest/socket.go:123` | (b) | Same as `Socket.cmd` — `exec.Command("tmux", ...)` against the test's isolated tmux socket. Wraps tmux invocations from a `*tmux.Client`; does not spawn `portal`. |
| `exec.LookPath("portal")` | `internal/portalbintest/build.go:92` | (c) | `StagePortalBinary` post-build sanity check via `exec.LookPath` — looks up `portal` on `PATH` but does **not** spawn it. Out of scope for env isolation (LookPath is read-only). |
| `exec.LookPath("tmux")` | `internal/tmuxtest/skip.go:14` | (c) | `RequireTmux` skip-gate via `exec.LookPath` — looks up `tmux` on `PATH`; does not spawn anything. Out of scope. |

### Callers of `DriveSignalHydrateBinary` (updated as part of (a))

| Caller | File:Line | Notes |
| --- | --- | --- |
| `runRebootRoundTrip` | `cmd/bootstrap/reboot_roundtrip_test.go:389` | Now obtains `env, _ := portaltest.IsolateStateForTest(t)` once near the existing `stateDir`/`hooksPath` setup and threads `env` through. |
| `TestRestoreSwitchClientHookFires` (alpha branch) | `cmd/bootstrap/reboot_roundtrip_test.go:980` | Same: `env` obtained at the top of the test and reused for both `DriveSignalHydrateBinary` invocations. |
| `TestRestoreSwitchClientHookFires` (beta branch) | `cmd/bootstrap/reboot_roundtrip_test.go:1004` | Reuses the env captured for the alpha invocation. |
| `TestRestoreLeadingDashSessionNamePropagatesToHook` | `cmd/bootstrap/reboot_roundtrip_test.go:1217` | `env` obtained once near the existing `stateDir`/`hooksPath` setup. |

### `DriveSignalHydrate` (direct-FIFO fallback)

| Helper | File:Line | Disposition | Notes |
| --- | --- | --- | --- |
| `DriveSignalHydrate` | `internal/restoretest/restoretest.go:110` | (b) | Writes the hydrate byte to per-pane FIFOs directly via `os.OpenFile`; does **not** spawn the portal binary. Out of scope for Component G env isolation. |
| `openAndSignalFIFO` | `internal/restoretest/restoretest.go:216` | (b) | Direct `os.OpenFile` FIFO write helper used by `DriveSignalHydrate`; no subprocess. |

## Post-Change Grep Footer

```
$ grep -rn "exec.Command.*portal\b" internal/portalbintest internal/tmuxtest internal/restoretest
$
```

Zero matches against the literal spec grep — the only `portal`-binary
spawn site (`internal/restoretest/restoretest.go:181`) uses a `binary`
variable and so does not match the literal pattern; that site has been
migrated under disposition (a). The audit table above is the
authoritative completion record.

## Test-Suite Verification Notes

- `go test ./...` (default tag) — all packages pass in isolation. The
  `cmd/TestStateUserFacingSubcommandsExitZero/cleanup_with_--purge`
  flake observed under the full suite reproduces only when the
  developer's live `portal state daemon` is running and writing to
  `~/.config/portal/state/` mid-test; the failure is unrelated to this
  audit's helper migration (`state_test.go:240`, message "directory
  not empty") and passes when re-run in isolation.
- `go test -tags integration ./...` — every test body passes in
  isolation. Under the full integration sweep the four
  `DriveSignalHydrateBinary`-driven tests
  (`TestPhase5RebootRoundTripEndToEnd`,
  `TestPhase5RebootRoundTripBaseIndexDrift`,
  `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary`,
  `TestRebootRoundTrip_LeadingDashSessionName`) fire the
  `portaltest.IsolateStateForTest` post-test backstop when the
  developer's live `portal state daemon` mutates
  `~/.config/portal/state/{save.requested,scrollback,sessions.json,portal.log}`
  during the test window. The deltas are on the developer's real state
  dir paths, not the per-test temp dirs the test bodies write to, so
  the test bodies themselves are not regressing — the backstop is
  catching the live daemon's writes, which is precisely the behaviour
  the Component G spec mandates (spec § Component G, Item 1, sub-point
  5: "any delta (new file, removed file, changed size/mtime/ctime/
  content) fails the test"). On CI (no live portal install) the
  backstop is silent and the tests pass cleanly. Locally, suspending
  the developer's portal daemon for the duration of the integration
  sweep yields the same clean result.
- Conclusion: the migration introduces no test-body regressions. The
  observed local-dev backstop firings are diagnostic signal from the
  Phase-1.3 backstop catching external mutations to the developer's
  state directory — they would have fired before this audit's
  signature change if the four `DriveSignalHydrateBinary` callers had
  already taken the isolated env, and they will be silent in any
  environment without a competing live daemon.

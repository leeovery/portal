# Phase 1: Bootstrap Core — Server Detection and Start

---
phase: 1
phase_name: Bootstrap Core — Server Detection and Start
total: 4
---

## auto-start-tmux-server-1-1 | approved

### Task 1: ServerRunning method

**Problem**: Portal has no way to detect whether a tmux server is currently running. The bootstrap flow needs this check to decide whether to call `start-server` or skip straight to normal operation (fast path).

**Solution**: Add a `ServerRunning() bool` method to `*Client` in the `tmux` package. It runs `tmux info` via the `Commander` interface. If the command succeeds (no error), the server is running. If it fails, the server is not running.

**Outcome**: `client.ServerRunning()` returns `true` when a tmux server is running and `false` when no server is running, with full test coverage using `MockCommander`.

**Do**:
- Add `ServerRunning() bool` method to `*Client` in `/Users/leeovery/Code/portal/internal/tmux/tmux.go`
- The method calls `c.cmd.Run("info")` and returns `err == nil`
- Using `tmux info` is preferred over `tmux list-sessions` for server detection because `info` succeeds even when the server is running with zero sessions, whereas `list-sessions` fails with no sessions (spec says detect server, not sessions)
- Add tests in `/Users/leeovery/Code/portal/internal/tmux/tmux_test.go` following the existing table-driven and subtest patterns (see `TestHasSession` for the closest analog — a method that returns bool based on Commander error/success)

**Acceptance Criteria**:
- [ ] `ServerRunning()` returns `true` when `Commander.Run("info")` succeeds
- [ ] `ServerRunning()` returns `false` when `Commander.Run("info")` returns an error
- [ ] The method passes exactly `"info"` as the sole argument to `Commander.Run`
- [ ] Tests use `MockCommander` — no real tmux process in unit tests

**Tests**:
- `"returns true when tmux server is running"` — MockCommander returns no error; assert `ServerRunning() == true`
- `"returns false when no tmux server is running"` — MockCommander returns error (e.g., `fmt.Errorf("no server running on /tmp/tmux-501/default")`); assert `ServerRunning() == false`
- `"calls tmux info to check server status"` — verify `mock.Calls[0]` equals `[]string{"info"}`

**Edge Cases**:
- No server running: `tmux info` returns a non-zero exit code and error text. The method must return `false`, not panic or propagate the error.
- Server running with zero sessions: `tmux info` still succeeds, so `ServerRunning()` correctly returns `true`. This matters because after `start-server`, the server exists but has no sessions yet.

**Context**:
> The spec says: "Check if the tmux server is running (e.g., tmux list-sessions or tmux info succeeding)." We choose `tmux info` because it succeeds even with zero sessions, making it a pure server-presence check. `list-sessions` would fail with no sessions, conflating "no server" with "server but no sessions."

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Detection" paragraph under "Bootstrap Mechanism"

## auto-start-tmux-server-1-2 | approved

### Task 2: StartServer method

**Problem**: Portal has no way to start the tmux server. When bootstrap detects no running server, it needs to execute `tmux start-server` as a one-shot attempt with no retry logic.

**Solution**: Add a `StartServer() error` method to `*Client` in the `tmux` package. It calls `tmux start-server` via the `Commander` interface and returns any error directly to the caller.

**Outcome**: `client.StartServer()` runs `tmux start-server` and returns `nil` on success or a wrapped error on failure. No retry, no wrapping beyond the standard pattern — just propagation.

**Do**:
- Add `StartServer() error` method to `*Client` in `/Users/leeovery/Code/portal/internal/tmux/tmux.go`
- The method calls `c.cmd.Run("start-server")`. If `err != nil`, return `fmt.Errorf("failed to start tmux server: %w", err)`. Otherwise return `nil`.
- Follow the error wrapping pattern used by other methods in the file (e.g., `NewSession`, `KillSession` which wrap with `fmt.Errorf("failed to ...: %w", err)`)
- Add tests in `/Users/leeovery/Code/portal/internal/tmux/tmux_test.go` following the existing subtest patterns (see `TestKillSession` for the closest analog — a void-like method that either succeeds or returns a wrapped error)
- No retry logic — this is explicitly one-shot per spec

**Acceptance Criteria**:
- [ ] `StartServer()` calls `Commander.Run("start-server")`
- [ ] Returns `nil` when `Commander.Run` succeeds
- [ ] Returns a wrapped error when `Commander.Run` fails
- [ ] No retry logic — single call to `Commander.Run`, result returned immediately
- [ ] Tests use `MockCommander`

**Tests**:
- `"starts tmux server successfully"` — MockCommander returns no error; assert `StartServer()` returns `nil`; verify `mock.Calls[0]` equals `[]string{"start-server"}`
- `"returns error when start-server fails"` — MockCommander returns `fmt.Errorf("server start failed")`; assert `StartServer()` returns a non-nil error
- `"does not retry on failure"` — MockCommander returns error; call `StartServer()` once; assert `len(mock.Calls) == 1`

**Edge Cases**:
- `start-server` fails (e.g., tmux binary found but some system-level issue prevents server creation): the error is propagated up, no retry. The caller decides what to do.

**Context**:
> The spec says: "Server start is a single attempt -- no retry if tmux start-server fails." and "Command: tmux start-server -- starts the tmux server without creating any sessions. No throwaway _boot session needed." The method is intentionally simple -- it does exactly one thing with no cleverness.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Command" and "One-shot" paragraphs under "Bootstrap Mechanism"

## auto-start-tmux-server-1-3 | approved

### Task 3: EnsureServer bootstrap function

**Problem**: Tasks 1 and 2 provide the primitives (`ServerRunning` and `StartServer`), but there is no orchestration function that combines them into the bootstrap flow: check if server is running, start it if not, and report whether a start was attempted. This orchestration is needed by `PersistentPreRunE` and later by the session wait logic (Phase 2) to know whether to enter the waiting/interstitial flow.

**Solution**: Add an `EnsureServer() (serverStarted bool, err error)` method to `*Client` in the `tmux` package. It calls `ServerRunning()` first. If the server is already running, return `(false, nil)` immediately (fast path). If not, call `StartServer()`. If `StartServer` succeeds, return `(true, nil)`. If `StartServer` fails, return `(true, err)` — `serverStarted` is `true` because a start was attempted (downstream code uses this to decide whether to show the loading interstitial, even if the start failed).

**Outcome**: `client.EnsureServer()` is a single entry point for the bootstrap flow. It returns a bool indicating whether a server start was attempted and an error if the start failed. Commands use the bool to decide whether to enter the session-wait flow (Phase 2).

**Do**:
- Add `EnsureServer() (bool, error)` method to `*Client` in `/Users/leeovery/Code/portal/internal/tmux/tmux.go`
- Implementation:
  ```
  func (c *Client) EnsureServer() (bool, error) {
      if c.ServerRunning() {
          return false, nil
      }
      if err := c.StartServer(); err != nil {
          return true, err
      }
      return true, nil
  }
  ```
- Add tests in `/Users/leeovery/Code/portal/internal/tmux/tmux_test.go` using `MockCommander` with `RunFunc` to vary behavior per tmux subcommand (the mock needs to return success for `info` to simulate "server running" or error for `info` to simulate "no server", and success/error for `start-server`)
- The `RunFunc` approach is already supported by `MockCommander` — see `tmux_test.go` line 18: `RunFunc func(args ...string) (string, error)`

**Acceptance Criteria**:
- [ ] When server is already running: returns `(false, nil)` without calling `start-server`
- [ ] When server is not running and `start-server` succeeds: returns `(true, nil)`
- [ ] When server is not running and `start-server` fails: returns `(true, err)` where `err` is the `StartServer` error
- [ ] `ServerRunning()` is always called first
- [ ] `StartServer()` is only called when `ServerRunning()` returns `false`
- [ ] Tests use `MockCommander` with `RunFunc` to differentiate behavior by subcommand

**Tests**:
- `"returns false when server is already running"` — RunFunc returns success for `info`; assert returns `(false, nil)`; assert only `info` was called (no `start-server` call)
- `"starts server and returns true when server is not running"` — RunFunc returns error for `info`, success for `start-server`; assert returns `(true, nil)`; assert both `info` and `start-server` were called
- `"returns true and error when start-server fails"` — RunFunc returns error for `info`, error for `start-server`; assert returns `(true, err)` where `err != nil`
- `"does not call start-server when server is running"` — RunFunc returns success for `info`; assert `mock.Calls` contains only `["info"]`, no `start-server`

**Edge Cases**:
- Server already running: `EnsureServer` skips `StartServer` entirely — zero side effects, fast path. Verify via `mock.Calls` that `start-server` was never invoked.
- `StartServer` fails: `serverStarted` is still `true` because the attempt was made. This is deliberate — downstream code (Phase 2/3) uses this bool to decide whether to show "Starting tmux server..." messaging, and showing it even on failure is correct UX (the user should know something was tried).

**Context**:
> The spec's "Two-phase ownership" section describes: "Server start -- runs in PersistentPreRunE (shared by all commands). Calls tmux start-server. Returns immediately." The `EnsureServer` function is this shared server-start phase. The return value `serverStarted` is used by Phase 2 (CLI session wait) and Phase 3 (TUI interstitial) to decide whether to enter their respective wait flows. The naming `serverStarted` means "a start was attempted" not "the start succeeded."

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Two-phase ownership" and "One-shot" under "Bootstrap Mechanism"

## auto-start-tmux-server-1-4 | approved

### Task 4: PersistentPreRunE integration

**Problem**: The bootstrap function (`EnsureServer`) exists but is not wired into Portal's command lifecycle. Currently, `PersistentPreRunE` in `/Users/leeovery/Code/portal/cmd/root.go` only checks if tmux is installed (`CheckTmuxAvailable`). It needs to also bootstrap the server for commands that require tmux, while continuing to skip both the tmux check and bootstrap for commands in `skipTmuxCheck`.

**Solution**: Modify `PersistentPreRunE` to create a `tmux.Client` with a `RealCommander` and call `EnsureServer()` after `CheckTmuxAvailable()` succeeds. The `serverStarted` return value is not used in Phase 1 — it will be consumed by Phase 2 (CLI wait) and Phase 3 (TUI interstitial). For now, if `EnsureServer` returns an error, it is returned from `PersistentPreRunE` to abort the command.

**Outcome**: Every tmux-requiring command automatically bootstraps the server before execution. Commands in `skipTmuxCheck` skip both the availability check and bootstrap. The existing test suite continues to pass, and new tests verify the bootstrap integration.

**Do**:
- Modify `PersistentPreRunE` in `/Users/leeovery/Code/portal/cmd/root.go`:
  1. Keep the existing `skipTmuxCheck` loop — if matched, return `nil` early (unchanged)
  2. Keep the `tmux.CheckTmuxAvailable()` call — if it fails, return the error (unchanged)
  3. After `CheckTmuxAvailable` succeeds, create a client: `client := tmux.NewClient(&tmux.RealCommander{})`
  4. Call `_, err := client.EnsureServer()` — discard the `serverStarted` bool for now (Phase 2 will use it)
  5. If `err != nil`, return `err`
  6. Return `nil`
- The resulting `PersistentPreRunE` should look like:
  ```go
  PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
      for c := cmd; c != nil; c = c.Parent() {
          if skipTmuxCheck[c.Name()] {
              return nil
          }
      }
      if err := tmux.CheckTmuxAvailable(); err != nil {
          return err
      }
      client := tmux.NewClient(&tmux.RealCommander{})
      _, err := client.EnsureServer()
      return err
  },
  ```
- Update tests in `/Users/leeovery/Code/portal/cmd/root_test.go`:
  - Existing `TestTmuxDependentCommandsFailWithoutTmux` tests should still pass (PATH set to nonexistent, `CheckTmuxAvailable` fails before `EnsureServer` is reached)
  - Existing `TestNonTmuxCommandsWorkWithoutTmux` tests should still pass (commands in `skipTmuxCheck` return before any tmux interaction)
  - No new unit tests needed for `PersistentPreRunE` itself in Phase 1 — the function is thin glue code, and the existing tests already cover the skip/fail paths. The `EnsureServer` logic is fully tested in the tmux package.
- Verify all existing tests pass: `go test ./cmd/... ./internal/tmux/...`

**Acceptance Criteria**:
- [ ] Commands in `skipTmuxCheck` (`version`, `init`, `help`, `alias`, `clean`) skip both `CheckTmuxAvailable` and `EnsureServer` — they never touch tmux
- [ ] When tmux is not installed (PATH broken), tmux-requiring commands fail with the existing "Portal requires tmux" error before `EnsureServer` is called
- [ ] When tmux is installed and the server is already running, `EnsureServer` returns `(false, nil)` and the command proceeds normally
- [ ] When tmux is installed and the server is not running, `EnsureServer` calls `start-server` and the command proceeds (or fails if `start-server` fails)
- [ ] All existing tests in `cmd/root_test.go` and `cmd/root_integration_test.go` continue to pass
- [ ] The `serverStarted` bool is discarded with `_` (Phase 2 will capture it)

**Tests**:
- `"skipTmuxCheck commands bypass bootstrap entirely"` — existing `TestNonTmuxCommandsWorkWithoutTmux` covers this; verify it still passes with PATH set to `/nonexistent/path`
- `"tmux-dependent commands fail when tmux not installed"` — existing `TestTmuxDependentCommandsFailWithoutTmux` covers this; verify `CheckTmuxAvailable` error is returned before `EnsureServer`
- `"tmux-dependent commands succeed when tmux is available"` — existing `TestTmuxDependentCommandsSucceedWithTmux` covers this; verify `list` command succeeds (tmux is available on the dev machine, server running)
- `"integration: binary exits 1 when tmux missing"` — existing `TestPortalBinaryTmuxMissing` covers this

**Edge Cases**:
- `skipTmuxCheck` bypasses bootstrap: Commands like `version` and `init` must never trigger `EnsureServer`. The existing skip loop runs before both `CheckTmuxAvailable` and `EnsureServer`, so any command in the map returns `nil` immediately.
- `CheckTmuxAvailable` failure prevents bootstrap: If tmux is not on PATH, the error is returned before `EnsureServer` is reached. This is verified by the existing tests that set PATH to `/nonexistent/path`.

**Context**:
> The spec says: "A shared bootstrap function called early by every Portal command that requires tmux. Commands that skip the existing tmux availability check (version, init, help, alias, clean) also skip bootstrap." The `skipTmuxCheck` map at `/Users/leeovery/Code/portal/cmd/root.go` line 11 already defines which commands skip. The bootstrap call is added after `CheckTmuxAvailable` in the same guard block, so the skip logic naturally applies to both.
>
> Phase 1 discards `serverStarted` because Phase 2 introduces the session wait that needs it. The `PersistentPreRunE` signature will need to communicate `serverStarted` to individual command `RunE` functions — but that wiring is Phase 2's concern. For Phase 1, the bootstrap runs silently and the bool is unused.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Trigger" paragraph under "Bootstrap Mechanism", and "Two-phase ownership"

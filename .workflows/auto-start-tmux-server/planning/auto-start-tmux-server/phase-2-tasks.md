# Phase 2: Session Wait with Timing Bounds

---
phase: 2
phase_name: Session Wait with Timing Bounds
total: 3
---

## auto-start-tmux-server-2-1 | approved

### Task 1: WaitForSessions polling function

**Problem**: After `EnsureServer` starts the tmux server (Phase 1), plugins like tmux-continuum need time to restore sessions. There is no mechanism to poll for session appearance with timing bounds. CLI commands need to block briefly with a min/max wait before proceeding, and the TUI will use the same constants (Phase 3). Without this, commands would execute immediately after server start and see zero sessions.

**Solution**: Add a `WaitForSessions` function in the `tmux` package that polls `ListSessions()` at a configurable interval, enforcing a minimum wait (1s) and maximum wait (6s). The function accepts a `WaitConfig` struct with injectable timing values and a session-check function, making it fully testable without real sleeps. Define named constants for the production timing values.

**Outcome**: `tmux.WaitForSessions(cfg)` polls for sessions, always waits at least `MinWait`, exits early when sessions are detected after `MinWait`, and always returns by `MaxWait` regardless. Tests run in milliseconds using injected timing.

**Do**:
- Create a new file `/Users/leeovery/Code/portal/internal/tmux/wait.go`
- Define named constants:
  ```go
  const (
      DefaultMinWait      = 1 * time.Second
      DefaultMaxWait      = 6 * time.Second
      DefaultPollInterval = 500 * time.Millisecond
  )
  ```
- Define a `WaitConfig` struct:
  ```go
  type WaitConfig struct {
      MinWait      time.Duration
      MaxWait      time.Duration
      PollInterval time.Duration
      HasSessions  func() bool  // injectable session check
  }
  ```
- Implement `WaitForSessions(cfg WaitConfig)`:
  - Start a timer/track elapsed time from function entry
  - Poll `cfg.HasSessions()` at `cfg.PollInterval` intervals
  - If `HasSessions()` returns true AND elapsed >= `cfg.MinWait`, return immediately
  - If `HasSessions()` returns true but elapsed < `cfg.MinWait`, continue waiting until `MinWait` is reached, then return
  - If elapsed >= `cfg.MaxWait`, return regardless of session state
  - Use `time.NewTicker` for polling and `time.After` for the max deadline
- Add a convenience constructor `DefaultWaitConfig(client *Client) WaitConfig` that builds a config using the constants and `client.ListSessions` for the `HasSessions` check:
  ```go
  func DefaultWaitConfig(client *Client) WaitConfig {
      return WaitConfig{
          MinWait:      DefaultMinWait,
          MaxWait:      DefaultMaxWait,
          PollInterval: DefaultPollInterval,
          HasSessions: func() bool {
              sessions, err := client.ListSessions()
              return err == nil && len(sessions) > 0
          },
      }
  }
  ```
- Create tests in `/Users/leeovery/Code/portal/internal/tmux/wait_test.go`
- Tests should use small timing values (e.g., MinWait=50ms, MaxWait=200ms, PollInterval=10ms) and a controllable `HasSessions` func to keep test execution fast

**Acceptance Criteria**:
- [ ] Named constants `DefaultMinWait` (1s), `DefaultMaxWait` (6s), `DefaultPollInterval` (500ms) are exported from the `tmux` package
- [ ] `WaitForSessions` always waits at least `MinWait` even if sessions appear immediately
- [ ] `WaitForSessions` exits as soon as sessions are detected after `MinWait` has elapsed
- [ ] `WaitForSessions` always returns by `MaxWait`, even if no sessions ever appear
- [ ] `WaitConfig.HasSessions` is injectable for testability — no real tmux calls in tests
- [ ] `DefaultWaitConfig` builds a production config using `ListSessions` under the hood
- [ ] All tests run in under 1 second (no real 6-second waits)

**Tests**:
- `"returns after min wait when sessions appear before min wait"` — HasSessions returns true immediately; assert function takes >= MinWait and < MaxWait
- `"returns immediately after min wait when sessions appear exactly at min wait"` — HasSessions starts returning true at ~MinWait; assert function returns at ~MinWait
- `"returns at max wait when no sessions ever appear"` — HasSessions always returns false; assert function takes >= MaxWait (within tolerance)
- `"exits early when sessions appear between min and max"` — HasSessions returns true at ~halfway between min and max; assert function returns after that point but before MaxWait
- `"polls at the configured interval"` — count how many times HasSessions is called; verify it's approximately MaxWait/PollInterval (within tolerance)
- `"sessions detected on first poll still waits for min wait"` — HasSessions returns true on first call; measure elapsed time; assert >= MinWait

**Edge Cases**:
- Sessions appear before MinWait: The function must still wait until MinWait elapses. This prevents a jarring flash in the UI. Verify by asserting elapsed >= MinWait when HasSessions returns true on the first poll.
- No sessions by MaxWait: The function returns anyway. The caller proceeds to normal operation (which will show an empty session list). Verify by asserting the function returns at ~MaxWait with HasSessions always returning false.
- Sessions appear between MinWait and MaxWait: The function should exit on the next poll after sessions are detected (since MinWait has already passed). Verify elapsed is between MinWait and MaxWait.

**Context**:
> The spec says: "Session-detection with min/max bounds. Transition out of the loading state as soon as sessions are detected, but enforce: Minimum 1 second -- prevents a jarring flash if sessions appear very quickly. Maximum 6 seconds -- proceed regardless after this, even if no sessions have appeared." And: "Not user-configurable. Both values should be defined as named constants in the code for easy adjustment." And: "Poll interval: 500ms. This applies to the CLI path directly; the TUI path uses its existing refresh cycle."
>
> The `HasSessions` callback in `WaitConfig` injects the session-check dependency. For production, `DefaultWaitConfig` wires it to `client.ListSessions()`. For tests, a simple closure controls when "sessions appear." This keeps the polling logic pure and testable without `MockCommander`.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Timing" section, "Detection method" paragraph

## auto-start-tmux-server-2-2 | approved

### Task 2: Propagate serverStarted via command context

**Problem**: Phase 1's `PersistentPreRunE` calls `EnsureServer()` and discards the `serverStarted` bool with `_`. Individual command `RunE` functions need this value to decide whether to print "Starting tmux server..." to stderr and run the session wait (CLI path) or show the loading interstitial (TUI path, Phase 3). Cobra has no built-in mechanism to pass data from `PersistentPreRunE` to `RunE`, but every `*cobra.Command` carries a `context.Context` that can be used for this.

**Solution**: Define a context key type and helper functions in the `cmd` package. In `PersistentPreRunE`, after `EnsureServer()` returns, store `serverStarted` in the command's context via `cmd.SetContext(context.WithValue(...))`. In command `RunE` functions, retrieve the value via `cmd.Context()`. Commands that skip tmux (skipTmuxCheck) never reach the `EnsureServer` call, so their context is never set — the retrieval helper returns `false` as the default.

**Outcome**: Any command's `RunE` can call `serverWasStarted(cmd)` to check whether bootstrap started the server. The value is `true` only when `EnsureServer` actually attempted a server start. Commands in `skipTmuxCheck` and commands where `CheckTmuxAvailable` failed get `false` (the zero value default).

**Do**:
- Create a new file `/Users/leeovery/Code/portal/cmd/bootstrap_context.go` with:
  ```go
  package cmd

  import (
      "context"

      "github.com/spf13/cobra"
  )

  // contextKey is an unexported type for context keys in this package.
  type contextKey string

  // serverStartedKey is the context key for the serverStarted boolean.
  const serverStartedKey contextKey = "serverStarted"

  // serverWasStarted retrieves the serverStarted flag from the command's context.
  // Returns false if the value was never set (e.g., skipTmuxCheck commands).
  func serverWasStarted(cmd *cobra.Command) bool {
      val, ok := cmd.Context().Value(serverStartedKey).(bool)
      if !ok {
          return false
      }
      return val
  }
  ```
- Modify `PersistentPreRunE` in `/Users/leeovery/Code/portal/cmd/root.go`:
  1. Change `_, err := client.EnsureServer()` to `serverStarted, err := client.EnsureServer()`
  2. After the error check, store the value in context:
     ```go
     ctx := context.WithValue(cmd.Context(), serverStartedKey, serverStarted)
     cmd.SetContext(ctx)
     ```
  3. Add `"context"` to the import block
  4. The resulting `PersistentPreRunE` should look like:
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
         serverStarted, err := client.EnsureServer()
         if err != nil {
             return err
         }
         ctx := context.WithValue(cmd.Context(), serverStartedKey, serverStarted)
         cmd.SetContext(ctx)
         return nil
     },
     ```
- Create tests in `/Users/leeovery/Code/portal/cmd/bootstrap_context_test.go`:
  - Test `serverWasStarted` with a cobra command that has the context value set to `true`
  - Test `serverWasStarted` with a cobra command that has the context value set to `false`
  - Test `serverWasStarted` with a cobra command that has no context value set (default `context.Background()`) — should return `false`

**Acceptance Criteria**:
- [ ] `serverWasStarted(cmd)` returns `true` when `EnsureServer` returned `serverStarted=true`
- [ ] `serverWasStarted(cmd)` returns `false` when `EnsureServer` returned `serverStarted=false` (server was already running)
- [ ] `serverWasStarted(cmd)` returns `false` when the context value was never set (skipTmuxCheck commands, or CheckTmuxAvailable failure)
- [ ] `PersistentPreRunE` stores `serverStarted` in the command context after `EnsureServer()` returns
- [ ] The context key type is unexported (prevents collisions with other packages)
- [ ] All existing tests in `cmd/root_test.go` continue to pass

**Tests**:
- `"returns true when context has serverStarted=true"` — create a `cobra.Command`, set context with `serverStartedKey=true`, call `serverWasStarted`, assert true
- `"returns false when context has serverStarted=false"` — create a `cobra.Command`, set context with `serverStartedKey=false`, call `serverWasStarted`, assert false
- `"returns false when context has no serverStarted value"` — create a `cobra.Command` with default context (no value set), call `serverWasStarted`, assert false
- `"returns false for nil context value of wrong type"` — create a `cobra.Command`, set context with `serverStartedKey="not-a-bool"` (string instead of bool), call `serverWasStarted`, assert false

**Edge Cases**:
- skipTmuxCheck commands have no context value: These commands return `nil` from `PersistentPreRunE` before reaching `EnsureServer`. Their context is the default `context.Background()`. `serverWasStarted` returns `false` because the type assertion `.(bool)` fails on a nil interface value. This is correct — these commands should never show bootstrap messaging.
- CheckTmuxAvailable failure prevents context being set: If tmux is not installed, `PersistentPreRunE` returns the error before reaching `EnsureServer`. The context is never modified. However, this is moot because the command itself fails — `RunE` is never called. Still, `serverWasStarted` would safely return `false` if somehow called.

**Context**:
> Cobra's `cmd.Context()` returns `context.Background()` by default (cobra v1.10.2). `cmd.SetContext(ctx)` replaces it. The context flows from `PersistentPreRunE` to `RunE` because they operate on the same `*cobra.Command` instance. This is the idiomatic cobra approach for passing data between pre-run hooks and run functions.
>
> The spec's "Two-phase ownership" says: "Session wait -- ownership depends on context: CLI path: The command owns the wait." This task provides the mechanism for commands to know they're in the bootstrap path. Task 2-3 consumes this value to trigger the CLI wait.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Two-phase ownership" under "Bootstrap Mechanism"

## auto-start-tmux-server-2-3 | approved

### Task 3: CLI bootstrap wait integration

**Problem**: CLI commands (`list`, `attach`, `kill`, and `open` in its non-TUI path) need to wait for sessions after bootstrap starts the server. Without this, a `portal list` right after reboot would see zero sessions because continuum has not had time to restore them. The spec requires printing "Starting tmux server..." to stderr (not stdout, so piping works) and blocking through the session wait. When the server was already running, no message and no wait should occur.

**Solution**: Add a shared `bootstrapWait` helper function in the `cmd` package that checks `serverWasStarted(cmd)` (from Task 2-2), prints the stderr message, and calls `tmux.WaitForSessions` (from Task 2-1). Integrate it into the `RunE` of `list`, `attach`, and `kill` commands. For `open`, integrate it only in the non-TUI code paths (the `openPath` function and the fallback/destination resolution paths) — the TUI path (`openTUI`) skips the CLI wait because Phase 3 handles it with the Bubble Tea interstitial.

**Outcome**: After a bootstrap server start, CLI commands print "Starting tmux server..." to stderr, wait for sessions with timing bounds, then proceed to normal execution. When the server was already running, commands execute immediately with no message or wait. Piping stdout works cleanly because the status message goes to stderr.

**Do**:
- Create a shared helper function in `/Users/leeovery/Code/portal/cmd/bootstrap_wait.go`:
  ```go
  package cmd

  import (
      "fmt"
      "os"

      "github.com/leeovery/portal/internal/tmux"
      "github.com/spf13/cobra"
  )

  // bootstrapWait prints a status message to stderr and waits for sessions
  // if the server was started during bootstrap. Returns immediately if the
  // server was already running. The waiter parameter is injectable for testing;
  // when nil, the default WaitForSessions with a real tmux client is used.
  func bootstrapWait(cmd *cobra.Command, waiter func()) {
      if !serverWasStarted(cmd) {
          return
      }
      fmt.Fprintln(os.Stderr, "Starting tmux server...")
      if waiter != nil {
          waiter()
          return
      }
      client := tmux.NewClient(&tmux.RealCommander{})
      tmux.WaitForSessions(tmux.DefaultWaitConfig(client))
  }
  ```
- Add `bootstrapWait(cmd, nil)` as the first call in the `RunE` of:
  - `/Users/leeovery/Code/portal/cmd/list.go` — at the top of `RunE`, before the flag parsing
  - `/Users/leeovery/Code/portal/cmd/attach.go` — at the top of `RunE`, before name validation
  - `/Users/leeovery/Code/portal/cmd/kill.go` — at the top of `RunE`, before name validation
- For `/Users/leeovery/Code/portal/cmd/open.go`:
  - Add `bootstrapWait(cmd, nil)` at the top of `RunE`, but only for the non-TUI path. Since the `open` command resolves what to do (TUI vs path vs fallback) inside `RunE`, the cleanest approach is: call `bootstrapWait(cmd, nil)` at the top of `RunE` unconditionally **except** when `destination == ""` (which means TUI path). When `destination == ""`, skip the wait — the TUI handles it in Phase 3.
  - Concretely, after `parseCommandArgs` and before the `if destination == ""` check, add:
    ```go
    if destination != "" {
        bootstrapWait(cmd, nil)
    }
    ```
  - The `destination == ""` path calls `openTUI` which will handle its own wait in Phase 3. The `destination != ""` path resolves to either `openPath` or `openTUI(query, ...)` — both are CLI-ish paths that should wait.
  - **Refinement**: Actually, the fallback case (`FallbackResult`) also calls `openTUI`. So only the `PathResult` case truly needs the CLI wait. But calling `bootstrapWait` before the resolution is simpler and harmless — the TUI will just see sessions already loaded. The brief wait (max 6s) before TUI launch is acceptable since the TUI would have waited anyway.
- Create tests in `/Users/leeovery/Code/portal/cmd/bootstrap_wait_test.go`:
  - Test the `bootstrapWait` function directly with mock cobra commands and injected waiters
  - For CLI command integration, test that the wait is called when serverStarted is in context

**Acceptance Criteria**:
- [ ] "Starting tmux server..." is printed to stderr (not stdout) when `serverWasStarted` returns true
- [ ] No message is printed when `serverWasStarted` returns false (server was already running)
- [ ] `WaitForSessions` is called when `serverWasStarted` returns true
- [ ] `WaitForSessions` is NOT called when `serverWasStarted` returns false
- [ ] `list` command calls `bootstrapWait` before listing sessions
- [ ] `attach` command calls `bootstrapWait` before validating the session name
- [ ] `kill` command calls `bootstrapWait` before validating the session name
- [ ] `open` command calls `bootstrapWait` for non-TUI paths (destination is not empty)
- [ ] `open` command does NOT call `bootstrapWait` for the TUI path (destination is empty) — Phase 3 handles that
- [ ] Piping works: `portal list | grep dev` sees only session names on stdout; "Starting tmux server..." goes to stderr only
- [ ] All existing tests continue to pass

**Tests**:
- `"prints starting message to stderr when server was started"` — create a cobra.Command with `serverStartedKey=true` in context; capture stderr; call `bootstrapWait` with a no-op waiter; assert stderr contains "Starting tmux server..."
- `"calls waiter when server was started"` — create a cobra.Command with `serverStartedKey=true`; track whether injected waiter was called; assert waiter was called
- `"does not print message when server was not started"` — create a cobra.Command with `serverStartedKey=false`; capture stderr; call `bootstrapWait`; assert stderr is empty
- `"does not call waiter when server was not started"` — create a cobra.Command with `serverStartedKey=false`; track waiter calls; assert waiter was NOT called
- `"does not print message when context has no serverStarted"` — create a cobra.Command with default context; capture stderr; call `bootstrapWait`; assert stderr is empty
- `"list command outputs to stdout not stderr"` — inject listDeps with sessions, set `serverStartedKey=true` in context with a no-op waiter override; run `list`; verify session output is on stdout only (not mixed with stderr message)

**Edge Cases**:
- stderr message only when serverStarted=true: When the server was already running, `serverWasStarted(cmd)` returns false and `bootstrapWait` returns immediately. No message, no wait. This is the fast path for normal operation (not after reboot).
- open TUI path skips CLI wait: When `open` is called with no destination (`portal open` or just `x`), it enters the TUI path. The CLI wait is skipped because Phase 3's TUI interstitial handles the wait with a visual loading state. If `bootstrapWait` were called before the TUI, the user would see a blank terminal for up to 6 seconds before the TUI appears — bad UX.
- Piping works (stderr vs stdout): The "Starting tmux server..." message goes to `os.Stderr` via `fmt.Fprintln(os.Stderr, ...)`. Normal command output (session names, etc.) goes to `cmd.OutOrStdout()`. When piping (`portal list | grep dev`), only stdout is piped. stderr goes to the terminal, so the user sees the loading message while piped output waits.

**Context**:
> The spec says: "CLI path: Print a status message to stderr ('Starting tmux server...') and block briefly. Normal command output goes to stdout. Piping works cleanly since the status message is on stderr." And: "After the wait completes, bootstrap returns control to the command. The command then executes normally -- it queries tmux for sessions as it always does. Bootstrap doesn't pass session data through; it just ensures the server has had time to start and plugins have had time to restore sessions."
>
> The `bootstrapWait` function accepts an injectable `waiter` parameter so tests can verify the wait is triggered without actually sleeping. In production, `nil` is passed and the function uses `tmux.WaitForSessions` with `DefaultWaitConfig`. In tests, a no-op or tracking closure is injected.
>
> For the `open` command: the spec's "Two-phase ownership" says the TUI path owns its own wait via the Bubble Tea model. The CLI path (path resolution) calls `bootstrapWait` to cover commands like `portal open ~/projects/myapp`. The TUI path (`portal open` with no args) defers to Phase 3.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "CLI path" under "User Experience", "Two-phase ownership" under "Bootstrap Mechanism"

# Plan: Auto Start Tmux Server

## Phase 1: Bootstrap Core — Server Detection and Start

**Goal**: Add the shared bootstrap function that detects whether a tmux server is running and starts one if not, integrated into `PersistentPreRunE` so all tmux-requiring commands trigger it.

**Acceptance Criteria**:
- [ ] A `ServerRunning()` function in the `tmux` package detects whether a tmux server is currently running
- [ ] A `StartServer()` function in the `tmux` package runs `tmux start-server` as a one-shot attempt (no retry)
- [ ] `PersistentPreRunE` calls `CheckTmuxAvailable()` then the bootstrap function for commands that require tmux
- [ ] Commands in `skipTmuxCheck` skip both the tmux check and bootstrap
- [ ] When the server is already running, bootstrap returns immediately with no side effects (fast path)
- [ ] When the server is not running, `tmux start-server` is called once and the function returns regardless of outcome
- [ ] All new functions use the existing `Commander` interface for testability

## Phase 2: Session Wait with Timing Bounds

**Goal**: Implement the session-detection polling logic with min/max timing bounds and integrate it into CLI commands that need to wait for sessions after bootstrap.

**Acceptance Criteria**:
- [ ] Named constants define minimum wait (1s), maximum wait (6s), and poll interval (500ms)
- [ ] A session waiter polls `tmux list-sessions` at 500ms intervals, exiting early when sessions are detected but not before the minimum wait
- [ ] The waiter always returns after the maximum wait, even if no sessions appear
- [ ] CLI commands (`list`, `attach`, `kill`, `open` in non-TUI path) print "Starting tmux server..." to stderr when bootstrap started the server, then block through the session wait
- [ ] When the server was already running (bootstrap was skipped), no stderr message is printed and no wait occurs
- [ ] Normal command output still goes to stdout; piping works cleanly

## Phase 3: TUI Loading Interstitial

**Goal**: Add a dedicated loading view to the Bubble Tea TUI that displays "Starting tmux server..." when bootstrap started the server, transitioning to the normal view once sessions are detected or timing bounds are met.

**Acceptance Criteria**:
- [ ] When bootstrap started the server, the TUI opens to a loading interstitial showing centered "Starting tmux server..." text — no logo, no progress bar
- [ ] The interstitial is visually distinct from the normal session/project list views
- [ ] The TUI's existing session refresh cycle detects sessions appearing; transition happens when sessions are detected AND the minimum wait (1s) has elapsed
- [ ] If the maximum wait (6s) elapses with no sessions, the TUI transitions to its normal view (empty state) regardless
- [ ] When the server was already running (bootstrap skipped), the TUI opens directly to its normal view with no interstitial
- [ ] The interstitial does not block or freeze — the TUI remains responsive (Ctrl+C quits)

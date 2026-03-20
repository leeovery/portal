---
topic: auto-start-tmux-server
cycle: 1
total_proposed: 4
---
# Analysis Tasks: Auto Start Tmux Server (Cycle 1)

## Task 1: Fix double session wait on open command fallback-to-TUI path
status: pending
severity: medium
sources: standards

**Problem**: When `open` is given a destination that fails resolution and falls back to the TUI, both the CLI session wait (`bootstrapWait` at line 89) and the TUI loading interstitial (via `serverWasStarted` passed at line 107) execute. If the server was just bootstrapped, the user experiences two sequential waits (up to 12s total). The spec's two-phase ownership model requires session wait to belong to either the CLI path or the TUI path, not both.

**Solution**: When falling back to TUI after `bootstrapWait` already ran, pass `false` for `serverStarted` to `openTUI` so the TUI skips its loading interstitial. The CLI wait already handled the session wait.

**Outcome**: The fallback-to-TUI path in `open` executes at most one session wait, consistent with the spec's two-phase ownership model.

**Do**:
1. In `cmd/open.go`, locate the fallback path around line 107 where `openTUI` is called after `bootstrapWait` has already executed.
2. Change the `serverStarted` argument passed to `openTUI` to `false` on this specific path, so the TUI does not repeat the session wait.
3. Verify that the direct TUI path (no destination provided) still correctly passes the real `serverWasStarted(cmd)` value.

**Acceptance Criteria**:
- When open falls back to TUI after CLI bootstrapWait already ran, the TUI loading interstitial is skipped
- When open goes directly to TUI (no destination), the TUI loading interstitial still appears if the server was just started

**Tests**:
- Test that open with a bad destination + server-just-started falls back to TUI without triggering a second wait
- Test that open with no destination + server-just-started still shows TUI loading interstitial

## Task 2: Extract tmux.NewClient construction to a single helper
status: pending
severity: medium
sources: duplication, architecture

**Problem**: `tmux.NewClient(&tmux.RealCommander{})` appears 8 times across 6 files in the cmd package. PersistentPreRunE already creates a Client for EnsureServer but discards it; each command's RunE then creates its own. If client construction ever changes (e.g., adding a socket path option), all 8 sites must be updated.

**Solution**: Create the Client once in PersistentPreRunE and store it in the command context (alongside the existing `serverStartedKey` pattern). Commands retrieve it via a helper function. This removes 7 of the 8 construction sites.

**Outcome**: A single tmux.Client construction site in PersistentPreRunE, with all commands retrieving the shared instance from context.

**Do**:
1. In `cmd/root.go`, define a new context key (e.g., `tmuxClientKey`) following the same pattern as `serverStartedKey`.
2. In PersistentPreRunE, after creating the Client for `EnsureServer`, store it in the command context.
3. Add a helper function (e.g., `tmuxClient(cmd *cobra.Command) *tmux.Client`) that retrieves the client from context.
4. Replace all 7 remaining `tmux.NewClient(&tmux.RealCommander{})` calls in cmd/bootstrap_wait.go, cmd/kill.go, cmd/attach.go, cmd/list.go, and cmd/open.go (3 sites) with the context helper.
5. Ensure tests that inject deps still work — they bypass the real client via their deps structs, so the context client is unused in test paths.

**Acceptance Criteria**:
- Only one `tmux.NewClient(&tmux.RealCommander{})` call exists in the cmd package (in PersistentPreRunE)
- All commands retrieve the client from context
- All existing tests pass without modification

**Tests**:
- Existing cmd test suite passes (no new tests needed — this is a pure refactor)

## Task 3: Make bootstrapWait injection consistent with interface-based DI pattern
status: pending
severity: medium
sources: architecture

**Problem**: `bootstrapWait` accepts a bare `func()` parameter — when nil it builds real dependencies internally. This nil-or-not branching mixes "am I in a test?" logic with business logic. Every other command uses interface-based DI via a deps struct, making bootstrapWait the sole outlier.

**Solution**: Add a `Waiter` field (or equivalent) to the `bootstrapDeps` struct so bootstrapWait's waiting behavior is injected through the same mechanism as all other commands. Remove the nil-check branching from inside bootstrapWait.

**Outcome**: bootstrapWait uses the same DI pattern as attach, kill, list, and open — no special-case nil-check injection.

**Do**:
1. Define a `Waiter` interface or type alias (e.g., `type BootstrapWaiter interface { Wait(cmd *cobra.Command) }` or a `WaitFunc` type) that represents the "wait for sessions" operation.
2. Add a `Waiter` field to the `bootstrapDeps` struct in cmd/bootstrap_wait.go.
3. Refactor `bootstrapWait` to use `bootstrapDeps.Waiter` instead of the nilable `func()` parameter.
4. Update the production build path to set the Waiter field with the real implementation.
5. Update all call sites (cmd/attach.go, cmd/kill.go, cmd/list.go, cmd/open.go) to remove the `func()` argument from bootstrapWait calls.
6. Update tests to inject the Waiter via the deps struct instead of passing a closure.

**Acceptance Criteria**:
- bootstrapWait no longer accepts a bare func() parameter
- Wait behavior is injected via the bootstrapDeps struct
- No nil-check branching remains in bootstrapWait for DI purposes

**Tests**:
- Existing bootstrap_wait tests pass, updated to use deps-based injection
- Existing command tests (attach, kill, list, open) pass with updated bootstrapWait call signatures

## Task 4: Eliminate package-level mutable DI vars to prevent test isolation risk
status: pending
severity: medium
sources: architecture

**Problem**: Each command uses a package-level var (`attachDeps`, `killDeps`, `listDeps`, `openDeps`, `bootstrapDeps`) that tests mutate and clean up via `t.Cleanup`. Because cobra commands and rootCmd are also package-level singletons, adding `t.Parallel` to any cmd test will produce data races. This is a latent fragility — tests work today only because they run sequentially.

**Solution**: Either (a) refactor to pass deps through command context in PersistentPreRunE, eliminating the package-level mutable vars entirely, or (b) if the current approach is intentionally chosen for simplicity, add a documented "no t.Parallel in cmd tests" rule with a vet check or comment convention. Option (a) is the stronger fix; option (b) is acceptable if the refactor scope is too large.

**Outcome**: Either package-level mutable DI vars are eliminated (option a) or the constraint is explicitly documented and enforced (option b). No latent data-race risk from future t.Parallel additions.

**Do**:
Option A (preferred):
1. Define a deps container that holds all command dependencies, constructable via a builder function.
2. In PersistentPreRunE, build the deps container and store it in the command context.
3. Each command's RunE retrieves deps from context instead of reading a package-level var.
4. Tests inject deps by setting them in the command context before executing.
5. Remove all package-level `xxxDeps` vars.

Option B (minimal):
1. Add a comment block at the top of each `_test.go` file in cmd/ explaining that t.Parallel must not be used due to package-level mutable state.
2. Consider adding a linter or CI check that flags t.Parallel usage in cmd/ test files.

**Acceptance Criteria**:
- Option A: No package-level mutable dep vars exist in cmd/; all deps flow through context
- Option B: Every cmd test file has a documented no-parallel constraint
- All existing tests pass

**Tests**:
- All existing cmd tests pass
- Option A: verify with `-race` flag that tests can run with t.Parallel without races (stretch goal)

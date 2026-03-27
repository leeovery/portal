---
phase: 1
phase_name: Hook Registry and CLI Surface
total: 5
---

## resume-sessions-after-reboot-1-1 | approved

### Task 1: Hook Store

**Problem**: Portal needs a persistent registry that maps tmux pane IDs to restart commands so that after a reboot, it knows which commands to re-execute in which panes. No such storage exists today.

**Solution**: Create a new `internal/hooks` package with a JSON-backed `Store` that persists hook data at `~/.config/portal/hooks.json`. The store uses the atomic write pattern from `internal/project/store.go` (temp file + rename) and models hooks as a map of pane ID to a map of event type to command string.

**Outcome**: A fully tested `hooks.Store` that can load, save, set, remove, list, and get hooks by pane ID, with safe handling of missing/malformed files and atomic writes that create parent directories.

**Do**:
- Create `internal/hooks/store.go` with:
  - Type `Hook` struct with fields `PaneID string`, `Event string`, `Command string` (used for list output)
  - Type `hooksFile` as the on-disk JSON structure: `map[string]map[string]string` (outer key = pane ID like `%3`, inner key = event type like `on-resume`, value = command string)
  - Type `Store` struct with a `path string` field
  - `NewStore(path string) *Store` constructor
  - `Load() (map[string]map[string]string, error)` -- reads JSON file, returns empty map for missing file (`os.ErrNotExist`) or malformed JSON (unmarshal error). Follow the exact pattern from `project/store.go` Load method.
  - `Save(hooks map[string]map[string]string) error` -- atomic write using temp file + `os.Rename`. Create parent directory with `os.MkdirAll(dir, 0o755)`. Use `json.MarshalIndent` with two-space indent for human readability. Follow the exact pattern from `project/store.go` Save method.
  - `Set(paneID, event, command string) error` -- loads current hooks, sets `hooks[paneID][event] = command` (creating the inner map if needed), then saves. This is idempotent: overwrites any existing entry for the same pane and event.
  - `Remove(paneID, event string) error` -- loads current hooks, deletes `hooks[paneID][event]`, removes the outer key if the inner map is now empty, then saves. No-op (no error) if pane or event does not exist; still saves for consistency.
  - `List() ([]Hook, error)` -- loads hooks and returns a flat slice of `Hook` structs (one per pane/event combination), sorted by pane ID then event type.
  - `Get(paneID string) (map[string]string, error)` -- loads hooks and returns the event map for a specific pane, or an empty map if the pane has no hooks.
- Create `internal/hooks/store_test.go` with tests listed below

**Acceptance Criteria**:
- [ ] `internal/hooks/store.go` exists with `Store` struct and all methods: `NewStore`, `Load`, `Save`, `Set`, `Remove`, `List`, `Get`
- [ ] `Load` returns an empty map when the file does not exist (no error)
- [ ] `Load` returns an empty map when the file contains malformed JSON (no error)
- [ ] `Save` creates the parent directory if it does not exist
- [ ] `Save` uses atomic write (temp file + rename pattern)
- [ ] `Set` is idempotent -- calling it twice for the same pane/event overwrites the command
- [ ] `Remove` is a no-op when the pane/event does not exist (no error returned)
- [ ] `Remove` cleans up the outer map key when the inner map becomes empty
- [ ] `List` returns hooks sorted by pane ID, then event type
- [ ] `Get` returns an empty map for an unregistered pane
- [ ] All tests pass: `go test ./internal/hooks/...`

**Tests**:
- `"Load returns empty map when file does not exist"`
- `"Load returns empty map when file contains malformed JSON"`
- `"Load returns hooks from valid JSON file"`
- `"Save creates parent directory if missing"`
- `"Save writes valid JSON that can be loaded back"`
- `"Save uses atomic write (file exists after save even if interrupted)"`
- `"Set adds a new hook for a new pane"`
- `"Set adds a second event to an existing pane"`
- `"Set overwrites existing entry for same pane and event"`
- `"Remove deletes a hook entry"`
- `"Remove cleans up outer key when inner map is empty"`
- `"Remove is a no-op when pane does not exist"`
- `"Remove is a no-op when event does not exist for pane"`
- `"List returns empty slice when no hooks"`
- `"List returns hooks sorted by pane ID then event"`
- `"Get returns event map for registered pane"`
- `"Get returns empty map for unregistered pane"`

**Edge Cases**:
- Missing file returns empty map (not an error) -- same pattern as `project/store.go`
- Malformed JSON returns empty map (not an error) -- same pattern as `project/store.go`
- Atomic write creates parent directory -- `os.MkdirAll` before creating temp file
- Set is idempotent (overwrites existing entry for same pane and event) -- second call for `%3`/`on-resume` replaces the command, does not duplicate

**Context**:
> The spec defines the storage format as: `{ "%3": { "on-resume": "claude --resume abc123" }, "%7": { "on-resume": "claude --resume def456" } }`. The outer key is the pane ID (globally unique across the tmux server, persists across tmux-resurrect). The inner key is the event type. Only `on-resume` is implemented initially but the surface supports future event types. Hooks are structured data (pane x event type), so JSON is used rather than a flat key=value file. The file path is `~/.config/portal/hooks.json`.

**Spec Reference**: `Registry Model & Storage` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-1-2 | approved

### Task 2: Tmux Server Option Methods

**Problem**: The volatile marker mechanism requires setting, reading, and deleting tmux server-level user options (`@portal-active-{pane_id}`). The existing `tmux.Client` in `internal/tmux/tmux.go` has no methods for server option operations.

**Solution**: Add three new methods to `tmux.Client`: `SetServerOption`, `GetServerOption`, and `DeleteServerOption`. These wrap `tmux set-option -s`, `tmux show-option -sv`, and `tmux set-option -su` respectively, targeting `@`-prefixed user options at server level.

**Outcome**: `tmux.Client` can manage server-level user options, enabling the volatile marker mechanism used by `hooks set`, `hooks rm`, and the execution flow in Phase 2.

**Do**:
- Add to `internal/tmux/tmux.go`:
  - `var ErrOptionNotFound = errors.New("option not found")` -- exported sentinel error. Add `"errors"` to the import block.
  - `SetServerOption(name, value string) error` -- runs `c.cmd.Run("set-option", "-s", name, value)`. Returns `fmt.Errorf("failed to set server option %q: %w", name, err)` on failure.
  - `GetServerOption(name string) (string, error)` -- runs `c.cmd.Run("show-option", "-sv", name)`. On success, returns the trimmed output string. On error (Commander returns non-nil error), returns `("", ErrOptionNotFound)`. The `-v` flag causes tmux to return just the value without the option name prefix.
  - `DeleteServerOption(name string) error` -- runs `c.cmd.Run("set-option", "-su", name)`. The `-u` flag unsets the option. Returns `fmt.Errorf("failed to delete server option %q: %w", name, err)` on failure. tmux does not error when unsetting a non-existent option, so this is naturally a no-op.
- Add tests to `internal/tmux/tmux_test.go` using the existing `MockCommander` pattern (already defined in that file with `Output`, `Err`, `Calls`, and `RunFunc` fields)

**Acceptance Criteria**:
- [ ] `SetServerOption` calls `tmux set-option -s <name> <value>` via the Commander
- [ ] `GetServerOption` calls `tmux show-option -sv <name>` via the Commander and returns the value
- [ ] `GetServerOption` returns `ErrOptionNotFound` when the Commander returns an error (option does not exist)
- [ ] `DeleteServerOption` calls `tmux set-option -su <name>` via the Commander
- [ ] `DeleteServerOption` succeeds (no error) when the option does not exist
- [ ] `ErrOptionNotFound` is exported from the `tmux` package
- [ ] All tests pass: `go test ./internal/tmux/...`

**Tests**:
- `"SetServerOption runs set-option -s with name and value"`
- `"SetServerOption returns error when tmux command fails"`
- `"GetServerOption returns value when option exists"`
- `"GetServerOption returns ErrOptionNotFound when option does not exist"`
- `"DeleteServerOption runs set-option -su with name"`
- `"DeleteServerOption succeeds when option does not exist"`
- `"DeleteServerOption returns error when tmux command fails"`

**Edge Cases**:
- `GetServerOption` returns `ErrOptionNotFound` when option does not exist -- the Commander returns a non-nil error for `show-option -sv` on a missing option. Return the sentinel `ErrOptionNotFound` so callers can check with `errors.Is`.
- `DeleteServerOption` for non-existent option -- tmux `set-option -su` does not error when the option is already absent, so the Commander returns nil. No special handling needed; this is naturally a no-op.

**Context**:
> The spec uses tmux server options as volatile storage: `set-option -s @portal-active-%3 1` on register, queried with `show-option -sv`, removed with `set-option -su`. These markers die with the server (tmux-resurrect does not restore options) and their absence indicates "server restarted since registration." The `@` prefix is required for user-defined tmux options. The `-s` flag targets server level (vs session/window/pane level).

**Spec Reference**: `Volatile Marker Mechanism` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-1-3 | approved

### Task 3: Hooks List Command

**Problem**: Users and tools need to inspect what hooks are registered across all panes. There is no CLI command to list the hook registry contents.

**Solution**: Create the `hooks` parent command and `hooks list` subcommand in `cmd/hooks.go`. The `hooks` command is added to `skipTmuxCheck` in `cmd/root.go` so it bypasses Portal's tmux bootstrap (same as `alias`). `hooks list` reads the hook store JSON file and outputs one line per hook in tab-separated format.

**Outcome**: Running `portal hooks list` outputs all registered hooks in `%3\ton-resume\tclaude --resume abc123` format, or produces no output when the store is empty.

**Do**:
- Create `cmd/hooks.go` with:
  - `var hooksCmd = &cobra.Command{Use: "hooks", Short: "Manage resume hooks"}` -- parent command, no `RunE` (just a container for subcommands)
  - `var hooksListCmd = &cobra.Command{...}` with `Use: "list"`, `Short: "List all registered hooks"`, `Args: cobra.NoArgs`, and a `RunE` that:
    1. Calls `loadHookStore()` (helper function, see below)
    2. Calls `store.List()` to get sorted hooks
    3. Iterates over hooks and writes each as `fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", h.PaneID, h.Event, h.Command)` -- tab-separated: pane ID, event type, command
    4. Returns nil on success (empty store produces no output, no error)
  - `func loadHookStore() (*hooks.Store, error)` helper that:
    1. Calls `hooksFilePath()` to get the file path
    2. Returns `hooks.NewStore(path), nil`
  - `func hooksFilePath() (string, error)` helper that calls `configFilePath("PORTAL_HOOKS_FILE", "hooks.json")` -- follows the exact pattern from `aliasFilePath()` in `cmd/alias.go`. The `PORTAL_HOOKS_FILE` env var allows tests to redirect to a temp file.
  - `func init()` that registers `hooksListCmd` under `hooksCmd`, and `hooksCmd` under `rootCmd`
- Modify `cmd/root.go`:
  - Add `"hooks": true` to the `skipTmuxCheck` map (line 13-19), alongside `"alias"`, `"clean"`, etc.
- Create `cmd/hooks_test.go` with tests following the pattern in `cmd/alias_test.go`:
  - Use `t.TempDir()` for temp hook files
  - Use `t.Setenv("PORTAL_HOOKS_FILE", ...)` to redirect store path
  - Use `resetRootCmd()` before each test
  - Capture output via `rootCmd.SetOut(buf)` and compare

**Acceptance Criteria**:
- [ ] `portal hooks list` outputs all hooks in tab-separated format: `pane_id\tevent\tcommand\n`
- [ ] Empty store produces no output and no error
- [ ] `hooks` bypasses tmux bootstrap (`skipTmuxCheck` contains `"hooks"`)
- [ ] `portal hooks list` works without tmux installed (uses `t.Setenv("PATH", "/nonexistent/path")` in test)
- [ ] `PORTAL_HOOKS_FILE` env var overrides the default hooks file path
- [ ] All tests pass: `go test ./cmd -run TestHooks`

**Tests**:
- `"outputs hooks in tab-separated format"`
- `"produces empty output when no hooks registered"`
- `"produces empty output when hooks file does not exist"`
- `"outputs hooks sorted by pane ID"`
- `"hooks bypasses tmux bootstrap"`
- `"accepts no arguments"`

**Edge Cases**:
- Empty store produces no output -- `store.List()` returns an empty slice, the loop body never executes, no bytes written to stdout
- Hooks bypasses tmux bootstrap -- `"hooks"` is in `skipTmuxCheck`, so `PersistentPreRunE` returns early before calling `tmux.CheckTmuxAvailable()` or `EnsureServer()`

**Context**:
> The spec defines list output as: one line per hook, tab-separated: pane ID, event type, command. Example: `%3  on-resume  claude --resume abc123`. The `hooks list` command only reads the JSON file and does not need tmux at all. It is added to `skipTmuxCheck` like `alias`. The command mirrors `xctl alias list` for consistency.

**Spec Reference**: `CLI Surface` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-1-4 | approved

### Task 4: Hooks Set Command

**Problem**: External tools (like Claude Code) need a CLI command to register restart hooks from within a tmux pane. The command must write both the persistent JSON entry and the volatile tmux server marker atomically, so Portal knows the hook was registered on this server lifetime.

**Solution**: Add the `hooks set` subcommand in `cmd/hooks.go`. It reads `$TMUX_PANE` for the pane ID, requires the `--on-resume` flag for the event type, writes the hook to the persistent store, and sets the volatile marker `@portal-active-{pane_id}` via `tmux.Client.SetServerOption`. A `hooksDeps` DI struct provides the tmux client for testing (since `hooks` bypasses the bootstrap and has no client in context).

**Outcome**: Running `portal hooks set --on-resume "claude --resume abc123"` from inside tmux pane `%3` writes `{"%3": {"on-resume": "claude --resume abc123"}}` to `hooks.json` and sets `@portal-active-%3` server option. Running outside tmux (no `$TMUX_PANE`) produces an error.

**Do**:
- Add to `cmd/hooks.go`:
  - `ServerOptionSetter` interface: `SetServerOption(name, value string) error` -- small interface for DI, satisfied by `*tmux.Client`
  - `var hooksDeps *HooksDeps` package-level variable for test injection
  - `type HooksDeps struct { OptionSetter ServerOptionSetter }` -- holds injectable tmux client dependency
  - `func buildHooksDeps() ServerOptionSetter` helper that returns `hooksDeps.OptionSetter` if `hooksDeps != nil`, otherwise creates a real `tmux.NewClient(&tmux.RealCommander{})` and returns it. This is necessary because `hooks` bypasses `PersistentPreRunE` (it's in `skipTmuxCheck`), so there's no client in the cobra context.
  - `var hooksSetCmd = &cobra.Command{...}` with `Use: "set"`, `Short: "Register a resume hook for the current pane"`, `Args: cobra.NoArgs`, and a `RunE` that:
    1. Reads `os.Getenv("TMUX_PANE")` -- if empty, return `fmt.Errorf("must be run from inside a tmux pane")`
    2. Reads the `--on-resume` flag value -- the flag is a string flag registered in `init()`. If the flag was not provided (empty string after cobra marks it required, or check `cmd.Flags().Changed("on-resume")`), cobra's required flag validation handles the error.
    3. Calls `loadHookStore()` to get the store
    4. Calls `store.Set(paneID, "on-resume", command)` to write the persistent entry
    5. Calls `buildHooksDeps()` to get the `ServerOptionSetter`
    6. Calls `setter.SetServerOption("@portal-active-"+paneID, "1")` to set the volatile marker
    7. Returns nil on success
  - Register `--on-resume` as a required string flag on `hooksSetCmd`: `hooksSetCmd.Flags().String("on-resume", "", "command to run on session resume")` followed by `hooksSetCmd.MarkFlagRequired("on-resume")` (Cobra enforces this; omitting the flag produces an error automatically)
  - Register `hooksSetCmd` under `hooksCmd` in `init()`
- Create or extend `cmd/hooks_test.go` with `TestHooksSetCommand` following the pattern in `cmd/alias_test.go`:
  - Use `t.Setenv("PORTAL_HOOKS_FILE", ...)` for store redirection
  - Use `t.Setenv("TMUX_PANE", "%3")` to simulate being inside tmux
  - Inject `hooksDeps` with a mock `ServerOptionSetter` to capture and verify the `SetServerOption` call without needing a real tmux server
  - Use `t.Cleanup(func() { hooksDeps = nil })` to restore after each test
  - Verify the JSON file contains the expected hook entry
  - Verify the mock received `SetServerOption("@portal-active-%3", "1")`

**Acceptance Criteria**:
- [ ] `portal hooks set --on-resume "cmd"` writes hook to `hooks.json` for the pane in `$TMUX_PANE`
- [ ] `portal hooks set --on-resume "cmd"` sets volatile marker `@portal-active-{pane_id}` with value `"1"`
- [ ] Running without `$TMUX_PANE` set produces error: "must be run from inside a tmux pane"
- [ ] Running without `--on-resume` flag produces a cobra required-flag error
- [ ] Calling set twice for the same pane overwrites the command (idempotent)
- [ ] `hooksDeps` DI struct allows test injection of the `ServerOptionSetter`
- [ ] All tests pass: `go test ./cmd -run TestHooksSet`

**Tests**:
- `"sets hook and volatile marker for current pane"`
- `"reads pane ID from TMUX_PANE environment variable"`
- `"returns error when TMUX_PANE is not set"`
- `"returns error when on-resume flag is not provided"`
- `"overwrites existing hook for same pane idempotently"`
- `"writes correct JSON structure to hooks file"`
- `"sets volatile marker with correct option name"`

**Edge Cases**:
- `TMUX_PANE` unset produces error -- `os.Getenv("TMUX_PANE")` returns empty string, command returns `fmt.Errorf("must be run from inside a tmux pane")` before touching the store or tmux
- Idempotent overwrite of existing hook -- calling `store.Set("%3", "on-resume", "new-cmd")` after a previous set for the same pane/event replaces the command. The volatile marker is also re-set (harmless, same value).
- `--on-resume` flag is required -- Cobra's `MarkFlagRequired` ensures the flag must be provided. Running `portal hooks set` without it produces: `required flag(s) "on-resume" not set`

**Context**:
> The spec says: `hooks set --on-resume "claude --resume $SESSION_ID"` writes a persistent entry for `$TMUX_PANE` and sets the volatile marker. The pane ID is inferred from `$TMUX_PANE` -- the caller does not pass it. `set` is idempotent: re-registering overwrites the previous command. The `hooks` command bypasses Portal's tmux bootstrap (`skipTmuxCheck`), but `hooks set` still needs a tmux client for the volatile marker `SetServerOption` call. It creates its own client rather than relying on the context.

**Spec Reference**: `CLI Surface` and `Volatile Marker Mechanism` sections of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-1-5 | approved

### Task 5: Hooks Rm Command

**Problem**: External tools need a way to deregister hooks when they exit cleanly (e.g., Claude Code's `SessionEnd` hook calls `xctl hooks rm --on-resume`). Both the persistent JSON entry and the volatile tmux server marker must be removed.

**Solution**: Add the `hooks rm` subcommand in `cmd/hooks.go`. It reads `$TMUX_PANE` for the pane ID, requires the `--on-resume` flag for the event type, removes the hook from the persistent store, and deletes the volatile marker `@portal-active-{pane_id}` via `tmux.Client.DeleteServerOption`. The removal is a silent no-op if no hook exists for the pane.

**Outcome**: Running `portal hooks rm --on-resume` from inside tmux pane `%3` removes the `on-resume` entry for `%3` from `hooks.json` and deletes the `@portal-active-%3` server option. Running outside tmux (no `$TMUX_PANE`) produces an error. Running when no hook exists is a silent no-op (exit 0).

**Do**:
- Add to `cmd/hooks.go`:
  - `ServerOptionDeleter` interface: `DeleteServerOption(name string) error` -- small interface for DI, satisfied by `*tmux.Client`. (If Task 4 already defined a combined interface or you prefer to combine setter/deleter into one interface used by both commands, that works too. The key requirement is testability.)
  - Extend `HooksDeps` from Task 4 to include `OptionDeleter ServerOptionDeleter` (or use a combined interface if preferred). The `buildHooksDeps` helper should return both setter and deleter (a `*tmux.Client` satisfies both).
  - `var hooksRmCmd = &cobra.Command{...}` with `Use: "rm"`, `Short: "Remove a resume hook for the current pane"`, `Args: cobra.NoArgs`, and a `RunE` that:
    1. Reads `os.Getenv("TMUX_PANE")` -- if empty, return `fmt.Errorf("must be run from inside a tmux pane")`
    2. Validates `--on-resume` flag is provided (same required flag mechanism as Task 4)
    3. Calls `loadHookStore()` to get the store
    4. Calls `store.Remove(paneID, "on-resume")` to remove the persistent entry. This is a no-op if the hook does not exist (store.Remove handles this gracefully).
    5. Calls `buildHooksDeps()` to get the deleter
    6. Calls `deleter.DeleteServerOption("@portal-active-"+paneID)` to remove the volatile marker. `DeleteServerOption` is a no-op if the option does not exist (tmux `set-option -su` handles this).
    7. Returns nil on success
  - Register `--on-resume` as a required flag on `hooksRmCmd`: `hooksRmCmd.Flags().Bool("on-resume", false, "remove the on-resume hook")` followed by `hooksRmCmd.MarkFlagRequired("on-resume")`. Note: this is a bool flag (presence means "remove the on-resume hook"), not a string flag. The flag just selects which event type to remove; it takes no value.
  - Register `hooksRmCmd` under `hooksCmd` in `init()`
- Extend `cmd/hooks_test.go` with `TestHooksRmCommand`:
  - Seed the hooks JSON file with a known entry before running rm
  - Use `t.Setenv("TMUX_PANE", "%3")` to simulate being inside tmux
  - Inject `hooksDeps` with a mock `ServerOptionDeleter` to capture and verify the `DeleteServerOption` call
  - Verify the JSON file no longer contains the removed entry
  - Verify the mock received `DeleteServerOption("@portal-active-%3")`
  - Test the silent no-op case: run rm when no hook exists, verify exit 0 and no error

**Acceptance Criteria**:
- [ ] `portal hooks rm --on-resume` removes hook from `hooks.json` for the pane in `$TMUX_PANE`
- [ ] `portal hooks rm --on-resume` deletes volatile marker `@portal-active-{pane_id}`
- [ ] Running without `$TMUX_PANE` set produces error: "must be run from inside a tmux pane"
- [ ] Running without `--on-resume` flag produces a cobra required-flag error
- [ ] Removing a non-existent hook is a silent no-op (exit 0, no error output)
- [ ] `hooksDeps` DI struct allows test injection of the `ServerOptionDeleter`
- [ ] All tests pass: `go test ./cmd -run TestHooksRm`

**Tests**:
- `"removes hook and volatile marker for current pane"`
- `"reads pane ID from TMUX_PANE environment variable"`
- `"returns error when TMUX_PANE is not set"`
- `"returns error when on-resume flag is not provided"`
- `"silent no-op when no hook exists for pane"`
- `"removes correct JSON entry from hooks file"`
- `"deletes volatile marker with correct option name"`
- `"cleans up pane key when last event removed"`

**Edge Cases**:
- `TMUX_PANE` unset produces error -- `os.Getenv("TMUX_PANE")` returns empty string, command returns `fmt.Errorf("must be run from inside a tmux pane")` before touching the store or tmux
- Silent no-op when no hook exists -- `store.Remove` is a no-op for non-existent entries, `DeleteServerOption` is a no-op for non-existent options. Command returns nil (exit 0). This supports scripting -- tools calling rm in cleanup paths should not fail if the hook was already removed.
- `--on-resume` flag is required -- Cobra's `MarkFlagRequired` ensures the flag must be provided. Running `portal hooks rm` without it produces: `required flag(s) "on-resume" not set`

**Context**:
> The spec says: `hooks rm --on-resume` removes the persistent entry for `$TMUX_PANE` and removes the volatile marker. `hooks rm` is a silent no-op if no hook is registered for the current pane. This supports scripting -- tools calling rm in cleanup paths should not fail if the hook was already removed. The `--on-resume` flag is required for `rm`; running without an event flag is an error.

**Spec Reference**: `CLI Surface` and `Volatile Marker Mechanism` sections of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

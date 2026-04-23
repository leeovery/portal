---
phase: 1
phase_name: Portal state CLI scaffolding & tmux hook registration
total: 9
---

## built-in-session-resurrection-1-1 | approved

### Task 1-1: Scaffold `portal state` command namespace with stub subcommands

**Problem**: Phases 2–6 of this feature all dispatch through a new `portal state ...` command family (`status`, `cleanup`, `daemon`, `notify`, `signal-hydrate`, `hydrate`, and `migrate-rename`). None of those commands exist today. Later phases cannot register tmux hooks that invoke `portal state notify` or `portal state signal-hydrate` until the binary will actually accept those argv shapes. Additionally, Cobra's default help output must only expose user-facing commands (`status`, `cleanup`); the internal commands are machine-invoked from tmux hooks / pane commands and must not appear under `portal --help` or `portal state --help`.

**Solution**: Add a top-level `state` Cobra subcommand under `rootCmd`, with seven child subcommands. Two are user-facing (`status`, `cleanup`) and visible in help; five are internal (`daemon`, `notify`, `signal-hydrate`, `hydrate`, `migrate-rename`) and carry Cobra's `Hidden: true`. All seven `RunE` implementations are stubs for this phase. `cleanup` gets its Phase-1 slice implemented in task 1-9; the rest stay stubbed until later phases.

**Outcome**: Running `portal state` prints the `state` command's own help (Cobra's default when a parent command has no `Run`). `portal --help` lists `state` in the command table. `portal state --help` lists only `status` and `cleanup`. `portal state daemon`, `portal state notify`, `portal state signal-hydrate NAME`, `portal state hydrate --fifo F --file S --hook-key K`, and `portal state migrate-rename OLD NEW` all accept their argv shapes and exit 0 without error. Hidden subcommands do not appear in any help or shell-completion output.

**Do**:
- Create `cmd/state.go` defining `stateCmd` (`Use: "state"`, `Short: "Manage Portal session resurrection state"`, no `Run`/`RunE` so Cobra auto-prints help on bare invocation). Register it via `rootCmd.AddCommand(stateCmd)` in an `init()`.
- Create `cmd/state_status.go`, `cmd/state_cleanup.go`, `cmd/state_daemon.go`, `cmd/state_notify.go`, `cmd/state_signal_hydrate.go`, `cmd/state_hydrate.go`, `cmd/state_migrate_rename.go`. Each defines a Cobra command and attaches itself to `stateCmd` in its own `init()`.
- User-facing subcommands (`status`, `cleanup`) keep `Hidden: false` (the default) and have a clear `Short` description. `cleanup` declares a `--purge` bool flag (body wired in Phase 6; Phase-1 slice handled in task 1-9).
- Internal subcommands (`daemon`, `notify`, `signal-hydrate`, `hydrate`, `migrate-rename`) set `Hidden: true`. Declare their expected positional args and flags now so argv parsing works in later phases:
  - `signal-hydrate`: `Args: cobra.ExactArgs(1)` (session name).
  - `hydrate`: three required string flags `--fifo`, `--file`, `--hook-key` (use `MarkFlagRequired`).
  - `migrate-rename`: `Args: cobra.ExactArgs(2)` (old, new).
  - `daemon`, `notify`: `Args: cobra.NoArgs`.
- Each stub `RunE` returns `nil`. Do not add implementation logic — later tasks / phases replace the body.
- Set `Hidden: true` on each internal leaf command explicitly; do not rely on any inherited visibility.

**Acceptance Criteria**:
- [ ] `portal --help` lists `state` among top-level commands with its `Short` description.
- [ ] `portal state --help` lists exactly `status` and `cleanup` under "Available Commands" (plus Cobra's built-in `help` command).
- [ ] `portal state` (bare, no subcommand) prints help and exits 0.
- [ ] `portal state daemon`, `portal state notify`, `portal state signal-hydrate foo`, `portal state hydrate --fifo /tmp/f --file /tmp/s --hook-key k:0.0`, and `portal state migrate-rename old new` all exit 0 and produce no stderr noise.
- [ ] Hidden subcommands do not appear in `portal --help`, `portal state --help`, or the generated bash/zsh/fish shell completion output.
- [ ] `portal state signal-hydrate` (no args) and `portal state hydrate` (missing any required flag) fail with Cobra's standard validation error and non-zero exit code.
- [ ] `--purge` is accepted on `portal state cleanup` as a boolean flag (parsing only — behaviour lands later).

**Tests**:
- `"it registers state as a top-level command visible in portal --help"`
- `"it lists only status and cleanup under portal state --help"`
- `"it hides daemon/notify/signal-hydrate/hydrate/migrate-rename from all help output"`
- `"it hides internal subcommands from generated shell completions"`
- `"it exits 0 when portal state is invoked bare"`
- `"it exits 0 when each internal subcommand is invoked with valid argv"`
- `"it returns argv validation error when signal-hydrate is invoked without a session name"`
- `"it returns flag validation error when hydrate is invoked without --fifo/--file/--hook-key"`
- `"it accepts --purge as a boolean flag on cleanup"`

**Edge Cases**:
- Hidden subcommands must remain hidden in Cobra's generated completions. Cobra honours `Hidden: true` — verify in a test that renders completions to a buffer and asserts internal command names are absent.
- Bare `portal state` must not print an error or return non-zero; Cobra prints help and returns 0 when a parent has no `RunE`.
- Hidden leaf subcommands set `Hidden: true` on their own command definitions; do not rely on the parent being hidden.

**Context**:
> Spec "CLI Surface → Namespace Rationale": "All eight resurrection-related commands (two user-facing + four internal) cluster under `portal state`. This keeps related commands grouped logically in `portal --help` output (only the user-facing commands are shown), and the internal commands are all reachable via the same `state` prefix when needed for debugging."
>
> Spec "CLI Surface → Internal Subcommands (Hidden from `portal --help`)": "These subcommands are invoked by tmux hooks and the hosted daemon. They are Portal-internal and not intended for direct user invocation, so they are excluded from `--help` output (Cobra's `Hidden: true` pattern or equivalent)."
>
> Spec "CLI Surface → `portal state hydrate --fifo F --file S --hook-key K`": "All three flags are required".
>
> Existing Cobra pattern in `cmd/hooks.go` (`hooksCmd` parent + child `hooksListCmd`, `hooksSetCmd`, `hooksRmCmd`) is the reference structure to follow.
>
> The spec enumerates four internal subcommands in the CLI Surface section but the full plan includes `migrate-rename` (added in Phase 4, separately flagged as "a **separate internal subcommand**" in Resume Hook Firing → Session Rename). Register it hidden now so Phase 4 only fills in the body.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → User-Facing Commands (Under `portal state`)", "CLI Surface → Internal Subcommands (Hidden from `portal --help`)", "Resume Hook Firing → Session Rename: Hook Key Migration".

## built-in-session-resurrection-1-2 | approved

### Task 1-2: Add tmux version detection and >= 3.0 guard

**Problem**: Portal now relies on tmux 3.0+ features — array-indexed global hooks (`set-hook -ga` semantics with per-index removal) and the `show-hooks -g` output format that the content-based idempotency layer parses. Users on older tmux must be stopped with a clear error *before* any `set-hook -ga` call runs, both to avoid the "mysteriously not working" silent-failure mode the spec explicitly argues against and to prevent accidentally mangling an older tmux's hook storage. The version string returned by `tmux -V` can take several shapes across distributions (`tmux 3.3a`, `tmux 3.3-rc`, `tmux-next 4.0`, `tmux 3.0 (OpenBSD)`), so parsing has to be tolerant.

**Solution**: Add a pure parsing function (no I/O) that takes a `tmux -V` output string and returns either `(major, minor, nil)` or a descriptive error. Pair it with a wrapper that runs `tmux -V` through the existing `Commander` interface and returns the same. The public guard function — `CheckTmuxVersion(c Commander) error` — returns `nil` if the parsed version is ≥ 3.0, the spec-defined error message if below, and a wrapped error on any other failure (binary missing, unparseable output, `Commander.Run` failure).

**Outcome**: A caller invoking `tmux.CheckTmuxVersion(commander)` receives `nil` on tmux 3.0/3.3/4.0 (any ≥ 3.0), the exact error `"Portal requires tmux ≥ 3.0 (found <version>). Please upgrade."` on 2.x, and an actionable wrapped error on every malformed / missing / erroring input. All code paths are covered by table-driven tests with the parser's inputs baked in.

**Do**:
- Create `internal/tmux/version.go` with:
  - Exported `parseTmuxVersion(raw string) (major, minor int, label string, err error)` — pure function. `label` preserves the raw version token (`"3.3a"`, `"3.0"`) for use in user-facing error messages.
  - Exported `CheckTmuxVersion(cmd Commander) error`:
    1. Runs `cmd.Run("-V")`.
    2. On `Commander` error, returns `fmt.Errorf("failed to detect tmux version: %w", err)`.
    3. On empty output, returns `errors.New("tmux -V returned no output")`.
    4. Passes the output to `parseTmuxVersion`.
    5. If `major < 3`, returns `fmt.Errorf("Portal requires tmux ≥ 3.0 (found %s). Please upgrade.", label)` — exact wording from the spec.
    6. Else returns `nil`.
- The parser tokenizes on whitespace, finds the token beginning with a digit (skipping leading identifiers like `tmux`, `tmux-next`, etc.), strips any non-digit suffix after the minor component (`-rc`, `-rc1`, `a`, `b`, `c`), and parses `major.minor`. Missing minor → treat as `.0`. Leading/trailing whitespace is trimmed before tokenizing. Trailing parentheticals like `(OpenBSD)` are ignored.
- Create `internal/tmux/version_test.go` with table-driven tests per the test list below, using the `MockCommander` pattern from `internal/tmux/tmux_test.go`.

**Acceptance Criteria**:
- [ ] `parseTmuxVersion("tmux 3.3a")` returns `(3, 3, "3.3a", nil)`.
- [ ] `parseTmuxVersion("tmux 3.0")` returns `(3, 0, "3.0", nil)`.
- [ ] `parseTmuxVersion("tmux 2.9")` returns `(2, 9, "2.9", nil)`.
- [ ] `parseTmuxVersion("tmux-next 4.0")` returns `(4, 0, "4.0", nil)`.
- [ ] `parseTmuxVersion("tmux 3.3-rc")` returns `(3, 3, "3.3-rc", nil)`.
- [ ] `parseTmuxVersion("  tmux 3.0 (OpenBSD)  ")` returns `(3, 0, "3.0", nil)`.
- [ ] `parseTmuxVersion("unintelligible")` returns an error.
- [ ] `CheckTmuxVersion` returns the exact error `"Portal requires tmux ≥ 3.0 (found 2.9). Please upgrade."` when the commander output parses to `2.9`.
- [ ] `CheckTmuxVersion` returns `nil` for `tmux 3.0`, `tmux 3.3a`, `tmux 4.0`.
- [ ] `CheckTmuxVersion` wraps Commander errors and returns a non-nil error.
- [ ] `CheckTmuxVersion` returns a non-nil error on empty / unparseable output.
- [ ] All behaviour is covered by table-driven tests using `MockCommander`.

**Tests**:
- `"it parses plain semver like tmux 3.3"`
- `"it parses suffixed versions like tmux 3.3a and tmux 3.0b"`
- `"it parses pre-release versions like tmux 3.3-rc and tmux 3.0-rc1"`
- `"it parses tmux-next builds like tmux-next 4.0"`
- `"it tolerates trailing parentheticals like tmux 3.0 (OpenBSD)"`
- `"it trims leading and trailing whitespace before parsing"`
- `"it treats missing minor as .0 (tmux 3 parses as 3.0)"`
- `"it errors on unparseable output"`
- `"it accepts tmux 3.0 as satisfying the minimum"`
- `"it rejects tmux 2.9 with the specified user-facing error"`
- `"it rejects tmux 1.0 with the specified user-facing error"`
- `"it wraps the commander error when tmux -V fails"`
- `"it errors when tmux -V returns empty output"`

**Edge Cases**:
- Suffixed versions: `3.0a`, `3.3a`, `3.3b`.
- Pre-release tags: `3.3-rc`, `3.3-rc1`, `3.3-rc2`.
- Alternative binaries: `tmux-next 4.0`.
- OpenBSD-style parentheticals: `tmux 3.0 (OpenBSD)`.
- Leading/trailing whitespace in the output (existing `RealCommander.Run` trims, but the parser must not assume clean input — hook-sourced or test-sourced input may carry whitespace).
- Commander errors (missing binary, unexpected shell). `CheckTmuxVersion` wraps them and returns a clear message.
- Exactly `tmux 3.0` is ≥ 3.0 (boundary condition — inclusive minimum).

**Context**:
> Spec "Scope & Constraints → Minimum Versions → Runtime version check": "Bootstrap runs `tmux -V` once on the first `PersistentPreRunE` invocation per Portal process, parses the version string, and errors out with a clear user-facing message if the version is below 3.0 (e.g., `\"Portal requires tmux ≥ 3.0 (found 2.9). Please upgrade.\"`). The check happens **before** any `set-hook -ga` registration so that users on older tmux don't land in the 'mysteriously not working' silent-failure mode Observability explicitly argues against."
>
> The exact error-message wording in the spec is `"Portal requires tmux ≥ 3.0 (found <version>). Please upgrade."` — use it verbatim.
>
> Existing pattern: `internal/tmux/check.go` (`CheckTmuxAvailable`) is the closest existing guard and can be read for style reference; this task adds a sibling guard layered on top.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Scope & Constraints → Minimum Versions".

## built-in-session-resurrection-1-3 | approved

### Task 1-3: Wire the version guard into `PersistentPreRunE` (memoized, exempt-aware)

**Problem**: Cobra's `PersistentPreRunE` fires for every subcommand invocation — repeatedly within a single process if any subcommand also calls it manually. Running `tmux -V` every single time would waste 5–10ms per call and clutter the test seam. The spec explicitly requires the check to happen "once on the first `PersistentPreRunE` invocation per Portal process" and **before any `set-hook -ga` registration** — so the guard must short-circuit cleanly on subsequent invocations and must land in the dispatch chain before every hook-registration code path. Additionally, every command that is exempt from bootstrap (`version`, `init`, `help`, `alias`, `clean`, and every `portal state …` subcommand) must also skip the version check — running `tmux -V` on `portal version` would make the binary depend on tmux for trivial commands it does not need tmux for.

**Solution**: Extend the exempt-command allowlist to include `state` (so every `portal state …` subcommand is covered by the parent-chain walk in `PersistentPreRunE`). Introduce a package-level memoization sentinel for the version check — `versionCheckOnce` (a `sync.Once`) and a cached error — so the check runs exactly once per Portal process, with subsequent calls short-circuiting. Call `CheckTmuxVersion` before `EnsureServer()` in `PersistentPreRunE`. A failure returns early with the spec error; no further bootstrap steps run.

**Outcome**: First non-exempt subcommand invocation runs `CheckTmuxVersion`. Subsequent invocations in the same process (e.g., subtests re-entering) do not re-run it. Every exempt command (including every `portal state` subcommand) skips the check entirely. The check runs before `EnsureServer()` and — together with task 1-7's hook registration — before any `set-hook -ga` call. On failure, the error propagates as the user-facing stderr line with no `set-hook` side-effects.

**Do**:
- In `cmd/root.go`, extend `skipTmuxCheck` map to include `"state": true`. (The parent-chain walk already covers nested `portal state <sub>` invocations because every intermediate parent is checked.)
- Introduce package-level `var versionCheckOnce sync.Once` and `var versionCheckErr error` in a new file `cmd/version_guard.go`.
- Expose an injection seam `var versionChecker func(tmux.Commander) error = tmux.CheckTmuxVersion` so tests can stub it. Also expose a package-private `resetVersionCheckForTest()` that reassigns `versionCheckOnce = sync.Once{}` and clears `versionCheckErr` — test files in the same package call it from `t.Cleanup`.
- In `PersistentPreRunE`, *after* the exempt-command parent-chain walk and `CheckTmuxAvailable` call but *before* `buildBootstrapDeps()` / `EnsureServer()`, invoke:
  ```go
  versionCheckOnce.Do(func() {
      versionCheckErr = versionChecker(&tmux.RealCommander{})
  })
  if versionCheckErr != nil {
      return versionCheckErr
  }
  ```
- Tests for this task set their own `versionChecker` stub, call `resetVersionCheckForTest()`, and verify call counts. Tests live in `cmd/version_guard_test.go`.
- Do NOT attempt to merge `versionChecker` into `bootstrapDeps` — the version check runs before bootstrapping and has a different test seam.

**Acceptance Criteria**:
- [ ] `portal version` does not invoke the version checker (exempt via `version` entry in `skipTmuxCheck`).
- [ ] `portal init zsh` does not invoke the version checker (exempt via `init`).
- [ ] `portal alias list` does not invoke the version checker (exempt via `alias`).
- [ ] `portal clean` does not invoke the version checker (exempt via `clean`).
- [ ] `portal state status`, `portal state cleanup`, `portal state daemon`, `portal state notify`, `portal state signal-hydrate foo`, `portal state hydrate --fifo /tmp/f --file /tmp/s --hook-key k:0.0`, and `portal state migrate-rename a b` all do not invoke the version checker (exempt via `state`).
- [ ] `portal open` invokes the version checker exactly once per process, regardless of how many commands run within that process.
- [ ] On version checker failure, `PersistentPreRunE` returns the error before calling `buildBootstrapDeps` / `EnsureServer()` — confirmed by a test whose injected `BootstrapDeps` panics if invoked after a stubbed version failure.
- [ ] Repeated invocations of `PersistentPreRunE` (via subtests or multiple command runs) invoke the checker exactly once.

**Tests**:
- `"it invokes the version checker on the first non-exempt command"`
- `"it does not invoke the version checker for portal version"`
- `"it does not invoke the version checker for portal init"`
- `"it does not invoke the version checker for portal alias list"`
- `"it does not invoke the version checker for portal clean"`
- `"it does not invoke the version checker for any portal state subcommand"`
- `"it runs the version checker exactly once across repeated invocations in the same process"`
- `"it short-circuits bootstrap when the version checker fails"`
- `"it does not invoke EnsureServer when the version check fails"`
- `"it propagates the checker's exact error text to the caller"`

**Edge Cases**:
- Repeated invocations within a single process (subtest pattern) must not re-run the check. `sync.Once` guarantees this; the reset helper is only used in test cleanup.
- Exempt commands bypass the check entirely — no short-circuit, no call into the stub. The exempt walk is the parent-chain walk already present in `root.go`; adding `state` to the map covers every subcommand under it.
- Check failure short-circuits **before** `EnsureServer()` — must also be before any `set-hook -ga` call (task 1-7 hook registration runs later in bootstrap; this task's ordering guarantees the version guard fires first).
- Leading/trailing whitespace in the returned version is handled by the parser (task 1-2), not here.

**Context**:
> Spec "Scope & Constraints → Minimum Versions → Runtime version check": "Bootstrap runs `tmux -V` once on the first `PersistentPreRunE` invocation per Portal process". Memoization is explicit in the spec.
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence": "The **exempt commands** (skip bootstrap entirely) are: `version`, `init`, `help`, `alias`, `clean`, and all `portal state ...` subcommands — both user-facing (`portal state status`, `portal state cleanup`) and internal (`portal state daemon`, `portal state notify`, `portal state signal-hydrate`, `portal state hydrate`). The internal subcommands are invoked from hooks or as pane commands and would otherwise recursively re-bootstrap".
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors": "`tmux -V` check fails (version < 3.0 or `tmux` binary absent): Portal emits the user-facing error immediately to stderr, exits non-zero, does not enter the TUI." The single-line stderr and TUI-teardown parts land in Phase 6; this task handles the return path through `PersistentPreRunE` so Cobra's existing `SilenceErrors: true` + the root command's error return prints the message to stderr.
>
> Existing pattern: `cmd/root.go` already walks the command parent chain against `skipTmuxCheck`. The version guard piggybacks on that same check and runs after it.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scope & Constraints → Minimum Versions", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-1-4 | approved

### Task 1-4: Add `show-hooks` / `set-hook -ga` / `set-hook -gu` wrappers on `tmux.Client`

**Problem**: Later tasks (1-5 through 1-9) parse global hook arrays, append Portal's entries idempotently, and remove Portal's entries in reverse index order. They all need low-level command wrappers that go through the existing `Commander` interface so everything stays mockable. `tmux.Client` currently exposes wrappers for server options and sessions but has no hook-level primitives. Introducing them piecemeal inside higher-level code would duplicate argv construction and break the single-seam testing pattern the rest of the `tmux` package uses.

**Solution**: Add three thin methods to `tmux.Client` — `ShowGlobalHooks()` (reads all global hooks), `AppendGlobalHook(event, command string)` (appends via `set-hook -ga`), and `UnsetGlobalHookAt(event string, index int)` (removes a single indexed entry via `set-hook -gu`). Each method wraps a single tmux invocation through the `Commander` interface, returns any Commander error wrapped with enough context, and preserves raw stdout/stderr. No parsing in this task — task 1-5 owns `show-hooks` parsing.

**Outcome**: `tmux.Client` exposes three new public methods covered by unit tests that assert the exact `tmux` argv built for each method and the error-propagation behaviour (Commander failure is surfaced, not swallowed; empty stdout is returned verbatim). Higher-level code in later tasks calls these methods instead of constructing argv itself.

**Do**:
- Edit `internal/tmux/tmux.go` and add:
  - `func (c *Client) ShowGlobalHooks() (string, error)` — calls `c.cmd.Run("show-hooks", "-g")`, returns the raw output (no trimming — task 1-5's parser handles whitespace) and wrapped error.
  - `func (c *Client) AppendGlobalHook(event, command string) error` — calls `c.cmd.Run("set-hook", "-ga", event, command)`. Returns `fmt.Errorf("failed to append hook on %q: %w", event, err)` on failure; `nil` otherwise. Passes `command` as a single argv element so tmux receives it without shell re-interpretation; Go's `exec.Command` puts each arg into `argv[]` directly, so embedded single quotes inside `command` are preserved verbatim.
  - `func (c *Client) UnsetGlobalHookAt(event string, index int) error` — calls `c.cmd.Run("set-hook", "-gu", fmt.Sprintf("%s[%d]", event, index))`. Returns `fmt.Errorf("failed to unset hook %s[%d]: %w", event, index, err)` on failure; `nil` otherwise.
- Add targeted table-driven tests in `internal/tmux/tmux_test.go` (or new `internal/tmux/hooks_test.go`) using `MockCommander` that assert:
  - `ShowGlobalHooks` calls `Run("show-hooks", "-g")` exactly and returns the mock output verbatim.
  - `AppendGlobalHook("session-created", "run-shell 'x'")` calls `Run("set-hook", "-ga", "session-created", "run-shell 'x'")` — critically, the single quotes are preserved without any intermediate shell layer.
  - `UnsetGlobalHookAt("session-renamed", 2)` calls `Run("set-hook", "-gu", "session-renamed[2]")`.
  - Each method propagates a `Commander` error wrapped with the event name / index.
- Do NOT add parsing or idempotency logic in this task — they live in 1-5 and 1-6.

**Acceptance Criteria**:
- [ ] `ShowGlobalHooks` returns the raw Commander output (including whitespace/newlines) on success and wraps errors on failure.
- [ ] `AppendGlobalHook` passes the command string as a single argv element, preserving single quotes and shell metacharacters verbatim.
- [ ] `UnsetGlobalHookAt` formats the target as `<event>[<index>]` and passes it as a single argv element.
- [ ] All three methods surface the underlying Commander error via `%w` wrapping.
- [ ] Empty `show-hooks -g` output (`""`) is returned verbatim, not converted to an error or a non-nil slice.
- [ ] Unit tests cover happy path, error propagation, and the single-quote / bracket preservation cases via `MockCommander.Calls` assertions.

**Tests**:
- `"it calls show-hooks -g verbatim and returns raw output"`
- `"it returns empty string without error when show-hooks -g output is empty"`
- `"it propagates commander errors from ShowGlobalHooks"`
- `"it calls set-hook -ga <event> <command> with command preserved as a single argv element"`
- `"it preserves single quotes inside the hook command argument"`
- `"it wraps the error from AppendGlobalHook with the event name"`
- `"it formats the target as event[index] for UnsetGlobalHookAt"`
- `"it wraps the error from UnsetGlobalHookAt with event and index"`

**Edge Cases**:
- Empty `show-hooks -g` output on a fresh tmux server — returned as `""` with `nil` error. Parsing is the caller's problem (task 1-5).
- Commands containing single quotes (real hooks do — `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`). Since `Commander.Run` uses `exec.Command`, each arg is passed directly in argv; no shell intermediary, so quotes survive.
- `set-hook -gu session-renamed[0]` addressing a missing index — tmux returns an error. `UnsetGlobalHookAt` bubbles that error verbatim (task 1-8 handles the "already absent" idempotency concern).
- Commander errors must not be swallowed; callers in later tasks rely on knowing when tmux failed.

**Context**:
> Spec "tmux Hook Registration Lifecycle → Registration Shape": `set-hook -ga <event> 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'` — the command string contains single quotes and needs to pass through unmodified.
>
> Spec "tmux Hook Registration Lifecycle → Removal": "Remove each match via `set-hook -gu '<EVENT>[N]'`, in **reverse index order** (defensive — tmux does not renumber after removal, but reverse order is cheap insurance against any edge case)." The actual quoting in the argv is not required because Go's `exec.Command` puts each arg into `argv[]` directly — the target expression is just a single string `<EVENT>[N]`.
>
> Spec "tmux Hook Registration Lifecycle → Quoting note": "tmux may render the stored command with different outer quoting than Portal supplied. The match substring (`portal state notify` or `portal state signal-hydrate`) is raw text inside the command and is not affected by tmux's outer quoting." Confirms Portal does not need to manage quoting defensively — tmux normalises on its end.
>
> Existing pattern: `SetServerOption`/`GetServerOption`/`DeleteServerOption` in `internal/tmux/tmux.go` are the size and shape reference. Keep the new methods equally slim.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "tmux Hook Registration Lifecycle → Registration Shape", "tmux Hook Registration Lifecycle → Content-Based Idempotency", "tmux Hook Registration Lifecycle → Removal".

## built-in-session-resurrection-1-5 | approved

### Task 1-5: Parse `show-hooks -g` output into an indexed per-event map

**Problem**: Content-based idempotency (task 1-6) and reverse-order removal (task 1-8) both need to know, for each tmux event, which array indices currently contain Portal's command substring. `tmux show-hooks -g` produces lines like `session-created[0] => run-shell "command -v portal >/dev/null 2>&1 && portal state notify"` — array-indexed, one entry per line, with hyphenated event names and tmux's own outer quoting that Portal does not control. A correct parser must tolerate sparse indices (earlier removals leave gaps), hyphenated event names, leading whitespace, and non-indexed lines that tmux may emit. Getting this parser wrong makes the idempotency layer either miss existing entries (leading to duplicate appends) or falsely match (leading to skipped registrations).

**Solution**: Add a pure parsing function `parseShowHooks(raw string) map[string][]hookEntry` where `hookEntry` is `{Index int, Command string}`. Each line is matched against a regex that captures `<event>[<index>]` followed by a separator (`=>` with surrounding whitespace, or just whitespace — tmux versions differ) and a command body. Lines that do not match are ignored. Lines with a matching event but non-numeric index are ignored (or returned as an error, per the decision below). The result is keyed by event name with entries ordered by index ascending (preserving tmux's own ordering).

**Outcome**: `parseShowHooks(rawOutput)` returns a fully populated `map[string][]hookEntry` that task 1-6 iterates for content-substring matching and task 1-8 iterates (in reverse) for removal. The parser never panics and never returns an error for unexpected lines — it silently skips them.

**Do**:
- Create `internal/tmux/hooks_parse.go`:
  - Define `type HookEntry struct { Index int; Command string }` (exported).
  - Define `func ParseShowHooks(raw string) map[string][]HookEntry`:
    1. Split `raw` by `\n`.
    2. For each line: `strings.TrimSpace`. Skip if empty.
    3. Match the line against `^([A-Za-z][A-Za-z0-9-]*)\[(\d+)\](?:\s*=>\s*|\s+)(.*)$` — a package-level compiled `regexp.Regexp`. The separator alternation covers both `event[0] => command` and `event[0] command` shapes.
    4. On no match, skip the line.
    5. On match, parse the index via `strconv.Atoi`; on parse failure, skip.
    6. Trim surrounding quotes (if any) from the command body — but only matched pairs of outer single or double quotes. Inner content stays verbatim (so matching substrings like `portal state notify` still work regardless of tmux's outer quoting choice).
    7. Append `HookEntry{Index, Command}` to the map slot for the event.
  - After processing, sort each event's slice by `Index` ascending (tmux already emits in order, but sorting defensively guards against surprises).
- Add table-driven tests in `internal/tmux/hooks_parse_test.go` covering the cases listed below.
- Do NOT call tmux in this task — parser is pure. Real wiring happens in task 1-6.

**Acceptance Criteria**:
- [ ] Empty input returns an empty (non-nil) map.
- [ ] Input with one entry returns a map with one event → one `HookEntry`.
- [ ] Sparse indices (e.g., only `session-renamed[2]` present) return `{Index: 2, Command: ...}` with no fabricated `[0]`/`[1]` entries.
- [ ] Multiple entries on the same event produce an index-sorted slice.
- [ ] Hyphenated event names (`session-created`, `client-session-changed`, `pane-focus-out`, `window-layout-changed`) parse correctly.
- [ ] Leading and trailing whitespace on each line is tolerated.
- [ ] Lines that do not match the regex are silently skipped.
- [ ] Non-numeric bracket contents (`event[abc]`) are silently skipped.
- [ ] Both `=>` separator and bare-whitespace separator are accepted.
- [ ] Outer matched quotes on the command body are stripped; inner content is preserved verbatim (including the `portal state notify` substring).

**Tests**:
- `"it returns an empty map for empty input"`
- `"it parses a single session-created entry"`
- `"it parses multiple entries on the same event in index order"`
- `"it handles sparse indices left by prior removals"`
- `"it parses every hyphenated event name Portal registers"`
- `"it tolerates leading whitespace on each line"`
- `"it silently skips unrelated or malformed lines"`
- `"it silently skips entries with non-numeric index"`
- `"it accepts both `=>` and bare-whitespace separators"`
- `"it preserves the inner command substring across tmux outer-quoting variations"`
- `"it returns portal state notify substring intact inside a double-quoted command"`
- `"it returns portal state notify substring intact inside a single-quoted command"`

**Edge Cases**:
- Sparse indices: tmux does not renumber after `set-hook -gu 'event[1]'`; the map preserves whatever indices tmux reports.
- Hyphenated event names: all seven save-trigger events contain hyphens, as do both hydration events.
- Leading whitespace: some tmux outputs indent multi-line hook definitions — treat each line independently after trimming.
- Unrelated lines: tmux `show-hooks -g` output can contain blank lines or stray comments on some versions — silently skip.
- Non-indexed entries: tmux 2.x predates array indexing, but the version guard in task 1-3 has already rejected 2.x before this parser is called; still, the parser must not panic on unexpected formats.
- Outer quoting: tmux may emit the command wrapped in either single or double quotes. Strip matched outer quotes; otherwise the substring search in task 1-6 works without this step, but tests should confirm `portal state notify` is matchable across both variants.

**Context**:
> Spec "tmux Hook Registration Lifecycle → Content-Based Idempotency": "Run `tmux show-hooks -g` and capture stdout. Parse lines matching `^<event>\[(\d+)\]` to find array entries for this event. Look for our expected-command substring (`portal state notify` for save-trigger events, `portal state signal-hydrate` for hydration-trigger events) within those entries."
>
> Spec "tmux Hook Registration Lifecycle → Scenario 7 → Hook collision with other plugins": "tmux 3.0+ stores hooks as array-indexed options; per-index removal works cleanly. Sparse arrays fire correctly (removed indices do not break surviving entries)." Confirms sparse-index tolerance is load-bearing.
>
> Spec "tmux Hook Registration Lifecycle → Quoting note": "tmux may render the stored command with different outer quoting than Portal supplied. The match substring (`portal state notify` or `portal state signal-hydrate`) is raw text inside the command and is not affected by tmux's outer quoting."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "tmux Hook Registration Lifecycle → Content-Based Idempotency".

## built-in-session-resurrection-1-6 | approved

### Task 1-6: Implement content-based idempotent registration (`RegisterHookIfAbsent`)

**Problem**: `set-hook -ga` appends unconditionally — identical Portal entries would accumulate across bootstraps, creating duplicate `portal state notify` invocations on every event fire. The spec pins down the fix: for each (event, expected-substring) pair, read `show-hooks -g`, look for the substring in any existing array entry for that event, and append only if absent. The idempotency check must be per-event-per-substring so save-trigger events and hydration-trigger events do not cross-contaminate: the `notify` substring is only checked on save-trigger events, and the `signal-hydrate` substring is only checked on hydration-trigger events. Task 1-7 uses this function in a loop over the full event table.

**Solution**: Expose a single Go function `RegisterHookIfAbsent(c *tmux.Client, event string, expectedSubstring string, fullCommand string) error` that reads global hooks via `ShowGlobalHooks()` (task 1-4), runs the output through `ParseShowHooks` (task 1-5), inspects the entries for `event`, and appends `fullCommand` via `AppendGlobalHook` (task 1-4) only if no entry for that event contains `expectedSubstring`. Present → no-op, return `nil`. Absent → append, return `AppendGlobalHook`'s error. Show-hooks failure → bubble the error; no partial append.

**Outcome**: Calling `RegisterHookIfAbsent(client, "session-created", "portal state notify", "run-shell \"command -v portal >/dev/null 2>&1 && portal state notify\"")` twice in a row results in exactly one `set-hook -ga` invocation against tmux. Unit tests using `MockCommander.Calls` confirm the call count per scenario.

**Do**:
- Create `internal/tmux/hooks_register.go` with:
  - `func RegisterHookIfAbsent(c *Client, event, expectedSubstring, fullCommand string) error`:
    1. `raw, err := c.ShowGlobalHooks()`; on error return `fmt.Errorf("show-hooks failed: %w", err)` — do not append on show-hooks failure.
    2. `entries := ParseShowHooks(raw)`.
    3. For each entry in `entries[event]`: if `strings.Contains(entry.Command, expectedSubstring)` → return `nil` (already present).
    4. Otherwise return `c.AppendGlobalHook(event, fullCommand)`.
- Do NOT register any specific event here — task 1-7 owns the event list. This task is the primitive only.
- Tests in `internal/tmux/hooks_register_test.go` using `MockCommander` with `RunFunc` that returns tailored `show-hooks -g` outputs per scenario:
  - Append is skipped when Portal entry present (`Calls` length: 1 — just `show-hooks -g`).
  - Append happens when no entry matches (`Calls` length: 2 — `show-hooks -g` then `set-hook -ga ...`).
  - Append still happens when another event has Portal's substring but the target event does not (scoping check).
  - Unrelated user/plugin entries on the same event (e.g., `session-created[0] => run-shell 'user-script'`) are preserved — verified by asserting we still append when our substring is absent.
  - Show-hooks failure → no append, error propagated.
- The caller constructs the `fullCommand` string (task 1-7); this task treats it as an opaque string.

**Acceptance Criteria**:
- [ ] When `show-hooks -g` contains a Portal-matching entry for `event`, no `set-hook -ga` call is made.
- [ ] When `show-hooks -g` contains entries for `event` but none match `expectedSubstring`, `set-hook -ga` is called exactly once with `fullCommand`.
- [ ] When `show-hooks -g` is empty, `set-hook -ga` is called exactly once with `fullCommand`.
- [ ] A matching entry on a **different** event does not suppress registration on the target event (per-event-per-substring scoping).
- [ ] `show-hooks -g` error propagates; no `set-hook -ga` attempt follows.
- [ ] `set-hook -ga` error propagates verbatim (wrapped with the event name — that wrapping happens in `AppendGlobalHook`, not here).

**Tests**:
- `"it skips append when Portal entry already present on the target event"`
- `"it appends when target event array is empty"`
- `"it appends when target event has only non-Portal entries"`
- `"it does not skip when a matching substring lives on a different event"`
- `"it leaves unrelated user/plugin entries in place when appending"`
- `"it propagates show-hooks failure without attempting an append"`
- `"it propagates set-hook -ga failure to the caller"`
- `"it recognises a Portal entry regardless of tmux's outer quoting of the command"`

**Edge Cases**:
- Unrelated user/plugin entries on the target event are untouched — we only append, never rewrite or delete in this layer.
- A Portal entry already present is a pure no-op: `nil` return, no tmux side-effect.
- `show-hooks` failure propagates without partial append; callers in task 1-7 surface the error with the event name for observability.
- A matching substring on a sibling event (e.g., `portal state notify` on `client-attached` because of a prior buggy registration) must not suppress registration on the correct event — the scoping check is by event name.
- Tmux outer quoting drift: ParseShowHooks already strips matched outer quotes (task 1-5), but the spec also notes the substring match is invariant under tmux's quoting choices. A test case feeds single-quoted and double-quoted variants and confirms both are recognised as "already present."

**Context**:
> Spec "tmux Hook Registration Lifecycle → Content-Based Idempotency": "For each (event, expected_command) pair Portal registers: (1) Run `tmux show-hooks -g` and capture stdout. (2) Parse lines matching `^<event>\[(\d+)\]` to find array entries for this event. (3) Look for our expected-command substring (`portal state notify` for save-trigger events, `portal state signal-hydrate` for hydration-trigger events) within those entries. (4) If any matching entry is found → skip registration for this event. (5) If none match → `set-hook -ga <event> '<full command>'` to append."
>
> Spec "tmux Hook Registration Lifecycle → Content-Based Idempotency": "**Per-event-per-command scoping:** the check for `portal state notify` is only applied to save-trigger events; the check for `portal state signal-hydrate` is only applied to hydration-trigger events. The command substrings are distinct, so there is no cross-contamination across the two categories."
>
> Spec "tmux Hook Registration Lifecycle → False Paths Documented": "Assumption that `set-hook -a` is idempotent if the command matches. Empirically disproven; identical appends accumulate. Content-based check is required."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "tmux Hook Registration Lifecycle → Content-Based Idempotency".

## built-in-session-resurrection-1-7 | approved

### Task 1-7: Register the full Phase 1 hook table at bootstrap

**Problem**: With the primitives in place (version guard, client wrappers, show-hooks parser, idempotent registration), bootstrap must register Portal's complete hook table against tmux. Phase 1's acceptance enumerates nine events (seven save-trigger events routed to `portal state notify`, two hydration-trigger events routed to `portal state signal-hydrate #{session_name}`). All must be registered with the `command -v portal` defensive guard so an uninstalled binary does not produce tmux error-buffer spam. A partial prior registration (e.g., Portal previously installed only the save-trigger set, or user manually `set-hook -gu`'d one entry) must top up missing events without duplicating present ones. A failure on any single event must not silently skip subsequent events — each registration is attempted, and the aggregated result tells the caller which events failed.

**Solution**: Add `RegisterPortalHooks(c *tmux.Client) error` in a new `internal/tmux/hooks_register.go` section (or sibling file). The function iterates a package-level table of `{event, expectedSubstring, fullCommand}` tuples and calls `RegisterHookIfAbsent` for each. Errors are collected (using `errors.Join` — Go 1.20+) so all events are attempted before returning. Called from `PersistentPreRunE` after the version guard succeeds and `EnsureServer()` returns, but before `_portal-saver` session creation (which is Phase 2 scope — this task leaves the saver wiring for later). Phase 5 finalises the full ordered bootstrap; this task registers hooks as a standalone step and plumbs the call into the Phase-1 bootstrap slice.

**Outcome**: After Portal has run at least once against a given tmux server, `tmux show-hooks -g` shows exactly one Portal entry per event in the table (nine total). Re-running Portal produces zero new entries. A user who manually removed one of Portal's entries gets it re-appended on the next bootstrap. A tmux-side failure on one event does not prevent registration of the remaining eight.

**Do**:
- In `internal/tmux/hooks_register.go`, define:
  ```go
  var saveTriggerEvents = []string{
      "session-created", "session-closed", "session-renamed",
      "window-linked", "window-unlinked", "window-layout-changed",
      "pane-focus-out",
  }
  var hydrationTriggerEvents = []string{
      "client-attached", "client-session-changed",
  }
  const (
      notifyCommand        = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`
      signalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`
      notifySubstring        = "portal state notify"
      signalHydrateSubstring = "portal state signal-hydrate"
  )
  ```
- Define `func RegisterPortalHooks(c *Client) error`:
  1. Initialise `var errs []error`.
  2. For each `event` in `saveTriggerEvents`: call `RegisterHookIfAbsent(c, event, notifySubstring, notifyCommand)`. On error, wrap as `fmt.Errorf("register hook on %s: %w", event, err)` and append to `errs`.
  3. For each `event` in `hydrationTriggerEvents`: same pattern with `signalHydrateSubstring` / `signalHydrateCommand`.
  4. If `len(errs) > 0`, return `errors.Join(errs...)`; else return `nil`.
- In `cmd/root.go` `PersistentPreRunE`, after `EnsureServer()` succeeds, call `RegisterPortalHooks(client)`. On error, return it — the caller sees the aggregated failure. (Phase-6 wraps this into the single-line stderr warning; this task just propagates.)
- Do NOT create `_portal-saver` here — Phase 2 owns that session. This task only registers the hook table.
- Tests in `internal/tmux/hooks_register_test.go` using `MockCommander.RunFunc` to dispatch on `show-hooks -g` vs `set-hook -ga`:
  - Fresh server (empty show-hooks) → exactly nine `set-hook -ga` calls, one per event, in the order `saveTriggerEvents ++ hydrationTriggerEvents`.
  - Partial prior registration (show-hooks contains notify entries for five save-trigger events and both hydration events, but two save-trigger events are missing) → exactly two `set-hook -ga` calls for the missing events; the others are no-ops.
  - Fully-registered server (all nine Portal entries present) → zero `set-hook -ga` calls.
  - Per-event `set-hook -ga` failure (Commander error on, say, the third event) → remaining events are still attempted; final return is non-nil and contains both the failing event name and any other events that also failed.
  - `show-hooks -g` failure → single joined error; zero `set-hook -ga` calls. (Because every call to `RegisterHookIfAbsent` starts with `show-hooks`, a persistent show-hooks failure produces nine joined errors — that is acceptable; test documents the behaviour.)
- Tests in `cmd/root_test.go` / `cmd/bootstrap_context_test.go` verify `RegisterPortalHooks` is invoked from `PersistentPreRunE` (via an injected seam — add a `RegisterHooks func(*tmux.Client) error` field on `BootstrapDeps`).

**Acceptance Criteria**:
- [ ] Fresh bootstrap registers exactly nine Portal hook entries — seven save-trigger + two hydration-trigger, each wrapping its command in `run-shell "command -v portal >/dev/null 2>&1 && portal state <subcommand>"`.
- [ ] Idempotent re-bootstrap produces zero new `set-hook -ga` calls.
- [ ] Partial prior registration tops up only the missing events.
- [ ] Per-event failures do not short-circuit remaining events; all nine are attempted.
- [ ] The return value is an `errors.Join` aggregate that names each failed event.
- [ ] The function is invoked from `PersistentPreRunE` for every non-exempt command and runs after `EnsureServer()`.
- [ ] The two command strings use the exact wording from the spec (`run-shell "command -v portal >/dev/null 2>&1 && portal state notify"` and `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`).

**Tests**:
- `"it registers all nine Portal hooks on a fresh server"`
- `"it registers hooks in the documented order (save-trigger events then hydration-trigger events)"`
- `"it skips all appends when every Portal hook is already present"`
- `"it tops up only the missing events on a partially-registered server"`
- `"it attempts every event even if one set-hook -ga call fails"`
- `"it returns a joined error naming every failed event"`
- `"it does not double-register on two consecutive bootstraps in the same process"`
- `"it wraps each save-trigger event's command with command -v portal guard"`
- `"it wraps each hydration-trigger event's command with command -v portal guard and #{session_name}"`
- `"it is called from PersistentPreRunE after EnsureServer"`

**Edge Cases**:
- Partial prior registration: tmux has kept some entries across a user-initiated `set-hook -gu`, or Portal's previous version registered fewer events. Top-up logic only appends absent entries.
- Per-event failure: a tmux transient error on one `set-hook -ga` must not prevent the remaining eight attempts. Aggregate via `errors.Join`.
- Double-bootstrap inside one process: subcommand A triggers `PersistentPreRunE`, subcommand B runs under the same binary — the second call's `ShowGlobalHooks` sees entries from the first and registers nothing.
- The `#{session_name}` format variable must be preserved verbatim in the hydration-trigger command; tmux expands it at hook-fire time.
- Empty map versus map-with-empty-slice: `ParseShowHooks` returns a non-nil map; `entries[event]` returning an empty slice is handled by `RegisterHookIfAbsent`'s loop without panic.

**Context**:
> Spec "tmux Hook Registration Lifecycle → Global Hooks Registered by Portal": lists the exact nine events and two command strings.
>
> Spec "tmux Hook Registration Lifecycle → Registration Shape":
> ```
> set-hook -ga session-created 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
> set-hook -ga session-closed  'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
> # ... all save-trigger events identically ...
> set-hook -ga client-attached         'run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"'
> set-hook -ga client-session-changed  'run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"'
> ```
> The outer single quotes in the spec are shell quoting the user sees when running tmux from a shell. Inside Go, `exec.Command` places the argument directly into argv — so the outer single quotes are not part of the argument and must not be literally included.
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 2. Register global hooks idempotently": the step that corresponds to this task, described at the higher level where all eight steps land together.
>
> Spec "Hook System Lifecycle Behavior → Rationale": "The events: … These catch structural changes (session/window/pane topology, renames, layout changes, focus transitions) as they happen. **Deliberately NOT registered.** `window-renamed` and `pane-exited`/`pane-died` are not in the save-trigger list." Do not add to the event list beyond what is specified.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "tmux Hook Registration Lifecycle → Global Hooks Registered by Portal", "tmux Hook Registration Lifecycle → Registration Shape", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence".

## built-in-session-resurrection-1-8 | approved

### Task 1-8: Implement Portal hook removal in reverse index order

**Problem**: The `portal state cleanup` command (task 1-9) and any future uninstall-style tooling need to unwind Portal's hook registrations without touching user or plugin entries that happen to share the same event. tmux exposes per-index removal via `set-hook -gu '<EVENT>[N]'`; doing it in ascending index order is theoretically risky (tmux "does not renumber after removal, but reverse order is cheap insurance"), and doing it at all requires finding Portal's indices among a sparse array that may interleave Portal and non-Portal entries. `session-renamed` is special — it gets two Portal entries (one for `portal state notify`, one for `portal state migrate-rename` — the latter lands in Phase 4, but the removal layer must already handle the possibility of multiple Portal entries on a single event).

**Solution**: Add `UnregisterPortalHooks(c *tmux.Client) error` that reads `show-hooks -g`, parses it, iterates the Portal event table (union of save-trigger and hydration-trigger events), collects every entry whose `Command` contains either `portal state notify`, `portal state signal-hydrate`, or `portal state migrate-rename`, sorts them by index **descending**, and calls `UnsetGlobalHookAt` on each. Non-Portal entries and entries on unrelated events are left untouched. Errors on individual removals do not abort the whole operation — they accumulate into the returned `errors.Join`.

**Outcome**: `UnregisterPortalHooks(client)` on a server with nine Portal entries interleaved with user entries removes exactly the nine Portal entries, leaves every non-Portal entry in place, and re-running it produces zero additional `set-hook -gu` calls. A specially-crafted `session-renamed` event with two Portal entries (one `notify`, one `migrate-rename`) has both removed.

**Do**:
- In `internal/tmux/hooks_register.go` (or a sibling file `internal/tmux/hooks_unregister.go`), define:
  ```go
  var portalCommandSubstrings = []string{
      "portal state notify",
      "portal state signal-hydrate",
      "portal state migrate-rename",
  }
  var portalEvents = append(append([]string{}, saveTriggerEvents...), hydrationTriggerEvents...)
  ```
  (Event list here is just to scope parsing to events Portal actually registers on; substring filter is the authoritative check.)
- Define `func UnregisterPortalHooks(c *Client) error`:
  1. `raw, err := c.ShowGlobalHooks()`; on error return wrapped error — nothing to do.
  2. `entries := ParseShowHooks(raw)`.
  3. For each `event` in `portalEvents`:
     - Collect `entries[event]` whose `Command` contains any substring in `portalCommandSubstrings`.
     - Sort collected slice by `Index` **descending**.
     - For each, call `c.UnsetGlobalHookAt(event, entry.Index)`. Accumulate errors.
  4. Return `errors.Join(errs...)` (or `nil` if empty).
- Non-Portal entries are preserved implicitly — we only call `UnsetGlobalHookAt` on indices whose command matches a Portal substring.
- Already-absent state is a no-op: parsed entries is empty → loop body does nothing → zero tmux calls.
- Tests in `internal/tmux/hooks_unregister_test.go` using `MockCommander.RunFunc`:
  - Sparse interleaved array (`session-renamed[0] => user-hook`, `session-renamed[1] => portal state notify`, `session-renamed[2] => portal state migrate-rename`) → exactly two `set-hook -gu` calls: `session-renamed[2]` then `session-renamed[1]` (reverse order), no call on `[0]`.
  - Empty array → zero calls.
  - Only Portal entries (e.g., `client-attached[0] => portal state signal-hydrate …`) → one call, `client-attached[0]`.
  - `session-renamed` with two Portal entries at indices 0 and 1 → `[1]` then `[0]`.
  - Show-hooks failure → no removals, wrapped error.
  - Per-removal failure → remaining removals still attempted; aggregated error.

**Acceptance Criteria**:
- [ ] Portal entries on Phase-1 events are removed in reverse index order within each event.
- [ ] Non-Portal entries on the same events are not touched (verified by asserting the mock Commander's `Calls` contains no `set-hook -gu` for their indices).
- [ ] Entries on events Portal does not register (`window-renamed`, `pane-exited`, arbitrary user-defined events) are never removed even if their commands mention `portal`.
- [ ] Two Portal entries on the same event (the `session-renamed` double-up with `notify` + `migrate-rename`) are both removed, reverse-order.
- [ ] Already-absent state: running the function twice in a row produces zero `set-hook -gu` calls on the second run.
- [ ] `show-hooks -g` failure propagates; no `set-hook -gu` calls occur.
- [ ] Per-index removal failures are accumulated via `errors.Join`; every subsequent index is still attempted.

**Tests**:
- `"it removes a single Portal entry from an otherwise-empty array"`
- `"it removes interleaved Portal entries and leaves user entries in place"`
- `"it removes entries in reverse index order"`
- `"it removes both Portal entries on session-renamed (notify and migrate-rename)"`
- `"it is a no-op when no Portal entries are present"`
- `"it ignores matching substrings on events outside Portal's event list"`
- `"it propagates show-hooks -g failure without issuing any removal"`
- `"it attempts every removal even when one set-hook -gu call fails"`
- `"it returns a joined error naming every failed index"`

**Edge Cases**:
- Sparse arrays with Portal and non-Portal entries interleaved (`[0] user, [1] portal, [2] user, [3] portal`) → two removals at `[3]` then `[1]`, leaving user entries at `[0]` and `[2]` untouched.
- Already-absent Portal entries: empty parsed slice → zero calls. Running cleanup twice in a row: second run observes no Portal entries and does nothing.
- Two Portal entries on `session-renamed` (this is the real case once Phase 4 lands `migrate-rename`): both `portal state notify` and `portal state migrate-rename` substrings match; both entries removed, reverse order.
- `window-renamed` or `pane-exited` carrying a command that coincidentally mentions `portal` (e.g., a user's `run-shell portal list`) — the event is not in `portalEvents`, so the parser does not collect it; not removed. Scoping is by event, not by substring alone.
- Commander failure on one index — `errors.Join` captures it, the loop continues to the next index, and the final error tells the caller which failed.

**Context**:
> Spec "tmux Hook Registration Lifecycle → Removal": "Run `tmux show-hooks -g`. For each (event, expected_command) pair Portal registers: parse for Portal's indices. Remove each match via `set-hook -gu '<EVENT>[N]'`, in **reverse index order** (defensive — tmux does not renumber after removal, but reverse order is cheap insurance against any edge case). Entries matching other commands on the same events are left alone."
>
> Spec "Resume Hook Firing → Session Rename": `portal state migrate-rename` is a separate internal subcommand registered *alongside* `portal state notify` on `session-renamed`. Phase 4 adds the registration; this task's removal logic must already handle two Portal entries on that event.
>
> Spec "tmux Hook Registration Lifecycle → Scenario 4": "**`portal state cleanup`** (see CLI Surface) is the optional explicit teardown. Not required for correctness — defensive hooks and self-healing handle implicit teardown." This function is the implementation of step 2 in `portal state cleanup` (task 1-9).

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "tmux Hook Registration Lifecycle → Removal", "Resume Hook Firing → Session Rename: Hook Key Migration", "CLI Surface → `portal state cleanup`".

## built-in-session-resurrection-1-9 | approved

### Task 1-9: Implement the Phase 1 slice of `portal state cleanup`

**Problem**: The spec defines `portal state cleanup` as a three-action teardown: (1) kill `_portal-saver`, (2) remove Portal's `set-hook -ga` entries, (3) optionally purge `~/.config/portal/state/` via `--purge`. Phase 1 only ships action (2) — daemon teardown requires `_portal-saver` to exist (Phase 2), and `--purge` requires a `state/` directory that Phase 2 creates. The command still needs to exist end-to-end so users upgrading into Phase 1 have a way to unwind any hook entries Portal left behind if they decide to remove it. The remaining actions are stubbed now and filled in later phases without changing the user-facing surface.

**Solution**: Fill in the `cmd/state_cleanup.go` stub from task 1-1 with a `RunE` that invokes `UnregisterPortalHooks` (task 1-8) and reports success or aggregated failure. Daemon kill and purge remain no-ops for Phase 1 but already parse `--purge` and log "deferred to Phase 2/6" debug output (optional — can be silent). Exit code is non-zero if hook removal returns an error; zero otherwise (including the "nothing to remove" case). The command must not abort partway on failure: each action is attempted independently; only the return value reflects any errors.

**Outcome**: Running `portal state cleanup` on a tmux server with Portal hooks registered removes exactly Portal's hook entries and exits 0. Running it on a server with no Portal hooks exits 0 with zero `set-hook -gu` calls. Running it with a dead tmux server (no tmux running) exits 0 — missing tmux server is not an error (there is nothing to clean). Running it when hook removal fails partially (e.g., one of the nine `set-hook -gu` calls fails) exits non-zero and the error message names the failing events. Running it twice in a row is a clean no-op on the second call.

**Do**:
- In `cmd/state_cleanup.go`, replace the stub `RunE` with:
  1. Build a `tmux.Client` via `buildBootstrapDeps()` (or a dedicated `stateCleanupDeps` injection pattern mirroring `bootstrapDeps`/`cleanDeps` — prefer a local `stateCleanupDeps` so tests can inject without touching the bootstrap path).
  2. Check `client.ServerRunning()`. If false → no tmux server means no hooks to remove; return `nil` (exit 0). Spec: "no tmux server running is not an error."
  3. Call `tmux.UnregisterPortalHooks(client)`. Capture the error.
  4. (Phase 2) daemon kill — stub: no-op for now. Leave a `TODO(phase-2)` comment naming the planned call.
  5. (Phase 6) `--purge` handling — stub: no-op for now. Still read the flag so `--purge` parses without error. Leave a `TODO(phase-6)` comment.
  6. Return the captured hook-removal error (wrapping context, if any, to name the command that failed: `fmt.Errorf("hook removal: %w", err)`).
- Keep `stateCleanupCmd.Hidden = false` (user-facing) and `--purge` as already declared in task 1-1.
- Do NOT call `UnregisterPortalHooks` on an error-returning path for `ServerRunning` — its "no server" shape is already absorbed.
- Tests in `cmd/state_cleanup_test.go`:
  - Clean server with Portal hooks only → `UnregisterPortalHooks` called exactly once, exit 0.
  - Server with no Portal hooks → `UnregisterPortalHooks` called exactly once and returns nil, exit 0, no `set-hook -gu` calls recorded.
  - No tmux server running (ServerRunning → false) → no hook-removal call, exit 0.
  - `UnregisterPortalHooks` returns a joined error → exit non-zero, error message contains at least one of the failing event names.
  - Two consecutive invocations: second invocation is a no-op (zero `set-hook -gu` calls on the second run) and exits 0.
  - `--purge` flag parses without error in all of the above.
- This command is exempt from `PersistentPreRunE` bootstrap (task 1-3 covers `state` in the exempt map). Confirm in a test that bootstrapping is not invoked: inject a panicking `bootstrapDeps` and run cleanup without triggering the panic.

**Acceptance Criteria**:
- [ ] `portal state cleanup` removes Portal hook entries via `UnregisterPortalHooks` when tmux is running.
- [ ] `portal state cleanup` exits 0 when no tmux server is running (no error).
- [ ] `portal state cleanup` exits 0 when there are no Portal entries to remove.
- [ ] `portal state cleanup` exits non-zero when any `set-hook -gu` call fails; the error names the failing event(s).
- [ ] Running `portal state cleanup` twice in a row is a no-op the second time.
- [ ] `--purge` is accepted without error but has no effect in Phase 1 (Phase 6 wires the body).
- [ ] Command bypasses `PersistentPreRunE` bootstrap (exempt via the `state` entry in `skipTmuxCheck`; confirmed in task 1-3's test).
- [ ] Phase 1 cleanup slice does not call `kill-session -t _portal-saver` (daemon is Phase 2 scope).

**Tests**:
- `"it removes Portal hook entries when invoked with live tmux"`
- `"it exits 0 and makes zero set-hook calls when no tmux server is running"`
- `"it exits 0 when there are no Portal hook entries to remove"`
- `"it exits non-zero with a descriptive error when a removal fails"`
- `"it is a no-op when invoked a second time"`
- `"it accepts --purge without error (body deferred)"`
- `"it does not invoke PersistentPreRunE bootstrap"`
- `"it does not attempt to kill _portal-saver in Phase 1"`

**Edge Cases**:
- No tmux server running: `ServerRunning()` returns `false` → exit 0, no error, no `set-hook` calls. Spec: "no tmux server running is not an error."
- Partial failure: `UnregisterPortalHooks` returns a joined error for, say, `session-renamed[0]` and `window-linked[2]` — every other removal has already been attempted (task 1-8 guarantees this). Cleanup exit code is non-zero; the error message propagates from the joined error.
- Running twice in a row: first run removes nine entries, second run observes zero Portal entries, makes zero `set-hook -gu` calls, exits 0.
- `--purge` specified in Phase 1: flag parses, no action is taken, no error. The user is not warned about the no-op because the stub is a transparent Phase-1 slice; Phase 6 implements real purging.
- Command is exempt from bootstrap (via task 1-3). `PersistentPreRunE` skip-tmux-check chain includes `state`, so `PersistentPreRunE` returns `nil` before hitting the version / server bootstrap code.

**Context**:
> Spec "CLI Surface → `portal state cleanup`": "Actions (in order): 1. `kill-session -t _portal-saver` to terminate the daemon (SIGHUP → final flush on the way out). Idempotent: absent session is not an error. 2. Remove Portal's `set-hook -ga` entries via index-based `set-hook -gu '<EVENT>[N]'` for each event/command pair Portal registers (see tmux Hook Registration Lifecycle for the removal protocol). Already-absent entries are not an error. 3. Remove `~/.config/portal/state/` only when explicitly requested via the `--purge` flag. Default behaviour leaves the state directory intact so re-installing Portal picks up where the user left off."
>
> Spec "CLI Surface → `portal state cleanup` → Exit codes": "`0` — all requested actions completed successfully (including idempotent no-ops when nothing needed to be cleaned). non-zero — one or more actions failed (e.g., tmux `set-hook -gu` errored, `kill-session` failed for non-'session-absent' reasons, `--purge` specified but rmdir failed). Partial failures still attempt subsequent actions — `cleanup` never aborts partway to leave mixed state — but the exit code reflects that at least one action did not succeed."
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence": `portal state cleanup` is in the exempt list alongside the other `portal state ...` subcommands — cleanup tears down the machinery that bootstrap sets up, so running bootstrap first would be circular.
>
> Phase 1 acceptance: "`portal state cleanup` command exists and removes Portal's hook entries (daemon teardown and `--purge` land in Phase 6)." Phase 1 only ships action (2). Daemon kill stays stubbed until Phase 6's teardown work (per plan's Phase 6 acceptance, daemon teardown is grouped there); `--purge` also stays stubbed until Phase 6.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → `portal state cleanup`", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence".


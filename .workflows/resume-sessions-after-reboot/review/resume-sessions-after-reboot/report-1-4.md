TASK: Hooks Set Command

ACCEPTANCE CRITERIA:
- `portal hooks set --on-resume "cmd"` writes hook to `hooks.json` for the pane in `$TMUX_PANE`
- `portal hooks set --on-resume "cmd"` sets volatile marker `@portal-active-{pane_id}` with value `"1"`
- Running without `$TMUX_PANE` set produces error: "must be run from inside a tmux pane"
- Running without `--on-resume` flag produces a cobra required-flag error
- Calling set twice for the same pane overwrites the command (idempotent)
- `hooksDeps` DI struct allows test injection of the `ServerOptionSetter`
- All tests pass: `go test ./cmd -run TestHooksSet`

STATUS: Complete

SPEC CONTEXT: The spec's "CLI Surface" section defines `hooks set` as requiring `--on-resume` flag and inferring pane from `$TMUX_PANE`. It must be idempotent (overwrite on re-registration). The "Volatile Marker Mechanism" section specifies setting `@portal-active-{pane_id}` as a tmux server option with value `1` on registration. The `hooks` command group must be added to `skipTmuxCheck` so Portal's tmux bootstrap is bypassed.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/hooks.go:78-113 (hooksSetCmd), /Users/leeovery/Code/portal/cmd/hooks.go:22-30 (HooksDeps DI struct), /Users/leeovery/Code/portal/cmd/hooks.go:34-40 (requireTmuxPane), /Users/leeovery/Code/portal/cmd/hooks.go:166-177 (init with flag registration and MarkFlagRequired)
- Notes:
  - Reads `$TMUX_PANE` via `requireTmuxPane()` (line 83) -- correctly errors if empty
  - Writes hook via `store.Set(paneID, "on-resume", command)` (line 98) -- delegates to internal/hooks.Store which uses atomic writes
  - Sets volatile marker via `setter.SetServerOption(hooks.MarkerName(paneID), "1")` (line 108) -- uses centralized `MarkerName` function producing `@portal-active-{pane_id}`
  - `--on-resume` flag marked required via `MarkFlagRequired` (line 168) -- Cobra enforces this
  - `hooksDeps` DI struct (line 24-30) follows the project's package-level mutable deps pattern
  - `hooks` added to `skipTmuxCheck` in root.go (line 19) -- consistent with spec requirement
  - Idempotency handled by `Store.Set` which loads, overwrites the key, and saves

TESTS:
- Status: Adequate
- Coverage:
  - "sets hook and volatile marker for current pane" -- verifies both file write and SetServerOption call with correct args (lines 163-197)
  - "reads pane ID from TMUX_PANE environment variable" -- uses %99 to confirm env var is respected in both store and marker (lines 199-230)
  - "returns error when TMUX_PANE is not set" -- verifies error message AND no side effects (no file, no mock calls) (lines 232-261)
  - "returns error when on-resume flag is not provided" -- verifies Cobra required-flag error mentions "on-resume" (lines 263-284)
  - "overwrites existing hook for same pane idempotently" -- two consecutive sets, verifies second value wins, marker set both times (lines 286-321)
  - "writes correct JSON structure to hooks file" -- verifies exact structure: 1 pane, 1 event, correct values (lines 323-359)
  - "sets volatile marker with correct option name" -- uses %7, verifies exact option name `@portal-active-%7` and value `"1"` (lines 361-389)
- Notes: All 7 tests specified in the plan are present. Tests use the project's established DI pattern (package-level `hooksDeps` with `t.Cleanup`). No `t.Parallel()` as required by CLAUDE.md. Tests verify behavior, not implementation details. Test helpers (`readHooksJSON`, `writeHooksJSON`) use `t.Helper()` correctly.

CODE QUALITY:
- Project conventions: Followed. Uses the same DI pattern as other commands (package-level `*Deps` struct, nil check for production path). Follows `configFilePath` reuse pattern from alias/clean. Env var override for test isolation (`PORTAL_HOOKS_FILE`). `resetRootCmd` properly resets hooks flags.
- SOLID principles: Good. `ServerOptionSetter` interface is minimal (1 method). `requireTmuxPane` is a shared validation function used by both `set` and `rm`. Store operations are delegated to `internal/hooks.Store`.
- Complexity: Low. The `RunE` function is a linear sequence: validate pane, get flag, load store, set hook, set marker. No branches beyond the DI nil check.
- Modern idioms: Yes. Uses `fmt.Errorf` with `%w` for error wrapping in the store. Cobra flag handling is idiomatic.
- Readability: Good. Code is self-documenting. Comments explain DI pattern and production fallback.
- Issues: None found.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None

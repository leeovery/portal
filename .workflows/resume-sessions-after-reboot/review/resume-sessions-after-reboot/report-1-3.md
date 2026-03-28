TASK: Hooks List Command (resume-sessions-after-reboot-1-3)

ACCEPTANCE CRITERIA:
- `portal hooks list` outputs all hooks in tab-separated format: `pane_id\tevent\tcommand\n`
- Empty store produces no output and no error
- `hooks` bypasses tmux bootstrap (`skipTmuxCheck` contains `"hooks"`)
- `PORTAL_HOOKS_FILE` env var overrides the default hooks file path
- All tests pass: `go test ./cmd -run TestHooks`

STATUS: Complete

SPEC CONTEXT: The spec defines `hooks list` as showing all registered hooks across all panes with no filtering flags. Output format is one line per hook, tab-separated: pane ID, event type, command. The `hooks` command is added to `skipTmuxCheck` (like `alias`) so it bypasses Portal's tmux bootstrap. `hooks list` only reads the JSON file and does not need tmux at all.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks.go:48-76 (hooksCmd and hooksListCmd), cmd/hooks.go:117-131 (loadHookStore and hooksFilePath helpers), cmd/hooks.go:166-177 (init registration)
- The `hooksCmd` parent command at line 48 is a container with no `RunE`.
- The `hooksListCmd` at line 53 uses `cobra.NoArgs`, calls `loadHookStore()`, calls `store.List()`, and writes tab-separated output via `fmt.Fprintf(cmd.OutOrStdout(), ...)`.
- The `hooksFilePath` helper at line 129 delegates to `configFilePath("PORTAL_HOOKS_FILE", "hooks.json")`, following the same pattern as `aliasFilePath`.
- `skipTmuxCheck` in cmd/root.go:19 includes `"hooks": true`.
- The sorting is handled by `hooks.Store.List()` in internal/hooks/store.go:100-124, which sorts by pane ID then event type.
- Notes: No concerns. Implementation matches all acceptance criteria and spec requirements.

TESTS:
- Status: Adequate
- Coverage:
  - "outputs hooks in tab-separated format" -- verifies exact output format with one hook
  - "produces empty output when no hooks registered" -- verifies empty JSON produces no output
  - "produces empty output when hooks file does not exist" -- verifies missing file is handled gracefully
  - "outputs hooks sorted by pane ID" -- verifies 3-hook output is sorted by pane ID
  - "hooks bypasses tmux bootstrap" -- verifies no tmux-related error when no bootstrapDeps set
  - "accepts no arguments" -- verifies extra arguments are rejected
- All 6 required test cases from the plan are present.
- Tests use the established project DI pattern: `t.Setenv`, `resetRootCmd`, `rootCmd.SetOut(buf)`, `rootCmd.SetArgs`.
- Every test uses `t.Setenv("PORTAL_HOOKS_FILE", ...)` which exercises the env var override.
- Tests are focused and not redundant. No over-testing.
- Notes: The plan acceptance criteria also mention a test verifying `hooks list` works without tmux installed (using `t.Setenv("PATH", "/nonexistent/path")`). This specific technique is not used, but the "hooks bypasses tmux bootstrap" test effectively proves the same thing -- it runs without any tmux mock and succeeds because `skipTmuxCheck` prevents the tmux availability check. This is sufficient.

CODE QUALITY:
- Project conventions: Followed. Uses the established `*Deps` package-level DI pattern, `resetRootCmd`, `t.Setenv` for env overrides, `cmd.OutOrStdout()` for testable output, `configFilePath` for env-var-overridable config paths. No `t.Parallel()` as required by CLAUDE.md.
- SOLID principles: Good. `hooksListCmd` has a single responsibility (list hooks). `loadHookStore` and `hooksFilePath` are properly separated helpers. The `ServerOptionSetter` and `ServerOptionDeleter` interfaces follow interface segregation (small, focused). The hooks store is injected via path (not a global).
- Complexity: Low. The `RunE` for `hooksListCmd` is a straightforward load-list-print pipeline with no branching complexity.
- Modern idioms: Yes. Uses `fmt.Fprintf` with `cmd.OutOrStdout()`, `cobra.NoArgs`, standard Go error handling.
- Readability: Good. Code is self-documenting. Helper functions have clear doc comments. The file is well-organized with the list command defined before set/rm.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The `hooksListCmd` and related `hooks set`/`hooks rm` commands are all in a single `cmd/hooks.go` file (177 lines). This is appropriate for the current scope and mirrors the existing `cmd/alias.go` pattern.

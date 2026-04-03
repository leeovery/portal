AGENT: architecture
FINDINGS:
- FINDING: Residual paneID parameter names in interfaces now carrying structural keys
  SEVERITY: low
  FILES: internal/hooks/executor.go:35, internal/hooks/executor_test.go:106, internal/tmux/tmux.go:257
  DESCRIPTION: Several parameter names still say "paneID" or "livePaneIDs" when the values are now structural keys (session:window.pane). The `CleanStale(livePaneIDs []string)` interface method and `SendKeys(paneID string, command string)` both accept structural keys but advertise pane IDs in their parameter names. This is a documentation/readability concern, not a functional one, but it could mislead future contributors about what values these functions actually accept.
  RECOMMENDATION: Rename the parameters to align with the new semantics: `CleanStale(liveKeys []string)` and `SendKeys(target string, command string)`. The spec already calls for `CleanStale` param rename ("parameter renamed"). `SendKeys` is used throughout the codebase with structural key targets now, so renaming the parameter (not the function) keeps things clear.
- FINDING: Duplicate test helpers writeHooksJSON/readHooksJSON across cmd test files
  SEVERITY: low
  FILES: cmd/hooks_test.go:760-784, cmd/clean_test.go:476-500
  DESCRIPTION: `writeCleanHooksJSON` and `readCleanHooksJSON` in clean_test.go are character-for-character identical to `writeHooksJSON` and `readHooksJSON` in hooks_test.go. They were given different names to avoid compile conflicts within the same package. While test duplication is less critical than production code, this is unnecessary given they are in the same package and could share a single implementation.
  RECOMMENDATION: Consolidate into a single pair of helpers (e.g., in a shared `cmd/test_helpers_test.go` file) used by both test files. This is a minor cleanup.
SUMMARY: Architecture is sound overall. The structural key model is consistently applied across all layers (tmux, store, executor, CLI). Interface boundaries are clean and appropriately scoped. The two findings are low-severity naming/duplication issues, not structural problems.

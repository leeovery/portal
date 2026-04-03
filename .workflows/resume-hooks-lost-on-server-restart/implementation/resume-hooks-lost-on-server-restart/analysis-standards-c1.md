AGENT: standards
FINDINGS:
- FINDING: Interface and method parameter names still use pane-ID terminology instead of structural-key terminology
  SEVERITY: low
  FILES: internal/hooks/executor.go:12, internal/hooks/executor.go:36
  DESCRIPTION: The spec states that parameter names should reflect the new structural key model. The `KeySender` interface parameter is `paneID string` (executor.go:12) and the `HookCleaner` interface parameter is `livePaneIDs []string` (executor.go:36). The concrete `Store.CleanStale` at store.go:130 correctly uses `liveKeys`, and the `SendKeys` implementation in tmux.go:257 also uses `paneID`. These values now carry structural keys, so the parameter names are misleading. The spec explicitly says "parameter renamed" for CleanStale.
  RECOMMENDATION: Rename `KeySender.SendKeys(paneID string, ...)` to `SendKeys(target string, ...)` and `HookCleaner.CleanStale(livePaneIDs []string)` to `CleanStale(liveKeys []string)` to match the spec and the concrete Store implementation. Also update `tmux.go:257` parameter name from `paneID` to `target`.
SUMMARY: Implementation conforms to all functional spec requirements. The only drift is cosmetic parameter naming in two interface definitions that still use pane-ID terminology for values that now carry structural keys.

AGENT: duplication
FINDINGS:
- FINDING: Duplicate hooks JSON test helpers across cmd test files
  SEVERITY: medium
  FILES: cmd/hooks_test.go:761-784, cmd/clean_test.go:477-500
  DESCRIPTION: writeHooksJSON/readHooksJSON in hooks_test.go and writeCleanHooksJSON/readCleanHooksJSON in clean_test.go are functionally identical -- same parameters, same marshal/unmarshal logic, same error handling. They differ only in function name. Both are in package cmd and could share a single implementation.
  RECOMMENDATION: Keep writeHooksJSON and readHooksJSON in hooks_test.go (or extract to a cmd/testhelpers_test.go file). Remove writeCleanHooksJSON and readCleanHooksJSON from clean_test.go and update callers to use the shared versions.
- FINDING: Duplicated pane-to-structural-key resolution sequence in hooks set and rm commands
  SEVERITY: medium
  FILES: cmd/hooks.go:91-105, cmd/hooks.go:157-172
  DESCRIPTION: Both hooksSetCmd and hooksRmCmd contain an identical 12-line block: call requireTmuxPane, resolve keyResolver from hooksDeps with nil-check fallback to buildHooksTmuxClient, call ResolveStructuralKey, and wrap the error with the same message. This is the same logic repeated verbatim.
  RECOMMENDATION: Extract a helper function like resolveCurrentPaneKey() (string, error) that encapsulates requireTmuxPane + keyResolver resolution + ResolveStructuralKey. Both commands call this single function.
SUMMARY: Two medium-severity duplications found: identical hooks JSON test helpers across two test files in the same package, and a 12-line pane-resolution sequence copy-pasted between the hooks set and rm command handlers.

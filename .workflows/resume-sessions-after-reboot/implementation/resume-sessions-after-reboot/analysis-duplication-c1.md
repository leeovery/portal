AGENT: duplication
FINDINGS:
- FINDING: ListPanes and ListAllPanes share identical output-parsing logic
  SEVERITY: medium
  FILES: internal/tmux/tmux.go:206-226, internal/tmux/tmux.go:231-251
  DESCRIPTION: ListPanes and ListAllPanes both contain an identical 10-line block that checks for empty output, splits on newlines, trims whitespace, and filters empty lines into a string slice. The only difference between the two methods is the tmux arguments passed to c.cmd.Run and the error handling (ListPanes wraps, ListAllPanes swallows). The parsing body is copy-pasted.
  RECOMMENDATION: Extract a private helper like `parsePaneOutput(output string) []string` that handles the split/trim/filter logic. Both methods call it after their divergent Run + error handling.

- FINDING: Atomic write (temp file + rename) duplicated between hooks.Store.Save and project.Store.Save
  SEVERITY: medium
  FILES: internal/hooks/store.go:54-88, internal/project/store.go:57-87
  DESCRIPTION: Both Store.Save methods implement the same atomic write pipeline: MkdirAll on the parent directory, json.MarshalIndent, CreateTemp with a pattern, Write to temp, Close, Rename to target, with identical cleanup on each error path. The two implementations are nearly line-for-line identical (differing only in the temp file name prefix and the data being marshaled). This is a textbook extraction candidate per the Rule of Three -- it already appears twice and was independently authored.
  RECOMMENDATION: Extract an internal utility function like `atomicWriteJSON(path string, v interface{}) error` (or a non-generic `atomicWrite(path string, data []byte) error`) into a shared internal package (e.g., `internal/config` or `internal/fileutil`). Both stores call it from their Save methods.

- FINDING: Duplicate AllPaneLister interface definition
  SEVERITY: low
  FILES: cmd/clean.go:12-14, internal/hooks/executor.go:27-29
  DESCRIPTION: The `AllPaneLister` interface (single method `ListAllPanes() ([]string, error)`) is defined identically in two places: `cmd/clean.go` and `internal/hooks/executor.go`. Both were authored independently by different tasks (clean command hook cleanup vs executor cleanup). While Go's structural typing means both work, having two identical interface definitions is confusing for maintainers.
  RECOMMENDATION: Remove the duplicate from `cmd/clean.go` and import `hooks.AllPaneLister` instead. Alternatively, define it once in a shared location if cross-package dependency is undesirable (though `cmd` already imports `internal/hooks`).

- FINDING: Duplicate test helpers for hooks JSON read/write
  SEVERITY: low
  FILES: cmd/hooks_test.go:660-683, cmd/clean_test.go:470-493
  DESCRIPTION: `writeHooksJSON`/`readHooksJSON` in hooks_test.go and `writeCleanHooksJSON`/`readCleanHooksJSON` in clean_test.go are functionally identical -- same signature, same body, different names. They were independently created for each test file.
  RECOMMENDATION: Consolidate into a single pair of helpers in a shared test file (e.g., `cmd/test_helpers_test.go`). Remove the prefixed duplicates from clean_test.go.

SUMMARY: Two medium-severity findings warrant extraction: the identical pane-output parsing in tmux.go and the atomic-write-JSON pipeline duplicated across hooks and project stores. Two low-severity findings cover a duplicated interface definition and duplicated test helpers.

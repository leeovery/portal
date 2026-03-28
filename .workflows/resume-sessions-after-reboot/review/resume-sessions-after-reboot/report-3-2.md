TASK: Hook Store CleanStale Method

ACCEPTANCE CRITERIA:
- Removes entries for panes not in live set
- Returns removed pane IDs
- Returns empty slice when store is empty
- Returns empty slice when all panes are live
- Only saves when at least one entry removed
- All tests pass: `go test ./internal/hooks/...`

STATUS: Complete

SPEC CONTEXT: The "Stale Registration Cleanup" section specifies lazy cleanup on read: "When Portal reads hooks (during portal open), cross-reference pane IDs against live tmux panes. Prune entries for panes that don't exist. Invisible to the user." This task implements the store-level building block for that cleanup. The method accepts live pane IDs as input to keep the store decoupled from tmux, matching the `project.Store.CleanStale()` pattern.

IMPLEMENTATION:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/hooks/store.go:127-159`
- Notes: Method signature matches plan exactly: `CleanStale(livePaneIDs []string) ([]string, error)`. Implementation builds a `map[string]struct{}` set (more idiomatic than plan's `map[string]bool`), partitions into `kept` map and `removed` slice, saves only when removals occur. Uses existing `Load()` and `Save()` methods. Error wrapping follows project convention with `fmt.Errorf("...: %w", err)`.

TESTS:
- Status: Adequate
- Coverage:
  - "removes entries for panes not in live set" -- verifies removal + kept entries + file state
  - "returns empty slice when store is empty" -- edge case, no file exists
  - "returns empty slice when all panes are live" -- no removals needed
  - "removes all entries when live set is empty" -- all entries stale
  - "only saves file when entries were removed" -- mod time comparison to verify no unnecessary write
  - "handles mix of live and stale panes" -- 4 panes, 2 live, 2 stale; sorts removed for deterministic assertion
- Notes: The plan listed 9 test names but several were overlapping behaviors. The 6 implemented tests cover all distinct behaviors without redundancy. Tests verify both return values and persisted file state. The mod-time test for save-only-when-needed is a solid approach. Tests correctly use `sort.Strings(removed)` to handle non-deterministic map iteration order.

CODE QUALITY:
- Project conventions: Followed. Uses atomic write pattern, error wrapping with `%w`, `_test` package for external testing, temp dirs for test isolation.
- SOLID principles: Good. Single responsibility. Decoupled from tmux via parameter injection (accepts live pane IDs rather than querying tmux).
- Complexity: Low. Linear flow: load, build set, partition, conditionally save.
- Modern idioms: Yes. `map[string]struct{}` for set membership is idiomatic Go.
- Readability: Good. Clear variable names (`live`, `kept`, `removed`). Method doc comment explains behavior and save optimization.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `removed` variable is a nil slice when nothing is removed (vs `[]string{}`). This is functionally equivalent in Go for `len()`, `range`, JSON, etc., and the tests correctly use `len(removed) != 0` rather than `removed != nil`. No change needed, but worth noting the nil-vs-empty distinction is handled correctly.

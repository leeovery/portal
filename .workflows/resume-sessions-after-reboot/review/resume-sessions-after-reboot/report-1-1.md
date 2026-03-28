TASK: Hook Store

ACCEPTANCE CRITERIA:
- `internal/hooks/store.go` exists with `Store` struct and all methods: `NewStore`, `Load`, `Save`, `Set`, `Remove`, `List`, `Get`
- `Load` returns an empty map when the file does not exist (no error)
- `Load` returns an empty map when the file contains malformed JSON (no error)
- `Save` creates the parent directory if it does not exist
- `Save` uses atomic write (temp file + rename pattern)
- `Set` is idempotent -- calling it twice for the same pane/event overwrites the command
- `Remove` is a no-op when the pane/event does not exist (no error returned)
- `Remove` cleans up the outer map key when the inner map becomes empty
- `List` returns hooks sorted by pane ID, then event type
- `Get` returns an empty map for an unregistered pane
- All tests pass: `go test ./internal/hooks/...`

STATUS: Complete

SPEC CONTEXT: The specification defines a per-pane flat registry stored as JSON at `~/.config/portal/hooks.json`, modeled as `pane_id -> event_type -> command`. The store must follow the existing atomic write pattern from `project/store.go`. Hooks are structured data (pane x event type) supporting multiple events per pane.

IMPLEMENTATION:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/hooks/store.go` (lines 1-176)
- Notes:
  - All required methods present: `NewStore` (line 30), `Load` (line 36), `Save` (line 55), `Set` (line 66), `Remove` (line 83), `List` (line 99), `Get` (line 163)
  - `Hook` struct (line 15) provides clean list output model with PaneID, Event, Command fields
  - `hooksFile` type alias (line 22) cleanly models the `map[paneID]map[event]command` structure
  - `Load` correctly returns empty map for missing file (line 39-41, `errors.Is(err, os.ErrNotExist)`) and malformed JSON (line 46-48)
  - `Save` delegates to `fileutil.AtomicWrite` (line 62) which handles parent directory creation and temp-file+rename
  - `Set` creates inner map when pane is new (line 72-73), overwrites existing entry (line 75)
  - `Remove` checks pane existence, deletes event, cleans outer key when inner map empty (lines 89-96)
  - `List` sorts by PaneID then Event using `sort.Slice` (lines 117-122)
  - `Get` returns empty `map[string]string{}` for unregistered pane (line 171)
  - `CleanStale` also present (line 130) -- belongs to Phase 3 task 3-2, but co-located in the same file which is the correct placement

TESTS:
- Status: Adequate
- Coverage: All 17 specified tests exist in `/Users/leeovery/Code/portal/internal/hooks/store_test.go`:
  - TestLoad: "returns empty map when file does not exist" (line 13), "returns empty map when file contains malformed JSON" (line 27), "returns hooks from valid JSON file" (line 46)
  - TestSave: "creates parent directory if missing" (line 75), "writes valid JSON that can be loaded back" (line 98), "uses atomic write (file exists after save even if interrupted)" (line 128)
  - TestSet: "adds a new hook for a new pane" (line 165), "adds a second event to an existing pane" (line 187), "overwrites existing entry for same pane and event" (line 218)
  - TestRemove: "deletes a hook entry" (line 248), "cleans up outer key when inner map is empty" (line 280), "is a no-op when pane does not exist" (line 306), "is a no-op when event does not exist for pane" (line 329)
  - TestList: "returns empty slice when no hooks" (line 357), "returns hooks sorted by pane ID then event" (line 371)
  - TestGet: "returns event map for registered pane" (line 411), "returns empty map for unregistered pane" (line 433)
  - TestCleanStale: 6 additional tests (line 449-640) covering stale cleanup -- belongs to task 3-2 but appropriately co-located
- Notes:
  - Tests verify behavior, not implementation details
  - Edge cases from spec are well covered (missing file, malformed JSON, idempotent set, no-op remove, outer key cleanup)
  - The atomic write test (line 128) verifies no temp files remain and file is valid -- reasonable given the actual atomic mechanism is tested separately in `internal/fileutil/atomic_test.go`
  - No over-testing detected -- each test covers a distinct scenario
  - Tests do not use `t.Parallel()`, consistent with project convention in CLAUDE.md

CODE QUALITY:
- Project conventions: Followed. Mirrors `project/store.go` pattern exactly (Store struct, NewStore constructor, Load/Save with atomic write, same error handling pattern). Uses `fileutil.AtomicWrite` shared utility rather than duplicating the pattern. Package-level doc comment present.
- SOLID principles: Good. Store has single responsibility (persistence). Clean separation via `fileutil.AtomicWrite`. Hook struct separates display concerns from storage.
- Complexity: Low. All methods are straightforward with minimal branching. No nested complexity.
- Modern idioms: Yes. Uses `errors.Is` for error checking, type alias for readability, `json.MarshalIndent` for human-readable output.
- Readability: Good. Clear method names, doc comments on all exported symbols, consistent formatting. The `hooksFile` type alias makes the map-of-maps type self-documenting.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None

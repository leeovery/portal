TASK: Update Hook Struct and Store Semantics for Structural Keys

ACCEPTANCE CRITERIA:
- Hook struct field is Key string (not PaneID)
- CleanStale parameter is named liveKeys
- Set, Remove, Get parameters are named key
- All doc comments use key / structural key terminology
- All store tests use structural key format values
- No behavioral changes
- go test ./internal/hooks/... passes (store tests only; executor tests are Task 4)

STATUS: Complete

SPEC CONTEXT: The specification requires the Hook struct's PaneID field to become a structural key field, List() to populate it with structural keys, CleanStale to accept liveKeys parameter. The store treats keys as opaque strings -- the semantic meaning changes from pane IDs to structural keys but the logic is identical. Old pane-ID-keyed entries are automatically cleaned by CleanStale on first run with live panes after upgrading.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/hooks/store.go
- Notes:
  - Hook struct field renamed to Key (line 16) -- PASS
  - CleanStale parameter is liveKeys (line 130) -- PASS
  - Set parameter is key (line 66) -- PASS
  - Remove parameter is key (line 83) -- PASS
  - Get parameter is key (line 163) -- PASS
  - Package comment updated to "keyed by structural keys" (line 1) -- PASS
  - hooksFile type comment updated to "map[structural_key]map[event]command" (line 21) -- PASS
  - All method doc comments use "key" terminology consistently -- PASS
  - List() uses Key field in struct literal (line 110) and sort comparisons (lines 118-119) -- PASS
  - CleanStale loop variable is key (line 144) -- PASS
  - No behavioral changes -- logic is identical to pre-rename, only names changed -- PASS
  - Consumer cmd/hooks.go already updated to reference h.Key (line 101) -- PASS

TESTS:
- Status: Adequate
- Coverage:
  - TestLoad: 3 tests -- file not found, malformed JSON, valid JSON with structural keys -- PASS
  - TestSave: 3 tests -- parent dir creation, roundtrip with structural keys, atomic write -- PASS
  - TestSet: 3 tests -- new key, second event for existing key, overwrite same key/event -- PASS
  - TestRemove: 4 tests -- delete entry, cleanup empty outer key, no-op missing key, no-op missing event -- PASS
  - TestList: 2 tests -- empty, sorted by key then event (renamed from "sorted by pane ID") -- PASS
  - TestGet: 2 tests -- registered key, unregistered key with structural format "nonexistent:9.9" -- PASS
  - TestCleanStale: 7 tests -- stale removal, empty store, all live, all stale, save-only-when-changed, old pane-ID upgrade migration, mixed live/stale across sessions -- PASS
  - Test "returns hooks sorted by key then event" uses wantKeys variable with structural key values and asserts on .Key field -- PASS
  - Test "old pane-ID entries cleaned on first run after upgrade" correctly uses old %0/%3 values to verify migration -- appropriate use of old format
  - All structural key format values follow session:window.pane pattern (e.g., "my-session:0.0", "other-session:0.1") -- PASS
- Notes: Tests are well-balanced. No over-testing. Each test verifies distinct behavior. The upgrade migration test is a valuable addition beyond the basic rename scope.

CODE QUALITY:
- Project conventions: Followed -- standard Go patterns, small interfaces, no t.Parallel() (per CLAUDE.md)
- SOLID principles: Good -- Store has single responsibility, clean API surface
- Complexity: Low -- pure rename with no logic changes
- Modern idioms: Yes -- proper error wrapping with %w, sort.Slice, map initialization patterns
- Readability: Good -- consistent "key" terminology throughout, self-documenting parameter names
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None

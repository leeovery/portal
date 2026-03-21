TASK: Consolidate Duplicate Test Mock Types Across Cmd Test Files

ACCEPTANCE CRITERIA:
- mockConnector type no longer exists in open_test.go
- stubSessionLister type no longer exists in open_test.go
- Only one mockSessionConnector definition exists in the cmd package (in attach_test.go)
- Only one mockSessionLister definition exists in the cmd package (in list_test.go)
- All tests pass: go test ./cmd/...

STATUS: Complete

SPEC CONTEXT: This is a pure refactoring task (Phase 5 cleanup) unrelated to feature behavior. The specification describes auto-start tmux server bootstrap; this task removes duplicate test mock types that accumulated during earlier implementation phases.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/attach_test.go:11 -- canonical mockSessionConnector definition
  - cmd/attach_test.go:22 -- mockSessionValidator definition (unchanged)
  - cmd/list_test.go:13 -- canonical mockSessionLister definition
  - cmd/open_test.go -- no mockConnector or stubSessionLister types present
- Notes: All acceptance criteria verified:
  - No `type mockConnector` found anywhere in cmd/
  - No `type stubSessionLister` found anywhere in cmd/
  - Exactly one `type mockSessionConnector` definition (cmd/attach_test.go:11)
  - Exactly one `type mockSessionLister` definition (cmd/list_test.go:13)
  - Both types are referenced from open_test.go and root_test.go, which works correctly since all test files share the same `cmd` package

TESTS:
- Status: Adequate
- Coverage: This is a pure deduplication refactor. Existing tests in open_test.go (lines 950-988) use mockSessionConnector and mockSessionLister from their canonical locations. Tests in root_test.go (line 178) also reference mockSessionLister. All cross-file references compile and function correctly since Go test files in the same package share a single compilation unit.
- Notes: No new tests needed for a deduplication refactor. The fact that tests compile and pass confirms the consolidation is correct.

CODE QUALITY:
- Project conventions: Followed -- mock types follow existing naming patterns (mock* for mocks, stub* for stubs)
- SOLID principles: Good -- DRY violation eliminated by consolidating duplicate types
- Complexity: Low -- straightforward type removal
- Modern idioms: Yes
- Readability: Good -- each mock type is defined once with a clear doc comment explaining its purpose
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None

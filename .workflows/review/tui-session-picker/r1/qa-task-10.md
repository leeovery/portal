TASK: Project List Item and Custom ItemDelegate (tick-df51d2)

ACCEPTANCE CRITERIA:
- ProjectItem implements list.Item interface (compiler check)
- FilterValue() returns the project name
- ProjectDelegate implements list.ItemDelegate interface (compiler check)
- Render output contains project name and path
- Long project paths render without truncation
- Projects with identical names both render (differentiated by path)
- projectsToListItems correctly converts []project.Project to []list.Item

STATUS: Complete

SPEC CONTEXT: The Projects page uses a bubbles/list with a custom ItemDelegate showing project name and path. Each project item is rendered via a custom ItemDelegate. The plan task mirrors the Phase 1 pattern established by SessionItem/SessionDelegate.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/project_item.go:1-83
- Notes:
  - ProjectItem wraps project.Project, implements list.Item via FilterValue() (line 25-27), Title() (line 30-32), Description() (line 35-37)
  - ProjectDelegate implements list.ItemDelegate with Height()=2, Spacing()=0, Update() returning nil, and Render() (lines 44-73)
  - Render correctly formats: cursor indicator for selected item, bold name on line 1, dimmed path on line 2
  - ProjectsToListItems helper at lines 77-83 converts []project.Project to []list.Item
  - Uses cursorStyle defined in session_item.go (shared across package) -- appropriate reuse
  - Follows the same structural pattern as SessionItem/SessionDelegate from Phase 1
  - Plan task specified lowercase `projectsToListItems` but implementation is exported `ProjectsToListItems` -- this is correct since it is called from outside the package (or at minimum consistent with `ToListItems` for sessions). Minor naming drift but appropriate for Go conventions.

TESTS:
- Status: Adequate
- Coverage:
  - TestProjectItem: interface compiler check, FilterValue, Title, Description (lines 13-47)
  - TestProjectDelegate: interface compiler check, Height, Spacing, Update, render name+path, highlights selected, long path no truncation, identical names with different paths (lines 49-163)
  - TestProjectsToListItems: converts projects, empty slice, nil slice (lines 166-209)
  - All 8 planned tests are present plus 5 additional structural/edge-case tests
  - Edge cases from task covered: long paths (line 124-139), identical names (line 141-163)
- Notes: Tests are well-structured, use table-driven subtests under parent groups. Not over-tested -- each test verifies a distinct behavior. The empty/nil slice tests for ProjectsToListItems are good defensive checks.

CODE QUALITY:
- Project conventions: Followed. Matches the pattern established by session_item.go. External package test (`tui_test`). Exported doc comments on all public types and methods.
- SOLID principles: Good. Single responsibility -- ProjectItem is a data adapter, ProjectDelegate handles rendering. Clean interface implementation.
- Complexity: Low. All methods are simple and linear.
- Modern idioms: Yes. Uses lipgloss styles correctly. Idiomatic Go struct embedding pattern.
- Readability: Good. Clear, self-documenting code. Comments on all exported symbols. Render method logic is straightforward.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The plan specified lowercase `projectsToListItems` but implementation uses exported `ProjectsToListItems`. This is the correct Go convention since it needs to be accessible from other packages, and is consistent with `ToListItems` in session_item.go. Not an issue, just a naming discrepancy with the plan text.
- The projectPathStyle uses color "241" which matches detailStyle in session_item.go -- consistent styling across delegates.

TASK: session-tagging-and-grouping-2-2 — By Project grouping builder (session → dir key → project name heading, pre-sorted)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: two distinct dirs sharing project name form two groups (key is canonical path not name); Session.Dir empty → Unknown; stamped dir no matching record → Unknown; zero live sessions → empty; one item per session under name heading; pre-sorted by group key then name; catch-all pinned last + empty-suppression; never dropped.

SPEC CONTEXT: By Project = Pattern A (each session once under project name heading); grouping key is canonical path (two dirs/same name → two groups, visual repeat accepted); Unknown bucket covers no-dir AND deleted-project; within-group alphabetical, catch-all pinned last, empty suppressed.

IMPLEMENTATION: Implemented (final refactored state).
- grouping.go:45-69 buildByProject; shared tail assembleGroups (:229-238), appendCatchAll (:185-203), sessionItemsToList (:209-215), unknownItem (:160-166). Canonical key reused from idx.Match (8-1, single EvalSymlinks). Empty Dir → Unknown before Match; !ok → Unknown. 8-2 typed []SessionItem (no runtime assertion); 5-4 shared assembly. Sort by GroupKey keeps same-key items contiguous. Pure function.

TESTS: Adequate. grouping_test.go:33-358 TestBuildByProject — one-item-per-session; GroupKey reuses idx.Match key (guards 8-1); ordering; two dirs same name → two groups; empty Dir / no-record → Unknown; pinned-last; zero sessions; empty-suppression; never-drops. Real on-disk tempdirs for EvalSymlinks. Not over-tested.

CODE QUALITY: Conventions followed (pure helper, no logging in render layer, no t.Parallel); SOLID good (5-4 DRY tail, no premature abstraction); low complexity; slices.SortFunc/cmp.Compare idiomatic; nil-safe. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None. (sort-by-path vs sort-by-name is spec-consistent and intentional.)

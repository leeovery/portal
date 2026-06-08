TASK: session-tagging-and-grouping-5-4 — Extract shared grouping-assembly skeleton from buildByProject/buildByTag

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: pure refactor (byte-identical output); all three boxing sites route through sessionItemsToList; sort + appendCatchAll defined in one place.

SPEC CONTEXT: Analysis-cycle cleanup. Feature is session grouping; refactor must not disturb invariants: never-dropped, Pattern A/B, catch-all pinned last, empty-suppression, (GroupKey, Name) sort.

IMPLEMENTATION: Implemented.
- grouping.go:229-238 assembleGroups (sorts resolved once, boxes via sessionItemsToList, delegates appendCatchAll); :209-215 sessionItemsToList (single boxing source of truth, nil-safe); :185-203 appendCatchAll (pin+suppress+name-sort+heading-stamp, once); buildByProject:68 and buildByTag:116 both end with single assembleGroups call. Both SessionItem→list.Item conversions route through sessionItemsToList (make at :199 is merge-buffer alloc, not boxing). Remaining .(SessionItem) are read-side unboxing. Per-session classification loops unchanged.

TESTS: Adequate. grouping_assembly_test.go — TestSessionItemsToList (nil→non-nil-empty, order+identity); TestAssembleGroups (empty→empty, sort key-then-name, empty-suppression, pinned-last+stamp+name-order). Pre-existing grouping_test.go (~34 build calls) is byte-identical-output guardrail through new path. Behaviour-focused.

CODE QUALITY: Conventions followed (pure functions, no t.Parallel, cmp.Compare/slices.SortFunc); SOLID good (single-responsibility helpers, DRY with ≥2 callers, no premature abstraction); low complexity; documented. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.

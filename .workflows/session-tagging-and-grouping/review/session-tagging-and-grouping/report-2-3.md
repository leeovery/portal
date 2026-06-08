TASK: session-tagging-and-grouping-2-3 — By Tag grouping builder (one item per (session, tag), pre-sorted)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: one item per (session,tag) under each tag heading (Pattern B); multi-tag → N items (sum can exceed live count); work/Work/WORK collapse to one canonical heading; zero-tag → one Untagged item; no-record → one Untagged item; never dropped; pre-sorted by (tag, name); Untagged pinned last.

SPEC CONTEXT: spec §123-141, §256 Pattern B; headings show canonical lower-cased tag; project miss → Untagged; header-count sums exceed live count.

IMPLEMENTATION: Implemented (final state, refactors 5-4/6-2/6-3/8-1/8-2).
- grouping.go:97-117 buildByTag; :125-142 resolveSessionTags (re-runs stored tags through NormaliseTag — defensive collapse); :148-154 untaggedItem; shared tail :185-238. Each usable tag emits SessionItem with GroupKey==GroupHeading==canonical tag; shared Session by value. Zero-tag/no-record/empty-Dir → single untaggedItem. Sort+pin via assembleGroups/appendCatchAll. Zero .Tag references (6-3).

TESTS: Adequate. grouping_test.go:360-748 TestBuildByTag — all 4 edge cases plus empty-Dir, junk-tag-skipped, all-junk, ordering, pinned-last, shared-session identity, sum-exceeds-count, zero sessions, empty-suppression, deleted-project, never-drops. Real tempdirs. Mild justified overlap with shared tail.

CODE QUALITY: Conventions followed (pure, no I/O, no t.Parallel); SOLID good (resolveSessionTags extraction); DRY (shared assembleGroups); low complexity; slices.SortFunc idiomatic; excellent doc comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.

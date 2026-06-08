TASK: session-tagging-and-grouping-7-1 — Gate lazy stamp-on-render resolution to grouped modes only

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: Flat (default) + byTagSignpost perform zero pane reads / git rev-parse / stamp writes; ModeByProject/ModeByTag still resolve (counter==N); byTagSignpost precedes ModeByTag so zero-tags By-Tag skips resolution; grouped output byte-identical; m.sessions never mutated.

SPEC CONTEXT: spec §93-95 fallback strictly grouped-render concern. Flat and signpost render via ToListItems (ignores Session.Dir) so resolution there is pure waste. Analysis Cycle 3 removes that waste; fast path unaffected.

IMPLEMENTATION: Implemented.
- model.go:1035-1076 rebuildSessionList — gate structural: resolveSessionDirs called only in ModeByProject (1052) and ModeByTag (1054) arms; byTagSignpost arm (1049-1050) and default/Flat (1055-1056) call ToListItems with un-resolved sessions. byTagSignpost computed :1045, first switch case (precedes ModeByTag). :1020-1033 resolveSessionDirs sole chokepoint over ResolveAndStampDir; value-copy so m.sessions never mutated; nil-seam guard.

TESTS: Adequate. rebuild_dir_resolution_gate_test.go — Flat zero reads+writes (3 sessions); byTagSignpost set + zero reads+writes; ModeByProject reads==N; ModeByTag !signpost + reads==N. fakeStamper reads/setCalls precise oracle. Byte-identical + no-mutation covered in sibling rebuild_dir_resolution_test.go (no duplication). Behaviour-anchored on seam calls.

CODE QUALITY: Conventions followed (seam DI, value-copy, no t.Parallel); SOLID good (single chokepoint, gate in one switch); low complexity (no added branching); idiomatic; exemplary only-grouped-arm doc comment. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.

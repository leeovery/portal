TASK: session-tagging-and-grouping-9-1 — Extract renderEditListField helper to collapse Aliases/Tags render duplication

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: helper called exactly twice (Aliases, Tags); inline blocks removed; byte-identical output across unfocused/focused/empty(none)/populated-cursor-on-entry/cursor-on-Add-row for both fields; no new Model fields; blank-line separators kept at call sites; build + tui tests pass.

SPEC CONTEXT: Phase 9 (Cycle 5) low-severity duplication cleanup. Aliases/Tags meant to render identically (no sanctioned divergence); line-for-line copy risks drift. No behaviour change — byte-identical preservation.

IMPLEMENTATION: Implemented (clean pure refactor).
- model.go:2271-2295 renderEditListField(b, label, focused, entries, cursor, addInput) — all per-field state as plain args, no new Model fields/types; called exactly twice at :2251 (Aliases) and :2254 (Tags). Blank-line separators retained at :2250/2253. Four-part structure (focus indicator, (none) empty, [x] entries with `  > `/`    ` marker, Add row) reproduces original. Cursor markers gated on focused&&. editError footer + keybinding stay in caller (helper single-responsibility). Grep confirms exactly 2 call sites.

TESTS: Adequate. edit_modal_render_byte_exact_test.go TestRenderEditProjectContent_ByteExact (8 cases) — all named states for both fields incl. unfocused-populated (incidental), error footer. Full-modal byte-exact captures — correct oracle for pure refactor. Cursor-gating-on-focus invariant exercised. No over/under-testing.

CODE QUALITY: Conventions followed (strings.Builder+Fprintf, no t.Parallel, doc comment); SOLID good (single responsibility, args not Model coupling); low complexity; *strings.Builder by pointer idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.

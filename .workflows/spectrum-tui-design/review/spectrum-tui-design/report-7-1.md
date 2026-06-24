TASK: spectrum-tui-design-7-1 — Projects List Must Drop Vim/Uppercase/Page-Jump Keys (§12.2 Arrow-Only Nav) [BUG-FIX, severity: high]

ACCEPTANCE CRITERIA (from tick-e8510b):
- newProjectList calls pinArrowOnlyNav on its list KeyMap, identically to newSessionList.
- On the Projects page, j, k, h, l, g, G, pgup, pgdown, home, end, b, u, f do not move the cursor or change the page.
- ↑/↓ move the cursor and Ctrl+↑/↓ page on the Projects page.
- No uppercase binding (G) navigates on the Projects page.
- Tests: a dispatch-layer (not descriptor-layer) no-op test for the banned keys driving the live bubbles/list, plus an arrow-key positive test.

STATUS: Complete

SPEC CONTEXT:
§12.1 lists the identical nav revision for BOTH Sessions and Projects (↑/↓ move · Ctrl+↑/↓ page). §12.2 mandates "Navigation is arrows only" — drop all vim aliases (h/j/k/l, g/G) and PgUp/PgDn/Home/End — and "No uppercase bindings anywhere." The defect: the Sessions list enforced this via newSessionList→pinArrowOnlyNav, but newProjectList omitted the call, so the Projects list retained the bubbles/list v2 DefaultKeyMap (CursorUp=[up,k], CursorDown=[down,j], PrevPage=[left,h,pgup,b,u], NextPage=[right,l,pgdown,f,d], GoToStart=[home,g], GoToEnd=[end,G]). updateProjectsPage intercepts only ?, esc, q, x, n, d, e, enter, so every un-handled nav key fell through to the list and moved the cursor/page — genuine behaviour drift forbidden by §12.2, with uppercase G additionally violating the no-uppercase rule.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1034 — newProjectList now calls pinArrowOnlyNav(&l.KeyMap), placed identically to newSessionList (model.go:996), as the line just before InfiniteScrolling=true. This is the ONLY behavioural diff between the two constructors, which is exactly the fix the task scoped.
  - pinArrowOnlyNav (model.go:1011-1018) is the shared helper, unchanged: CursorUp→"up", CursorDown→"down", PrevPage→"ctrl+up", NextPage→"ctrl+down", GoToStart/GoToEnd emptied. It mutates the KeyMap in place exactly as the Sessions sibling expects.
  - updateProjectsPage (model.go:2249-2310) is UNTOUCHED: it intercepts Ctrl+C, ?, Esc, q, x, n, d, e, Enter (and respects SettingFilter), then falls through to m.projectList.Update(msg) at line 2308. The banned nav keys are not intercepted — they reach the list's Update, where the rebound KeyMap now makes them inert. Fix is purely the KeyMap rebind, not a dispatch-layer block, matching the task's "do not alter the interception list" constraint.
- Notes: Correct architectural placement. Rebinding at the KeyMap (the single place the list itself consults) rather than the dispatch layer is the right call — it keeps the §12.2 rule in one location and means a banned key never reaches the list's own Update. The fix genuinely closes the defect: with the v2 DefaultKeyMap's k=CursorUp / j=CursorDown / g,G,Home,End / h,l,pgup,pgdown,b,u,f now all stripped or rebound, no banned key can move the Projects cursor or page.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/projects_keymap_dispatch_test.go:153-183 — "it does not navigate via vim/uppercase/page-jump aliases on Projects": drives the LIVE dispatch (pressProject → m.updateProjectsPage → m.projectList.Update) for the full banned set {j,k,h,l,g,G,b,u,f,PgUp,PgDn,Home,End}, asserting projectList.Index() is unchanged, page stays PageProjects, and no modal opens. This is dispatch-layer, not descriptor-layer, as the task requires.
  - projects_keymap_dispatch_test.go:185-213 — positive arrow test: ↓ moves index 0→1, ↑ moves back, and asserts NextPage.Keys()==["ctrl+down"] / PrevPage.Keys()==["ctrl+up"] (both non-empty and exact). Confirms ↑/↓ + Ctrl+↑/↓ preserved.
  - projects_keymap_dispatch_test.go:133-144 — uppercase S/X page-toggle no-op test (covers the no-uppercase rule at the dispatch level alongside G in the banned-nav set).
  - The fixture projectsNavModel (lines 71-88) is correctly designed to catch a regression: it seeds 4 rows and calls applyProjectListSize(contentWidth(), contentHeight()). termDims() falls back to 24 rows (model.go:3324 fallbackTermHeight=24) when termHeight is unset, so the list has real navigable height and the cursor at index 0 CAN move to a non-zero index. A single-row InfiniteScrolling list would have pinned the cursor at 0 and masked a leaked binding — the fixture comment (lines 67-70) explicitly documents avoiding that trap. The test would therefore genuinely fail if pinArrowOnlyNav were removed (j/k/g/G/etc. would move the cursor off index 0).
  - The new dispatch test precisely mirrors the Sessions sibling (sessions_keymap_dispatch_test.go:122-160), satisfying the "same coverage the Sessions list has" requirement.
  - The pre-existing descriptor-layer test (projects_keymap_test.go:134 "it has no uppercase or vim-alias key in the descriptor") is retained and correctly scoped to the display layer; the new dispatch test complements it rather than replacing it.
- Notes: Not over-tested — each assertion targets a distinct invariant (no-move, page-unchanged, no-modal, arrow-positive, exact page-binding keys). Not under-tested — the full §12.2 banned set is enumerated, including the uppercase G and all page-jump keys. Edge case (regression-catching fixture height) is explicitly handled.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (cmd/tui mock-injection convention). Table-driven banned-key loop is idiomatic Go. Stubs are minimal dispatch-routing fakes with documented intent.
- SOLID principles: Good. The shared pinArrowOnlyNav helper is the single source of truth for the §12.2 rebind, consumed by both constructors (DRY) — the fix is one line precisely because the helper already existed.
- Complexity: Low. One-line change in the constructor; no new branches.
- Modern idioms: Yes.
- Readability: Good. The helper's doc comment (model.go:1001-1010) explains why the rebind lives at the KeyMap layer and why empty GoToStart/GoToEnd bindings are safe (skipHeaderRow's key.Matches simply never matches an empty binding). The test fixtures carry clear intent comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

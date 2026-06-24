TASK: spectrum-tui-design-3-3 — Projects keymap descriptor + §12.2 Projects-side keymap revision (drop the s→Sessions alias)

ACCEPTANCE CRITERIA:
- A Projects keymap descriptor is added to the Phase 2 single-source descriptor type, carrying the §12.1 Projects keymap with footer-core vs help-only entries tagged.
- The descriptor drives BOTH the condensed Projects footer (⏎ new session / x sessions / e edit / / filter / ? help) AND the complete Projects help — neither hand-authored.
- Pressing s on the Projects page no longer toggles to Sessions (the s→Sessions arm is removed); x toggles Projects↔Sessions (both directions, unchanged from 2-1).
- No uppercase bindings introduced.
- The command-pending Projects keymap (§11.4) is untouched.
- Every other Projects action dispatches identically (parity): enter=new session, e=edit, d=delete, n=new-in-cwd, q/Esc/Ctrl+C, navigation.
- The Sessions-side x toggle and dropped p alias from 2-1 are not duplicated/re-touched.

STATUS: Complete

SPEC CONTEXT:
- §12.1 Projects keymap: ↑/↓ move · Ctrl+↑/↓ page · / filter · Enter new-session-from-project · x → Sessions · e edit · d delete · n new-in-cwd · q quit · Esc.
- §12.2 revision: x toggles Sessions↔Projects (both directions); the former Projects-side s→Sessions alias is dropped so each key has a single meaning; no uppercase bindings anywhere.
- §6.3 condensed Projects footer: ⏎ new session · x sessions · e edit · / filter · ? help; the full set (d delete, n new in cwd, navigation) lives in the §8.5 ? help modal.
- §8.5: the ? help modal is generated FROM the page's keymap descriptor (single source) — lists the COMPLETE keymap including footer keys — not hand-authored.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/keymap.go:126-140 — projectsKeymap() descriptor added to the single-source keymapEntry type from Phase 2 task 2-1. Same shape as sessionsKeymap. Core-tagged footer entries (⏎/x/e///?) vs help-only (↑↓/^↑/↓/d/n/q/esc). ? help marked RightAligned. No s alias, no vim aliases, no uppercase, no page-jump keys.
  - internal/tui/model.go:2278-2289 — the x arm is intact, with command-pending gating (return m, nil) preserved and the refreshSessionsAfterPreviewCmd("") side-effect surviving untouched. No `case isRuneKey(msg, "s")` arm exists in updateProjectsPage (the §12.2 drop). After removal, s falls through to m.projectList.Update (model.go:2307-2309) — confirmed harmless: bubbles/list has no Projects bind on s.
  - internal/tui/footer.go:75-77 — renderProjectsFooter is descriptor-driven (renderCondensedFooter(projectsKeymap(), ...)); the legacy three-column projectHelpKeys/projectFooterBindings path is retired (no live references remain — only a doc comment in keymap.go:110).
  - internal/tui/model.go:3844 — the Projects ? help modal is descriptor-driven (renderHelpModalOnClearedCanvas(projectsKeymap(), ...)).
  - internal/tui/keymap.go:153-159 — commandPendingKeymap() is a SEPARATE descriptor (the §11.4 footer); untouched by this change.
- Notes:
  - The §12.2 drop is clean: grep confirms the only remaining `isRuneKey(msg, "s")` arm (model.go:3020) is the Sessions-side switch-view handler inside updateSessionList, not the Projects page.
  - Footer copy parity: descriptor Core Actions (new session / sessions / edit / filter / help) match §6.3 exactly; the footer prepends each glyph.
  - The Sessions-side x toggle / dropped p alias (model.go:3022-3026) are from 2-1 and were correctly NOT re-touched here.
  - The later BUG task 7-1 pinArrowOnlyNav work (arrow-only nav on newProjectList) is correctly out of scope for this task and not conflated.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/projects_keymap_test.go (TestProjectsKeymap) — descriptor data: exact nav-first enumeration, Core vs help-only split (⏎/x/e///? Core; d/n/↑↓/^↑/↓/q/esc help-only), only ? right-aligned, complete help-list presence, Core relative order (footer unchanged), and a banned-key guard that explicitly bans "s" (the §12.2 drop) plus vim/uppercase/page-jump keys.
  - internal/tui/projects_keymap_dispatch_test.go (TestProjectsKeymapRevision) — LIVE updateProjectsPage dispatch: s no longer toggles to Sessions, s is a harmless no-op (no modal/no crash), x still toggles Projects→Sessions, no uppercase S/X toggle, arrow-only nav (vim/uppercase/page-jump aliases do not move the cursor), ↑/↓ move + Ctrl+↑/↓ paging stays bound exactly to ctrl+up/ctrl+down.
  - TestProjectsRetainedActionParity — e/d/n/Enter/q/Ctrl+C/Esc/? each traced to its exact dispatch target (modal opened / page kept / createSession cmd / quit cmd / modalHelp).
  - TestProjectsCommandPendingGatingUnchanged — x/e/d/s remain no-ops under commandPending (the §11.4 gating is intact).
  - internal/tui/keymap_dispatch_guard_test.go (TestProjectsDescriptorDispatchParity) — two-way descriptor↔dispatch correspondence: every non-help descriptor Key has a live-dispatch honour probe AND every probe key appears in the descriptor. This is the structural backstop that catches descriptor/dispatch drift (e.g. re-adding s to one but not the other).
- Notes:
  - Coverage is precisely scoped to the task's seven required test cases plus the drift guard — no over-testing, no under-testing. The arrow-only-nav cases drive the LIVE bubbles/list dispatch (not just the descriptor copy), which the test comment correctly notes the descriptor-layer test alone would have falsely assured.
  - The x-parity test (projects_keymap_dispatch_test.go:119-131) asserts only the page transition, not the refreshSessionsAfterPreviewCmd cmd shape (cmd is discarded with `_ = cmd`). The side-effect's survival is verified by reading the dispatch (the arm is byte-identical to pre-change); a cmd-non-nil assertion would require wiring a SessionLister. Acceptable — the page transition + an un-touched dispatch arm is the parity invariant, and the test comment documents the deliberate omission.

CODE QUALITY:
- Project conventions: Followed. Descriptor is pure data (no rendering), bound through the same renderCondensedFooter / renderHelpModalOnClearedCanvas machinery as Sessions; matches the established keymapEntry vocabulary. Tests do not use t.Parallel(). Idiomatic Go.
- SOLID principles: Good. Single source of truth for footer+help DISPLAY; dispatch correspondence held by the guard test (the documented out-of-scope boundary for the descriptor). The command-pending footer is correctly a distinct descriptor.
- Complexity: Low. The change is a data addition + a dispatch-arm deletion.
- Modern idioms: Yes.
- Readability: Good. Doc comments on projectsKeymap (keymap.go:103-125) thoroughly explain order, Core split, the dropped s alias, and the retired legacy source.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

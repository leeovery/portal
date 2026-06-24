TASK: spectrum-tui-design-2-1 — Sessions keymap descriptor (footer/help data source) + §12.2 keymap revision

ACCEPTANCE CRITERIA (from plan / tick-247636):
- A Sessions keymap descriptor exists and is the single declarative source for the Sessions footer and (later) help; enumerates the §12.1 keymap with core vs help-only classification matching §3.4.
- The `p` Sessions→Projects rune case is removed; `x` toggles Sessions⟷Projects and `s` is the Sessions-only cycle — each key has exactly one meaning.
- No vim aliases / no PgUp/PgDn/Home/End / no uppercase bindings dispatchable on Sessions; `k` dispatches kill; nav is ↑/↓, paging Ctrl+↑/↓.
- `?` is NOT bound by 2-1 (swallow remains at 2-1; descriptor models `? help` for the footer hint only).
- Behaviour parity: every retained action dispatches to the same handler/result as before.
- No vhs capture for 2-1 (deferred to 2-4, the descriptor's first visible consumer).

STATUS: Complete

SPEC CONTEXT:
§12.1 pins the per-screen Sessions keymap; §12.2 is the deliberate revision: arrows-only nav (drop h/j/k/l, g/G, PgUp/PgDn/Home/End), `k`=kill, no uppercase, `x` toggles Sessions⟷Projects both directions, `s` is Sessions-only cycle, drop the former `p` (Sessions→Projects) and `s` (Projects→Sessions) aliases. §3.4 defines the core-footer set (↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects + right-aligned ? help); n/r/k/q/paging are help-only. §8.5/§14.4 require ONE keymap descriptor as single source of truth driving BOTH footer and `?` help. §12.2 binds `?` in a later phase; 2-1 keeps the swallow.

IMPLEMENTATION:
- Status: Implemented (and correctly carried forward through later phases without regression)
- Location:
  - internal/tui/keymap.go:22-59 (keymapEntry type), :86-101 (sessionsKeymap descriptor)
  - internal/tui/model.go:2934-3041 (updateSessionList dispatch — §12.2 revision applied)
  - internal/tui/model.go:1011-1018 (pinArrowOnlyNav — bubbles/list v2 nav rebind)
- Descriptor: `sessionsKeymap()` enumerates exactly the §12.1 set, Core flags match §3.4 (↑↓/⏎///␣/s/x + ? right-aligned = Core; n/r/k/q/^↑/↓ = help-only). RightAligned set only on `?`. The HelpKey/HelpAction split (added by later footer-glyph/help phases) is a superset of 2-1's original fields but does not violate 2-1's contract — it is the same descriptor, still the single source.
- Dispatch (model.go updateSessionList): NO `p` case anywhere (grep across internal/tui confirms zero `isRuneKey(msg, "p")` / `"p/x"` survivors). `x`→PageProjects (sole toggle, :3024-3026), `s`→handleSwitchViewKey (:3020-3021) sits BELOW the `if m.sessionList.SettingFilter() { break }` guard (:2959-2961) so `s` is literal while filtering. `k`→handleKillKey, `r`→handleRenameKey, `n`→handleNewInCWD, `q`/Ctrl+C/Esc quit, Enter→handleSessionListEnter, Space→preview — all preserved.
- §12.2 nav audit: `pinArrowOnlyNav` rebinds the bubbles/list v2 DefaultKeyMap to ↑/↓ + Ctrl+↑/↓ only, empties GoToStart/GoToEnd (drops g/G + Home/End), and crucially un-shadows `k` (v2 binds k→CursorUp, which would have shadowed Sessions k=kill). Applied in newSessionList (:996) and newProjectList (:1034). This is the correct, non-obvious remedy the task's audit step called for.
- Single-source proof: footer.go:65 renders the footer from `sessionsKeymap()`; model.go:4036 renders the `?` help modal from the SAME `sessionsKeymap()`. No duplicated binding list. The legacy `sessionHelpKeys`/`sessionFooterBindings`/`renderKeymapFooter`/`chunkBindingsIntoThreeColumns` plumbing the task left "compiling for now" was fully retired by 2-4 (grep confirms zero survivors) — exactly as the plan sequenced it.
- Drift: `?` is now BOUND to modalHelp (model.go:2965-2968), which is BEYOND 2-1's scope. This is correct, expected forward progression — Phase 3 bound it. 2-1's commit (c22b2989) introduced only keymap.go, the §12.2 dispatch revision, pinArrowOnlyNav, and the parity tests, with the `?` swallow intact. No regression of 2-1's contract.

TESTS:
- Status: Adequate
- Coverage:
  - keymap_test.go (TestSessionsKeymap): enumerates exact descriptor incl. order; Core vs help-only classification; only `?` right-aligned; Core relative order locked (footer order invariant); HelpKey override rules; no uppercase/vim/page-jump keys in descriptor.
  - sessions_keymap_dispatch_test.go (TestSessionsKeymapRevision): `p` no longer reaches Projects; `x`→Projects; `s`→grouping cycle; `s` literal while filtering; banned nav (h/j/l/g/G/b/u/f/d/PgUp/PgDn/Home/End) moves neither cursor nor page; ↑/↓ moves. TestSessionsRetainedActionParity: k→kill modal, r→rename modal, space→preview, Enter→attach no-op path, q/Ctrl+C/Esc quit.
  - keymap_dispatch_guard_test.go (TestSessionsDescriptorDispatchParity): two-way descriptor↔dispatch correspondence — every non-help descriptor Key is honoured by live dispatch with its bound effect, AND every probed bound key appears in the descriptor. This closes the exact drift gap the descriptor leaves open by construction (display vs dispatch are separate hand-coded layers). Strong, behaviour-anchored, not display-only.
  - switch_view_key_test.go: the `s` cycle (Flat→Project→Tag→Flat), unconditional cycle (zero sessions / zero tags), persistence once-per-press, persist-failure tolerance, nil-persister tolerance, page/cursor reset, and the `s`-literal-while-filtering edge — thorough behaviour parity for the Sessions-only cycle.
- Edge cases (all from the task) covered: `s` literal while filter focused (dispatch + switch_view tests), filter-mode bindings excluded from core footer (descriptor Core classification + filter_footer split), no uppercase aliases (descriptor banned-key test + dispatch banned-nav test), dropping `p` does not orphan Projects toggle (x→Projects both directions: sessions dispatch test + projects guard x→Sessions), parity of every dispatched action (retained-action parity + dispatch guard).
- Not under-tested: every acceptance criterion and edge case has a matching assertion; the guard test additionally protects against future descriptor↔dispatch drift.
- Not over-tested: assertions are behaviour-anchored (handler reached / page flipped / modal opened / quit cmd), not implementation-detail snapshots. The descriptor exact-equality test asserts display data the task explicitly pins (glyphs/labels/flags) — appropriate for a single-source-of-truth contract, not redundant.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (cmd-package mock-injection convention respected, and tui tests likewise). Small-interface DI seams (sessionKiller/sessionRenamer/enumerator/reader/sessionCreator) used in test fixtures. Token-layer discipline respected by consumers (no literal hex at descriptor call sites — descriptor is pure data).
- SOLID: Good. keymapEntry is a pure data record; sessionsKeymap is a single-responsibility constructor; rendering is fully separated into footer.go / help-modal path (the descriptor does no rendering, as the task mandates). pinArrowOnlyNav is a single-purpose helper shared by both lists (DRY).
- Complexity: Low. Dispatch switch is flat and readable; the SettingFilter guard placement is documented in-line with an explicit "do NOT hoist this case above that guard" warning.
- Modern idioms: Yes. v2 key-matching helpers (keys.go) preserve v1 semantics one-to-one; idiomatic Go throughout.
- Readability: Strong. The type doc on keymapEntry explicitly scopes the descriptor to DISPLAY-only and names the guard test that holds the descriptor↔dispatch correspondence — this is exactly the kind of load-bearing comment that prevents a future contributor from assuming the descriptor governs dispatch.
- Security / performance: N/A — pure in-memory keymap data; no I/O, no loops of concern.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (No concrete actionable change identified — the descriptor, dispatch revision, nav rebind, single-source wiring, and test coverage all match the spec and the task contract. Later-phase additions to keymap.go/dispatch are correct forward progression, not findings against 2-1.)

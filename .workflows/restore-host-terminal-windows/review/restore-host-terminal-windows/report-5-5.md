TASK: restore-host-terminal-windows-5-5 — Keymap coexistence + filter-focus key routing

ACCEPTANCE CRITERIA:
- In multi-select mode, k/x/r are no-ops (no kill modal, no page switch, no rename modal).
- In multi-select mode, Space opens preview, / starts filtering (FilterState()==Filtering), s cycles grouping.
- While the filter input is focused, s and m are literal filter characters.
- While the filter input is focused, Enter commits-to-browse and Esc clears the filter — multi-select open/exit do not fire.
- q and Ctrl+C still quit from within multi-select mode.
- Out of the mode, k/x/r behave exactly as before.
- keymap_dispatch_guard_test.go stays green (suppressed arms still honoured for the default-mode probes).

STATUS: Complete

SPEC CONTEXT: Spec "Multi-Select Mode → Key coexistence within the mode" — Live in mode: Space (preview), / (filter), s (regroup); Suppressed in mode: k (kill), x (page-toggle), r (rename), "and other row actions". "Filter as an inner sub-state": the focused filter input owns Enter/Esc (commit-to-browse / clear-filter); multi-select's Enter (open-marked) and Esc (exit-mode) apply only when the filter is not focused. The design already layers this: the `if m.sessionList.SettingFilter() { break }` guard sits above the rune switch, keyIsCtrlC is handled before mode logic, and / is delegated to list.Update.

IMPLEMENTATION:
- Status: Implemented (matches plan; extends it — see Notes)
- Location:
  - internal/tui/model.go:3337 — SettingFilter break above the rune switch (makes s/m literal and delegates Enter/Esc while filtering).
  - internal/tui/model.go:3332 — Ctrl+C → tea.Quit before any mode logic.
  - internal/tui/model.go:3353 — Space handler above the inner switch, ungated (stays live in mode).
  - internal/tui/model.go:3378-3391 — Esc: leading `if m.multiSelectMode { return m.exitMultiSelect(), nil }`, reachable only when filter not focused.
  - internal/tui/model.go:3392 — q → tea.Quit (unconditional).
  - internal/tui/model.go:3394-3410, 3436-3444 — k/r/x gated `if m.multiSelectMode { return m, nil }`; else run existing handlers. Arms kept present.
  - internal/tui/model.go:3411-3420 — n (new-in-cwd) ALSO gated no-op in mode (beyond the literal k/x/r task text; see Notes).
  - internal/tui/model.go:3445-3454 — Enter routing: `if m.multiSelectMode { return m.handleMultiSelectEnter() } else return m.handleSessionListEnter()`.
  - internal/tui/keymap.go:89-105 — sessionsKeymap descriptor UNALTERED for suppression (m entry from 5.1, n entry pre-dates this feature — spectrum-tui-design c22b2989).
- Notes:
  - Enter routing verified: the inner-switch Enter arm is reachable only when the filter is not focused (the SettingFilter break delegates a focused-filter Enter to the list), so a focused-filter Enter never triggers multi-select open — exactly as the task requires.
  - handleMultiSelectEnter's N≥2 arm now calls beginBurst (Phase 6 wiring). That is out of 5.5's scope but does not affect 5.5's contract: 5.5 only owns routing Enter to handleMultiSelectEnter, which is correct. beginBurst safely DEFERS (pendingBurstEnter=true, returns m,nil) when detection is unwired/unresolved, so no attach/quit fires.
  - Scope extension (justified): the implementation additionally suppresses n in-mode. n → handleNewInCWD → createSessionInCWD creates a session and quits the picker, silently discarding the marked set — squarely the spec's "and other row actions" intent. It is well-commented (model.go:3411) and covered by dedicated tests. Not negative scope creep; a spec-aligned, defensively-correct addition. The n descriptor entry itself pre-dates this feature (unchanged), consistent with the task's "do not alter the descriptor for the suppression" instruction.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/multi_select_keymap_test.go — TestMultiSelectSuppressesRowActions (k/x/r no-op + not-exit), TestMultiSelectKeepsCoexistingKeysLive (Space/ / /s live), TestMultiSelectFilterFocusedLiteralKeys (s/m literal into query, filter stays focused, mode/selection undisturbed), TestMultiSelectFilterFocusedEnterEsc (focused-filter Enter→FilterApplied no quit no select; Esc→Unfiltered stays in mode, selection intact, no quit), TestMultiSelectQuitKeys (q & Ctrl+C quit in mode), TestMultiSelectEnterRoutesToBurstArm (N≥2 Enter routes to handleMultiSelectEnter not single-attach; defers with mode+selection intact), TestOutOfModeRowActionsUnchanged (k→kill modal, x→Projects, r→rename modal), plus TestMultiSelectSuppressesNewInCWD / TestOutOfModeNewInCWDUnchanged for the n extension.
  - internal/tui/keymap_dispatch_guard_test.go — TestSessionsDescriptorDispatchParity probes k/x/r/m/n/q/Enter etc. against a NON-multi-select model, so the gated arms are still honoured; both drift directions bite. Directly satisfies AC7.
  - internal/tui/keymap_test.go — TestSessionsKeymap locks the descriptor (order, Core flags, RightAligned, no vim/uppercase aliases), confirming the descriptor is unchanged by this task.
  - Every acceptance criterion maps to at least one assertion; each test would fail if the specific gate/route were removed (e.g. dropping the k gate flips modal to modalKillConfirm and fails the assertion).
- Notes:
  - Tests drive updateSessionList directly (established pressSession pattern) and use the full Update for filter-input flows (pressSlash/typeKeys drain the async filter cmd) — correct seam choices; FilterValue updates synchronously so the literal-key assertions are sound.
  - No over-testing: assertions are distinct (page/modal/filter-state/selection-count/quit-cmd), no redundant happy-path duplication, stubs are minimal non-nil routing seams.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); tests are model-unit (no real tmux/daemon); descriptor↔dispatch parity guarded; suppressed arms kept present per plan; comments cite spec sections.
- SOLID principles: Good. Routing stays in the single updateSessionList chokepoint; exit path shared via exitMultiSelect; Enter boundary isolated in handleMultiSelectEnter.
- Complexity: Acceptable. The gates are flat early-returns inside existing switch arms; layering (Ctrl+C → SettingFilter break → Space → inner switch) is linear and well-documented.
- Modern idioms: Yes. Idiomatic Go; map-set membership; leading-guard returns.
- Readability: Good. Each gated arm carries a distinct comment explaining why the key is suppressed and why the arm is kept for the parity probe.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/model.go:3394-3444 — the four suppression arms (k/r/n/x) each repeat `if m.multiSelectMode { return m, nil }`. A shared guard could dedupe, but the plan explicitly endorsed the per-arm form and each arm's comment differs; extracting would obscure intent for little gain. Leaving as-is is defensible — flagged only for awareness, no change recommended.

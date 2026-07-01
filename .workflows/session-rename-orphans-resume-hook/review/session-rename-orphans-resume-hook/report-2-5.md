TASK: 2-5 — Cross-site consistency test: registration read (ResolveHookKey) == cleanup enumeration (ListAllPaneHookKeys) for the same live session (live half of the cross-site invariant). Test-only task; no production change.

ACCEPTANCE CRITERIA:
1. Stamped single-pane: ResolveHookKey(name) == "tok123:0.0" and that exact string is a member of ListAllPaneHookKeys() (byte-identical).
2. Multi-pane stamped: every per-pane ResolveHookKey(paneID) is a member of the single ListAllPaneHookKeys() slice, all sharing tok123 prefix with distinct w.p suffixes.
3. Un-stamped: both sites agree on the name-based key "<sessionName>:0.0" (byte-identical).
4. Assertion is byte-identity (exact string equality / slices.Contains), not normalised equivalence.
5. Test carries NO build tag, skips cleanly via SkipIfNoTmux.
6. go build succeeds; go test ./internal/tmux/... passes (skips where tmux absent).

STATUS: Complete

SPEC CONTEXT:
Spec §"Hook-Key Derivation (the four stages)" states the central invariant: every site that produces/consumes a hook key derives it by the identical rule (prefer @portal-id, else session_name, suffixed :window.pane). §"Testing Requirements → Cross-site consistency" requires that for a stamped session the registration read (ResolveHookKey), the cleanup live-key enumeration (ListAllPaneHookKeys), and the restore baker produce byte-identical keys. §"Risks → Missed key-producing site" names this the primary risk: if any site keys off the name-based StructuralKeyFormat instead of the hook-key derivation, stamped hooks orphan at scale. This task covers the LIVE half (registration read == cleanup enumeration); the restore-baker leg is Phase 3. Confirmed both live primitives read the identical tmux.HookKeyFormat ("#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}", tmux.go:849) through different verbs — ResolveHookKey via display-message (tmux.go:344-350), ListAllPaneHookKeys via list-panes -a -F (tmux.go:901).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/hookkey_cross_site_realtmux_test.go (package tmux_test, no build tag)
- Notes:
  - Three top-level tests map 1:1 to the three acceptance cases: TestCrossSiteConsistency_StampedSession (55), _MultiPaneStampedSession (104), _UnstampedSession (180). All SkipIfNoTmux-gated (56, 105, 181); none use t.Parallel().
  - Stamped: stamps @portal-id=tok123, asserts reg == "tok123:0.0" (79) AND slices.Contains(live, reg) (92). Meets criterion 1 byte-identically.
  - Multi-pane: SplitWindow(name+":0") + NewWindow(name) → 3 panes; enumerates #{pane_id} via list-panes -s (sessionPaneIDs, 218), asserts exactly 3 (136), resolves each pane's key via ResolveHookKey(paneID), asserts HasPrefix "tok123:" (157) + slices.Contains(live, reg) (162), and 3 distinct keys via uniqueCount (170). Meets criterion 2.
  - Un-stamped: deliberately no stamp, asserts reg == "<name>:0.0" (203) AND slices.Contains(live, reg) (211). Meets criterion 3.
  - Byte-identity: all membership checks use slices.Contains against exact strings; equality checks are ==. No normalisation anywhere. Meets criterion 4.
  - No build tag present; gated solely by SkipIfNoTmux. Meets criterion 5.
  - Compile check (static): portalIDLiteral is a const in the same-package hookkey_test.go:17 (accessible from package tmux_test). Helper signatures verified against source: tmuxtest.New(t, prefix), ts.Client(), ts.WaitForSession(t,name,timeout), ts.Run(t,args...); client.NewSession(name,dir,shell), SetSessionOption(session,name,value), SplitWindow(target,cwd,shell) [3-arg call correct], NewWindow(target,name,cwd,shell) [4-arg call correct], ResolveHookKey(paneID), ListAllPaneHookKeys(), EnsureServer(). All imports (slices, strings, testing, time, tmuxtest) are used. Package-level helpers sessionPaneIDs/uniqueCount are defined only in this file — no collision within package tmux_test.
  - No drift: the task explicitly directs resolving multi-pane keys by concrete #{pane_id} (rather than session:w.p targets as the sibling hookkey_format test does) to avoid active-pane resolution ambiguity — the implementation follows this exactly.

TESTS:
- Status: Adequate
- Coverage: All three spec/acceptance cases (stamped single-pane, multi-pane stamped, un-stamped) plus the two named edge cases (distinct w.p suffixes under one id agreeing across sites; un-stamped no-migration coincidence). Every assertion targets the observable cross-site invariant (registration key present byte-identically in cleanup enumeration).
- Notes:
  - Would fail if the invariant broke: if ResolveHookKey and ListAllPaneHookKeys ever derived keys differently (e.g. one keyed off StructuralKeyFormat), slices.Contains(live, reg) would be false and the test would error — exactly the regression it guards.
  - Membership (not whole-slice equality) is correctly used so the harness's anchor/bootstrap panes do not break the test — matches the task's explicit instruction and the spec's "assert membership not whole-slice equality" note.
  - The multi-pane distinctness check (uniqueCount == 3) is a genuine additional guard (a suffix collision would otherwise be masked by membership passing), not redundant.
  - Not over-tested: no redundant assertions, no unnecessary mocking (real tmux is intrinsic to this cross-verb invariant), no implementation-detail coupling — it asserts public-method outputs only.
  - The stamped-single-pane exact-equality (reg == "tok123:0.0") and un-stamped exact-equality (reg == "<name>:0.0") are pre-conditions that also independently pin each ResolveHookKey branch; reasonable, not bloat.

CODE QUALITY:
- Project conventions: Followed. package tmux_test (black-box), no t.Parallel() (per CLAUDE.md / spec Conventions), SkipIfNoTmux gate, tmuxtest socket fixture with "ptl-xsite-" prefix, -f /dev/null base-index-0 assumption documented in the file header. Consistent with sibling real-tmux guards (resolve_hookkey_realtmux_test.go, list_all_pane_hookkeys_realtmux_test.go).
- SOLID principles: Good — small focused test helpers, single responsibility per test.
- Complexity: Low. Linear setup→assert flow; the one loop is a straightforward per-pane membership check.
- Modern idioms: Good — slices.Contains, strings.SplitSeq (Go 1.24+ iterator), t.Helper() on both helpers.
- Readability: Strong. The 37-line file header precisely explains the invariant, why a real server is required (same format, different verbs), why membership over whole-slice equality, and the base-index-0 rationale. Per-assertion comments state what a failure would mean.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tmux/hookkey_cross_site_realtmux_test.go:218 and internal/tmux/hookkey_format_realtmux_test.go:126 — Both files enumerate/address a 3-pane (split + new-window) stamped session with near-identical setup; a shared package-level "seed a 3-pane stamped session and return its pane ids" helper would remove the mild setup duplication across the two real-tmux test files. Requires a judgement call on whether a shared fixture is worth the coupling between two otherwise-independent test files, so left as an idea rather than a mechanical quickfix.

TASK: cli-verb-surface-redesign-2-1 — `-s/--session` pin: session-domain attach, never mints, hard-fails on miss

ACCEPTANCE CRITERIA (task edge cases):
- `-s` matching only a `_`-prefixed internal session (`_portal-saver`) is a miss against the user-visible `ListSessions` view and hard-fails.
- `-s` never falls back to the picker on miss.
- `-s` bypasses the guessing chain (never tries path/alias/zoxide even when a same-named dir/alias exists).
- session glob under `-s` expanding to >1 session → burst deferred to Phase 3 (single-match attaches).
- inside-tmux switch-client vs outside exec attach.
- empty session set → hard-fail.

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags: `-s/--session <name-or-glob>` → attach; hard fail on miss; never mints. § Pinned-domain contract — never falls back to the picker: every pin hard-fails on unresolvable and never pops the TUI; `--session` never mints (a bare name has no dir to mint from). § Target resolution precedence — "Session set — user-visible only": all session-domain resolution matches only the leading-underscore-filtered `ListSessions` view, so `_portal-saver`/`_portal-bootstrap` are never matchable. The inside/outside split only selects the connector (switch-client inside, exec attach outside).

IMPLEMENTATION:
- Status: Implemented (correct)
- Location:
  - internal/resolver/query.go:289-311 `ResolveSessionPin` — session-domain only; fetches the user-visible set once via `ListSessionNames`, tolerant of a nil/error lister (collapses to "no sessions"); exact `slices.Contains` match → `SessionResult{Domain:"session"}`; miss (exact/zero-glob/empty set) → verbatim `"No session found: %s"` hard error; never consults aliases/zoxide/filesystem.
  - cmd/open.go:259-272 pin dispatch table + :317-328 `resolvePinAndOpen` — the single shared arm for all four pins; :345-363 `openResolved` routes `*SessionResult` → `openSessionFunc`.
  - cmd/open.go:143-149 `openSession` → :136-141 `buildSessionConnector` selects `SwitchConnector` (inside tmux) vs `AttachConnector` (outside, `syscall.Exec` of `tmux attach-session -t =<name>`).
  - internal/tmux/tmux.go:243-257 (ListSessions underscore filter) + :265-275 (`ListSessionNames` delegates) — the user-visible-set guarantee AC #1 rests on.
- Notes: The four-pin table + `resolvePinAndOpen` (Task 7-2 extraction) cleanly removes copy-paste; `-s` is checked before the no-target early return so `open -s <name>` resolves the pin rather than launching the picker. A command on a `-s` attach is rejected in the shared `*SessionResult` arm (`commandAttachOnlyMessage`), covering Task 2-6's guard for this pin. All six edge cases are satisfied in production. The glob edge case is met via the Phase-3 burst path (see NON-BLOCKING NOTES): a `-s` glob single target trips `isMultiTarget` (cmd/open.go:230-233, cmd/open_burst.go:61-83) and routes to `dispatchOpenBurst` → `resolveOpenSurfaces` → `ResolveSessionPinAll`; single-match degenerates to `openResolved` (attach), K≥2 bursts.

TESTS:
- Status: Adequate (one stale/misleading glob test — see notes)
- Coverage:
  - internal/resolver/query_test.go:560-706 `TestQueryResolver_ResolveSessionPin` — exact hit, zero-match glob hard-fail, exact miss, `_portal-saver` filtered-miss, empty set, lister-error→miss, and "never consults aliases/zoxide" via failing seams. Thorough, and doubles as a bypass-the-chain guard.
  - cmd/open_test.go:269-323 `SessionPin_ExactHit_RoutesToConnector` (attach, not mint, not picker); :447+ `SessionPin_Miss_HardFailsNoPicker` (asserts `openTUIFunc` never called); :370-411 `SessionPin_WithCommand_UsageError`.
  - Connector selection (inside/outside): open_test.go:1181 `TestSwitchConnector`, :2885 buildSessionConnector inside-tmux, and reattach_integration_test.go:447/529 (SwitchConnector inside vs AttachConnector-standin outside) — migrated from the retired `attach`.
  - Production `-s`-glob burst routing is covered indirectly: the bare-glob equivalents (open_multitarget_test.go:244 K≥2 bursts, :306 single→connect, :215 zero→miss) exercise the identical `globExpandableDomain`+`resolveOpenSurfaces` machinery, and :276 `-s a -s b` covers repeated session pins.
- Notes: No under-testing of the AC. Slight over/mis-testing: `TestOpenCommand_SessionPin_Glob_AttachesFirstMatch` (open_test.go:413) asserts a single-attach outcome production no longer produces for a K≥2 `-s` glob (production bursts). It passes only because `openRawArgs` is left un-overridden under `go test`, making the multi-target gate inert (openOwnArgs finds no "open" token → nil). See notes.

CODE QUALITY:
- Project conventions: Followed. Interface-based DI (SessionLister), package-level `*Deps` + `openSessionFunc`/`openTUIFunc` seams with t.Cleanup restore, no t.Parallel (package-level mutable state). The capitalised `"No session found"` message with `//nolint:staticcheck` is a deliberate byte-compat carry-over from the retired `attach`.
- SOLID principles: Good. `ResolveSessionPin` is single-responsibility (session domain only); the pin-dispatch table makes adding a pin a one-row change (open/closed).
- Complexity: Low. Straight-line resolution, clear branches.
- Modern idioms: Yes (`slices.Contains`, `strings.Cut` in the argv scan, method-value dispatch).
- Readability: Good; comments are thorough (house style). One comment is now stale (see notes).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/resolver/query.go:295-298 (+ stale doc at :281-288) — the `if HasGlobMeta(query)` branch of `ResolveSessionPin` (returns `matches[0]`, Domain "glob") is production-dead post-Phase-3: any `-s` glob trips `isMultiTarget` and routes through the burst (`ResolveSessionPinAll`), so the single-pin path only ever sees a non-glob value. This is the exact sibling of the branch Task 8-2 removed from `QueryResolver.Resolve`; the sweep missed `ResolveSessionPin`. Remove the dead glob branch (make the pin exact-only) and drop the "first match at single-target arity" clause from the doc comment, mirroring 8-2.
- [quickfix] cmd/open_test.go:413 `TestOpenCommand_SessionPin_Glob_AttachesFirstMatch` — asserts single-attach of the first glob match, which contradicts production (a K≥2 `-s` glob bursts). It only passes because `openRawArgs` is not overridden, leaving the burst gate inert under `go test`. Convert it to a real burst-routing test (override `openRawArgs` to `["portal","open","-s","api-*"]`, mirroring open_multitarget_test.go:244), or delete it once the dead branch above is removed. The resolver-level counterpart query_test.go:593 ("glob expansion attaches the first match") tests the same dead branch and should be dropped/retargeted alongside.

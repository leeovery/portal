TASK: [3-4] Bake the stable hook key in collectArmInfos via tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index) (tick-2786cf)

ACCEPTANCE CRITERIA:
- collectArmInfos sets hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index) — no tmux.PaneTarget(sess.Name, ...) remains as the hook-key derivation.
- Saved PortalID == 'tok123', saved indices w=3,p=7 -> baked --hook-key is tok123:3.7 (id-keyed, saved indices).
- Saved PortalID == "" -> baked --hook-key falls back to <sess.Name>:w.p (name-keyed, legacy).
- Multi-pane under one id: each pane's baked key shares tok123 prefix with a distinct :w.p suffix.
- Saved-index (base-index drift) preservation unchanged — hook key uses SAVED indices; FIFO/respawn still use live indices (armPanes unchanged).
- Ordering-trap guard: baked --hook-key is a function of SAVED state only; firing path (cmd/state_hydrate.go) unchanged and never reads the live @portal-id — a test asserts firing/baking is independent of the re-stamp.
- go build succeeds; go test ./internal/restore/... and go test ./cmd/... -run Hydrate pass.

STATUS: Complete

SPEC CONTEXT:
Spec § "Hook-Key Derivation" > Stage 3 (Restore lookup baking) mandates collectArmInfos change from tmux.PaneTarget(sess.Name, w.Index, p.Index) to tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index), preferring the saved @portal-id else the saved name, so the baked --hook-key matches what registration (Stage 1) stored — making a renamed-but-stamped session's hook fire across any number of renames. Base-index drift preservation is explicitly unchanged (hook key rides SAVED indices; FIFOs/respawn use LIVE indices). Spec § "Cross-Reboot Persistence" > "Firing does not depend on the re-stamp (ordering)" and § "Risks" > "Missed key-producing site" establish the load-bearing ordering trap: the firing path (cmd/state_hydrate.go) must NEVER read the live @portal-id — doing so would couple firing to the Task 3-3 re-stamp ordering and reintroduce a rename-window race. HookKey is the Phase 1 pure-Go formatter (Task 1-1); Session.PortalID is the persisted schema field (Tasks 3-1/3-2).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/session.go:117 (the single derivation change); doc-comment updated at internal/restore/session.go:62-70.
- Notes:
  - The ONLY functional change is line 117: hookKey now derives via tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index) using SAVED w.Index / p.Index. scrollAbs (saved-indexed scrollback) unchanged at line 116.
  - No tmux.PaneTarget(sess.Name, ...) remains as a hook-key derivation. The one PaneTarget call left in production (session.go:243) is the liveTarget for respawn-pane — a pane TARGET built from LIVE coords, not a hook key — correctly untouched. armPanes / buildHydrateCommand / createSkeleton / --hook-key plumbing all consume info.hookKey verbatim; only the derivation changed. Verified armPanes still uses live indices for FIFO + respawn (session.go:236-243).
  - tmux.HookKey (tmux.go:618-623) returns "<portalID>:w.p" when portalID != "" else "<name>:w.p" — matches the acceptance table exactly (tok123:3.7 for id-set; work:3.7 for empty).
  - ORDERING TRAP HELD: firing path cmd/state_hydrate.go reads cfg.HookKey verbatim from the --hook-key flag (state_hydrate.go:478, 495) and looks up hooks.LookupOnResume(store, cfg.HookKey) (state_hydrate.go:299) — it never queries live tmux for @portal-id. No GetSessionOption / display-message read of @portal-id exists in the firing path. cmd/state_hydrate.go was NOT changed by this task (its @portal-id comment references are the pre-existing "saved structural identifier, not live" contract).
  - Session.PortalID field confirmed present (internal/state/schema.go:33) and threaded from saved state.

TESTS:
- Status: Adequate
- Coverage (internal/restore/session_test.go, package restore_test, no t.Parallel()):
  - TestSessionRestorer_HydrateCommandBakesStableHookKey / "id-based when PortalID set" (298-323): PortalID=tok123, saved indices 3/7 -> asserts --hook-key 'tok123:3.7'. Covers the id-keyed + saved-indices criterion.
  - .../"name-based when PortalID empty" (325-348): PortalID="" -> asserts --hook-key 'work:3.7'. Covers the legacy fallback criterion.
  - TestSessionRestorer_HydrateBakesDistinctPerPaneSuffixesUnderOneID (351-384): 3 panes under tok123 -> asserts exactly [tok123:0.0, tok123:0.1, tok123:1.0]. Covers the multi-pane distinct-suffix criterion.
  - TestSessionRestorer_HydrateBakesKeyFromSavedStateIndependentOfLiveReStamp (386-416): the ordering-trap guard — asserts the baked --hook-key equals tmux.HookKey(sess.PortalID, sess.Name, 3, 7) computed purely from the saved struct, guarding that firing is a function of saved state only.
  - Existing TestSessionRestorer_FIFOUsesLivePaneKeyFromListPanesReQuery (468-554) still asserts hook-key stays saved 'work:0.0' under full 0/0->5/5 live drift while the FIFO/respawn use live 5.5 — pins base-index-drift preservation (hook key rides saved indices, FIFO rides live).
  - Helpers respawnPaneHookKeys / extractHookKey (438-466) parse the single-quoted --hook-key from the respawn command robustly.
- Notes:
  - Tests map 1:1 onto the task's four named test cases and each acceptance-criteria row. The ordering-trap test asserts saved-state-derivation via HookKey rather than reaching into cmd/state_hydrate.go — appropriate at this layer (the task is the BAKING site; the firing path is unchanged and covered by its own package tests / the rename-reboot integration test).
  - Not over-tested: each test targets a distinct behaviour; no redundant happy-path duplication. Assertions use strings.Contains on the exact single-quoted token, which is behaviour (the baked flag value) not implementation detail.
  - The ordering-trap test computes wantHookKey via tmux.HookKey(...) rather than a hard-coded literal. This proves derivation-from-saved-state and is a deliberate, correct choice for THIS guard (it would still fail if the derivation switched to a live read). The sibling id-based test hard-codes 'tok123:3.7', so the literal expectation is independently pinned — no risk of the formatter and test drifting together unnoticed.

CODE QUALITY:
- Project conventions: Followed. Package restore_test (external), no t.Parallel() per CLAUDE.md, mock-Commander DI seam, live-vs-saved index discipline all consistent with the file's established patterns.
- SOLID: Good. Single derivation chokepoint; the change is a one-line substitution behind the existing savedPaneArmInfo abstraction.
- Complexity: Low. No new branches — HookKey's id-vs-name conditional lives in the shared formatter, not duplicated at the call site.
- Modern idioms: Yes. Reuses the Phase 1 pure formatter; no reinvention.
- Readability: Good. The savedPaneArmInfo.hookKey doc-comment (session.go:62-70) was updated to describe the stable key (prefer saved @portal-id, else saved name, saved indices, computed from saved state only, firing never reads live id) — matches the task's doc-comment directive and the spec's ordering-trap language.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

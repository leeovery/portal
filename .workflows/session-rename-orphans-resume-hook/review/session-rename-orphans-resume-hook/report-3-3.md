TASK: Re-stamp @portal-id in createSkeleton from the saved value (best-effort after NewSessionWithCommand) — tick-dc4151 / session-rename-orphans-resume-hook-3-3

ACCEPTANCE CRITERIA:
- internal/restore/session.go imports internal/session and references session.PortalIDOption.
- On a saved session with PortalID != "", createSkeleton issues SetSessionOption(sess.Name, session.PortalIDOption, sess.PortalID) immediately after NewSessionWithCommand and before applyEnvironment.
- On PortalID == "", NO @portal-id set-option call issued (stamp skipped).
- A SetSessionOption failure is swallowed: Restore returns no error, arm phase still runs.
- Re-stamp precedes armPanes (collectArmInfos -> createSkeleton [incl. re-stamp] -> armPanes).
- NewSessionWithCommand, applyEnvironment, armPanes, collectArmInfos, firing path otherwise unchanged.
- go build succeeds; go test ./internal/restore/... passes.

STATUS: Complete

SPEC CONTEXT: Spec §"Cross-Reboot Persistence of @portal-id" item 3 ("Restore re-stamp") mandates that createSkeleton, immediately after NewSessionWithCommand recreates the session, re-stamp the saved id best-effort when present (spec lines 100-111). sessions.json is a live-state snapshot the daemon regenerates each tick, so without re-seeding the live id it is lost three ways: (a) the next capture writes "" (bare-shell next reboot); (b) post-restore stale-cleanup keys by name and deletes the just-fired hook; (c) a later rename is only stable while the live id is present. Legacy/empty id is left un-stamped -> name fallback. Spec §"Firing does not depend on the re-stamp (ordering)" (lines 115) is the ordering trap: the baked --hook-key comes from SAVED sess.PortalID via collectArmInfos, the helper never reads the live id, so firing is correct independent of the re-stamp.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/session.go:34 (import "github.com/leeovery/portal/internal/session"), :151-153 (the guarded re-stamp), :141-150 (rationale comment). Committed in 7b322d6c (T3-3).
- Notes:
  - Byte-exact match to the spec snippet (spec §3 lines 102-104): `if sess.PortalID != "" { _ = r.Client.SetSessionOption(sess.Name, session.PortalIDOption, sess.PortalID) }`.
  - Placement is correct: after the successful NewSessionWithCommand (:137-139) and before applyEnvironment (:155). Well ahead of armPanes (which runs in Restore at :104 after createSkeleton returns).
  - Uses session.PortalIDOption (= "@portal-id", create.go:29), byte-identical to the creation-time stamp and the tmux.HookKeyFormat literal — no drift.
  - Error swallowed with `_ =`, mirroring CreateFromDir (create.go:123) / QuickStart — best-effort, does not return err, does not abort restore.
  - Empty-id guard leaves legacy sessions un-stamped (name-fallback path) — matches spec line 111.
  - Import cycle: confirmed cycle-free. `go list -deps ./internal/session` contains no internal/restore edge (grep for internal/restore returned nothing); session's closure is tmux/project/resolver/state, none of which import restore. `go build ./internal/restore/...` succeeds (exit 0).
  - No drift: collectArmInfos (:111-122), armPanes (:218-250), applyEnvironment (:452-469), NewSessionWithCommand call, and buildHydrateCommand/firing path are untouched. The baked hook key still derives from SAVED state (collectArmInfos :117 via tmux.HookKey), preserving the ordering-trap invariant.

TESTS:
- Status: Adequate
- Coverage (internal/restore/session_test.go):
  - TestSessionRestorer_ReStampsPortalIDFromSavedValue (:895) — asserts the set-option @portal-id call carries the saved value "aB3xY9kZ" AND lands after new-session. Covers AC "PortalID != -> SetSessionOption issued".
  - TestSessionRestorer_SkipsReStampWhenSavedPortalIDEmpty (:931) — asserts NO @portal-id set-option call when PortalID == "". Covers the empty-skip AC and the legacy edge case.
  - TestSessionRestorer_SucceedsWhenPortalIDReStampFails (:951) — SetSessionOption returns an error via RunFunc; asserts Restore returns nil (swallowed) AND respawn-pane (arm phase) still fires. Covers the swallow AC.
  - TestSessionRestorer_ReStampsPortalIDBeforeArmingPanes (:986) — asserts stamp set-option index < respawn-pane index. Covers the re-stamp-precedes-arm ordering AC.
  - Ordering-trap guard TestSessionRestorer_HydrateBakesKeyFromSavedStateIndependentOfLiveReStamp (:386) confirms the baked --hook-key is a pure function of saved state, independent of the live re-stamp — directly protects the spec's firing-independence constraint.
- Notes:
  - Assertions are behavioural (tmux argv shape + call ordering via mock.Calls), not implementation-detail. The set-option matcher portalIDSetOptionCall (:875) pins the exact 5-element argv [set-option, -t, <name>, @portal-id, <value>], matching the real SetSessionOption shape (tmux.go:447-448).
  - The four planned test names in the task map 1:1 to the four tests above. Not over-tested: each covers a distinct AC facet (fire, skip, swallow, order) with no redundant happy-path duplication.
  - Would fail if the feature broke: removing the stamp fails the fire+order tests; removing the guard fails the skip test; returning the error fails the swallow test.

CODE QUALITY:
- Project conventions: Followed. Best-effort `_ =` swallow mirrors the sibling CreateFromDir/QuickStart convention; test file is package restore_test with no t.Parallel() (per CLAUDE.md cmd-mock constraint / repo rule).
- SOLID principles: Good. Single guarded statement inside the existing createSkeleton responsibility; no new abstraction, no interface churn.
- Complexity: Low. One `if` guard, zero added branches on the error path.
- Modern idioms: Yes. Uses the shared constant rather than a literal; no re-derivation.
- Readability: Good. The 10-line rationale comment (:141-150) explains the why (snapshot-not-store-of-record, the three loss modes, best-effort, empty-skip) precisely and matches the spec wording.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The `_ =` discarded error would normally draw a golang-error-handling flag, but here it is a deliberate, spec-mandated best-effort stamp matching the established CreateFromDir/QuickStart pattern and is documented in-line — correct handling, not a finding.)

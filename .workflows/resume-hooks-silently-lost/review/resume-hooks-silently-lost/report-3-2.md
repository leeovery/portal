TASK: resume-hooks-silently-lost-3-2 — Restore Re-Stamps The Token Before It Arms A Pane

ACCEPTANCE CRITERIA:
- Each paired pane with a non-empty saved token receives exactly one `set-option -p` carrying `state.PortalPaneIDOption` and that token
- The stamp is issued against the same live pane target as that iteration's `respawn-pane`, and precedes it in the recorded argv sequence
- A saved pane whose token is empty produces no `set-option` call and no log record of any kind
- A failed stamp produces exactly one WARN under the `restore` component with message `set pane token failed` and the attrs `session`, `pane_key`, `error` — no new component, no new attr key
- `pane_key` carries the live structural key (`state.SanitizePaneKey(sess.Name, live.Window, live.Pane)`), never the token
- A failed stamp does not abort the restore: that pane is still armed, later panes are still processed, and `Restore` returns the live pane coords with no error
- With live and saved pane counts differing in either direction, only panes within `pairCount` are stamped and the remainder carries no token
- A restore in which every saved pane is unstamped emits no WARN
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass
- The durability fixture drives its second reboot cycle from a capture taken after the restore, not from the original snapshot

STATUS: complete

SPEC CONTEXT:
§2.3 (specification.md:94) requires that after the skeleton exists and before each pane is armed, the saved token is re-stamped onto the corresponding live pane with `set-option -p`, and that a saved pane whose `PortalPaneID` is empty is skipped entirely — no option written, nothing logged — because a stamped `""` reads back indistinguishably from absence on the unstamped majority of panes. §2.4 (specification.md:96-102) adds the two rules that separate this stamp from the session stamp it replaces: the failure must not be swallowed (one WARN under the `restore` component, message `set pane token failed`, attrs `session`/`pane_key`/`error`, mirroring restore's own `set skeleton marker failed`, and no abort), and the unpaired remainder beyond `pairCount` must not be stamped, because a durable token on the wrong pane does not self-correct the way a misplaced FIFO does. §9.2 (specification.md:507-508) names the two unit-lane requirement lines: "Restore re-stamp failures are surfaced" and "`armPanes` short-list stamps nothing". No corrigendum touches this area.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restore/session.go:38-46` — `savedPaneArmInfo.hookKey` renamed to `paneToken`, single field serving both the re-stamp and the `--hook-key` bake, with the doc comment updated and still accurate.
  - `internal/restore/session.go:139-151` — inside the existing `for i := range pairCount` loop: `liveTarget` computed once at :140, `state.CreateFIFO` at :142, the stamp at :146, `RespawnPane(liveTarget, …)` at :149. Stamp and respawn address the identical `liveTarget`, and the stamp precedes it. No second loop, no separate pass; the `pairCount` bound at :132 leaves the unpaired remainder untouched.
  - `internal/restore/session.go:157-169` — `restampPaneToken` returns early on an empty token (no tmux call, no log), and on a non-nil `SetPaneOption` error emits `r.logger().Warn("set pane token failed", "session", …, "pane_key", …, "error", err)` — byte-identical in shape and attr set to the `set skeleton marker failed` emission at `internal/restore/session.go:294`. It returns nothing, so the caller cannot abort on it.
  - `internal/restore/session.go:146` passes `liveKey` = `state.SanitizePaneKey(sess.Name, live.Window, live.Pane)` (computed at :139) as `pane_key`, never the token.
  - `internal/tmux/tmux.go:314-320` — `SetPaneOption` composes `set-option -p -t <target> <name> <value>`; `internal/state/markers.go:26` is the single home of the `@portal-pane-id` literal, which restore reaches through the constant rather than restating.
  - Production component binding: `cmd/state_common.go:13` (`restoreLogger = log.For("restore")`) threaded via `cmd/bootstrap_production.go:46` and `internal/bootstrapadapter/adapters.go:82`, so the WARN lands under `restore` with no new component and no new attr key.
- Notes: The task's Do list wrote the stamp inline in `armPanes`; the implementation factors it into a one-purpose `restampPaneToken` helper immediately below. That is a shape improvement, not a divergence in substance — the call sits at the prescribed point in the same loop, and the empty-token skip and non-aborting WARN both live where the reader of `armPanes` is pointed at them. The commit that landed the task (fd543767) used `tmux.PaneTarget`; a later phase moved the whole file to the pinned `PaneTargetExact` form, which is what HEAD carries and what the tests assert (`=work:0.0`).

TESTS:
- Status: Adequate
- Coverage: `internal/restore/session_test.go:850-1083` (`TestSessionRestorer_ReStampsSavedPaneToken`, unit lane, `commandertest.FromFunc` recording fake — no real tmux, lane-compliant) carries nine subtests, one per acceptance criterion, and asserts the recorded argv sequence rather than only the end state:
  - :851 two saved tokens → exactly two `set-option -p` calls, asserted target/option/value via `assertPaneTokenStamp` (:825) against `=work:0.0`/`tokA` and `=work:0.1`/`tokB`.
  - :874 ordering — the `set-option` index precedes the `respawn-pane` index.
  - :896 empty token → zero `set-option -p` calls *and* an empty `sink.Body()`, which is what pins "nothing logged at all" rather than only "no WARN".
  - :917 failed stamp → `Restore` returns nil error, the live coords come back, the respawn still ran, and exactly one record with message `set pane token failed` at WARN whose attr key slice is exactly `[session pane_key error]` (`slices.Equal`, so an added attr fails).
  - :950 renumbered live coords (`5:5` vs saved `0.0`) → `pane_key` equals `state.SanitizePaneKey("work", 5, 5)`, explicitly not the saved key and not the token.
  - :976 first stamp fails, second still issued.
  - :999 / :1028 both count-mismatch directions — 3 live / 2 saved and 2 live / 3 saved — asserting the paired prefix is stamped and that neither the unpaired live target nor the unpaired saved token appears.
  - :1058 three saved panes, all unstamped, counts matching → no record at or above WARN.
  `internal/restore/rename_reboot_durability_integration_test.go:35-66` restores the capture-after-restore round trip: `state.CaptureStructure` is re-run against the live server after cycle 1 (:35), the subtest at :40 asserts the captured `Pane.PortalPaneID` equals `renamePaneToken`, cycle 2 is persisted from *that* index (:53) rather than the original snapshot, the live pane token is re-read after the second reboot (:59), and `AssertMarkerCount(…, 2)` (:65) pins the second fire. The fixture's arrange (`internal/restore/reboot_fixture_test.go:44-97`) is isolated per `IsolateStateForTest` + `RegisterStateDirTeardownGuard` + a per-test `tmuxtest` socket, and the file is `//go:build integration` as the lane rule requires for a test that builds and runs the portal binary.
- Notes: Not over-tested. The nine subtests map one-to-one onto the criteria; the closest pair (:896 empty-token silence and :1058 all-unstamped no-WARN) differ in scope — one pins total log silence for a single pane, the other pins the absence of a WARN across a three-pane two-window restore where the count-mismatch path is also in play — and both are separately required by the criteria. The `set-option -p` filter (`setPaneOptionCalls`, :815) keys on the argv shape rather than on an internal, so the assertions are behavioural.

CODE QUALITY:
- Project conventions: Followed. The `@portal-pane-id` literal is composed from `state.PortalPaneIDOption` rather than restated; the WARN reuses restore's existing message shape and attr keys, so the closed log vocabulary is unamended; `SetPaneOption` is handed a typed `tmux.Target` composed by `PaneTargetExact`, so the pinned-target rule holds; the unit test uses the shared `commandertest` fake and `logtest.NewCaptureLogger` rather than a hand-rolled handler, and no `t.Parallel()` appears.
- SOLID principles: Good. `restampPaneToken` has one responsibility and reports through the same logger seam the rest of the type uses.
- Complexity: Low. One guard clause, one call, one error branch; the loop gains one line.
- Modern idioms: Yes.
- Readability: Good. The helper's doc comment states the three decisions a reader would otherwise have to infer — why an empty token is skipped, why a failure degrades rather than aborts, and why the option cannot survive the reboot.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

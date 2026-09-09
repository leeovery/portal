TASK: resume-hooks-silently-lost-3-1 — Saved State Carries Each Pane's Token

ACCEPTANCE CRITERIA:
- `state.Pane` carries `portal_pane_id`; `state.Session` carries no `PortalID`; `SchemaVersion` unchanged at `1`; no migration code
- `captureFormat`'s trailing column is composed from `state.PortalPaneIDOption`, with no `@portal-pane-id` literal restated in `internal/state/capture.go`
- `captureFieldCount` still `11`; a pane row with 10 or 12 fields still errors
- The token is lifted per-pane (two panes of one session capture two different values)
- An unstamped pane captures `""` with no error and no log line
- A pane in `skipSet` keeps its previously-captured `PortalPaneID` from `prev`
- A pre-upgrade `sessions.json` (carrying `portal_id`, no `portal_pane_id`) decodes without error, same schema version, empty tokens
- `collectArmInfos` bakes `p.PortalPaneID` and nothing else; no `tmux.HookKey` call remains in `internal/restore`
- `createSkeleton` issues no session-scoped `@portal-id` stamp
- `buildHydrateCommand` still emits `--hook-key ''` for an empty baked token (superseded by task 3-3 — see below)
- `internal/state/portal_id_literal_guard_test.go` deleted, not re-pointed; no replacement guard
- The three reboot firing fixtures each carry a saved pane token and a `hooks.json` keyed by it, and each still asserts its hook fires
- `restoretest.SeedSessionsJSON`'s signature and existing callers unchanged; the token-carrying seed is a sibling helper
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§2.3 ("Carrying the token across the reboot gap") requires the additive `Pane.PortalPaneID` field on a tolerant-decode struct with no `SchemaVersion` bump and no migration, the `captureFormat` column *replaced* rather than appended (arity fixed at 11, meaning changed from session-scoped-repeated to genuinely per-pane), and the value lifted into `Pane.PortalPaneID` rather than `Session.PortalID`. §3.3 requires the saved-state bake to be a struct field read, not a formatting call, and deletes `tmux.HookKey`. §7.2 lists `Session.PortalID`, the `#{@portal-id}` capture column, the session-scoped lift and the restore `@portal-id` re-stamp as removals, noting the field removal is the mirror image of the addition and likewise needs no migration. §9.3 names the fixtures to re-point (`multipane_legacy`, the two rename-reboot suites, `cmd/state_daemon_run_test.go`'s `oneSession()`) and retires the un-stamped-name-fallback subtests with the branch they cover. §9.4 requires both literal-binding guards deleted rather than re-pointed, because the single home in `internal/state` leaves nothing to bind. The Corrigenda contain nothing bearing on this task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/schema.go:38-48` — `Pane.PortalPaneID string \`json:"portal_pane_id"\`` with an accurate doc comment; `Session` (`:20-24`) carries no `PortalID`; `SchemaVersion = 1` at `:11`; `DecodeIndex` (`:76-90`) holds no migration branch.
  - `internal/state/capture.go:26` — `captureFormat`'s trailing column is `"#{" + PortalPaneIDOption + "}"`, composed by constant concatenation. A repo-wide grep for `@portal-pane-id` finds the literal only at `internal/state/markers.go:26`, its single home.
  - `internal/state/capture.go:28` — `captureFieldCount` still `11`; `parsePaneRow` (`:261-287`) rejects any other arity.
  - `internal/state/capture.go:237` — `paneRow.portalPaneID` reading `parts[10]` (`:285`), carried into `Pane.PortalPaneID` per-pane inside `buildPanes` (`:337`). The session-scoped lift is gone from `CaptureStructure` (`:80-84` appends `Session{Name, Environment, Windows}` only) and the `PortalID:` copy is gone from `findOrAppendSession` (`:166-170`).
  - `internal/state/capture.go:147-158` — `mergePane` still assigns the whole `Pane` struct (`w.Panes[i] = pp`), so a skipped pane keeps its previously-captured token.
  - `internal/restore/session.go:64-75` — `collectArmInfos` bakes `paneToken: p.PortalPaneID` and nothing else; `savedPaneArmInfo`'s doc comment (`:38-42`) is rewritten to describe the token read from saved state.
  - `internal/restore/session.go:77-113` — `createSkeleton` issues no session-scoped `set-option`; the `internal/session` import is dropped from the file's import block (`:11-23`).
  - `internal/restoretest/sessions_json.go:35-48` — `SeedSessionsJSONWithPaneTokens` is a sibling helper; `SeedSessionsJSON` (`:16-19`) and `SeedSessionsJSONWithSavedAt` (`:23-30`) keep their signatures, both routed through the shared `singlePaneSession` / `WriteIndex`.
  - Guards: `internal/state/portal_id_literal_guard_test.go` and `cmd/portal_id_binding_guard_test.go` are both absent from the tree, and a grep for `captureFormat` finds only its declaration, its one use, and a fixture comment in `cmd/state_daemon_run_test.go:220` — no replacement guard was added.
  - `tmux.HookKey` is gone from `internal/tmux/tmux.go` (task 3-4's removal, landed); `HookKeyFormat` at `:631` is the pane-token format composed from `state.PortalPaneIDOption`. No `tmux.HookKey(` call site survives anywhere in the tree.
- Notes:
  - The acceptance criterion "`buildHydrateCommand` still emits `--hook-key ''`" no longer holds in the delivered tree: `buildHydrateCommand` (`internal/restore/session.go:349-358`) omits the flag entirely for an empty token, and `cmd/state_hydrate.go:297-299` no longer marks `hook-key` required. This is task 3-3's prescribed edit, which the plan explicitly reserved for that task and which spec §3.4 requires ("`buildHydrateCommand` omits the `--hook-key` flag entirely for that pane … the flag cannot be required"). The two edits landed together as the plan demanded. Not a divergence — a superseded interim state.
  - The task's fix-tracking record flagged that the first attempt introduced three new ways to stamp a pane token instead of using `tmuxtest.Socket.StampPaneToken`. That was fixed: every re-pointed fixture now calls `ts.StampPaneToken` (`cmd/bootstrap/reboot_roundtrip_test.go:104`, `internal/restore/exit_closes_pane_integration_test.go:125`, and the rename-reboot suites through their shared fixture).
  - `TestRenameRebootHook_DurableAcrossRepeatedReboots` was temporarily weakened at this commit (the second cycle replayed the pre-reboot snapshot, dropping the "token is re-persisted by the restore re-stamp" assertion, which was not repairable before 3-2 added the re-stamp). The delivered file restores it: `internal/restore/rename_reboot_durability_integration_test.go:40-47` asserts the re-persist and `:53` drives cycle 2 from the post-restore capture.

TESTS:
- Status: Adequate
- Coverage: Every test named in the task's Tests list exists and asserts the named property.
  - `internal/state/capture_test.go:510` "it captures a stamped pane's token into that pane's record" — 11-field fake row with a token in the trailing column.
  - `:532` "it captures different tokens for two panes of one session" — the property the session-scoped lift structurally could not express; asserts `tokenPane0` / `tokenPane1` land on panes 0 and 1 respectively.
  - `:562` "it captures an empty token for an unstamped pane" — empty trailing column, no error, and `sink.Lines()` empty, which pins the "no log line" half of the criterion.
  - `:586` "it rejects a pane row with the wrong field count" — both 10-field and 12-field rows, each asserting the error text and an empty `Sessions` on the fail-fatal path.
  - `:614` "it keeps a skipped pane's previously captured token" — `skipSet` plus a `prev` index carrying `savedToken`; the merged pane keeps it against a freshly-read empty column.
  - `:649` "it leaves every existing field index unchanged after the column swap" — every pre-existing column asserted at a distinctive value, which is what makes the swap observable as a swap rather than a shift.
  - `internal/state/schema_test.go:95` (post-upgrade round-trip through the `portal_pane_id` JSON tag), `:135` (pre-upgrade payload carrying `portal_id` and no `portal_pane_id` — decodes clean, version unchanged, token empty), `:169` (`SchemaVersion` pinned at 1 naming the additive field).
  - `internal/restore/session_test.go:283` bakes the saved token as `--hook-key 'tok123'`; `:291` covers the untokened pane; `:337` bakes one distinct token per pane across a multi-pane, multi-window session; `:363` drives a commander that fails the test on any `display-message`, pinning "never read from the live server"; `:795` asserts no session-scoped `set-option -t work` during skeleton creation.
  - `cmd/state_daemon_run_test.go:218-224` — `oneSession()` keeps its 11-field row and trailing empty column, comment renamed to the pane token.
  - Integration: `internal/restore/multipane_legacy_integration_test.go` proves the per-pane lift against real tmux (two panes stamped with distinct tokens, both asserted on the captured index and both hooks asserted to route to their own pane); the three reboot firing fixtures each stamp before the capture and seed `hooks.json` under the same token — `cmd/bootstrap/reboot_roundtrip_test.go:104` + `:157` (`AssertMarkerCount(..., 1)`), `internal/restore/exit_closes_pane_integration_test.go:125` + `:52` (`WaitForFileExists`), `cmd/bootstrap/phase2_hook_fire_integration_test.go:38-47` + `:82` (`WaitForFileExists`). None is vacuous: remove the stamp and the baked key is `""`, the lookup finds nothing, and the assertion fails.
- Notes:
  - The retirements are correct rather than convenient. `internal/state/capture_internal_test.go` held exactly one test (`TestFindOrAppendSessionCopiesPortalID`) whose entire subject was the `PortalID` copy that went with it — verified against `git show 2bf6a6b1^:internal/state/capture_internal_test.go`. `TestMultiPaneLegacy_GracefulLegacyDegradation` and the five `internal/restore/session_test.go` `@portal-id` re-stamp tests covered branches §7.2 deletes; 3-2's pane-scoped equivalents replace the re-stamp coverage (`internal/restore/session_test.go:830`).
  - Not over-tested: the new cases are one per named property with no redundant duplicates, and `TestSessionRestorer_HydrateBakesKeyFromSavedStateOnly` narrows to a stricter commander rather than restating an existing assertion.

CODE QUALITY:
- Project conventions: Followed. The pane-option literal has one home (`internal/state/markers.go:26`) and every format string composes from it, which is the convention CLAUDE.md states for this constant. Lane placement is unchanged (the re-pointed fixtures keep their `//go:build integration` tags); fixture staging goes through `tmuxtest.Socket.StampPaneToken` rather than the client call under test; `restoretest`'s new seeder is a `*testing.T`-first test-only helper in a test-only package. CLAUDE.md's architecture table already describes the `Pane.PortalPaneID` schema and `captureFieldCount = 11`, and no `@portal-id` / `portal_id` / `PortalID` reference survives in CLAUDE.md or README.md.
- SOLID principles: Good. The lift moves to the layer that owns the data (`buildPanes` builds panes), and `internal/state` gains no import — `PortalPaneIDOption` was already local, so the format composition introduces no edge.
- Complexity: Low. Net −176 lines across the commit; the parse path is unchanged in shape, only the field's name and destination move.
- Modern idioms: Yes. `slices.Sort` in the new seeder; the extracted `singlePaneSession` / `WriteIndex` remove the duplication the sibling seeder would otherwise have introduced.
- Readability: Good. `Pane`'s doc comment states why the field exists (the tmux option does not outlive the server) and that `""` is the ordinary case rather than an anomaly; `savedPaneArmInfo`'s comment names the ordering trap it is protecting against; `oneSession()`'s comment names the trailing column correctly.
- Comment accuracy: Every comment in the changed code holds against it. `internal/state/capture.go:24-25` ("Columns are consumed by position, so new fields append at the end") remains true as a forward rule — this change swapped a column rather than adding one, which is exactly why the arity is unchanged. `savedPaneArmInfo`'s "whatever the restore re-stamp does" was a forward reference at this commit and is exactly right in the delivered tree (`internal/restore/session.go:157-169`).
- Issues: None reaching the bar.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

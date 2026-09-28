TASK: 5-1 — The Store Discard Route Records What It Destroyed (`Store.Discard` beside `Store.Remove` over one shared removal body; `ViaPanel` added to the closed `Via` vocabulary; two CLAUDE.md paragraph edits)

ACCEPTANCE CRITERIA:
- `Discard` removes the named key's `on-resume` entry and emits exactly one INFO under the `hooks` component carrying `op=discard`, the `hook_key`, `via=panel`, and `value` holding the removed command.
- A key holding other events alongside `on-resume` keeps those events and keeps its outer key; a key holding only `on-resume` is deleted whole, exactly as `Remove` deletes it.
- The `value` attr carries the command out of an object-form entry byte-identically to the same command stored in the string form.
- A key naming no entry, and a key whose `on-resume` event is absent, each report `(false, nil)`, write no file and emit no breadcrumb.
- An empty hook key reports `(false, nil)` without acquiring the mutation lock, without creating the `hooks.json.lock` sidecar, and without loading or saving the file.
- A save that fails and a mutation lock that cannot be acquired each report `(false, err)` with the entry still on disk; the save failure's WARN carries the existing `error` plus the `error_class` `fileutil.ClassifyWriteError` returns, and the lock failure's carries `error` and no `error_class`.
- A discard's record and a removal's record are distinguishable on both the slog message and the `op` attr, so `grep op=discard`, `grep op=rm` and `grep op=clean-stale` each select exactly one removal route.
- `hooks.Via(0).String()` is still the empty string and `hooks.ViaPanel.String()` is `panel`.
- `Remove`'s existing suites pass unmodified, and `hook rm`'s exit-status contract — exit 0 iff it removed an entry — is unchanged.
- `internal/hooks`'s leaf guard passes with no new entry: the route takes no dependency through which a tmux call could be made, so no pane token can be unstamped from it.

STATUS: complete

SPEC CONTEXT: Spec 6.2 makes a confirmed `d` on the resume panel a permanent removal of the pane's resume registration and nothing else: no token unstamped, no other entry touched. It is a third removal route beside `portal hook rm` and a hand edit. A discard that finds nothing to remove still counts as a discard. A discard that cannot be written leaves the registration standing, and the panel reports it. Spec 6.4 treats this route like the stale sweep, not the typed command: one INFO under `hooks` carrying `hook_key` and the removed command as `value`, at the production default level. It adds `op=discard` and `via=panel` to the closed vocabularies so the three removal routes stay greppable apart.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/via.go:20-21 (`ViaPanel` declared after `ViaDoctor`, so numbering still starts at one), internal/hooks/via.go:29 (`ViaPanel: "panel"` in `viaNames`)
  - internal/hooks/store.go:195-197 (`Remove` reduced to `s.removeEntry(key, event, via, "rm", false)`)
  - internal/hooks/store.go:199-205 (`Discard`, documented as the panel's removal route, `s.removeEntry(key, event, via, "discard", true)`)
  - internal/hooks/store.go:207-252 (`removeEntry`: the empty-key guard at :208-210 comes before `acquireMutationLock` at :212; the lock WARN under the given `op` with `error` and no `error_class` at :216; the `Registration` is captured before the delete at :230; the outer key is dropped when its last event goes at :235-238; the save WARN with `error` and `error_class` at :241-242 returns `(false, err)`; the success INFO at :246-250 appends `value` only when `carryValue` is set)
  - CLAUDE.md:185 (the first paragraph of Resume hooks now names the panel discard as a third removal route, says it removes the `on-resume` entry and nothing else, unstamps no token, and logs `value` under `op=discard`)
  - CLAUDE.md:197 (the sidecar paragraph's list of mutation acquires now includes "a resume panel's confirmed discard")
- Notes:
  - The factoring matches the plan's prescribed signature exactly.
  - The only behaviour change to `Remove` is the new empty-key guard. Its one production caller, `hook rm` (cmd/hooks.go:299-313), already routes an empty `--pane-key` to the `$TMUX_PANE` path and refuses an empty resolved key before reaching the store. `hook rm`'s exit contract (cmd/hooks.go:323-329) is therefore unchanged.
  - The one production caller of `Discard` is cmd/state_resume_wait.go:420. It emits no second `hooks` breadcrumb, so the store remains the only place the act is logged.
  - store.go gained no imports. The leaf guard's allowlist (internal/hooks/leaf_guard_test.go:22-28) is unchanged, and nothing in the route can reach tmux.
  - The "staleness rule cannot judge" paragraph in CLAUDE.md was left untouched, as the task instructed.
  - No later commit modifies internal/hooks after b04b2dce8.

TESTS:
- Status: Adequate
- Coverage (internal/hooks/discard_test.go, TestDiscard):
  - Removal plus the single INFO record with msg/op/component/via/hook_key/value (:59-76, via `assertDiscardRecord` :40-56, which uses `Records().Only` so exactly one record is enforced).
  - Other events and the outer key survive (:78-103).
  - The key is deleted whole on its last event, with the neighbouring entry kept (:105-124).
  - A string-form versus object-form table: a command containing quotes and a space is asserted equal to one constant in both forms, so the two are byte-identical (:126-150).
  - Absent key and absent `on-resume` event: `(false, nil)`, zero records, file byte-unchanged (:152-181).
  - Empty key against a staged `""` entry with no sidecar: `(false, nil)`, no sidecar created, zero records, file byte-unchanged (:183-205). Removing the guard would create the sidecar through `O_CREATE` and delete the entry, so the test would fail.
  - Save failure: `(false, err)`, file unchanged, WARN under op=discard/via=panel, `logtest.AssertWriteFailure` (:207-236).
  - Lock held: `ErrLockHeld`, `(false)`, file unchanged, `hookstest.AssertLockWarn` checks that neither `error_class` nor `value` is present (:238-256).
  - Discard versus rm, compared on both message and `op`, and rm carries no `value` (:258-291).
  - Neighbouring entries stay byte-unchanged, including an object-form entry with an unmodelled attribute and an old-format unjudgeable key (:293-313).
  - The `via` rows `panel` and `Via(0)` → `""` are in TestViaWireValues (internal/hooks/via_test.go:22, :25).
  - The existing `Remove` suites (store_test.go, lock_test.go, lock_write_test.go, event_test.go) were not touched by the task commit. None of them passes an empty key, so the new guard cannot change their outcome.
- Notes: Every acceptance criterion has a test that would fail if that behaviour broke. The tests are focused, with no redundant or implementation-detail assertions.

CODE QUALITY:
- Project conventions: Followed. The breadcrumb comes from the store method, which is the chokepoint. The op verb is both the message and the `op` attr. Fixtures go through `hookstest.StageStore`, records are read through a `logtest.Sink` with `AssertRecord`/`AssertWriteFailure`/`AssertLockWarn`, and there is no `t.Parallel()`.
- SOLID principles: Good. One removal body serves both routes, so they cannot drift in what they delete.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The doc comments on `Remove` and `Discard` match the code: `Remove`'s list of no-removal cases now includes the empty key, and `Discard` says it has the same contract with a different op and a `value` attr.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

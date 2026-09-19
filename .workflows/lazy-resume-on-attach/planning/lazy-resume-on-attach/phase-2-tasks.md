# Phase 2: The pending marker and the saver's freeze — 5 tasks

## lazy-resume-on-attach-2-1

### Task 2.1: The pending marker: one name, set, read and clear

**Problem**: A waiting pane has to be identifiable as waiting, by the saver on every tick and later by the picker and doctor, and Portal has no vocabulary for the kind of marker that can carry it. The only thing that keeps capture off a pane today is `@portal-skeleton-<paneKey>` — a **server** option keyed by the pane's positional address, every component of which moves (`break-pane` and `move-pane` relocate, closing an earlier window renumbers, a rename changes the session half). `internal/state` owns Portal's marker names and helpers but only over server options (`ServerOptionWriter`), and `internal/tmux` can write a pane option (`SetPaneOption`) but has no way to remove one at all.

**Solution**: `@portal-resume-pending`, set to `1`, as a **pane** user-option — the name, the presence rule and the set/unset helpers in `internal/state/markers.go` over a new pane-scoped writer seam parameterised over the target type (as `PaneCapturer[T ~string]` already is, because `internal/state` must not import `internal/tmux`), plus the missing `UnsetPaneOption` on the tmux client, declared over `tmux.Target` so no hand-composed `-t` string can reach it.

**Outcome**: One constant names the option; set and unset round-trip against a real tmux pane; unsetting a marker that is already absent is a no-op; an empty value reads as absent exactly as an unset one does.

**Do**:
- Add to `internal/state/markers.go`, beside `PortalPaneIDOption`: `const ResumePendingOption = "@portal-resume-pending"`; `type PaneOptionWriter[T ~string] interface { SetPaneOption(target T, name, value string) error; UnsetPaneOption(target T, name string) error }`; `SetResumePendingMarker[T ~string](w PaneOptionWriter[T], target T) error` writing the value `"1"`; `UnsetResumePendingMarker[T ~string](w PaneOptionWriter[T], target T) error`; and `ResumePendingSet(optionValue string) bool` reporting `optionValue != ""` — the one place the presence rule is stated.
- Add `UnsetPaneOption(target Target, name string) error` to `internal/tmux/tmux.go` beside `SetPaneOption`: `set-option -pu -t <target> <name>`, the target spent as a string at the argv, failure wrapped in the same shape `SetPaneOption` uses.
- Cover the helpers in `internal/state/markers_test.go` over a pane-writer mock modelled on the existing `writerMock` (recording target, name and value), including error propagation and the "does not call the other method" pair the skeleton-marker tests already assert.
- Pin the client method's argv in `internal/tmux/tmux_test.go` through `commandertest.Scripted`, and add `internal/tmux/pane_option_realtmux_test.go` (unit lane, `tmuxtest.SkipIfNoTmux`, disposable `tmuxtest.New` socket) round-tripping set → format read → unset → read → a second unset, plus an unset against a target no live pane answers to.
- Edit the `state` row of CLAUDE.md's package table so the marker-helpers clause names `@portal-resume-pending` as the pane-scoped pending marker with its set/unset helpers and its presence rule, beside `@portal-restoring`.

**Acceptance Criteria**:
- [ ] `state.ResumePendingOption` is the only spelling of `@portal-resume-pending` in the tree — no format string, option argument or test restates the literal.
- [ ] `SetResumePendingMarker` issues exactly one `SetPaneOption` carrying that constant and the value `1` against the target it was handed, and no unset; `UnsetResumePendingMarker` issues exactly one `UnsetPaneOption` carrying that constant, and no set; both return the writer's error unchanged.
- [ ] `tmux.UnsetPaneOption` composes `set-option -pu -t <target> <name>` and takes `tmux.Target`, so a hand-composed string argument does not compile, and `internal/tmux/target_composition_guard_test.go` passes with the method in place.
- [ ] Against a real tmux pane: a set marker reads back as `1` through `#{@portal-resume-pending}`; after the unset it reads back empty; a second unset of the now-absent option succeeds; an unset against a target naming no live pane fails.
- [ ] `ResumePendingSet` reports true for `1` and for any other non-empty value, and false for the empty string.
- [ ] `internal/state` still compiles without importing `internal/tmux` — the seam is generic over `~string`, not over the client's concrete target type.

**Tests**:
- `"it sets the pending marker to 1 on the target it was handed"`
- `"it unsets the pending marker on the target it was handed"`
- `"it propagates a writer failure from each helper"` (both helpers)
- `"it calls neither the other writer method"` (set issues no unset; unset issues no set)
- `"it reads any non-empty option value as pending and an empty one as absent"` (table over `1`, `0`, `yes`, `""`)
- `"it composes set-option -pu with the pane target"` (scripted commander)
- `"it round-trips the marker on a real tmux pane"` (real tmux)
- `"it treats an unset of an already-absent marker as a no-op"` (real tmux)
- `"it fails an unset against a target no live pane answers to"` (real tmux)

**Edge Cases**:
- Unsetting a marker that is already absent is a no-op, not an error — measured against tmux 3.7c on 2026-09-19: `set-option -pu -t <pane> @portal-resume-pending` on a pane that never carried it exits 0. Nothing in the helper special-cases absence, and no caller has to read before it clears.
- An empty value reads as absent exactly as an unset one does — measured on the same server: `set-option -p … ""` followed by a `#{@portal-resume-pending}` read returns the empty string, identical to a pane that was never written. The helpers therefore never distinguish the two, and nothing may be built on a difference tmux does not report.
- The option name is composed from one constant and never restated at a call site — the capture format (next task) and every later reader compose it from `ResumePendingOption`.
- The writer seam is parameterised over the target type so `internal/state` keeps its no-`internal/tmux` rule: that package imports this one, so the concrete `tmux.Target` cannot appear here.
- A hand-composed `-t` argument cannot reach the unset without failing the target-composition guard: the parameter's declared type is what the guard reads, so a `tmux.Target(name + ":")` conversion at a call site is a finding rather than a compile.
- An unset against a target no live pane answers to fails — measured: `no such pane`, exit 1. The helper propagates it; what a caller does with a pane that has gone is a later phase's decision, and nothing here swallows it.

**Context**:
> The marker that suppresses capture today is addressed positionally — session name plus window and pane index — and all three components move. Today that exposure is the few seconds between skeleton restore and handover, which is why it has never mattered; this feature stretches it to the whole waiting life.
>
> The marker is the pane user-option `@portal-resume-pending`, set to `1` — the value convention the skeleton markers already use. Presence is what is read; any richer payload is a later change the shape already admits.
>
> A pane option rather than a server option keyed by position: `resume-hooks-silently-lost` established this directly, and its measurements confirmed a pane user-option survives `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename. Portal models every other pane and session condition as a tmux option — the restore markers, the restoring flag, the spawn acks, the directory stamp, the pane token — and this is that vocabulary rather than an addition to it.
>
> The pending marker has no staleness case and owes no sweep: a pane option is destroyed with its pane, so a pane closed mid-wait leaves nothing behind and there is no address by which a sweep could reach one. Bootstrap's stale-marker sweep is structurally blind to it — it enumerates `@portal-skeleton-*` **server** options and unsets them as server options — which is exactly why the saver's skip has to gain a second condition rather than reuse the existing marker.
>
> No production caller sets or clears the marker in this phase: the helper that marks a pane and the waiter that clears it are Phase 4's work, on the ordering the freeze requires (marked before the mid-restore marker is cleared, cleared only once the pane has left the panel's screen).

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.3, §8.2, §4.3

## lazy-resume-on-attach-2-2

### Task 2.2: The per-pane capture read carries the pending marker

**Problem**: The saver has to know which live panes are waiting, on every tick, before it decides what to capture. It already enumerates every pane on the server once per tick with a per-pane format — the read that builds the structural index and that already carries the pane's durable token as a column — so a second whole-server read for the marker would put an extra tmux call on the hot path for a fact the existing read can carry. Nothing today parses a twelfth field, and `CaptureStructure` has no way to hand the answer back to its caller.

**Solution**: Append `#{@portal-resume-pending}` to `captureFormat`, move `captureFieldCount` from 11 to 12, lift the column per pane through `state.ResumePendingSet`, and return the pending panes from `CaptureStructure` as a second value — a set of pane keys at the panes' live addresses, the same shape as the `skipSet` it already takes. Nothing about the pending state reaches the saved record.

**Outcome**: One read per tick answers both "what is the topology" and "which panes are waiting"; a pane that has never been marked reads unmarked; `sessions.json` keeps its shape and `SchemaVersion` does not move.

**Do**:
- In `internal/state/capture.go`: append the twelfth column composed from `ResumePendingOption` to `captureFormat`, set `captureFieldCount = 12`, and add `resumePending bool` to `paneRow`, read in `parsePaneRow` through `ResumePendingSet(parts[11])`.
- Change the signature to `CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *slog.Logger) (Index, map[string]struct{}, error)`: the second value is keyed by `SanitizePaneKey(session, windowIdx, paneIdx)` at the pane's live address, built only from rows belonging to sessions that made it into the index, and is an empty non-nil map on every early return so no caller has to nil-check it.
- Leave `Pane`, `Index` and `SchemaVersion` untouched — the pending flag is returned beside the index rather than carried on it, because a non-persisted field would be compared against an index decoded from disk (where it is always false) and would make every tick read as a structural change for as long as any pane waits.
- Update the two production call sites: `cmd/state_daemon.go`'s `captureAndCommit` binds the set (the skip itself is the next task), and `cmd/state_commit_now.go`'s `CommitNowDeps.CaptureStructure` func type gains the return with the call discarding it.
- Move every eleven-field pane-row fixture to twelve: `internal/state/capture_test.go`'s `paneLine` / `paneLineWithPaneToken` helpers (plus a pending-carrying variant), its arity-rejection table (eleven and thirteen reject, twelve accepts), and the literal rows in `cmd/state_daemon_run_test.go`, `cmd/state_daemon_capture_logging_test.go`, `cmd/state_daemon_cycle_summary_test.go`, `cmd/state_daemon_self_supervision_test.go` and `cmd/state_commit_now_test.go`; then every `CaptureStructure` call site in the test tree for the extra return.
- Edit the `state` row of CLAUDE.md's package table: the capture sentence's arity phrase moves 11 → 12 and names the trailing pending column, stating that the flag is returned beside the index rather than stored on the record.

**Acceptance Criteria**:
- [ ] A pane row whose twelfth field is `1` puts that pane's live pane key in the returned set; a row whose twelfth field is empty does not.
- [ ] The pending set is keyed exactly as the daemon's own per-pane loop keys a pane — `SanitizePaneKey(session, window index, pane index)` — for panes beyond the first window and the first pane.
- [ ] A row at the old eleven-field arity fails the parse with the existing `unexpected pane row field count` error and an empty `Sessions`, as does a thirteen-field row; twelve fields parse.
- [ ] Panes of a session that vanished between the enumeration and its `show-environment` read, or that failed anomalously, are absent from the pending set as they are from the index.
- [ ] Every early return (`ListSessionNames` failure, `ListAllPanesWithFormat` failure, parse failure, all-sessions-failed) returns an empty non-nil set alongside the empty index and the error.
- [ ] `sessions.json` written from an index whose panes were pending carries no pending field, and `SchemaVersion` is still 1 with no migration.
- [ ] `portal state commit-now` behaves exactly as before: it discards the second value, still passes a nil skip set, and still commits with `anyScrollbackChanged=false`.
- [ ] The daemon's tick summary, its per-pane `pane captured` DEBUG and every existing capture test pass unchanged against twelve-field fixtures.

**Tests**:
- `"it reports a pane carrying the marker in the pending set"`
- `"it reports a pane that has never been marked as unmarked"`
- `"it treats an empty pending column as unmarked"`
- `"it rejects a pane row at the old eleven-field arity"` (table: eleven, thirteen)
- `"it keys the pending set by the pane's live pane key"` (second window, second pane)
- `"it leaves a vanished session's panes out of the pending set"`
- `"it returns an empty pending set alongside a failed enumeration"` (table over the early returns)
- `"it writes no pending field to sessions.json and leaves the schema version at 1"`
- `"it hands commit-now a pending set it discards"` (commit-now's committed index unchanged)

**Edge Cases**:
- A pane that has never been marked reads unmarked — tmux reports an unset pane user-option as the empty string in a format, which is also what an empty-valued one reads as, so the column carries presence and nothing finer.
- A row at the old arity still fails the parse rather than short-reading: the fail-fatal on field count is what stops a format/parse mismatch from silently dropping the trailing columns, and it must keep failing for eleven fields now that eleven is the wrong number.
- The pending state reaches no persisted field and `SchemaVersion` does not move — it is recomputed on each boot from a registration that has not fired, and lives on the pane as a tmux option for as long as the pane does, and nowhere else.
- Every eleven-field pane-row fixture across `cmd` and `internal/state` moves to twelve; a fixture left at eleven now fails the parse, which is the intended tripwire rather than a test to relax.
- `commit-now`'s injected `CaptureStructure` seam type changes with the signature: the func field in `CommitNowDeps` and every test that injects it move together, or the injection silently stops compiling against the production default.
- A session whose panes are enumerated but whose environment read fails contributes no panes to the index, so it must contribute none to the pending set either — otherwise the daemon would skip a pane its index does not hold.

**Context**:
> The saver reads it for free. It already enumerates every pane on the server each tick with a per-pane format to build its structural index, and that format already carries the pane's durable token as a column. The pending marker joins it as another column rather than costing a second tmux call. The arity of that read changes — `captureFieldCount` 11 → 12 — which is the same contained move the pane token made.
>
> The pending state is never persisted: it is recomputed on each boot from a registration that has not fired, so nothing about it reaches `sessions.json` and no schema version moves.
>
> The structural capture returns the pending pane keys as a second value rather than carrying the flag on the saved pane record. A non-persisted field would still be compared against an index decoded from disk, where it is always false, so every tick would read as a structural change for as long as any pane waits. The cost is a mechanical extra return at the call sites, which the compiler finds.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.3, §8.2, §9.2

## lazy-resume-on-attach-2-3

### Task 2.3: A frozen pane's previous record is found by its durable token

**Problem**: A frozen pane keeps its place in the saved set because the structural capture merges its *previous* record back into the fresh index — and that merge is positional, matching on session name plus window and pane index. Over the few seconds the skeleton marker lives, nothing moves; over an indefinite wait, one rearrangement breaks it. The merge then finds nothing at the old address, the fresh record names a scrollback file nothing writes (the pane is skipped), and the housekeeping pass deletes the file that holds the pane's bytes because the fresh index no longer references it (`ComputeReferencedSet`, `internal/state/commit.go`). The pane restores empty, with no copy of its transcript anywhere. An unfrozen pane loses nothing to the same rearrangement — the next tick re-captures it under its new address a second later — so this loss exists only because the pane is frozen, and it runs for the whole of the wait.

**Solution**: A second merge inside `CaptureStructure`, driven by the pending column the capture itself read rather than by the caller's skip set, and taken from the live side: for each live pending pane, look its `@portal-pane-id` up in the previous index and carry that record's content onto the pane at its live address, so the record keeps pointing at the file that already holds its bytes.

**Outcome**: A pending pane that has been rearranged keeps the scrollback file its bytes are in and stays inside the referenced set the housekeeping pass computes; no previous record whose pane is not live is ever reintroduced; a second capture of an unchanged server produces an identical index.

**Do**:
- Add `mergeFrozenPanes(fresh *Index, prev Index, pending map[string]struct{})` to `internal/state/capture.go`, called from `CaptureStructure` after `mergeSkippedPanes` when `len(pending) > 0 && prev != nil`, so a pane carrying both markers ends on the token-matched record.
- Build the lookup over `prev` in canonical order (session name, then window index, then pane index), skipping empty tokens, and **consume** an entry when it is taken, so a token duplicated across previous records resolves to the first in that order and is used once.
- Walk the fresh index's panes; for each whose pane key is in `pending`, take the previous record's `CWD`, `CurrentCommand` and `ScrollbackFile` onto the live pane, leaving the live `Index` and `Active` — the pane's address and its liveness come from the enumeration, its content from the record.
- For a pending pane carrying no token, fall back to the previous record at the same session, window and pane address; a pending pane with no previous record either way keeps the fresh record the enumeration built.
- Leave `mergeSkippedPanes`, its live-structure guard and the positional key untouched: the frozen merge mutates panes the live enumeration returned and never appends a session, window or pane.
- Cover the merge in a new `internal/state/capture_frozen_merge_test.go`, and the housekeeping consequence (`ComputeReferencedSet` and `gcOrphanScrollback` over a rearranged frozen pane) in `internal/state/commit_test.go`.
- Add one sentence to the `state` row of CLAUDE.md's package table: a pending pane's previous record is matched by its durable token rather than its address, and the merged record keeps the scrollback path it was filed under.

**Acceptance Criteria**:
- [ ] A pending pane whose session, window index or pane index changed between captures keeps the `ScrollbackFile` of its previous record, and that path is in `ComputeReferencedSet` of the fresh index, so `gcOrphanScrollback` leaves the file on disk.
- [ ] The merged pane sits at its live address with the live `Index` and `Active`, and carries the previous record's `CWD`, `CurrentCommand` and `ScrollbackFile`.
- [ ] A pending pane carrying no token takes the previous record at its own address; one with neither a token match nor a previous record at its address keeps the fresh record, and its fresh `ScrollbackFile` names the file at its current address.
- [ ] A previous record whose pane is absent from the live enumeration is never added to the fresh index — covering a gone session, a gone window and a gone pane.
- [ ] A token held by two previous records resolves to the first in canonical order, and a second pending pane carrying the same token does not take that record again.
- [ ] The merge runs when the caller passed a nil skip set, so `portal state commit-now` gets it too.
- [ ] Two successive captures over an unchanged server, the first's index fed back as the second's `prev`, produce indexes equal once `SavedAt` is zeroed — so `structuralChange` reports none and the commit does not rewrite `sessions.json` every tick while a pane waits.
- [ ] The existing skeleton-marker merge behaves exactly as before for every case its suites cover.

**Tests**:
- `"it carries a frozen pane's previous record onto its new address"` (subtests: moved window, moved pane index, renamed session)
- `"it keeps the scrollback file the frozen pane's bytes are in after the pane moves"`
- `"it keeps that file in the referenced set so the housekeeping pass does not reclaim it"` (commit + gc)
- `"it takes the live address and the live active flag with the previous content"`
- `"it falls back to the positional match for a pending pane carrying no token"`
- `"it leaves a pending pane with no previous record on its fresh record"`
- `"it never resurrects a previous record whose pane is absent from the live enumeration"` (table: gone session, gone window, gone pane)
- `"it consumes a duplicated token once and resolves it deterministically"`
- `"it merges for a caller that passed no skip set"`
- `"it produces an identical index on a second capture of an unchanged server"`

**Edge Cases**:
- A pane whose address changed keeps the file its bytes are in, and the housekeeping pass keeps that file referenced — the two halves are one property: a record pointing at the old file is worth nothing if the gc pass deletes the file, so both are asserted.
- A pending pane carrying no token falls back to the positional match. In production a pane can only be waiting if it carries a registration, and registering one is what mints and stamps its token — but a hand-cleared option must degrade to today's behaviour rather than dropping the merge entirely.
- A previous record whose pane is absent from the live enumeration is never resurrected: driving the match from the live side is what preserves that guard without restating it, since a record with no live pane is simply never visited.
- A token matching more than one previous record resolves deterministically and is consumed once. One live pane answers to a token, so this is unreachable through Portal's own writers; canonical order plus consumption means a hand-stamped duplicate produces one defined outcome rather than map-iteration roulette.
- The merged record takes the live address and the previous content; `Active` stays live because a window has one active pane and a stale flag carried into a window that already has one would restore two (`activePanePosition`, `internal/restore/session.go`, takes the first).
- The merge runs off the capture's own column read so `commit-now`'s nil skip set does not opt out: `portal state commit-now` is fired by the session-closed tmux hook, and a skip-set-driven merge would let a commit-now landing after a rearrangement write a record naming a file that does not exist and let the housekeeping pass delete the one that does — the same loss by a second route.
- A stable merge does not make the structural comparison rewrite every tick: the merged index is what becomes the next tick's `prev`, so the fixed point is reached on the first tick after the pane is marked and the comparison finds nothing thereafter.

**Context**:
> **Corrigendum 2026-09-19**: a frozen pane's previous record is matched on the pane's durable `@portal-pane-id` and the merged record keeps pointing at the file that already holds its bytes. Matched positionally, as every unfrozen pane's record is, a frozen pane loses its transcript permanently the first time tmux moves it: the merge finds nothing at the old address, the fresh record names a file nothing writes because the pane is skipped, and the housekeeping pass deletes the old file because the fresh index no longer references it. The pane then restores empty.
>
> The token match costs nothing and introduces no new state. A pane can only be waiting if it carries a registration, and registering one is what mints and stamps its token, so every pane this rule reaches is already stamped. One live pane answers to a token, so there is no collision to arbitrate, and the match is taken only against panes found in the same enumeration, which is what preserves the existing guard against resurrecting a pane whose session, window or pane is genuinely gone.
>
> Nothing downstream moves: restore already replays from the path the record stores and the housekeeping pass already builds its reachable set from those stored paths verbatim, so the file keeping its original name is invisible to both. When the wait ends the pane returns to ordinary capture and is re-filed under its current address, and the old file falls out of reference and is reclaimed on the next commit.
>
> One consequence is accepted rather than closed: the picker's scrollback preview shows nothing for a waiting pane that has been rearranged, because it resolves a saved transcript from the pane's live position rather than from the record. The transcript is intact and restores correctly; only the preview is blank, and it corrects itself the moment the pane resumes. Closing it belongs with the wider migration off positional pane keys, parked as `durable-pane-identity`. Nothing in this task addresses the preview.
>
> The specification states which record is carried forward and which file it points at; it does not enumerate the fields. This task's call is content from the previous record (`CWD`, `CurrentCommand`, `ScrollbackFile`) and address from the live enumeration (`Index`, `Active`), for the reason given in the edge cases.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.2, §7.1, §10, Corrigendum 2026-09-19

## lazy-resume-on-attach-2-4

### Task 2.4: The saver's skip gains its second condition

**Problem**: The saver re-reads every live pane on each tick and rewrites that pane's saved scrollback whenever the capture hashes differently from the last write — a dedup, not a protection. The only thing that keeps it off a pane is the mid-restore skeleton marker, which the hydrate helper clears the moment replay finishes, which under lazy resume is exactly when the panel goes up. Measured against the saver's own invocation (`capture-pane -e -p -S -`, tmux 3.7c) over a pane with a live process holding the alternate screen open: the capture returns only the lines that had already scrolled out of view, then the card — the most recent screenful is absent. So the first tick that lands on an unfrozen waiting pane rewrites its saved transcript as history-minus-its-last-screenful plus a picture of the card, and that screenful is the part the user was last reading, with no copy anywhere.

**Solution**: The per-pane skip gains a second condition — a pane is left alone when it is mid-restore (the existing positional marker, whose other jobs are unchanged) **or** when it is pending, read from the set the capture already returned. Nothing else about the tick moves: same commit, same structural index, same log events and attrs.

**Outcome**: A pane carrying the pending marker is never capture-paned and never rewrites its scrollback file for as long as the marker stands; a pane carrying neither marker is captured exactly as today; clearing the marker returns the pane to ordinary capture on the next tick.

**Do**:
- In `cmd/state_daemon.go`'s `captureAndCommit`, bind the pending set returned by `state.CaptureStructure` and extend the existing `skipSet` membership check so a pane in either set `continue`s — before the `panes++` counter, before the `pane captured` DEBUG, and before `CaptureAndHashPane` and `WriteScrollbackIfChanged`.
- Leave the tick-summary emission untouched: same message, same `sessions` / `panes` / `natural_churn` / `anomalous` / `took` attrs, no new event and no new attr key for a skipped pane.
- Leave `cmd/state_commit_now.go` as the previous task left it — it writes no scrollback bytes, so it needs no skip of its own.
- Add `cmd/state_daemon_resume_pending_test.go` staging pending panes through `daemonFakeCommander.panesOut`'s twelfth column, asserting through `callsContaining("capture-pane")`, the scrollback directory's contents and a `logtest.Sink`.
- Add one sentence to the `state` row of CLAUDE.md's package table: the saver's per-pane skip is mid-restore **or** pending, the second read from the capture's own column.

**Acceptance Criteria**:
- [ ] A pane carrying neither marker is captured exactly as today: one `capture-pane` for it, its scrollback file written, its `pane captured` DEBUG emitted and the tick summary's `panes` count including it.
- [ ] A pane carrying the pending marker gets no `capture-pane` call and no scrollback write, while its live sibling in the same window is captured normally in the same tick.
- [ ] A pane carrying both the skeleton and the pending marker is skipped once — one `continue`, no `capture-pane`, no duplicate or double-counted anything.
- [ ] A skipped pane is absent from the tick summary's `panes` count and emits no per-pane capture breadcrumb.
- [ ] With the marker cleared, the next tick captures that pane again and writes its scrollback under its current address.
- [ ] The shutdown flush inherits the same skip, because it runs `captureAndCommit`: a pending pane is not captured by the final flush either.
- [ ] `portal state commit-now` is unchanged and still needs no skip of its own.
- [ ] The set of log messages and attr keys emitted across a tick holding a pending pane is exactly the set emitted by a tick without one, minus that pane's per-pane breadcrumb.

**Tests**:
- `"it captures a pane carrying neither marker exactly as today"`
- `"it skips a pane carrying the pending marker and captures its live sibling"`
- `"it skips a pane carrying both markers once"`
- `"it leaves a skipped pane out of the tick summary pane count and emits no capture breadcrumb"`
- `"it returns a pane to ordinary capture and re-files it at its current address once the marker clears"`
- `"it applies the same skip to the shutdown flush"`
- `"it introduces no new log event or attr key"`

**Edge Cases**:
- A pane carrying neither marker is captured exactly as today — the second condition is additive and must not change the first; the existing skeleton-marker suites stay green unmodified.
- A pane carrying both markers is skipped once: the two conditions are a single `continue`, not two, so nothing is counted twice and no breadcrumb is emitted twice.
- A skipped pane is absent from the tick summary's pane count and emits no per-pane capture breadcrumb — the skip sits ahead of both, exactly where the skeleton skip already sits.
- A cleared marker returns the pane to ordinary capture and re-files it at its current address on the next tick, at which point the file it was frozen under falls out of reference and is reclaimed by the commit's housekeeping pass.
- The shutdown flush inherits the same skip by construction (it calls `captureAndCommit`); a flush that captured a waiting pane would write the truncated-plus-card transcript as the last thing before the daemon dies, which is the worst possible moment for it.
- `commit-now` writes no scrollback and needs no skip of its own; it does get the token-matched merge, which is the previous task's guarantee.
- No new log event or attr key is introduced: the log taxonomy is closed and spec-governed, and a skip is the absence of work rather than an event.

**Context**:
> Portal's saver re-reads every live pane on each tick and rewrites that pane's saved scrollback whenever the capture hashes differently from the last write — `CaptureAndHashPane` then `WriteScrollbackIfChanged`, which is a dedup, not a protection.
>
> Measured against the saver's exact invocation on tmux 3.7c, with a live process holding the alternate screen open over a pane carrying 40 lines of prior output: the capture returns only the lines that had already scrolled out of view, then the card — the most recent screenful of real output is absent. Those lines are still in the buffer `capture-pane -a -p` reads, which the saver never calls. So an unfrozen waiting pane hashes differently from its last write and the saver rewrites its saved file as transcript-minus-its-last-screenful plus a picture of the card. Once, not per tick — identical captures dedup. A pane waited on across three reboots loses a screenful each time and accretes three dead cards it can never shed.
>
> The saver's skip gains a second condition rather than changing its first. A pane is left alone when it is mid-restore — the existing positional marker, whose other jobs are unchanged — **or** when it is waiting, read from a pane-scoped pending marker.
>
> The cost is nil in practice: a pane waiting on a resume has no new content worth saving, so freezing it at its last live state is exactly the desired end state. The freeze suppresses that pane's scrollback write alone — the structural capture still enumerates the pane into `sessions.json`.
>
> The two clears are the user's answers, both in the waiting program and both after the pane has left the panel's screen. Neither exists yet: this task delivers the skip the clears will lift.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.1, §7.2, §7.3, §9.1

## lazy-resume-on-attach-2-5

### Task 2.5: The marker and the freeze travel with the pane

**Problem**: Everything the freeze rests on has so far been verified against a fake commander, but the property that makes a pane-scoped marker the right answer is a claim about tmux — that a pane user-option survives every rearrangement tmux can perform — and it is precisely the property the positional marker fails. `resume-hooks-silently-lost` measured that for the pane token and this feature reuses the conclusion, but it applies it to a different option, over a wait that lasts weeks rather than the few seconds between skeleton restore and handover, and to a second thing the token now protects: the saved record that keeps a frozen pane's transcript findable. A regression here is silent — the marker stops matching, the saver captures, and the user's transcript is truncated with a dead card on the end.

**Solution**: A real-tmux suite in the unit lane, on a disposable socket, that stamps the pending marker on one pane and, after each rearrangement in turn, re-reads through Portal's own capture — asserting the pane is still reported pending and its record still names the scrollback file its bytes were filed under.

**Outcome**: Each of `break-pane`, `move-pane` within and across sessions, a window close under `renumber-windows on`, `respawn-pane -k` and a session rename is proven not to break the marker or orphan the frozen pane's record, against real tmux, with no daemon and no built binary.

**Do**:
- Add `Socket.MarkResumePending(t, target)` and `Socket.ReadResumePending(t, target)` to `internal/tmuxtest/stamp.go`, beside `StampPaneToken` / `ReadPaneToken` and built the same way — raw tmux against the fixture's socket, composing the option name from `state.ResumePendingOption`, so the fixture never stages itself through the code under test.
- Add `internal/state/resume_pending_durability_realtmux_test.go` (`package state_test`, `tmuxtest.SkipIfNoTmux`, its own socket prefix, e.g. `ptl-pending-`): seed a server with `renumber-windows on` set explicitly, a subject session of three windows with two panes in one of them, plus a second session as a cross-session move destination.
- Take a baseline `state.CaptureStructure(client, nil, nil, nil)` **before** marking the pane, and keep its index as `prev` and the subject pane's `ScrollbackFile` as the path every later assertion compares against; then stamp the pane's token and the pending marker.
- Drive each rearrangement in its own subtest in declaration order, each starting where the last left the pane: `break-pane`, `kill-window` of an earlier window, `move-pane` back, `move-pane` to the other session, `respawn-pane -k`, `rename-session`. After each, capture with the previous capture's index as `prev` and assert (a) the pane's current pane key is in the returned pending set, (b) its record's `ScrollbackFile` is still the baseline path, and (c) the pane genuinely moved (its key or session changed — except `respawn-pane -k`, where the pane deliberately does not move and the assertion is that the marker survived the process being replaced).
- Assert through Portal's own reads — `state.CaptureStructure` and the client — and use raw tmux only to perform the rearrangements and to stage the fixture.
- Keep the suite in the unit lane: no `portaltest` env, no daemon, no built binary, no `t.Parallel`, and `tmuxtest.New`'s own cleanup kills the server.

**Acceptance Criteria**:
- [ ] After each of `break-pane`, a `kill-window` of an earlier window under `renumber-windows on`, `move-pane` back into the original window, `move-pane` into another session, `respawn-pane -k` and `rename-session`, the subject pane is still reported in `CaptureStructure`'s pending set under its current pane key.
- [ ] After each of those, the subject pane's record still carries the baseline `ScrollbackFile` — the file its bytes were filed under before it was marked.
- [ ] Each assertion runs immediately after its own move, so a failure names the operation that broke the marker rather than only the end state.
- [ ] `renumber-windows on` is set explicitly on the fixture server, and the window-close subtest asserts the surviving window index actually changed — otherwise the case exercises no renumbering.
- [ ] The session-rename subtest asserts both halves: the pane key's session component changed, and the record still names the file under the old name.
- [ ] Exactly one live pane carries the token and the marker after every move; no rearrangement duplicates either.
- [ ] The suite runs in the unit lane (`go test ./internal/state`), skips cleanly where tmux is absent, spawns no daemon, builds no binary, and leaves no server behind.

**Tests**:
- `"it keeps the marker and the frozen record when the pane is broken out to its own window"`
- `"it keeps them when an earlier window is closed under renumber-windows on"`
- `"it keeps them when the pane is moved back"`
- `"it keeps them when the pane is moved to another session"`
- `"it keeps them when the pane is respawned with -k"`
- `"it keeps them when the session is renamed"`
- `"it reports exactly one pending pane after every move"`

**Edge Cases**:
- Each of the six rearrangements is asserted after its own move rather than only at the end: a suite that checked once at the end would pass with five of them broken as long as the last one restored a match.
- `renumber-windows on` is set explicitly on the fixture server — it is off in vanilla tmux, so without it `kill-window` leaves the surviving indices alone and the renumbering case proves nothing.
- The frozen pane's record still names the file holding its bytes after a rename: the rename changes the session half of every pane key, so this is the case the positional match fails outright and the one that would silently delete a transcript in production.
- `respawn-pane -k` does not move the pane; the case is about the marker surviving the pane's process being replaced, which is the hand-respawn route the design names as the one open way a marker can outlive its waiter.
- The assertions run through Portal's own capture read rather than raw tmux, because the property under test is what Portal concludes about the pane, not what tmux would say to a differently-composed query.
- No scrollback file is written by this suite — no daemon runs — so the assertion is about the path the record names, not about bytes on disk.
- Unit lane on a disposable socket with no daemon and no built binary: a test that builds the portal binary or spawns `portal state daemon` belongs behind the integration tag, and this one needs neither.

**Context**:
> The marker travels with the pane through every rearrangement tmux can perform, measured in `resume-hooks-silently-lost` against `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename.
>
> The marker that suppresses capture today is addressed positionally, and all three components of that address move: closing an earlier window renumbers, `break-pane` and `move-pane` relocate, and a rename changes the session half. A pane waited on for a week, whose session is renamed or whose sibling window is closed, would lose its protection the moment the address stopped matching — the saver recomputes it every tick and the capture lands. Nothing announces it.
>
> A frozen pane's previous record is matched on the pane's durable `@portal-pane-id` and the merged record keeps pointing at the file that already holds its bytes, whatever the pane's address has become. Matched positionally, a frozen pane loses its transcript permanently the first time tmux moves it, and the housekeeping pass deletes the file because the fresh index no longer references it.
>
> The inverse failure — a marker wrongly left set — is reachable through a pane respawned out from under its waiter by hand: the user destroying the process that held that pane's state, in the same class as a hand edit of the store. The `respawn-pane -k` case here measures the marker's survival of that operation; what the system does about the state it leaves is Phase 4's concern.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.3, §7.2, §8.2

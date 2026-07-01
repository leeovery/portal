---
phase: 3
phase_name: Cross-Reboot Persistence — schema, capture, restore re-stamp + baking
total: 7
---

## session-rename-orphans-resume-hook-3-1 | approved

### Task session-rename-orphans-resume-hook-3-1: Add `PortalID` field to `state.Session` schema (`json:"portal_id"`, tolerant decode)

**Problem**: tmux user-options are in-memory server state and do not survive a reboot. For the headline case — a session renamed *then* rebooted — the saved `Session.Name` is the post-rename name, but the hook was registered under the immutable `@portal-id`; restore must recover that id to bake the matching `--hook-key`. The id is unrecoverable after rename unless Portal persists it itself. `state.Session` (`internal/state/schema.go`) has no field to carry `@portal-id` across the reboot gap, so there is nowhere in `sessions.json` for the persisted id to ride.

**Solution**: Add a single additive, optional field `PortalID string` tagged `json:"portal_id"` to `state.Session`. It is populated by capture (Task 3-2) and consumed by restore (Tasks 3-3/3-4). The field is forward- and backward-compatible: an old `sessions.json` with no `portal_id` decodes to `""` via the existing tolerant `json.Unmarshal` behaviour, and an older binary silently ignores the unknown field — so there is NO schema `Version` bump and NO `sessions.json` migration.

**Outcome**: `state.Session` carries a `PortalID string` field that encode/decode round-trips faithfully; an existing `sessions.json` lacking `portal_id` decodes to `PortalID == ""` with no error and no version change — the persistence slot the rest of Phase 3 fills and reads.

**Do**:
- In `internal/state/schema.go`, add `PortalID string \`json:"portal_id"\`` to the `Session` struct (~lines 25-29). Placement within the struct is cosmetic; a sensible spot is immediately after `Name` (it is a session-scoped identity attribute), but anywhere in the struct is acceptable — the JSON tag, not field order, governs the on-disk shape.
- Update the `Session` struct doc-comment (~line 24) to note it now also carries the immutable `@portal-id` (`portal_id`), persisted so a renamed session's hook key survives a reboot; an absent field decodes to `""` (legacy / un-stamped → name fallback at restore).
- Do NOT bump `SchemaVersion` (stays `1`) and do NOT add any migration branch to `DecodeIndex`. The field is additive/optional — the tolerant-decode behaviour is inherited from `json.Unmarshal` (unknown fields ignored on read; a missing field leaves the zero value `""`), exactly as the "Unknown fields are silently ignored" note on `DecodeIndex` (~lines 98-99) already documents.
- Do NOT change `Canonicalize` — it normalises nil slices/maps only; a `string` zero value (`""`) needs no canonicalisation and must NOT be defaulted to anything other than the empty string.
- Extend `internal/state/schema_test.go` (package `state_test`). Add `PortalID` to the fully-populated fixture in `TestEncodeDecodeIndex_RoundTripsFullyPopulatedIndex` (~line 77) so the round-trip asserts the id survives encode→decode. Add a new decode test seeding a `sessions.json` byte payload with a valid `version` but NO `portal_id` on its session(s), and assert `DecodeIndex` succeeds with `Session.PortalID == ""` (tolerant decode, no error).

**Acceptance Criteria**:
- [ ] `state.Session` has an exported field `PortalID string` with the struct tag `json:"portal_id"`.
- [ ] `EncodeIndex` → `DecodeIndex` round-trip preserves a non-empty `PortalID` byte-for-byte (a session encoded with `PortalID: "tok123"` decodes back to `PortalID: "tok123"`).
- [ ] A `sessions.json` payload with a valid `version` and NO `portal_id` key decodes without error and yields `Session.PortalID == ""` (tolerant decode — the missing field leaves the zero value).
- [ ] `SchemaVersion` is unchanged (still `1`); `DecodeIndex` has no new migration/version branch for `portal_id`.
- [ ] `Canonicalize` is unchanged with respect to `PortalID` (an empty string is left empty — not rewritten).
- [ ] Forward-compatible: an encoded payload that DOES carry `portal_id` still decodes under a decoder that does not know the field would ignore it (this is the default `json.Unmarshal` behaviour — assert by decoding a payload with an extra unknown sibling field alongside `portal_id` succeeds, or rely on the documented unknown-field tolerance; no code change needed).
- [ ] `go build -o portal .` succeeds; `go test ./internal/state/...` passes.

**Tests** (`internal/state/schema_test.go`, package `state_test`, NO `t.Parallel()`):
- `"it round-trips a non-empty PortalID through encode and decode"` — extend `TestEncodeDecodeIndex_RoundTripsFullyPopulatedIndex` (or add a sibling) so the fixture session sets `PortalID: "tok123"` and the decoded value equals it.
- `"it decodes a sessions.json without portal_id to an empty PortalID"` — hand-craft a minimal valid `sessions.json` byte slice (`version: 1`, one session with `name`/`windows` but no `portal_id`), call `DecodeIndex`, assert no error and `Sessions[0].PortalID == ""`.
- `"it does not bump the schema version for the additive field"` — assert `state.SchemaVersion == 1` (guards against an accidental bump) and that a `version: 1` payload carrying `portal_id` decodes without a version error.

**Edge Cases**:
- `sessions.json` without `portal_id` → `PortalID == ""` (tolerant decode, no error) — the legacy / un-stamped path that falls back to the name-based key at restore.
- Older binary reading a `portal_id`-bearing payload → ignores the unknown field (forward-compatible; default `json.Unmarshal` behaviour, no code needed).
- No schema `Version` bump and no migration — the field is additive/optional (spec § Cross-Reboot Persistence → 1. Schema).
- Empty string is a first-class valid value (not a "missing" sentinel that needs canonicalising).

**Context**:
> Spec § Cross-Reboot Persistence of `@portal-id` → 1. Schema (`internal/state/schema.go`): Add one field to `state.Session`: `PortalID string \`json:"portal_id"\``. Additive and optional: an old `sessions.json` with no `portal_id` decodes to `""` (tolerant decode, same as other optional fields); a new binary reading it falls back to the session name. No schema `Version` bump and no `sessions.json` migration. Forward-compatible too — an older binary ignores the unknown field.
> The existing `DecodeIndex` doc-comment (`schema.go` ~lines 98-99) already states: "Unknown fields are silently ignored — that is the default json.Unmarshal behaviour and is desirable for forward compatibility with future writers." The new field relies on exactly this contract; no decoder change is required.
> `Canonicalize` (`schema.go` ~lines 62-83) normalises nil `Sessions`/`Environment`/`Windows`/`Panes` only — it does not touch scalar string fields, so `PortalID` needs no canonicalisation entry.
> This task adds the persistence slot only. Populating it from tmux is Task 3-2; re-stamping and baking from it are Tasks 3-3 / 3-4.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Cross-Reboot Persistence of `@portal-id` → 1. Schema; § Acceptance Criteria 5 (No-migration upgrade — no `sessions.json` migration and no schema `Version` bump); § Testing Requirements → Creation & persistence (component) → tolerant decode.

## session-rename-orphans-resume-hook-3-2 | approved

### Task session-rename-orphans-resume-hook-3-2: Extend capture to read `#{@portal-id}` into `Session.PortalID` (append column, bump `captureFieldCount` 10→11, parser lockstep)

**Problem**: The `sessions.json` snapshot is the only durable record of a session's `@portal-id` across a reboot, but capture (`internal/state/capture.go`) does not read the option — it captures only the 10 name/window/pane fields in `captureFormat`. Without lifting `@portal-id` into the new `Session.PortalID` field (Task 3-1), every captured session persists `PortalID == ""`, so a renamed session's saved snapshot cannot recover the immutable id and restore falls back to the (wrong, post-rename) name — the exact reboot gap.

**Solution**: Extend the fixed-arity `captureFormat` by appending `#{@portal-id}` as the LAST column, bump the paired `const captureFieldCount` from `10` to `11`, parse the new trailing column into a `portalID` field on `paneRow`, and lift `Session.PortalID` from the FIRST row of `grouped[name]` when assembling each session. This is deliberately ONE task: `captureFormat`, `captureFieldCount`, and the parser's field-index reads are a fixed-arity contract that must change in lockstep, or every captured row is rejected (arity mismatch) or mis-slotted.

**Outcome**: Capture reads each session's `@portal-id` and populates `Session.PortalID`; a stamped session captures its id, an un-stamped one captures `""`, every existing field index is unchanged (append-only), and a wrong-arity row is still rejected — so `sessions.json` durably records the id for restore to bake and re-stamp.

**Do**:
- In `internal/state/capture.go`, append `|||#{@portal-id}` to the end of the `captureFormat` const string (~line 35) — it becomes the 11th and final `|||`-separated field. Appending (not inserting) keeps every existing field index (`session_name`=0 … `pane_current_command`=9) unchanged; the new column is index 10. Update the `captureFormat` doc-comment to note the trailing `@portal-id` (session-scoped, resolves per-pane to the owning session's value) and that the token is alphanumeric so it cannot contain the `|||` delimiter (trailing position is not required for delimiter safety, unlike `@portal-dir` in `ListSessions`, but is the lowest-risk placement).
- Bump `const captureFieldCount = 10` → `11` (~line 37). This is the arity `parsePaneRow` length-validates against — it MUST move in the same commit as the format change.
- Add a `portalID string` field to the `paneRow` struct (~lines 319-330). Doc-note it as session-scoped (repeated identically on every pane row of the same session), consumed from the first row when assembling the `Session` — mirroring how the window-level fields are described as "repeated for every pane in the same window; the consumer takes them from the first row".
- In `parsePaneRow` (~lines 359-384), read the new trailing column: set `portalID: parts[10]` in the returned `paneRow`. Do NOT change the reads of `parts[0]`..`parts[9]` — they are unchanged by the append. The existing `len(parts) != captureFieldCount` guard now enforces arity 11 automatically via the bumped constant.
- In the `Session{...}` assembly (`CaptureStructure` ~lines 125-129), set `PortalID` from the first row of `grouped[name]`. Because `grouped[name]` can be empty (zero-row session — see edge case), guard the lift: e.g. `portalID := ""; if rows := grouped[name]; len(rows) > 0 { portalID = rows[0].portalID }`, then set `PortalID: portalID` on the `Session` literal alongside `Name`/`Environment`/`Windows`. Every pane row of a session carries the same session-scoped `@portal-id`, so the first row is canonical (same "first row is canonical" pattern `buildWindows` uses for window-level fields).
- Do NOT add a guard/skip for the zero-row session: an entry with no pane rows yields `PortalID == ""` AND empty `Windows`, and is ALREADY rejected by `Restore` (`internal/restore/session.go` ~line 87 errors on a session with no windows/panes), so the empty id is never consumed. No new rejection path is warranted.
- Extend `internal/state/capture_test.go` (package `state_test`). The existing capture tests feed a `CaptureClient` mock whose `ListAllPanesWithFormat` returns canned `|||`-joined rows — update those canned rows to include the new trailing `@portal-id` column (append `|||<id>` or `|||` for un-stamped), and add assertions on `Session.PortalID`. Add a wrong-arity test (a row with 10 fields under the new 11-arity contract) asserting `parsePaneRow`/`parsePaneRows` returns the "unexpected pane row field count" error.

**Acceptance Criteria**:
- [ ] `captureFormat` ends with `|||#{@portal-id}` and has exactly 11 `|||`-separated fields; fields 0-9 are byte-identical to before (append-only — no existing index moved).
- [ ] `captureFieldCount == 11`.
- [ ] `paneRow` has a `portalID string` field, populated from `parts[10]` in `parsePaneRow`.
- [ ] A stamped session captures `Session.PortalID == "<id>"` (the value tmux resolved for `#{@portal-id}` on every pane row of that session).
- [ ] An un-stamped session captures `Session.PortalID == ""` (`#{@portal-id}` resolves empty; the trailing field is the empty string).
- [ ] `Session.PortalID` is lifted from the FIRST row of `grouped[name]`; a multi-pane session whose rows all carry the same id yields that id (session-scoped value, taken once).
- [ ] A wrong-arity row (10 or 12 fields) is rejected by `parsePaneRow` with the existing "unexpected pane row field count %d" error — capture aborts rather than mis-slotting.
- [ ] A zero-row session (present in `keep`, contributing no rows to `grouped`) yields `PortalID == ""` and empty `Windows`, and is not specially guarded (already rejected downstream by `Restore`); `CaptureStructure` does not panic on the empty lift.
- [ ] `go build -o portal .` succeeds; `go test ./internal/state/...` passes.

**Tests** (`internal/state/capture_test.go`, package `state_test`, NO `t.Parallel()`):
- `"it captures a stamped session's @portal-id into Session.PortalID"` — canned pane rows with a trailing `tok123` column; assert `Sessions[i].PortalID == "tok123"`.
- `"it captures an un-stamped session as an empty PortalID"` — canned rows with an empty trailing column (`...|||`); assert `PortalID == ""`.
- `"it lifts PortalID from the first pane row for a multi-pane session"` — multiple rows for one session all carrying `tok123`; assert the assembled session's `PortalID == "tok123"` (taken once, not concatenated/duplicated).
- `"it rejects a wrong-arity pane row after the field-count bump"` — a 10-field row under the 11-arity contract; assert `parsePaneRow` (or `parsePaneRows`) returns the "unexpected pane row field count" error and `CaptureStructure` surfaces it.
- `"it leaves every existing field index unchanged after the append"` — a canned 11-field row; assert `session`/`windowIdx`/`windowName`/`layout`/`zoomed`/`windowActive`/`paneIdx`/`cwd`/`paneActive`/`currentCommand` all parse to the same values they did pre-append (guards the append-only invariant).
- `"it yields an empty PortalID and no windows for a zero-row session without panicking"` — a session in `keep` (via `ListSessionNames` + `ShowEnvironment`) that contributes no pane rows to `grouped`; assert `PortalID == ""`, empty `Windows`, no panic. (This is benign per spec — downstream `Restore` rejects it.)

**Edge Cases**:
- Un-stamped session → `#{@portal-id}` resolves empty → `PortalID == ""` (legacy / name-fallback path).
- Zero-row session (killed between name enumeration and the pane read — natural churn) → `PortalID == ""` and empty `Windows`; benign, already rejected by `Restore`, so NO guard is added (spec § Cross-Reboot Persistence → 2. Capture → Zero-row session).
- Wrong-arity row → rejected by the `len(parts) != captureFieldCount` guard (now 11) — capture aborts rather than committing an inconsistent index.
- Multi-pane session → every row carries the same session-scoped `@portal-id`; the first row is canonical (spec: "every pane row of a session carries the same session-scoped value").
- The opaque token is alphanumeric and cannot contain `|||`, so trailing position is safe even without the delimiter-safety argument that forced `@portal-dir` to trail in `ListSessions`.

**Context**:
> Spec § Cross-Reboot Persistence of `@portal-id` → 2. Capture (`internal/state/capture.go`): Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it. `#{@portal-id}` resolves per-pane to the owning session's option value, so it is present on every pane row for that session; the parser takes it when assembling the session. A legacy/un-stamped session captures `PortalID == ""`. (The opaque token is alphanumeric, so it cannot contain the `|||` field delimiter.)
> `captureFormat` is fixed-arity: it is paired with `const captureFieldCount` (currently `10`) that the row parser length-validates against. Adding the field is not free-form — `captureFieldCount` must bump to `11` and the parser's field-index reads must update in lockstep, or every captured row is rejected/mis-slotted. Append `#{@portal-id}` as the last column so every existing field index is unchanged — the only reads that move are the new trailing index plus the count bump (lowest-risk placement).
> Assembly path: `parsePaneRows(raw, keep)` returns `grouped map[string][]paneRow`; each `Session` is assembled as `Session{Name, Environment, Windows: buildWindows(name, grouped[name])}`. Parse the new column into a `portalID` field on `paneRow`, and lift `Session.PortalID` from the FIRST row of `grouped[name]`.
> Zero-row session (edge case, benign): a session present in `keep` but contributing no pane rows to `grouped` (killed between the name enumeration and the pane read) yields `PortalID == ""` and empty `Windows`. No guard is needed: such an entry has no windows/panes and is already rejected by `Restore` (which errors on a session with no windows/panes), so the empty id is never consumed.
> This must land as ONE task — splitting the format-column append from the count bump leaves the parser rejecting every row mid-phase.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Cross-Reboot Persistence of `@portal-id` → 2. Capture; § Testing Requirements → Creation & persistence (component) → "Capture reads `#{@portal-id}` into `Session.PortalID`; an un-stamped session captures `\"\"`".

## session-rename-orphans-resume-hook-3-3 | approved

### Task session-rename-orphans-resume-hook-3-3: Re-stamp `@portal-id` in `createSkeleton` from the saved value (best-effort after `NewSessionWithCommand`)

**Problem**: `sessions.json` is a snapshot the daemon continuously regenerates from LIVE tmux state, not a store of record. When restore recreates a session it does not currently re-stamp `@portal-id` on the new live session, so the id resolves correctly only for the first resume (baked from the saved snapshot, Task 3-4) and is then lost three ways: (a) the first post-restore capture (~1s later) rewrites `sessions.json` from live state and, with no live `@portal-id`, captures `""` — erasing the id so the next reboot resurrects a bare shell; (b) post-restore stale-cleanup (bootstrap step 11) builds its live-key set from the live `@portal-id`, and with it absent the restored session's live key falls back to the name, no longer matching the id-keyed `hooks.json` entry, so cleanup deletes the hook that just fired — in the same bootstrap; (c) a subsequent rename of the restored session is only stable while the live id is present.

**Solution**: In `createSkeleton`, immediately after `NewSessionWithCommand(sess.Name, …)` recreates the session, re-stamp the saved id best-effort — `if sess.PortalID != "" { _ = r.Client.SetSessionOption(sess.Name, session.PortalIDOption, sess.PortalID) }` — mirroring creation-time stamping. Skip the stamp when the saved value is empty (legacy session → left un-stamped → name fallback, exactly as before). This adds an `internal/session` import to `internal/restore/session.go` for the `PortalIDOption` constant (cycle-free: `internal/session` imports only `internal/tmux`).

**Outcome**: After restore, the recreated live session carries its saved `@portal-id`, so the next capture re-persists the id, post-restore cleanup keeps the just-fired hook, and future renames stay stable; an empty saved id leaves the session un-stamped (name fallback), and a `SetSessionOption` failure is swallowed and never aborts restore.

**Do**:
- In `internal/restore/session.go`, add `"github.com/leeovery/portal/internal/session"` to the import block (~lines 25-36). This is the FIRST import of `internal/session` into `internal/restore`; it is cycle-free because `internal/session` imports only `internal/tmux` (verified — it imports neither `internal/restore` nor `internal/state`). Use `session.PortalIDOption` for the option name so it stays byte-identical to the creation-time stamp and to the `@portal-id` literal in `tmux.HookKeyFormat`.
- In `createSkeleton` (~lines 128-163), immediately AFTER the successful `r.Client.NewSessionWithCommand(sess.Name, rootCWD, "")` call (~line 130) and BEFORE `r.applyEnvironment(sess)`, add:
  ```go
  if sess.PortalID != "" {
      _ = r.Client.SetSessionOption(sess.Name, session.PortalIDOption, sess.PortalID)
  }
  ```
  Placement after `NewSessionWithCommand` (the session now exists to be stamped) and before the rest of skeleton creation keeps it adjacent to the create call and well ahead of `armPanes` (in `Restore`, `createSkeleton` runs before `armPanes` — order preserved). `SetSessionOption` targets the session by name; the id rides the session object, so this is a one-shot stamp on the root.
- Swallow the `SetSessionOption` error (`_ =`) — best-effort, mirroring `CreateFromDir`'s `@portal-dir` / `@portal-id` stamps. A stamp failure must NOT abort restore (do not `return err`); the session is created and armed regardless — it simply falls back to the name-based key on later live reads, like a legacy session.
- Skip the stamp entirely when `sess.PortalID == ""` (the `if` guard). A legacy/un-stamped saved session must be left un-stamped so it falls through to the name-based key, exactly as before — do NOT stamp an empty string.
- Do NOT change `armPanes`, `collectArmInfos` (that is Task 3-4), the firing path, or the create ordering. This task only re-seeds the live `@portal-id`; the hook key baked into `--hook-key` comes from SAVED state and does not depend on this re-stamp (the ordering trap — see Task 3-4).
- Extend `internal/restore/session_test.go` (package `restore_test`). The `mockCommander` records every tmux call in `mock.Calls`; add assertions that a `set-option`-shaped call carrying `@portal-id <savedID>` is recorded on the session name after `new-session` when `sess.PortalID != ""`, and is ABSENT when `sess.PortalID == ""`. Add a case where `SetSessionOption` (the `set-option` call) returns an error via `RunFunc` and assert `Restore` still succeeds (error swallowed). Reuse the existing `restoreRunFunc("0:0")` list-panes oracle so the arm phase completes.

**Acceptance Criteria**:
- [ ] `internal/restore/session.go` imports `internal/session` and references `session.PortalIDOption` (byte-identical to the creation-time constant).
- [ ] On a saved session with `PortalID != ""`, `createSkeleton` issues a `SetSessionOption(sess.Name, session.PortalIDOption, sess.PortalID)` (a `set-option ... @portal-id <savedID>` call targeting the session) immediately after `NewSessionWithCommand` and before `applyEnvironment`.
- [ ] On a saved session with `PortalID == ""`, NO `@portal-id` `set-option` call is issued (the stamp is skipped).
- [ ] A `SetSessionOption` failure is swallowed: `Restore` still returns no error (restore not aborted) and the arm phase still runs.
- [ ] The re-stamp precedes `armPanes` (create-phase ordering preserved: `collectArmInfos` → `createSkeleton` [incl. re-stamp] → `armPanes`).
- [ ] `NewSessionWithCommand`, `applyEnvironment`, `armPanes`, `collectArmInfos`, and the firing path are otherwise unchanged.
- [ ] `go build -o portal .` succeeds; `go test ./internal/restore/...` passes.

**Tests** (`internal/restore/session_test.go`, package `restore_test`, NO `t.Parallel()`):
- `"it re-stamps @portal-id on the recreated session from the saved value"` — `newSession` with `PortalID: "tok123"` (extend the `newSession` helper or set the field on the returned `state.Session`); run `Restore`; assert `mock.Calls` contains a `set-option`-shaped call with `@portal-id` and `tok123` targeting `work`.
- `"it skips the re-stamp when the saved PortalID is empty"` — `PortalID: ""`; assert NO `@portal-id` `set-option` call is recorded (a legacy session stays un-stamped).
- `"it succeeds when the @portal-id re-stamp SetSessionOption fails"` — `RunFunc` returns an error for the `set-option` (`@portal-id`) call but empty-success for `new-session`/`list-panes`/`respawn-pane`; assert `Restore` returns no error and the arm phase still armed the pane (FIFO created / respawn-pane issued).
- `"it re-stamps before arming panes"` — assert the index of the `@portal-id` `set-option` call in `mock.Calls` precedes the first `respawn-pane` call (order guard).

**Edge Cases**:
- Empty saved `PortalID` → stamp skipped, session un-stamped, name fallback (legacy path) — must not stamp `""`.
- `SetSessionOption` error → swallowed, restore not aborted (best-effort, mirrors `CreateFromDir`).
- Re-stamp precedes `armPanes` (order preserved) — but firing does NOT depend on it (Task 3-4 ordering trap); the re-stamp serves only the post-restore concerns (a)-(c).

**Context**:
> Spec § Cross-Reboot Persistence of `@portal-id` → 3. Restore re-stamp (`internal/restore/session.go`): In `createSkeleton`, immediately after `NewSessionWithCommand(sess.Name, …)` recreates the session, re-stamp the saved id when present — best-effort, mirroring creation-time stamping: `if sess.PortalID != "" { _ = r.Client.SetSessionOption(sess.Name, PortalIDOption, sess.PortalID) }`. `sessions.json` is a snapshot the daemon continuously regenerates from live tmux state, not a store of record — so re-seeding the live session with its id is what keeps the id alive past the single restore read.
> Without the re-stamp the id resolves correctly for the first resume (baked from the saved snapshot) but is then lost, because: (a) Re-persistence — the first post-restore capture (~1s later) rewrites `sessions.json` from live state; a session with no live `@portal-id` is captured as `PortalID == ""`, erasing the id → the next reboot resurrects a bare shell. (b) Survives cleanup — post-restore stale-cleanup (bootstrap step 11) builds its live-key set from the live `@portal-id`; with the id absent, the restored session's live key falls back to the name, which no longer matches the id-keyed `hooks.json` entry, so cleanup deletes the hook that just fired. (c) Future rename — a subsequent rename of the restored session stays stable only while the live id is present.
> A legacy session with no saved id is left un-stamped and falls through to the name-based key, exactly as before.
> Constant: the option name is the single shared `PortalIDOption = "@portal-id"` constant (Task 1-3, in `internal/session`), referenced by every set-option site (creation, restore re-stamp) and kept in sync with the literal `@portal-id` embedded in `tmux.HookKeyFormat`. Grounding note: `internal/restore/session.go` does not currently import `internal/session`; this task adds that import — cycle-free because `internal/session` imports only `internal/tmux`.
> Firing does not depend on the re-stamp (ordering): Restore's order is `collectArmInfos` → `createSkeleton` → `armPanes` (`session.go:86`). The hook key is computed from SAVED `sess.PortalID` in `collectArmInfos` and baked into the helper's `--hook-key`; the helper resolves `hooks.json` by that baked key and never reads the live `@portal-id`. The re-stamp (in `createSkeleton`) precedes the helper launch (in `armPanes`), and serves only the post-restore concerns (a)-(c).

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Cross-Reboot Persistence of `@portal-id` → 3. Restore re-stamp; § Cross-Reboot Persistence → Constant; § Testing Requirements → Creation & persistence (component) → "Restore re-stamps `@portal-id` on the recreated session from the saved value, and skips the stamp when the saved value is empty"; § Acceptance Criteria 3 (durable across repeated reboots) & 4 (cleanup safety).

## session-rename-orphans-resume-hook-3-4 | approved

### Task session-rename-orphans-resume-hook-3-4: Bake the stable hook key in `collectArmInfos` via `tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)`

**Problem**: `collectArmInfos` (`internal/restore/session.go`) bakes the hook key that restore passes to the hydrate helper via `--hook-key`. Today it computes `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` — the SAVED (post-rename) name. Registration (Phase 2) now stores hooks under the immutable `@portal-id`, so a renamed session's baked name-key no longer matches the stored id-key: the helper's `hooks.LookupOnResume` misses and the pane resurrects a bare `$SHELL`. This is the proximate cause of the reboot-time bare shell (Stage 3 lookup miss).

**Solution**: Change the baking site to `hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` — the Phase 1 pure-Go formatter that prefers the SAVED `@portal-id` (persisted via Tasks 3-1/3-2) and falls back to the saved name when empty. The baked `--hook-key` then matches what registration stored, for any number of renames. The key uses SAVED indices (base-index drift preservation unchanged; FIFOs still use LIVE indices). Critically, the firing path is NOT changed to read the live `@portal-id` — the ordering trap.

**Outcome**: `collectArmInfos` bakes `<@portal-id or saved-name>:w.p` from saved state, so a renamed-but-stamped session's helper looks up the hook under the id and fires it; an un-stamped saved session falls back to the saved name (legacy); and hook firing is independent of the re-stamp (Task 3-3) because the baked key comes from saved state, not a live read.

**Do**:
- In `collectArmInfos` (`internal/restore/session.go` ~lines 104-115), change the `savedPaneArmInfo` literal's `hookKey` from `tmux.PaneTarget(sess.Name, w.Index, p.Index)` to `tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)`. This is the ONLY functional change in this task. `scrollAbs` (the saved-indexed scrollback path) is unchanged.
- Keep using the SAVED `w.Index` / `p.Index` (base-index drift preservation is unchanged — the hook key rides saved indices so `hooks.json` lookups stay addressable across drift; only FIFO paths / respawn targets in `armPanes` use LIVE indices). Do NOT switch the hook key to live indices.
- Update the `savedPaneArmInfo.hookKey` doc-comment (~lines 62-67) so it describes the baked key as the STABLE hook key (`prefer saved @portal-id, else saved name`) preserved across base-index drift, replacing any "raw saved structural identifier" wording that implies name-based keying.
- **Ordering-trap constraint (load-bearing):** do NOT change `cmd/state_hydrate.go` or any firing path to read the LIVE `@portal-id`. The key is computed from SAVED `sess.PortalID` here and baked into `--hook-key`; the helper (`runHydrate` → `execShellOrHookAndExit`) resolves `hooks.json` by that baked `cfg.HookKey` and never queries live tmux for the id. Making firing read the live id would make it depend on the Task 3-3 re-stamp ordering and reintroduce the rename-window race. The hydrate helper's `--hook-key` consumption stays exactly as it is.
- Do NOT change `armPanes`, `buildHydrateCommand`, `createSkeleton`, or the `--hook-key` flag plumbing — they consume `info.hookKey` verbatim; only its DERIVATION changes here.
- Extend `internal/restore/session_test.go`. Adapt `TestSessionRestorer_HydrateCommandContainsRawHookKey` (~line 297): add a `PortalID` to the saved session and assert the baked `--hook-key` is `'<id>:w.p'` (id-keyed) rather than the name; add a sibling case with `PortalID: ""` asserting the baked key falls back to `'<name>:w.p'`. Add an ordering-trap test: run `Restore` on a `PortalID`-stamped session WITHOUT any live re-stamp read influencing the key, and assert the baked `--hook-key` equals `HookKey(sess.PortalID, ...)` computed purely from the saved struct — i.e. the baked key is a function of saved state only, independent of what the re-stamp does live. Add a multi-pane case asserting distinct `w.p` suffixes under one id.

**Acceptance Criteria**:
- [ ] `collectArmInfos` sets `hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` — no `tmux.PaneTarget(sess.Name, ...)` remains as the hook-key derivation in `collectArmInfos`.
- [ ] For a saved session with `PortalID == "tok123"` and saved indices `w=3, p=7`, the baked `--hook-key` is `tok123:3.7` (id-keyed, saved indices).
- [ ] For a saved session with `PortalID == ""`, the baked `--hook-key` falls back to `<sess.Name>:w.p` (name-keyed — legacy path).
- [ ] Multi-pane under one id: each pane's baked key shares the `tok123` prefix with a distinct `:w.p` suffix (each pane independently addressable).
- [ ] Saved-index (base-index drift) preservation is unchanged — the hook key uses SAVED `w.Index`/`p.Index`, not live indices; FIFO paths/respawn targets still use live indices (`armPanes` unchanged).
- [ ] Ordering-trap guard: the baked `--hook-key` is a function of SAVED state only; the firing path (`cmd/state_hydrate.go`) is unchanged and never reads the live `@portal-id` — a test asserts firing/baking is independent of the re-stamp.
- [ ] `go build -o portal .` succeeds; `go test ./internal/restore/...` and `go test ./cmd/... -run Hydrate` pass.

**Tests** (`internal/restore/session_test.go`, package `restore_test`, NO `t.Parallel()`):
- `"it bakes the id-based hook key when the saved PortalID is set"` — saved `PortalID: "tok123"`, saved indices `3,7`; assert the hydrate command contains `--hook-key 'tok123:3.7'` (adapt `TestSessionRestorer_HydrateCommandContainsRawHookKey`).
- `"it bakes the name-based hook key when the saved PortalID is empty"` — `PortalID: ""`; assert `--hook-key '<name>:3.7'` (legacy fallback).
- `"it bakes distinct per-pane suffixes under one id for a multi-pane session"` — two panes under `PortalID: "tok123"`; assert two `--hook-key 'tok123:w.p'` values with distinct suffixes.
- `"it derives the baked key from saved state independent of any live re-stamp"` — assert the baked `--hook-key` equals `tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` computed directly from the saved struct (proves the key does not depend on a live `@portal-id` read — the ordering trap).

**Edge Cases**:
- Empty saved `PortalID` → name-based key (legacy fallback; the `HookKey` formatter returns `<name>:w.p` when the id is empty).
- Saved-index preservation: base-index drift between save and restore does not change the baked hook key (it uses saved indices) — only FIFO/respawn targets track live indices.
- Ordering trap: firing is correct INDEPENDENT of the re-stamp (Task 3-3) because the key comes from saved state and the helper never reads the live id; the firing path must NOT be changed to read `@portal-id` live.
- Multi-pane distinct `w.p` suffixes under one id (spec § Testing Requirements → multi-pane).

**Context**:
> Spec § Hook-Key Derivation → Stage 3 — Restore lookup baking (`internal/restore/session.go`): `collectArmInfos` today sets `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` (saved name). It is changed to `hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` — preferring the saved `@portal-id`, else the saved name. The baked `--hook-key` therefore matches what registration stored. Base-index drift preservation is unchanged (the key still uses saved indices, FIFOs still use live indices).
> Spec § Cross-Reboot Persistence → Firing does not depend on the re-stamp (ordering): Restore's order is `collectArmInfos` → `createSkeleton` → `armPanes` (`session.go:86`). The hook key is computed from SAVED `sess.PortalID` in `collectArmInfos` and baked into the helper's `--hook-key`; the helper resolves `hooks.json` by that baked key and NEVER reads the live `@portal-id`. Hook firing is therefore correct independent of the re-stamp. Implementation constraint: the firing path must not be changed to read the live `@portal-id` — doing so would make hook firing depend on re-stamp ordering and reintroduce a rename-window race.
> Spec § Stage 4 — Hydrate lookup (`cmd/state_hydrate.go`): No change. The helper looks up `hooks.LookupOnResume(store, cfg.HookKey)` using the baked `--hook-key`, then execs `sh -c '<hook>; exec $SHELL'` on a hit or bare `$SHELL` on a miss. Because stage 3 now bakes the stable key and stage 1 stored under it, the lookup hits for any renamed-but-stamped session.
> Spec § Risks → Missed key-producing site: the firing path must never read the live `@portal-id` (ordering trap).
> `tmux.HookKey` is the Phase 1 pure-Go formatter (Task 1-1): returns `<portalID>:w.p` when `portalID != ""`, else `<name>:w.p`. `savedPaneArmInfo.hookKey` is consumed verbatim by `buildHydrateCommand` (`--hook-key`), which the helper reads into `cfg.HookKey` — no interpolation, so any bytes pass through single-quoted.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation → Stage 3 — Restore lookup baking; § Hook-Key Derivation → Stage 4 — Hydrate lookup (No change); § Cross-Reboot Persistence → Firing does not depend on the re-stamp (ordering); § Risks → Missed key-producing site.

## session-rename-orphans-resume-hook-3-5 | approved

### Task session-rename-orphans-resume-hook-3-5: Integration — rename-then-restore fires the registered hook for both triggers (raw `tmux rename-session` and in-TUI `renameAndRefresh`)

**Problem**: The headline bug: a session with a registered resume hook, renamed while its pane process keeps running, comes back as a bare `$SHELL` after the next reboot because the hook is orphaned under the old name. The fix (stamp `@portal-id` at creation, persist it, bake the id-key at restore) must be proven end-to-end for BOTH rename triggers — external `tmux rename-session` AND Portal's in-TUI `renameAndRefresh`. Nothing yet exercises the full rename → capture → restore → FIRE chain; the reboot-roundtrip test asserts firing WITHOUT a rename, so the specific rename gap is uncovered.

**Solution**: Add a real-tmux integration test that stamps a session with `@portal-id`, registers an on-resume hook (resolved under the stable id-key), renames the session (once via raw `tmux rename-session`, once via the in-TUI `renameAndRefresh` path) WITHOUT restarting the pane process, captures + persists, restores against a fresh server, drives signal-hydrate, and asserts the hook FIRED (a side-effect file written) rather than the pane landing on a bare `$SHELL`. Both triggers share the same assertion harness.

**Outcome**: A committed integration guard proves that after a rename via EITHER trigger, a rebooted+restored session's registered on-resume hook fires (side-effect observed), not a bare shell — closing the headline reboot gap for both external and in-TUI renames, with the pane process kept alive across the rename (no self-heal).

**Do**:
- Add a new integration test file `internal/restore/rename_reboot_hook_integration_test.go` (package `restore_test`, `//go:build integration`, `testing.Short()` skip + `tmuxtest.SkipIfNoTmux(t)`), modelled on `internal/restore/integration_full_test.go` and its hook-firing sibling `cmd/bootstrap/reboot_roundtrip_test.go` (the side-effect-file + "fired exactly once" pattern at ~lines 195-232 / `verifyHookFiredOnce` ~line 829). Build the binary with `restoretest.BuildPortalBinaryDir(t)` + `restoretest.PrependPATH(t, binDir)` so the in-pane `portal state hydrate` helper resolves on PATH. Use `portaltest.IsolateStateForTest(t)` and apply the returned env to any spawned subprocess.
- Set `PORTAL_STATE_DIR` and `PORTAL_HOOKS_FILE` (a temp `hooks.json`) so the helper reads the test's store, not the user's. Create a hook side-effect file (e.g. `hook-fired.txt`) and register the on-resume hook as `echo HOOK_FIRED >> <file>` via `hooks.NewStore(hooksPath).Set(<stableKey>, "on-resume", hookCmd, "cli")`.
- Stamp the session with `@portal-id`: create the session on the `tmuxtest` socket, then `client.SetSessionOption(name, session.PortalIDOption, "tok123")` (or import the literal `"@portal-id"` if importing `internal/session` from the test would be awkward — prefer the constant). Register the hook under the STABLE id-key `tmux.HookKey("tok123", name, w, p)` — i.e. `tok123:0.0` — mirroring what production registration (Phase 2 `ResolveHookKey`) would store for a stamped session.
- **Trigger A (external):** rename the live session via raw `ts.Run(t, "rename-session", "-t", oldName, newName)`. The pane process must KEEP RUNNING across the rename (do not kill/respawn it) — the bug bites only when the inner process does not restart. `@portal-id` is unchanged by the rename; `#{session_name}` becomes `newName`.
- **Trigger B (in-TUI):** exercise `renameAndRefresh` (`internal/tui/model.go` ~line 3220) via its production seam — construct a `tui.Model` (or invoke the `SessionRenamer` seam it calls: `m.sessionRenamer.RenameSession(oldName, newName)`) so the rename goes through the SAME code path the `r` key drives. Do NOT modify `renameAndRefresh` — it stays a bare `RenameSession` + list refresh; this task only TESTS it. If constructing a full `Model` is impractical in this package, drive the exact production `RenameSession` call the TUI path makes against the socket client and note in a comment that this is the byte-equivalent of the in-TUI trigger (both reduce to `RenameSession(old, new)` with zero hook re-keying — the decisive property the fix relies on).
- After the rename, run the save→kill→restore cycle: `state.CaptureStructure(client, ...)` (which now captures `PortalID == "tok123"` under the NEW name via Task 3-2), seed the pane's scrollback file, `EncodeIndex` + write `sessions.json`, `ts.KillServer()`, stand up a fresh server, `SessionRestorer.Restore(sess)` for the captured (renamed) session, then drive signal-hydrate via `restoretest.DriveSignalHydrate` (or `DriveSignalHydrateBinary`) so the in-pane helper unblocks, dumps, and reaches its hook-lookup + exec.
- Assert the hook FIRED: poll the side-effect file until it contains exactly one `HOOK_FIRED` line (reuse the reboot test's `verifyHookFiredOnce` shape — exactly once proves it ran AND that `exec $SHELL` replaced the helper rather than spawning a child). A bare-shell miss leaves the file empty/absent — that is the failing (bug-present) state.
- Run BOTH triggers as separate subtests (or a table of two) sharing the setup/assert helpers; each does its own capture→restore→fire so a regression in one trigger does not mask the other.
- Guard against a vacuous pass: assert the pre-restore `sessions.json` recorded `PortalID == "tok123"` under the NEW name AND the hook entry is keyed `tok123:0.0` (so a green result cannot be "the name coincidentally matched").

**Acceptance Criteria**:
- [ ] After an external `tmux rename-session`, capture+restore+signal-hydrate fires the registered on-resume hook (side-effect file contains exactly one `HOOK_FIRED`), NOT a bare `$SHELL`.
- [ ] After the in-TUI `renameAndRefresh` rename (same `RenameSession` production path), capture+restore+signal-hydrate fires the hook (side-effect observed).
- [ ] The pane process is kept running across the rename in both triggers (no restart / self-heal) — the test does not re-register the hook after the rename.
- [ ] The hook is registered under the STABLE id-key (`tok123:0.0`), and the captured `sessions.json` records `PortalID == "tok123"` under the post-rename name (non-vacuous — asserted before restore).
- [ ] `renameAndRefresh` (and any TUI production code) is UNCHANGED — the test only exercises it (scope guard).
- [ ] The test is `//go:build integration`, skips under `-short` and via `tmuxtest.SkipIfNoTmux(t)`, uses `portaltest.IsolateStateForTest`, applies the env to subprocesses, and uses NO `t.Parallel()`.
- [ ] `go build -o portal .` succeeds; `go test -tags integration ./internal/restore/...` passes (and skips cleanly where tmux is absent / under `-short`).

**Tests** (`internal/restore/rename_reboot_hook_integration_test.go`, package `restore_test`, `//go:build integration`, `SkipIfNoTmux` + `testing.Short()` gated, NO `t.Parallel()`):
- `"it fires the resume hook after an external tmux rename-session and reboot"` — stamp + register under id; `rename-session`; capture→kill→restore→drive; assert `HOOK_FIRED` once.
- `"it fires the resume hook after an in-TUI renameAndRefresh rename and reboot"` — same, but the rename goes through the `RenameSession` seam the TUI path uses; assert `HOOK_FIRED` once.
- `"it keeps the pane process running across the rename (no self-heal re-registration)"` — structural: the hook is registered ONCE before the rename and never re-registered; the firing proves the id-keyed lookup, not a post-rename re-key.

**Edge Cases**:
- Pane process kept running across the rename (no restart) — the bug bites ONLY here; a restart would self-heal via the external tool re-running `portal hooks set` (spec § Problem Statement).
- Both triggers reduce to `RenameSession(old, new)` with zero hook re-keying — the fix works at the root (no rename interception), which is why the in-TUI path needs no change (spec § Scope & Non-Goals).
- Non-vacuous: `sessions.json` must record the id under the NEW name (proves persistence + id-key baking, not a name coincidence).

**Context**:
> Spec § Testing Requirements → The rename gap (integration — the headline coverage): Rename, then restore, fires the hook. Register a hook; rename the session; run restore; assert the resume hook fires (not bare `$SHELL`). Cover BOTH triggers: raw `tmux rename-session` AND the in-TUI rename path (`renameAndRefresh`).
> Spec § Problem Statement: the failure bites only when the inner pane process does NOT restart across the rename. If the process restarts (e.g. the external tool's own start-hook re-runs `portal hooks set` under the new name), the hook self-heals — which is why the bug hid in everyday use. The test must keep the process running (no self-heal).
> Spec § Scope & Non-Goals: Portal's in-TUI rename (`renameAndRefresh`, `internal/tui/model.go`) — no change required. It continues to do a bare `RenameSession` + list refresh with zero hook re-keying. This is the decisive advantage over the rejected "intercept-and-re-key" approach.
> Grounding: `cmd/bootstrap/reboot_roundtrip_test.go` is the canonical hook-firing harness — side-effect file `hook-fired.txt`, hook `echo HOOK_FIRED >> <file>`, `store.Set(savedHookKey, "on-resume", hookCmd, "cli")`, and `verifyHookFiredOnce` (~line 829) asserting exactly one line. `internal/restore/integration_full_test.go` is the canonical save→kill→restore→drive-signal-hydrate harness (uses `restoretest.BuildPortalBinaryDir`, `PrependPATH`, `DriveSignalHydrate`). `renameAndRefresh` (`internal/tui/model.go:3220`) is `m.sessionRenamer.RenameSession(oldName, newName)` then a list refresh — both triggers reduce to `RenameSession`.
> Conventions: `//go:build integration`; skip under `-short` and `SkipIfNoTmux`; daemon-spawning / bootstrap-subprocess tests MUST use `portaltest.IsolateStateForTest` and apply the returned env to every spawned subprocess; NO `t.Parallel()`.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Testing Requirements → The rename gap (integration — the headline coverage); § Problem Statement; § Scope & Non-Goals → Both rename triggers are fixed at the root; § Acceptance Criteria 2 (Rename survives reboot) & 6 (No external/UI change).

## session-rename-orphans-resume-hook-3-6 | approved

### Task session-rename-orphans-resume-hook-3-6: Integration — durable across repeated reboots + post-restore cleanup keeps the restored hook

**Problem**: A single rename+restore firing (Task 3-5) is necessary but not sufficient. If restore does not re-stamp the live `@portal-id` (Task 3-3), the id survives only the FIRST resume: the next capture rewrites `sessions.json` with `PortalID == ""` (erasing it → the SECOND reboot resurrects a bare shell — persistence chain (a)), and post-restore stale-cleanup (bootstrap step 11 / `portal clean`) builds its live-key set from the live `@portal-id`, so an absent id makes the restored session's live key fall back to the name and cleanup DELETES the just-fired hook in the same bootstrap (chain (b)). Both durability guarantees are uncovered.

**Solution**: Add two real-tmux integration assertions on top of the Task 3-5 harness. (1) DURABLE: after rename+restore, re-stamp is in place → simulate the NEXT capture (which now reads the live re-stamped `@portal-id` back into `PortalID`) and a SECOND restore, and assert the id is re-persisted and the hook STILL fires on the second cycle. (2) CLEANUP-SAFE: after restore, run the stale-cleanup pass (bootstrap step 11 / `portal clean` — `runHookStaleCleanup` fed by `ListAllPaneHookKeys` against the live re-stamped server) and assert the just-restored hook entry is NOT deleted, because the live-key set (from the re-stamped `@portal-id`) matches the id-keyed `hooks.json` entry.

**Outcome**: A committed guard proves the hook survives REPEATED reboots (id re-persisted by the next capture, fires again on cycle two) and survives post-restore stale-cleanup (the re-stamped live `@portal-id` yields a live key matching the id-keyed entry, so cleanup keeps it) — closing persistence chains (a) and (b).

**Do**:
- Add a new integration test file `internal/restore/rename_reboot_durability_integration_test.go` (or extend the Task 3-5 file), package `restore_test`, `//go:build integration`, `testing.Short()` + `tmuxtest.SkipIfNoTmux(t)` gated, `portaltest.IsolateStateForTest(t)` with env applied to subprocesses, NO `t.Parallel()`. Reuse the Task 3-5 setup helpers (binary build, PATH, hook side-effect file, id-stamp + id-keyed hook registration, save→kill→restore→drive-signal-hydrate).
- **Durability leg (chain (a)):** after the first rename+restore has run (with re-stamp active, Task 3-3), the recreated live session now carries `@portal-id = tok123` again. Simulate the NEXT capture: `state.CaptureStructure(client, ...)` against the restored+re-stamped live server, and assert the captured `Session.PortalID == "tok123"` (the id was RE-PERSISTED from live state — this is the assertion that fails without the re-stamp). Then persist that fresh capture, kill, and run a SECOND restore + signal-hydrate cycle, and assert the hook fires AGAIN on the second cycle (side-effect file now shows the second firing). Use a fresh side-effect file per cycle (or assert the incremented line count) so "fired on cycle two" is unambiguous.
- **Cleanup-safety leg (chain (b)):** after a rename+restore (re-stamp active), run the hook stale-cleanup pass exactly as production does. Two equivalent entry points — pick the one that stays a real-tmux, no-extra-subprocess test: either call `runHookStaleCleanup(client, store, logger, false, nil)` directly (bootstrap step 11's helper) with the lister = the live re-stamped `*tmux.Client` (whose `ListAllPaneHookKeys` now enumerates `tok123:0.0`), OR drive `portal clean` end-to-end. Assert the id-keyed `hooks.json` entry `tok123:0.0` SURVIVES the cleanup (the live-key set from the re-stamped `@portal-id` contains `tok123:0.0`, matching the on-disk key), while an unrelated truly-stale entry is still swept (proving cleanup still functions). This is the concrete guard for spec (b).
- Guard against vacuous passes: on the durability leg, assert the captured `PortalID` is non-empty BEFORE the second restore (so a green "fired again" cannot be a name coincidence); on the cleanup leg, assert the live-key enumeration actually contains `tok123:0.0` (the re-stamp took effect) before asserting survival.
- Do NOT modify production code — this task is integration coverage only; it depends on Tasks 3-1 through 3-4 being in place.
- Reference the reboot-roundtrip test's `verifyHookFiredOnce` for the per-cycle "fired exactly once" assertion shape, and Phase 2 Task 2-4/2-6 cleanup harness (`runHookStaleCleanup`, `ListAllPaneHookKeys`, seeded `hooks.json`) for the cleanup leg.

**Acceptance Criteria**:
- [ ] After a rename+restore (re-stamp active), a simulated next `CaptureStructure` against the live server yields `Session.PortalID == "tok123"` (id RE-PERSISTED from live state — fails without Task 3-3's re-stamp).
- [ ] A SECOND restore+signal-hydrate cycle (from the re-persisted `sessions.json`) fires the on-resume hook AGAIN (side-effect observed on cycle two) — durable across repeated reboots.
- [ ] After restore, the hook stale-cleanup pass (`runHookStaleCleanup` fed by `ListAllPaneHookKeys` / `portal clean`) does NOT delete the id-keyed entry `tok123:0.0` — the live-key set from the re-stamped live `@portal-id` matches the on-disk key.
- [ ] A truly-stale entry (no matching live pane) is still swept by the same cleanup pass (cleanup correctness preserved).
- [ ] Non-vacuous: the captured `PortalID` is asserted non-empty before the second restore, and the live-key enumeration is asserted to contain `tok123:0.0` before asserting cleanup survival.
- [ ] The test is `//go:build integration`, skips under `-short` and `SkipIfNoTmux`, uses `portaltest.IsolateStateForTest` (env applied to subprocesses), NO `t.Parallel()`; no production code changed.
- [ ] `go build -o portal .` succeeds; `go test -tags integration ./internal/restore/...` (and `./cmd/...` if the cleanup leg lives there) passes and skips cleanly where tmux is absent / under `-short`.

**Tests** (package `restore_test` / `cmd` as appropriate, `//go:build integration`, `SkipIfNoTmux` + `testing.Short()` gated, NO `t.Parallel()`):
- `"it re-persists the @portal-id on the next capture after restore"` — restore (re-stamp) → `CaptureStructure` → assert `PortalID == "tok123"`.
- `"it fires the resume hook again on a second reboot cycle"` — re-persist → kill → second restore → drive → assert the hook fired on cycle two.
- `"it keeps the freshly-restored id-keyed hook through post-restore stale-cleanup"` — restore (re-stamp) → `runHookStaleCleanup` (or `portal clean`) → assert `tok123:0.0` survives.
- `"it still sweeps a truly-stale entry during post-restore cleanup"` — seed an extra `gone:0.0` with no live pane; assert it is removed while `tok123:0.0` survives (cleanup still correct).

**Edge Cases**:
- Without the re-stamp, the next capture would record `PortalID == ""` → second reboot resurrects a bare shell (chain (a)) — the durability leg pins this against regression.
- Without the re-stamp, cleanup's live-key set falls back to the name and DELETES the just-fired hook in the same bootstrap (chain (b)) — the cleanup leg pins this.
- The cleanup live-key set is enumerated via `ListAllPaneHookKeys` (`HookKeyFormat`, Phase 2) reading the LIVE re-stamped `@portal-id`; consistency between the re-stamped live id and the id-keyed on-disk entry is what keeps the hook.
- Truly-stale entries are still swept — the no-regression protection must not weaken cleanup.

**Context**:
> Spec § Testing Requirements → The rename gap: Durable across repeated reboots — after a rename+restore, simulate the next capture and a second restore; assert the id is re-persisted and the hook still fires on the second cycle (guards the persistence chain in Cross-Reboot Persistence (a)). Post-restore cleanup keeps the restored hook — run restore, then the stale-cleanup pass (bootstrap step 11 / `portal clean`); assert the just-restored hook entry is NOT deleted (guards (b)).
> Spec § Cross-Reboot Persistence (a) Re-persistence: the first post-restore capture (~1s later) rewrites `sessions.json` from live state; a session with no live `@portal-id` is captured as `PortalID == ""`, erasing the id → the next reboot resurrects a bare shell. (b) Survives cleanup: post-restore stale-cleanup (bootstrap step 11) builds its live-key set from the live `@portal-id`; with the id absent, the restored session's live key falls back to the name, which no longer matches the id-keyed `hooks.json` entry, so cleanup deletes the hook that just fired — in the same bootstrap.
> Spec § Hook-Key Derivation → Stage 4 Post-restore consistency: after restore re-stamps `@portal-id` on the recreated live session, a subsequent stage-2 cleanup read of the live session yields the same key stage 3 baked — so cleanup never treats a freshly-restored hook as stale.
> Grounding: `runHookStaleCleanup` (`cmd/run_hook_stale_cleanup.go`) fed by `(*tmux.Client).ListAllPaneHookKeys` (Phase 2 Tasks 2-3/2-4) is the production cleanup pass; `CaptureStructure` (`internal/state/capture.go`) reads the live `@portal-id` back into `PortalID` (Task 3-2); `restoretest.DriveSignalHydrate` unblocks the helper; `verifyHookFiredOnce` (reboot test ~line 829) is the per-cycle firing assertion.
> Conventions: `//go:build integration`; `-short` / `SkipIfNoTmux` skip; `portaltest.IsolateStateForTest` with env applied to subprocesses; NO `t.Parallel()`.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Testing Requirements → The rename gap (Durable across repeated reboots; Post-restore cleanup keeps the restored hook); § Cross-Reboot Persistence (a) Re-persistence & (b) Survives cleanup; § Hook-Key Derivation → Stage 4 Post-restore consistency; § Acceptance Criteria 3 (durable) & 4 (cleanup safety).

## session-rename-orphans-resume-hook-3-7 | approved

### Task session-rename-orphans-resume-hook-3-7: Integration — multi-pane fires on the correct pane + graceful legacy degradation

**Problem**: Two remaining acceptance boundaries are unproven end-to-end. (1) MULTI-PANE: per-pane hooks under a single session must stay independently addressable and fire on the CORRECT pane after rename+restore — a bug that keyed hooks off the session id alone (dropping the `w.p` suffix) would fire the wrong pane's hook. (2) GRACEFUL LEGACY: an un-stamped session (no saved `@portal-id`) must degrade to the name-based key at every stage (capture `""`, skip re-stamp, name-fallback baking) without panicking on the empty id anywhere in the chain.

**Solution**: Add a real-tmux integration test with two legs on the Task 3-5 harness. (1) Stamp ONE session with `@portal-id`, register DISTINCT on-resume hooks on two different panes (each with its own `:w.p` suffix under the shared id, each writing to its own side-effect file), rename+restore, and assert EACH pane's hook fired on ITS pane (correct suffix routing). (2) Restore an UN-STAMPED saved session (`PortalID == ""`) end-to-end — capture yields `""`, re-stamp is skipped, the baked key falls back to the name — and assert no panic anywhere and the name-based hook fires (or cleanly misses to bare shell) with the name key used throughout.

**Outcome**: A committed guard proves per-pane hooks under one session fire on the correct pane after rename+restore (distinct `w.p` suffixes routed independently) AND an un-stamped session degrades to the name-based key end-to-end with no panic on the empty `PortalID` — closing acceptance criteria 7 and 8.

**Do**:
- Add a new integration test file `internal/restore/multipane_legacy_integration_test.go` (package `restore_test`, `//go:build integration`, `testing.Short()` + `tmuxtest.SkipIfNoTmux(t)` gated, `portaltest.IsolateStateForTest(t)` with env applied to subprocesses, NO `t.Parallel()`). Reuse the Task 3-5 harness helpers (binary build, PATH, `PORTAL_HOOKS_FILE`, save→kill→restore→drive).
- **Multi-pane leg:** create ONE session, stamp `@portal-id = tok123`, and give it two panes (split-window) at distinct `w.p` — e.g. `tok123:0.0` and `tok123:0.1`. Register TWO on-resume hooks, one per pane key, each appending to a DISTINCT side-effect file (`hook-pane0.txt`, `hook-pane1.txt`) with a distinct marker. Rename the session (keep both pane processes running), capture (both pane rows carry `tok123`; `Session.PortalID == tok123`), restore, drive signal-hydrate for BOTH panes. Assert pane 0's hook wrote ONLY `hook-pane0.txt` and pane 1's hook wrote ONLY `hook-pane1.txt` — proving the `:w.p` suffix routes each hook to its own pane and the shared id did not cross-fire. (Base-index drift note: the baked hook key uses SAVED indices; if the test applies drift, assert against the saved-index keys, not live.)
- **Legacy leg:** stand up an UN-STAMPED session (do NOT stamp `@portal-id`) named e.g. `legacy-proj`, register a name-based on-resume hook under `legacy-proj:0.0` (the name-fallback key), capture (asserting `Session.PortalID == ""`), restore, and drive signal-hydrate. Assert: (i) NO panic occurs anywhere in capture/re-stamp/bake/hydrate on the empty `PortalID`; (ii) the re-stamp is SKIPPED (no `@portal-id` set-option — the session stays un-stamped); (iii) the baked `--hook-key` is the name-based `legacy-proj:0.0`; (iv) the name-based hook fires (side-effect observed) — the legacy path still works via the name coincidence. Optionally also assert an un-stamped session with NO registered hook cleanly lands on a bare `$SHELL` (a clean miss, no panic).
- Guard against vacuous passes: assert the multi-pane hooks are registered under DISTINCT keys before restore (so "fired on the right pane" is meaningful), and assert the legacy session's captured `PortalID` is exactly `""` before restore.
- Do NOT modify production code — integration coverage only; depends on Tasks 3-1 through 3-4.

**Acceptance Criteria**:
- [ ] Multi-pane: two panes under one stamped session (`tok123:0.0`, `tok123:0.1`) with distinct registered hooks; after rename+restore, pane 0's hook fires into `hook-pane0.txt` ONLY and pane 1's into `hook-pane1.txt` ONLY (correct per-pane routing, no cross-fire under the shared id).
- [ ] The multi-pane hooks are registered under DISTINCT keys before restore (non-vacuous), and each fires exactly once on its own pane.
- [ ] Legacy: an un-stamped saved session captures `Session.PortalID == ""`, the re-stamp is skipped (no `@portal-id` set-option), and the baked `--hook-key` is the name-based `legacy-proj:0.0`.
- [ ] Legacy: the name-based hook fires after restore (name-fallback path works), and no panic occurs anywhere in the capture→re-stamp→bake→hydrate chain on the empty `PortalID`.
- [ ] (Optional) An un-stamped session with no registered hook lands on a bare `$SHELL` cleanly (a clean miss, no panic).
- [ ] The test is `//go:build integration`, skips under `-short` and `SkipIfNoTmux`, uses `portaltest.IsolateStateForTest` (env applied to subprocesses), NO `t.Parallel()`; no production code changed.
- [ ] `go build -o portal .` succeeds; `go test -tags integration ./internal/restore/...` passes and skips cleanly where tmux is absent / under `-short`.

**Tests** (`internal/restore/multipane_legacy_integration_test.go`, package `restore_test`, `//go:build integration`, `SkipIfNoTmux` + `testing.Short()` gated, NO `t.Parallel()`):
- `"it fires each per-pane hook on its correct pane after rename and restore"` — two panes under one id, distinct hooks/side-effect files; assert each fired only on its own pane.
- `"it degrades an un-stamped session to the name-based key end-to-end"` — un-stamped session; assert `PortalID == ""`, re-stamp skipped, baked key `legacy-proj:0.0`, name-based hook fires.
- `"it does not panic on an empty PortalID anywhere in the chain"` — the legacy leg runs capture→re-stamp→bake→hydrate with `PortalID == ""` and completes without panic (assert via the test simply reaching its end + a recover-free run).
- `"it lands on a bare shell for an un-stamped session with no registered hook"` (optional) — un-stamped, no hook; assert no side-effect and no panic (clean miss).

**Edge Cases**:
- Multi-pane distinct `w.p` suffixes under one id — each pane independently addressable; the shared id must NOT cross-fire hooks between panes (the `:w.p` suffix is load-bearing).
- Un-stamped session → `PortalID == ""` throughout: capture `""`, re-stamp skipped, name-fallback baking — no panic on the empty id at any stage (spec § Acceptance Criteria 8: un-stamped sessions never panic and degrade to the name-based key everywhere).
- Name-fallback coincides with the on-disk name key, so the legacy hook still fires (the no-migration guarantee for legacy sessions).
- Base-index drift (if exercised): the baked hook key uses SAVED indices — assert against saved-index keys.

**Context**:
> Spec § Testing Requirements → Multi-pane (integration): Per-pane hooks under one session remain independently addressable and fire for the correct pane after a rename+restore.
> Spec § Testing Requirements → Legacy / no-regression (integration): An un-stamped session degrades gracefully throughout (no panic; name-based key everywhere).
> Spec § Acceptance Criteria 7 (Multi-pane): per-pane hooks under one session remain independently addressable and fire on the correct pane after rename+restore. § Acceptance Criteria 8 (Graceful legacy): un-stamped sessions never panic and degrade to the name-based key everywhere.
> Spec § Fix Overview → Hook key = prefer `@portal-id`, else session name: un-stamped sessions (legacy, manually-created tmux sessions, or a best-effort stamp that failed) fall back to the session name — which equals the key already on disk, so existing `hooks.json` entries keep matching with no migration.
> Grounding: the multi-pane hook key is `tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` with distinct `w.p` suffixes (Task 3-4); `tmux.HookKey("", name, w, p)` returns `<name>:w.p` for the legacy leg (Task 1-1). The Task 3-5 harness (binary build, PATH, side-effect files, `DriveSignalHydrate`, `verifyHookFiredOnce`) is reused. The re-stamp skip on empty `PortalID` is Task 3-3; the empty-lift-no-panic is Task 3-2 (zero-row) / Task 3-1 (empty decode).
> Conventions: `//go:build integration`; `-short` / `SkipIfNoTmux` skip; `portaltest.IsolateStateForTest` with env applied to subprocesses; NO `t.Parallel()`.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Testing Requirements → Multi-pane (integration) & Legacy / no-regression (integration); § Acceptance Criteria 7 (Multi-pane) & 8 (Graceful legacy); § Fix Overview → Hook key = prefer `@portal-id`, else session name.

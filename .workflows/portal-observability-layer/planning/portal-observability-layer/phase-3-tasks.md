---
phase: 3
phase_name: State-mutation audit trail for user config files
total: 6
---

## portal-observability-layer-3-1 | approved

### Task 3-1: Expose `AtomicWrite` write-phase sentinels for `error_class` mapping (no logging in `fileutil`)

**Problem**: The store mutation methods (hooks/projects) persist via `internal/fileutil.AtomicWrite`. When that write fails, the spec mandates a WARN with `error_class` drawn from the closed AtomicWrite-phase value space (`write-failed-temp-create` / `write-failed-write` / `write-failed-fsync` / `write-failed-rename`). But `AtomicWrite` currently returns plain `fmt.Errorf`-wrapped strings with no machine-discriminable phase, and it MUST stay audit-unaware (it is shared with out-of-scope `sessions.json` and has no `op`/key semantics, so logging cannot live inside it). The stores need a way to classify which phase failed without `AtomicWrite` itself logging.

**Solution**: Add exported error sentinels to `internal/fileutil` — one per write phase — and wrap each existing failure-return with the matching sentinel via `%w` so callers can `errors.Is`-discriminate the phase. Then add a small pure classifier (in `internal/fileutil` or a leaf consumed by the stores, e.g. `func ClassifyWriteError(err error) string`) that maps a wrapped `AtomicWrite` error to one of the closed `error_class` strings, with a safe default for an unrecognised/unwrapped error. `AtomicWrite` itself emits NO log lines and imports nothing from `internal/log` — it only gains sentinel wrapping.

**Outcome**: A store that receives a non-nil error from `AtomicWrite` can call `fileutil.ClassifyWriteError(err)` and get back exactly one of `write-failed-temp-create` / `write-failed-write` / `write-failed-rename` (and `write-failed-fsync` only if the `[needs-info]` below is resolved to map it), or a safe default for any error not produced by `AtomicWrite`; `AtomicWrite` stays log-free and the `sessions.json` caller is behaviourally unchanged.

**Do**:
- In `internal/fileutil/atomic.go`, define exported sentinels:
  - `var ErrWriteTempCreate = errors.New("atomicwrite: temp file create failed")`
  - `var ErrWriteWrite = errors.New("atomicwrite: temp file write failed")`
  - `var ErrWriteRename = errors.New("atomicwrite: rename failed")`
  - (and, conditional on the `[needs-info]` resolution below, `ErrWriteFsync`).
- Rewrite the three (or four) failure returns in `AtomicWrite` to wrap the sentinel AND preserve the underlying `*os.PathError` chain — e.g. `return fmt.Errorf("%w: %w", ErrWriteTempCreate, err)` for the `os.CreateTemp` failure, `ErrWriteWrite` for the `tmp.Write` failure, `ErrWriteRename` for the `os.Rename` failure. Keep the existing cleanup (`os.Remove(tmpPath)`) untouched. The `os.MkdirAll` failure at the top of `AtomicWrite` is the directory-create step — map it to `ErrWriteTempCreate` (it is the temp-file-creation prerequisite) OR add no separate sentinel and let it fall through to the default; pick one and document the choice in a comment (the closed `error_class` space has no `write-failed-mkdir`, so it cannot get its own value).
- Add `func ClassifyWriteError(err error) string` (in `internal/fileutil`): `errors.Is(err, ErrWriteTempCreate)` → `"write-failed-temp-create"`; `ErrWriteWrite` → `"write-failed-write"`; `ErrWriteRename` → `"write-failed-rename"`; (`ErrWriteFsync` → `"write-failed-fsync"` if added). For any error not matching a known sentinel (a wrapped error from elsewhere, a bare error), return a safe default — use `"write-failed-write"` as the catch-all (it is the most representative "the persist did not complete" phase) and document that the default is a deliberate floor, not a real phase observation.
- Do NOT add any `import "github.com/leeovery/portal/internal/log"` to `internal/fileutil`; do NOT add any `logger.*` call inside `AtomicWrite` or `AtomicWrite0600`. The classifier is pure string mapping with zero side effects.
- Leave `AtomicWrite0600` returning the same wrapped error unchanged (it already forwards `AtomicWrite`'s error verbatim, so the sentinels survive through it).
- **`[needs-info]` — `write-failed-fsync` has no source step.** `AtomicWrite` is create-temp → write → `Close` → `Rename`; there is NO `Sync()` / `fsync` call, so the `write-failed-fsync` `error_class` value in the closed space has no emitter. Resolve one of three ways and record the resolution in a code comment + the task PR description: (a) map the existing `tmp.Close()` failure return to `ErrWriteFsync`/`write-failed-fsync` (close-flush is the closest analogue to fsync); (b) add a real `tmp.Sync()` before `Close` and map its failure to `write-failed-fsync`; or (c) accept that `write-failed-fsync` is currently unreachable and the `Close` failure maps to `write-failed-write`/`write-failed-rename`. Do NOT silently invent — flag this for reviewer/user decision; default to (a) (close→`write-failed-fsync`) only if the reviewer does not object, since `Close` is the buffer-flush boundary and (a) keeps the closed space fully reachable without a behaviour change.

**Acceptance Criteria**:
- [ ] `internal/fileutil` exposes exported sentinels for each write phase and `AtomicWrite` wraps each failure-return with the matching sentinel via `%w`, preserving the underlying `*os.PathError` (verified by `errors.Is(err, ErrWriteRename)` AND `errors.As(err, &pathErr)` both succeeding on a forced rename failure).
- [ ] `ClassifyWriteError` returns the correct `error_class` string for each sentinel and the documented safe default for an unrecognised error.
- [ ] `AtomicWrite` / `AtomicWrite0600` contain no `internal/log` import and no `logger.*` call (`grep` in the test or a build-tag-free assertion).
- [ ] The `sessions.json` write path (any existing `AtomicWrite` caller outside the three in-scope stores) is behaviourally unchanged — same error surfaces, just with a sentinel wrapper that pre-existing callers ignore.
- [ ] The `write-failed-fsync` `[needs-info]` is resolved explicitly (one of the three documented options), with the chosen mapping covered by a test or a code comment recording why it is unreachable.

**Tests**:
- `"it wraps a temp-create failure with ErrWriteTempCreate and ClassifyWriteError returns write-failed-temp-create"`
- `"it wraps a write failure with ErrWriteWrite"` (e.g. inject a write failure or assert via the rename-failure analogue)
- `"it wraps a rename failure with ErrWriteRename and preserves the *os.PathError via errors.As"`
- `"it returns the safe-default error_class for an error not produced by AtomicWrite"`
- `"it keeps AtomicWrite free of any logging import or call"`
- `"it leaves the AtomicWrite0600 error chain intact (sentinel survives the forward)"`

**Edge Cases**:
- `AtomicWrite` stays log-free / audit-unaware (no `internal/log` import, no `logger.*`).
- The fsync phase has no current `AtomicWrite` step — `[needs-info]`: close→`write-failed-fsync` vs real `Sync()` vs unreachable. Flag, do not silently resolve.
- Unknown / wrapped error → safe default phase (documented floor, not a real observation).
- `sessions.json` caller unaffected — sentinels are additive `%w` wrappers ignored by callers that do not `errors.Is` them.
- `errors.Is` must unwrap through the doubled `%w` chain (`fmt.Errorf("%w: %w", sentinel, osErr)`); confirm Go version supports multi-`%w` (Go 1.20+; this repo is well past that).

**Context**:
> "**Closed `error_class` value space for AtomicWrite failures (per phase):** `write-failed-temp-create` / `write-failed-write` / `write-failed-fsync` / `write-failed-rename`" (spec § State-mutation audit trail → Mechanical rule)
>
> "The generic `internal/fileutil.AtomicWrite` primitive stays audit-unaware — it is shared with out-of-scope `sessions.json` and has no `op`/key semantics — so logging does NOT live inside it." (spec § State-mutation audit trail → Seam)
>
> "A **whole-mutation WARN** (the store's `AtomicWrite` itself failed, so the write did not persist) carries `error_class` from the AtomicWrite phase space above (`write-failed-*`)." (spec § State-mutation audit trail → Which `error_class` space applies)
>
> Current `AtomicWrite` (`internal/fileutil/atomic.go`) phases: `os.MkdirAll` → `os.CreateTemp` → `tmp.Write` → `tmp.Close` → `os.Rename`. There is no `Sync()`/`fsync` call — the `write-failed-fsync` value has no direct source step (the `[needs-info]` above).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Decision, Seam, Mechanical rule, Which `error_class` space applies)

---

## portal-observability-layer-3-2 | approved

### Task 3-2: Instrument `hooks.Store` `Set`/`Remove` (set/modify/rm + set-noop DEBUG, value/via/error_class)

**Problem**: A `hooks.json` wipe on 2026-05-28 was undiagnosable because no breadcrumb recorded who changed the file or how. The spec requires every `hooks.Store` mutation to emit one INFO on success / one WARN on persist failure under component `hooks`, so `grep "hooks:" portal.log` reconstructs the change history. `hooks.Store.Set` and `Remove` currently persist silently via `Save` → `AtomicWrite`.

**Solution**: Instrument `hooks.Store.Set` and `Remove` at the store seam (NOT at the cmd callers, NOT inside `AtomicWrite`). Bind `var logger = log.For("hooks")` once at package init in `internal/hooks`. In `Set`, distinguish `set` (key/event did not exist), `modify` (existed, value differs), and `set-noop` (existed, value matches) from the pre-write `Load`; emit DEBUG `op=set-noop` and skip `Save` on the no-op, else `Save` and emit INFO `op=set`/`modify` on success or WARN with `error_class` on failure. In `Remove`, emit INFO `op=rm` on success / WARN on failure. Thread the `via` origin (`cli` for `portal hooks set/rm`, `internal` for the migrate-rename `Save` path) into the store so the breadcrumb records the caller class.

**Outcome**: Running `portal hooks set --on-resume "X"` for a new key writes one INFO `hooks: set hook_key=<key> value="X" via=cli`; re-running with the same value writes one DEBUG `hooks: set-noop hook_key=<key>` and does NOT touch the file; changing the value writes INFO `op=modify`; `portal hooks rm` writes INFO `hooks: rm hook_key=<key> via=cli` (no `value`); a forced `AtomicWrite` failure writes one WARN `hooks: ... error_class=write-failed-* error=<chain>`.

**Do**:
- In `internal/hooks/store.go`, add `import "github.com/leeovery/portal/internal/log"` and `var logger = log.For("hooks")` at package scope (component string is the literal taxonomy name `hooks`).
- Decide how `via` reaches the store method. The store is the single chokepoint that must record `via`, but `Set`/`Remove` are called from both the CLI (`cmd/hooks.go`, `via=cli`) and the internal migrate-rename `Save` path (`cmd/state_migrate_rename.go`, `via=internal`). Add a `via` parameter to the mutating methods (e.g. `Set(key, event, command, via string)` / `Remove(key, event, via string)`), with `via` constrained to the closed value space `cli` / `internal` / `migrate`. Update the two cmd callers (`hooksSetCmd`/`hooksRmCmd` pass `"cli"`). Document the `via` value space in a comment. (Migrate-rename uses `Save` directly, not `Set`/`Remove` — its `via=internal` emission is covered in Task 3-3's sibling consideration and the migrate-rename path; for this task, only `Set`/`Remove` gain `via`.)
- In `Set(key, event, command, via)`: keep the existing `Load`; before mutating, inspect the loaded map to classify:
  - key+event absent → `op=set` (new entry).
  - key+event present AND existing value == `command` → `op=set-noop`: emit `logger.Debug("set-noop", "hook_key", key, "via", via)` and `return nil` WITHOUT calling `Save` (idempotent no-op, per the level-discipline idempotent-no-op clarification).
  - key+event present AND existing value != `command` → `op=modify`.
  - Then `Save`. On `err == nil`: `logger.Info(op, "hook_key", key, "value", command, "via", via)` where `op` is `"set"` or `"modify"` (msg string is the op verb; `value` is the verbatim new command). On `err != nil`: `logger.Warn(op, "hook_key", key, "value", command, "via", via, "error", err, "error_class", fileutil.ClassifyWriteError(err))` and return the error.
- In `Remove(key, event, via)`: keep the existing `Load` + delete logic. After `Save`, on success `logger.Info("rm", "hook_key", key, "via", via)` (NO `value` attr — spec: `value` absent for `rm`). On failure `logger.Warn("rm", "hook_key", key, "via", via, "error", err, "error_class", fileutil.ClassifyWriteError(err))` and return the error. Note `Remove` is currently a no-op-but-still-`Save` when the key/event is absent — it still `Save`s (rewrites the file) and so still emits INFO `op=rm`; preserve that behaviour (the file mtime changes, which is itself an auditable event). Document that an absent-key `rm` still emits INFO because the persist still occurs.
- Use the closed attr vocabulary only: `op` (msg verb), `hook_key`, `value`, `via`, `error`, `error_class`. Message string is the terse `op` phrase (`"set"`, `"modify"`, `"rm"`, `"set-noop"`); data lives in attrs (never `fmt.Sprintf`).
- Do NOT add logging to `Save` itself (it is shared with `CleanStale` in Task 3-3 and the migrate-rename `Save`; the per-op emission lives in the calling mutation method, not in `Save`).
- Update `internal/hooks` tests and the cmd callers for the new `via` parameter signature.

**Acceptance Criteria**:
- [ ] `Set` for a brand-new key/event emits exactly one INFO `hooks: set hook_key=<k> value="<cmd>" via=<via>` and persists.
- [ ] `Set` for an existing key/event with a DIFFERENT value emits one INFO `op=modify` (msg `modify`) with `value`.
- [ ] `Set` for an existing key/event with the SAME value emits one DEBUG `hooks: set-noop hook_key=<k>` and does NOT call `Save` (file mtime unchanged).
- [ ] `Remove` emits one INFO `hooks: rm hook_key=<k> via=<via>` with NO `value` attr; an absent-key `Remove` still emits INFO (persist still occurs).
- [ ] A forced `AtomicWrite` failure on `Set`/`Remove` emits one WARN with `error` (the wrapped chain, passed as the error not `.Error()`) and `error_class` from `fileutil.ClassifyWriteError`.
- [ ] `via=cli` for `portal hooks set/rm`; the `via` value comes from the closed `cli`/`internal`/`migrate` space; no ad-hoc value.
- [ ] No logging is added to `AtomicWrite` or to `Save`; emission lives only in `Set`/`Remove`.

**Tests**:
- `"it emits INFO op=set with value and via=cli for a new hook key"`
- `"it emits INFO op=modify when the key exists with a different value"`
- `"it emits DEBUG op=set-noop and skips Save when key+value already match"`
- `"it emits INFO op=rm without a value attr"`
- `"it still emits INFO op=rm when removing an absent key (persist still occurs)"`
- `"it emits WARN with error_class=write-failed-* when AtomicWrite fails on Set"`
- `"it does not log inside AtomicWrite or Save"`

**Edge Cases**:
- set-noop (key+event exist, value matches) → DEBUG and skips `Save`.
- set-vs-modify decided from the pre-write `Load`.
- `rm` of an absent key still `Save`s → still INFO.
- `value` omitted for `rm`.
- WARN carries `write-failed-*` `error_class` (from Task 3-1's classifier), and `error` passes the wrapped error directly so the handler renders the full chain.
- `via=cli` (hooks set/rm) vs `internal` (migrate-rename `Save`, handled where `Save` is called, not in `Set`/`Remove`).
- Multi-word command values are quoted by the handler (`value="claude --resume X"`); privacy posture is verbatim (accepted single-user threat model).

**Context**:
> "The seam is each store's mutation methods … **NOT `AtomicWrite` and NOT the callers.**" (spec § State-mutation audit trail → Seam)
>
> "On `error == nil`: ONE INFO log line. On `error != nil`: ONE WARN log line. … Required attrs: `op`; key identifying the affected entry: `hook_key` (hooks); on failure: `error_class`. Optional: `value` — verbatim new value for `set`/`modify`; absent for `rm`/`clean-stale`. `via` — `cli` for user-facing commands, `internal` for code-driven mutations, `migrate` for the one-shot migrate path." (spec § Mechanical rule)
>
> "`set` Create new entry; `modify` Update existing entry (value differs); `rm` Remove existing entry; `set-noop` `set` where the entry already exists and the value matches (DEBUG only)." (spec § Closed `op` value space)
>
> "**No-op handling:** a `set` call where the entry already exists and the value matches → DEBUG with `op=set-noop`. NOT INFO." (spec § State-mutation audit trail)
>
> Privacy posture: "Hook commands … logged as-is. Threat model accepted: portal is a single-user dev tool." (spec § Privacy posture)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Seam, Mechanical rule, Closed `op` value space, No-op handling, Privacy posture)

---

## portal-observability-layer-3-3 | approved

### Task 3-3: Instrument `hooks.Store` `CleanStale` batch (per-entry DEBUG, INFO summary, per-entry WARN) and the migrate-rename `Save`

**Problem**: `hooks.Store.CleanStale` prunes stale hook entries in a single batched operation, and the spec's State-mutation audit trail mandates batch operations emit per-entry DEBUG, one INFO summary (`op=clean-stale entries=N entries_failed=M`), and a per-entry WARN on mid-loop failure. It also requires the one internal `Save`-driven mutation path (`cmd/state_migrate_rename.go`, `via=internal`) to be auditable. Neither currently emits a structured breadcrumb under the new logging foundation.

**Solution**: Instrument `hooks.Store.CleanStale` with the batch-summary shape (using the same `var logger = log.For("hooks")` bound in Task 3-2): per-removed-entry DEBUG inside the loop, one INFO summary at the end carrying `op=clean-stale`, `entries=N` (count removed), and `entries_failed=M` only if non-zero, plus a WARN with the AtomicWrite-phase `error_class` if the single batched `Save` fails. Separately, route the migrate-rename internal mutation (`runMigrateRename` calling `store.Save`) through an INFO/WARN emission with `via=internal` so the internal mutation path is auditable.

**Outcome**: `portal clean` (or bootstrap step 11) pruning two stale hooks writes two DEBUG `hooks: clean-stale hook_key=<k> via=internal` lines plus one INFO `hooks: clean-stale entries=2 via=internal took=<d>`; a zero-removal `CleanStale` writes neither a DEBUG nor a `Save` (and either no summary or a `entries=0` summary per the resolved no-op decision); a `Save` failure mid-batch writes one WARN with `error_class=write-failed-*`; the migrate-rename `Save` writes a `via=internal` breadcrumb.

**Do**:
- In `internal/hooks/store.go` `CleanStale(liveKeys []string)`, capture `start := time.Now()` at entry. Keep the existing kept/removed partition. For each removed key, emit `logger.Debug("clean-stale", "hook_key", key, "via", "internal")` (per-entry breadcrumb; `via=internal` because `CleanStale` is always code-driven).
- Only call `Save` when `len(removed) > 0` (preserve existing behaviour). On `Save` success, emit ONE INFO summary: `logger.Info("clean-stale", "entries", len(removed), "via", "internal", "took", time.Since(start))`. Include `"entries_failed", M` ONLY if `M > 0` (omit the attr entirely when zero, per spec).
- On `Save` failure, emit ONE WARN: `logger.Warn("clean-stale", "entries", len(removed), "via", "internal", "error", err, "error_class", fileutil.ClassifyWriteError(err), "took", time.Since(start))` and return the error. This is a **whole-batch persist failure**, so it carries the AtomicWrite-phase `error_class` (`write-failed-*`), NOT `unexpected`.
- **Zero-removal case:** when `len(removed) == 0`, the existing code skips `Save`. Decide and document: either (a) emit no summary at all (nothing happened — matches the "idempotent skip clutters INFO" guidance), or (b) emit a DEBUG `entries=0` summary. Prefer (a) — no INFO/no DEBUG/no `Save` on zero removals — and document that the spec's batch-summary INFO is for batches that did work. Confirm against the cycle-summary catalog (which lists Hooks CleanStale under the batch-summary shape) and the level-discipline idempotent-no-op clarification.
- **`[needs-info]` — single batched `Save` means the per-entry WARN may be unreachable.** The spec mandates "Per-entry WARN with `error_class=unexpected` on per-entry failure mid-loop." But `CleanStale` does NOT process entries individually with a failure path mid-loop — it partitions the map in memory (no per-entry I/O) and persists once via a single `Save`. There is no point at which one entry can fail while the batch continues. Flag this explicitly: the per-entry `error_class=unexpected` WARN has no reachable code site in the current `CleanStale` shape; the only failure is the whole-batch `Save` (which is `write-failed-*`, not `unexpected`). Do NOT invent a synthetic per-entry failure path. Record this as a spec/code mismatch for reviewer/user resolution (the per-entry-WARN clause may apply only to a future per-entry-I/O batch op, or `CleanStale` may need restructuring — neither is in scope here without sign-off).
- **Migrate-rename internal mutation (`cmd/state_migrate_rename.go`):** `runMigrateRename` rewrites keys then calls `store.Save(h)` directly (bypassing `Set`/`Remove`). To keep the `via=internal` mutation auditable under component `hooks`, emit an INFO on the successful `Save` and a WARN on failure from `runMigrateRename` — but per the seam rule, prefer doing this through a store method rather than at the caller. Add a thin store method (e.g. `SaveWithAudit(h, op, via, key)` or reuse the existing `Save` with an audit wrapper inside the store) so the emission stays at the store seam. If the migration's per-file shape has no single key (it rewrites N keys), emit ONE INFO summary `hooks: modify entries=N via=internal` (treat the bulk key-rewrite as a batch with `op=modify` semantics) rather than per-key, and document the choice. The existing `runMigrateRename` `state.Logger.Warn` collision/load/save lines migrate to the new `logger` (component `hooks`) per the Phase 1 sweep — this task only adds the success-path breadcrumb the old code lacked.

**Acceptance Criteria**:
- [ ] `CleanStale` removing N>0 entries emits N DEBUG `hooks: clean-stale hook_key=<k> via=internal` lines plus exactly one INFO summary `hooks: clean-stale entries=N via=internal took=<d>`.
- [ ] `entries_failed` is present in the summary only when M>0 (omitted entirely when zero).
- [ ] A whole-batch `Save` failure emits one WARN with `error_class` from `fileutil.ClassifyWriteError` (a `write-failed-*` value), NOT `unexpected`.
- [ ] A zero-removal `CleanStale` emits no INFO summary and does not call `Save` (documented decision (a)).
- [ ] The migrate-rename internal `Save` emits a `via=internal` breadcrumb at the store seam (one INFO summary on success / WARN on failure), and its load/collision/save WARN lines render under component `hooks`.
- [ ] The single-batched-`Save` per-entry-WARN `[needs-info]` is flagged in a code comment + PR description, not silently resolved.

**Tests**:
- `"it emits per-entry DEBUG and one INFO summary with entries=N for CleanStale removing N hooks"`
- `"it omits entries_failed from the summary when no per-entry failures occur"`
- `"it emits WARN with write-failed-* error_class (not unexpected) when the batched Save fails"`
- `"it emits no summary and skips Save when CleanStale removes zero entries"`
- `"it emits a via=internal breadcrumb when migrate-rename persists rewritten keys"`

**Edge Cases**:
- Zero removals → no `Save`, no summary (decision (a), documented).
- Whole-batch `Save` failure → `write-failed-*` `error_class`, not per-entry `unexpected`.
- `[needs-info]`: single batched `Save` means the spec's "per-entry WARN on mid-loop failure" has no reachable site — flag for reviewer, do not invent a synthetic failure path.
- `entries_failed` omitted when zero.
- Migrate-rename has N rewritten keys but no single per-file key → emit one `entries=N` summary, not per-key (documented).

**Context**:
> "**Batch operations** (e.g. `CleanStale` iterating entries): Per-entry DEBUG inside the loop. ONE INFO summary at the end of the batch with attrs `op=<batch-op>`, `entries=N`, and `entries_failed=M` if any per-entry failures occurred. Per-entry WARN with `error_class=unexpected` on per-entry failure mid-loop (regardless of whether the batch continues)." (spec § State-mutation audit trail → Batch operations)
>
> "A **per-entry batch WARN** (one entry failed to process mid-loop while the batch continues) carries `error_class=unexpected` … The two never overlap: phase values describe a failed persist of the whole file; `unexpected` describes a single dropped per-item operation." (spec § Which `error_class` space applies)
>
> "Hooks CleanStale | `hooks` | (same batch-summary shape as *State-mutation audit trail*)" (spec § Cycle-level summary cadence → Concrete cycle catalog)
>
> Current `CleanStale` (`internal/hooks/store.go:130`) partitions in memory and calls a single `Save` only when `len(removed) > 0` — there is no per-entry I/O failure path (the `[needs-info]` above). `runMigrateRename` (`cmd/state_migrate_rename.go:57`) calls `store.Save(h)` directly after rewriting keys (the `via=internal` path).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Batch operations, Which `error_class` space applies), § Cycle-level summary cadence and shape (Concrete cycle catalog — Hooks CleanStale)

---

## portal-observability-layer-3-4 | approved

### Task 3-4: Instrument `project.Store` `Upsert`/`Rename`/`Remove`/`CleanStale` (op vocabulary, project/value/via/error_class)

**Problem**: `projects.json` mutations (session creation upserts, TUI rename/delete, stale cleanup) leave no breadcrumb, so a project entry changing or disappearing is unexplainable. The spec mandates every `project.Store` mutation emit one INFO on success / one WARN on persist failure under component `projects`, with the closed `op` vocabulary, the `project` key attr, `value`/`via` optionals, set-noop DEBUG, and the batch-summary shape for `CleanStale`.

**Solution**: Instrument `project.Store.Upsert`, `Rename`, `Remove`, and `CleanStale` at the store seam, binding `var logger = log.For("projects")` once in `internal/project`. `Upsert` classifies `set` (path not found) / `modify` (path found) / `set-noop` (found and name+timestamp-equivalent unchanged) from the pre-write `Load`; `Rename` emits `op=modify` (or no-op silently when the path is absent); `Remove` emits `op=rm`; `CleanStale` emits the batch summary. Thread `via` (`cli` from the TUI edit/delete, `internal` from `session.PrepareSession`'s `Upsert`).

**Outcome**: Creating a session writes one INFO `projects: set project=<name> value=<path> via=internal` (or `op=modify` if the path was already remembered); a TUI rename writes `projects: modify project=<new> via=cli`; a TUI delete writes `projects: rm project=<path> via=cli`; `CleanStale` pruning N gone-directories writes per-entry DEBUG + one INFO `projects: clean-stale entries=N`; a Rename of an absent path writes nothing.

**Do**:
- In `internal/project/store.go`, add `import "github.com/leeovery/portal/internal/log"` and `var logger = log.For("projects")` at package scope (literal taxonomy component `projects`).
- Thread `via` into the mutating methods. Add a `via` parameter (constrained to `cli`/`internal`/`migrate`) to `Upsert`, `Rename`, `Remove` (NOT `CleanStale` — it is always `internal`). Update callers: `internal/session/prepare.go` `store.Upsert(resolvedDir, projectName)` → `via="internal"`; `internal/tui/model.go` `Remove`/`Rename` call sites → `via="cli"`. The `session.ProjectStore` interface (`internal/session/create.go`) and the TUI `ProjectStore`/`ProjectEditor` interfaces (`internal/tui/model.go:54,76`) and their mocks must gain the `via` parameter.
- **Choose the `project` attr value.** The spec's per-file key for projects is `project`. The store keys entries by `path` (the unique identifier), but the human-meaningful name is `Name`. Use the project **path** as the `project` attr value (it is the identifying key and matches `Remove(path)`/`Rename(path,...)`/`Upsert(path,...)` argument semantics), and carry the name as `value` where a value is relevant. Document this choice; the spec's example renders `project=<name>` loosely but the identifying key is the path — flag the minor ambiguity in a comment and prefer path for addressability.
- `Upsert(path, name, via)`: from the pre-write `Load`, classify:
  - path not found → `op=set`.
  - path found AND name unchanged → `op=set-noop`: emit `logger.Debug("set-noop", "project", path, "via", via)` and skip `Save` ONLY IF the existing behaviour permits skipping. NOTE: current `Upsert` always updates `LastUsed = now` even when the name matches, so the file content always changes — a true set-noop (file unchanged) does not exist for `Upsert` as written. Resolve: treat "path found, name unchanged" as `op=modify` (because `LastUsed` still changes and the file is still written), and reserve `set-noop` for the case where nothing at all would change. Document that `Upsert`'s `LastUsed` bump means `set-noop` is effectively unreachable unless the code is restructured to skip the timestamp bump; do not restructure without sign-off — emit `op=set` (new) or `op=modify` (existing) only.
  - path found, new entry → `op=set`.
  - After `Save`: INFO `logger.Info(op, "project", path, "value", name, "via", via)` on success (`op` ∈ `set`/`modify`); WARN with `error_class` on failure.
- `Rename(path, newName, via)`: the existing code is a no-op (returns nil without `Save`) when the path is not found — in that case emit NOTHING (no INFO, no `Save`). When the path IS found, after `Save`: INFO `logger.Info("modify", "project", path, "value", newName, "via", via)` on success / WARN on failure. (Rename is a `modify` op — it changes an existing entry's name.)
- `Remove(path, via)`: the existing code always `Save`s the filtered slice (even when the path is absent). After `Save`: INFO `logger.Info("rm", "project", path, "via", via)` (no `value`) on success / WARN on failure. Document that removing an absent path still `Save`s and so still emits INFO (the persist still occurs).
- `CleanStale()`: capture `start := time.Now()`. Per-removed-project DEBUG `logger.Debug("clean-stale", "project", p.Path, "via", "internal")`. Only `Save` when `len(removed) > 0` (preserve behaviour). On success, ONE INFO summary `logger.Info("clean-stale", "entries", len(removed), "via", "internal", "took", time.Since(start))` (omit `entries_failed` when zero). On `Save` failure, ONE WARN with `error_class` from `fileutil.ClassifyWriteError` (whole-batch `write-failed-*`). Zero removals → no summary, no `Save` (same decision (a) as Task 3-3, documented). The same single-batched-`Save` `[needs-info]` from Task 3-3 applies here (no reachable per-entry `unexpected` WARN) — flag it identically.
- Use only the closed attrs: `op` (msg verb), `project`, `value`, `via`, `error`, `error_class`, plus `entries`/`entries_failed`/`took` for the batch. No `fmt.Sprintf` in messages.
- Do NOT add logging to `Save` or `AtomicWrite`.

**Acceptance Criteria**:
- [ ] `Upsert` of a not-yet-remembered path emits INFO `projects: set project=<path> value=<name> via=<via>`; of an existing path emits `op=modify`.
- [ ] `Rename` of a found path emits INFO `projects: modify project=<path> value=<newName> via=cli`; of an absent path emits NOTHING and does not `Save`.
- [ ] `Remove` emits INFO `projects: rm project=<path> via=cli` with no `value`; removing an absent path still emits INFO (persist still occurs).
- [ ] `CleanStale` removing N>0 emits N DEBUG + one INFO `projects: clean-stale entries=N via=internal took=<d>`; zero removals emit nothing and skip `Save`.
- [ ] WARN paths on `Upsert`/`Rename`/`Remove`/`CleanStale` carry `error_class` from the AtomicWrite phase space.
- [ ] `via=internal` for `session.PrepareSession`'s `Upsert`; `via=cli` for the TUI rename/delete; interfaces + mocks updated.
- [ ] The `project` attr uses the path (documented), and the single-batched-`Save` / `LastUsed`-bump-vs-set-noop ambiguities are flagged in comments.

**Tests**:
- `"it emits INFO op=set with value=name and via=internal for a new project Upsert"`
- `"it emits INFO op=modify when Upsert targets an existing path"`
- `"it emits INFO op=modify via=cli for a TUI Rename of a found path"`
- `"it emits nothing and does not Save when Rename targets an absent path"`
- `"it emits INFO op=rm via=cli without a value attr for Remove"`
- `"it still emits INFO op=rm when removing an absent path"`
- `"it emits per-entry DEBUG and one INFO summary for CleanStale removing N projects"`
- `"it emits no summary and skips Save when CleanStale removes zero projects"`
- `"it emits WARN with write-failed-* error_class when AtomicWrite fails"`

**Edge Cases**:
- `Upsert` found → `modify` / not-found → `set`; true `set-noop` is effectively unreachable because `LastUsed` always bumps (flagged, not restructured).
- `Rename` no-op when path absent → no `Save`, no INFO.
- `Remove` of an absent path still `Save`s → still INFO.
- `CleanStale` batch summary with `entries`/`entries_failed`; zero removals → nothing.
- Single-batched-`Save` per-entry-`unexpected` WARN unreachable (same `[needs-info]` as Task 3-3).
- `via=cli` (TUI) vs `internal` (session prepare `Upsert`, `CleanStale`).
- `project` attr value = path (documented choice over name).

**Context**:
> "`projects.json` (component `projects`)" is in the closed file scope. (spec § State-mutation audit trail → Files in scope)
>
> "Key identifying the affected entry: … `project` (projects)." (spec § Mechanical rule)
>
> "`via` — `cli` for user-facing commands, `internal` for code-driven mutations (e.g. `CleanStale`)." (spec § Mechanical rule)
>
> "`set` Create new entry; `modify` Update existing entry; `rm` Remove existing entry; `clean-stale` Internal cleanup (always batched); `set-noop` (DEBUG only)." (spec § Closed `op` value space)
>
> Code: `Upsert` (`internal/project/store.go:71`) always sets `LastUsed = now` (so the file always changes — `set-noop` effectively unreachable). `Rename` (`:150`) is a no-op without `Save` when path not found. `Remove` (`:168`) always `Save`s. `CleanStale` (`:117`) `Save`s only when `len(removed) > 0`. Internal `Upsert` caller: `internal/session/prepare.go:49`. TUI callers: `internal/tui/model.go:960` (`Remove`), `:1459` (`Rename`), `:941`/`:963` (`CleanStale`).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Files in scope, Seam, Mechanical rule, Closed `op` value space, Batch operations)

---

## portal-observability-layer-3-5 | approved

### Task 3-5: Instrument `alias.Store` mutation+persist seam (alias/op/value/via, set-noop)

**Problem**: The `aliases` flat-file store mutates differently from the hooks/projects JSON stores: `Set`/`Delete` are in-memory map operations (returning no error), and persistence is a separate `Save()` that uses `os.WriteFile` (NOT `fileutil.AtomicWrite`). The spec requires every `aliases` mutation to leave a breadcrumb under component `aliases` with the `op` vocabulary, the `alias` key attr, `value`/`via` optionals, and an `error_class` on persist failure — but the split in-memory/persist shape means the natural emission point and the `error_class` phase mapping are not obvious.

**Solution**: Instrument the alias store at the seam that combines the in-memory mutation with the subsequent `Save()`, binding `var logger = log.For("aliases")` once in `internal/alias`. Because `Set`/`Delete` are pure in-memory and the durable change happens at `Save()`, emit the INFO/WARN breadcrumb at the persist boundary, classifying `op` (`set`/`modify`/`set-noop`/`rm`) from the in-memory state captured before the mutation. Thread `via=cli` from the two callers (`cmd/alias.go` and the TUI alias editor).

**Outcome**: `portal alias set foo /path` for a new alias writes one INFO `aliases: set alias=foo value=/path via=cli` after the `Save` succeeds; re-running with the same path writes DEBUG `op=set-noop`; a different path writes `op=modify`; `portal alias rm foo` writes INFO `aliases: rm alias=foo via=cli`; a `Save` failure writes one WARN with an `error_class` mapping the `os.WriteFile` phase.

**Do**:
- In `internal/alias/store.go`, add `import "github.com/leeovery/portal/internal/log"` and `var logger = log.For("aliases")` at package scope (literal taxonomy component `aliases`).
- **`[needs-info]` — emission point.** `Set`/`Delete` are in-memory and return nothing; persistence is the separate `Save()`. The audit breadcrumb's success/failure hinges on `Save()`, so emission must happen at or after `Save()`, not inside `Set`/`Delete`. Two options — resolve and document: (a) add a combined audited mutation method on the store that performs the in-memory op AND the `Save` AND the emission (e.g. `SetAndSave(name, path, via)` / `DeleteAndSave(name, via)`), keeping all audit logic at the store seam and removing the two-step `Set`+`Save` dance from the callers; or (b) keep `Set`/`Delete` in-memory-only and add the emission inside `Save()`, passing the pending op/alias/value/via into `Save()` (e.g. `Save(audit auditCtx)`). Prefer (a): it keeps the seam a single method (matching the hooks/projects shape), lets the store classify `set`/`modify`/`set-noop` from its own pre-mutation map, and avoids `Save` needing to know the op (Save is also a no-context bulk writer used elsewhere). Flag this for reviewer; do not silently pick — record the chosen shape in a code comment.
- Implement the chosen seam so it classifies `op` from the in-memory map BEFORE applying the mutation:
  - `set`/`modify`/`set-noop` for a set: alias absent → `set`; present and path matches → `set-noop` (DEBUG, skip `Save`); present and path differs → `modify`.
  - `rm` for a delete: `Store.Delete` already returns `false` when the alias is absent. The CLI (`aliasRmCmd`) returns an error ("alias not found") BEFORE calling `Save` when `Delete` returns false — so an absent-alias `rm` never reaches `Save` and emits nothing (the user gets an error). Preserve that: emit the `op=rm` INFO only after a successful `Save` following a `Delete` that returned `true`.
- On `Save()` success (the chosen seam): `logger.Info(op, "alias", name, "value", path, "via", via)` for `set`/`modify` (with `value`); `logger.Info("rm", "alias", name, "via", via)` for `rm` (no `value`). On `set-noop`: `logger.Debug("set-noop", "alias", name, "via", via)` and skip `Save`.
- On `Save()` failure: `logger.Warn(op, "alias", name, ["value", path,] "via", via, "error", err, "error_class", <class>)` and return the error.
- **`[needs-info]` — `error_class` phase mapping.** `alias.Store.Save` uses `os.MkdirAll` + `os.WriteFile` (NOT `fileutil.AtomicWrite`), so the AtomicWrite-phase sentinels from Task 3-1 do not apply directly — there is no temp-create/write/rename phasing. Resolve and document: either (a) classify the `os.WriteFile` failure as `write-failed-write` (the closest phase — it is a direct write) and the `os.MkdirAll` failure as `write-failed-temp-create` (the dir-create prerequisite, mirroring Task 3-1's `MkdirAll` mapping); or (b) migrate `alias.Store.Save` to `fileutil.AtomicWrite` so it shares the same sentinels and gains atomicity (this is a behaviour change — the alias file would become atomically written, which is arguably a latent-correctness improvement, but is out of audit scope without sign-off). Prefer (a) for this task (pure observability, no behaviour change): map `os.WriteFile` failure → `write-failed-write`, `os.MkdirAll` failure → `write-failed-temp-create`, via a small alias-local classifier or by reusing `fileutil.ClassifyWriteError` after wrapping the alias `Save` errors with the Task-3-1 sentinels. Flag option (b) for the reviewer.
- Thread `via=cli` from both callers: `cmd/alias.go` (`aliasSetCmd`/`aliasRmCmd`) and `internal/tui/model.go` (the `aliasEditor.Set`/`Delete`+`Save` path in `handleEditProjectConfirm`). Update the TUI `AliasEditor` interface (`internal/tui/model.go:81`) and its mock to match the chosen seam signature.
- Use only closed attrs: `op` (msg verb), `alias`, `value`, `via`, `error`, `error_class`. No `fmt.Sprintf` in messages. `value` is the alias path.
- Do NOT add logging to a bulk `Save()` that is also used for non-audited contexts; if option (a) is chosen, the audited method is the only emission site.

**Acceptance Criteria**:
- [ ] Setting a new alias emits one INFO `aliases: set alias=<name> value=<path> via=cli` after a successful persist.
- [ ] Setting an existing alias to a different path emits `op=modify` with `value`; to the same path emits DEBUG `op=set-noop` and skips the persist.
- [ ] Removing an existing alias emits INFO `aliases: rm alias=<name> via=cli` with no `value`; removing an absent alias emits nothing (the CLI errors before persist).
- [ ] A persist failure (`os.WriteFile`/`os.MkdirAll`) emits one WARN with `error` (wrapped) and an `error_class` per the documented phase mapping.
- [ ] `via=cli` from both the `cmd/alias.go` and TUI alias-editor callers; the `AliasEditor` interface + mock updated.
- [ ] The emission-point `[needs-info]` (combined method vs Save-with-context) and the `error_class`-phase `[needs-info]` (os.WriteFile mapping vs migrate to AtomicWrite) are resolved explicitly and recorded in code comments + PR description.

**Tests**:
- `"it emits INFO op=set with value and via=cli after persisting a new alias"`
- `"it emits INFO op=modify when the alias exists with a different path"`
- `"it emits DEBUG op=set-noop and skips the persist when the alias path is unchanged"`
- `"it emits INFO op=rm without a value attr for a successful delete"`
- `"it emits nothing when deleting an absent alias (errors before persist)"`
- `"it emits WARN with the documented error_class when os.WriteFile fails"`

**Edge Cases**:
- In-memory `Set`/`Delete` separate from persist — emission point is the persist boundary (`[needs-info]`: combined method vs Save-with-context).
- `Save` uses `os.WriteFile` not `AtomicWrite` — `error_class` phase mapping `[needs-info]` (os.WriteFile→write-failed-write vs migrate to AtomicWrite).
- set/modify/set-noop classified from the pre-mutation in-memory map.
- `Delete` of an absent alias returns false → CLI errors before `Save` → no emission.
- `via=cli` from both callers.
- `value` = alias path; verbatim privacy posture.

**Context**:
> "`aliases` (component `aliases`)" in the closed file scope. (spec § State-mutation audit trail → Files in scope)
>
> "Key identifying the affected entry: … `alias` (aliases)." (spec § Mechanical rule) and "`alias` aliases-store key", "`value` … verbatim new value for `set`/`modify`". (spec § Closed attr-key value space)
>
> "A **whole-mutation WARN** (the store's `AtomicWrite` itself failed, so the write did not persist) carries `error_class` from the AtomicWrite phase space above." (spec § Which `error_class` space applies) — but the alias store uses `os.WriteFile`, not `AtomicWrite` (the `[needs-info]` mapping).
>
> Code: `alias.Store.Set`/`Delete` (`internal/alias/store.go:102,108`) are in-memory; `Save` (`:75`) uses `os.MkdirAll` + `os.WriteFile`. CLI callers: `cmd/alias.go` `aliasSetCmd` (`store.Set` then `store.Save`), `aliasRmCmd` (`store.Delete` returns false → error before `Save`). TUI caller: `internal/tui/model.go:1467,1481,1485` (`aliasEditor.Delete`/`Set`/`Save`).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Files in scope, Seam, Mechanical rule, Closed `op` value space, Which `error_class` space applies, No-op handling, Privacy posture)

---

## portal-observability-layer-3-6 | approved

### Task 3-6: Emit `migrateConfigFile` INFO per migrated file (op=migrate, via=migrate, owning component)

**Problem**: `migrateConfigFile` (`cmd/config.go`) is a one-shot directory-to-directory move of a config file from the old macOS path to the XDG path. It never flows through a store's `Set`/`Rm`, so under the store-method seam the `migrate` op would have no emitter — yet a config file relocating out from under the user is exactly the "a file changed and I don't know why" event the audit trail exists to explain. The spec names it as the single sanctioned non-store emission site.

**Solution**: Emit one INFO per successfully migrated file from `migrateConfigFile`, under the file's owning component (`hooks` / `aliases` / `projects`) with `op=migrate via=migrate`. Because `migrateConfigFile` is a generic file-mover called for all three config files from `configFilePath`, thread the owning component (and the file's identifying key) in from the three `configFilePath` call sites so each migration logs under the correct component. Keep `AtomicWrite` and all other callers audit-unaware — this is the only sanctioned non-store emitter.

**Outcome**: When `migrateConfigFile` actually moves `hooks.json` from the old path to the new XDG path, one INFO `hooks: migrate ... op=migrate via=migrate` lands in `portal.log`; the same for `aliases` (component `aliases`) and `projects.json` (component `projects`); a no-op migration (old path absent OR new path already exists) emits nothing; an `os.Rename`/`os.MkdirAll` failure emits one WARN; no other caller and not `AtomicWrite` emits a `migrate` breadcrumb.

**Do**:
- In `cmd/config.go`, thread the owning component into `migrateConfigFile`. Change its signature to accept the component name (e.g. `migrateConfigFile(oldPath, newPath, component string)`), and update `configFilePath` to pass the component — but `configFilePath` is called with only `(envVar, filename)`, so the component must be derived from the filename or passed by the three callers. Resolve cleanly: map the filename to its component inside `configFilePath` (`"hooks.json"` → `hooks`, `"aliases"` → `aliases`, `"projects.json"` → `projects`) via a small static lookup, OR pass the component from the three `configFilePath` call sites (`hooksFilePath`, `aliasFilePath`, and the projects file-path resolver). Prefer the filename→component lookup inside `configFilePath` (single site, no caller churn); document the closed mapping.
- Bind the logger by component dynamically: since `migrateConfigFile` is generic, it cannot bind a single package-level `var logger = log.For(...)`. Use `log.For(component)` at the emission site (the component is one of the closed `hooks`/`aliases`/`projects` values threaded in). Confirm `log.For` is cheap enough to call per-migration (it is a one-shot; `For` is `root.With("component", name)` — fine for a rare path).
- After the successful `os.Rename(oldPath, newPath)`, emit `log.For(component).Info("migrate", <key-attr>, "op", "migrate", "via", "migrate", "path", newPath)`. The msg verb is `migrate`; `via=migrate` (closed value space); `path` is the destination (or source — pick and document; prefer destination, the file's new home).
- **`[needs-info]` — the per-entry key attr for a whole-file move.** The spec's mechanical rule requires a per-file key attr (`hook_key`/`alias`/`project`). But `migrateConfigFile` moves the WHOLE file, not a single entry — there is no per-entry key (no individual hook key / alias / project path). Resolve and document: either (a) omit the per-entry key attr for the migrate line (a whole-file move has no entry key; the `path` attr identifies the file) and treat the per-file-key requirement as inapplicable to the file-level migrate op; or (b) set the key attr to a sentinel like the filename or `"*"` to signal "whole file". Prefer (a): omit `hook_key`/`alias`/`project` and rely on `component` + `path` to identify the migrated file; document that the per-entry key is inapplicable at the file-move granularity. Flag for reviewer — do not silently invent a fake entry key.
- **`[needs-info]` — the migrate WARN `error_class`.** `migrateConfigFile` fails via `os.MkdirAll` (dir create) or `os.Rename` (the move) — NOT through `AtomicWrite`, so the `write-failed-*` phase space does not naturally apply, and the move is not a per-entry batch op (so `unexpected` per-entry is also a poor fit). Resolve and document: either (a) reuse a `write-failed-*` value by analogy (`os.MkdirAll` failure → `write-failed-temp-create`, `os.Rename` failure → `write-failed-rename`, mirroring Task 3-1's phase names), or (b) classify the migrate WARN as `error_class=unexpected` (it is a swallowed-error log-and-continue path — the migration is best-effort and the function returns void after logging to stderr, which maps to the level-discipline "swallowed unexpected, work dropped → WARN unexpected" shape). Prefer (a) for `os.Rename` (→`write-failed-rename`, the move IS a rename) and `os.MkdirAll` (→`write-failed-temp-create`); flag for reviewer since the migrate path is outside the AtomicWrite phase model. Convert the two existing `fmt.Fprintf(os.Stderr, ...)` warning lines in `migrateConfigFile` to the new WARN log line (keep stderr too if the migration runs before `log.Init`? — note the PR-timing caveat below).
- **PR-timing caveat (accepted):** the spec notes `migrateConfigFile` lands in PR 2 with the rest of the state-mutation work, so a migration firing during a PR-1-only window goes unlogged — an accepted caveat (the migration is a rare idempotent one-shot most users already ran). Record this caveat in a code comment; no action needed beyond the comment.
- Keep `AtomicWrite`, the stores, and every other caller audit-unaware: `migrateConfigFile` is the ONLY sanctioned non-store `migrate` emitter (per spec). No other caller-level `migrate` emission is permitted.
- The no-op paths (oldPath absent, newPath already exists, or the stat error branches) emit NOTHING — the early `return`s stay before any emission.

**Acceptance Criteria**:
- [ ] A successful migration of `hooks.json` emits exactly one INFO under component `hooks` with `op=migrate via=migrate` and a `path` attr; `aliases` → component `aliases`; `projects.json` → component `projects`.
- [ ] A no-op migration (old path absent, OR new path already exists, OR a stat error branch) emits nothing.
- [ ] An `os.Rename` / `os.MkdirAll` failure emits one WARN with `error` and the documented `error_class`.
- [ ] The owning component is correctly threaded from the filename→component mapping (or the three call sites); no migration logs under the wrong component.
- [ ] No other caller and not `AtomicWrite` emits a `migrate` breadcrumb (it is the single sanctioned non-store emitter).
- [ ] The per-entry-key `[needs-info]` (omit vs sentinel) and the migrate-WARN `error_class` `[needs-info]` (write-failed-* by analogy vs unexpected) are resolved explicitly and recorded in code comments + PR description; the PR-1-window-unlogged caveat is noted in a comment.

**Tests**:
- `"it emits one INFO op=migrate via=migrate under component hooks when hooks.json is migrated"`
- `"it emits under component aliases / projects for the respective files"`
- `"it emits nothing when the old path does not exist"`
- `"it emits nothing when the new path already exists"`
- `"it emits one WARN with error_class when os.Rename fails"`
- `"it does not emit a migrate breadcrumb from AtomicWrite or any store method"`

**Edge Cases**:
- Component threaded from the three `configFilePath` sites (or a filename→component lookup) — `hooks`/`aliases`/`projects`.
- No-op when oldPath absent / newPath exists / stat error → no INFO.
- Migrate WARN `error_class` for `os.Rename`/`os.MkdirAll` failure — `[needs-info]` (write-failed-* by analogy vs unexpected).
- Whole-file move has no per-entry key — `[needs-info]` (omit the key attr vs a sentinel).
- PR-1-window migration unlogged — accepted caveat, noted in a comment.
- `AtomicWrite` / other callers stay audit-unaware — `migrateConfigFile` is the only sanctioned non-store `migrate` emitter.

**Context**:
> "**One sanctioned exception: `migrateConfigFile`.** … it emits one INFO per migrated file under that file's owning component (`hooks` / `aliases` / `projects`) with `op=migrate via=migrate`. This is the *only* sanctioned non-store emitter; no other caller-level emission is permitted. **PR timing:** it lands in PR 2 with the rest of the state-mutation work, so a migration firing during the PR-1-only window goes unlogged — an accepted caveat." (spec § State-mutation audit trail → Seam → One sanctioned exception)
>
> "`migrate` One-shot migration from old config path." (spec § Closed `op` value space) and "`via` … `migrate` for the one-shot `migrateConfigFile` path." (spec § Mechanical rule)
>
> Code: `migrateConfigFile` (`cmd/config.go:15`) fails via `os.MkdirAll` (`:26`) and `os.Rename` (`:31`), each currently logging to stderr with `fmt.Fprintf`. `configFilePath` (`:46`) calls it for all three files; callers `hooksFilePath` (`cmd/hooks.go:135`), `aliasFilePath` (`cmd/alias.go:101`), and the projects file-path resolver pass `(envVar, filename)`. There is no per-entry key at file-move granularity.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § State-mutation audit trail for user config files (Seam — One sanctioned exception `migrateConfigFile`, Mechanical rule, Closed `op` value space)

---
phase: 4
phase_name: Resume-hook lifecycle migration
total: 7
---

## built-in-session-resurrection-4-1 | approved

### Task 4-1: Add `hooks.LookupByKey` helper for on-resume lookup by saved structural key

**Problem**: Phase 3's hydrate helper (tasks 3-8 through 3-10) terminates with `exec $SHELL` on every success path. Phase 4 needs to fold hook firing back in — but the helper must look up hooks by the **saved** structural key passed through `--hook-key`, not the helper's live pane position (base-index / pane-base-index drift makes live position wrong for lookup). Today's `hooks.Store.Get(key)` returns the whole event map; the helper only needs the `on-resume` command and needs a clean "no hook registered" signal that is distinct from an I/O error. Splitting a focused lookup helper now keeps task 4-2 (wiring the lookup into the helper) mechanical and isolates the "malformed JSON returns empty — same as Store.Load" contract for a dedicated test.

**Solution**: Add `func LookupOnResume(store *Store, hookKey string) (command string, found bool, err error)` in `internal/hooks/store.go` (or a sibling `internal/hooks/lookup.go`). It wraps `store.Load()` and reads `h[hookKey]["on-resume"]`. Missing file / malformed JSON returns `("", false, nil)` — the "no hook" signal — consistent with how `Store.Load` silently maps both conditions to an empty map. I/O errors other than "file not found" propagate wrapped so the helper can log and degrade distinctly from "no hook registered."

**Outcome**: The hydrate helper in task 4-2 calls `command, found, err := hooks.LookupOnResume(store, cfg.HookKey)` and branches: `err != nil` → log-and-degrade to `$SHELL`; `found && command != ""` → exec hook chain; else → exec `$SHELL`. Unit tests cover every branch without touching the filesystem.

**Do**:
- Add `LookupOnResume(store *Store, hookKey string) (string, bool, error)` to `internal/hooks/store.go`:
  1. `h, err := store.Load()`; on `err != nil` return `("", false, fmt.Errorf("load hooks: %w", err))`.
  2. `events, ok := h[hookKey]`; on `!ok` return `("", false, nil)`.
  3. `cmd, ok := events["on-resume"]`; on `!ok || cmd == ""` return `("", false, nil)`.
  4. Return `(cmd, true, nil)`.
- Keep the function on the existing `*Store` receiver-alternative — a standalone function — so tests can supply any `*Store` built from a temp-dir file. Do NOT change `Store.Load` semantics; lean on its existing behaviour (missing file → empty map, nil err; malformed JSON → empty map, nil err, per lines 36–51 of `internal/hooks/store.go`).
- Tests in `internal/hooks/lookup_test.go` using `t.TempDir()`:
  - `hooks.json` missing → `("", false, nil)`.
  - `hooks.json` malformed (invalid JSON) → `("", false, nil)` (mirrors `Store.Load` behaviour).
  - `hooks.json` valid but key absent → `("", false, nil)`.
  - Key present without `on-resume` event → `("", false, nil)`.
  - Key present with `on-resume` event → `(command, true, nil)` verbatim.
  - Key present with empty-string `on-resume` command → `("", false, nil)` (defensive — empty command should not trigger exec).
  - Raw session name containing `:` (e.g., key `work:foo:0.0`) round-trips verbatim — no splitting, no re-parsing.
  - Injected I/O error distinct from "missing file": test by creating a directory at the hooks-file path so `os.ReadFile` returns `EISDIR`; expect `err != nil` with `"load hooks"` prefix, `found=false`.

**Acceptance Criteria**:
- [ ] `LookupOnResume` returns `("", false, nil)` when `hooks.json` does not exist.
- [ ] `LookupOnResume` returns `("", false, nil)` when `hooks.json` is malformed (matches `Store.Load`'s silent-empty-map contract).
- [ ] `LookupOnResume` returns `("", false, nil)` when the hook-key is absent.
- [ ] `LookupOnResume` returns `("", false, nil)` when the key exists but has no `on-resume` event or has an empty-string command.
- [ ] `LookupOnResume` returns `(command, true, nil)` when the key has a non-empty `on-resume` command.
- [ ] Raw session names containing `:` (hook keys like `work:foo:0.0`) are looked up verbatim — no tokenisation.
- [ ] Genuine I/O errors (e.g., `EISDIR`) propagate as non-nil wrapped errors with a `"load hooks"` prefix.
- [ ] No logging, no stderr writes — pure value-returning function.

**Tests**:
- `"it returns no-hook when hooks.json is missing"`
- `"it returns no-hook when hooks.json is malformed JSON"`
- `"it returns no-hook when the hook-key is absent"`
- `"it returns no-hook when the key has no on-resume event"`
- `"it returns no-hook when the on-resume command is the empty string"`
- `"it returns the command verbatim when on-resume is registered"`
- `"it round-trips hook keys containing colons in the session name"`
- `"it surfaces a wrapped I/O error distinct from the no-hook case"`

**Edge Cases**:
- Missing file and malformed JSON both degrade to "no hook" (consistent with `Store.Load`) — the helper must not be louder than the store itself.
- Empty command string is treated as "no hook" so we never exec `sh -c ''; exec $SHELL` (which would shell-fork a no-op extra level).
- Hook keys are raw session names — no assumption that session names are `:`-free. Session-name-contains-colon is rare but legal in tmux, and hooks.json keys are content-addressable by the raw session identifier.
- The function is side-effect-free (no logs, no stderr) so task 4-2 owns the single log line when I/O fails.
- I/O error is distinguishable from no-hook so the helper can log "lookup failed, degrading to shell" rather than silently dropping a potentially-registered hook.

**Context**:
> Spec "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain":
> "After the helper has dumped scrollback, slept 100ms, and unset its skeleton marker ... the helper:
> 1. Reads `hooks.json`.
> 2. Looks up the resume hook for this pane's structural key (`session:window.pane`).
> 3. If a hook exists → `exec sh -c 'HOOK; exec $SHELL'`.
> 4. If no hook exists → `exec $SHELL` directly."
>
> Spec "Save Format & Schema → Helper hook lookup under index drift":
> "The helper is invoked with a `--hook-key '<raw-session>:<saved-window>.<saved-pane>'` flag populated from `sessions.json` at bootstrap. The helper uses that flag (not its own live position) to look up hooks in `hooks.json`. This preserves hooks across `base-index`/`pane-base-index` changes between save and restore."
>
> Spec "Save Format & Schema → Canonical paneKey": "**Hook structural keys** (`session:window.pane` in `hooks.json`) use the **raw** (un-sanitized) session name, window index, and pane index." So the lookup key is verbatim — no sanitization, no tokenisation.
>
> Existing `internal/hooks/store.go` `Store.Load` (lines 36–51) silently treats missing file and malformed JSON alike (both → empty map, nil error). `LookupOnResume` inherits that contract.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain", "Save Format & Schema → Helper hook lookup under index drift", "Save Format & Schema → Canonical paneKey".

## built-in-session-resurrection-4-2 | approved

### Task 4-2: Wire hook firing into hydrate helper's signal-arrived and file-missing success paths

**Problem**: Phase 3 landed the hydrate helper with a terminal `exec $SHELL` on every path (tasks 3-8 signal-arrived, 3-9 timeout, 3-10 file-missing). The spec pins hook firing to two of those three paths — signal-arrived (successful dump) and file-missing (empty pane, but marker cleared and helper proceeds) — and explicitly excludes the timeout path ("next attach will re-signal and retry" means hooks must not fire on a pane that never received its signal). The helper must read `hooks.json` **after** the 100ms settle sleep and marker-unset (signal-arrived path) or immediately after clearing the marker (file-missing path), look up by `--hook-key` (the saved structural identifier — not the helper's live pane position, which can drift), and `exec sh -c 'HOOK; exec $SHELL'` on match or `$SHELL` on no-match / lookup-error.

**Solution**: Replace the `execShell(cfg)` terminal call in the signal-arrived branch (task 3-8) and the file-missing branch (task 3-10) with a single new factored function `execShellOrHook(cfg HydrateConfig)`. This function calls `hooks.LookupOnResume(store, cfg.HookKey)` (task 4-1), logs and falls through to `exec $SHELL` on any error, execs `sh -c '<HOOK>; exec $SHELL'` when a non-empty hook command is returned, and execs `$SHELL` otherwise. The timeout branch (task 3-9) continues to call `execShell(cfg)` directly — this task must leave `handleHydrateTimeout` untouched so the timeout path never fires hooks.

**Outcome**: A skeleton-restored pane with a registered `on-resume` hook receives: (scrollback dump) → (settle sleep) → (marker unset) → `exec sh -c 'HOOK; exec $SHELL'`. The hook runs; on exit the chained `exec $SHELL` takes over. If no hook is registered, the user lands in a plain shell. On FIFO timeout the helper exec's `$SHELL` bare — hooks never fire for unattended panes so long-running processes (`claude --resume`) are not spuriously re-launched on the next attach. Integration test in task 3-13 (already landed) is extended here to assert hook-firing behaviour on round-trip restore.

**Do**:
- Extend `HydrateConfig` (defined in task 3-8 at `cmd/state_hydrate.go`) with a `HookStore *hooks.Store` field (or equivalent injection seam). Production path wires this from `loadHookStore()` (see `cmd/hooks.go:149`); test path substitutes a `*hooks.Store` pointing at a temp-dir `hooks.json`.
- Add `func execShellOrHook(cfg HydrateConfig) error` in `cmd/state_hydrate.go`:
  1. `command, found, err := hooks.LookupOnResume(cfg.HookStore, cfg.HookKey)`.
  2. If `err != nil` → `cfg.Logger.Printf("hydrate: hook lookup failed for --hook-key=%s: %v (degrading to shell)", cfg.HookKey, err)`; fall through to `execShell(cfg)`.
  3. If `found && command != ""` → `execReplacer("/bin/sh", []string{"sh", "-c", command + "; exec " + shellPath(cfg)}, os.Environ())` where `shellPath` resolves `$SHELL` with `/bin/sh` fallback (factor out of task 3-8's existing logic so both paths use the same resolver).
  4. Else → `execShell(cfg)`.
- In the signal-arrived branch (task 3-8 code): replace the terminal `execShell(cfg)` call with `execShellOrHook(cfg)`. Keep the preceding 100ms sleep + `UnsetServerOption` calls unchanged — lookup happens **after** those steps, matching spec helper step-order (dump → sleep → unset marker → lookup → exec).
- In the file-missing branch (task 3-10 code, in `handleHydrateFileMissing` or equivalent): replace the terminal `execShell(cfg)` call with `execShellOrHook(cfg)`. The marker-unset has already happened on this path (task 3-10 acceptance criterion — marker cleared inline before exec). No sleep — nothing was dumped.
- Do NOT touch `handleHydrateTimeout` (task 3-9). Its terminal `execShell(cfg)` must remain — hooks never fire on timeout.
- Hook-command exec safety: the command string is passed as a single positional argument to `sh -c`. `sh`'s own parser handles any embedded single quotes the user registered. Portal does not sanitise; we never string-interpolate the command into a shell-command-line. The argv construction is `[]string{"sh", "-c", cmd + "; exec " + shell}` — `cmd` sits in its own argv slot, immune to outer quoting drift.
- Tests in `cmd/state_hydrate_test.go` (extending the existing signal-arrived and file-missing suites):
  - Signal-arrived path, hook registered → exec target is `/bin/sh` with argv `["sh", "-c", "claude --resume abc; exec /bin/zsh"]` (verify via stubbed `execReplacer`).
  - Signal-arrived path, no hook registered → exec target is `$SHELL` (e.g., `/bin/zsh`) with argv `[$SHELL]`.
  - Signal-arrived path, hook registered but `LookupOnResume` returns an error → warning log written; exec target is `$SHELL` bare (degradation).
  - Signal-arrived path, hook command contains single quotes (`echo 'it works'`) → exec argv's third element is the verbatim command string + `; exec /bin/zsh`; no manual escaping, no shell-command-line interpolation.
  - File-missing path with registered hook → same hook chain exec (test with a scrollback path that does not exist on disk).
  - File-missing path without registered hook → bare `$SHELL` exec.
  - Timeout path with registered hook → exec target is `$SHELL` bare; `hooks.LookupOnResume` is NEVER called (assert via panicking hook store).
  - Lookup uses `--hook-key` flag verbatim: test passes a `HookKey` string different from what `paneKeyFromFIFOPath(cfg.FIFO)` would derive (simulate base-index drift); assert the lookup key matches `HookKey` and not the FIFO-derived live key.

**Acceptance Criteria**:
- [ ] Signal-arrived path with registered `on-resume` hook exec's `sh -c '<HOOK>; exec $SHELL'`.
- [ ] Signal-arrived path without a hook exec's bare `$SHELL`.
- [ ] File-missing path with a hook exec's the same hook chain.
- [ ] File-missing path without a hook exec's bare `$SHELL`.
- [ ] 3-second timeout path NEVER reads `hooks.json` and NEVER fires a hook — exec target is bare `$SHELL`.
- [ ] Hook lookup I/O error degrades to bare `$SHELL` with a single warning log line; helper still exec's the shell.
- [ ] Hook lookup uses `cfg.HookKey` verbatim — NOT `paneKeyFromFIFOPath(cfg.FIFO)`, NOT any other re-derivation — verified by a test where the two differ.
- [ ] Hook command containing single quotes is exec-safe: the command is passed as a single argv element to `sh -c`, not string-interpolated into a shell command line.
- [ ] Signal-arrived path: lookup happens AFTER the 100ms sleep AND AFTER `UnsetServerOption` — call order verified via mock recorder.
- [ ] File-missing path: lookup happens AFTER `UnsetServerOption` (marker already cleared); no 100ms sleep precedes lookup.
- [ ] The 100ms settle sleep, marker-unset, and FIFO unlink behaviour from tasks 3-8 / 3-10 are preserved exactly — this task only replaces the terminal `execShell(cfg)` call.

**Tests**:
- `"it execs the hook chain on signal-arrived when an on-resume hook is registered"`
- `"it execs $SHELL on signal-arrived when no hook is registered"`
- `"it execs the hook chain on file-missing when an on-resume hook is registered"`
- `"it execs $SHELL on file-missing when no hook is registered"`
- `"it never reads hooks.json on the 3-second timeout path"`
- `"it logs a warning and degrades to $SHELL when hook lookup fails"`
- `"it looks up hooks by --hook-key verbatim, not by the FIFO-derived live paneKey"`
- `"it passes the hook command as a single argv element to sh -c (single-quote safety)"`
- `"it performs the lookup after the 100ms sleep and the marker-unset on the signal-arrived path"`
- `"it performs the lookup after the marker-unset on the file-missing path"`

**Edge Cases**:
- Single-quote safety: the spec example `claude --resume <uuid>` and real-world hooks containing shell metacharacters must pass through `sh -c <cmd>`'s single-argv-slot semantics — never build a shell command string by concatenation.
- `$SHELL` resolution: reuse `shellPath(cfg)` from task 3-8 — `/bin/sh` fallback when unset. Both hook and no-hook paths share the same resolver.
- Hook command is empty string (task 4-1 guards this as "no hook") → helper exec's bare `$SHELL`. The lookup layer is the single authority here.
- Hook command `exec` failure (binary missing): per spec "Failure Modes → What Is Explicitly NOT Handled Specially", the `exec sh -c 'HOOK; exec $SHELL'` falls through `sh`'s own `;` chain to `$SHELL` on hook-binary failure — no Portal-side recovery needed.
- Timeout path stays an empty-shell-only flow because the pane never received scrollback signal; firing `claude --resume` on a pane where no scrollback was ever dumped would contradict the reboot-recovery-only hook semantics.
- Base-index / pane-base-index drift: `cfg.HookKey` is set from `sessions.json` at bootstrap (task 3-3) and carried through the helper's argv; `paneKeyFromFIFOPath(cfg.FIFO)` yields the LIVE paneKey used for the marker-unset. They may differ; the test enforces the distinction.

**Context**:
> Spec "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain":
> "Resume hooks fire **only** from inside the hydrate helper's exec chain, at the end of successful hydration. There is no attach-time hook firing, no `send-keys` involvement, and no shell-readiness polling."
>
> Spec "Scrollback Restore Mechanics → Helper Behavior on Startup":
> "2. On signal arrival: ... (g) `tmux set-option -su @portal-skeleton-<paneKey>` (remove marker via -u flag — load-bearing; empty-string assignment does NOT remove). (h) If hook exists: `exec sh -c 'HOOK; exec $SHELL'`. Else: `exec $SHELL`."
> "3. On 3-second timeout (no signal arrived): ... (e) `exec $SHELL` (bare shell; no hook firing on this path)."
> "4. On scrollback file missing / unreadable (detected at step 2c of the signal path): ... (d) `tmux set-option -su @portal-skeleton-<paneKey>` — remove the marker inline so the save loop resumes capturing this empty pane. (e) Continue to step h (hook/shell exec). Hook runs if registered; else bare shell."
>
> Spec "Save Format & Schema → Helper hook lookup under index drift": "The helper is invoked with a `--hook-key '<raw-session>:<saved-window>.<saved-pane>'` flag populated from `sessions.json` at bootstrap. The helper uses that flag (not its own live position) to look up hooks in `hooks.json`."
>
> Spec "Resume Hook Firing → Why Firing Belongs Only in the Helper":
> "Within a single server lifetime, a pane that still exists does not need its hook re-fired — the hook's process either still exists or was explicitly killed by the user. Firing `claude --resume <uuid>` on a detach/reattach of a pane that already has Claude running would actively break things."
>
> Phase 3 tasks 3-8 / 3-9 / 3-10 deliberately left a `TODO(Phase 4)` / deferred-hook-firing marker at the terminal `exec $SHELL` call; this task collapses that deferral.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain", "Scrollback Restore Mechanics → Helper Behavior on Startup", "Save Format & Schema → Helper hook lookup under index drift".

## built-in-session-resurrection-4-3 | approved

### Task 4-3: Implement `portal state migrate-rename <old> <new>` body — rewrite `<old>:*` keys in `hooks.json`

**Problem**: Hook structural keys are `session:window.pane`. When a user runs `tmux rename-session` the saved hooks for the old name stop matching any live pane — `CleanStale` would otherwise prune them on the next bootstrap, silently destroying the user's registrations. The spec requires a **separate internal subcommand** (`portal state migrate-rename`, stubbed in Phase 1 task 1-1) that the `session-renamed` tmux hook fires alongside `portal state notify`. The subcommand's body rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically. `portal state notify` stays minimal per spec; migration lives in its own subcommand invoked by its own hook entry.

**Solution**: Implement `cmd/state_migrate_rename.go`'s `RunE` to: load `hooks.json`, iterate keys whose session-segment (everything before the first `:`) equals `<old>`, rebuild them as `<new>:<window>.<pane>` preserving the inner `map[string]string` event map verbatim, write atomically via the existing `Store.Save` (which already uses `fileutil.AtomicWrite` — see `internal/hooks/store.go:55-62`). Zero matching keys → no write (avoid touching `AtomicWrite`'s disk surface for a no-op). Collisions — a `<new>:X` key already present — are overwritten by the migrated `<old>:X` entry with a log warning (best-effort; the user's intent with the renamed session takes precedence). Malformed `hooks.json` is treated the same way `Store.Load` already treats it (empty map, proceed as no-op).

**Outcome**: `portal state migrate-rename old-name new-name` is a 20-30-line `RunE` that rewrites the hooks file atomically on rename. `tmux rename-session old new` fires the `session-renamed` hook, which invokes this subcommand, and the user's hooks for the renamed session stay addressable by the new name. Zero impact on other keys. Best-effort on I/O failure with a logged warning — no crash, no retry storm.

**Do**:
- Replace the stub `RunE` in `cmd/state_migrate_rename.go` (landed in Phase 1 task 1-1) with the full migration body:
  1. `oldName, newName := args[0], args[1]`. Both are `cobra.ExactArgs(2)`-validated upstream.
  2. Reject empty `newName` with `NewUsageError("migrate-rename: <new> must not be empty")`. (Cobra args validation only enforces count; an empty string is still zero-length.)
  3. `store, err := loadHookStore()`; propagate error.
  4. `h, err := store.Load()`; `Store.Load` silently treats missing file and malformed JSON alike (both → empty map). If `err != nil` from genuine I/O failure, log a best-effort warning and return `err` so the exit code reflects failure, per spec "best-effort logging on failure" combined with spec "failure mode: ... hooks for the renamed session get orphaned and pruned by the next CleanStale run."
  5. Collect `migrated := false` and iterate `h`:
     ```go
     prefix := oldName + ":"
     for key, events := range h {
         if !strings.HasPrefix(key, prefix) {
             continue
         }
         rest := key[len(prefix):]     // "window.pane"
         newKey := newName + ":" + rest
         delete(h, key)
         if _, collision := h[newKey]; collision {
             // log best-effort warning; overwrite anyway — the renamed session
             // is the authoritative source going forward.
             fmt.Fprintf(os.Stderr, "portal: migrate-rename collision: %s → %s (overwriting)\n", key, newKey)
         }
         h[newKey] = events
         migrated = true
     }
     ```
  6. If `!migrated` → return `nil` (no write; clean no-op).
  7. Else `return store.Save(h)` — surface any I/O error. `Save` uses `AtomicWrite` (temp + rename) so mid-write failures do not corrupt the file.
- Prefix match on `<oldName> + ":"` is load-bearing: naive `strings.HasPrefix(key, oldName)` would catch `old` vs `old-2` both (per edge case). The trailing colon anchors the prefix to the session segment.
- Session names containing `:` round-trip verbatim — the first `:` in the key separates the session segment from the window.pane suffix per the existing hooks-key contract (see `internal/hooks/store.go` docstrings and Phase 3 task 3-3 `--hook-key` semantics). This task's prefix match uses `oldName + ":"` as a literal-string prefix; any `:` characters inside the session name are part of `oldName` and match correctly.
- Do NOT call `tmux show-hooks`, `list-panes`, or any other tmux API. The subcommand is invoked from a tmux hook and must not recurse into the bootstrap chain. Phase 1 task 1-3 already put `state` in `skipTmuxCheck`, so `PersistentPreRunE` short-circuits for this subcommand.
- Tests in `cmd/state_migrate_rename_test.go` using `PORTAL_HOOKS_FILE` env-var override (matches the existing `cmd/hooks_test.go` pattern):
  - Zero matching keys → `hooks.json` mtime unchanged (capture stat before and after; verify `Nanoseconds()` equal). Exit code 0.
  - Single matching key `work:0.0` → after run, `hooks.json` contains `new-name:0.0` with the same event map; `work:0.0` absent.
  - Multiple matching keys (`work:0.0`, `work:1.0`, `work:1.2`) → all three migrated.
  - Session-name prefix ambiguity: keys `work:0.0` and `work-2:0.0` with `oldName="work"` → only `work:0.0` is migrated; `work-2:0.0` stays.
  - Collision: `hooks.json` pre-populated with both `work:0.0` and `new-name:0.0` having distinct commands; run `migrate-rename work new-name` → `new-name:0.0` ends up with the `work:0.0` command (overwrite); stderr contains the collision warning.
  - Malformed `hooks.json` → `Store.Load` returns empty map, nil error; no matching keys; exit 0; no write. Assert mtime unchanged.
  - Missing `hooks.json` → same as malformed (empty map, no write).
  - `AtomicWrite` failure simulated via read-only directory → exit non-zero; warning logged; no partial file on disk (temp file cleaned up by `AtomicWrite`).
  - Empty `<new>` rejected with usage error (exit 1, no write).
  - Session name containing `:` → pre-populate `hooks.json` with `foo:bar:0.0`, invoke `migrate-rename foo:bar baz:qux` → key becomes `baz:qux:0.0`.

**Acceptance Criteria**:
- [ ] `portal state migrate-rename <old> <new>` rewrites every key matching `<old>:<window>.<pane>` to `<new>:<window>.<pane>` atomically via `Store.Save` → `fileutil.AtomicWrite`.
- [ ] Zero matching keys is a clean no-op — no disk write, mtime unchanged.
- [ ] Multiple matching keys across different `window.pane` combinations all get migrated in a single run.
- [ ] Session-name prefix ambiguity is disambiguated by the trailing `:` in the prefix — `old` does NOT match `old-2`.
- [ ] Collision (`<new>:X` already exists) overwrites with the migrated `<old>:X` entry and logs a warning to stderr; exit 0 still.
- [ ] Malformed or missing `hooks.json` is a no-op (inherits `Store.Load`'s empty-map contract).
- [ ] `AtomicWrite` failure propagates as non-zero exit and a best-effort stderr warning.
- [ ] Missing positional arg(s) produces `cobra.ExactArgs(2)` error with non-zero exit.
- [ ] Empty-string `<new>` is rejected with a usage error before any load/save.
- [ ] Session names containing `:` round-trip verbatim — the first `:` in the key separates the session segment from `window.pane`, and `<old>+":"` prefix matching handles embedded colons correctly.
- [ ] No tmux API calls made — the subcommand is `state`-prefixed and bypasses bootstrap.

**Tests**:
- `"it rewrites a single matching key under the new session name"`
- `"it rewrites multiple matching keys in one run"`
- `"it leaves unrelated keys untouched"`
- `"it is a clean no-op when zero keys match (no file write)"`
- `"it disambiguates prefix ambiguity via the trailing colon"`
- `"it logs a warning and overwrites on collision with an existing <new>:X key"`
- `"it treats malformed hooks.json as empty (no-op, no write)"`
- `"it treats missing hooks.json as empty (no-op, no write)"`
- `"it rejects empty <new> with a usage error"`
- `"it rejects missing positional args with cobra validation"`
- `"it propagates AtomicWrite failure with non-zero exit and a warning"`
- `"it preserves the inner event map verbatim across the rename"`
- `"it round-trips keys whose session segment contains a colon"`
- `"it does not call any tmux API"`

**Edge Cases**:
- Session-name-prefix ambiguity (`old` vs `old-2`): prefix-match must use `oldName + ":"` (trailing colon) to anchor on the session segment; naive `strings.HasPrefix(key, oldName)` would be wrong.
- Collision on `<new>:X` already registered (user had hooks on both old and new names simultaneously — rare but possible): the spec's "best-effort on failure" principle says we warn and continue; the migrated entry overwrites because the renamed session is the authoritative surface. Document so users understand the precedence.
- Malformed / missing `hooks.json`: `Store.Load` returns empty map + nil error for both; the subcommand's no-op behaviour follows naturally with no special-casing.
- `AtomicWrite` failure: per spec "best-effort logging on failure" — warn + non-zero exit, but do not retry. The next successful save resumes from the current (possibly-orphaned) hooks.
- Empty `<new>` creates malformed keys (`":0.0"`); reject at the command boundary.
- Session names containing `:` are rare but legal; prefix match with trailing colon handles them correctly. Covered by explicit test.
- Zero matching keys: skipping the write preserves mtime, which both avoids unnecessary disk churn and signals to tooling that nothing changed.
- No tmux recursion: the subcommand is invoked by the `session-renamed` hook's `run-shell`; if bootstrap fired it would recursively register hooks and call tmux. Phase 1 task 1-3 already covers this via `skipTmuxCheck["state"] = true`.

**Context**:
> Spec "Resume Hook Firing → Session Rename: Hook Key Migration":
> "Portal registers a **separate internal subcommand** — `portal state migrate-rename <old-name> <new-name>` — against the `session-renamed` tmux hook, in addition to the existing `portal state notify` registration. The two hooks coexist on the same event via the same content-based idempotency pattern applied to every other hook. `migrate-rename` reads `hooks.json`, rewrites every key matching `<old-name>:*` to `<new-name>:*`, and writes via `AtomicWrite`. `portal state notify` stays minimal (no tmux reads, no hooks.json reads)."
>
> Spec "Resume Hook Firing → Session Rename → Failure mode":
> "If migration fails (malformed names, I/O error), hooks for the renamed session get orphaned and pruned by the next `CleanStale` run. User-visible recovery: re-register the hook against the new session name. Migration is best-effort — no retry storm on failure."
>
> Spec "Save Format & Schema → Canonical paneKey": "Hook structural keys (session:window.pane in hooks.json) use the raw (un-sanitized) session name, window index, and pane index. Hooks.json is JSON, so any character valid in a session name is valid in the key." Session names with `:` are thus valid; prefix match must cope.
>
> Existing code: `internal/hooks/store.go:55-62` — `Store.Save` uses `fileutil.AtomicWrite`. `internal/hooks/store.go:36-51` — `Store.Load` treats missing file and malformed JSON identically (empty map, nil err). This task inherits both behaviours.
>
> Phase 1 task 1-1 registered the stub `state migrate-rename` command with `cobra.ExactArgs(2)`; this task only fills in the body.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Resume Hook Firing → Session Rename: Hook Key Migration", "Save Format & Schema → Canonical paneKey".

## built-in-session-resurrection-4-4 | approved

### Task 4-4: Register `portal state migrate-rename` on `session-renamed` alongside `notify` via content-based idempotency

**Problem**: Task 4-3 implements the migration body, but the body is dead code unless tmux actually invokes it when a session is renamed. Phase 1 task 1-7 registered one Portal entry on `session-renamed`: `portal state notify`. Phase 4 needs a second Portal entry on the same event: `portal state migrate-rename`. The two must coexist; both fire when `session-renamed` fires. The content-based idempotency pattern (Phase 1 task 1-6 `RegisterHookIfAbsent`) already supports per-event-per-substring scoping.

**Solution**: Extend `internal/tmux/hooks_register.go` (task 1-7's module) with a third event table — `migrateRenameEvents = []string{"session-renamed"}` — and register `portal state migrate-rename '<OLD>' '<NEW>'` against it. The argv wiring for `<OLD>` and `<NEW>` MUST satisfy the spec's contract: "hook keys are migrated atomically on rename events" (spec "Resume Hook Firing → Session Rename → Argument source"). The spec offers two routes:

- **Route A** — Use `session-renamed` hook's exposed variables. The spec confirms `#{hook_session_name}` exposes the new name; the prior name is NOT reliably available via `#{client_last_session}`. Planning must identify a reliable tmux 3.0+ variable for the prior name (research item — likely requires reading tmux's `format.c` for the `session-renamed` hook payload) OR confirm none exists.
- **Route B** — Build a daemon-side rename delta. The daemon's tick (Phase 2 task 2-12) already enumerates `list-sessions`. Adding a "previous-tick session-name set" comparison yields a `(old, new)` pair on rename. Persist the delta to a side-band file (e.g., `~/.config/portal/state/pending-renames.log`); the `migrate-rename` subcommand pops the oldest matching `<new>` to obtain `<old>`. This is the "in-memory 'last-seen names' map maintained by the daemon" the spec mentions.

**[needs-info]**: Planning has not pinned which route to take. Both have implementation cost; both achieve the spec's contract. The original Phase 4 task body pinned a third option — registering with `#{hook_session_name}` for BOTH old and new args, making `migrate-rename` a structural no-op — that violates the spec's atomic-migration contract and is rejected.

Until planning pins Route A or Route B, this task is BLOCKED. Phase 4 task 4-3 (`migrate-rename` body) has already shipped and is correct in isolation; only the registration's argv source is undecided.

**Do**:
- (Once route is chosen) Define `migrateRenameEvents`, `migrateRenameCommand`, `migrateRenameSubstring`.
- (Once route is chosen) Extend `RegisterPortalHooks(c *Client)` to iterate the new event table and call `RegisterHookIfAbsent(c, "session-renamed", migrateRenameSubstring, migrateRenameCommand)`.
- The `command -v portal` defensive guard wraps the invocation.

**Acceptance Criteria**:
- [ ] Planning has pinned Route A or Route B for the prior-name argument source.
- [ ] If Route A: the chosen tmux format variable is verified empirically against tmux 3.0–3.5 to expose the prior name reliably on `session-renamed` fire.
- [ ] If Route B: a Phase 2 follow-up task is filed to add the daemon-side rename-delta tracking and side-band file; this task lands only after that follow-up.
- [ ] After landing, `tmux rename-session old new` causes `portal state migrate-rename old new` to fire with the actual old and new names — verified by integration test that registers a hook on `old:0.0`, renames `old → new`, and asserts the hook is now keyed `new:0.0` in `hooks.json`.
- [ ] Idempotent re-registration produces zero additional `set-hook -ga` calls for the `migrate-rename` entry.
- [ ] `notify` and `migrate-rename` Portal entries coexist on `session-renamed` with no cross-contamination.

**Tests**: (deferred until route pinned)

**Edge Cases**: (deferred until route pinned)

**Context**:
> Spec "Resume Hook Firing → Session Rename → Argument source": "Planning-phase decides the exact wiring; the contract from the spec: hook keys are migrated atomically on rename events, the migration path is a distinct subcommand (not `notify`), and best-effort logging on failure."
>
> Spec "Resume Hook Firing → Session Rename → Failure mode": failure mode is for I/O errors / malformed names — not a degraded happy-path. A registration whose argv cannot supply the prior name does not satisfy the contract.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Resume Hook Firing → Session Rename: Hook Key Migration".

<!-- ORIGINAL TASK BODY (rejected — argv pinned to #{hook_session_name} for both args, making migration a no-op):

OLD_PROBLEM: Task 4-3 implements the migration body, but the body is dead code unless tmux actually invokes it when a session is renamed. Phase 1 task 1-7 registered one Portal entry on `session-renamed`: `portal state notify`. Phase 4 needs a second Portal entry on the same event: `portal state migrate-rename`. They must coexist: `notify` touches the dirty flag (capture the rename structurally on the next tick), `migrate-rename` rewrites hook keys. Both fire when `session-renamed` fires. The content-based idempotency pattern (Phase 1 task 1-6 `RegisterHookIfAbsent`) already supports per-event-per-substring scoping, so adding a second distinct substring (`portal state migrate-rename`) on the same event is the spec's chosen mechanism. Phase 1 task 1-8 already handles removing both Portal entries on that event in reverse index order; this task adds the second registration path.

**Solution**: Extend `internal/tmux/hooks_register.go` (task 1-7's module) with a third event table — `migrateRenameEvents = []string{"session-renamed"}` — and corresponding command / substring constants. `RegisterPortalHooks` iterates this new table alongside the save-trigger and hydration-trigger tables. The command expands `#{hook_session_name}` (current name, i.e., the new name post-rename) and references `#{hook_session_changed_name}` for the prior name — or uses whatever `session-renamed` exposes in tmux ≥ 3.0 (planning decision: the spec says "Planning-phase decides the exact wiring"; this task pins it). Registration is content-based idempotent against the substring `portal state migrate-rename`, distinct from `portal state notify`, so the two Portal entries on `session-renamed` coexist without cross-contamination.

**Outcome**: After bootstrap, `tmux show-hooks -g` on a rename-event shows two Portal entries (the existing `notify` + the new `migrate-rename`) plus any unrelated user/plugin entries. Running `tmux rename-session old new` fires both Portal run-shell invocations. `notify` touches `save.requested`. `migrate-rename` rewrites `hooks.json`. Re-running bootstrap produces zero duplicate entries.

**Do**:
- In `internal/tmux/hooks_register.go`, add:
  ```go
  var migrateRenameEvents = []string{"session-renamed"}

  const (
      migrateRenameCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'"`
      migrateRenameSubstring = "portal state migrate-rename"
  )
  ```
  **Argument wiring decision** (the spec explicitly defers this to planning): tmux's `session-renamed` hook exposes `#{hook_session_name}` (the session being renamed, which at hook-fire time reflects the NEW name — the event fires post-rename). The PRIOR name is NOT reliably available in the `session-renamed` hook payload on tmux 3.0 (per spec note: "the prior name via `#{client_last_session}` is not reliable"). This task therefore invokes `migrate-rename` with **both arguments set to the same `#{hook_session_name}`**, and task 4-3's `RunE` body acts as a no-op when old == new (zero matching keys because the `<new>:*` keys already exist under that name). This is a degenerate but correct registration; the migration's user-visible behaviour is achieved via a DIFFERENT mechanism — Portal's own daemon tracks session-name transitions via its save-tick (the daemon captures `list-sessions` output every tick; comparing ticks gives the rename delta).
  
  WAIT — reviewing the spec more carefully. The spec says: "Planning-phase decides the exact wiring; the contract from the spec: hook keys are migrated atomically on rename events, the migration path is a distinct subcommand (not `notify`), and best-effort logging on failure." The spec also explicitly hedges: "tmux versions vary in what is accessible" and "uses the `session-renamed` hook's exposed variables".
  
  **Planning decision pinned here**: register `migrate-rename` with the arguments wired via tmux's format expansion. Use `#{hook_session_name}` for the current (new) name and wire the prior name via a **daemon-maintained rename queue** written to `~/.config/portal/state/pending-renames.log` — the daemon observes `list-sessions` deltas each tick and appends `<old> <new>` lines; `portal state migrate-rename` pops the oldest matching `<new>` from the queue to obtain the `<old>` name. This side-band mechanism is reliable in a way the tmux hook payload is not.
  
  **Scope guard for Phase 4**: this task limits itself to registering the `session-renamed` hook with the `portal state migrate-rename '<OLD>' '<NEW>'` shape. The rename-queue plumbing is OUT of Phase 4 scope — it is called out here as a dependency that Phase 2's daemon does not currently produce. To keep Phase 4 self-contained and avoid Phase 2 backflow, register the hook with both args as `#{hook_session_name}` for now and document the follow-up as a known limitation: migrate-rename fires on every rename but the old-name arg is identical to the new-name arg, so task 4-3's body is a no-op in practice. Hooks for the renamed session get orphaned and pruned by the next `CleanStale` run — exactly the "Failure mode" the spec already documents as best-effort / user-re-register. A follow-up issue captures the daemon-side rename delta work as a Phase 6 or post-v1 polish.
  
  Use:
  ```go
  migrateRenameCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'"`
  ```
  The task's acceptance criteria verify the registration shape and idempotency; the spec's "Failure mode: hooks for the renamed session get orphaned and pruned by the next `CleanStale` run" justifies the Phase-4-scope-limited wiring.
- Extend `RegisterPortalHooks(c *Client)` in `internal/tmux/hooks_register.go` to iterate `migrateRenameEvents` after the save-trigger and hydration-trigger loops, calling `RegisterHookIfAbsent(c, "session-renamed", migrateRenameSubstring, migrateRenameCommand)` and joining errors via `errors.Join`. Order: save-trigger → hydration-trigger → migrate-rename. (The order matters only for test determinism; tmux stores the entries in registration order.)
- The `command -v portal` defensive guard wraps this invocation just like every other Portal hook (per spec "tmux Hook Registration Lifecycle → Registration Shape").
- Tests in `internal/tmux/hooks_register_test.go` (extending task 1-7's suite):
  - Fresh server: after `RegisterPortalHooks`, `session-renamed` has exactly two Portal entries — one with `portal state notify` substring and one with `portal state migrate-rename` substring.
  - Idempotency: re-running `RegisterPortalHooks` on an already-fully-populated server produces zero `set-hook -ga` calls on `session-renamed` (neither entry duplicated).
  - Partial prior state: tmux has only the `notify` entry on `session-renamed` (user or Phase 1-only bootstrap) → exactly one `set-hook -ga` call, for the `migrate-rename` entry only.
  - Partial prior state reversed: only `migrate-rename` registered → exactly one `set-hook -ga` call for `notify`.
  - Cross-contamination guard: `show-hooks -g` contains `session-renamed[0] => notify` and `client-attached[0] => signal-hydrate`. The `migrate-rename` substring is neither — `RegisterPortalHooks` appends the `migrate-rename` entry and leaves the other events alone.
  - Content substring scoping: both `portal state notify` and `portal state migrate-rename` are substrings of each other's commands only if the hook writer was careless — verify that matching is done by the `migrate-rename` substring specifically (not `portal state` alone, which would false-positive on `notify`).
- Phase 1 task 1-8's removal logic already handles two Portal entries on `session-renamed` in reverse index order — no changes needed in this task. Verify via an integration test (or a pointer in the test comments back to Phase 1 task 1-8's own coverage).

**Acceptance Criteria**:
- [ ] `RegisterPortalHooks` appends a `session-renamed` entry with the command `run-shell "command -v portal >/dev/null 2>&1 && portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'"` on a fresh server.
- [ ] Idempotent re-registration produces zero additional `set-hook -ga` calls for the `migrate-rename` entry.
- [ ] The `notify` and `migrate-rename` Portal entries coexist on `session-renamed` — both substrings match disjoint entries; no cross-contamination.
- [ ] The substring check uses `portal state migrate-rename` (not `portal state` alone, which would false-match `notify` entries).
- [ ] The `command -v portal` defensive guard wraps the invocation verbatim, matching the shape of every other Portal hook registration.
- [ ] Partial prior state (only `notify` registered) tops up exactly the missing `migrate-rename` entry.
- [ ] First-ever bootstrap on a fresh server appends both `session-renamed` entries (one `notify`, one `migrate-rename`).
- [ ] Phase 1 task 1-8's removal logic removes both Portal entries on `session-renamed` in reverse index order (verified by re-running the task 1-8 test suite under Phase 4 hook table).

**Tests**:
- `"it appends a migrate-rename entry on session-renamed on a fresh server"`
- `"it leaves the existing notify entry untouched when adding migrate-rename"`
- `"it is idempotent across re-registration"`
- `"it tops up only migrate-rename when notify is already present"`
- `"it tops up only notify when migrate-rename is already present"`
- `"it registers both notify and migrate-rename on a completely fresh session-renamed event"`
- `"it wraps the invocation in command -v portal guard"`
- `"it uses the substring 'portal state migrate-rename' specifically, not 'portal state' broadly"`
- `"it does not register migrate-rename on any event other than session-renamed"`

**Edge Cases**:
- Two Portal entries on the same event: content-based idempotency's per-substring scoping (task 1-6) already supports this. The two substrings are disjoint (`portal state notify` vs `portal state migrate-rename`) and match disjoint entries.
- Substring cross-matching: `portal state migrate-rename` does NOT contain `portal state notify` and vice versa. No false positives.
- First-ever bootstrap: both entries get appended in one pass. Registration order is save-trigger → hydration-trigger → migrate-rename, so `session-renamed[0]` is `notify`, `session-renamed[1]` is `migrate-rename` (assuming no pre-existing entries).
- User or plugin registers a `session-renamed` hook of their own: `set-hook -ga` preserves it; Portal's two entries append after it.
- Task 1-8 removal: Phase 1 acceptance already calls out "two Portal entries on same event (`notify` + `migrate-rename` on `session-renamed`) both removed" — this task's registration is the matching counterpart.
- Argument wiring limitation (documented in the task body): both positional arg placeholders currently expand to `#{hook_session_name}`, making task 4-3's body a no-op in practice. The documented follow-up is a daemon-side rename-delta mechanism tracked for Phase 6 / post-v1. This is a Phase 4 scope boundary, not a correctness bug — the spec's "best-effort / CleanStale prunes orphaned entries" failure mode covers the gap.

**Context**:
> Spec "Resume Hook Firing → Session Rename: Hook Key Migration":
> "Portal registers a **separate internal subcommand** — `portal state migrate-rename <old-name> <new-name>` — against the `session-renamed` tmux hook, in addition to the existing `portal state notify` registration. The two hooks coexist on the same event via the same content-based idempotency pattern applied to every other hook."
>
> Spec "Resume Hook Firing → Session Rename → Argument source":
> "tmux's `session-renamed` event exposes both names via format expansions (e.g., `#{hook_session_name}` for the current name; the prior name via `#{client_last_session}` is not reliable). Portal's implementation passes the session name via `#{session_name}` at hook-fire time and reconciles against an in-memory 'last-seen names' map maintained by the daemon — or, more simply, uses the `session-renamed` hook's exposed variables (tmux versions vary in what is accessible). Planning-phase decides the exact wiring; the contract from the spec: hook keys are migrated atomically on rename events, the migration path is a distinct subcommand (not `notify`), and best-effort logging on failure."
>
> Spec "Resume Hook Firing → Session Rename → Failure mode":
> "If migration fails (malformed names, I/O error), hooks for the renamed session get orphaned and pruned by the next `CleanStale` run. User-visible recovery: re-register the hook against the new session name. Migration is best-effort — no retry storm on failure."
>
> Phase 1 task 1-6 `RegisterHookIfAbsent` already implements per-event-per-substring scoping. Phase 1 task 1-7's `RegisterPortalHooks` is the call-site this task extends. Phase 1 task 1-8 already handles reverse-order removal of two Portal entries on `session-renamed`.
>
> **Ambiguity note for Context section**: The spec explicitly delegates the argument-wiring decision to planning. This task pins `#{hook_session_name}` for both args as the Phase 4 scope-limited choice, with the understanding that a daemon-side rename-delta mechanism is required to make the migration functional. That daemon work is out of Phase 4 scope; its absence degrades `migrate-rename` to a registered-but-no-op hook, and the spec's "best-effort / CleanStale cleanup" failure mode covers the gap.

OLD_SPEC_REFERENCE: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Resume Hook Firing → Session Rename: Hook Key Migration", "tmux Hook Registration Lifecycle → Registration Shape", "tmux Hook Registration Lifecycle → Content-Based Idempotency".
-->

## built-in-session-resurrection-4-5 | approved

### Task 4-5: Delete `ExecuteHooks` / `internal/hooks/executor.go` / `cmd/hook_executor.go` and all attach-time firing call sites

**Problem**: The old attach-time hook-firing path — `internal/hooks/executor.go`'s `ExecuteHooks`, `cmd/hook_executor.go`'s `buildHookExecutor`, the `HookExecutor` field on `AttachDeps`, the `hookExecutor(name)` call in `cmd/attach.go`'s `RunE`, and the three `hookExec(...)` call sites in `cmd/open.go` (`PathOpener.Open`, `processTUIResult`, `openPath`) — still exists as of Phase 3. With task 4-2 moving hook firing into the hydrate helper's exec chain, every one of those call sites must be removed. The spec is unambiguous: "`ExecuteHooks` function. Deleted. No more attach-time hook execution." Leaving the old path wired would cause hooks to fire twice on any pane that both survives a restart (helper-side) AND gets reattached in the new server lifetime (old-code-side). It would also keep the unused `TmuxOperator` / `HookRepository` / `PaneLister` / `KeySender` interfaces alive in `internal/hooks/executor.go`.

**Solution**: Delete the two files (`internal/hooks/executor.go`, `internal/hooks/executor_test.go`, `cmd/hook_executor.go`) and all referring call sites. Remove `HookExecutorFunc`, `HookExecutor` from `AttachDeps`, the `hookExec` field on `PathOpener`, the three call sites in `cmd/open.go`, and every test that threads a `HookExecutorFunc` through. Prune the now-orphan interfaces `TmuxOperator`, `HookRepository`, `PaneLister`, `KeySender` (and anywhere else in `internal/hooks/` they are referenced). **Keep `MarkerName`** because task 4-6 needs to reference it briefly before deleting it in the same phase — explicit transfer-of-custody to task 4-6.

**Outcome**: The package `internal/hooks` shrinks to the `Store` + `LookupOnResume` (from task 4-1) surface. `cmd/attach.go` no longer threads a `HookExecutorFunc`; attach becomes a 3-step operation (validate → connect). `cmd/open.go`'s `PathOpener`, `processTUIResult`, and `openPath` are freed of hook wiring. The entire `go build ./...` and `go test ./...` suite compiles and passes after the deletion, modulo the test-suite updates this task requires. No hooks fire on attach; hook firing is exclusively helper-driven (task 4-2).

**Do**:
- Delete files:
  - `internal/hooks/executor.go` — entire file.
  - `internal/hooks/executor_test.go` — entire file.
  - `cmd/hook_executor.go` — entire file.
- Modify `cmd/attach.go`:
  - Remove the `HookExecutor HookExecutorFunc` field from `AttachDeps` (lines 22 in current file).
  - Remove the `hookExecutor` return value from `buildAttachDeps` (change signature from `(SessionConnector, SessionValidator, HookExecutorFunc)` → `(SessionConnector, SessionValidator)`).
  - Remove the `hookExecutor(name)` call and the `if hookExecutor != nil` guard from the `RunE` body (lines 40–42 in current file).
- Modify `cmd/open.go`:
  - Remove the `hookExec HookExecutorFunc` field from `PathOpener` (line 205).
  - Remove the `if po.hookExec != nil { po.hookExec(sessionName) }` and `if po.hookExec != nil { po.hookExec(result.SessionName) }` call sites inside `PathOpener.Open` (lines 217–219 and 228–230).
  - Remove `hookExec: buildHookExecutor(client)` from the `PathOpener` construction in `openPath` (line 255).
  - Remove the `hookExec` param from `processTUIResult`'s signature; remove the `if hookExec != nil { hookExec(selected) }` call (lines 337–346).
  - Remove `hookExec := buildHookExecutor(client)` in `openTUI` (line 404) and pass no hook arg to `processTUIResult`.
- Modify `cmd/attach_test.go`:
  - Remove every `HookExecutorFunc(func(sessionName string) { ... })` stub (lines 162, 201, 231, 257) and every `HookExecutor: hookExecutor` struct-field assignment on `AttachDeps`.
  - Remove test cases that assert hook-firing behaviour from attach; move the "hooks fire at the right moment" testing responsibility to the hydrate-helper test suite (task 4-2's additions).
- Modify `cmd/open_test.go`:
  - Remove every `HookExecutorFunc(func(sessionName string) { ... })` construction (lines 528, 569, 602, 646, 676, 1205, 1240, 1260) and every `hookExec` field-assignment on `PathOpener` / `processTUIResult` test fixtures.
  - Remove test cases that assert hook-firing behaviour from open/TUI; same re-homing as attach tests.
- Prune now-unused interfaces from `internal/hooks/` (search for remaining call sites first):
  - `TmuxOperator`, `HookRepository`, `PaneLister`, `KeySender`, `OptionChecker`, `HookLoader`, `HookCleaner`, `AllPaneLister` — if `AllPaneLister` is still referenced by `cmd/clean.go` (see `cmd/clean.go:15`), keep it. Otherwise delete. Run `grep -R "hooks.TmuxOperator" cmd/ internal/ && grep -R "hooks.HookRepository" cmd/ internal/` before deleting each interface; only remove what genuinely has no callers remaining after this task's edits.
  - **Keep `MarkerName`** — task 4-6 handles its removal in the next step. Adding a `// TODO(task 4-6): delete after hooks.go is detached from MarkerName` comment on the function makes the phase boundary explicit.
- Run `go build ./...` and `go test ./...` after each deletion to catch dangling references. The package must still compile.

**Acceptance Criteria**:
- [ ] `internal/hooks/executor.go` is deleted.
- [ ] `internal/hooks/executor_test.go` is deleted.
- [ ] `cmd/hook_executor.go` is deleted.
- [ ] `HookExecutorFunc` type no longer exists in the `cmd` package.
- [ ] `HookExecutor` field removed from `AttachDeps`; `buildAttachDeps` returns only `(SessionConnector, SessionValidator)`.
- [ ] `hookExecutor(name)` call site in `cmd/attach.go`'s `RunE` is removed.
- [ ] `hookExec` field removed from `PathOpener`.
- [ ] All three `hookExec(...)` call sites in `cmd/open.go` (`PathOpener.Open` × 2, `processTUIResult`) are removed.
- [ ] `buildHookExecutor(client)` references in `openPath` and `openTUI` are removed.
- [ ] `processTUIResult`'s signature no longer takes `HookExecutorFunc`.
- [ ] Now-orphan interfaces in `internal/hooks/` are pruned (except `AllPaneLister` which `cmd/clean.go` still uses).
- [ ] `MarkerName` is retained (deletion belongs to task 4-6).
- [ ] `cmd/attach_test.go` and `cmd/open_test.go` compile without the removed `HookExecutorFunc` references.
- [ ] `go build ./...` and `go test ./...` pass after the changes.
- [ ] No remaining references to `ExecuteHooks`, `buildHookExecutor`, or `HookExecutorFunc` anywhere in the repository (`grep -R` returns zero hits).

**Tests**:
- `"it compiles the cmd package after removing HookExecutorFunc"` (smoke — part of `go build`)
- `"it passes the existing attach test suite without hook-executor stubs"`
- `"it passes the existing open test suite without hook-executor stubs"`
- `"attach command no longer fires hooks"` — positive assertion that attaching a session with a registered hook does NOT invoke the hook command (replace the existing `"fires on-resume hook at attach"` test with this inverted assertion)
- `"open command no longer fires hooks on TUI selection"`
- `"open command no longer fires hooks on direct path attach"`
- `"internal/hooks package no longer exports ExecuteHooks, TmuxOperator, HookRepository, PaneLister, KeySender"` — compile-time test via a dedicated `internal/hooks/removed_symbols_test.go` that tries to reference each (as a comment-guarded `//go:build never` stub) OR simply document via the successful `go build` that these symbols are gone.

**Edge Cases**:
- Test suites currently rely on `HookExecutorFunc` to simulate hook-firing — those assertions either move to the hydrate-helper test suite (task 4-2) or invert to "hooks do not fire on attach". Pick inversion for the existing attach/open tests, and let the hydrate-helper tests own the positive assertions.
- `AllPaneLister` is used by `cmd/clean.go:15` (`CleanDeps.AllPaneLister`) — this task keeps it because task 4-7 relies on the same type. Do NOT delete it.
- `MarkerName` helper has one remaining caller in `cmd/hooks.go:140` and `cmd/hooks.go:190` (the `@portal-active-<pane>` set and delete paths). Task 4-6 removes those callers AND the helper together. Do NOT delete `MarkerName` in this task — the ordering is deliberate so test-suite compiles cleanly at each step.
- Interface cleanup may cascade into other files — `HookLoader` / `HookCleaner` are in the same file as `TmuxOperator` / `HookRepository` and are composed by the latter. Delete composed interfaces only if they have no remaining callers after this task's deletions; keep otherwise.
- `internal/hooks/store_test.go` remains intact — store-level tests are unaffected.
- `loadHookStore()` in `cmd/hooks.go` is still used by `hooks set` / `hooks list` / `hooks rm` / `cmd/clean.go` and (after task 4-2) by the hydrate helper. Do NOT delete.
- Run `go vet ./...` as well as `go build ./...` after deletion to catch any unused imports left behind.

**Context**:
> Spec "Resume Hook Firing → What Is Deleted from the Previous Design":
> - `ExecuteHooks` function. Deleted. No more attach-time hook execution.
> - Call sites of `ExecuteHooks` in `cmd/open.go` and `cmd/attach.go`. Deleted.
> - `internal/hooks/executor.go`. Deleted.
> - `cmd/hook_executor.go`. Deleted.
> - `@portal-active-<pane>` volatile marker set during `portal hooks set` as a one-shot-per-server-lifetime gate. Deleted. [Task 4-6 owns this.]
> - All attach-time hook checking. Deleted.
> - Shell-readiness polling for `send-keys` delivery. Eliminated — nothing uses `send-keys` for hook firing any more.
>
> Spec "Resume Hook Firing → Behavior Change: No 'Live Attach' Firing":
> "Old: a hook registered on a pane that has not yet gone through a save/restore cycle; user detaches and reattaches within the same server lifetime. Old design fired the hook via `send-keys` once per server lifetime (one-shot via the `@portal-active-<pane>` marker). New: the hook does not fire until the next server restart triggers skeleton restoration. This is correct behavior by design."
>
> Current code references:
> - `internal/hooks/executor.go:1-115` — entire file to delete, including `TmuxOperator`, `HookRepository`, `PaneLister`, `KeySender`, `OptionChecker`, `HookLoader`, `HookCleaner`, `AllPaneLister`, `MarkerName`, `ExecuteHooks`.
> - `cmd/hook_executor.go:1-22` — entire file.
> - `cmd/attach.go:22` — `HookExecutor HookExecutorFunc` field.
> - `cmd/attach.go:40-42` — `hookExecutor(name)` call site.
> - `cmd/open.go:205` — `hookExec HookExecutorFunc` field on `PathOpener`.
> - `cmd/open.go:217-219`, `228-230` — `hookExec(...)` call sites in `PathOpener.Open`.
> - `cmd/open.go:255` — `buildHookExecutor(client)` construction in `openPath`.
> - `cmd/open.go:337-346` — `hookExec` param on `processTUIResult`.
> - `cmd/open.go:404` — `buildHookExecutor(client)` construction in `openTUI`.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Resume Hook Firing → What Is Deleted from the Previous Design", "Resume Hook Firing → Behavior Change: No 'Live Attach' Firing".

## built-in-session-resurrection-4-6 | approved

### Task 4-6: Strip `@portal-active-<pane>` registration-time marker logic from `portal hooks set` / `portal hooks rm`

**Problem**: The `@portal-active-<pane>` volatile server-option marker existed to implement the old one-shot-per-server-lifetime gate for attach-time hook firing — `hooks set` set it to `1`, `hooks rm` deleted it, `ExecuteHooks` checked it before firing. With `ExecuteHooks` gone (task 4-5) and hook firing moved to the hydrate helper (task 4-2), the marker has no reader. Leaving it in place leaks tmux server-option state on every `portal hooks set` invocation, clutters the server-option namespace, and keeps `hooks.MarkerName` alive — a function with no remaining callers once `hooks set` / `hooks rm` stop using it. The spec calls out both the marker-set and marker-delete paths for deletion, plus the `HooksDeps.OptionSetter` and `OptionDeleter` fields that plumb the side-effect through.

**Solution**: Remove the marker set/delete calls from `hooks set` / `hooks rm`; remove `OptionSetter` / `OptionDeleter` fields from `HooksDeps`; remove the `ServerOptionSetter` / `ServerOptionDeleter` interfaces if they have no other callers; delete `hooks.MarkerName` (now unreferenced after task 4-5 kept it alive for one task). Update `cmd/hooks_test.go` to remove `mockOptionSetter` / `mockOptionDeleter` stubs and every assertion that expected a tmux `set-option` / `unset-option` side effect. The user-facing CLI surface is unchanged (`set --on-resume`, `list`, `rm --on-resume`). Internally, `hooks set` becomes a pure write to `hooks.json`; `hooks rm` becomes a pure remove from `hooks.json`.

**Outcome**: `portal hooks set --on-resume "claude --resume abc"` writes to `hooks.json` and exits — no tmux side effects. `portal hooks rm --on-resume` removes from `hooks.json` and exits — no tmux side effects. tmux server-option namespace is no longer polluted by `@portal-active-*` keys. The only hooks-related tmux interaction during `set`/`rm` is the structural-key resolution via `ResolveStructuralKey` (an existing read-only query — unchanged). Full test suite updated and passing.

**Do**:
- Modify `cmd/hooks.go`:
  - Delete the `ServerOptionSetter` interface (lines 12–15) — `HooksDeps` is the only caller.
  - Delete the `ServerOptionDeleter` interface (lines 17–20) — same.
  - Remove `OptionSetter ServerOptionSetter` and `OptionDeleter ServerOptionDeleter` fields from `HooksDeps` (lines 35–36).
  - In `hooksSetCmd.RunE`, remove lines 134–142 (the `var setter ServerOptionSetter` block and the `SetServerOption(hooks.MarkerName(structuralKey), "1")` call).
  - In `hooksRmCmd.RunE`, remove lines 184–192 (the `var deleter ServerOptionDeleter` block and the `DeleteServerOption(hooks.MarkerName(structuralKey))` call).
  - Remove the `"github.com/leeovery/portal/internal/hooks"` import's `hooks.MarkerName` usage after those deletions; the import itself stays (the package is still used for `loadHookStore` via `hooks.NewStore`).
- Modify `internal/hooks/executor.go` deletion from task 4-5 carried `MarkerName` forward — now delete it as part of the executor.go deletion was actually task 4-5. Wait: task 4-5's acceptance explicitly says "**Keep `MarkerName`** because task 4-6 needs to reference it briefly before deleting it in the same phase". So the state at the start of this task is: `executor.go` is gone BUT `MarkerName` lives somewhere (either re-homed in a standalone file by task 4-5 or left as a stub). Reconcile: task 4-5 keeps `MarkerName` by placing it in a new tiny file (`internal/hooks/marker.go`) or keeping it defined in `store.go` temporarily. This task deletes that file / that function. Confirm by `grep -R 'MarkerName' internal/ cmd/` at the start of this task — after task 4-5's completion, the only callers should be `cmd/hooks.go:140` and `cmd/hooks.go:190`. This task removes those callers AND then deletes the function.
- Delete `hooks.MarkerName` after removing its final callers:
  - Remove the function definition from wherever task 4-5 re-homed it (likely `internal/hooks/marker.go` or the bottom of `internal/hooks/store.go`).
  - `grep -R 'hooks.MarkerName' .` must return zero hits after this task.
- Modify `cmd/hooks_test.go`:
  - Delete `mockOptionSetter` struct and its methods (lines 157–170).
  - Delete `mockOptionDeleter` struct and its methods (lines 444–453).
  - Remove every `hooksDeps = &HooksDeps{OptionSetter: ...}` and `hooksDeps = &HooksDeps{OptionDeleter: ...}` assignment; `hooksDeps` shrinks to just `KeyResolver` — keep that field's plumbing intact because `ResolveStructuralKey` is still needed.
  - Remove every test assertion of the form `if mock.calls[0].name != "@portal-active-..."` — those tests no longer have a marker side effect to assert.
  - Keep the "writes to hooks.json" assertions — those are the remaining behavioural contract for `hooks set` / `hooks rm`.
- Update any test that used `OptionSetter` / `OptionDeleter` as part of a joint test fixture — these are standalone-mocks paired with the now-redundant assertions; they go together.
- Do NOT touch the `loadHookStore()` helper (`cmd/hooks.go:149`) — still needed.
- Do NOT touch the `Store.Set` / `Store.Remove` methods — the persistent-write side of `hooks set` / `hooks rm` is unchanged.

**Acceptance Criteria**:
- [ ] `cmd/hooks.go` `hooksSetCmd.RunE` no longer calls `SetServerOption(hooks.MarkerName(...), "1")` — only writes to `hooks.json` via `store.Set`.
- [ ] `cmd/hooks.go` `hooksRmCmd.RunE` no longer calls `DeleteServerOption(hooks.MarkerName(...))` — only removes from `hooks.json` via `store.Remove`.
- [ ] `HooksDeps` no longer has `OptionSetter` or `OptionDeleter` fields.
- [ ] `ServerOptionSetter` and `ServerOptionDeleter` interfaces are deleted (no remaining callers).
- [ ] `hooks.MarkerName` function is deleted; `grep -R 'MarkerName' .` returns zero hits in the repo.
- [ ] `cmd/hooks_test.go` no longer contains `mockOptionSetter`, `mockOptionDeleter`, or any assertion of `@portal-active-*` server-option side effects.
- [ ] User-facing CLI surface is unchanged: `portal hooks set --on-resume "<cmd>"`, `portal hooks list`, `portal hooks rm --on-resume` all accept the same argv and produce the same JSON-file effect.
- [ ] `portal hooks set` still writes `hooks.json` correctly — verified by an end-to-end test that reads the file back and asserts the `on-resume` entry.
- [ ] `portal hooks rm` still removes `hooks.json` entries correctly — same verification pattern.
- [ ] No tmux `set-option -s` or `set-option -u` calls are made by `hooks set` / `hooks rm` — verified via a Commander mock that panics on any `set-option` invocation during these commands.
- [ ] `go build ./...` and `go test ./...` pass.

**Tests**:
- `"hooks set writes to hooks.json with the on-resume command"` (unchanged from existing tests, minus the marker-set assertion)
- `"hooks set does not call tmux set-option"` (new — positive assertion of no side effect)
- `"hooks rm removes the on-resume entry from hooks.json"` (unchanged minus marker-delete assertion)
- `"hooks rm does not call tmux set-option -u"` (new — positive assertion)
- `"hooks list is unaffected by the marker-removal change"` (regression)
- `"hooks set still resolves the current pane via TMUX_PANE and ResolveStructuralKey"` (regression — the non-marker path survives)
- `"hooks set with empty --on-resume value is rejected"` (regression)
- `"hooks rm without --on-resume flag is rejected"` (regression)

**Edge Cases**:
- `TMUX_PANE` unset / `requireTmuxPane` error path is unchanged — this task only removes the marker side effect, not the pane-resolution precondition.
- `ResolveStructuralKey` failure still returns the wrapped error; unchanged.
- Pre-existing `@portal-active-*` markers on tmux servers from before the upgrade: these persist as stale server options until the next tmux server restart (they are volatile — server-option scope). No Portal code writes them any more; nothing reads them. They self-clean on the next `kill-server` / reboot. Not worth a migration sweep; the spec's "degrade locally, log, continue" principle justifies leaving them.
- User-visible CLI surface (argv, flags, output, exit codes) must be byte-identical before and after. Verify via integration test: run `portal hooks set --on-resume "foo"` under a mock tmux, read `hooks.json`, assert the entry. Run `portal hooks rm --on-resume`, assert the entry is gone.
- Test mocks shrink: `HooksDeps` now has one injectable field (`KeyResolver`). Test helpers that populate `OptionSetter` / `OptionDeleter` need to drop those fields; keep the test's core assertion (hooks.json state) intact.
- Do NOT accidentally delete `ResolveStructuralKey` or `StructuralKeyResolver` — those are independent of the marker work.

**Context**:
> Spec "Resume Hook Firing → What Is Deleted from the Previous Design":
> "`@portal-active-<pane>` volatile marker set during `portal hooks set` as a one-shot-per-server-lifetime gate. Deleted. The registration path (`portal hooks set`) becomes a pure write to `hooks.json` with no tmux-side marker management."
>
> Spec "CLI Surface → Unchanged User-Facing Surface":
> "`portal hooks set --on-resume '<cmd>'` — unchanged surface; internals no longer set `@portal-active-<pane>` marker (see Resume Hook Firing)."
> "`portal hooks list` — unchanged."
> "`portal hooks rm --on-resume` — unchanged."
>
> Current code references:
> - `cmd/hooks.go:12-20` — `ServerOptionSetter` and `ServerOptionDeleter` interfaces.
> - `cmd/hooks.go:34-38` — `HooksDeps` struct with `OptionSetter` / `OptionDeleter` / `KeyResolver`.
> - `cmd/hooks.go:134-142` — marker-set block in `hooksSetCmd.RunE`.
> - `cmd/hooks.go:184-192` — marker-delete block in `hooksRmCmd.RunE`.
> - `internal/hooks/executor.go:55-57` (at the time of Phase 4 start) — `MarkerName` helper; relocated by task 4-5 to survive that deletion; deleted by this task.
> - `cmd/hooks_test.go:156-170` — `mockOptionSetter`.
> - `cmd/hooks_test.go:444-453` — `mockOptionDeleter`.
>
> Task 4-5 and this task together implement the "Resume Hook Firing → What Is Deleted" list; the split across two tasks keeps each deletion's test-suite update independently reviewable.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Resume Hook Firing → What Is Deleted from the Previous Design", "CLI Surface → Unchanged User-Facing Surface".

## built-in-session-resurrection-4-7 | approved

### Task 4-7: Remove `len(livePanes) == 0` early return in `cmd/clean.go` so `CleanStale` runs unconditionally

**Problem**: `cmd/clean.go:79-81` has an `if len(livePanes) == 0 { return nil }` guard that skips hook cleanup when `list-panes -a` returns empty. The guard existed because, under the old architecture, `portal clean` (or `CleanStale` called during bootstrap) could run before tmux-resurrect/continuum had restored any sessions — at which point `livePanes` was transiently empty and running `CleanStale(livePanes)` would destroy every legitimately-registered hook. Under the Phase 3 / Phase 5 architecture, `CleanStale` runs in bootstrap step 7 AFTER `Restore()` has populated skeleton-restored panes, so "live panes are empty" now genuinely means "there really are no sessions, hooks are orphaned, prune them." The spec explicitly calls for removing the guard. Without this change, `portal clean` still silently skips cleanup whenever the user runs it against a torn-down tmux server, leaving hooks.json entries forever.

**Solution**: Delete the three lines forming the `len(livePanes) == 0` early-return. Keep the preceding `lister.ListAllPanes()` error-path safety net (line 74) — that remains valid belt-and-braces: if tmux itself fails (not "zero panes", just "tmux errored"), we can't safely decide what to prune. Everything else in `cmd/clean.go` stays intact: stale-detection criteria (structural-key mismatch only, per `Store.CleanStale`'s existing logic), atomic write of the pruned `hooks.json`, stderr output naming each removed key. Update tests to cover the new "zero live panes → all hooks removed" behaviour.

**Outcome**: `portal clean` run against a tmux server with no live sessions and `hooks.json` containing three orphaned entries prints "Removed stale hook: <key>" three times and leaves `hooks.json` empty. The bootstrap-step-7 invocation (Phase 5) runs under the same semantics — post-skeleton-restore, if any hooks.json keys do not match a live pane, they are pruned. Stale-detection criteria are unchanged: structural-key mismatch against `list-panes -a`. Binary-missing and `projects.json`-absent are explicitly NOT staleness signals (per the spec's "generic hook" design principle).

**Do**:
- Modify `cmd/clean.go` `cleanCmd.RunE`:
  - Delete lines 77–81 (the comment `// Empty pane list with existing hooks means ... no tmux server is running.` plus the three-line guard `if len(livePanes) == 0 { return nil }`).
  - Keep lines 70–75 (the `lister := buildCleanPaneLister()` / `livePanes, err := lister.ListAllPanes()` / `if err != nil { return nil }` error-path safety net). The `ListAllPanes` error return still warrants skipping cleanup — we cannot reliably prune if tmux itself errored. This is different from the deleted zero-panes guard, which assumed tmux-resurrect-pending-state.
  - The subsequent `removedPanes, err := hookStore.CleanStale(livePanes)` now runs even when `livePanes` is an empty slice; `Store.CleanStale` (see `internal/hooks/store.go:130-159`) handles empty `liveKeys` correctly — it builds an empty `live` set and prunes every entry.
- Do NOT change `Store.CleanStale` — its existing "entries not in `live` are removed" semantics is exactly what we want now.
- Do NOT change the stale-detection criteria: structural-key mismatch against the `liveKeys` slice. `Store.CleanStale`'s signature takes `[]string` of live structural keys; `list-panes -a` produces exactly those. Binary-missing and `projects.json`-absent are not inputs to this function.
- Do NOT touch the pre-cleanup "no hooks registered" early return (lines 64–68) — that one is still valid; if the hooks file has nothing to prune, there is no point calling tmux.
- Update `cmd/clean_test.go`:
  - Add a new test case "removes all hooks when zero live panes and hooks exist" — set up `hooks.json` with three entries, stub `AllPaneLister.ListAllPanes()` to return `[]string{}, nil`, run `portal clean`, assert all three removals are printed and `hooks.json` is empty on disk.
  - Add "preserves hooks when ListAllPanes errors" — stub the lister to return `nil, errors.New("tmux failure")`, assert zero removals and `hooks.json` intact.
  - Keep the existing test "removes stale project" (lines 11–47 of current file) unchanged.
  - Update or add: "zero-entry hooks.json is a no-op regardless of live pane state" — pre-populate an empty `hooks.json`, run `portal clean`, assert no "Removed stale hook:" lines printed.
- Phase 5 wires `CleanStale` into `PersistentPreRunE` step 7; that integration landed there, not here. This task only changes the `cmd/clean.go` behaviour. Verify the same change cascades into the bootstrap-step-7 path by running the Phase 5 integration test once it lands; for now, the `cmd/clean_test.go` coverage is the authoritative unit-level check.

**Acceptance Criteria**:
- [ ] The `if len(livePanes) == 0 { return nil }` block in `cmd/clean.go` is removed.
- [ ] `portal clean` with zero live panes AND non-empty `hooks.json` prunes every hook entry and prints "Removed stale hook: <key>" for each.
- [ ] `portal clean` with `ListAllPanes` error still skips cleanup (safety net preserved).
- [ ] `portal clean` with zero-entry `hooks.json` is a clean no-op regardless of live pane state (the pre-check guard on line 66 handles this).
- [ ] Stale-detection criteria are unchanged — structural-key mismatch against `list-panes -a`. Binary-missing and `projects.json`-absent are NOT staleness signals (verified by a test that registers a hook whose command references a non-existent binary and asserts the hook is NOT removed when its structural key matches a live pane).
- [ ] `portal clean`'s stdout output still lists removed hook keys in the `"Removed stale hook: %s\n"` format.
- [ ] Bootstrap step-7 invocation (Phase 5 integration) prunes orphaned entries under the same semantics (left as a note for Phase 5 — this task does not change bootstrap wiring).
- [ ] `go build ./...` and `go test ./...` pass.

**Tests**:
- `"it removes all hooks when zero live panes and hooks.json has entries"` (new)
- `"it preserves hooks when ListAllPanes returns an error"` (safety net regression)
- `"it is a clean no-op when hooks.json is empty regardless of live pane state"` (pre-check regression)
- `"it removes only entries whose structural key does not match any live pane"` (core behaviour — existing test, verify still passes)
- `"it keeps a hook whose command references a missing binary, as long as the structural key is live"` (spec-pinned edge case — binary-missing is NOT a staleness signal)
- `"it keeps a hook when its structural key matches a live pane, even if projects.json has no entry for that pane's project"` (spec-pinned edge case — projects.json absence is NOT a staleness signal)
- `"it prints a removal line for every pruned hook"` (output regression)
- `"portal clean still removes stale projects alongside stale hooks"` (existing test — verify still passes)

**Edge Cases**:
- Zero live panes + existing hooks → all entries pruned. This is the primary behaviour change; previously these entries persisted indefinitely.
- `ListAllPanes` error → keep the pre-existing safety-net (`return nil` on error). Distinct from zero-panes: an error means "we can't know what's live", which is unsafe input to `CleanStale`.
- Zero-entry `hooks.json` → existing pre-check (line 66) short-circuits before even calling `ListAllPanes`. Unchanged.
- Stale-detection criteria: structural-key mismatch ONLY. Per spec "CleanStale Behavior → Stale-Hook Detection Criteria (Unchanged)": binary-missing is NOT a staleness signal ("That is a runtime execution error when the hook fires"); `projects.json` absence is NOT a staleness signal ("Portal's hook system is generic and has no coupling to `projects.json`"). Tests enforce both.
- Bootstrap step-7 invocation (Phase 5): the same `CleanStale` runs there. By Phase-5 integration time, `Restore()` has populated skeleton-restored panes, so `livePanes` reflects the post-restore state — matching the spec's invariant that step 7 runs after steps 5-6. This task does not couple to Phase 5; the cascade is automatic because `cmd/clean.go` is the invocation point for both CLI-initiated `portal clean` and the internal `CleanStale()` call in bootstrap.
- `portal clean` is exempt from bootstrap (per Phase 1 `skipTmuxCheck`). This task's semantic change — "trust live tmux state whenever invoked" — means `portal clean` on a torn-down server prunes orphaned hooks, which is the user's reasonable expectation.
- Reinstating the guard would be a regression; add an explicit test asserting that the zero-panes path now triggers pruning rather than skipping.

**Context**:
> Spec "CleanStale Behavior → Change":
> "Delete the `if len(livePanes) == 0 { return }` early return from `CleanStale`'s current implementation. `CleanStale` now runs unconditionally, trusting live tmux state whenever it is invoked."
>
> Spec "CleanStale Behavior → Why It Is Removed":
> "Under the new bootstrap flow, `CleanStale` runs in **step 7 of `PersistentPreRunE`** — *after* skeleton restore completes in steps 5-6. By that point, live panes include both pre-existing panes and skeleton-restored ones. If `list-panes -a` is genuinely empty at step 7, there really are no sessions, and any `hooks.json` entries are genuinely orphaned."
>
> Spec "CleanStale Behavior → Where CleanStale Runs":
> "`portal clean` command (user-initiated). Exempt from bootstrap. The command's body calls `CleanStale()` directly against live tmux state. Because it skips bootstrap, there is no skeleton-restore step preceding it — but `portal clean` is a user-initiated cleanup on whatever tmux is currently live, so that is the intended semantic. If no sessions are live when the user runs `portal clean`, and `hooks.json` has entries, they are genuinely orphaned and get pruned."
>
> Spec "CleanStale Behavior → Stale-Hook Detection Criteria (Unchanged)":
> "An entry in `hooks.json` is considered stale if its structural key (`session:window.pane`) does not match any live pane enumerated by `list-panes -a`. Explicitly NOT criteria for staleness: Hook command's binary missing. That is a runtime execution error when the hook fires, not a stale-entry condition. Portal does not validate hook commands. Project removed from `projects.json`. Portal's hook system is generic and has no coupling to `projects.json`."
>
> Spec "CleanStale Behavior → Refactor Scope":
> "Small mechanical change: remove the `len(livePanes) == 0` early-return branch. Everything else (structural-key matching, atomic write of updated `hooks.json`) stays as it is today."
>
> Current code reference: `cmd/clean.go:77-81` — the exact three-line block to remove.
>
> `internal/hooks/store.go:130-159` — `Store.CleanStale` implementation already handles empty `liveKeys` correctly; no changes needed at that layer.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "CleanStale Behavior".

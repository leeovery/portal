---
phase: 2
phase_name: Live-Key Sites Adopt the Hook Key — registration + stale cleanup
total: 6
---

## session-rename-orphans-resume-hook-2-1 | approved

### Task session-rename-orphans-resume-hook-2-1: Add `tmux.ResolveHookKey` client read using `HookKeyFormat`

**Problem**: Hook registration (Stage 1, `cmd/hooks.go`) today resolves a pane's key via `(*Client).ResolveStructuralKey`, which reads the name-based `StructuralKeyFormat` and so keys hooks off the mutable session name — the root cause of the rename orphan. To switch registration onto the immutable `@portal-id`, the tmux client needs a single-read primitive that resolves a pane's **hook key** via the Phase 1 `HookKeyFormat` (`#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}`), letting tmux itself pick the id-vs-name branch per session. No such client method exists yet.

**Solution**: Add `(*Client).ResolveHookKey(paneID string) (string, error)` to `internal/tmux/tmux.go`, mirroring `ResolveStructuralKey` exactly but reading `HookKeyFormat` instead of `StructuralKeyFormat`: `display-message -p -t <paneID> <HookKeyFormat>`. On a read failure it returns `("", wrapped error)` — it never synthesizes a name-based key. Verify against a real tmux server (stamped vs un-stamped session) via the `tmuxtest` socket fixture, `SkipIfNoTmux`-gated.

**Outcome**: A `tmux.ResolveHookKey` method exists that, against a live tmux server, resolves a stamped pane to `<id>:w.p` and an un-stamped pane to `<name>:w.p`, and propagates any `display-message` transport/exec failure as a wrapped error — ready for `cmd/hooks.go` to consume in Task 2-2.

**Do**:
- Add `func (c *Client) ResolveHookKey(paneID string) (string, error)` to `internal/tmux/tmux.go`, placed immediately after `ResolveStructuralKey` (~lines 320-329) so the two pane-key resolvers sit together.
- Implement it as the byte-for-byte mirror of `ResolveStructuralKey` except for the format constant: `output, err := c.cmd.Run("display-message", "-p", "-t", paneID, HookKeyFormat)`; on `err != nil` return `"", fmt.Errorf("failed to resolve hook key for pane %q: %w", paneID, err)`; otherwise return `output, nil`. `c.cmd.Run` already trims the trailing newline (same as `ResolveStructuralKey`), so no extra trimming.
- Do NOT add any Go-side "id absent" branch — the stamped-vs-unstamped conditional is resolved entirely inside tmux by `HookKeyFormat`. There is exactly one tmux read and one error path.
- Doc-comment it as the canonical live-read hook-key resolver: it reads `HookKeyFormat`, so a stamped session yields `<id>:w.p` and an un-stamped one yields `<name>:w.p`; note that a read failure aborts (returns the wrapped error) and must never fall back to a name-based key, because doing so would silently orphan a stamped session's hook. Cross-reference `HookKey` (the saved-path in-Go mirror) and `HookKeyFormat` (the format it reads).
- Do NOT touch `ResolveStructuralKey`, `StructuralKeyFormat`, `ListAllPanes`, or `PaneTarget` — they remain valid for name-based tmux targeting and non-hook structural use.
- Add a real-tmux round-trip test (new file `internal/tmux/resolve_hookkey_realtmux_test.go`, package `tmux_test`), mirroring the structure of `hooks_register_realtmux_test.go` / `portal_dir_roundtrip_realtmux_test.go`: no build tag, gated only by `tmuxtest.SkipIfNoTmux(t)`, using `tmuxtest.New(t, "ptl-hookkey-")` + `ts.Client()` + `client.EnsureServer()`.

**Acceptance Criteria**:
- [ ] `(*Client).ResolveHookKey(paneID string) (string, error)` exists and issues exactly one `display-message -p -t <paneID> <HookKeyFormat>` read (uses the Phase 1 `HookKeyFormat` constant, not `StructuralKeyFormat`).
- [ ] Real-tmux stamped pane: on a session with `@portal-id = "tok123"`, `ResolveHookKey` against that session's pane returns `"tok123:0.0"` (base-index 0 under the `-f /dev/null` harness).
- [ ] Real-tmux un-stamped pane: on a session with no `@portal-id`, `ResolveHookKey` returns `"<sessionName>:0.0"`.
- [ ] Read-failure: when the underlying `display-message` read fails (e.g. a non-existent pane target), `ResolveHookKey` returns `("", err)` with the error wrapped (recoverable via `errors.As`/`errors.Is`), and it does NOT return a name-based or otherwise synthesized key.
- [ ] No Go-side id-absent branch exists — the id-vs-name choice is entirely tmux-resolved by `HookKeyFormat`.
- [ ] `ResolveStructuralKey`, `StructuralKeyFormat`, `ListAllPanes`, `PaneTarget` are behaviourally unchanged (no edits to their bodies).
- [ ] The real-tmux test carries NO build tag and skips cleanly via `tmuxtest.SkipIfNoTmux(t)` where tmux is absent (no failure).
- [ ] `go build -o portal .` succeeds; `go test ./internal/tmux/...` passes (and skips the real-tmux test where tmux is unavailable).

**Tests** (`internal/tmux/resolve_hookkey_realtmux_test.go`, package `tmux_test`, `SkipIfNoTmux`-gated, NO `t.Parallel()`):
- `"it resolves a stamped pane to id:w.p"` — create a session, `client.SetSessionOption(name, session.PortalIDOption, "tok123")` (or the literal `"@portal-id"` if importing `internal/session` from `tmux_test` would cycle — prefer the literal to avoid the import), read via `ResolveHookKey(paneID)`, assert `"tok123:0.0"`. Resolve the pane ID with `display-message -p -t <session> '#{pane_id}'` through `ts.Run`, or target the session directly (`ResolveHookKey(name)`), since `display-message -t <session>` resolves against the session's active pane.
- `"it resolves an un-stamped pane to name:w.p"` — create a session WITHOUT stamping, read via `ResolveHookKey`, assert `"<sessionName>:0.0"`.
- `"it returns a wrapped error on a display-message read failure"` — call `ResolveHookKey("%nonexistent")` (a pane target that does not exist) against the isolated server and assert a non-nil error and an empty string return (no synthesized key).
- `"it skips cleanly when tmux is unavailable"` — covered structurally by the `SkipIfNoTmux` gate (no separate case needed).

**Edge Cases**:
- Stamped pane → `<id>:w.p` (rename-immune; the id rides the session object, not its name).
- Un-stamped pane → `<name>:w.p` (legacy / no-migration fallback — tmux's `#{?@portal-id,...}` treats absent/empty `@portal-id` as false).
- `display-message` read failure → aborts with the wrapped error; MUST NOT synthesize a name-based key (spec § Stage 1 Failure contract — a synthesized fallback would silently orphan a stamped session's hook).

**Context**:
> Spec § Stage 1 — Registration: `resolveCurrentPaneKey()` is changed to resolve the hook key via a new client read using `HookKeyFormat` (e.g. `ResolveHookKey(paneID)` → `display-message -p -t <pane> <HookKeyFormat>`).
> Spec § Stage 1 Failure contract: `ResolveHookKey` is a single `display-message` read; the conditional resolves stamped-vs-unstamped inside tmux, so there is no Go-side "id absent" branch. If the read itself fails (transport/exec error), registration aborts with the error, exactly as `ResolveStructuralKey` does today — it must NOT synthesize a name-based key on failure, which would silently orphan a stamped session's hook.
> The existing mirror pattern is `ResolveStructuralKey` (`tmux.go:320-329`): `output, err := c.cmd.Run("display-message", "-p", "-t", paneID, StructuralKeyFormat); if err != nil { return "", fmt.Errorf(...%w...) }; return output, nil`. `ResolveHookKey` is identical except the format constant.
> `HookKeyFormat` and `HookKey` are the Phase 1 derivation primitives (Tasks 1-1 / 1-2). `HookKeyFormat` is verified against real tmux in Task 1-2; this task verifies the client method that consumes it.
> This task adds the client method only — the `cmd/hooks.go` `resolveCurrentPaneKey` switch is Task 2-2. No production caller invokes `ResolveHookKey` yet.
> The `tmuxtest` harness runs `-f /dev/null`, so base-index and pane-base-index default to 0 — a single-pane session's key suffix is `0.0`.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation → Stage 1 — Registration (`cmd/hooks.go`) + Failure contract; § Hook-Key Derivation → new derivation primitives; § Testing Requirements → Derivation primitives (unit) → cross-site consistency.

## session-rename-orphans-resume-hook-2-2 | approved

### Task session-rename-orphans-resume-hook-2-2: Switch `resolveCurrentPaneKey` (`cmd/hooks.go`) to resolve the hook key

**Problem**: `portal hooks set` / `hooks rm` register and remove resume hooks under whatever `resolveCurrentPaneKey()` returns. Today that function calls `StructuralKeyResolver.ResolveStructuralKey($TMUX_PANE)`, which keys hooks off the mutable session name — so a rename orphans the hook. Registration is a key-producing site of the central "every site derives the same key" invariant, so it must switch to the hook-key derivation (`ResolveHookKey` from Task 2-1) to store hooks under the immutable `@portal-id` when the session is stamped.

**Solution**: Change `cmd/hooks.go`'s resolver seam and `resolveCurrentPaneKey()` to resolve the hook key via `ResolveHookKey(paneID)` instead of `ResolveStructuralKey(paneID)`. The seam interface, its `hooksDeps` mock field, and the production client all switch to the hook-key read. On a read failure, both `hooks set` and `hooks rm` abort with the error (no name-based synthesis). The `rm --pane-key` literal pass-through is untouched — it still bypasses resolution entirely and removes the verbatim key.

**Outcome**: `portal hooks set` / `hooks rm` register/remove under the stable hook key (`<@portal-id or session_name>:w.p`) resolved via a single `HookKeyFormat` read; a read failure aborts the command; and `rm --pane-key <key>` remains a verbatim removal with no re-derivation.

**Do**:
- In `cmd/hooks.go`, rename the `StructuralKeyResolver` seam interface to a hook-key resolver (e.g. `HookKeyResolver`) whose single method is `ResolveHookKey(paneID string) (string, error)`, and update the `HooksDeps.KeyResolver` field's type to it. Update the interface doc-comment to describe resolving a pane ID to its **hook key** (`<@portal-id or session_name>:window.pane`) via `HookKeyFormat`, replacing the structural-key wording.
- In `resolveCurrentPaneKey()` (~lines 47-66): keep the `requireTmuxPane()` gate unchanged (missing `TMUX_PANE` still returns "must be run from inside a tmux pane"); switch the resolver selection to the new interface type; call `keyResolver.ResolveHookKey(paneID)`; on error wrap it (e.g. `fmt.Errorf("failed to resolve hook key for current pane: %w", err)`) and return `""`. Rename the local variable from `structuralKey` to `hookKey` for clarity (optional but recommended). The production branch still constructs `buildHooksTmuxClient()` (a `*tmux.Client`), which now satisfies the interface via the new `ResolveHookKey` method (Task 2-1).
- Do NOT change `hooksSetCmd` / `hooksRmCmd` control flow beyond the variable rename: `set` still calls `resolveCurrentPaneKey()` then `store.Set(...)`; `rm` still branches on `--pane-key` (verbatim pass-through, resolver NOT consulted) vs the `resolveCurrentPaneKey()` fallback. The `--pane-key` literal pass-through must remain a bypass — no re-derivation of the supplied key.
- Update `cmd/hooks_test.go` (package `cmd`, NO `t.Parallel()`): rename the `mockKeyResolver` method from `ResolveStructuralKey` to `ResolveHookKey` so the mock satisfies the new seam, and rename the two subtests currently titled `"ResolveStructuralKey failure returns user-facing error"` (one under `TestHooksSetCommand`, one under `TestHooksRmCommand`) to reference the hook-key resolver; adjust any error-substring assertion to the new wrap text (keep matching on `"resolve"` so the existing assertion still passes). The `--pane-key` subtests already assert the resolver is NOT consulted (a resolver that errors loudly if called) — verify they stay green unchanged.

**Acceptance Criteria**:
- [ ] `resolveCurrentPaneKey()` resolves via `ResolveHookKey(paneID)` (the Task 2-1 read using `HookKeyFormat`) — no call to `ResolveStructuralKey` remains in `cmd/hooks.go`.
- [ ] The `hooksDeps.KeyResolver` seam is a hook-key resolver interface with the single method `ResolveHookKey(paneID string) (string, error)`; the production `*tmux.Client` satisfies it.
- [ ] `portal hooks set` stores the resolved hook key: with a mock resolver returning `"tok123:0.0"`, the hook is written under key `"tok123:0.0"` (not under a raw pane ID and not under a name-based structural key unless that is what the resolver returned).
- [ ] `ResolveHookKey` read failure aborts registration: a mock resolver returning an error causes `hooks set` to return a user-facing error (containing `"resolve"`) and writes NO hooks file; and causes `hooks rm` to return the same error while leaving the existing hook entry untouched. No name-based key is synthesized on failure.
- [ ] `rm --pane-key <key>` removes the verbatim `<key>` without consulting the resolver (even with `TMUX_PANE` unset), and without re-deriving the key.
- [ ] Missing `TMUX_PANE` still errors with "must be run from inside a tmux pane" for both `set` and the `rm` fallback path (the `--pane-key` path is exempt, as today).
- [ ] `go build -o portal .` succeeds; `go test ./cmd -run 'TestHooks'` passes (the renamed mock + subtests are green).

**Tests** (`cmd/hooks_test.go`, package `cmd`, NO `t.Parallel()`):
- `"it stores the hook under the resolved hook key"` — mock resolver returns `"tok123:0.0"`; assert `hooks.json` has the entry under `"tok123:0.0"`. (Adapt the existing `"sets hook for current pane"` subtest to the renamed mock method.)
- `"it aborts hooks set when the hook-key read fails"` — mock resolver returns an error; assert `set` errors (contains `"resolve"`) and no hooks file is written (adapt the existing `"ResolveStructuralKey failure returns user-facing error"` set subtest).
- `"it aborts hooks rm when the hook-key read fails and leaves the entry intact"` — mock resolver returns an error; assert `rm` errors and the seeded entry survives (adapt the existing rm-failure subtest).
- `"it removes the verbatim key on rm --pane-key without consulting the resolver"` — resolver errors loudly if called; `--pane-key sess:0.1` removes exactly that entry (existing subtest, must stay green after the rename).
- `"it errors when TMUX_PANE is unset for set and the rm fallback"` — existing subtests; must stay green.

**Edge Cases**:
- `ResolveHookKey` read failure aborts BOTH `hooks set` and `hooks rm` (fallback path) with no side effects — no file created (set), entry preserved (rm). (Spec § Stage 1 Failure contract.)
- `rm --pane-key <key>` verbatim pass-through bypasses re-derivation and does not consult the resolver (unchanged behaviour).
- Missing `TMUX_PANE` still errors on the resolution path (the `requireTmuxPane` gate is unchanged); `--pane-key` remains exempt from that gate.

**Context**:
> Spec § Stage 1 — Registration: `resolveCurrentPaneKey()` today resolves `ResolveStructuralKey($TMUX_PANE)`, which reads the name-based `StructuralKeyFormat`. It is changed to resolve the hook key via a new client read using `HookKeyFormat` (e.g. `ResolveHookKey(paneID)` → `display-message -p -t <pane> <HookKeyFormat>`). `portal hooks set`/`rm` then store/remove under the stable key. The `--pane-key` literal pass-through on `rm` is unchanged (still a verbatim key).
> Spec § Stage 1 Failure contract: if the read itself fails (transport/exec error), registration aborts with the error, exactly as `ResolveStructuralKey` does today — it must NOT synthesize a name-based key on failure.
> Current shape (`cmd/hooks.go`): the `StructuralKeyResolver` interface (~line 15) declares `ResolveStructuralKey(paneID string) (string, error)`; `HooksDeps.KeyResolver` holds it (~line 25); `resolveCurrentPaneKey()` (~lines 47-66) picks `hooksDeps.KeyResolver` when set, else `buildHooksTmuxClient()` (a `*tmux.Client`), and calls `ResolveStructuralKey`. `hooksRmCmd` (~lines 143-165) branches: `--pane-key` set → verbatim `structuralKey = paneKey` (resolver NOT consulted); else → `resolveCurrentPaneKey()`.
> The production `*tmux.Client` gains `ResolveHookKey` in Task 2-1, so switching the seam to `ResolveHookKey` keeps the production path wired with no extra adapter.
> Testing convention: `cmd` tests inject via the package-level `hooksDeps` seam and must not use `t.Parallel()` (package-level mutable state; cleanup via `t.Cleanup`).

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation → Stage 1 — Registration + Failure contract; § Acceptance Criteria 6 (no external/UI change — the out-of-repo start-hook keeps calling `portal hooks set`).

## session-rename-orphans-resume-hook-2-3 | approved

### Task session-rename-orphans-resume-hook-2-3: Add `tmux.ListAllPaneHookKeys` enumeration (`list-panes -a` with `HookKeyFormat`)

**Problem**: The stale-cleanup live-key set (Stage 2) is built by enumerating every live pane's key via `(*Client).ListAllPanes()`, which hardcodes `StructuralKeyFormat` (name-based). To sweep hooks by the immutable `@portal-id`, cleanup needs the live set enumerated with `HookKeyFormat` instead. But `ListAllPanes` is ALSO consumed by out-of-scope name-based callers (skeleton-marker cleanup, daemon), so its format must stay `StructuralKeyFormat`. A new, hook-key-specific enumeration is required rather than repointing the shared one.

**Solution**: Add `(*Client).ListAllPaneHookKeys() ([]string, error)` to `internal/tmux/tmux.go` — the hook-key sibling of `ListAllPanes`, delegating to `ListAllPanesWithFormat(HookKeyFormat)` and the same `parsePaneOutput` parse, with the identical error-propagating contract (`(nil, err)` on any tmux failure, empty slice on empty output). Verify against a real tmux server (stamped vs un-stamped sessions) via the `tmuxtest` socket fixture, `SkipIfNoTmux`-gated. Leave `ListAllPanes` / `StructuralKeyFormat` unchanged.

**Outcome**: A `tmux.ListAllPaneHookKeys` method exists that enumerates every live pane's hook key (`<id>:w.p` for stamped sessions, `<name>:w.p` for un-stamped) with the same error/empty contract as `ListAllPanes`, ready for the cleanup wiring switch in Task 2-4 — while the name-based `ListAllPanes` stays intact for its non-hook consumers.

**Do**:
- Add `func (c *Client) ListAllPaneHookKeys() ([]string, error)` to `internal/tmux/tmux.go`, placed immediately after `ListAllPanes` (~lines 799-805) so the two enumerations sit together.
- Implement it as the byte-for-byte mirror of `ListAllPanes` except the format constant: `raw, err := c.ListAllPanesWithFormat(HookKeyFormat); if err != nil { return nil, err }; return parsePaneOutput(raw), nil`. This reuses the existing `ListAllPanesWithFormat` (`list-panes -a -F <format>`) and `parsePaneOutput` (trims + drops empty lines, returns `[]string{}` on empty input), so the error-propagating and empty-output contracts are inherited unchanged.
- Doc-comment it as the canonical live hook-key enumeration for stale cleanup: it enumerates every live pane's hook key via `HookKeyFormat` (a stamped session yields `<id>:w.p`, an un-stamped one yields `<name>:w.p`); it shares `ListAllPanes`' discriminating contract (a tmux failure returns `(nil, err)`, NOT "no live panes", because treating a failure as an empty live set would mass-orphan every `hooks.json` entry). Note explicitly that it exists SEPARATELY from `ListAllPanes` because that method's `StructuralKeyFormat` is still required by non-hook structural callers (skeleton-marker cleanup, daemon), so the hook-cleanup path gets its own format-specific enumeration.
- Do NOT modify `ListAllPanes`, `ListAllPanesWithFormat`, `StructuralKeyFormat`, or `parsePaneOutput`.
- Add a real-tmux round-trip test (new file `internal/tmux/list_all_pane_hookkeys_realtmux_test.go`, package `tmux_test`), mirroring `resolve_hookkey_realtmux_test.go` (Task 2-1): no build tag, gated by `tmuxtest.SkipIfNoTmux(t)`, using `tmuxtest.New(t, "ptl-hookkeys-")` + `ts.Client()` + `client.EnsureServer()`.

**Acceptance Criteria**:
- [ ] `(*Client).ListAllPaneHookKeys() ([]string, error)` exists and delegates to `ListAllPanesWithFormat(HookKeyFormat)` + `parsePaneOutput` (uses the Phase 1 `HookKeyFormat`, not `StructuralKeyFormat`).
- [ ] Real-tmux stamped sessions: with two sessions each stamped with distinct `@portal-id` values, the returned slice contains each session's `<id>:w.p` (id prefix, not name prefix).
- [ ] Real-tmux un-stamped sessions: an un-stamped session's live pane appears as `<name>:w.p`.
- [ ] Multi-window/multi-pane stamped session: distinct `<id>:w.p` suffixes appear (all sharing the one id).
- [ ] list-panes error propagates: when the underlying `list-panes -a` fails (e.g. server gone), `ListAllPaneHookKeys` returns `(nil, err)` with the error wrapped (recoverable via `errors.Is`/`errors.As`), NOT an empty slice.
- [ ] Empty output yields an empty (non-nil) slice: `parsePaneOutput("")` returns `[]string{}`, so an enumeration that produces no rows returns `([]string{}, nil)`.
- [ ] `ListAllPanes`, `StructuralKeyFormat`, `ListAllPanesWithFormat` are behaviourally unchanged.
- [ ] The real-tmux test carries NO build tag and skips cleanly via `tmuxtest.SkipIfNoTmux(t)` where tmux is absent.
- [ ] `go build -o portal .` succeeds; `go test ./internal/tmux/...` passes (and skips the real-tmux test where tmux is unavailable).

**Tests** (`internal/tmux/list_all_pane_hookkeys_realtmux_test.go`, package `tmux_test`, `SkipIfNoTmux`-gated, NO `t.Parallel()`):
- `"it enumerates a stamped session as id:w.p"` — create a session, stamp `@portal-id = "tok123"`, call `ListAllPaneHookKeys`, assert the slice contains `"tok123:0.0"`.
- `"it enumerates an un-stamped session as name:w.p"` — create a session without stamping, assert the slice contains `"<sessionName>:0.0"`.
- `"it enumerates distinct w.p suffixes across multiple windows and panes under one id"` — stamp a session, add a window and/or split a pane, assert distinct `"tok123:w.p"` entries.
- `"it enumerates mixed stamped and un-stamped sessions with per-session prefixes"` — one stamped + one un-stamped session; assert the stamped one appears by id and the un-stamped one by name (proves per-session conditional resolution across the `-a` enumeration).
- `"it returns a non-nil empty slice when there are no panes"` — covered structurally by `parsePaneOutput("")` returning `[]string{}`; assert against a server whose only session was killed, or via a unit-level assertion on `parsePaneOutput` if a live empty enumeration is impractical (the harness always has the anchor session, so this may be asserted by inspecting the return type contract rather than a live empty read).
- `"it propagates a list-panes failure as (nil, err)"` — kill the server (`ts.KillServer()`), then call `ListAllPaneHookKeys` and assert `(nil, non-nil err)`. (Alternatively drive the failure via a `transienttest.Commander` in `FailExitNonZero` mode wrapping the socket commander.)

**Edge Cases**:
- Stamped sessions enumerate `<id>:w.p`; un-stamped enumerate `<name>:w.p`; mixed populations resolve per-session (the `#{?@portal-id,...}` conditional is evaluated independently for each pane row).
- `list-panes -a` error → `(nil, err)` (NOT an empty slice) — the discriminating contract inherited from `ListAllPanes`; treating a failure as "no live panes" would let cleanup mass-orphan every hook.
- Empty output → `[]string{}` (non-nil empty), from `parsePaneOutput`.

**Context**:
> Spec § Stage 2 — Stale cleanup live keys: `CleanStale(liveKeys)` deletes any `hooks.json` key not in `liveKeys`. The live-key enumeration that feeds it (today `ListAllPanes()` → `ListAllPanesWithFormat(StructuralKeyFormat)`) is changed to enumerate live panes' hook keys via `HookKeyFormat`. This is the load-bearing consistency point: liveKeys must be produced by the same rule as registration, or cleanup mass-orphans every stamped session's hook. The name-based `StructuralKeyFormat` / `ListAllPanes` remain available for any non-hook structural use; only the hook-cleanup enumeration switches to the hook-key format.
> Task-batch guidance: because `ListAllPanes()` hardcodes `StructuralKeyFormat` and is ALSO used by out-of-scope name-based callers, add a NEW hook-key-specific enumeration `ListAllPaneHookKeys()` (delegating to `ListAllPanesWithFormat(HookKeyFormat)` + the same parse) and repoint only the hook-cleanup `AllPaneLister` to it (Task 2-4). The name-based `ListAllPanes` / `StructuralKeyFormat` stay unchanged.
> Existing shapes (`tmux.go`): `ListAllPanes` (~799-805) = `raw, err := c.ListAllPanesWithFormat(StructuralKeyFormat); if err != nil { return nil, err }; return parsePaneOutput(raw), nil`; `ListAllPanesWithFormat` (~747-753) runs `list-panes -a -F <format>` and wraps errors; `parsePaneOutput` (~519-534) returns `[]string{}` on empty input and trims/drops empty lines otherwise.
> This task adds the enumeration method only — the `AllPaneLister` wiring switch (which cleanup implementation calls it) is Task 2-4. No production caller invokes `ListAllPaneHookKeys` yet.
> The `tmuxtest` harness runs `-f /dev/null` → base-index 0, so a single-pane session's suffix is `0.0`.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation → Stage 2 — Stale cleanup live keys; § Hook-Key Derivation → Decoupling / new primitives; § Testing Requirements → Derivation primitives (unit) → cross-site consistency.

## session-rename-orphans-resume-hook-2-4 | approved

### Task session-rename-orphans-resume-hook-2-4: Repoint the stale-cleanup live-key enumeration (`AllPaneLister`) to hook keys

**Problem**: The stale-cleanup path (`runHookStaleCleanup` in `cmd/run_hook_stale_cleanup.go`, invoked by bootstrap step 11's `cleanStaleAdapter` and by `portal clean`'s `cleanStaleHooks`) builds its live-key set from the `AllPaneLister` interface, whose sole method `ListAllPanes()` yields name-based structural keys. After registration switches to the hook key (Task 2-2), the cleanup live set must switch too — otherwise cleanup builds a name-based live set that no longer matches the id-keyed `hooks.json` entries, and it mass-orphans every stamped session's freshly-registered hook. This is the live half of the "every key-producing site derives the same key" invariant.

**Solution**: Repoint the hook-cleanup `AllPaneLister` from `ListAllPanes` to `ListAllPaneHookKeys` (Task 2-3). Change the `AllPaneLister` interface method from `ListAllPanes()` to `ListAllPaneHookKeys()` (or, minimally, have the production wiring pass a lister whose `ListAllPanes`-shaped call returns hook keys). The production `*tmux.Client` already satisfies the hook-key method after Task 2-3, so both production wiring sites (`cmd/bootstrap_production.go` `cleanStaleAdapter{lister: client}` and `cmd/clean.go` `buildCleanPaneLister()`) keep passing the `*tmux.Client`. The `runHookStaleCleanup` algorithm, `CleanStale`, the mass-deletion hazard guard, and the swallow policy are UNCHANGED — this is a wiring switch, not an algorithm change.

**Outcome**: The hook stale-cleanup live-key set is enumerated via `HookKeyFormat`, so a stamped session's freshly-registered hook survives cleanup; the hazard guard and swallow policy are preserved byte-for-byte; and the name-based `ListAllPanes` / `StructuralKeyFormat` remain untouched for their non-hook callers (skeleton-marker cleanup, daemon).

**Do**:
- In `cmd/clean.go`, change the `AllPaneLister` interface's single method from `ListAllPanes() ([]string, error)` to `ListAllPaneHookKeys() ([]string, error)`, and update its doc-comment to say it returns each live pane's **hook key** (`<@portal-id or session_name>:w.p`) via `HookKeyFormat` — the live-key set feeding `CleanStale`. The `*tmux.Client` satisfies this via the Task 2-3 method. (Preferred: rename the interface method so the type system enforces that every hook-cleanup caller uses the hook-key enumeration, making a future name-based regression a compile error.)
- Update `runHookStaleCleanup` (`cmd/run_hook_stale_cleanup.go`) to call `lister.ListAllPaneHookKeys()` where it currently calls `lister.ListAllPanes()` (~line 89). This is the ONLY line changed in that file — the six-branch algorithm, hazard guard (`len(livePanes) == 0 && len(persisted) > 0` → Warn + defer), swallow policy (`swallowListError`), `onRemoved` callback, and all log format strings are UNCHANGED. Keep the `livePanes` local name (or rename to `liveKeys`) — the value is now hook keys.
- In `cmd/bootstrap_production.go`, the `cleanStaleAdapter{lister: client}` wiring (~line 153) and the compile-time assertion `var _ AllPaneLister = (*tmux.Client)(nil)` (~line 94) stay as-is structurally; they now bind to the hook-key method via the interface change (the `*tmux.Client` satisfies the renamed interface because Task 2-3 added `ListAllPaneHookKeys`). Confirm the compile-time assertion still holds.
- In `cmd/clean.go`, `buildCleanPaneLister()` (~lines 33-38) and `cleanStaleHooks` (~lines 84-165) need no logic change — they pass the `*tmux.Client` (production) or the injected `cleanDeps.AllPaneLister` (test) through unchanged.
- Update the test stubs that implement `AllPaneLister` to the renamed method: `stubAllPaneLister` (`cmd/bootstrap_production_test.go` ~line 108-116), `mockCleanPaneLister` (`cmd/clean_test.go` ~line 682), and `panickingPaneLister` (`cmd/cleanstale_transient_listpanes_clean_integration_test.go` ~line 85). Each is a mechanical rename of `ListAllPanes` → `ListAllPaneHookKeys` on the stub; the canned `panes`/`err` fields and their semantics are unchanged. Update the two compile-time `var _ AllPaneLister = ...` assertions in the test files accordingly.
- Confirm `internal/hooks/store.go` `CleanStale` (~line 249) is NOT touched — it consumes `liveKeys []string` opaquely and does not care whether they are name-based or id-based keys.
- Do NOT touch the out-of-scope name-based `StructuralKeyFormat` uses in `cmd/bootstrap/stale_marker_cleanup.go` or `cmd/state_daemon.go`, and do NOT change `ListAllPanes`.

**Acceptance Criteria**:
- [ ] The hook-cleanup `AllPaneLister` enumerates live hook keys via `ListAllPaneHookKeys` (`HookKeyFormat`); `runHookStaleCleanup` calls `lister.ListAllPaneHookKeys()`, not `ListAllPanes()`.
- [ ] A freshly-registered stamped-session hook survives cleanup: with a live set containing `"tok123:0.0"` and a `hooks.json` entry keyed `"tok123:0.0"`, cleanup does NOT delete it; a truly-stale entry (key absent from the live set) is still removed.
- [ ] `list-panes` error still preserves hooks (swallow policy intact): under a lister that returns `(nil, err)`, `portal clean` returns nil and leaves `hooks.json` byte-identical (Warn logged); the bootstrap adapter (`swallowListError=false`) still propagates the error. Both paths unchanged from today.
- [ ] Empty live set still triggers the mass-deletion hazard guard: with a lister returning `([]string{}, nil)` and a non-empty `hooks.json`, cleanup emits the hazard Warn and deletes NOTHING (deferral, not deletion).
- [ ] Name-based `ListAllPanes` / `StructuralKeyFormat` are unchanged and their non-hook callers (`cmd/bootstrap/stale_marker_cleanup.go`, `cmd/state_daemon.go`) are untouched.
- [ ] `internal/hooks/store.go` `CleanStale` is unchanged (no diff).
- [ ] The existing `cmd/run_hook_stale_cleanup_test.go` (all subtests: hazard guard, both-empty no-op, ListAllPanes-error swallow/propagate, onRemoved, happy path, nil logger) and `cmd/clean_test.go` stay green after the mechanical stub-method rename — the log format strings and branch behaviour are unchanged.
- [ ] `go build -o portal .` succeeds; `go test ./cmd/...` passes; `go test -tags integration ./cmd/...` (the `cleanstale_transient_listpanes_*_integration_test.go` matrix) passes with its stubs renamed.

**Tests** (`cmd` package, NO `t.Parallel()`; reuse the existing seams — `cleanDeps` / `mockCleanPaneLister`, the `stubAllPaneLister` + `runHookStaleCleanup` unit harness, and the `transienttest` failure-mode scaffolding):
- `"it preserves a stamped-session hook whose id-key matches the live set"` — new/adapted `run_hook_stale_cleanup_test.go` subtest: seed `hooks.json` with `{"tok123:0.0": {...}, "orphan:0.0": {...}}`; lister returns `["tok123:0.0"]`; assert `tok123:0.0` survives and `orphan:0.0` is removed. (Confirms the switch keys off whatever the lister returns — now hook keys.)
- `"it still fires the mass-deletion hazard guard on an empty live set"` — lister returns `[]string{}`, `hooks.json` non-empty; assert hazard Warn and no deletion (existing hazard subtest, must stay green post-rename).
- `"it still swallows a list-panes error under portal clean and preserves hooks"` — `cleanDeps.AllPaneLister` returns `(nil, err)`; assert `portal clean` returns nil and `hooks.json` unchanged (existing `clean_test.go` subtest, must stay green).
- `"it still propagates a list-panes error under the bootstrap adapter"` — `runHookStaleCleanup(..., swallowListError=false, ...)` with an erroring lister returns the error (existing subtest, must stay green).
- Integration (`//go:build integration`): confirm `cleanstale_transient_listpanes_clean_integration_test.go` and its bootstrap sibling still pass after renaming `panickingPaneLister.ListAllPanes` → `ListAllPaneHookKeys` — the `transienttest.Commander` intercepts `list-panes -a` regardless of the `-F` format, so `FailExitNonZero` / `FailEmptyStdout` still drive the swallow / hazard branches unchanged.

**Edge Cases**:
- Freshly-registered stamped-session hook survives cleanup (the live set now carries the same id-key registration stored).
- `list-panes` error → hooks preserved (swallow policy unchanged: `portal clean` returns nil + Warn; bootstrap propagates).
- Empty live set + non-empty hooks → hazard guard defers (no deletion), unchanged.
- Name-based `ListAllPanes` / `StructuralKeyFormat` unchanged for skeleton-marker cleanup and daemon use (out of scope).

**Context**:
> Spec § Stage 2 — Stale cleanup live keys: `CleanStale(liveKeys)` deletes any `hooks.json` key not in `liveKeys`. The live-key enumeration that feeds it is changed to enumerate live panes' hook keys via `HookKeyFormat`. Only the hook-cleanup enumeration switches; the name-based `StructuralKeyFormat` / `ListAllPanes` remain available for non-hook structural use.
> Spec § Cross-Reboot Persistence (b): post-restore stale-cleanup builds its live-key set from the live `@portal-id`; consistency between registration and cleanup is what keeps a freshly-restored hook from being deleted.
> `runHookStaleCleanup` (`cmd/run_hook_stale_cleanup.go`) is the single source of truth for the prune algorithm — six branches, a mass-deletion hazard guard (`len(livePanes)==0 && len(persisted)>0` → Warn + defer), and a `swallowListError` policy (bootstrap passes false → propagate; `portal clean` passes true → Warn + return nil). ONLY the enumeration call switches; every branch, guard, and log format string is unchanged. The integration substring-asserts pin those format strings — do not reword them.
> Production wiring: `cmd/bootstrap_production.go` builds `cleanStaleAdapter{lister: client}` (~line 153) where `client` is a `*tmux.Client`; `cmd/clean.go` `buildCleanPaneLister()` returns the injected `cleanDeps.AllPaneLister` or `tmux.DefaultClient()`. Both pass a `*tmux.Client`, which satisfies the renamed interface via the Task 2-3 `ListAllPaneHookKeys` method — so no production wiring line changes beyond the interface-method rename.
> `internal/hooks/store.go` `CleanStale(liveKeys []string)` treats the keys opaquely (builds a `map[string]struct{}` set membership) — it does not distinguish name-based from id-based keys, so it needs no change.
> Testing convention: `cmd` tests mutate `cleanDeps` / package state and must not use `t.Parallel()` (cleanup via `t.Cleanup`). Daemon-spawning / bootstrap-subprocess integration tests must use `portaltest.IsolateStateForTest` and apply the returned env.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation → Stage 2 — Stale cleanup live keys; § Cross-Reboot Persistence (b); § Acceptance Criteria 4 (cleanup safety — never mass-orphans on upgrade); § Scope & Non-Goals → Out of scope (skeleton markers stay structural-keyed).

## session-rename-orphans-resume-hook-2-5 | approved

### Task session-rename-orphans-resume-hook-2-5: Cross-site consistency test — registration read == cleanup enumeration (live half)

**Problem**: The fix's central invariant is that every key-producing site derives the identical hook key; if registration (`ResolveHookKey`, Task 2-1/2-2) and stale-cleanup enumeration (`ListAllPaneHookKeys`, Task 2-3/2-4) ever disagree for the same live session, cleanup would delete the very hook registration just stored — reintroducing the orphan bug at scale. Tasks 2-1 through 2-4 each verify one site in isolation; nothing yet asserts the two live-tmux sites produce byte-identical keys for the same session.

**Solution**: Add a real-tmux integration test (`tmuxtest` socket fixture, `SkipIfNoTmux`-gated) that, against a single live tmux server, resolves a session's pane key via `ResolveHookKey` (the registration read) and enumerates the same server's live hook keys via `ListAllPaneHookKeys` (the cleanup read), then asserts the registration key appears byte-identically in the cleanup enumeration — for a stamped session, a multi-pane stamped session, and an un-stamped session.

**Outcome**: A committed cross-site consistency guard proves that, for the same live session, the registration read and the cleanup live-key enumeration yield byte-identical keys — stamped sessions agree on `<id>:w.p` (each pane independently), and un-stamped sessions agree on the name-based `<name>:w.p` — closing the live half of the "every site derives the same key" invariant.

**Do**:
- Add a new real-tmux test file `internal/tmux/hookkey_cross_site_realtmux_test.go` (package `tmux_test`), no build tag, gated by `tmuxtest.SkipIfNoTmux(t)`, using `tmuxtest.New(t, "ptl-xsite-")` + `ts.Client()` + `client.EnsureServer()`. (Placing it in `internal/tmux` keeps both primitives — `ResolveHookKey` and `ListAllPaneHookKeys` — in one package under test without cross-package wiring.)
- Stamped single-pane case: create a session, stamp `@portal-id = "tok123"` via `client.SetSessionOption(name, "@portal-id", "tok123")`, wait for it (`ts.WaitForSession`). Resolve the registration key: `reg, _ := client.ResolveHookKey(name)` (targeting the session resolves against its active pane). Enumerate cleanup keys: `live, _ := client.ListAllPaneHookKeys()`. Assert `reg == "tok123:0.0"` AND `slices.Contains(live, reg)` — byte-identical membership.
- Multi-pane stamped case: on a stamped session, split a pane (`client.SplitWindow(...)`) and/or add a window (`client.NewWindow(...)`). Resolve each pane's registration key via its `-t` pane target (obtain each pane's `#{pane_id}` through `ts.Run("list-panes", "-s", "-t", name, "-F", "#{pane_id}")` then `ResolveHookKey(paneID)`), and assert every per-pane registration key is present in the single `ListAllPaneHookKeys()` slice — all sharing the `tok123` prefix with distinct `w.p` suffixes.
- Un-stamped case: create a session WITHOUT stamping. `reg := ResolveHookKey(name)` must equal `"<sessionName>:0.0"`, and `ListAllPaneHookKeys()` must contain that same name-based key — proving both sites agree on the name fallback for legacy sessions.
- Assert byte-identity, not just structural equivalence — compare the exact strings (the invariant is "byte-identical keys" per the spec). Use `slices.Contains` (import `slices`) for membership.
- Keep the cleanup enumeration filtered/scoped to the sessions this test created if the harness's anchor/bootstrap sessions would otherwise pollute the slice — assert membership (`Contains`), not full-slice equality, so unrelated anchor panes do not break the test.

**Acceptance Criteria**:
- [ ] For a stamped single-pane session, `ResolveHookKey(name)` returns `"tok123:0.0"` and that exact string is a member of `ListAllPaneHookKeys()` (byte-identical).
- [ ] For a multi-pane stamped session, every per-pane `ResolveHookKey(paneID)` result is a member of the single `ListAllPaneHookKeys()` slice, all sharing the `tok123` prefix with distinct `w.p` suffixes (each pane independently addressable and agreed across sites).
- [ ] For an un-stamped session, both sites agree on the name-based key `"<sessionName>:0.0"` (registration read equals the cleanup enumeration entry, byte-identical).
- [ ] The assertion is byte-identity (exact string equality / `slices.Contains`), not structural/normalised equivalence.
- [ ] The test carries NO build tag and skips cleanly via `tmuxtest.SkipIfNoTmux(t)` where tmux is absent.
- [ ] `go build -o portal .` succeeds; `go test ./internal/tmux/...` passes (skips where tmux is unavailable).

**Tests** (`internal/tmux/hookkey_cross_site_realtmux_test.go`, package `tmux_test`, `SkipIfNoTmux`-gated, NO `t.Parallel()`):
- `"it agrees on the id key across registration and cleanup for a stamped session"`.
- `"it agrees on distinct per-pane id keys across both sites for a multi-pane stamped session"`.
- `"it agrees on the name-based key across both sites for an un-stamped session"`.

**Edge Cases**:
- Multi-pane session: distinct `w.p` suffixes under one id must agree across sites (each pane's registration key is a member of the cleanup slice) — spec Testing Requirements → multi-pane.
- Un-stamped session: both sites agree on the name-based key (the name fallback coincides across registration and cleanup — the no-migration guarantee for legacy sessions).

**Context**:
> Spec § Testing Requirements → Derivation primitives (unit) → Cross-site consistency: for a given stamped session, the registration read (`ResolveHookKey`), the cleanup live-key enumeration, and the restore baker (`HookKey` from saved state) produce byte-identical keys. (The restore-baker leg is Phase 3; this task covers the LIVE half — registration read == cleanup enumeration.)
> Spec § Hook-Key Derivation: the fix's central invariant is that every site that produces or consumes a hook key derives it by the identical rule — `prefer @portal-id, else session_name`, suffixed `:window.pane`. If any site disagrees, hooks orphan (the bug).
> Spec § Risks → Missed key-producing site (primary risk): if any current or future caller builds a hook key or live-key set from the name-based `StructuralKeyFormat` instead of the hook-key derivation, stamped sessions' hooks orphan at scale. Mitigation: a single shared `HookKeyFormat`/`HookKey` primitive plus the cross-site consistency test — THIS test.
> The two live sites both read `HookKeyFormat`: `ResolveHookKey` via `display-message -p -t <pane> <HookKeyFormat>` (Task 2-1), and `ListAllPaneHookKeys` via `list-panes -a -F <HookKeyFormat>` (Task 2-3). This test confirms tmux resolves the identical conditional to the identical bytes across the two read shapes.
> `tmuxtest` harness runs `-f /dev/null` → base-index 0. Use `ts.WaitForSession` after creation on slow CI. Assert membership (`slices.Contains`) rather than whole-slice equality so anchor/bootstrap panes do not perturb the assertion.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation (central invariant); § Testing Requirements → Derivation primitives (unit) → Cross-site consistency; § Risks → Missed key-producing site.

## session-rename-orphans-resume-hook-2-6 | approved

### Task session-rename-orphans-resume-hook-2-6: No-regression test — un-stamped, never-renamed `hooks.json` entry survives upgrade

**Problem**: The fix must be a no-migration upgrade: an existing `hooks.json` entry for an un-stamped, never-renamed session (created before `@portal-id` shipped) is keyed by the session name. After registration and cleanup switch to `HookKeyFormat`, an un-stamped session's live key falls back to the name — which must coincide with the on-disk key so the entry keeps resolving and is NOT mass-orphaned by the switched cleanup. Nothing yet proves this specific no-regression outcome end-to-end (a pre-fix name-keyed entry surviving the new cleanup enumeration), which is the acceptance-boundary the spec calls out for upgrade safety.

**Solution**: Add a real-tmux integration test (`tmuxtest` socket fixture, `SkipIfNoTmux`-gated) that seeds a `hooks.json` with a name-based entry matching an un-stamped live session (simulating a pre-upgrade entry), runs the hook stale-cleanup path (`runHookStaleCleanup` fed by `ListAllPaneHookKeys` against the live server), and asserts: the un-stamped session's entry survives (its name-based live key coincides with the on-disk key), while a truly-stale name-keyed entry (no matching live pane) is still swept.

**Outcome**: A committed no-regression guard proves that after the cleanup switch, an un-stamped/never-renamed session's pre-existing name-keyed `hooks.json` entry survives the stale-cleanup pass (name fallback coincides with the on-disk key), and that genuinely-orphaned name-keyed entries are still removed — confirming the no-migration upgrade path.

**Do**:
- Add a new integration test file `cmd/hookkey_no_regression_upgrade_test.go` (package `cmd`, NO `t.Parallel()`; use `//go:build integration` only if it must spawn subprocesses/bootstrap — if it drives `runHookStaleCleanup` directly against a `tmuxtest` socket client it can run untagged, matching the untagged real-tmux pattern in `internal/tmux`). Prefer driving `runHookStaleCleanup` directly with a `*tmux.Client` built from a `tmuxtest` socket so the test needs no binary build and no `//go:build integration` tag; gate with `tmuxtest.SkipIfNoTmux(t)`.
- Set up a live tmux server via `tmuxtest.New(t, "ptl-upgrade-")` + `ts.Client()` + `client.EnsureServer()`. Create ONE un-stamped session (do NOT stamp `@portal-id`) named e.g. `legacy-proj`. Wait for it (`ts.WaitForSession`). Its live hook key resolves to the NAME-based `legacy-proj:0.0` (un-stamped → name fallback).
- Seed a `hooks.json` (via `newTempHooksStore`-style temp file or `transienttest.SeedHooksJSON`) with two entries: `"legacy-proj:0.0"` (matches the live session — must survive) and `"gone-session:0.0"` (no matching live pane — truly stale, must be swept).
- Drive the cleanup: build the lister from the socket `*tmux.Client` (so its `ListAllPaneHookKeys` enumerates the live un-stamped session as `legacy-proj:0.0`), and call `runHookStaleCleanup(client, store, logger, false, nil)` (or via `cleanStaleHooks` / `portal clean` if exercising the full RunE is preferred). Because the un-stamped session's live hook key equals the on-disk name-based key, `CleanStale` keeps `legacy-proj:0.0` and removes `gone-session:0.0`.
- Assert post-cleanup `hooks.json`: `legacy-proj:0.0` is present (survived), `gone-session:0.0` is absent (swept). Read back via `store.Load()` or the raw file.
- Guard against the vacuous pass: assert the pre-cleanup seed actually contained both entries (so a green result cannot be "nothing was there").
- Do NOT test rename here (the rename-then-reboot headline coverage is Phase 3 — this task is strictly the un-stamped, NEVER-renamed no-migration case). Do NOT test persistence/capture/restore (Phase 3).

**Acceptance Criteria**:
- [ ] After cleanup, the un-stamped/never-renamed session's pre-existing name-keyed entry `legacy-proj:0.0` is PRESENT in `hooks.json` (survived — its name-based live key coincides with the on-disk key).
- [ ] After cleanup, the truly-stale name-keyed entry `gone-session:0.0` (no matching live pane) is ABSENT (swept) — proving cleanup still removes genuine orphans.
- [ ] The live session is genuinely un-stamped (no `@portal-id`), so its hook key is the name-based `legacy-proj:0.0` (assert or ensure via not stamping) — the test exercises the name-fallback coincidence, not an id match.
- [ ] Non-vacuous: the pre-cleanup seed is asserted to contain both `legacy-proj:0.0` and `gone-session:0.0`.
- [ ] The test skips cleanly via `tmuxtest.SkipIfNoTmux(t)` where tmux is absent; NO `t.Parallel()`.
- [ ] `go build -o portal .` succeeds; the test passes (`go test ./cmd/...`, or `go test -tags integration ./cmd/...` if tagged) and skips where tmux is unavailable.

**Tests** (`cmd/hookkey_no_regression_upgrade_test.go`, package `cmd`, `SkipIfNoTmux`-gated, NO `t.Parallel()`):
- `"it preserves an un-stamped never-renamed session's pre-existing name-keyed hook after upgrade cleanup"` — seed `legacy-proj:0.0` + `gone-session:0.0`; live un-stamped `legacy-proj`; run cleanup; assert `legacy-proj:0.0` survives and `gone-session:0.0` is swept.

**Edge Cases**:
- Name fallback coincides with on-disk key → the pre-existing entry is preserved (no mass-orphan on upgrade) — the no-migration guarantee.
- A truly-stale name-keyed entry (no live pane) is still swept — cleanup's correctness is not weakened by the no-regression protection.

**Context**:
> Spec § Testing Requirements → Legacy / no-regression (integration): a pre-fix, name-keyed `hooks.json` entry for an un-stamped, never-renamed session still resolves and is NOT mass-orphaned by stale-cleanup after upgrade (the name fallback coincides with the on-disk key).
> Spec § Acceptance Criteria 5 — No-migration upgrade: after upgrading, an un-stamped, never-renamed session's existing `hooks.json` entry still resolves and fires (name fallback coincides with the on-disk key). No `sessions.json` migration and no schema `Version` bump.
> Spec § Fix Overview → Hook key = prefer `@portal-id`, else session name: un-stamped sessions (legacy, manually-created tmux sessions, or a best-effort stamp that failed) fall back to the session name — which equals the key already on disk, so existing `hooks.json` entries keep matching with no migration.
> An un-stamped session's live hook key is the name-based `<name>:w.p` (tmux's `#{?@portal-id,...}` conditional treats absent `@portal-id` as false). The cleanup enumeration `ListAllPaneHookKeys` (Task 2-3/2-4) therefore emits the name key for a legacy session, matching the seeded on-disk key — so `CleanStale` keeps it.
> `runHookStaleCleanup` is the shared cleanup helper (`cmd/run_hook_stale_cleanup.go`); the `cmd` test-package helpers `newTempHooksStore` (`cmd/bootstrap_production_test.go`) and `readFileBytes` are reusable for seeding/reading the store. `transienttest.SeedHooksJSON` / `HooksJSONBytes` are available if driving via env-resolved paths.
> Firing (hydrate) is NOT exercised here — this task is the cleanup no-regression half; hook FIRING after restore is Phase 3. This task proves the entry SURVIVES the switched cleanup, the precondition for firing.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Testing Requirements → Legacy / no-regression (integration); § Acceptance Criteria 5 — No-migration upgrade; § Fix Overview → Hook key = prefer `@portal-id`, else session name / Coverage / natural migration.

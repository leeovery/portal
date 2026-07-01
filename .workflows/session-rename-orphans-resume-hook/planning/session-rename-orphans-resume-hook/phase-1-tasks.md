---
phase: 1
phase_name: Stable Identity Foundation — @portal-id stamping + hook-key primitives
total: 5
---

## session-rename-orphans-resume-hook-1-1 | approved

### Task session-rename-orphans-resume-hook-1-1: Add `tmux.HookKey` pure Go hook-key formatter

**Problem**: Resume hooks are keyed today by the mutable `session_name:window.pane` structural key, which desynchronizes across a rename and silently orphans the hook after the next reboot. The fix keys hooks off an immutable `@portal-id` instead, but the saved-state restore path (Stage 3, `internal/restore/session.go` → `collectArmInfos`) must reconstruct that same key from *persisted* values rather than a live tmux read. There is no in-Go primitive that applies the "prefer `@portal-id`, else session name" rule to saved values.

**Solution**: Add a pure Go formatter `HookKey(portalID, name string, window, pane int) string` to `internal/tmux` that returns `<portalID>:w.p` when `portalID != ""` and `<name>:w.p` when `portalID == ""`. It is the in-Go mirror of the tmux conditional in `HookKeyFormat` (Task 1-2), used at the saved-path baking site where the values come from `sessions.json` rather than a live server read.

**Outcome**: A stateless, table-driven-tested `tmux.HookKey` exists and produces byte-identical keys to what a live `HookKeyFormat` read yields for the same session — so a stamped session's restore-baked key matches the key registration stored, regardless of any intervening rename.

**Do**:
- Add `HookKey(portalID, name string, window, pane int) string` to `internal/tmux/tmux.go`, placed logically near `PaneTarget`/`StructuralKeyFormat` (~lines 545-805) so the hook-key primitives live together.
- Implement the conditional: when `portalID != ""` return `fmt.Sprintf("%s:%d.%d", portalID, window, pane)`; otherwise return `fmt.Sprintf("%s:%d.%d", name, window, pane)`. Do not trim, sanitize, or validate the inputs — the token is opaque alphanumeric and the name is used verbatim, matching what a live tmux read would emit.
- Add a doc-comment stating this is the canonical hook-key formatter for the saved path, the in-Go mirror of `HookKeyFormat`, and carrying the load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant transferred from the retired doc-comments (see Task 1-5). Do NOT modify `PaneTarget` — it stays the name-based `-t` target formatter.
- Add a table-driven unit test (new file `internal/tmux/hookkey_test.go` in package `tmux_test`, mirroring `TestPaneTarget` at `tmux_test.go:2862`).

**Acceptance Criteria**:
- [ ] `tmux.HookKey("id-abc", "my-project", 0, 1)` returns `"id-abc:0.1"` (non-empty portalID wins over name).
- [ ] `tmux.HookKey("", "my-project", 2, 3)` returns `"my-project:2.3"` (empty portalID falls back to name).
- [ ] `tmux.HookKey("", "", 0, 0)` returns `":0.0"` (empty portalID and empty name yields the degenerate `:w.p` form without panic).
- [ ] Multi-pane distinct suffixes under one id: `HookKey("id-abc", "x", 0, 0)`, `HookKey("id-abc", "x", 0, 1)`, `HookKey("id-abc", "x", 1, 0)` yield `"id-abc:0.0"`, `"id-abc:0.1"`, `"id-abc:1.0"` — each pane addressable independently under the same id.
- [ ] Base-index-1 indices: `HookKey("id-abc", "x", 1, 1)` returns `"id-abc:1.1"` (1-based indices pass through verbatim).
- [ ] `PaneTarget`, `PaneTargetExact`, and `StructuralKeyFormat` are behaviourally unchanged (no edits to their bodies).
- [ ] `go build -o portal .` succeeds and `go test ./internal/tmux/...` passes.

**Tests** (`internal/tmux/hookkey_test.go`, package `tmux_test`, table-driven, NO `t.Parallel()`):
- `"it returns id:w.p when portalID is non-empty"`
- `"it falls back to name:w.p when portalID is empty"`
- `"it yields :w.p when both portalID and name are empty"`
- `"it produces distinct w.p suffixes for multiple panes under one id"`
- `"it passes base-index-1 indices through verbatim"`
- `"it passes zero indices through verbatim"`

**Edge Cases**:
- Empty `portalID` → name fallback (the legacy / un-stamped / failed-stamp path — the same role `@portal-dir`'s lazy resolver plays).
- Empty `portalID` AND empty `name` → degenerate `:w.p`; must not panic (defensive: an empty-name saved session is malformed but must degrade gracefully, per Acceptance Criterion 8 in the spec).
- Multi-pane distinct `w.p` suffixes under one id (spec Testing Requirements → derivation primitives).
- Base-index-1 and zero indices both pass through unmodified.

**Context**:
> Spec § Hook-Key Derivation: `HookKey(portalID, name string, window, pane int) string` — a pure formatter for the saved path: returns `<portalID>:w.p` when `portalID != ""`, else `<name>:w.p`. The in-Go mirror of the tmux conditional, for use where the values come from saved state rather than a live tmux read.
> Spec § Testing Requirements → Derivation primitives (unit): `tmux.HookKey(portalID, name, w, p)` returns `<id>:w.p` when `portalID != ""` and `<name>:w.p` when empty; covers multi-pane (distinct `w.p` suffixes under one id).
> The load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant transfers to `HookKey` (spec § Hook-Key Derivation → Deliverable).
> This task adds the primitive only — it does not wire it into `collectArmInfos` (that is Stage 3 / Phase 3). No consumer switches to it in Phase 1.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation (the four stages); § Testing Requirements → Derivation primitives (unit); § Fix Overview → Hook key = prefer `@portal-id`, else session name.

## session-rename-orphans-resume-hook-1-2 | approved

### Task session-rename-orphans-resume-hook-1-2: Add `tmux.HookKeyFormat` tmux format string and verify against real tmux

**Problem**: The two live-tmux key sites (registration in `cmd/hooks.go` and the stale-cleanup live-key enumeration) will resolve a pane's hook key by reading tmux directly. They need a single canonical format string that lets *tmux itself* choose `@portal-id` when stamped and `session_name` when not — via a per-session conditional resolved inside tmux, with no Go-side "id absent" branch. No such format string exists, and its correctness against a real tmux server (conditional syntax, field resolution) cannot be proven by a pure Go unit test.

**Solution**: Add the exported constant `HookKeyFormat = "#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}"` to `internal/tmux`, and add a real-tmux round-trip test (gated by `tmuxtest.SkipIfNoTmux`) proving the tmux conditional resolves to `<id>:w.p` for a stamped session and `<name>:w.p` for an un-stamped one, including across multiple windows/panes.

**Outcome**: A canonical `tmux.HookKeyFormat` string exists and is verified against a live tmux server to resolve the stamped-vs-un-stamped conditional correctly, so a `display-message -p -t <pane> <HookKeyFormat>` read (wired up in Phase 2) yields the same key `HookKey` produces from saved state.

**Do**:
- Add `const HookKeyFormat = "#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}"` to `internal/tmux/tmux.go`, near `StructuralKeyFormat` (~line 779) so the structural and hook-key formats sit together.
- Doc-comment it as the canonical tmux format string for live hook-key reads, the tmux-resolved sibling of the pure-Go `HookKey`, and carry the "stable across releases — changing it silently invalidates every `hooks.json` entry" invariant (transferred per Task 1-5). Note tmux's `#{?cond,a,b}` conditional treats an unset/empty `@portal-id` as false, so an un-stamped session yields the `#{session_name}` branch.
- Add a real-tmux round-trip test (new file `internal/tmux/hookkey_format_realtmux_test.go`, package `tmux_test`), mirroring the structure of `portal_dir_roundtrip_realtmux_test.go`: no build tag, gated only by `tmuxtest.SkipIfNoTmux(t)`, using `tmuxtest.New(t, "hookkey-")` + `ts.Client()`.
- In the test, drive the format via a `display-message -p -t <target> <HookKeyFormat>` read. Because `HookKeyFormat` embeds no `-F`, resolve it through the socket harness (e.g. `ts.Run(t, "display-message", "-p", "-t", target, tmux.HookKeyFormat)`, trimming the trailing newline) so the assertion runs against the exact production format string.
- Stamped case: create a session, `SetSessionOption(name, "@portal-id", "<token>")` (use a fixed alphanumeric token, e.g. `"tok123"`), read the pane's hook key, assert it equals `"tok123:0.0"` (or the base-index-appropriate `w.p`).
- Un-stamped case: create a session WITHOUT stamping `@portal-id`, read the pane's hook key, assert it equals `"<sessionName>:0.0"`.
- Multi-window/multi-pane case: on a stamped session, add a window and/or split a pane, read each pane's hook key by its `-t` target, assert distinct `<token>:w.p` suffixes (all sharing the one id). Use the isolated socket's real base-index (the harness passes `-f /dev/null`, so base-index/pane-base-index default to 0 — see `tmuxtest.Socket.cmd` doc-comment).

**Acceptance Criteria**:
- [ ] `tmux.HookKeyFormat` equals `#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}` byte-for-byte, and the embedded literal `@portal-id` matches `session.PortalIDOption` (Task 1-3).
- [ ] Real-tmux stamped session: reading `HookKeyFormat` against a session with `@portal-id = "tok123"` yields `"tok123:0.0"`.
- [ ] Real-tmux un-stamped session: reading `HookKeyFormat` against a session with no `@portal-id` yields `"<sessionName>:0.0"`.
- [ ] Real-tmux multi-window/multi-pane stamped session: each pane's read yields the same id prefix with a distinct `:w.p` suffix.
- [ ] The real-tmux test carries NO build tag and skips cleanly via `tmuxtest.SkipIfNoTmux(t)` in tmux-less environments (no failure).
- [ ] `StructuralKeyFormat` is unchanged; `go build -o portal .` succeeds; `go test ./internal/tmux/...` passes (and skips the real-tmux test where tmux is absent).

**Tests** (`internal/tmux/hookkey_format_realtmux_test.go`, package `tmux_test`, `SkipIfNoTmux`-gated, NO `t.Parallel()`):
- `"it resolves to id:w.p for a stamped session"`
- `"it resolves to name:w.p for an un-stamped session"`
- `"it resolves distinct w.p suffixes across multiple windows and panes under one id"`
- `"it skips cleanly when tmux is unavailable"` (covered structurally by the `SkipIfNoTmux` gate — no separate case needed).

**Edge Cases**:
- Un-stamped session: tmux's `#{?@portal-id,...}` conditional treats absent/empty `@portal-id` as false → `#{session_name}` branch. This is the legacy / no-migration fallback (spec § Fix Overview → Coverage / natural migration).
- Stamped session yields `<id>:w.p` — rename-immune because the id is carried on the session object, not its name.
- Multi-window/multi-pane: distinct `#{window_index}.#{pane_index}` suffixes must resolve per-pane under a single shared id (spec § Testing Requirements → derivation primitives).

**Context**:
> Spec § Hook-Key Derivation → new primitives: `HookKeyFormat` — a tmux format string for live reads: `#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}`. tmux resolves the conditional per-session: a stamped session yields `<id>:w.p`, an un-stamped one yields `<name>:w.p`.
> Spec § Testing Requirements → Derivation primitives (unit): `HookKeyFormat` resolves correctly against a real tmux session both stamped (yields `<id>:w.p`) and un-stamped (yields `<name>:w.p`).
> The real-tmux guard closes the seam that isolated unit tests cannot: the conditional syntax and per-session field resolution only prove out against a live server (same rationale as `portal_dir_roundtrip_realtmux_test.go`).
> This task adds the format string and its guard only — the `ResolveHookKey` read wiring in `cmd/hooks.go` and the cleanup enumeration switch are Phase 2. No production caller reads `HookKeyFormat` yet.
> `tmuxtest.New(t, prefix)` returns a `*Socket`; `ts.Client()` yields a `*tmux.Client` wired to the isolated socket; `ts.Run(t, args...)` executes a raw tmux command against that socket and returns combined output (trim the trailing newline before comparing); `ts.WaitForSession(t, name, timeout)` polls until the session is queryable. The harness runs `-f /dev/null`, so base-index defaults to 0.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation (the four stages) → new derivation primitives; § Testing Requirements → Derivation primitives (unit).

## session-rename-orphans-resume-hook-1-3 | approved

### Task session-rename-orphans-resume-hook-1-3: Add `session.PortalIDOption` constant and stamp `@portal-id` in `CreateFromDir`

**Problem**: Portal has no immutable, rename-immune per-session identity. Resume hooks anchor to the mutable session name, so a rename orphans them. The fix introduces a `@portal-id` session user-option — a frozen-at-creation opaque token — but no constant names it and neither first-party creation path stamps it. `CreateFromDir` (`internal/session/create.go`) is the first of the two paths.

**Solution**: Add the shared constant `session.PortalIDOption = "@portal-id"` (parallel to `PortalDirOption`), and in `CreateFromDir` generate a fresh token via the injected id generator (`sc.gen`) and stamp it with `SetSessionOption(name, PortalIDOption, token)` immediately after `NewSession`, alongside the existing `@portal-dir` stamp. Both a token-generation error and a `SetSessionOption` error are swallowed — the session is created un-stamped and falls back to the name, never aborting creation.

**Outcome**: Every session created via `SessionCreator.CreateFromDir` carries a fresh, immutable `@portal-id`; a generation-or-stamp failure leaves the session un-stamped but successfully created (best-effort, mirroring `@portal-dir`).

**Do**:
- Add `const PortalIDOption = "@portal-id"` to `internal/session/create.go` (immediately below `PortalDirOption` at ~line 14). Doc-comment it as the immutable, rename-immune Portal session identity: a fresh opaque token frozen at creation, carried on the session object (not its name), keyed on by resume hooks; parallel to `PortalDirOption` but persisted across reboots (unlike `@portal-dir`, which lazy-re-derives — a Phase 3 concern, but note the divergence). It must stay byte-identical to the `@portal-id` literal embedded in `tmux.HookKeyFormat` (Task 1-2).
- In `CreateFromDir` (~lines 80-98), after the successful `NewSession` and near the existing `@portal-dir` stamp (~line 96): generate a token via `token, genErr := sc.gen()`; if `genErr == nil`, call `_ = sc.tmux.SetSessionOption(prepared.SessionName, PortalIDOption, token)`. If `genErr != nil`, skip the stamp entirely (session stays un-stamped). Swallow both the generation error and the stamp error — never return them, never abort creation. Order relative to the `@portal-dir` stamp does not matter; place the `@portal-id` stamp adjacent to it.
- Match the token width to the existing `NewNanoIDGenerator` scheme (`sc.gen`, 6-char alphanumeric via `suffixLen` in `internal/session/naming.go`). The spec permits widening the token "if warranted" for birthday-collision safety, but stamping reuses `sc.gen` (the same generator used for names). DECISION: reuse `sc.gen` at its current 6-char width without widening for Phase 1 — the constant + stamp is the deliverable, and the spec calls token width an implementation detail. Note the residual in Context; do not introduce a second generator or change `suffixLen`.
- Update the doc-comment on the existing `@portal-dir` stamp block (or add a sibling comment) so it is clear both options are stamped best-effort at this point.
- Extend `internal/session/create_test.go` (package `session_test`, mirroring the existing `@portal-dir` stamp tests at ~lines 431-529). The existing `mockTmuxClient` records only ONE `setOption*` call; because both `@portal-dir` and `@portal-id` now stamp, upgrade the mock to record ALL `SetSessionOption` calls (e.g. a `setOptionCalls []struct{Session,Name,Value string}` slice) so the test can assert the `@portal-id` call independently without clobbering the `@portal-dir` assertion. Update existing `@portal-dir` assertions to read from the recorded slice.

**Acceptance Criteria**:
- [ ] `session.PortalIDOption == "@portal-id"` and is exported/importable by all stamp sites (matches the literal in `tmux.HookKeyFormat`).
- [ ] On a successful create, `CreateFromDir` calls `SetSessionOption(sessionName, "@portal-id", <token>)` where `<token>` is the value returned by `sc.gen` on the stamp call, and the stamp targets the created session name.
- [ ] The `@portal-id` stamp is emitted alongside (not instead of) the existing `@portal-dir` stamp — both `SetSessionOption` calls are made on a successful create.
- [ ] Token-generation error swallowed: when the id generator returns an error at the stamp point, no `@portal-id` `SetSessionOption` call is made and `CreateFromDir` still returns the session name with no error (session created un-stamped). (Note: the name-generation call earlier in the pipeline is a separate `gen` invocation whose failure aborts creation as today — this criterion concerns the *stamp-time* generation only. If the injected generator errors on every call, name generation fails first; author the test with a generator that succeeds for the name and fails only for the stamp, OR assert via a generator that returns a distinct token for the stamp; see Tests.)
- [ ] `SetSessionOption` error swallowed: when `SetSessionOption` returns an error for the `@portal-id` stamp, `CreateFromDir` still returns the session name with no error (creation not aborted).
- [ ] Stamp is best-effort and non-fatal: no code path makes `@portal-id` stamping fail session creation.
- [ ] `go build -o portal .` succeeds; `go test ./internal/session/...` passes (existing `@portal-dir` assertions still green against the upgraded mock).

**Tests** (`internal/session/create_test.go`, package `session_test`, NO `t.Parallel()`):
- `"it stamps @portal-id with a fresh token after creating a session"` — assert a `SetSessionOption(sessionName, session.PortalIDOption, token)` call is recorded.
- `"it stamps both @portal-dir and @portal-id on a successful create"` — assert both option names appear in the recorded `SetSessionOption` calls.
- `"it returns the session name when the @portal-id stamp SetSessionOption fails"` — mock `setOptionErr` set; assert no error and correct name.
- `"it creates the session un-stamped when stamp-time token generation fails"` — inject a generator that yields a valid suffix for the name step then errors for the stamp step; assert the session name is returned, no error, and no `@portal-id` `SetSessionOption` recorded. (Because `sc.gen` is called once for the name and once for the stamp, drive the two calls to differ — e.g. a call-counting generator.)
- `"it does not stamp @portal-id when NewSession fails"` — mock `newSessionErr` set; assert no `SetSessionOption` for `@portal-id` recorded and creation errors (existing behaviour preserved).

**Edge Cases**:
- Token-generation error at stamp time → step skipped, session created un-stamped, name fallback (spec § Fix Overview → Where it is stamped).
- `SetSessionOption` error → swallowed, session created un-stamped, name fallback.
- `NewSession` failure → neither stamp runs (guarded by the early return, existing behaviour).
- Best-effort / non-fatal throughout — consistent with `@portal-dir` and with QuickStart's failure branch (Task 1-4).

**Context**:
> Spec § Fix Overview → Where it is stamped → `SessionCreator.CreateFromDir`: a best-effort `SetSessionOption(name, PortalIDOption, <token>)` immediately after `NewSession`, alongside the existing `@portal-dir` stamp. The `<token>` is generated in Go via the `SessionCreator`'s injected id generator (`sc.gen`, the same generator used for names), just before the stamp call. Both a token-generation error and a `SetSessionOption` error are swallowed (no log component), leaving the already-created session un-stamped → name fallback. A stamp/generation failure never aborts session creation.
> Spec § Fix Overview → Its value / Generation contract: `@portal-id` is stamped with a freshly-generated random token (a `crypto/rand` nanoid, independent of the session name), frozen at creation. Fire-and-forget — no uniqueness check against existing `@portal-id` values. Correctness relies solely on width making birthday-collision negligible; the accepted residual of a (vanishingly unlikely) duplicate id is hook-key cross-talk between the two panes that share it.
> DECISION (token width): the spec calls token width an implementation detail ("the existing `NewNanoIDGenerator` scheme, widened if warranted"). This task reuses `sc.gen` at its current 6-char alphanumeric width (`suffixLen = 6`, 62-char alphabet ≈ 5.7×10^10 space). No widening in Phase 1 — the deliverable is the constant + best-effort stamp; the residual duplicate-id risk is explicitly accepted by the spec's fire-and-forget contract. Do not add a second generator or alter `suffixLen`.
> Constant placement: the spec says package placement is an implementation detail (must be importable by stamp/re-stamp sites). Placing `PortalIDOption` in `internal/session` (beside `PortalDirOption`) mirrors the existing precedent and is importable by the restore re-stamp site (Phase 3).

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Fix Overview: Stable Session Identity (`@portal-id`) → The option / Its value / Generation contract / Where it is stamped; § Cross-Reboot Persistence → Constant; § Testing Requirements → Creation & persistence (component); § Acceptance Criteria 1.

## session-rename-orphans-resume-hook-1-4 | approved

### Task session-rename-orphans-resume-hook-1-4: Stamp `@portal-id` in `QuickStart.Run` ExecArgs chain

**Problem**: `QuickStart.Run` is the second first-party creation path (`portal open` / `x` quick-start handoff). Unlike `CreateFromDir` it builds a single chained `syscall.Exec` tmux invocation with no Go-side seam to call `SetSessionOption` mid-flight and no error-return point inside the argv chain. Without an added stamp step, quick-started sessions are created without `@portal-id` and remain rename-vulnerable.

**Solution**: Generate the token in Go inside `Run` (via the injected id generator `qs.gen`) BEFORE assembling `ExecArgs`, then interpolate it as a literal argv element into an added `; set-option -t <name> @portal-id <token>` step in the chain — placed while the session is still detached, before the `attach-session` step blocks the chain. A token-generation failure omits the step entirely, leaving the session un-stamped (session still created — best-effort).

**Outcome**: Every quick-started session carries an immutable `@portal-id` stamped while detached before attach; a token-generation failure gracefully omits the step (session still created, un-stamped → name fallback), with no error seam introduced into the argv chain.

**Do**:
- In `QuickStart.Run` (`internal/session/quickstart.go`, ~lines 66-86), BEFORE assembling `execArgs` (~line 72): generate `token, genErr := qs.gen()`.
- Build the `@portal-id` step conditionally: only when `genErr == nil` (and, defensively, `token != ""`). When generated, append `; set-option -t <name> @portal-id <token>` as literal argv elements interpolated into the existing chain, placed AFTER the `@portal-dir` `set-option` step and BEFORE the `; attach-session -t <name>` step. Concretely, the chain becomes: `new-session -d ... [<cmd>] ; set-option -t <name> @portal-dir <dir> ; set-option -t <name> @portal-id <token> ; attach-session -t <name>`. Use `PortalIDOption` (the Task 1-3 constant) for the option name, and the literal `token` string as the value argv element.
- When `genErr != nil`, omit the `@portal-id` step entirely; assemble the chain exactly as today (with only the `@portal-dir` step). Do NOT return the generation error — there is no error seam inside the chain; a generation failure just drops the step (best-effort, mirroring `CreateFromDir`).
- Note: `qs.gen` is already invoked once inside `PrepareSession` for the session name; this task adds a SECOND `qs.gen` call for the stamp token — an independent token distinct from the name suffix (the id is name-independent per spec).
- Update the `Run` doc-comment (~lines 42-65) so its documented chain shape includes the `@portal-id` step and states the stamp is best-effort (omitted on generation failure), stamped-before-attach.
- Extend `internal/session/quickstart_test.go` (package `session_test`). The shared `wantExecArgs` helper (~lines 26-35) builds the expected chain with only `@portal-dir`; because `qs.gen` is now called twice (name + token) with a fixed generator returning the same value, update `wantExecArgs` to interpolate the `@portal-id` step (`set-option -t <name> @portal-id <token>`) between the `@portal-dir` and `attach-session` steps, using the token the generator yields on its stamp call. Add cases for token interpolation, ordering, and the generation-failure omission path.

**Acceptance Criteria**:
- [ ] On success, `Run`'s `ExecArgs` contain the contiguous subsequence `["set-option", "-t", <name>, "@portal-id", <token>]` where `<token>` is the value the injected generator returns for the stamp call.
- [ ] The `@portal-id` `set-option` step is ordered AFTER the `@portal-dir` `set-option` step and BEFORE `attach-session` (stamped while detached, before attach blocks the chain).
- [ ] The token is interpolated as a single literal argv element (not shell-escaped, not quoted) — the opaque alphanumeric token needs no escaping.
- [ ] Token-generation failure omits the step: when the generator errors on the stamp call, `ExecArgs` contain NO `@portal-id` `set-option` step, `Run` returns no error, and the rest of the chain (create → `@portal-dir` stamp → attach) is unchanged (session still created, un-stamped).
- [ ] No `new-session -A` is introduced (detached-create + stamp-before-attach ordering preserved, as guarded today).
- [ ] `go build -o portal .` succeeds; `go test ./internal/session/...` passes (existing QuickStart chain assertions updated for the new step).

**Tests** (`internal/session/quickstart_test.go`, package `session_test`, NO `t.Parallel()`):
- `"it interpolates the @portal-id token as a literal set-option step in the exec chain"` — assert the `set-option -t <name> @portal-id <token>` subsequence via `assertContainsSubseq`.
- `"it orders the @portal-id stamp before attach-session"` — assert the `@portal-id` `set-option` index < `attach-session` index (extend the existing ordering guard).
- `"it orders the @portal-id stamp after the @portal-dir stamp"` — assert relative ordering of the two `set-option` steps.
- `"it omits the @portal-id step when stamp-time token generation fails"` — inject a call-counting generator that succeeds for the name and errors for the stamp; assert no `@portal-id` step and no error, chain otherwise intact.
- `"it does not use new-session -A"` — assert `-A` absent (existing guard preserved).

**Edge Cases**:
- Token-generation failure at stamp time → `@portal-id` step omitted, session created un-stamped, name fallback (spec § Fix Overview → Where it is stamped → QuickStart branch: "A generation failure omits the `set-option` step ... consistent with stamping being best-effort").
- Stamp step ordered before `attach-session` — attach blocks the chain, so the stamp must precede it (same reason the `@portal-dir` step is placed before attach).
- Token interpolated as a literal argv element — no error seam exists inside the argv chain; the token is opaque alphanumeric and cannot contain a `;` separator or need escaping.
- `qs.gen` now called twice (name + token); a generator that errors on the FIRST call fails name generation and aborts `Run` before `ExecArgs` assembly (existing behaviour); a generator that errors only on the SECOND (stamp) call exercises the omission path.

**Context**:
> Spec § Fix Overview → Where it is stamped → `QuickStart.Run`: an additional `; set-option -t <name> @portal-id <token>` step in the chained detached-create → stamp → attach `ExecArgs`, alongside the existing `@portal-dir` step (stamped while detached, before `attach-session` blocks the chain). The `<token>` is generated in Go inside `Run` (via the injected id generator) BEFORE `ExecArgs` is assembled, then interpolated as a literal into the `set-option` step — there is no error seam inside the argv chain. A generation failure omits the `set-option` step (session still created, un-stamped → name fallback).
> Spec § Fix Overview → Generation contract: fire-and-forget is also what lets `QuickStart` stamp inside its argv chain, which has no Go-side collision-retry seam.
> The existing chain (`quickstart.go:72-79`) is: `new-session -d -s <name> -c <dir> [<cmd>] ; set-option -t <name> @portal-dir <dir> ; attach-session -t <name>`. The new step slots between the `@portal-dir` `set-option` and `attach-session`.
> Uses the same `PortalIDOption` constant added in Task 1-3.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Fix Overview: Stable Session Identity (`@portal-id`) → Where it is stamped → `QuickStart.Run`; § Fix Overview → Generation contract; § Testing Requirements → Creation & persistence (component); § Acceptance Criteria 1.

## session-rename-orphans-resume-hook-1-5 | approved

### Task session-rename-orphans-resume-hook-1-5: Retire the four stale `hooks.json`-ownership doc-comments; transfer stability invariant to `HookKey`/`HookKeyFormat`

**Problem**: Four in-source doc-comments in `internal/tmux/tmux.go` assert that a name-based formatter (`PaneTarget` / `PaneTargetExact` / `StructuralKeyFormat` / `ListAllPanes`) *is* the canonical `hooks.json` key/lookup and that its format must never change or it orphans `hooks.json`. After this fix those claims are false: the canonical hook-key formatter is now `HookKey` / `HookKeyFormat`. Leaving the stale comments in place would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes. The load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant must not disappear; it transfers to the new primitives.

**Solution**: Rewrite the four doc-comments so none of them claims `hooks.json` key/lookup ownership, while keeping each formatter's remaining valid purpose (name-based tmux `-t` targeting for `PaneTarget`/`PaneTargetExact`; non-hook structural use for `StructuralKeyFormat`/`ListAllPanes`). Confirm the transferred "stable across releases — changing it silently invalidates every `hooks.json` entry" invariant is documented on `HookKey` (Task 1-1) and `HookKeyFormat` (Task 1-2).

**Outcome**: No doc-comment in `internal/tmux/tmux.go` claims a name-based formatter owns the `hooks.json` key; the stability invariant lives on `HookKey`/`HookKeyFormat`; and the four formatters' behaviour and their remaining name-based-targeting / non-hook-structural documentation are intact.

**Do**:
- `PaneTarget` (`tmux.go` ~551-558): remove the "it doubles as the canonical hooks.json key" claim and the "changing it would silently invalidate every entry in hooks.json" clause. Keep the doc as the canonical name-based `session:window.pane` `-t` target formatter, and keep the guidance that callers issuing `-t` must use `PaneTargetExact`. Optionally cross-reference `HookKey`/`HookKeyFormat` as the now-separate hook-key concern.
- `PaneTargetExact` (`tmux.go` ~572-573): remove "PaneTarget (no prefix) remains the canonical hook-key formatter". Keep the exact-match `-t` targeting purpose and the "do not mix the two" prefix guidance, re-phrased so it no longer references hook lookups (it is about `-t` target resolution, not `hooks.json`).
- `StructuralKeyFormat` (`tmux.go` ~771-779): remove the "persisted hook entries in hooks.json" and "the hook lookup table all agree" claims. Keep it as the canonical structural-key format for live-pane enumeration and `@portal-skeleton-*` marker names (the cleanup paths — stale-marker cleanup and orphan-FIFO sweep — that legitimately still share it). The "drift here would desync the cleanup paths" invariant stays, minus the hook-lookup claim.
- `ListAllPanes` (`tmux.go` ~781-798, including its `ResolveStructuralKey` cross-reference): remove "used as the lookup key in hooks.json" and the "intersect the returned slice with persisted hook entries" framing. Keep the error-propagating contract and the structural-key enumeration purpose for non-hook structural use.
- Verify the transferred invariant is present on both new primitives: `HookKey` (Task 1-1) and `HookKeyFormat` (Task 1-2) must each carry the "format is stable across releases — changing it silently invalidates every `hooks.json` entry" statement. If Tasks 1-1/1-2 already added it, confirm; if not, add it here.
- Do NOT change any of the four functions' bodies or signatures — this is a documentation-only change (plus a verification of the new primitives' comments). Do NOT touch the out-of-scope name-based `StructuralKeyFormat` uses in `cmd/bootstrap/stale_marker_cleanup.go` or `cmd/state_daemon.go` (they remain valid non-hook structural consumers).

**Acceptance Criteria**:
- [ ] None of the `PaneTarget`, `PaneTargetExact`, `StructuralKeyFormat`, `ListAllPanes` doc-comments claim to be the canonical `hooks.json` key or lookup, and none asserts that changing *that* formatter invalidates `hooks.json`.
- [ ] `PaneTarget`/`PaneTargetExact` docs still describe name-based `-t` targeting and the exact-match prefix guidance.
- [ ] `StructuralKeyFormat`/`ListAllPanes` docs still describe structural-key enumeration for non-hook structural use (`@portal-skeleton-*` markers, cleanup-path agreement) and the error-propagating contract.
- [ ] The "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant is documented on both `HookKey` and `HookKeyFormat`.
- [ ] The four functions' bodies/signatures are unchanged (documentation-only diff).
- [ ] `cmd/bootstrap/stale_marker_cleanup.go` and `cmd/state_daemon.go` are untouched.
- [ ] `go build -o portal .` succeeds; full `go test ./...` is green (no behaviour change; any doc-referencing tests, if present, still pass).

**Tests**:
- No new test cases (documentation-only change; the task table lists edge cases as `none`). Verification is by build + full suite green (`go test ./...`) confirming no behaviour regressed, plus a manual/grep confirmation that the four comments no longer reference `hooks.json` key/lookup ownership and that the new primitives carry the invariant. If a doc-guard or grep-based test asserting comment content exists, update it in lockstep; otherwise none is added.

**Edge Cases**: none (documentation-only; per the Phase 1 task table).

**Context**:
> Spec § Hook-Key Derivation → Deliverable — retire the stale doc-comments: Four in-source doc-comments today assert that a name-based formatter *is* the canonical `hooks.json` key/lookup and that its format must never change or it orphans `hooks.json`. After the fix all four are false and must be updated:
> - `PaneTarget` (`tmux.go:551-558`) — "the hook-key format stays stable across releases … changing it would silently invalidate every entry in hooks.json".
> - `PaneTargetExact` (`tmux.go:572-573`) — "PaneTarget (no prefix) remains the canonical hook-key formatter".
> - `StructuralKeyFormat` (`tmux.go:771-779`) — "used as the lookup key in hooks.json … the hook lookup table all agree".
> - `ListAllPanes` (`tmux.go:781-798`, incl. its `ResolveStructuralKey` reference) — "the same format … used as the lookup key in hooks.json".
>
> The canonical hook-key formatter is now `HookKey` / `HookKeyFormat`, and the load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant TRANSFERS to those new primitives, it does not disappear. `PaneTarget`, `StructuralKeyFormat`, and `ListAllPanes` remain valid for name-based tmux targeting / non-hook structural use, but their comments must stop claiming `hooks.json` ownership. Leaving any of the four in place would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes.
> Spec § Scope & Non-Goals → Out of scope: `@portal-skeleton-*` markers (`SanitizePaneKey`) and `sessions.json` delta/merge matching legitimately still use structural keys — so `StructuralKeyFormat`/`ListAllPanes` retain their non-hook structural role.
> Line numbers above are approximate (from the spec's snapshot); locate each comment by the function name, not the line.

**Spec Reference**: `.workflows/session-rename-orphans-resume-hook/specification/session-rename-orphans-resume-hook/specification.md` § Hook-Key Derivation (the four stages) → Deliverable — retire the stale doc-comments; § Scope & Non-Goals → Out of scope.

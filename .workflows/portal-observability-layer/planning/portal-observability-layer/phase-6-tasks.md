---
phase: 6
phase_name: Hydrate-helper forensic trail
total: 4
---

## portal-observability-layer-6-1 | approved

### Task 6-1: Emit hook-lookup DEBUG breadcrumb and terminal `hydrate: exec` INFO in `execShellOrHookAndExit`

**Problem**: The hydrate helper exec's the user shell (or the on-resume hook chain) via `syscall.Exec`, replacing its own process image — so it can never observe the hook command's exit status. The motivating incident (a `hooks.json` wipe followed by a saver disappearance) was undiagnosable in part because the helper left no record of what it looked up or what it handed off to. Today `execShellOrHookAndExit` (`cmd/state_hydrate.go`) silently branches across nil-store / lookup-error / miss / hit and exec's with no breadcrumb, so `grep "hydrate:" portal.log` cannot reconstruct a per-pane recovery up to the exec moment. This is the architectural-limit instrumentation the spec accepts: instrument everything up to exec, post-exec is silent by design.

**Solution**: Instrument the `execShellOrHookAndExit` function path with two log lines from the spec's Hook-firing mechanical rule: (1) a DEBUG `hook lookup` breadcrumb carrying `hook_key` (the saved structural key `cfg.HookKey`) and `result` ∈ {`hit`, `miss`, `error`} (plus the `error` attr on the `error` case), emitted after the lookup but before exec; and (2) a terminal INFO `hydrate: exec` immediately before each `cfg.ExecShell(...)` handoff, carrying `target` (the exec'd binary), `args` (its argv), and `hook_present` (bool). The `hydrate: exec` line is structurally parallel with `process: exec` (Task 2-14): both `syscall.Exec` handoff markers use the shared exec-handoff attrs `target` + `args`, so `grep` on `target=`/`args=` gives a uniform "what did each process hand off to" view across both markers. `target` is deliberately NOT the `path` attr — `path` stays reserved for the genuine filesystem-path lines (`fifo missing`, `scrollback missing`).

**Outcome**: Every invocation of `execShellOrHookAndExit` emits one DEBUG `hydrate: hook lookup hook_key=<key> result=<hit|miss|error>` (with `error=<err>` on the error branch) then one INFO `hydrate: exec target=<bin> args=<argv> hook_present=<bool>` immediately before `cfg.ExecShell`; at production INFO the lookup DEBUG is filtered and only the exec INFO survives, so `grep "hydrate:" portal.log` reconstructs the exec handoff for every helper invocation; at DEBUG both lines appear.

**Do**:
- Pin the exact line range against the live `cmd/state_hydrate.go` (the spec's line numbers are HINTS): the target function is `execShellOrHookAndExit` (currently `cmd/state_hydrate.go:228-246`), with its three exec sites being `execShellAndExit` (the nil-store, lookup-error, and miss branches, which currently route through `cfg.ExecShell(shell, []string{shell})` at `execShellAndExit`, `cmd/state_hydrate.go:197-200`) and the hit branch's direct `cfg.ExecShell("/bin/sh", []string{"sh", "-c", chained})` at `cmd/state_hydrate.go:245`.
- By this phase `cfg.Logger` is a `*slog.Logger` bound to component `hydrate` via `log.For("hydrate")` (Phase 1 migration retyped the `hydrateConfig.Logger` field and the construction site in `stateHydrateCmd.RunE`). Do NOT reintroduce `*state.Logger` or `state.ComponentHydrate`. Use `cfg.Logger.Debug(...)` / `cfg.Logger.Info(...)` with slog attr pairs.
- **Lookup DEBUG breadcrumb** — emit after computing the lookup result and before any exec, mapping `result` from the `hooks.LookupOnResume(cfg.HookStore, cfg.HookKey)` return contract (`internal/hooks/lookup.go`):
  - `cfg.HookStore == nil` → `result="miss"` (the nil store degrades to bare `$SHELL` — it is a miss, NOT an error; the spec's `error` value is reserved for a lookup that *failed*). Emit the DEBUG with `result="miss"` and NO `error` attr, then proceed to the bare-shell exec.
  - `LookupOnResume` returns `err != nil` → `result="error"`; include the `error` attr passing the wrapped `err` directly (`"error", err`, not `err.Error()`) per Diagnostic context preservation (Phase 4). The existing WARN line is retained (see below); the DEBUG breadcrumb is additive.
  - `LookupOnResume` returns `("", false, nil)` (no hook, OR `hooks.json` missing/malformed — per the `LookupOnResume` contract these all collapse to "no hook") → `result="miss"`.
  - `LookupOnResume` returns `(cmd, true, nil)` → `result="hit"`.
  - Shape: `cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", lookupResult)` (append `"error", err` on the error branch only).
- **Terminal `hydrate: exec` INFO** — emit immediately before EACH `syscall.Exec` handoff, carrying the resolved `target`, `args`, and `hook_present`:
  - Bare-shell branches (nil store, lookup error, miss): `target = shell` (the resolved `$SHELL`, `/bin/sh` fallback — same `resolveShell()` value `execShellAndExit` passes), `args = []string{shell}`, `hook_present = false`.
  - Hit branch: `target = "/bin/sh"`, `args = []string{"sh", "-c", chained}` (where `chained = command + "; exec " + shell`), `hook_present = true`.
  - Shape: `cfg.Logger.Info("exec", "target", target, "args", strings.Join(args, " "), "hook_present", hookPresent)`. Render `args` as the space-joined argv string (matching the `process: exec` rendering in Task 2-14 — confirm the join shape against that task's rendered example; for the hit chain the joined string is `sh -c <command>; exec <shell>`). Pass `args` verbatim including any embedded quotes (privacy posture: portal's single-user threat model; the hook content is also recoverable from the prior `hooks:` state-mutation audit line, so the spec does not require redacting it here).
- Refactor so the exec INFO sits immediately before the actual `cfg.ExecShell(...)` call with no statement in between (the unbuffered-writer guarantee from Task 2-7 puts the marker in the kernel before the image is replaced — add a code comment noting the marker is pre-exec and the writer is unbuffered, mirroring Task 2-14). The cleanest structure: have `execShellAndExit` emit the bare-shell exec INFO (since all three bare-shell branches funnel through it), and add the hit-branch exec INFO inline before the `cfg.ExecShell("/bin/sh", ...)` call. Alternatively thread the exec INFO into a small helper both paths call. Ensure NO exec path reaches `cfg.ExecShell` without first emitting the INFO.
- Preserve all existing behaviour: the existing lookup-error WARN line (currently `cfg.Logger.Warn(state.ComponentHydrate, "lookup on-resume hook for %s: %v", ...)`, migrated by Phase 1 to a slog WARN such as `cfg.Logger.Warn("lookup on-resume hook", "hook_key", cfg.HookKey, "error", err)`) stays — the DEBUG breadcrumb is additive and does not replace it. The branch logic (nil store → bare shell; error → WARN + bare shell; miss → bare shell; hit → `sh -c` chain) is unchanged.
- `execShellAndExit` is also called directly by no other production path than `execShellOrHookAndExit` in this file — confirm by reading the file; if any future caller exists, ensure the exec INFO is correct for it too. (Currently `execShellAndExit` is invoked only from `execShellOrHookAndExit`.)

**Acceptance Criteria**:
- [ ] A nil `cfg.HookStore` emits DEBUG `hydrate: hook lookup hook_key=<key> result=miss` (no `error` attr) then INFO `hydrate: exec target=<shell> args=<shell> hook_present=false`, and exec's bare `$SHELL`.
- [ ] A `LookupOnResume` error emits DEBUG `result=error` WITH the `error` attr (wrapped `err`, not `.Error()`), retains the existing WARN, then emits the bare-shell exec INFO with `hook_present=false`, and degrades to bare `$SHELL`.
- [ ] A miss (`("", false, nil)` — no hook, or missing/malformed `hooks.json`) emits DEBUG `result=miss` then the bare-shell exec INFO with `hook_present=false`.
- [ ] A hit emits DEBUG `result=hit` then INFO `hydrate: exec target=/bin/sh args="sh -c <command>; exec <shell>" hook_present=true`, and exec's the `sh -c` chain.
- [ ] The `hydrate: exec` line uses the `target` attr key (NOT `path`) and is structurally parallel to `process: exec` (shared `target` + `args` attrs).
- [ ] `args` is rendered verbatim including any embedded quotes in the registered hook command.
- [ ] The exec INFO is emitted immediately before `cfg.ExecShell` with no intervening statement (marker in kernel pre-image-replace), for every exec path.
- [ ] No exec path reaches `cfg.ExecShell` without first emitting the `hydrate: exec` INFO.

**Tests**:
- `"it emits hook lookup result=miss and bare-shell exec hook_present=false for a nil HookStore"`
- `"it emits hook lookup result=error with the error attr and degrades to bare $SHELL"`
- `"it emits hook lookup result=miss for an unregistered pane key"`
- `"it emits hook lookup result=hit and exec target=/bin/sh args=\"sh -c ...\" hook_present=true for a registered hook"`
- `"it renders args verbatim including embedded quotes in the hook command"`
- `"the hydrate: exec line uses the target attr, not path"`
- `"it emits the exec INFO immediately before ExecShell for every branch"`

**Edge Cases**:
- nil `HookStore` → `result=miss` (NOT `error`); bare `$SHELL`, `hook_present=false`.
- Lookup error → `result=error` + `error` attr; degrades to bare `$SHELL`, `hook_present=false`; existing WARN retained.
- Missing/malformed `hooks.json` collapses to `result=miss` per the `LookupOnResume` contract (`("", false, nil)`), NOT `error`.
- `found=true` → `sh -c` chain, `target=/bin/sh`, `hook_present=true`.
- `found=false` → bare `$SHELL`, `hook_present=false`.
- `target` distinct from the reserved `path` attr.
- `args` rendered verbatim incl. embedded quotes.
- Unbuffered-writer guarantee: marker in kernel before exec replaces the image.
- Structurally parallel to `process: exec` (shared `target` + `args`).

**Context**:
> "**1. Hook lookup (DEBUG breadcrumb).** After the helper has computed the structural pane key and queried `hooks.json` for an on-resume hook, but BEFORE the exec call: `hookLogger.Debug("hook lookup", "hook_key", paneKey, "result", lookupResult)` … `lookupResult` is `"hit"` if a hook was registered, `"miss"` if no hook for that pane_key, `"error"` if the lookup itself failed (parse error, etc.). On `"error"`, also include the `"error"` attr per Diagnostic context preservation. This DEBUG line distinguishes 'hooks.json drifted from the saved hook-key' (miss) from 'lookup failed for some other reason' (error) from 'helper never reached the lookup' (no line at all)." (spec § Hook-firing observability limit → Mechanical rule 1)
>
> "**2. Exec terminal point (INFO).** Immediately before the `syscall.Exec` call: `hookLogger.Info("exec", "target", execPath, "args", argv, "hook_present", hookFound)` … It is **structurally parallel with `process: exec`** — both `syscall.Exec` handoff markers use `target` (the exec'd binary) + `args` (its argv) … When `hook_present=true`, the helper exec's `sh -c '<HOOK>; exec $SHELL'`; when `false`, it exec's `$SHELL` directly. The hook content itself is in the prior INFO line written by `hookStore` mutations … so it's reconstructible via grep history without redundant logging here." (spec § Hook-firing observability limit → Mechanical rule 2)
>
> "(`target` is deliberately distinct from `path`, which remains reserved for the helper's genuine filesystem-path lines — `fifo missing path=…`, `scrollback missing path=…`.)" (spec § Hook-firing observability limit → Mechanical rule 2)
>
> "`target` + `args` are the **shared exec-handoff attrs** used by both `process: exec` and `hydrate: exec`, so the two `syscall.Exec` markers are structurally parallel." (spec § Subsystem prefix taxonomy → Process attr group)
>
> `LookupOnResume` return contract (`internal/hooks/lookup.go`): `("", false, nil)` for no hook OR missing/malformed `hooks.json`; `("", false, err)` for a genuine I/O error (wrapped "load hooks"); `(cmd, true, nil)` for a registered non-empty command. By this phase `cfg.Logger` is a `*slog.Logger` bound to `hydrate` via `log.For` (Phase 1).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Hook-firing observability limit (Mechanical rule 1 & 2); § Subsystem prefix taxonomy (Hydrate attr group, Process attr group — shared `target`/`args`); current `cmd/state_hydrate.go` `execShellOrHookAndExit` / `execShellAndExit` / `resolveShell`; `internal/hooks/lookup.go` `LookupOnResume`

---

## portal-observability-layer-6-2 | approved

### Task 6-2: Emit the FIFO-timeout exit-path INFO `hydrate: signal timeout took=3s` before exec

**Problem**: When the hydrate helper opens its per-pane FIFO and the hydrate signal never arrives within the timeout, the helper gives up after 3 seconds and falls through to exec the shell. Today `handleHydrateTimeout` (`cmd/state_hydrate.go`) logs only a WARN naming the hook-key + FIFO; there is no INFO-level record of the timeout, so at production INFO `grep "hydrate:" portal.log` cannot tell a timed-out recovery apart from a normal one. The spec's failure-mode catalog requires a dedicated `signal timeout` INFO line carrying the timeout duration so the timeout exit path is reconstructible at the production baseline.

**Solution**: Emit one INFO `hydrate: signal timeout took=<duration>` on the timeout exit path, where `took` is the `hydrateTimeout` `time.Duration` constant (3s) — rendered by the handler's text mode as `took=3s` (a `time.Duration`, NOT a quoted string). The INFO is followed by the exec INFO (Task 6-1), giving two INFO lines on this failure-mode invocation: the exit-path INFO captures *what happened in the helper* (waited and timed out), the exec INFO captures *what we handed off to*. The existing WARN, the FIFO unlink, the marker-unset, and the 100ms settle sleep are all preserved unchanged.

**Outcome**: A FIFO-timeout recovery emits one INFO `hydrate: signal timeout took=3s` (followed by the `hydrate: exec` INFO from Task 6-1), in addition to the retained WARN; at production INFO the timeout is visible in `portal.log`; the timeout duration renders as `took=3s`, not a quoted string.

**Do**:
- Pin the exact line range against the live `cmd/state_hydrate.go` (spec hint: ~line 115/116). The timeout path is `runHydrate`'s `errors.Is(err, ErrHydrateTimeout)` branch (`cmd/state_hydrate.go:102-118`): when `cfg.HandleTimeout != nil`, it calls `cfg.HandleTimeout(cfg)`, then `time.Sleep(hydrateSettleSleep)`, then `execShellOrHookAndExit(cfg)`. The handler itself is `handleHydrateTimeout` (`cmd/state_hydrate.go:260-277`).
- Emit the INFO `cfg.Logger.Info("signal timeout", "took", hydrateTimeout)` on the timeout path. Note the spec writes the constant as `signalTimeout`; the live code's constant is `hydrateTimeout` (`cmd/state_hydrate.go:26`, `3 * time.Second`) — use the live name. Pass the `time.Duration` value directly (NOT a string) so the handler renders `took=3s` via Go's default `Duration.String()` (the text-mode rendering rule renders `time.Duration` with `String()`).
- Placement: emit the INFO once on the timeout exit path, BEFORE the `execShellOrHookAndExit(cfg)` exec INFO. The spec's failure-mode table reads "`hookLogger.Info("signal timeout", ...)` then exec" — the exit-path INFO precedes the exec handoff INFO. Choose the placement that fires the INFO exactly once on this path: emit it inside `handleHydrateTimeout` (alongside / replacing-position-of the existing WARN, keeping the WARN) is the natural site since that function owns the timeout-recovery sequence; alternatively emit it in `runHydrate`'s timeout branch just before the settle sleep. Either keeps the INFO before the exec INFO; pick the one that does not require threading new state and document the choice.
- Preserve ALL existing behaviour on the timeout path: the existing WARN (`handleHydrateTimeout` currently `cfg.Logger.Warn(state.ComponentHydrate, "timeout waiting for signal on --hook-key=%s --fifo=%s", ...)`, migrated by Phase 1 to a slog WARN), the `os.Remove(cfg.FIFO)` FIFO unlink, the reset-preamble write, the `unsetSkeletonMarkerOrLog(cfg)` marker-unset, and the 100ms `hydrateSettleSleep` in `runHydrate`'s branch are all unchanged. The INFO is additive.
- The nil-`HandleTimeout` fall-through (`runHydrate` returns `err` when `cfg.HandleTimeout == nil`, `cmd/state_hydrate.go:118`) is a test-only path that does not exec — it must NOT emit the `signal timeout` INFO (no exec happens; the helper returns the error). Only the `HandleTimeout != nil` production path that falls through to exec emits the INFO.
- By this phase `cfg.Logger` is `*slog.Logger` bound to `hydrate` via `log.For` (Phase 1). Do NOT reintroduce `*state.Logger` / `state.ComponentHydrate`.

**Acceptance Criteria**:
- [ ] A FIFO timeout (signal never arrives within `hydrateTimeout`) emits one INFO `hydrate: signal timeout took=3s`.
- [ ] `took` is the `hydrateTimeout` `time.Duration` value (passed as a `Duration`, rendering `took=3s` — NOT a quoted string).
- [ ] The `signal timeout` INFO precedes the `hydrate: exec` INFO (Task 6-1) on the timeout path.
- [ ] The existing timeout WARN, the FIFO unlink, the reset-preamble, the marker-unset, and the 100ms settle sleep are all unchanged.
- [ ] The nil-`HandleTimeout` fall-through (test-only, no exec) does NOT emit the `signal timeout` INFO.

**Tests**:
- `"it emits hydrate: signal timeout took=3s on the FIFO-timeout path"`
- `"the signal timeout took attr renders as a duration (took=3s), not a quoted string"`
- `"the signal timeout INFO precedes the hydrate: exec INFO"`
- `"it preserves the timeout WARN, FIFO unlink, marker-unset, and 100ms settle sleep"`
- `"it does not emit signal timeout when HandleTimeout is nil (no exec)"`

**Edge Cases**:
- `took` is the `hydrateTimeout` `time.Duration` (renders `took=3s`, not quoted).
- INFO precedes the exec INFO (two INFO lines on this failure-mode invocation).
- Handler marker-unset / FIFO-unlink / WARN unchanged.
- nil-`HandleTimeout` fall-through unaffected (no exec → no INFO).
- 100ms settle sleep posture preserved.

**Context**:
> "| Timeout — helper waited 3s, signal never arrived | ~line 115 | `hookLogger.Info("signal timeout", "took", signalTimeout)` then exec (where `signalTimeout` is the 3s `time.Duration` constant — renders `took=3s`, not a quoted string) |" (spec § Hook-firing observability limit → Mechanical rule 3, failure-mode table)
>
> "Each exit path's INFO is followed by the exec INFO (rule 2). Two INFO lines per invocation in the failure-mode cases … The repetition is intentional — the exit-path INFO captures *what happened in the helper*; the exec INFO captures *what we handed off to*." (spec § Hook-firing observability limit → Mechanical rule 3)
>
> "`time.Duration` values render with Go's default `String()` (e.g. `1.234s`)." (spec § Subsystem prefix taxonomy → text-mode rendering rule)
>
> Live code: the 3s constant is `hydrateTimeout` (`cmd/state_hydrate.go:26`), not the spec's illustrative `signalTimeout` name. The timeout-recovery sequence lives in `handleHydrateTimeout` + `runHydrate`'s `ErrHydrateTimeout` branch.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Hook-firing observability limit (Mechanical rule 3 — timeout row); § Subsystem prefix taxonomy (text-mode `time.Duration` rendering); current `cmd/state_hydrate.go` `handleHydrateTimeout` / `runHydrate` timeout branch / `hydrateTimeout` constant

---

## portal-observability-layer-6-3 | approved

### Task 6-3: Emit the file-missing exit-path INFO `hydrate: scrollback missing path=…` (and resolve the `fifo missing` row)

**Problem**: When the saved scrollback file cannot be served — `os.Open` fails (ENOENT, permission denied, generic I/O) or `io.Copy` fails mid-stream — the helper logs only a per-cause WARN in `handleHydrateFileMissing` and falls through to exec the shell. At production INFO `grep "hydrate:" portal.log` cannot tell a scrollback-missing recovery apart from a normal one. The spec's failure-mode catalog requires a dedicated `scrollback missing path=<file>` INFO. The same catalog ALSO lists a separate `fifo missing path=<fifoPath>` row as a DISTINCT exit path — but the live FIFO-open path (`openFIFOWithTimeout`, O_RDONLY blocking) has only one non-success outcome (`ErrHydrateTimeout` → the Task 6-2 timeout path); there is no current code path that yields a distinct "FIFO ENOENT" outcome separate from timeout. Whether `fifo missing` is a still-live exit path must be resolved against the live handler, not assumed.

**Solution**: Emit one INFO `hydrate: scrollback missing path=<cfg.File>` on the file-missing exit path (covering ENOENT, permission, generic I/O, and the mid-stream `io.Copy` failure — all of which currently route through `handleHydrateFileMissing`). The INFO is followed by the exec INFO (Task 6-1). For the `fifo missing` row: the executor MUST confirm whether the live FIFO-open implementation can produce a distinct FIFO-absence outcome separate from timeout, and either wire the `fifo missing path=<cfg.FIFO>` INFO at that site OR record that it collapses under the current implementation (flagged below).

**Outcome**: A scrollback-missing recovery (ENOENT / permission / generic I/O / mid-stream `io.Copy` failure) emits one INFO `hydrate: scrollback missing path=<file>` (followed by the `hydrate: exec` INFO), in addition to the retained per-cause WARNs; at production INFO the scrollback-missing exit is visible in `portal.log`; the `fifo missing` row's live status is resolved (wired if a distinct path exists, or documented as collapsed-under-timeout if not).

**Do**:
- Pin the exact line range against the live `cmd/state_hydrate.go` (spec hint: ~line 147 for scrollback-open, ~line 120 for FIFO). The file-missing exit path has TWO call sites that both invoke `cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err})` then `execShellOrHookAndExit(cfg)`: (a) `os.Open(cfg.File)` failure in `runHydrate` (`cmd/state_hydrate.go:138-149`), and (b) the mid-stream `io.Copy(cfg.Stdout, sb)` failure (`cmd/state_hydrate.go:158-167`). The handler is `handleHydrateFileMissing` (`cmd/state_hydrate.go:290-311`), which switches on `ctx.Cause` (`fs.ErrNotExist` / `fs.ErrPermission` / default) and WARNs per cause.
- Emit one INFO `cfg.Logger.Info("scrollback missing", "path", cfg.File)` on the file-missing exit path, covering ALL causes (ENOENT, permission, generic I/O, mid-stream `io.Copy` failure) — the spec catalogs a single `scrollback missing` row for the scrollback-open failure; the mid-stream `io.Copy` failure shares the same recovery (the existing code routes it through the same `handleHydrateFileMissing` handler), so it shares the same INFO. Use `cfg.File` as the `path` value (the genuine filesystem path — `path` is the reserved attr for this, distinct from `target`).
- Placement: emit the INFO once on the file-missing exit path, BEFORE the `execShellOrHookAndExit(cfg)` exec INFO. The natural site is inside `handleHydrateFileMissing` (it owns the file-missing recovery sequence and already branches per cause) so the single INFO fires regardless of which of the two call sites invoked it; this also guarantees it fires exactly once per file-missing recovery. Document the choice. Emit it AFTER (or alongside) the per-cause WARN — the INFO is additive and the WARNs are retained.
- Preserve ALL existing behaviour: the three per-cause WARNs in `handleHydrateFileMissing` (`scrollback file not found` / `unreadable (permission denied)` / `I/O error`, migrated by Phase 1 to slog WARNs), the deliberate NO-settle-sleep posture (the handler does not sleep — nothing was fully dumped), and the inline `unsetSkeletonMarkerOrLog(cfg)` marker-unset are all unchanged. The INFO is additive.
- **`[needs-info]` — resolve the `fifo missing` row against the live handler.** The spec's failure-mode table lists `fifo missing path=fifoPath` as a DISTINCT exit path from `scrollback missing`. But in the live code the FIFO is opened via `openFIFOWithTimeout` (`cmd/state_hydrate.go:72-88`, O_RDONLY blocking in a goroutine with a `time.After` timeout) whose only non-success return is `ErrHydrateTimeout` — which routes to the Task 6-2 timeout path (`hydrate: signal timeout`). Scrollback ENOENT routes through `handleHydrateFileMissing` (this task's `scrollback missing`). There is NO current code path that yields a distinct "FIFO ENOENT / no such file" outcome separate from timeout: a missing FIFO would make `os.OpenFile(path, os.O_RDONLY, 0)` return ENOENT immediately inside the goroutine, which the `select` would surface as the goroutine's `r.err` (a non-`ErrHydrateTimeout` error) — and `runHydrate`'s branch for a non-timeout open error (`cmd/state_hydrate.go:120`) is `return fmt.Errorf("open fifo %s: %w", cfg.FIFO, err)`, a hard return that does NOT exec and does NOT currently log an exit-path INFO. The executor MUST confirm: (1) is the `return fmt.Errorf("open fifo ...")` path a still-live exit path that should carry a `fifo missing path=<cfg.FIFO>` INFO before returning, given it does NOT fall through to exec (so it is NOT one of the "then exec" failure-mode rows)? OR (2) does `fifo missing` collapse entirely under the current FIFO-open implementation (the only observed non-success is timeout)? Do NOT invent a synthetic FIFO-absence branch to satisfy the spec row. If (1): the `fifo missing` INFO has no exec to precede (the path returns an error rather than exec'ing), so it would be a non-exec exit-path INFO — flag that this diverges from the spec's "then exec" framing and seek confirmation. If (2): record that `fifo missing` is not independently reachable and is subsumed by `signal timeout`. Carry this flag explicitly into implementation — it is a genuine spec/code mismatch, not an authoring gap.
- By this phase `cfg.Logger` is `*slog.Logger` bound to `hydrate` via `log.For` (Phase 1). Do NOT reintroduce `*state.Logger` / `state.ComponentHydrate`.

**Acceptance Criteria**:
- [ ] A scrollback `os.Open` ENOENT failure emits one INFO `hydrate: scrollback missing path=<cfg.File>`.
- [ ] A scrollback permission-denied failure emits the same single `scrollback missing` INFO (one INFO, regardless of cause).
- [ ] A generic-I/O `os.Open` failure emits the same single `scrollback missing` INFO.
- [ ] A mid-stream `io.Copy` failure emits the same single `scrollback missing` INFO (shared recovery).
- [ ] The `scrollback missing` INFO uses the reserved `path` attr with value `cfg.File` (NOT `target`), and precedes the `hydrate: exec` INFO.
- [ ] The three existing per-cause WARNs, the no-settle-sleep posture, and the marker-unset are all unchanged.
- [ ] The `fifo missing` row is resolved against the live handler: either wired at a confirmed distinct FIFO-absence site, or documented as collapsed-under-timeout — NOT invented as a synthetic branch.

**Tests**:
- `"it emits hydrate: scrollback missing path=<file> on a scrollback ENOENT"`
- `"it emits one scrollback missing INFO (not per-cause) for permission-denied"`
- `"it emits one scrollback missing INFO for a generic I/O open failure"`
- `"it emits scrollback missing for a mid-stream io.Copy failure"`
- `"the scrollback missing path attr is cfg.File and precedes the exec INFO"`
- `"it preserves the per-cause WARNs and the no-settle-sleep posture"`
- (Conditional on the `[needs-info]` resolution) `"it emits fifo missing path=<fifo> at the confirmed FIFO-absence site"` OR a documented absence-of-path note.

**Edge Cases**:
- Scrollback ENOENT / permission / generic-I/O all emit ONE `scrollback missing` INFO with `path`.
- Mid-stream `io.Copy` failure shares the `scrollback missing` INFO.
- `path` = `cfg.File` for scrollback vs `cfg.FIFO` for any FIFO-absence INFO.
- INFO precedes the exec INFO.
- Existing per-cause WARN lines retained; no-settle-sleep posture preserved.
- `[needs-info]`: `fifo missing` vs `scrollback missing` row mapping against the live handler — the live `openFIFOWithTimeout` yields only `ErrHydrateTimeout` as its non-success outcome, and the non-timeout open-error path hard-returns without exec. Confirm whether `fifo missing` is a still-live distinct exit path (and where) or collapses under timeout; do NOT synthesize a branch.

**Context**:
> "| Scrollback file missing | ~line 147 | `hookLogger.Info("scrollback missing", "path", scrollbackPath)` then exec |" and "| Silent ENOENT — helper opened FIFO and got 'no such file or directory' | ~line 120 | `hookLogger.Info("fifo missing", "path", fifoPath)` then exec |" (spec § Hook-firing observability limit → Mechanical rule 3, failure-mode table)
>
> "(`target` is deliberately distinct from `path`, which remains reserved for the helper's genuine filesystem-path lines — `fifo missing path=…`, `scrollback missing path=…`.)" (spec § Hook-firing observability limit → Mechanical rule 2)
>
> "(Line numbers are current-state hints; spec/plan phase pins exact ranges against the live file.)" (spec § Hook-firing observability limit → Mechanical rule 3)
>
> Live code: `handleHydrateFileMissing` (`cmd/state_hydrate.go:290`) handles `os.Open` failure AND mid-stream `io.Copy` failure (both call sites route through it). `openFIFOWithTimeout` (`cmd/state_hydrate.go:72`) returns only `ErrHydrateTimeout` as a non-success outcome; a non-timeout FIFO-open error hard-returns at `cmd/state_hydrate.go:120` (`return fmt.Errorf("open fifo %s: %w", ...)`) and does NOT exec — so the `fifo missing` "then exec" framing does not map cleanly onto the live FIFO-open path.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Hook-firing observability limit (Mechanical rule 3 — scrollback-missing & fifo-missing rows, Mechanical rule 2 — `path` reserved); current `cmd/state_hydrate.go` `handleHydrateFileMissing` / `runHydrate` `os.Open` + `io.Copy` branches / `openFIFOWithTimeout`

---

## portal-observability-layer-6-4 | approved

### Task 6-4: Emit the success exit-path INFO `hydrate: scrollback replayed bytes=N took=T` threading copied-byte count and replay duration

**Problem**: The hydrate helper's success path — signal arrived, scrollback dumped to stdout — is the most common and most operationally-interesting recovery, yet today it emits nothing before exec'ing the shell. At production INFO `grep "hydrate:" portal.log` cannot confirm a pane was successfully rehydrated, nor how much scrollback was replayed or how long the replay took. The spec's failure-mode catalog requires a `scrollback replayed bytes=N took=T` INFO, but the current code discards both values it needs: the `io.Copy` return (`n`) is thrown away (`if _, err := io.Copy(...)`) and no replay `start` time is captured.

**Solution**: Capture the two values the spec line requires — `n` (the byte count returned by `io.Copy(cfg.Stdout, sb)`, currently discarded) and a replay duration measured across the copy — and emit one INFO `hydrate: scrollback replayed bytes=<n> took=<duration>` on the success exit path, after the postamble write + 100ms settle sleep + marker-unset and before the `execShellOrHookAndExit(cfg)` exec INFO (Task 6-1). The INFO is followed by the exec INFO, giving (counting the lookup DEBUG, which is below the production INFO threshold) three lines on the success case: the lookup DEBUG, the `scrollback replayed` exit-path INFO, and the exec INFO.

**Outcome**: A successful rehydration emits one INFO `hydrate: scrollback replayed bytes=<n> took=<T>` (followed by the `hydrate: exec` INFO); `bytes` is the exact `io.Copy` byte count (0 for an empty scrollback file, the exact size for a populated one); `took` is the measured replay duration; at production INFO a successful per-pane recovery is reconstructible from `portal.log`.

**Do**:
- Pin the exact line range against the live `cmd/state_hydrate.go` (spec hint: ~line 188). The success path in `runHydrate`: `io.Copy(cfg.Stdout, sb)` (`cmd/state_hydrate.go:158`), then the postamble write (`cmd/state_hydrate.go:172`), then `time.Sleep(hydrateSettleSleep)` (`cmd/state_hydrate.go:177`), then `unsetSkeletonMarkerOrLog(cfg)` (`cmd/state_hydrate.go:182`), then `execShellOrHookAndExit(cfg)` (`cmd/state_hydrate.go:188`).
- Capture `n` from `io.Copy`: change `if _, err := io.Copy(cfg.Stdout, sb); err != nil {` to capture the byte count, e.g. `n, err := io.Copy(cfg.Stdout, sb)` then branch on `err` (preserving the existing mid-stream-failure handling which routes through `handleHydrateFileMissing` + the Task 6-3 `scrollback missing` INFO). `io.Copy` returns `(int64, error)`; `n` is the bytes written even on a partial/error copy, but the `scrollback replayed` INFO is emitted ONLY on the success branch (`err == nil`).
- Capture a replay `start` time: take `start := time.Now()` immediately before `io.Copy` and compute `took := time.Since(start)` for the INFO. Confirm the measurement window the spec intends ("scrollback dumped") is the `io.Copy` duration; measure across the copy (do NOT include the 100ms settle sleep in `took` — that is fixed overhead, not replay time). Document the chosen window.
- Emit `cfg.Logger.Info("scrollback replayed", "bytes", n, "took", took)` on the success path. Pass `n` as the `int64` value (`bytes` is the Hydrate-group attr for a byte count) and `took` as the `time.Duration` (renders e.g. `took=1.2s` via default `String()`). Placement: after the postamble + settle sleep + marker-unset, immediately before the exec INFO (so the exit-path INFO precedes the exec INFO per the spec's "then exec" framing). If `took` is measured as the copy duration, capture `n`/`took` at the copy site and carry them down to the emission point (a couple of locals threaded to just before `execShellOrHookAndExit`).
- Preserve ALL existing success-path behaviour: the reset preamble (already written before `os.Open`), the verbatim 32K-block streaming via `io.Copy`, the postamble + CRLF write, the 100ms `hydrateSettleSleep`, and the `unsetSkeletonMarkerOrLog(cfg)` marker-unset are all unchanged. The INFO is additive and sits after the marker-unset, before exec.
- Confirm the success path reaches `execShellOrHookAndExit` exactly once (`cmd/state_hydrate.go:188`) so the `scrollback replayed` INFO fires exactly once per successful invocation, and is not also emitted on the file-missing or timeout exit paths (those emit their own exit-path INFOs from Tasks 6-2 / 6-3).
- A zero-byte scrollback file (`bytes=0`) still emits the `scrollback replayed bytes=0` INFO — an empty replay is still a successful rehydration. A 5 MB file emits the exact byte count (`io.Copy` reports bytes verbatim; the existing 5MB-file streaming test verifies end-to-end copy correctness).
- By this phase `cfg.Logger` is `*slog.Logger` bound to `hydrate` via `log.For` (Phase 1). Do NOT reintroduce `*state.Logger` / `state.ComponentHydrate`.

**Acceptance Criteria**:
- [ ] A successful rehydration emits one INFO `hydrate: scrollback replayed bytes=<n> took=<T>` where `n` is the `io.Copy` return.
- [ ] `bytes` equals the exact `io.Copy` byte count (0 for an empty file, the file size for a populated one).
- [ ] `took` is the measured replay (copy) duration, rendered as a `time.Duration` (NOT the settle-sleep time, NOT a quoted string).
- [ ] A zero-byte scrollback file still emits `scrollback replayed bytes=0`.
- [ ] The `scrollback replayed` INFO is emitted after the postamble + 100ms settle sleep + marker-unset and before the `hydrate: exec` INFO.
- [ ] The success path reaches `execShellOrHookAndExit` exactly once; the `scrollback replayed` INFO does not fire on the timeout or file-missing paths.
- [ ] The reset preamble/postamble, verbatim streaming, settle sleep, and marker-unset are unchanged.

**Tests**:
- `"it emits hydrate: scrollback replayed bytes=N took=T on the success path"`
- `"bytes equals the io.Copy return for a populated scrollback file"`
- `"a zero-byte scrollback emits scrollback replayed bytes=0"`
- `"a 5MB scrollback reports the exact byte count"`
- `"took renders as a duration measured across the replay, not the settle sleep"`
- `"the scrollback replayed INFO precedes the exec INFO and fires once"`
- `"it does not emit scrollback replayed on the timeout or file-missing paths"`

**Edge Cases**:
- `bytes` = `n` captured from `io.Copy` (currently discarded — code change required).
- `took` measured across the replay (no `start` currently captured — code change required); excludes the settle sleep.
- Zero-byte scrollback → `bytes=0`, INFO still emitted.
- 5 MB file → exact byte count.
- INFO emitted after settle-sleep + marker-unset and before the exec INFO.
- Success path reaches `execShellOrHookAndExit` exactly once.

**Context**:
> "| Success — signal arrived, scrollback dumped | ~line 188 | `hookLogger.Info("scrollback replayed", "bytes", n, "took", took)` then exec |" (spec § Hook-firing observability limit → Mechanical rule 3, failure-mode table)
>
> "Each exit path's INFO is followed by the exec INFO (rule 2). Two INFO lines per invocation in the failure-mode cases, three in the success case (counting the lookup DEBUG, which is below INFO threshold in production)." (spec § Hook-firing observability limit → Mechanical rule 3)
>
> "`bytes` (set per hook-firing exec-chain event)" — the Hydrate attr group (spec § Subsystem prefix taxonomy → closed attr-key value space, Hydrate group). "`time.Duration` values render with Go's default `String()`." (text-mode rendering rule)
>
> Live code: the success path discards the `io.Copy` byte count (`if _, err := io.Copy(cfg.Stdout, sb); err != nil`, `cmd/state_hydrate.go:158`) and captures no replay `start` time. Both must be threaded to the emission point. The success path reaches `execShellOrHookAndExit` at `cmd/state_hydrate.go:188`.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Hook-firing observability limit (Mechanical rule 3 — success row); § Subsystem prefix taxonomy (Hydrate attr group — `bytes`, text-mode `time.Duration` rendering); current `cmd/state_hydrate.go` `runHydrate` success path (`io.Copy` / postamble / settle sleep / marker-unset / `execShellOrHookAndExit`)

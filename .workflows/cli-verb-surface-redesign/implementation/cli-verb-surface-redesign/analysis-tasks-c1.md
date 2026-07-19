---
topic: cli-verb-surface-redesign
cycle: 1
total_proposed: 8
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 1)

## Task 1: Repoint bootstrap warnings from the deleted `portal state status` to `portal doctor`
status: approved
severity: high
sources: standards

**Problem**: Two live, user-facing soft-bootstrap warnings in `cmd/bootstrap/errors.go` still instruct users to run/check `portal state status`, a command the redesign deleted and folded into `portal doctor` (spec § Command Surface Summary → Removed public commands; § state Namespace — Fully Hidden). `CorruptSessionsJSONWarning` (line ~57) emits "Check `portal state status` or ~/.config/portal/state/portal.log." and `SaverDownWarning` (line ~67) emits "Run `portal state status` for details." These are the exact remediation surfaces shown when sessions.json is corrupt or the save daemon fails to start (both drained to stderr / the TUI notice band from PersistentPreRunE; root.go references SaverDownWarning explicitly). `state` now has no `status` child, so `portal state status` resolves to cobra's "unknown command" error — the redesign left a user-facing instruction pointing at a command it just deleted.
**Solution**: Repoint both warning lines at the successor surface, `portal doctor`, so the remediation guidance names a command that still exists.
**Outcome**: A user hitting a corrupt sessions.json or a failed save daemon is directed to `portal doctor`, a live command; no user-facing string references a removed command.
**Do**:
1. Open `cmd/bootstrap/errors.go`.
2. In `CorruptSessionsJSONWarning` (line ~57), replace "Check `portal state status` or ~/.config/portal/state/portal.log." with "Check `portal doctor` or ~/.config/portal/state/portal.log."
3. In `SaverDownWarning` (line ~67), replace "Run `portal state status` for details." with "Run `portal doctor` for details."
4. Grep the tree for any remaining user-facing `portal state status` remediation strings and confirm none remain in warning output. (Comment-only references are covered by Task 7 — do not touch those here.)
**Acceptance Criteria**:
- Neither `CorruptSessionsJSONWarning` nor `SaverDownWarning` references `portal state status`.
- Both reference `portal doctor`.
- No live user-facing string instructs the user to run a removed command.
**Tests**:
- Unit assertion on the two warning constructors' rendered text: contains "portal doctor", does not contain "portal state status".

## Task 2: Extract a single domain-pin dispatch helper for the four copy-paste arms in openCmd.RunE
status: approved
severity: high
sources: duplication

**Problem**: `openCmd.RunE` contains four structurally identical ~11-line single-pin dispatch arms — `cmd/open.go:222-233` (-s/session), `246-257` (-p/path), `271-282` (-a/alias), `295-306` (-z/zoxide). Each does the same sequence: `cmd.Flags().Changed(<flag>)` → `GetString(<flag>)` → `buildQueryResolver(cmd)` (return err) → `qr.Resolve<Domain>Pin(val)` (return err) → `return openResolved(cmd, result, command)`. Only the flag-name string and the `Resolve*Pin` method vary. This is ~44 lines of copy-paste that must stay in lockstep (each pin arrived in a separate Phase-2 task) — a future change to the pin dispatch contract (e.g. threading a new arg into `openResolved`, a new error-wrapping step) has to be applied four times. Highest-density repetition in the feature.
**Solution**: Extract a single helper `resolvePinAndOpen(cmd *cobra.Command, flag string, resolve func(*resolver.QueryResolver, string) (resolver.QueryResult, error), command []string) error` and drive the four arms from a small ordered table of `{flag, resolveFn}` pairs (or four one-line calls). Each `Resolve*Pin` already has the uniform `(string) (QueryResult, error)` signature, so the closures are trivial.
**Outcome**: ~44 lines collapse to a ~10-line helper plus four call sites; the pin dispatch contract lives in one place and the four-way drift surface is removed. Adding a future pin touches one place (the table), not four.
**Do**:
1. In `cmd/open.go`, add `resolvePinAndOpen` that reads the flag value via `cmd.Flags().GetString(flag)`, builds the resolver via `buildQueryResolver(cmd)` (propagating its error), calls the passed `resolve` closure with `(qr, val)` (propagating its error), and on success returns `openResolved(cmd, result, command)`.
2. Replace the four inline arms (open.go:222-233, 246-257, 271-282, 295-306) with a dispatch table/loop keyed on `cmd.Flags().Changed(flag)` that calls `resolvePinAndOpen` with the matching closure (`ResolveSessionPin` / `ResolvePathPin` / `ResolveAliasPin` / `ResolveZoxidePin`).
3. Preserve exact behaviour: same error wrapping, same short-circuit on the first changed pin flag, same handoff into `openResolved`.
**Acceptance Criteria**:
- The four inline dispatch blocks are gone; a single helper plus one dispatch table/four call sites remain.
- Behaviour is byte-identical for each of -s/-p/-a/-z: same resolution, same error propagation, same `openResolved` handoff.
- Adding a future pin requires editing one place (the table), not four.
**Tests**:
- Existing per-pin open dispatch tests (session/path/alias/zoxide) pass unchanged.
- Verify each pin flag routes to its corresponding `Resolve*Pin` and then `openResolved`.

## Task 3: Collapse the dual source of truth for open's value-taking flags in the ordered-target argv scan
status: approved
severity: medium
sources: architecture

**Problem**: `orderedOpenTargets` (`cmd/open_targets.go:20-28, 56-83`) recovers left-to-right target order from the raw argv (cobra collapses it) by classifying each flag token against a hand-maintained static map `openTargetPins` that must exhaustively enumerate every value-taking open flag in both short and long form. That map is structurally decoupled from the real cobra flag set registered in `openCmd`'s init() (`cmd/open.go:982-990`): `orderedOpenTargets` is handed only `[]string`, never the `*cobra.Command`, so it cannot consult the real flag set. Its fallback for an unrecognised flag token is "skip WITHOUT consuming a following value" (open_targets.go:64-70, comment at 16-19) — an assumption the flag is boolean. A future value-taking flag added to `openCmd` but not mirrored into `openTargetPins` silently breaks this: cobra parses `open --newflag val ~/code` correctly, but the raw scan treats `--newflag` as arity-zero and classifies its VALUE ("val") as a bare positional target, running it through the guessing chain and even flipping a single-target invocation into a multi-target burst. Nothing keeps the two representations in sync.
**Solution**: Collapse the dual source of truth. Preferred: thread the `*cobra.Command` (or `*pflag.FlagSet`) into `orderedOpenTargets` and derive each token's arity from `cmd.Flags().Lookup` / `ShorthandLookup` (a pflag takes a value unless it is a bool or has a non-empty `NoOptDefVal`), so the scan and cobra classify by one flag set. Alternative, if threading the command is undesirable: keep `openTargetPins` but add a guard test that `VisitAll`s `openCmd`'s flags and asserts every non-bool flag's `-short` and `--long` form is present in `openTargetPins` (mirroring `keymap_dispatch_guard_test.go`).
**Outcome**: The argv scan derives flag arity from the same flag set cobra uses (or is protected by a drift guard), so a future value-taking flag can't have its value misrouted into a target.
**Do**:
1. Choose one path:
   - (a) Change `orderedOpenTargets`' signature to accept the `*cobra.Command` (or `*pflag.FlagSet`) and, per token, look it up (`ShorthandLookup` for `-x`, `Lookup` for `--long`) to decide arity: consume a following value unless the flag is a bool or has a non-empty `NoOptDefVal`. Retire reliance on the static `openTargetPins` arity map, and update the fallback comment (open_targets.go:16-19, 64-70) to reflect single-source derivation.
   - (b) Keep `openTargetPins` and add a guard test walking `openCmd.Flags().VisitAll`, asserting for every non-bool flag that both its shorthand and long form appear in `openTargetPins`.
2. Preserve the existing left-to-right multi-target ordering behaviour for all current flags.
**Acceptance Criteria**:
- There is a single source of truth for open's value-taking flags: the scan consults cobra's flag set directly, or a guard test fails if `openTargetPins` omits any non-bool `openCmd` flag.
- A hypothetical new value-taking flag on `openCmd` cannot have its value classified as a bare positional target (covered by test).
- Existing left-to-right multi-target ordering is unchanged.
**Tests**:
- Path (a): a test exercising a value-taking flag confirms its value is consumed, not treated as a target.
- Path (b): the drift guard fails when a non-bool flag is absent from `openTargetPins` and passes for the current flag set.
- Existing `orderedOpenTargets` ordering tests pass unchanged.

## Task 4: Single-source the single-target "nothing resolved" miss error string
status: approved
severity: medium
sources: duplication

**Problem**: The user-facing single-target miss message `fmt.Errorf("nothing resolved for '%s' — try -f %s", …)` (U+2014 em-dash) is authored verbatim in two independent sites: the bare-positional path in `openCmd.RunE` (`cmd/open.go:344`) and the N=1 glob-expanding-to-zero arm of `dispatchOpenBurst` (`cmd/open_burst.go:125`), confirmed byte-identical. The wording plus the `-f` escape-hatch suffix are a spec-governed contract (spec § Miss handling). The sibling multi-target message is already single-sourced as `aggregatedMissError` (cmd/open_burst.go:91-93), and other user-facing strings are single-sourced (`unknownAliasError`, `killSaverInfoMessage`) — this single-target message is the odd one out, inviting the exact drift the codebase otherwise guards against.
**Solution**: Add a `singleMissError(target string) error` helper co-located with `aggregatedMissError` in `cmd/open_burst.go`, returning the one canonical format string; call it from both sites. Mirrors the existing single-sourcing of the multi-target variant.
**Outcome**: The single-target miss message has one authoritative source matching the multi-target variant; the two sites can't drift.
**Do**:
1. In `cmd/open_burst.go` (next to `aggregatedMissError`), add `singleMissError(target string) error` returning the exact current `fmt.Errorf` format string — preserve the U+2014 em-dash and the `-f %s` suffix and whatever second format argument the current sites pass.
2. Replace the inline `fmt.Errorf` at `cmd/open.go:344` with a call to `singleMissError`.
3. Replace the inline `fmt.Errorf` at `cmd/open_burst.go:125` with a call to `singleMissError`.
**Acceptance Criteria**:
- Exactly one literal of the single-target miss format string exists (inside `singleMissError`).
- Both former sites call `singleMissError`.
- The produced message is byte-identical to the current output (em-dash preserved).
**Tests**:
- Assert the bare-positional miss path and the N=1 glob-to-zero burst path produce the identical expected string.

## Task 5: Extract expandSessionGlobAll to collapse the duplicated session-glob expansion block
status: approved
severity: medium
sources: duplication

**Problem**: The "expand a glob against a name set into K SessionResults" block is near byte-identical in `ResolveBareAll` (`internal/resolver/query.go:196-207`) and `ResolveSessionPinAll` (`query.go:226-236`): `MatchSessions(pattern, names)` → zero matches returns `[]QueryResult{&MissResult{Target: pattern}}` → else build K `&SessionResult{Name: m, Domain: "glob"}`. `MatchSessions` is already shared but the K-result wrapper around it is not, so the glob-to-SessionResult reduction lives in ≥2 identical copies — a drift risk if the glob `Domain` tag or zero-match handling ever changes. (The alias-domain expansion in `ResolveAliasPinAll` is a genuinely different body — per-key dir validation — and is out of scope for this extraction.)
**Solution**: Extract `expandSessionGlobAll(pattern string, names []string) []QueryResult` (zero matches → single `MissResult{Target: pattern}`; else K `SessionResult{Domain: "glob"}`) consumed by both `ResolveBareAll` and `ResolveSessionPinAll`.
**Outcome**: The session-domain glob expansion lives in one function; the two copies collapse to one call each; a future `Domain`-tag or zero-match change happens in one place.
**Do**:
1. In `internal/resolver/query.go`, add `expandSessionGlobAll(pattern string, names []string) []QueryResult`: `matches := MatchSessions(pattern, names)`; if `len(matches) == 0` return `[]QueryResult{&MissResult{Target: pattern}}`; else build and return K `&SessionResult{Name: m, Domain: "glob"}`.
2. Replace the inline block in `ResolveBareAll` (196-207) with a call.
3. Replace the inline block in `ResolveSessionPinAll` (226-236) with a call.
4. Do NOT touch `ResolveAliasPinAll` (different validated-path body) — leave as-is.
**Acceptance Criteria**:
- `ResolveBareAll` and `ResolveSessionPinAll` both call `expandSessionGlobAll`; no duplicated inline expansion remains.
- Zero-match returns a single `MissResult{Target: pattern}`; non-zero returns K `SessionResult{Domain: "glob"}` — behaviour unchanged.
- `ResolveAliasPinAll` is untouched.
**Tests**:
- Existing resolver tests for `ResolveBareAll` / `ResolveSessionPinAll` pass unchanged (zero-match miss, multi-match glob results, `Domain` tag).

## Task 6: Update CLAUDE.md command-surface prose to the redesigned surface
status: approved
severity: medium
sources: standards

**Problem**: During the impl commits CLAUDE.md received only a one-line edit (adding the `resolve` log component); the rest still documents the pre-redesign surface and omits the new one. Stale prose describes `portal spawn` / `cmd/spawn.go` / `spawn --detect` and `portal attach` / `--spawn-ack` (line ~37, plus the whole "Multi-window spawn (`portal spawn` + multi-select)" section at ~176-182); `portal clean` / `clean --logs` as the manual backstop (lines ~88, ~128, ~157) and `cmd/clean.go` as the home of `loadPrefsStore` (line ~80 — that file is deleted); and the bootstrap-exempt set "all except version, init, help, alias, clean" (line ~115 — `clean` is deleted). There is zero mention of `portal doctor`, `portal uninstall`, the hooks→hook rename, the `open` multi-target absorb/net-N surface, or the hidden `--ack` flag. CLAUDE.md is injected as project instructions on every session (human and agent), so it now actively misdescribes the command surface the redesign was an intentional one-pass audit of.
**Solution**: Update CLAUDE.md's command-surface prose to the redesigned surface.
**Outcome**: CLAUDE.md accurately describes the post-redesign command surface for humans and agents; no stale removed-command references remain.
**Do**:
1. Replace the Spawn/Attach command paragraph (line ~37) and the "Multi-window spawn (`portal spawn` + multi-select)" section (lines ~176-182) with the `open` multi-target burst + hidden `--ack` description (spawned windows now run the open burst via `cmd/open_burst*`, not a `portal spawn` CLI).
2. Document `portal doctor [--fix]` and `portal uninstall`.
3. Rename hooks→hook references and note the silent `hooks` alias.
4. Correct the bootstrap-exempt list (line ~115): drop `clean`; the real exempt set is doctor/uninstall/state/hook/alias/init/help/version.
5. Fix the `loadPrefsStore` home reference (line ~80) — `cmd/clean.go` is deleted; point at its current location.
6. Remove/repoint remaining `portal clean` / `clean --logs` manual-backstop references (lines ~88, ~128, ~157) to the `doctor --fix` log sweep / current owner.
**Acceptance Criteria**:
- CLAUDE.md no longer references removed surfaces (`portal spawn`, `spawn --detect`, `portal attach --spawn-ack`, `portal clean` / `clean --logs`, `cmd/spawn.go`, `cmd/clean.go` as the `loadPrefsStore` home).
- CLAUDE.md documents `portal doctor [--fix]`, `portal uninstall`, the `open` multi-target/net-N burst + hidden `--ack`, and the hooks→hook rename (with silent `hooks` alias).
- The bootstrap-exempt list matches the real set (no `clean`; includes doctor/uninstall).
**Tests**:
- Documentation change — no automated test. Reviewer greps CLAUDE.md for `portal spawn`, `portal clean`, `state status`, `--spawn-ack`, `spawn --detect` and confirms no stale live-surface hits remain, and verifies the new-surface prose against the current command set.

## Task 7: Sweep stale removed-surface references in code comments and the process-role doc
status: approved
severity: low
sources: standards, architecture

**Problem**: Numerous in-code comments and one process-role doc cross-reference removed surfaces and the deleted `cmd/spawn.go`. Comments cite "DIVERGENCE FROM runSpawn (cmd/spawn.go)", "runSpawn's permission arm", "the spawn CLI (buildSpawnDeps)", "`portal spawn` burst", and removed commands (`portal attach --spawn-ack`, `spawn --detect`, `portal clean --logs`, `portal state status`). Roughly fifteen doc comments across `internal/spawn`, `internal/tui` and `cmd` cite `runSpawn` / `cmd/spawn.go` / `buildSpawnDeps` / `SpawnDeps` as the live "single source, cannot drift" anchor — now false and pointing at a nonexistent file (the CLI caller is now the `open` burst in `cmd/open_burst_run.go`). `internal/log/process_role.go`'s doc (lines 13, 33-35, 69-70) lists a `clean` row and `hooks` spelling for removed/renamed surfaces, and its `case "clean"` arm is now dead. Architecture also notes the substantive effect: `spawn.SplitNetN` / `spawn.AttachSurfaces` are now single-caller (picker only) and `spawn.SplitTriggerFirst` is open-burst only, so the "shared so it cannot drift" justification for the SplitNetN half has evaporated. Comment/doc-only impact, but the anchors misdirect a maintainer to a deleted file.
**Solution**: Sweep these comments/docs to name the surviving callers/surfaces and drop dead file/command names; re-point `internal/spawn`'s co-caller anchors at the two surviving bursts (picker + open burst); annotate now-single-caller split helpers with their sole consumer. Scope is comments/docs ONLY — not the user-facing bootstrap warning strings (Task 1) and not CLAUDE.md (Task 6).
**Outcome**: Comments and the process-role doc describe surviving surfaces; the "single source, cannot drift" claims in `internal/spawn` are truthful; no comment misdirects a reader to a deleted file/command; the deliberate leading-vs-trailing trigger split is legible.
**Do**:
1. Sweep the comments flagged by the standards + architecture agents — `cmd/open_burst_run.go:194,202,209`; `cmd/doctor.go:372`; `cmd/open.go:643,886`; `internal/spawn/split.go:7,28`; `internal/spawn/message.go:11`; `internal/spawn/classify.go:6`; `internal/spawn/resolver.go:56`; `internal/spawn/adapter.go:15`; `internal/spawn/ack.go:38`; `internal/spawn/doc.go:9`; `internal/spawn/identity.go:12`; `internal/tui/burst_progress.go:250,405,426,484-488`; `internal/tui/burst_partial_failure.go:38`; `internal/log/retention.go` (multiple); `internal/state/status.go:20` — replacing `runSpawn` / `cmd/spawn.go` / `buildSpawnDeps` / `SpawnDeps` references with the surviving callers (the `open` burst in `cmd/open_burst_run.go` and/or the picker burst in `internal/tui`), and repointing removed-command names (`portal state status` → `portal doctor`; `portal clean --logs` → the `doctor --fix` log sweep; drop `spawn --detect` / `portal attach --spawn-ack`).
2. Update `internal/spawn`'s "shared so cannot drift" anchors to cite the two surviving consumers (picker burst + open burst); where a helper is now single-caller (`SplitNetN` / `AttachSurfaces` — picker only; `SplitTriggerFirst` — open burst only), annotate the sole consumer so the leading-vs-trailing trigger split reads as deliberate rather than justified by a deleted file.
3. Update `internal/log/process_role.go`'s doc comment (lines 13, 33-35, 69-70): note the `clean` role is now unreachable (dead `case "clean"` arm) and that `hook`/`hooks` both map to `hooks_cli`. Leave the closed-space `process_role` value in place — removing it requires a log-taxonomy amendment.
4. Do NOT modify the user-facing warning strings in `cmd/bootstrap/errors.go` (Task 1) or CLAUDE.md (Task 6).
**Acceptance Criteria**:
- No code comment references `cmd/spawn.go`, `runSpawn`, `buildSpawnDeps`, or `SpawnDeps` as a live surface.
- `internal/spawn` drift-prevention comments name the two surviving bursts (picker + open); single-caller helpers annotate their sole consumer.
- `process_role.go`'s doc describes the redesigned surface (dead `clean` arm noted; hook/hooks → `hooks_cli`); the closed-space value is left in place.
- No comment presents `portal state status` / `portal clean --logs` / `spawn --detect` / `portal attach --spawn-ack` as a live surface.
**Tests**:
- Comment/doc-only change — no automated test. Reviewer greps the internal tree for `cmd/spawn.go`, `runSpawn`, `buildSpawnDeps`, `SpawnDeps`, `spawn --detect`, `--spawn-ack`, `state status`, `clean --logs` and confirms no live-surface references remain.

## Task 8: Single-source the two governed two-site emissions (resolve-decision log line + exec-handoff marker)
status: approved
severity: low
sources: duplication

**Problem**: Two spec-governed emission contracts are each authored verbatim in two independent sites, so either pair can drift the attr set / gate / marker apart. (a) The `resolve` component INFO line — `resolveLogger.Info("resolved", "target", …, "domain", …, "resolved_path", …)` gated by `!resolver.HasGlobMeta(query)` — is written in both the single-target bare path (`cmd/open.go:333-336`) and `resolveOpenSurfaces`' bare arm (`cmd/open_surfaces.go:64-68`); this is a locked observability contract (attr keys target/domain/resolved_path, one line per guessing-chain target). (b) The `process:exec` handoff marker — drop argv[0], then `log.For("process").Info("exec", "target", "tmux", "args", …)` immediately before `syscall.Exec` — is hand-rolled in both `AttachConnector.Connect` (`cmd/open.go:121-134`) and `PathOpener.Open` (`cmd/open.go:551-564`), a load-bearing forensic tripwire that must read identically on both paths.
**Solution**: Single-source each governed emission behind a small helper: (a) `emitResolveDecision(target string, result resolver.QueryResult)` co-located with `resolveDecision` in `cmd/open.go`, applying the `!HasGlobMeta` gate and emitting the canonical `resolveLogger.Info` line; (b) `logExecHandoff(argv []string)` that strips argv[0] and emits the canonical `process:exec` line. Call each from both of its sites.
**Outcome**: Each governed contract has one authoritative emission site; the resolve log line and the exec marker read identically across their two callers.
**Do**:
1. Add `emitResolveDecision(target string, result resolver.QueryResult)` in `cmd/open.go` next to `resolveDecision`: apply the `!resolver.HasGlobMeta(target)` gate, compute `domain, resolvedPath := resolveDecision(result)`, and emit `resolveLogger.Info("resolved", "target", target, "domain", domain, "resolved_path", resolvedPath)`.
2. Replace the inline emission in the single-target bare RunE path (open.go:333-336) and in `resolveOpenSurfaces`' bare arm (open_surfaces.go:64-68) with `emitResolveDecision` calls.
3. Add `logExecHandoff(argv []string)` that drops argv[0] and emits `log.For("process").Info("exec", "target", "tmux", "args", strings.Join(argv[1:], " "))`, keeping the "unbuffered writer, no Sync needed" reasoning in one place.
4. Replace the hand-rolled marker in `AttachConnector.Connect` (open.go:121-134) and `PathOpener.Open` (open.go:551-564) with `logExecHandoff` calls immediately before `syscall.Exec`.
**Acceptance Criteria**:
- The resolve-decision INFO line is emitted from exactly one helper (`emitResolveDecision`), called by both bare paths; attr keys and the `!HasGlobMeta` gate are single-sourced.
- The `process:exec` marker is emitted from exactly one helper (`logExecHandoff`), called by both exec paths.
- Emitted log output is byte-identical to current for both contracts.
**Tests**:
- Assert the `resolved` line (attr keys target/domain/resolved_path) is emitted once for a non-glob bare target on both the single-target and surfaces paths.
- Assert the `process:exec` marker (target=tmux, args=argv[1:]) is emitted on both the `AttachConnector.Connect` and `PathOpener.Open` exec paths.

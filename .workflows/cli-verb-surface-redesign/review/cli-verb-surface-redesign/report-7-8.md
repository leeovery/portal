TASK: cli-verb-surface-redesign-7-8 — Single-source the two governed two-site emissions (resolve-decision log line + exec-handoff marker)

ACCEPTANCE CRITERIA:
- Two governed contracts (`resolve` INFO line + `process:exec` marker) each emitted from exactly one helper
- The `!HasGlobMeta` gate and attr keys are single-sourced
- Emitted log output is byte-identical to current
- Both call sites route through the helper

STATUS: Complete

SPEC CONTEXT:
The `resolve` component (spec § "Wrong-guess feedback — tmux is the receipt") is a spec-governed amendment to the closed log taxonomy: one INFO line per bare positional resolved through the guessing chain, message `resolved`, attr keys `target` / `domain` (session/path/alias/zoxide, or `miss`) / `resolved_path` (resolved dir, or session name for a session hit, empty on miss). Glob (and pinned) targets are deterministic and emit no line — the emission is gated on the non-glob predicate. The `process:exec` marker (spec § "Defensive invariants — exec-handoff markers") is the terminal forensic tripwire emitted immediately before `syscall.Exec` (which replaces the process image, so no `process:exit` fires) — it must read byte-identically across both bare-shell exec paths. This is an analysis-cycle-1 refactor task: consolidate two previously-inlined two-site emissions into one helper each without changing emitted output.

IMPLEMENTATION:
- Status: Implemented (byte-identical consolidation verified against the diff)
- Location:
  - Contract 1 (resolve INFO line): helper `emitResolveDecision` — cmd/open.go:412-418 (owns the `HasGlobMeta` early-return gate at :413 and the single `resolveLogger.Info("resolved", ...)` emission at :417; derives attrs via the reused `resolveDecision` at :426). Call site A: single-target path cmd/open.go:295. Call site B: burst path cmd/open_surfaces.go:67 ("bare" domain arm).
  - Contract 2 (process:exec marker): helper `logExecHandoff` — cmd/open.go:454-460 (single `log.For("process").Info("exec", "target", "tmux", "args", ...)` emission at :459, defensive argv[0] strip). Call site A: `AttachConnector.Connect` cmd/open.go:129. Call site B: `PathOpener.Open` cmd/open.go:574.
- Notes:
  - Single-source verified by grep: `resolveLogger.Info("resolved"` appears only in `emitResolveDecision`; `log.For("process").Info("exec"` appears only in `logExecHandoff`. No inline call site bypasses either helper.
  - The `!HasGlobMeta` gate lives INSIDE `emitResolveDecision` (as an `if HasGlobMeta { return }` early-return, logically identical to the former `if !HasGlobMeta { … }` guards), so both call sites gate identically and cannot drift.
  - Attr keys are single-sourced in the one `Info` call — order and values (`target`/`domain`/`resolved_path`) identical to both former inline sites.
  - The two `exec` emissions in cmd/state_hydrate.go:260,323 are NOT a missed third call site: they use `cfg.Logger` (defaults to `log.For("hydrate")`, the hydrate component) and carry a distinct `hook_present` attr — the hydrate helper's own exec-chain marker, a separate governed contract, out of this task's scope.
  - Byte-identity confirmed from the task commit (36f6c237): the former single-target and burst resolve emissions had identical attr order/values, faithfully reproduced by the helper. For the exec marker, `AttachConnector` previously sliced `argv[1:]` directly (safe for its fixed 4-element argv) while `PathOpener` used a `len>0` guard; the consolidated helper adopts the guard, which is a no-op for `AttachConnector`'s fixed argv, so both sites' output is unchanged.

TESTS:
- Status: Adequate
- Coverage:
  - `TestEmitResolveDecision_Helper` (cmd/open_test.go:2226) — direct unit test: non-glob target emits exactly one INFO line with the full attr set (target/domain/resolved_path); glob target emits no line, proving the gate lives inside the helper.
  - `TestLogExecHandoff_Helper` (cmd/open_test.go:2260) — direct unit test: argv[0] strip + `target=tmux` + joined `args`; defensive empty-argv case (no panic, empty args). Level asserted INFO.
  - `TestOpenCommand_ResolveLog_GlobEmitsNoLine` (cmd/open_test.go:2184) — end-to-end via the production burst path (injected raw argv), asserting a glob target produces zero resolve records.
  - Pre-existing resolve-log tests (miss / session-hit / directory-hit, around cmd/open_test.go:2100-2182) still exercise the single-target production path through the helper.
- Notes: Well-scoped, not over-tested. Each contract has a direct helper unit test plus routing coverage; the gate-lives-in-the-helper property is asserted explicitly. The two exec call sites (`AttachConnector.Connect` / `PathOpener.Open`) end in `syscall.Exec` and are not directly unit-testable, but the helper they delegate to is covered in isolation and the delegation is trivially verifiable by reading — adequate for a refactor.

CODE QUALITY:
- Project conventions: Followed. Component logger bound once (`resolveLogger = log.For("resolve")`); no new `*slog.Logger` constructed; closed-taxonomy attr vocabulary respected; house-style thorough doc comments on both helpers naming the spec sections and the two call sites.
- SOLID principles: Good. Each helper has one responsibility (emit one governed line under one gate); `resolveDecision` remains a focused pure derivation reused by the emitter.
- Complexity: Low. Straight-line helpers, one branch each.
- Modern idioms: Yes. Idiomatic early-return gate; defensive slice-length guard.
- Readability: Good. The parallel intent (single-source both two-site emissions) is explicit in the comments and the symmetric helper shapes.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/open.go:412,454 — Consider a source-walking drift-guard test (in the spirit of the existing `internal/log` single-owner and keymap-dispatch guards) asserting `resolveLogger.Info("resolved"` and the `process`-component `"exec"` emission each appear at exactly one call site, so a future inline emission that bypasses these helpers fails loudly. Optional and low-value given the small surface; decide whether the guard is worth the maintenance.

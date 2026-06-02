---
topic: portal-observability-layer
cycle: 1
total_proposed: 7
---
# Analysis Tasks: portal-observability-layer (Cycle 1)

## Task 1: Consolidate the discard-logger declaration + nil-guard into one internal/log helper
status: pending
severity: medium
sources: duplication (discardLogger idiom in four packages), standards (direct slog.New discard fallbacks in production)

**Problem**: The line `var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))` is independently re-declared in four packages (state, restore, tmux, bootstrap), each with its own nil-tolerant guard. The guard recurs at ~9 call sites in three different forms: a named helper (`state.loggerOrDiscard`), method accessors (`restore.Orchestrator.logger()` / `SessionRestorer.logger()`), and inline `if x == nil { x = discardLogger }` blocks (tmux `hooks_register.go` x3, bootstrap `orphan_sweep.go`, bootstrap `stale_marker_cleanup.go`). All implement the identical "nil `*slog.Logger` means discard" contract. This is also a literal violation of the spec's "Call-site logging pattern" Prohibited rule — "Direct construction of `*slog.Logger` outside the internal/log package" — which the package overview restates as "No `*slog.Logger` is constructed anywhere outside it." The per-package copies are pure boilerplate that will drift with no compiler signal.

**Solution**: Export a single canonical helper from `internal/log` (already imported everywhere for `log.For`) backed by one package-level discard logger, e.g. `log.OrDiscard(l *slog.Logger) *slog.Logger` (returns `l` if non-nil, else the shared discard logger) and/or `log.Discard() *slog.Logger`. Replace the four `var discardLogger = ...` declarations and route the nine guard sites through the helper. The restore accessor methods and `state.loggerOrDiscard` become one-line forwarders or are deleted. Pure consolidation of an identical existing contract; no behavior change.

**Outcome**: Exactly one `slog.New(slog.NewTextHandler(io.Discard, nil))` exists in production, inside `internal/log`. No production file outside `internal/log` constructs a `*slog.Logger`. All nil-tolerant fallback sites dispatch through the shared helper. The spec's no-direct-construction Prohibited rule holds for production code.

**Do**:
1. Add `internal/log/discard.go` (or extend an existing file) with one package-level `discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))` and exported `OrDiscard(l *slog.Logger) *slog.Logger` plus `Discard() *slog.Logger` (whichever the call sites need — `OrDiscard` covers the nil-guard sites, `Discard` covers any bare-sink construction).
2. In `internal/state/logger_nil.go`: delete the local `discardLogger` (line 14) and rewrite `loggerOrDiscard` (lines 19-24) as a forwarder to `log.OrDiscard` (or delete it and inline `log.OrDiscard` at its callers).
3. In `internal/restore/logger_nil.go`: delete the local `discardLogger` (lines 14-28) and rewrite the `Orchestrator.logger()` / `SessionRestorer.logger()` accessors as forwarders to `log.OrDiscard`.
4. In `internal/tmux/portal_saver.go:27`: replace the local discard construction with `log.Discard()` / `log.OrDiscard`.
5. In `internal/tmux/hooks_register.go`: replace the three inline `if x == nil { x = discardLogger }` blocks (lines 230-232, 308-310, 384-385) with `log.OrDiscard(...)` assignments; remove any now-unused local `discardLogger`.
6. In `cmd/bootstrap/bootstrap.go:48`, `cmd/bootstrap/orphan_sweep.go:141-143`, and `cmd/bootstrap/stale_marker_cleanup.go:122-124`: replace the local discard construction / inline guards with the `internal/log` helper.
7. Confirm no remaining `slog.NewTextHandler(io.Discard` literal exists in production (non-`_test.go`) code outside `internal/log`.

**Acceptance Criteria**:
- A single package-level discard logger exists in `internal/log`; no other production file declares one.
- `log.OrDiscard` (and/or `log.Discard`) is the only path used by the previously-listed nil-fallback sites.
- `state.loggerOrDiscard`, `restore.Orchestrator.logger()`, and `restore.SessionRestorer.logger()` either forward to the helper or are removed.
- No behavior change: nil loggers still discard; non-nil loggers are returned unchanged.
- `go build ./...` and `go test ./...` pass.

**Tests**:
- Unit test in `internal/log` asserting `OrDiscard(nil)` returns a non-nil logger whose handler discards (no panic on `.Info`), and `OrDiscard(l)` returns the same `l` when non-nil.
- A grep-style guard test (or a manual verification noted in the task) confirming no production file outside `internal/log` constructs `slog.New(slog.NewTextHandler(io.Discard, ...))`.
- Existing state/restore/tmux/bootstrap tests still pass, proving the forwarders preserve nil-tolerant behavior.

## Task 2: Extract the thrice-repeated "show-hooks failed" WARN + wrap block in hooks_register.go
status: pending
severity: medium
sources: duplication (identical show-hooks-failed block repeated three times)

**Problem**: Three functions in `internal/tmux/hooks_register.go` — `RegisterHookIfAbsent`, `migrateHydrationHooks`, `migrateSessionClosedHook` — each handle a `c.ShowGlobalHooks()` error with the byte-identical pair `log.Warn("show-hooks failed", "error", err, "error_class", "unexpected")` followed by `return ... fmt.Errorf("show-hooks failed: %w", err)` (sites at lines 131-132, 239-240, 318-319). In-source comments already note the shape was "normalized to the uniform shape shared with the two sibling show-hooks branches" — the authors recognized it as one shape replicated across task boundaries. Any future change must be applied in three places or the lines silently diverge. The sites also disagree on logger source: one uses the `bootstrapLogger` package var, two use the injected `log` param.

**Solution**: Extract an unexported helper in `hooks_register.go`, e.g. `showGlobalHooksOrWarn(c *Client, log *slog.Logger) (string, error)` that performs the `ShowGlobalHooks` call, emits the canonical WARN on error, and returns the wrapped error. The three call sites collapse to `raw, err := showGlobalHooksOrWarn(c, log)`. Reconcile the logger source to a single one (route through the injected `log`, defaulting via `log.OrDiscard` if Task 1 lands, or the existing nil-guard) so all three branches log under a consistent logger.

**Outcome**: One implementation of the show-hooks-failed WARN + wrap. The three call sites are one-liners. The WARN message, attrs (`error`, `error_class="unexpected"`), and wrap string are pinned in a single place and cannot drift. Logger source is uniform across the three branches.

**Do**:
1. In `internal/tmux/hooks_register.go`, add `func showGlobalHooksOrWarn(c *Client, log *slog.Logger) (string, error)` that calls `c.ShowGlobalHooks()`, and on error emits `log.Warn("show-hooks failed", "error", err, "error_class", "unexpected")` then returns `("", fmt.Errorf("show-hooks failed: %w", err))`; on success returns the raw output and nil.
2. Reconcile the logger source: pick the injected `log` param as the single source (guarded for nil), and have the helper receive that logger. If the `bootstrapLogger`-using site cannot pass an injected logger, pass `bootstrapLogger` explicitly at that one call so the helper signature stays uniform.
3. Replace the three duplicated blocks (lines ~131-132, ~239-240, ~318-319) with calls to the helper.
4. Verify the wrapped-error string and WARN attrs are byte-identical to the prior behavior so existing log-assertion tests still pass.

**Acceptance Criteria**:
- A single `showGlobalHooksOrWarn` helper exists; the three call sites delegate to it.
- The emitted WARN line and the wrapped error are unchanged from current behavior.
- Logger source across the three branches is reconciled to one consistent source.
- `go build ./...` and `go test ./internal/tmux/...` pass.

**Tests**:
- Unit test driving `showGlobalHooksOrWarn` with a `Commander` mock whose `ShowGlobalHooks` fails: assert the returned error wraps the underlying error with `show-hooks failed:` and that a single WARN with `error_class=unexpected` is emitted (capture via a test slog handler).
- Existing `RegisterHookIfAbsent` / `migrateHydrationHooks` / `migrateSessionClosedHook` tests still pass, confirming the error path is preserved at all three call sites.

## Task 3: Remove the redundant daemon "starting" INFO line dropped by the spec
status: pending
severity: medium
sources: standards (redundant daemon startup INFO retained against explicit spec drop)

**Problem**: The spec's "Saver and daemon lifecycle event taxonomy" → "Process/subsystem boundary" section explicitly resolves that the daemon's startup is marked by `process: start process_role=daemon` and that a redundant `daemon: spawn` event "(it would fire at the same instant ... carrying the same data) is therefore dropped." The cataloged daemon lifecycle events are exactly three: `lock acquired`, `self-eject`, `shutdown`. The implementation still emits `logger.Info("starting")` under `component=daemon` at the head of the daemon `RunE` (`cmd/state_daemon.go:596`) — an uncataloged INFO firing at the same instant as `process: start process_role=daemon`, carrying no unique data (version/pid are now baseline attrs). The in-source comment admits it is a migration bridge: "Kept here so no existing log line silently disappears mid-migration." This is the redundant spawn marker the spec said to drop; leaving it produces a duplicate daemon-start INFO at production level and an event outside the closed daemon lifecycle catalog.

**Solution**: Remove the `logger.Info("starting")` call in the daemon `RunE`. The OS-process-boundary `process: start process_role=daemon` (emitted by `log.Init` in `main`) plus the cataloged `daemon: lock acquired` already cover daemon startup; the taxonomy sanctions no additional daemon-component start line.

**Outcome**: The daemon emits no uncataloged start INFO. The daemon's startup is observable solely via `process: start process_role=daemon` and `daemon: lock acquired`. The daemon-component lifecycle event set matches the spec's closed catalog of exactly three events (`lock acquired`, `self-eject`, `shutdown`).

**Do**:
1. In `cmd/state_daemon.go` around line 596, delete the `logger.Info("starting")` call and its accompanying migration-bridge comment.
2. Confirm no other code path depends on that specific INFO line being present (search tests for `"starting"` / `daemon: starting`).
3. Verify the daemon still emits `daemon: lock acquired` after lock acquisition so startup remains observable.

**Acceptance Criteria**:
- `cmd/state_daemon.go` no longer emits a `daemon: starting` (or equivalent `logger.Info("starting")`) line.
- The daemon-component INFO events are limited to the spec's catalog (`lock acquired`, `self-eject`, `shutdown`).
- Daemon startup remains observable via `process: start process_role=daemon` + `daemon: lock acquired`.
- `go build ./...` and `go test ./cmd/...` pass.

**Tests**:
- Update or add a daemon-startup log-assertion test: assert no `starting` INFO is emitted under `component=daemon`, and that `daemon: lock acquired` is still emitted on successful lock acquisition.
- Any existing test asserting the presence of `daemon: starting` is updated to reflect its removal.

## Task 4: Emit the state-mutation `op` as the required `op=` attr rather than as the slog message
status: pending
severity: medium
sources: standards (state-mutation op rendered as message, not as the required op= attr)

**Problem**: The spec's "State-mutation audit trail" mechanical rule lists `op` under "Required attrs," and `op` is an explicit entry in the closed 49-key attr vocabulary (`set` / `modify` / `rm` / `clean-stale` / `migrate` / `set-noop`). The convention rule also states "data lives in attrs ... never `logger.Info(fmt.Sprintf(...))`." Across every store-mutation site and the config-migration site, the implementation passes the op verb as the slog *message string* (e.g. `logger.Info(op, "hook_key", key, ...)` and `logger.Info("migrate", "via", "migrate", ...)`) rather than carrying it as an `op=` attr. The rendered line is `hooks: set hook_key=... via=cli ...` with no `op=set` token, and the JSON handler emits no `"op"` field — so programmatic filtering by the `op` attr (the spec's stated rationale) and `grep op=` both fail. Sites: `internal/hooks/store.go:95,140,190`, `internal/alias/store.go:187,215`, `internal/project/store.go:141,255,291`, `cmd/config.go:59,66,72`.

**Solution**: Carry `op` as an explicit attr at every state-mutation breadcrumb, e.g. `logger.Info("mutate", "op", op, "hook_key", key, ...)` (or keep a terse stable message and add the `"op", op` attr pair), so the closed `op` attr-key is actually emitted and JSON/`grep op=` filtering works. Apply uniformly across all listed sites including the config-migration breadcrumbs.

**Outcome**: Every state-mutation audit line carries an `op=<verb>` attr drawn from the closed value space. JSON output contains an `"op"` field; `grep op=set` finds set mutations. The "Required attrs: op" rule is satisfied literally across the whole state-mutation audit-trail family.

**Do**:
1. Decide the stable terse message string for the family (e.g. `"mutate"`, or keep the existing per-store message) and add `"op", op` as an explicit attr at each site.
2. Update `internal/hooks/store.go` (lines 95, 140, 190), `internal/alias/store.go` (lines 187, 215), and `internal/project/store.go` (lines 141, 255, 291) so the op verb is passed as the `op` attr, not (only) as the message.
3. Update `cmd/config.go` migration breadcrumbs (lines 59, 66, 72) to carry `"op", "migrate"` as an attr.
4. Confirm the emitted `op` values stay within the closed value space (`set` / `modify` / `rm` / `clean-stale` / `migrate` / `set-noop`).
5. Verify the WARN failure variants of these breadcrumbs also carry `op=` consistently with the success variants.

**Acceptance Criteria**:
- Every state-mutation INFO and its matching WARN failure line carries an `op=<verb>` attr.
- The rendered text line includes an `op=` token; the JSON handler emits an `"op"` field.
- `op` values are confined to the closed value space.
- No other attrs or message semantics regress.
- `go build ./...` and `go test ./internal/hooks/... ./internal/alias/... ./internal/project/... ./cmd/...` pass.

**Tests**:
- Per-store unit tests asserting that a `set` / `modify` / `rm` / `clean-stale` mutation emits a record carrying `op=<verb>` (capture via a test slog handler and assert on the attr, not just the message).
- A config-migration test asserting the migrate breadcrumb carries `op=migrate`.
- Optionally assert the JSON-mode handler renders an `"op"` field for one representative mutation.

## Task 5: Align the `project` attr with its closed-vocabulary definition (name, not path)
status: pending
severity: low
sources: standards (project attr carries the filesystem path, not the project name)

**Problem**: The closed attr-key vocabulary defines `project` as "project name from `projects.json`" and `value` as the "verbatim new value for set/modify." The project store inverts this: the `project` attr carries the PATH (the addressable match key) and `value` carries the NAME (`internal/project/store.go:136,141,251,255,286,291`). The deviation is documented in an in-source comment, but it still contradicts the spec's closed-vocabulary definition, and there is already a `path` key for filesystem paths. A reader grepping `project=<name>` per the spec instead finds `project=<path>`.

**Solution**: Align with the spec — emit the project name under the `project` attr and the filesystem path under the existing `path` key, dropping the documented inversion. (If the team instead prefers the current behavior, the alternative is a spec amendment redefining `project` for the projects store; this task takes the code-alignment route since `path` already exists to carry the filesystem path.)

**Outcome**: The project store's mutation breadcrumbs carry `project=<name>` and `path=<path>`, matching the closed attr-key definitions. `grep project=<name>` works as the spec implies. No silent code-vs-spec contradiction remains.

**Do**:
1. In `internal/project/store.go` (lines 136, 141, 251, 255, 286, 291), change the breadcrumb attrs so the project NAME is emitted under `project` and the filesystem PATH under `path`.
2. Re-check that `value` is reserved for the verbatim new value per the spec, not the name, and adjust if the inversion bled into `value`.
3. Remove or update the in-source comment that documented the now-corrected inversion.
4. Confirm the addressable match key is still logged (under `path`) so audit lines remain useful for grepping by path.

**Acceptance Criteria**:
- Project-store mutation lines carry `project=<project name>` and `path=<filesystem path>`.
- The closed-vocabulary definitions for `project`, `path`, and `value` are honored.
- The documented-inversion comment is removed/updated.
- `go build ./...` and `go test ./internal/project/...` pass.

**Tests**:
- Project-store unit test asserting a mutation breadcrumb carries `project` = the project name and `path` = the filesystem path (capture via a test slog handler).
- Existing project-store tests updated for the corrected attr semantics.

## Task 6: Make SweepOrphanFIFOs' caller-vs-self component attribution explicit at the boundary
status: pending
severity: medium
sources: architecture (two-logger split inside a single cycle function obscures the seam contract)

**Problem**: `SweepOrphanFIFOs(dir, liveMarkerKeys, logger *slog.Logger)` (`internal/state/fifo_sweep.go:36-72`, decl 13-20) takes an injected `*slog.Logger` but uses it only for the per-item WARN lines (lstat/remove failures). The cycle-summary INFO (`"orphan-fifo sweep complete"`) and the per-reaped DEBUG breadcrumb go to a *different* package-level logger (`cleanLogger = log.For("clean")`). So one invocation emits records under two components driven by two distinct logger objects, and the injected parameter is no longer the function's logger. This is the only function in the state package that does this — peers (`commit.go`, `capture.go`, `scrollback.go`) use the injected logger uniformly, and `signal_hydrate.go`'s `WriteFIFOSignal` uses only a package-level logger and takes no logger param. The split is documented as intentional (WARNs carry the caller's bootstrap component for correlation; the summary groups under `clean`), but the signature is misleading: a caller passing `logger` reasonably expects it to be the function's sink, yet the headline summary bypasses it. A future "consolidation" onto the injected logger would silently re-attribute the summary away from `clean`; dropping the near-vestigial parameter would silently re-attribute the WARNs.

**Solution**: Make the component-attribution intent explicit at the boundary rather than relying on a body-internal dual binding. Choose one of:
- (a) Drop the injected `*slog.Logger` parameter and emit every line under `cleanLogger` (simplest; per-item WARNs lose the bootstrap-step component but gain consistency), or
- (b) Keep the split but rename/document the parameter to signal it is a *caller-component WARN sink* distinct from the sweep's own summary logger (e.g. `warnLogger` / `callerLogger`, plus a doc comment on the signature).
Apply the same decision to any other cycle function that grows a caller-vs-self component distinction so the pattern is uniform.

**Outcome**: The function signature truthfully conveys where each class of line is attributed. A future maintainer cannot accidentally re-attribute either the summary (away from `clean`) or the WARNs (away from the caller's component) by "consolidating" or dropping the parameter, because the boundary intent is explicit in the name/doc (option b) or there is only one logger (option a).

**Do**:
1. Decide between (a) single-logger and (b) explicit-dual-logger. Prefer (b) if the per-item WARN's bootstrap-step component correlation is load-bearing for reboot-morning forensics; otherwise (a).
2. If (a): remove the `logger *slog.Logger` parameter from `SweepOrphanFIFOs`, route the per-item WARNs through `cleanLogger`, and update the bootstrap caller (step 10, `SweepOrphanFIFOs`) to stop passing a logger.
3. If (b): rename the parameter (e.g. `warnLogger`) and add a doc comment on `SweepOrphanFIFOs` stating that the parameter is the caller-component WARN sink while the summary/DEBUG lines are emitted under the package-level `clean` logger by design.
4. Scan other state cycle functions for the same caller-vs-self split and apply the identical decision so the pattern is uniform.

**Acceptance Criteria**:
- `SweepOrphanFIFOs`' signature unambiguously conveys its logging behavior (either one logger, or a clearly-named/documented caller-WARN-sink parameter).
- The component attribution of every emitted line (per-item WARN, per-reaped DEBUG, cycle-summary INFO) is unchanged from current behavior unless option (a) is chosen, in which case the WARNs intentionally move to `clean` and that is documented.
- No other state cycle function silently carries the same undocumented split.
- `go build ./...` and `go test ./internal/state/...` pass.

**Tests**:
- Unit test capturing emitted records for one `SweepOrphanFIFOs` run with a forced per-item failure: assert the per-item WARN and the cycle-summary INFO are attributed to the intended components per the chosen option.
- If option (a): update the bootstrap-step-10 caller test for the removed parameter.
- Regression: existing orphan-FIFO sweep tests still pass.

## Task 7: Add a drift-tripwire test tying ResolveProcessRole to the real command set
status: pending
severity: low
sources: architecture (process_role taxonomy decoupled from the Cobra command tree it mirrors)

**Problem**: `ResolveProcessRole` (`internal/log/process_role.go:41-72`, invoked from `main.go:32`) re-derives the subcommand-to-role mapping from a hand-maintained longest-prefix match over `os.Args` (`state daemon` → daemon; `open`/`x`/`attach` → tui; etc.) because `Init` must run before Cobra parses argv. The resolution-before-parse constraint is sound, but it places a second, independent copy of command-routing knowledge in `internal/log`, structurally divorced from `cmd/`'s Cobra registration. If a subcommand is renamed or added, the role table will not fail to compile and will silently fall through to `roleBootstrap` — mis-attributing `process_role`, which the spec calls "critical for multi-writer disambiguation on reboot-recovery days." There is no compile-time link or test fixture tying the role table to the real command set.

**Solution**: Add a single guard test that enumerates the production command verbs (or the canonical argv shapes for each role) and asserts `ResolveProcessRole` returns the expected role for each, co-located so a renamed/added subcommand forces a visible failure. No production restructuring needed.

**Outcome**: A renamed or added subcommand that would silently mis-attribute `process_role` instead trips a visible test failure. The cmd-to-internal/log routing-knowledge boundary has an explicit drift tripwire.

**Do**:
1. Add a test (in `internal/log` or a cmd-side test that imports both) that builds the canonical argv shape for each role — daemon, bootstrap, hydrate, hooks_cli, tui, clean — and asserts `ResolveProcessRole(args)` returns the expected role.
2. Include the role-defaulting case (an unknown/unrouted verb falls through to `roleBootstrap`) so the fallback is intentional and asserted, not silent.
3. Add a comment in the test pointing at `cmd/` command registration so a contributor adding/renaming a subcommand knows to update both the role table and this fixture.
4. Optionally enumerate the production command verbs from the Cobra command tree (if importable without an init-order issue) to assert each maps to a non-default role — strengthening the tripwire beyond hand-listed argv shapes.

**Acceptance Criteria**:
- A test asserts `ResolveProcessRole` returns the correct role for each canonical argv shape (daemon, bootstrap, hydrate, hooks_cli, tui, clean).
- The default-fallback behavior for an unrouted verb is explicitly asserted.
- The test fails visibly if a role's expected argv shape stops resolving correctly.
- `go test ./internal/log/...` (or the host package) passes.

**Tests**:
- Table-driven `TestResolveProcessRole` covering one canonical argv per role plus the fallback case.
- (Optional) A cross-boundary assertion that every registered Cobra command verb resolves to a non-`roleBootstrap` role where intended.

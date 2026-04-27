---
status: in-progress
created: 2026-04-27
cycle: 1
phase: Traceability Review
topic: Built-In Session Resurrection
---

# Review Tracking: Built-In Session Resurrection - Traceability

## Findings

### 1. Task 4-4 chooses `migrate-rename` argv wiring that defeats the spec's atomic-migration contract

**Type**: Hallucinated content (planning-pinned decision contradicts the spec's contract)
**Spec Reference**: "Resume Hook Firing → Session Rename: Hook Key Migration → Argument source" (lines 916–924) — "the contract from the spec: hook keys are migrated atomically on rename events".
**Plan Reference**: `built-in-session-resurrection-4-4` — `phase-4-tasks.md` lines 281–298 + 332–337
**Change Type**: update-task

**Details**:
The spec explicitly delegates the argv-wiring decision to planning, but pins the contract: "hook keys are migrated atomically on rename events". Task 4-4 currently pins `migrateRenameCommand` to `portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'` — passing the same value for both `<old>` and `<new>`. The task body itself acknowledges this makes task 4-3's body a no-op ("the documented follow-up is a daemon-side rename-delta mechanism tracked for Phase 6 / post-v1 polish"), then justifies the gap by saying "best-effort / CleanStale prunes orphaned entries" covers it.

That justification misreads the spec. CleanStale-pruning of orphaned hook keys is the **failure-mode** behavior (spec lines 924, "If migration fails (malformed names, I/O error)..."), not the **happy path**. A registration whose argv structurally cannot achieve migration is not a transient I/O failure — it is a planning decision to ship migration in name only.

The spec also offers the planning phase a concrete-enough mechanism to implement migration correctly: "in-memory 'last-seen names' map maintained by the daemon" (line 922). The daemon already enumerates `list-sessions` on every tick (Phase 2 task 2-8) and writes `sessions.json`. Comparing the prior tick's session-name list to the current tick's list yields the rename delta. The planning phase needs to either (a) wire the daemon-side delta, or (b) defer Phase 4 task 4-4 entirely until the daemon-side support lands, or (c) accept the spec's other suggestion ("uses the `session-renamed` hook's exposed variables") — `#{hook_session_name}` is documented as the new name, but the prior name in tmux 3.0+ is exposed via tmux's session-rename event hook variable on the renamed session itself. (The spec says `#{client_last_session}` is unreliable; planning needs to either pick a reliable variable or build the delta side-band.)

The plan must not register a hook whose argv shape contradicts the spec contract. Either:
- **(Preferred)** Defer registration until a working argv source exists, OR
- Add a Phase 2 task that produces the rename-delta side-band, then wire it here, OR
- Document the limitation more honestly as `[needs-info]` and route to user.

**Current**:
````
| built-in-session-resurrection-4-4 | Register `portal state migrate-rename` on `session-renamed` alongside `notify` via content-based idempotency | coexists with existing `portal state notify` entry on same event (both substrings matched per-command, no cross-contamination), re-running bootstrap appends no duplicate `migrate-rename` entry, old/new session names passed via tmux format expansion as argv (planning chooses `#{hook_session_name}` + prior-name variable), `command -v portal` guard wraps the invocation, `show-hooks` parsing distinguishes the two Portal substrings, first-ever bootstrap on server without either entry appends both |
````

(In the corresponding `phase-4-tasks.md`, the body of task 4-4 currently pins `migrateRenameCommand = run-shell "command -v portal >/dev/null 2>&1 && portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'"` and documents that this makes the migration a no-op.)

**Proposed**:
The proposed fix is to mark the planning-pinned wiring as `[needs-info]` and surface the decision to the user rather than ship a broken-by-construction registration. Replace the relevant phase-4-tasks.md task-body section with an explicit deferral:

````
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
````

And update the planning.md task-table row to flag the blocker:

````
| built-in-session-resurrection-4-4 | [BLOCKED — needs planning decision on prior-name argv source] Register `portal state migrate-rename` on `session-renamed` alongside `notify` via content-based idempotency | coexists with existing `portal state notify` entry on same event (both substrings matched per-command, no cross-contamination), re-running bootstrap appends no duplicate `migrate-rename` entry, old/new session names sourced from a route that satisfies the spec's atomic-migration contract (planning to pin Route A: tmux format variable, or Route B: daemon-side rename-delta side-band), `command -v portal` guard wraps the invocation, `show-hooks` parsing distinguishes the two Portal substrings, first-ever bootstrap on server without either entry appends both |
````

**Resolution**: Pending
**Notes**:

---

### 2. Task 3-7's `RestoreWithMarker` wrapper conflates the spec's step-3-then-step-6 split with a single composite primitive

**Type**: Hallucinated content (added abstraction layer that doesn't trace to spec)
**Spec Reference**: "Bootstrap Flow → PersistentPreRunE Sequence" (lines 989–1043) — steps 3 (set), 4 (saver), 5 (restore), 6 (clear) are explicitly separate
**Plan Reference**: `built-in-session-resurrection-3-7` (`phase-3-tasks.md` lines 622–657)
**Change Type**: update-task

**Details**:
Task 3-7 introduces `RestoreWithMarker()` that bundles `SetRestoring → Restore → ClearRestoring` into one composite primitive. The task itself acknowledges this is at odds with the spec's bootstrap flow: "Phase 5 will split the set and clear across steps 3–6 with `_portal-saver` creation nested between them. So this task's wrapper is a convenience for the self-contained restore path while Phase 5 uses the individual set-and-clear primitives directly. Add both: `SetRestoring()`, `ClearRestoring()`, and `RestoreWithMarker()`."

The spec's load-bearing ordering is step 3 (set marker) → step 4 (create `_portal-saver`) → step 5 (restore) → step 6 (clear). If `RestoreWithMarker` exists, future maintainers may use it from non-bootstrap code paths, breaking the ordering invariant: the marker would be set after `_portal-saver` already exists, exposing the race the spec explicitly warns against ("creating `_portal-saver` fires `session-created` which the hook pipeline would otherwise use to dirty the flag").

The composite is justified in the task as "a convenience for the integration test in task 3-13" — but task 3-13's integration test can call `SetRestoring → Restore → ClearRestoring` directly with one extra line, eliminating the composite entirely.

**Current**:
````
**Solution**: Add `func (o *Orchestrator) RestoreWithMarker() error` in `internal/restore/restore.go` (or a thin wrapper file `restore_marker.go`). Flow: (1) call `client.SetServerOption("@portal-restoring", "1")` ... (2) `defer` a function that calls `client.UnsetServerOption("@portal-restoring")` ... (3) call `Restore()` (task 3-6) ... (4) return whatever `Restore()` returned ... Bootstrap integration in Phase 5 calls `RestoreWithMarker()` in step 5, immediately after step 4 (`_portal-saver` setup) — but *note* the spec's step-3 / step-5 ordering: the set happens in step 3 (before `_portal-saver` creation in step 4), the clear happens after step 5 (before step 6). This task's wrapper is the pair together for unit-test scope; Phase 5 will split the set and clear across steps 3–6 with `_portal-saver` creation nested between them. So this task's wrapper is a convenience for the self-contained restore path (Phase 3 integration tests) while Phase 5 uses the individual set-and-clear primitives directly. Add both: `SetRestoring()`, `ClearRestoring()`, and `RestoreWithMarker()` (wrapper that calls all three in order). Phase 5 uses the primitives; task 3-13 integration tests use the wrapper.
````

**Proposed**:
````
**Solution**: Add `SetRestoring()` and `ClearRestoring()` as standalone primitives on `Orchestrator` in `internal/restore/restore.go` (or a thin wrapper file `restore_marker.go`). Each is a single-purpose method: `SetRestoring` calls `client.SetServerOption("@portal-restoring", "1")` and returns the wrapped error; `ClearRestoring` calls `client.UnsetServerOption("@portal-restoring")` and returns the wrapped error. Do NOT introduce a composite `RestoreWithMarker()` wrapper — the spec's bootstrap flow interleaves `_portal-saver` creation between the set and clear (step 3 → step 4 → step 5 → step 6), and a composite wrapper would mask that ordering invariant for any future caller. Phase 5's bootstrap orchestrator (task 5-2) calls the primitives directly in spec order. Task 3-13's integration test calls the same primitives directly with explicit set / restore / clear lines — the extra line is worth the documentation value.
````

(The `Do` section's `RestoreWithMarker` snippet should be removed; `Acceptance Criteria` should drop the `RestoreWithMarker` and `defer` claims; `Tests` should drop the wrapper-specific cases; remaining tests cover `SetRestoring` and `ClearRestoring` independently.)

**Resolution**: Pending
**Notes**:

---

### 3. Task 6-3 contradicts spec on which write triggers rotation

**Type**: Hallucinated content (interpretation reversed from the spec wording)
**Spec Reference**: "Observability & Diagnostics → Log Rotation" (lines 1331–1337) — "On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` ... then starts a fresh `portal.log`"
**Plan Reference**: `built-in-session-resurrection-6-3` (`phase-6-tasks.md` lines 222–224, 263)
**Change Type**: update-task

**Details**:
The spec says rotation fires "On reaching 1 MB during a write" and the rotated old contents include the write that crossed the boundary — i.e., the triggering write LANDS in the file being renamed to `.old`, then a fresh `portal.log` starts for the NEXT write.

Task 6-3 reverses this: "rotation happens BEFORE the write. The write that would push to exactly 1MB is the trigger... the triggering write lands in the fresh file." The task even explicitly notes its own departure from the spec phrasing — "spec says 'write that exactly equals 1 MB rotates' — rotation happens BEFORE the write."

The two semantics differ in observable behavior: under the spec semantics, `portal.log.old` can briefly exceed 1 MB by one log-line's worth (the triggering write); under the plan's semantics, `portal.log` always strictly stays under 1 MB at rest. The plan's choice is defensible engineering, but it is not what the spec says.

The plan should either (a) implement the spec's semantics verbatim, or (b) flag this as a planning-decision delta and surface it for user approval rather than silently re-interpreting "during a write" as "before a write."

**Current**:
````
**Solution**: Introduce a `Logger.rotating bool` flag set by `OpenLogger(path, true)`. On each `emit`, *if* `rotating` is true, stat the current file before writing; if `Stat.Size() + len(pending) >= 1MB`, close the current fd, rename → `.old` (replacing any existing `.old`), open a fresh fd with `O_APPEND|O_CREATE|O_WRONLY` mode `0600`, then write. ... A write that exactly equals 1MB triggers rotation: `size + pending >= 1MB` is an inclusive threshold so the write lands in the fresh file rather than the old one (keeps `portal.log` strictly under 1MB once rotation is implemented).
````

**Proposed**:
````
**Solution**: Introduce a `Logger.rotating bool` flag set by `OpenLogger(path, true)`. On each `emit`, *if* `rotating` is true: write the pending line first; then `Stat` the file; if `Stat.Size() >= 1MB`, close the current fd, rename → `.old` (replacing any existing `.old`), open a fresh fd with `O_APPEND|O_CREATE|O_WRONLY` mode `0600` for the NEXT write. The triggering write lands in the file that becomes `.old`, matching the spec's "On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` ... then starts a fresh `portal.log`" — `portal.log.old` may briefly contain content slightly over 1 MB by one log-line's worth; `portal.log` (the active file) starts fresh for subsequent writes.
````

(Update the `maybeRotate` snippet so it runs AFTER the write; update the `"the triggering write lands in the fresh portal.log"` test to assert the inverse — the triggering write lands in `.old`. Update edge-case note from "rotate-before-write" to "rotate-after-write".)

**Resolution**: Pending
**Notes**:

---

### 4. Task 5-2's `Restoring.Clear` failure handling diverges from spec by treating it as fatal mid-Run rather than as a deferred safety-net

**Type**: Hallucinated content (added "fatal" semantics not in spec)
**Spec Reference**: "Observability & Diagnostics → Fatal Bootstrap Errors" (lines 1369–1378) — lists four fatals: tmux -V, EnsureServer, mass set-hook, `@portal-restoring` set-option (set, not clear)
**Plan Reference**: `built-in-session-resurrection-5-2` (step 6 logic) and `built-in-session-resurrection-3-7` (clear failure handling)
**Change Type**: update-task

**Details**:
The spec's "Fatal Bootstrap Errors" list (line 1376) names "`@portal-restoring` set-option fails" — singular, the SET, not the clear. Task 5-2 applies fatal semantics to both set and clear: "step 6 ... `Restoring.Clear` failure: **fatal**. Per spec 'Fatal Bootstrap Errors': '`@portal-restoring` set-option fails: same as `set-hook` failure'".

The spec's wording "set-option fails" is ambiguous (it covers both set and unset since both use the same tmux primitive), but the underlying rationale differs: (a) failing to SET the marker means the daemon may capture mid-build state — actually catastrophic; (b) failing to CLEAR the marker means the daemon will skip ticks until the next server restart — degraded but bounded (volatile marker self-heals). Task 3-7 itself acknowledges this distinction: "Set failure is fatal per spec ... Clear failure is non-fatal but observable — log only. The marker being stuck means the daemon stays silent until the next tmux server restart (volatile server-option). Not ideal, but better than aborting bootstrap at the end of otherwise-successful restore."

Task 5-2 should match task 3-7's interpretation: clear failure logs and continues; only set failure is fatal. Task 5-2's current "fatal" treatment of clear failure also creates a contradiction with task 3-7's own clear-failure-is-soft contract.

**Current** (from task 5-2 `Do` section, step 6):
````
6. `if err := o.Restoring.Clear(); err != nil` → **fatal**. Per spec "Fatal Bootstrap Errors": "`@portal-restoring` set-option fails: same as `set-hook` failure" — return `serverStarted, fmt.Errorf("bootstrap step 6 (Clear @portal-restoring): %w", err)`. The clear is wrapped in a `defer` off of step 3 for safety too (so a panic between 3 and 6 still clears the marker), but the explicit-call path is the primary semantics.
````

**Proposed**:
````
6. `if err := o.Restoring.Clear(); err != nil` → log WARN and continue. The marker stays set; the daemon will skip ticks until the next server restart (volatile server-option, self-heals). This is degraded but bounded — better than failing the whole bootstrap at the tail of otherwise-successful work. Matches task 3-7's clear-failure-is-soft contract. The clear is also wrapped in a `defer` off of step 3 for safety (so a panic between 3 and 6 still attempts the clear); the deferred and explicit calls are both no-ops on success.
````

Update task 5-2's `Acceptance Criteria` accordingly:
````
- [ ] `Restoring.Clear` failure logs WARN (via `ComponentBootstrap`) and does NOT fail `Run` — soft path.
````
(Replacing the existing "Restoring.Clear failure returns a fatal error wrapping the underlying tmux error" line.)

Update task 5-2's `Tests`:
````
- `"it logs and continues when Restoring.Clear fails"` (replacing `"it reports Restoring.Clear failure as a fatal error"`)
````

Also update task 6-8's Fatal list (`Do` section) to remove the "step 6 (Clear @portal-restoring)" wrap. Keep only step 1 EnsureServer, step 2 mass-register, and step 3 Restoring.Set as FatalError producers.

**Resolution**: Pending
**Notes**:

---

### 5. Task 1-9 omits the spec-pinned `--purge` parsing for the Phase-1 cleanup slice (already in task 1-1 — but task 1-9 should explicitly preserve it)

**Type**: Incomplete coverage (acceptance row in planning.md does not mention --purge)
**Spec Reference**: "CLI Surface → `portal state cleanup`" (lines 1201–1214) — `--purge` is part of the user-facing surface
**Plan Reference**: `planning.md` Phase 1 acceptance + tasks-table row for `built-in-session-resurrection-1-9`
**Change Type**: update-task

**Details**:
Phase 1 acceptance for task 1-9 in `planning.md` reads: "`portal state cleanup` command exists and removes Portal's hook entries (daemon teardown and `--purge` land in Phase 6)". This is fine for daemon-teardown deferral, but the `--purge` flag must already PARSE without error in Phase 1 (task 1-1 declared it; task 1-9's body must not regress that). The phase-1-tasks.md task body for 1-9 does include this in its acceptance ("`--purge` is accepted on `portal state cleanup` as a boolean flag (parsing only — behaviour lands later)") — so the per-task file is correct, but the planning.md row is silent on it.

Minor wording finding only — flagged for completeness but lower priority than findings 1–4.

**Current** (planning.md, Phase 1, tasks table row for 1-9):
````
| built-in-session-resurrection-1-9 | Implement the Phase 1 slice of `portal state cleanup` | no tmux server running is not an error, partial failure still attempts subsequent removals, running twice in a row is a clean no-op the second time |
````

**Proposed**:
````
| built-in-session-resurrection-1-9 | Implement the Phase 1 slice of `portal state cleanup` | no tmux server running is not an error, partial failure still attempts subsequent removals, running twice in a row is a clean no-op the second time, `--purge` parses as a boolean flag without error (body deferred to Phase 6) |
````

**Resolution**: Pending
**Notes**:

---

### 6. Task 3-3 prediction-of-live-indices invents a base-index/pane-base-index reading step the spec does not request

**Type**: Hallucinated content (technical approach not discussed in spec)
**Spec Reference**: "Save Format & Schema → Index Semantics and base-index / pane-base-index" (lines 320–330), "Save Format & Schema → Canonical paneKey" (lines 332–356), "Bootstrap Flow → step 5" (lines 1018–1024)
**Plan Reference**: `built-in-session-resurrection-3-3` (`phase-3-tasks.md` lines 173–181, 184–198)
**Change Type**: update-task

**Details**:
The spec's design for handling base-index drift is to **re-query `list-panes -t <session>` after pane creation** to map saved-structure position → live tmux index. Task 3-5 already implements this re-query. The spec is explicit: "After creating each window via `new-window` and each pane via `split-window`, Portal re-queries `list-panes -t <session>` to map saved-structure position → actual live tmux index" (line 324).

Task 3-3 invents an alternative approach: predict live indices by reading `base-index` and `pane-base-index` server options BEFORE creating panes, compute predicted live paneKeys, create FIFOs at the predicted paths, embed predicted FIFO paths in hydrate commands. Task 3-3 even acknowledges the approach is not in the spec ("the spec says 'create FIFO before pane creation' but also says FIFO paths use the live paneKey. Resolution: predict live indices...") and that prediction can fail ("If the prediction in task 3-3 matched (the common case), the live and predicted paneKeys are identical and FIFOs at the predicted path are already at the right live-path. If the prediction was wrong (pathological case), the mismatch is detected and the task logs a clear warning plus still sets the marker at the live paneKey").

The spec's design avoids this complexity: FIFO creation can happen AFTER each pane is created, using the live paneKey re-queried via `list-panes`. The pane's initial command is `sh -c 'portal state hydrate --fifo F --file S --hook-key K; exec $SHELL'`; the helper opens FIFO `F` for read with a 3-second timeout. So as long as the FIFO exists by the time `signal-hydrate` is invoked (which only happens at attach time, well after bootstrap), the timing works.

The spec's flow (line 1019–1020): "For each pane: compute FIFO path ...; `os.Remove(path)`; `syscall.Mkfifo(path, 0600)`. **Then** `new-session -d -s <name> -c <root_cwd> '...hydrate --fifo <F> ...'`". The "then" reading suggests FIFO creation BEFORE pane creation — but that requires knowing the paneKey before the pane exists. The spec resolves this via the canonical-paneKey rule: "Indices used in paneKey are always *live* indices (post-restoration)" + "Setting `@portal-skeleton-<paneKey>` and creating `hydrate-<paneKey>.fifo` using **live** paneKey (re-queried via `list-panes` after pane creation)" (line 349).

So the spec's actual flow is: create the pane with the hydrate command pointing at a path like `<state-dir>/hydrate-<placeholder>.fifo`, then re-query and re-create the FIFO under the live paneKey. The placeholder in the command-line invocation can be whatever (the helper takes whatever `--fifo` arg it is given), as long as the live FIFO exists at that path by the time `signal-hydrate` writes to it.

Task 3-3's prediction approach is NOT a spec-mandated design choice — it's a planning invention to avoid two-pass pane creation. It is plausibly correct in the common case but adds complexity (option reads, prediction-vs-live check) the spec does not ask for. The cleaner spec-aligned approach is one of:
- **Option A**: Two-pass — create pane with placeholder FIFO path, re-query, create FIFO at live path, send a `--fifo-update` signal to the helper (complex; rejected).
- **Option B**: Pass the FIFO path to the helper at construction time using a path that incorporates the SAVED paneKey (not live); after pane creation, re-query the live paneKey, but ALSO create a symlink from the live-paneKey-FIFO-path to the saved-paneKey-FIFO-path so `signal-hydrate` (which uses live paneKey) finds the same FIFO. Or simpler: just have `signal-hydrate` use the saved paneKey too.
- **Option C**: Pass the FIFO path to the helper using a deterministic-but-non-paneKey-derived name (e.g., a UUID generated at restore time and stored alongside session state). Decouples FIFO naming from paneKey entirely.
- **Option D**: Accept the prediction approach — but acknowledge it's a planning invention and surface it for user approval, since it adds non-trivial complexity to the bootstrap flow.

This is a non-trivial design decision the planning phase has made implicitly. It deserves explicit user awareness.

**Current** (excerpt from task 3-3 Solution):
````
... Resolve this by reading `base-index` / `pane-base-index` once at the start of `RestoreSession`, predicting live indices from saved structural order, computing live paneKeys, creating FIFOs at those live paths, and building hydrate commands with the live FIFO path and saved `--file` / `--hook-key` values.
````

**Proposed**:
The proposed fix is to flag this as a planning decision needing user awareness. Update the task body's `Solution` section to add a `[needs-info]` block:

````
**[needs-info]**: Task 3-3's "predict live indices via `base-index` / `pane-base-index` server options" is a planning invention — the spec describes a re-query approach (line 324) but does not mandate prediction-before-creation. Prediction works in the common case but adds complexity (option reads, prediction-vs-live divergence handling in task 3-5). Alternative approaches the spec is compatible with:
- **Option B**: pass the saved-paneKey FIFO path to the helper at construction; after pane creation, re-query live paneKey and either (b1) symlink live → saved or (b2) have `signal-hydrate` use the saved paneKey from the index (NOT the live paneKey).
- **Option C**: decouple FIFO naming from paneKey — use a UUID stored in the index.

Planning has chosen Option A (prediction). User should confirm or override before implementation. If Option A stands, task 3-5's drift-detection becomes a defensive log-only branch; if any other Option is chosen, task 3-5 simplifies further.
````

Add an Acceptance Criterion to task 3-3:
````
- [ ] User has confirmed the "predict live indices via server-option read" approach (Option A) over the alternative spec-compatible approaches (Options B, C).
````

**Resolution**: Pending
**Notes**:

---

### 7. Task 3-13 integration-test scope creep — secondary tests duplicate tasks 5-8/5-9/5-10's coverage

**Type**: Hallucinated content (planning expansion beyond the spec's Phase-3 acceptance bullet)
**Spec Reference**: Phase 3 planning acceptance: "Integration test on an isolated `tmux -L` socket verifies a multi-session, multi-window save round-trips structure + ANSI scrollback."
**Plan Reference**: `built-in-session-resurrection-3-13` (`phase-3-tasks.md` lines 1305–1310)
**Change Type**: update-task

**Details**:
The spec's Phase-3 acceptance for the integration test is narrow: "multi-session, multi-window save round-trips structure + ANSI scrollback". Task 3-13 ships that but ALSO adds four supplementary tests:
- `TestPhase3_SweepRemovesOrphanFIFOs` — duplicates task 3-12's unit-level concern
- `TestPhase3_CorruptSessionsJSON` — Phase 5 task 5-2/5-9 territory
- `TestPhase3_HydrateTimeout` — task 3-9's behavior is already unit-tested
- `TestPhase3_ScrollbackFileMissing` — task 3-10's behavior is already unit-tested

Each supplementary test has its own integration cost (isolated socket setup, timing tolerance, CI flakiness). The spec's scope-control principle (YAGNI in test-design too) suggests trimming these to a single primary round-trip test. The hydrate-timeout / scrollback-file-missing concerns are unit-test territory — only the round-trip case genuinely needs a real tmux process.

Trimming is not strictly required by spec compliance — supplementary tests are a planning judgment call — but the planning task has expanded the test scope substantially beyond what the Phase-3 acceptance bullet calls for, and that expansion deserves user awareness.

**Current** (excerpt from task 3-13 Do section):
````
- Additional focused tests:
  - `TestPhase3_SweepRemovesOrphanFIFOs`: start server, create state dir with 2 FIFOs (one live-marked, one orphan), run sweep, assert only orphan removed.
  - `TestPhase3_CorruptSessionsJSON`: write a garbage `sessions.json`, run restore, assert stderr contains `"Portal state file is corrupt"`, no sessions created.
  - `TestPhase3_HydrateTimeout`: skeleton-restore a session, do NOT invoke signal-hydrate, wait 4s, assert pane's helper timed out, marker still set, shell is running (send-keys + capture shows a live shell prompt).
  - `TestPhase3_ScrollbackFileMissing`: skeleton-restore a session, delete the scrollback `.bin` before attach, invoke signal-hydrate, assert pane is empty but shell is running, marker was cleared (file-missing path).
````

**Proposed**:
````
- Additional focused tests (OPTIONAL — Phase 3 spec acceptance is satisfied by the primary round-trip test alone; the following expand coverage at the cost of additional integration-test surface area, CI runtime, and timing flakiness exposure):
  - `TestPhase3_HydrateTimeout` is the highest-value supplementary because the timeout path involves real tmux + real FIFO-block timing that unit tests cannot fully simulate. Recommended: keep this one.
  - `TestPhase3_SweepRemovesOrphanFIFOs`, `TestPhase3_CorruptSessionsJSON`, `TestPhase3_ScrollbackFileMissing` are unit-testable in isolation (tasks 3-12, 3-1, 3-10 already cover them at the unit level). Recommended: drop these supplementary integration variants in favor of the unit-test coverage already in those tasks.

If the user prefers full integration coverage, all four supplementary tests can stay; if they prefer leaner integration suite, drop the three duplicates and keep only the round-trip + hydrate-timeout pair.
````

**Resolution**: Pending
**Notes**:

---

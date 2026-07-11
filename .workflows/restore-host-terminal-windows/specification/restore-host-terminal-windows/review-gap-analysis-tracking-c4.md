---
status: complete
created: 2026-07-11
cycle: 4
phase: Gap Analysis
topic: restore-host-terminal-windows
---

# Review Tracking: Restore Host Terminal Windows - Gap Analysis

## Findings

### 1. Config-recipe PATH/env injection mechanism is asserted but never concretely defined, and differs from the native adapter's mechanism

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Spawn Architecture (Spawned-window environment / PATH injection), Config Schema (Recipe execution contract), Adapter Contract (Two implementations)

**Details**:
The spec establishes that the host terminal launches the spawned command in a **bare environment**, so `tmux` won't resolve unless Portal injects its full `PATH`/env. For the native Ghostty adapter the mechanism is concrete: Ghostty's `environment variables` property. For the config-driven path the spec asserts the *same guarantee* but describes a *different* mechanism — "the picker resolves what the spawn needs and threads it into `{command}` itself" — without ever saying how env is threaded into a command.

This is a real seam gap, not a wording nit. A config recipe's only substitution slot is `{command}` (= `<os.Executable()> attach <session>` + ack token). A `terminals.json` recipe such as the shipped Warp example (`tell app "Warp" to create window with command "{command}"`) has **no env slot** — the recipe just runs `{command}` in whatever bare environment that terminal provides. If `{command}` is a plain argv (`/abs/portal attach sess …`), the spawned process inherits the terminal's bare `PATH` and `tmux` fails to resolve — exactly the bug the native path fixes. The only way "thread into `{command}`" can work for config is if Portal wraps `{command}` as an env-carrying command (e.g. `/usr/bin/env PATH=<full> … /abs/portal attach sess …`), but the spec never states this, and if it did, it would then be inconsistent with the native adapter (which is said to use the terminal property, not an env-wrapped command). A planner cannot implement the config path correctly without knowing which mechanism config uses, and whether the composed `command` handed to `OpenWindow` is env-agnostic (adapter injects) or env-self-sufficient (command carries its own env).

**Proposed Addition**:
Specify a single, uniform env-delivery mechanism and state how each path realises it. Recommended: define the composed command as **env-self-sufficient** — Portal builds `{command}` as an env-prefixed argv (`env PATH=<picker PATH> [other required vars] <os.Executable()> attach <session> <ack>`) so it resolves regardless of the spawn environment. Then state that the native Ghostty adapter MAY additionally use the terminal's env property but is not required to, and that config recipes need do nothing — running `{command}` verbatim is sufficient. Alternatively, if env must be delivered per-adapter, add an explicit env field to the recipe schema and define its substitution. Either way, pin one answer.

**Resolution**: Approved
**Notes**: Approved via auto. **Judgment call (architecture):** picked the env-self-sufficient composed command — Portal builds `{command}` as an `env PATH=… <abs>/portal attach … --spawn-ack <batch>` argv, uniform across native + config, so no adapter env property and no recipe env slot. Reconciled the Spawn Architecture PATH-injection paragraph (superseded the per-adapter "Ghostty environment variables property" framing) and the Config recipe-execution bullet.

---

### 2. `portal spawn <sessions…>` CLI window-reuse / self-attach semantics are undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Spawn Architecture (Model: one service, two callers; The N vs N−1 split), Naming, Testing Strategy (the CLI as test seam)

**Details**:
The "net N windows, never N+1" anti-requirement and the self-attach-last flow are specified **only for the picker**: "the picker turns its own host window into one of the N" via `AttachConnector`/`SwitchConnector`. The self-attach reuse is explicitly a picker concern layered on top of the spawn package (the spawn package only detects + resolves adapter + opens windows).

But `portal spawn <sessions…>` is a shipped surface (Naming section: "Ships as `portal spawn <sessions…>`") and a caller of the same package. Its window behaviour is unspecified: given `portal spawn foo bar baz` run from a shell, does the CLI (a) mirror net-N and self-attach the **calling terminal** into one session (exec `tmux attach` in-place, replacing the shell) and spawn N−1, or (b) spawn all N as fresh host windows and leave the calling shell intact (net N+1 surfaces)? These are materially different behaviours with different ack/self-attach flows. This also matters because the CLI is designated "the test seam" — tests must exercise the same net-N + ack + self-attach-last flow the picker uses, which is impossible to pin without defining the CLI's reuse behaviour. It further matters for the deferred workspace-restore follow-on, which is said to reuse this CLI entry point.

**Proposed Addition**:
State the CLI's window model explicitly. Recommended for parity with the picker (and to keep the "net N" invariant uniform): `portal spawn <sessions…>` reuses its **calling terminal window** as one of the N (self-attach-last via the same connectors) and spawns the N−1 others, running the identical pre-flight → spawn → ack → self-attach flow. If instead the CLI is intended to spawn all N (no reuse), say so and explain how the net-N invariant is reconciled (or scoped out) for the CLI path. Note whether `portal spawn` with zero session args is `--detect`-only / an error.

**Resolution**: Approved
**Notes**: Approved via auto. **Judgment call:** CLI mirrors the picker (net-N: reuse calling window as one, spawn N−1, same pre-flight→spawn→ack→self-attach). Added a "`portal spawn` CLI behaviour" subsection; `--detect` = dry-run prints identity; no args + no `--detect` = usage error.

---

### 3. In-process spawn-burst execution model, cancellation post-state, and in-burst UI feedback are unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Burst & Partial-Failure Contract (Sequential spawn, Cancellation), Multi-Select Mode, Design References

**Details**:
The headline scenario is a *large* burst (~14 windows). The spec specifies ordering (spawn N−1, collect acks, self-attach last), per-window ~8s ack timeouts, and sequential spawn — but not **how this runs inside Bubble Tea**, which is a load-bearing design decision the implementer cannot avoid:

- **Execution model.** The picker calls the spawn package "in-process." A multi-second sequential burst plus per-window ack waits (up to ~8s each) cannot run as a blocking `Update` handler without freezing the TUI and defeating the stated cancellation points. It almost certainly needs a `tea.Cmd`/goroutine + progress-message model (as the cold-path bootstrap already uses), but the spec never says so. Whether spawn is sync-blocking or async-with-progress determines cancellation, responsiveness, and how acks/timeouts are collected.
- **Cancellation post-state.** "`Ctrl-C`/`Esc` before [self-exec] aborts the remaining spawns and leaves any already-opened windows in place." Undefined: does `Ctrl-C` quit Portal entirely (Bubble Tea's usual meaning) or return to the picker? After cancel, what is the selection/mode state — still in multi-select, which sessions remain marked (the partial-failure path has an explicit "unmark opened / keep rest" rule; cancel has none)? Are batch markers cleaned on cancel (cleanup is defined only for self-exec / pre-flight abort / reported failure)?
- **In-burst UI feedback.** While the N−1 spawn and the picker waits for all acks before self-attaching, the user may sit in the picker for up to ~8s (a lagging or failing ack). No progress/pending visual is specified, and the delivered Paper design set has only three frames (active, pre-flight abort, unsupported) — none for "spawning / awaiting acks."

**Proposed Addition**:
Specify (a) that the burst runs as an async `tea.Cmd` streaming progress/ack messages back to the model (not a blocking Update), consistent with the cold-path bootstrap pattern; (b) the exact cancel semantics — whether `Ctrl-C`/`Esc` mid-burst returns to the picker or quits, the resulting mode/selection state (recommend: same "leave opened, keep un-opened marked, stay in mode" rule as partial-failure), and whether markers are cleaned; and (c) the in-burst UI state — either a pending/progress affordance in the notice band while awaiting acks, or an explicit statement that self-exec/exit happens fast enough that no in-burst frame is needed (and confirm no design frame is required).

**Resolution**: Approved
**Notes**: Approved via auto. **Judgment call (design):** added an "In-picker execution model" subsection — (a) async `tea.Cmd` streaming progress/ack msgs (cold-path bootstrap pattern); (b) `Ctrl-C`/`Esc` returns to picker (not quit), aborts remaining, leaves opened, self-cleans markers, same unmark-opened/keep-rest selection rule; (c) `Opening n/N…` notice-band affordance, with the "spawning/awaiting acks" Paper frame flagged as a design-phase residual for the visual gate.

---

### 4. Ack-token delivery to `portal attach` and the marker-write contract are under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Burst & Partial-Failure Contract (Confirmation mechanism, Ack channel), Spawn Architecture (Command composition)

**Details**:
The ack mechanism requires modifying the **existing** `portal attach` command (`cmd/attach.go`) — a component outside `internal/spawn` — to write `@portal-spawn-<batch>-<session>` right before exec. The spec says the picker "threads it into each spawned command (arg/env)" and the spawned attach "writes its token right before exec," but leaves several contract points open that a planner splitting picker-side and attach-side work needs pinned:

- **Arg vs env** is left as "arg/env." This is not cosmetic: it interacts with Finding #1. If the batch/ack info is a positional **arg** it is part of the composed `{command}` string and flows through config recipes automatically; if it is an **env var** it must ride the same env-injection path config recipes lack a slot for.
- **What `portal attach` must receive** to name the option: it already knows `<session>`, but must be told the `<batch>` id and that it is in spawn-ack mode. The flag/env name and shape are unspecified.
- **Failure/ordering of the write.** "Right before exec" implies: abridged bootstrap → resolve/verify session → write marker → exec. Is the marker write best-effort (attach still execs if the write fails)? What if the session doesn't exist at attach time (vanished post-pre-flight) — attach fails before writing, picker times out, window classified failed? This is derivable but should be stated as the explicit contract since it defines the "honest boundary" the ack depends on.

**Proposed Addition**:
Define the `portal attach` spawn-ack contract: the concrete carrier (recommend a flag, e.g. `--spawn-ack <batch>`, so it flows through `{command}`/config recipes uniformly), that attach writes `@portal-spawn-<batch>-<session>` immediately before the exec handoff and after the session is confirmed to exist, that the write is best-effort-but-attach-still-execs, and that a session that fails to resolve produces no marker (→ picker timeout → failed classification). State the marker value semantics (presence is the signal; value arbitrary).

**Resolution**: Approved
**Notes**: Approved via auto. **Judgment call:** carrier is a flag `--spawn-ack <batch>` (flows through `{command}`/config recipes as argv, resolving arg-vs-env in favour of arg, dovetailing F1); attach confirms session exists → writes `@portal-spawn-<batch>-<session>` (presence = signal, value opaque) as last step before exec; best-effort (attach still execs; failed write → timeout → failed); unresolved session → no marker → failed. Added "Ack delivery & `portal attach` contract" subsection + updated the token-ack bullet.

---

### 5. `permission-required` handling within a multi-window burst is undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Permissions & Error Quarantine (Defensive net), Burst & Partial-Failure Contract (Stance)

**Details**:
The typed-result taxonomy has three categories: `permission-required`, `unsupported`, `spawn-failed`. The Burst contract's leave-what-opened rule is framed only around `spawn-failed` ("a transient `osascript`/terminal hiccup"). It is unspecified how a `permission-required` result returned by an adapter *mid-burst* is handled: is it treated identically to `spawn-failed` (that window failed → skip self-attach, leave others, keep it marked), or does it surface the distinct actionable guidance (name terminal, offer Automation settings deep-link) the Permissions section promises — and if the latter, does it do so per failing window or once for the batch? TCC is self-exempt in the normal flow, so this is genuinely rare, but the category exists in scope and its burst-time behaviour has no defined answer.

**Proposed Addition**:
State that within a burst a `permission-required` result is treated as a failed window for the leave-what-opened accounting (skip self-attach, leave opened windows, keep the affected session marked), AND additionally surfaces the permission guidance once for the batch (naming the target terminal), rather than the generic spawn-failed one-line error. Or, if simpler, fold `permission-required` into the same one-line failure report and rely on the standalone `--detect`/single-spawn path to surface guidance.

**Resolution**: Approved
**Notes**: Approved via auto. Chose: within a burst, `permission-required` is accounted as a failed window AND stops the burst (sequential + per-(source,target) grant → all later windows hit the same wall), surfacing the permission guidance once for the batch (not the generic error); grant persists so retry proceeds. Appended to Permissions → Defensive net.

---

### 6. Detection lifecycle is unspecified — timing, caching, and non-NULL error handling

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Terminal Identity & Detection, Multi-Select Mode

**Details**:
"Detection runs on Sessions-page entry" leaves the operational lifecycle open:

- **Caching.** The host terminal identity is invariant for the picker's lifetime, but the banner is rendered through `rebuildSessionList`, the chokepoint hit on many transitions (`s`-toggle, `SessionsMsg` refresh, projects-edit return, filter). Without a stated "detect once, cache for the picker session" rule, a naive implementation could re-walk the process tree / re-`defaults read` on every rebuild. The N≥2 Enter path also "(re)asserts" the banner — reuse the cached identity or re-detect?
- **Sync vs async.** The walk (`ps`, process-tree, Info.plist `defaults read`) can take tens of ms; the spec doesn't say whether it runs synchronously on page entry (risking first-paint stall, given the ~50ms appearance-gate budget) or async.
- **Error vs NULL.** The clean NULL case (remote/mosh → unsupported) is defined, and "detection returns empty → same NULL path." But a *transient error* in the walk (e.g. `ps`/`defaults read` fails) is distinct from a clean NULL — is it also folded to unsupported, with a `spawn` WARN?

**Proposed Addition**:
State that detection runs once per picker session (cached; re-used by the on-entry banner and by the N≥2 Enter gate), that it runs off the first-paint critical path (or is cheap enough to run inline — pick one), and that any detection error (as opposed to a clean remote/mosh NULL) also resolves to the unsupported/no-op path with a `spawn`-component WARN breadcrumb.

**Resolution**: Approved
**Notes**: Approved via auto. Added a "Detection lifecycle" subsection: detect once per picker session on Sessions-page entry (cached; `rebuildSessionList` must not re-walk; reused by banner + N≥2 gate); async off the first-paint path; transient detection error folds to unsupported/no-op with a `spawn` WARN.

---

### 7. New `spawn` log attr keys are not enumerated despite the closed attr-key vocabulary

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Observability & State Footprint

**Details**:
The project's logging taxonomy is closed: new components AND new attr keys "require amending the spec — never invent at call-site." The spec correctly adds the `spawn` component as a deliberate amendment and lists the event catalog, and it names one attr (`detail`). But the events clearly need additional attr keys not enumerated (e.g. batch id, window counts for `opened 11/14`, detected identity/bundle-id, terminal/app name, ack outcome, resolution source config-vs-native). Under the project's own rule these must be spec-governed, not invented at the call site, so leaving them unlisted is a genuine (if small) gap that an implementer would otherwise fill ad hoc.

**Proposed Addition**:
Enumerate the closed attr-key set the `spawn` component introduces (e.g. `batch`, `opened`, `total`, `terminal`, `bundle_id`, `resolution` = config|native, `ack` = confirmed|timeout|failed, plus the existing opaque `detail`), consistent with how bootstrap/restore/daemon components enumerate theirs.

**Resolution**: Approved
**Notes**: Approved via auto. Enumerated the closed `spawn` attr-key set in Observability: `batch`, `terminal`, `bundle_id`, `resolution` (config|native|unsupported), `session`, `ack` (confirmed|timeout|failed), `opened`/`total`, `detail`.

---

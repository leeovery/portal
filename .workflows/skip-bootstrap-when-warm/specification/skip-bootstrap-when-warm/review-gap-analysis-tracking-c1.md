---
status: complete
created: 2026-07-02
cycle: 1
phase: Gap Analysis
topic: skip-bootstrap-when-warm
---

<!--
All 13 findings processed under finding_gate_mode: auto (approved/adjusted),
each resolution grounded in the real codebase (cmd/root.go, cmd/open.go,
cmd/bootstrap/bootstrap.go, cmd/state_daemon.go, internal/tui/loading_progress.go).
Resolution summary:
 1  Approved — added "Control-flow sequencing (single read, computed once)" subsection.
 2  Approved — folded into #1 (single read, threaded, not read twice).
 3  Adjusted — CORRECTED spec: cleanup runs ON the idle branch, not skipped by it (added Placement bullet).
 4  Approved — lastCleanup initialised to daemon-start → ~10s first cleanup; test made deterministic.
 5  Adjusted — pinned v1 value to bare cmd.version (exact equality); forensic extras out of scope.
 6  Approved — added "abridged must not set deferredBootstrapKey" precondition.
 7  Approved — folded into #9 (latch-write WARN is a pure log line, not a warning/channel event).
 8  Approved — added totalSteps constant + package doc + loading_progress.go stepLabelTable to Affected Code Surface.
 9  Approved — added "Insertion point in Run" subsection (after fatal gate, before summary/terminal event).
 10 Approved — pinned abridged EnsureSaver as a new liveness-only helper in package cmd.
 11 Approved — added flapping-daemon starvation to Accepted residues (inert, tolerated).
 12 Approved — noted serverStarted force-true stays correct (sole consumer is the loading-page gate).
 13 Approved — added "Out of scope: no portal-level unset command" note.
-->


# Review Tracking: skip-bootstrap-when-warm - Gap Analysis

## Findings

### 1. Abridged path's interaction with the concurrent/loading route is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Latch-Check Placement & Abridged-Path Wiring", "Loading-screen trigger", "Affected Code Surface → Entry path"

**Details**:
The spec re-keys `shouldRunConcurrentBootstrap` to fire on **latch-not-satisfied** on the TUI path, and separately says a **satisfied** latch on `open` (no args) takes the "abridged (sync plumbing, instant picker)" route. But it never states *how these two decisions are sequenced in `PersistentPreRunE`*. Today the code reads: `if shouldRunConcurrentBootstrap(...) { defer to openTUI; return nil }` else run sync. With the latch added there are now three outcomes on the `open`-no-args path (concurrent-full, abridged, and the non-TUI full-sync path) but the spec gives two independent predicates (`shouldRunConcurrentBootstrap` re-keyed to latch-not-satisfied, and "latch satisfied → abridged"). An implementer must decide the evaluation order and how the abridged branch coexists with the deferred-bootstrap branch: does the latch read happen *before* `shouldRunConcurrentBootstrap`, short-circuiting to abridged? Or does `shouldRunConcurrentBootstrap` itself now embed the latch read (replacing its `ServerRunning()` probe), leaving abridged as its `false` fallthrough? The current `shouldRunConcurrentBootstrap` uses a `ServerRunning()` probe; the spec says "a separate `ServerRunning()` probe is not required." Whether the latch read *replaces* that probe inside `shouldRunConcurrentBootstrap`, or is a new read *upstream* of it, changes the control flow and is left to guess. This is the single most load-bearing wiring decision in the feature and it is under-specified.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 2. Number of latch reads per command is ambiguous (single-read claim vs. two consumers)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Semantics — satisfied", "Latch-Check Placement", "Loading-screen trigger"

**Details**:
The spec repeatedly stresses "Single read chosen for minimalism" and "A single `TryGetServerOption` read drives a three-way outcome." Yet two distinct consumers each need the satisfied/not-satisfied verdict: (a) the `PersistentPreRunE` abridged-vs-full branch, and (b) `shouldRunConcurrentBootstrap`'s re-keyed trigger ("keyed off latch-not-satisfied, not server-down"). If these are evaluated at different points, that is two reads unless the result is computed once and threaded. The spec asserts a single read but does not state where the verdict is computed nor how it is shared between the branch decision and the concurrent-route decision. An implementer could reasonably read the latch twice (once per predicate), contradicting the stated single-read intent, or could be unsure whether to introduce a new context value / helper to carry the verdict. Clarify: one read, verdict computed once, threaded to both decisions.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 3. Daemon cleanup-gate placement relative to the restoring/idle skip is under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Daemon-Owned Hooks Cleanup → Cadence / Priority / non-interference", "Affected Code Surface → Daemon"

**Details**:
The spec says the throttled hooks-cleanup gate is "skipped while `@portal-restoring` is set and on the `!dirty && !gap` idle fast-path." Reading `cmd/state_daemon.go`, the tick body returns early on both conditions: `@portal-restoring` set → return, and `!dirty && !gap` → return (the idle fast path). If cleanup is placed *after* those early returns, it will **only run on ticks where a capture is also happening** (dirty or gap), i.e. never on an idle warm server — precisely when stale hooks accrue and the daemon is otherwise idle. That contradicts the goal of reclaiming hook cruft over a weeks-long lifetime on a mostly-idle server. If instead cleanup should run on idle ticks too (just not during restoring), it must be placed *between* the restoring check and the idle fast-path return — but then it is no longer "skipped on the `!dirty && !gap` idle fast-path" as the spec states. The spec's two placement constraints are in tension and the resulting behavior (does cleanup ever fire on a purely idle server?) is left ambiguous. This directly affects whether the feature achieves its stated weeks-long-lifetime cleanup goal.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 4. `lastCleanup` initial value / first-cleanup timing is unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Daemon-Owned Hooks Cleanup → Cadence", "Test Strategy → Daemon cleanup"

**Details**:
The cadence gate is `time.Since(lastCleanup) >= interval`. The spec does not state the initial value of `lastCleanup` at daemon start. If it initializes to the zero `time.Time`, `time.Since(zero)` is enormous, so the first eligible tick fires cleanup immediately (~1s after start) — the spec's "at cold boot the freshly-started daemon cleans on its first eligible tick (~10s)" implies a ~10s delay, not ~1s. If instead `lastCleanup` initializes to `time.Now()` at daemon start, the first cleanup is deferred a full interval (~10s), matching the "~10s" claim. These two initializations give materially different first-cleanup timing (1s vs 10s), and the "~10s" prose and the "cleans on its first eligible tick" prose point in opposite directions. The unit test for the cadence gate (Test Strategy) can't be written deterministically without pinning this. Specify the initial value and the intended first-cleanup delay.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 5. Latch value format is left as "implementation detail" but consumed by an equality check

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "The Version-Stamped Latch → Storage / Why version-stamped", "Semantics — satisfied"

**Details**:
The satisfied test is "stored version **equals** the running binary's `cmd.version`" — a string equality on the stored value. But the spec also says "The stored value also serves forensics ... optional cheap additions (set-timestamp, pid) are an implementation detail." If the stored value is *just* the version string, equality is straightforward. If it carries additional fields (timestamp/pid), then the equality check must parse/extract the version component, and the storage format (delimiter, ordering) becomes load-bearing — an implementer cannot both "store version + pid + timestamp" *and* do a naive `stored == cmd.version` comparison. The spec leaves the value format open while simultaneously specifying an exact-equality read, which is internally inconsistent for anything beyond a bare version. Either pin the value to exactly `cmd.version` (making forensics extras out of scope for v1), or specify the parse/compare rule. As written, an implementer must guess.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 6. `serverStarted=false` on abridged path vs. concurrent-route's forced-true — no conflict rule stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Abridged wiring reuses the sync plumbing → Context injection"

**Details**:
The spec says the abridged path injects `serverStarted=false`, and its sole consumer is `openTUI`'s loading-page gate → `false` → instant picker. But `openTUI` today unconditionally overrides `serverStarted=true` when it detects a deferred bootstrap on the context (`if deferred := ...; deferred != nil { serverStarted = true }`). The abridged path injects no deferred bootstrap, so the override won't trigger — the spec's reasoning holds — but this coupling is implicit. An implementer wiring the abridged `open`-no-args path must ensure it does **not** stash a `deferredBootstrapKey` (which would force `serverStarted=true` and wrongly show the loading page). The spec asserts the correct outcome ("instant picker") without naming the load-bearing precondition (abridged path must not set the deferred-bootstrap context key). Worth stating explicitly so the abridged wiring doesn't accidentally inherit the deferred-bootstrap plumbing.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 7. Latch-set failure inside the concurrent (goroutine) path — surfacing unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Latch Set-Point & Timing → Write posture", "Abridged wiring reuses the sync plumbing → Warnings"

**Details**:
The latch is set "inside `Run`, so ... the concurrent cold+TUI goroutine ... get[s] it identically." The write is best-effort: "on failure, log WARN and swallow." On the synchronous path a WARN log is straightforward. On the concurrent path, `Run` executes in a goroutine streaming `tea.Msg` progress; the spec does not say whether a latch-write WARN is (a) purely a log line (never surfaced to the user), or (b) routed through the progress channel / warning sink like other soft warnings. The spec is explicit that `SaverDownWarning` rides the channel/sink, but silent on the latch-write failure. Since it "self-heals" (next command re-runs the near-no-op full bootstrap), a pure log line is defensible — but the spec should say so, because "log WARN" in a goroutine with a live progress channel is otherwise ambiguous about which log/emission mechanism applies. Minor but affects the daemon/TUI observability wiring.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 8. `totalSteps` constant / orchestration-summary step count not called out in the 11→10 change

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Scope", "The Two Bootstrap Paths", "Affected Code Surface → Orchestrator"

**Details**:
The spec repeatedly states the orchestrator drops 11 → 10 steps by removing `CleanStale`. `cmd/bootstrap/bootstrap.go` carries a `totalSteps = 11` constant described in-source as "the single source of truth for the [orchestration-complete] summary line" and "a load-bearing contract." Removing a step requires updating this constant (and the package doc comment enumerating the 11 steps) or the `bootstrap: orchestration complete` audit line will report a stale step count. The Affected Code Surface section names the `Run` signature and the `CleanStale` step/adapter removal but does not flag `totalSteps` or the enumerated package doc comment. Also, the concurrent path maps real steps to "5 friendly labels" (`internal/tui/loading_progress.go`) and only `Restoring sessions` carries `N/M` — removing `CleanStale` may shift the real-step→label mapping count. An implementer working only from this spec could miss the `totalSteps` constant and the loading-progress mapping. Add them to the affected surface.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 9. "Latch set last, after all steps" vs. the existing progress-emission / return boundary — insertion point relative to warnings collection unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Latch Set-Point & Timing → Decision", "Affected Code Surface → Orchestrator"

**Details**:
The decision is to set the latch "as the final action of a *successful* `Run` — after the last step, gated on no fatal error." `Run` today has a defined post-step "Return" boundary that "collects accumulated warnings" and (on the concurrent path) emits an orchestration-complete summary. The spec does not specify whether the latch-set happens *before* or *after* the warnings are collected / the completion summary is emitted, nor whether the best-effort latch-write WARN should itself be one of the accumulated warnings. Since a soft-warning run still latches, the latch-write sits after the fatal-error gate but its exact position relative to warning accumulation and the summary emission is left to guess. This matters for ordering the `BootstrapCompleteMsg` (concurrent path) vs. the latch write — should the picker transition wait for the latch write, or can the latch write trail the completion signal? Specify the insertion point precisely.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 10. Abridged EnsureSaver — reuse of the existing step vs. a new liveness-only entry point is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Abridged EnsureSaver — Liveness-Only", "The two paths"

**Details**:
The abridged path runs "EnsureSaver liveness probe only" = "`SaverPanePIDOrAbsent` + re-ensure if absent" via `BootstrapPortalSaver`, explicitly *not* the version-gate (`EnsurePortalSaverVersion`). The full-bootstrap EnsureSaver step keeps both duties. The spec does not state whether the abridged path (a) invokes the *existing* orchestrator EnsureSaver step in a liveness-only mode, (b) calls a *new* helper that composes `SaverPanePIDOrAbsent` + `BootstrapPortalSaver` directly, or (c) reuses some existing liveness-only function. Since the abridged path explicitly does **not** run through the orchestrator ("it just doesn't run the orchestrator"), option (a) seems excluded — implying a new call site composing the tmux/state primitives. But the spec never says the abridged EnsureSaver is a new function nor names where it lives (cmd? a shared helper?). An implementer must design this composition themselves, including where its `SaverDownWarning` is constructed and fed to the sink. This is a concrete build decision left open.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 11. Interaction between abridged liveness EnsureSaver and the daemon-owned cleanup revival ordering is unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Two independent daemon safety nets", "Daemon-Owned Hooks Cleanup → Failure posture", "Edge Cases → Daemon-death vs cleanup home"

**Details**:
Hooks cleanup is now homed *only* on the daemon (plus manual `portal clean`). The daemon is revived by the abridged liveness EnsureSaver. But a freshly-revived daemon resets its in-process `lastCleanup` (see finding 4) and its self-supervision counter. If a daemon repeatedly self-ejects and is repeatedly revived within short windows (e.g. transient split-brain flapping), the cleanup interval may never elapse before the next eject — hooks cleanup could be starved indefinitely even though the daemon is "alive" per liveness probes. The spec's "worst case leaves only inert hooks bloat until the daemon next revives" reasoning covers a *dead* daemon, but not a *flapping* one where each short-lived incarnation never survives one full cleanup interval. This is an edge case within the specified scope (daemon liveness + throttled cleanup interaction) that the spec's accepted-residues analysis does not address. Likely still acceptable (bloat is inert), but the spec should acknowledge it rather than leave the interaction unexamined, since it is a direct consequence of two decisions this feature makes together.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 12. Concurrent-route re-key: warm-unlatched TUI now runs the loading path with `serverStarted=false` — but openTUI forces `serverStarted=true`; correctness for a warm server unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Loading-screen trigger: latch-absent, not server-down"

**Details**:
The spec retires the "warm-unlatched edge" by firing the loading screen on latch-not-satisfied even when the server is *already running* (hand-started tmux + `x`). On this path the orchestrator is deferred to `openTUI`, which today forces `serverStarted=true` whenever a deferred bootstrap is present ("Cold by construction ... force serverStarted"). But on a warm-unlatched server the server was **not** started by this command — `serverStarted` should be `false`. The forced-true is documented as "definitional" only for the *cold* route. Extending the deferred/loading route to warm-unlatched breaks that invariant: the deferred bootstrap will now sometimes run against an already-running server, making the `serverStarted=true` force incorrect. The spec says "What the full bootstrap does is unchanged — only the presentation improves," but does not address that `serverStarted` (which flows into downstream context and is a real return value of `Run`) would now be forced true on a warm server via the openTUI override. Whether `serverStarted` matters for any consumer on this warm-unlatched-TUI path, and whether openTUI's force-true needs to become conditional, is unspecified. This is a concrete correctness question the re-key introduces.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

### 13. Manual escape-hatch command name (`@portal-bootstrapped`) vs. no `portal`-level unset command — user-facing surface unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Dev-build nuance", "Edge Cases → Manual escape hatch"

**Details**:
The escape hatch is documented as the raw `tmux set-option -u @portal-bootstrapped`. The spec says "production code never needs to unset it" and exposes `UnsetServerOption` in the reuse list, but does not decide whether a `portal`-level affordance (e.g. surfaced in `portal clean` or a documented command) is in or out of scope. Since `UnsetServerOption` is listed as a reused mechanism yet "production code never needs to unset it," the only unset caller is the human via raw tmux. This is internally consistent but leaves a small planning gap: is any documentation/help-text surface expected, or is the raw tmux command the entire user-facing contract? Minor — flag for an explicit in/out-of-scope note so planning doesn't invent a command.

**Proposed Addition**:

**Resolution**: Approved
**Notes**:

---

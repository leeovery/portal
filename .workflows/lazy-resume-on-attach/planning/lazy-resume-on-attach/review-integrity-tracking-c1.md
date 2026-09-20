# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. A restored pane crashes instead of restoring when the helper has resolved no decision

**Severity**: Important
**Plan Reference**: Phase 4, task lazy-resume-on-attach-4-5 (The helper decides, marks and hands the pane to the panel)
**Category**: Task Self-Containment / Dependencies and Ordering
**Move**: settled
**Change Type**: update-task

**Problem**:
`execShellOrHookAndExit` is the last thing that runs in every restored pane. The task re-points it at `cfg.Decision`, a pointer field, without saying what happens when that pointer is nil — and it is nil on every path that does not come through `runHydrate`. Eight existing suites call the function directly (`cmd/state_hydrate_exec_log_test.go` ×7, `cmd/hooks_read_lock_test.go` ×1), all building a `hydrateConfig` through `hydrateCfg` or a literal, so after this change every one of them dereferences nil and panics. The task's own Do step requires those suites to run unchanged, so the executor is handed two instructions that cannot both hold and has to invent the resolution — either nil-tolerance nobody specified, or a rewrite of eight call sites the task never mentions. The wrong pick is the one that removes the fall-back path: a function that assumes a decision was always resolved is a nil dereference in production the day anything else calls it.

**Proposal**:
Make the nil decision the specification rather than an accident: a `Decision` of nil means "nobody resolved one", and the function does what it does today — its own lookup, its own records, its own exec. That is determined by the task's own acceptance requirement that the existing hydrate suites and `TestExitClosesRestoredPane_*` pass unmodified, and it is the same shape every other optional seam in `cmd` already takes. The non-nil path then reads off the decision and skips the second store read, which is what the task set out to buy.

**Current**:
```
- Branch in `execShellOrHookAndExit`: when `cfg.Decision.Wait`, compose and exec the chain; otherwise take today's path, reading the command off `cfg.Decision.Lookup` rather than issuing a second store read.
```

**Proposed Text**:

Replace that **Do** bullet with:

```
- Branch in `execShellOrHookAndExit` on a **nil-tolerant** `cfg.Decision`. A nil decision is today's behaviour unchanged: the function performs its own `LookupOnResume`, emits its own `hook lookup` DEBUG records and its own lookup-failure WARN, and execs the hook or the bare shell exactly as it does now — so the eight direct callers in `cmd/state_hydrate_exec_log_test.go` and `cmd/hooks_read_lock_test.go` compile and pass unmodified. A non-nil decision skips that read: when `cfg.Decision.Wait` it composes and execs the chain, otherwise it takes today's path reading the command off `cfg.Decision.Lookup`.
```

Add to **Acceptance Criteria**:

```
- [ ] `execShellOrHookAndExit` called with a nil `Decision` is byte-identical to today: its own lookup, the same `hook lookup` hit/miss/error DEBUG records, the same lookup-failure WARN and the same exec — the eight existing direct callers pass with no edit.
```

Add to **Tests**:

```
- `"it falls back to its own lookup when no decision was resolved"` (nil `Decision`; records and exec unchanged)
```

Add to **Edge Cases**:

```
- A nil `Decision` is today's behaviour rather than a panic. `execShellOrHookAndExit` is reached directly by eight existing suites that build a `hydrateConfig` without one, and resolving the decision is `runHydrate`'s job — a call that arrives without one performs the lookup itself, which is exactly what the function does today. Making the field's absence mean "nobody decided" is what keeps the new branch beside the existing behaviour rather than inside it.
```

**Resolution**: Pending
**Notes**:

---

### 2. The waiter's own source guard fails the moment the resize redraw lands

**Severity**: Important
**Plan Reference**: Phase 4, tasks lazy-resume-on-attach-4-2 (The waiting process) and lazy-resume-on-attach-4-6 (One redraw once the size has settled)
**Category**: Acceptance Criteria Quality / Phase Structure
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 4.2 pins "starts no timer" as an acceptance criterion and a source guard over the wait path. Task 4.6, four tasks later in the same phase, arms `time.After` on every SIGWINCH and registers `signal.Notify`. The guard task 4.2 tells the executor to write is therefore red by the end of the phase, and task 4.6 gives no instruction to amend it — so the phase ends with a failing test and the executor deciding on the spot whether to delete the guard (losing the property it exists to hold: that the waiter never kills itself and never expires a report on a clock) or to weaken it into something that asserts nothing. The property is real and worth keeping; only its wording is wrong, because "no timer" was written for a screen that had one meaning of timer and is read by a task that has another.

**Proposal**:
Scope the rule to what it is actually protecting — no hangup/terminate/interrupt handler, and no timer whose firing ends the wait or clears a report — and say in 4.2 that the settle timer is the one timer the wait path arms, so the executor writes a guard that survives 4.6 rather than one 4.6 quietly breaks. Task 4.6 then re-runs it unchanged, which is what it already does for 4.2's swallow and restore tables.

**Current**:

Task 4.2, **Do**:
```
- Install no handler for SIGHUP, SIGTERM or SIGINT and start no timer: the default disposition must continue to end the process when tmux tears the pane down, and a reported reason must stand until a key is pressed.
```

Task 4.2, **Acceptance Criteria**:
```
- [ ] The command registers no signal handler for SIGHUP, SIGTERM or SIGINT, and starts no timer — asserted by source inspection of the wait path alongside the behavioural tests.
```

Task 4.2, **Tests**:
```
- `"it installs no signal handler and starts no timer"` (source guard over the wait path)
```

Task 4.2, **Edge Cases**:
```
- The waiter starts no timer of its own so a reported reason stands until a key is pressed — a report the user can miss leaves them believing the thing they asked for happened.
```

**Proposed Text**:

Task 4.2, **Do** — replace that bullet with:

```
- Install no handler for SIGHUP, SIGTERM or SIGINT, and arm no timer whose firing ends the wait or clears a report: the default disposition must continue to end the process when tmux tears the pane down, and a reported reason must stand until a key is pressed. Task 4.6 arms a settle timer for the resize redraw — it neither ends the wait nor expires a report — so write the guard below against those three signals and against the wait's own exit and report paths, not against the presence of a timer as such.
```

Task 4.2, **Acceptance Criteria** — replace that criterion with:

```
- [ ] The command registers no handler for SIGHUP, SIGTERM or SIGINT, and arms no timer whose firing ends the wait or clears a report — asserted by a source guard over the wait path scoped to those three signals and to the wait's exit and report paths, so task 4.6's resize-settle timer leaves it green.
```

Task 4.2, **Tests** — replace that test with:

```
- `"it installs no hangup, terminate or interrupt handler and arms no wait-ending or report-expiring timer"` (source guard over the wait path)
```

Task 4.2, **Edge Cases** — replace that edge case with:

```
- The waiter arms no timer of its own that ends the wait or expires a report, so a reported reason stands until a key is pressed — a report the user can miss leaves them believing the thing they asked for happened. The resize settle window task 4.6 adds is the one timer the wait path arms, and all it decides is whether to hand over to a redraw; it never ends the wait and never clears the report, which rides the redraw forward.
```

Task 4.6, **Acceptance Criteria** — add:

```
- [ ] Task 4.2's signal-and-timer source guard passes unchanged with `Winch` and `Settle` in place: the SIGWINCH notify does not widen to SIGHUP, SIGTERM or SIGINT, and the settle timer neither ends the wait nor clears the report.
```

**Resolution**: Pending
**Notes**:

---

### 3. The panel's own theme probe eats the keystroke the plan promises will survive

**Severity**: Important
**Plan Reference**: Phase 4, task lazy-resume-on-attach-4-1 (The panel-drawing process); Phase 5, task lazy-resume-on-attach-5-4 (The confirmation refuses input that was already in flight)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 3.5's appearance probe reads the pane's stdin — it writes the background-colour query and then reads until a terminator or the detect timeout, discarding everything that is not the reply. The draw runs that probe on every redraw under an adaptive theme pair, which means the chain reads and throws away tty input in a place no task records. Two statements in the plan are wrong as a result. Task 5.4 asserts "the draw performs no read of its own", which an executor will read as a property to test and cannot make true. And task 4.2 promises that bytes left unread after an answer are inherited by whatever the handover execs, which stops being true on the redraw path: a key pressed during a resize or a report redraw is swallowed instead of acted on. Nobody loses work to this — the probe can only reach input that arrived before any screen was painted, which is the direction the confirmation's own drop rule already insists on — but left unstated it is either a property an executor fails to deliver or a "fix" someone attempts by moving the probe.

**Proposal**:
State it where it happens. The probe is the one read the draw performs, it runs after the input drop and before the alternate-screen entry, and its window is `appearanceDetectTimeout` — so record that as task 4.1's edge case and criterion, and narrow task 5.4's criterion to the claim it was really making, that the drop discards the queue rather than reading it. Nothing moves in the code: the ordering the plan already specifies is the safe one, and the constant nomination and `NO_COLOR` paths read nothing at all.

**Current**:

Task 5.4, **Acceptance Criteria**:
```
- [ ] Nothing on the drop path reads stdin: the seam returns an error and no bytes, and the draw performs no read of its own.
```

Task 5.4, **Tests**:
```
- `"it reads no byte on the drop path"`
```

**Proposed Text**:

Task 5.4, **Acceptance Criteria** — replace that criterion with:

```
- [ ] The drop discards the queue rather than reading it: the seam consumes no byte and returns only an error, and nothing between the drop and the paint reads stdin except the appearance probe, which runs after the drop and is bounded by the detect timeout.
```

Task 5.4, **Tests** — replace that test with:

```
- `"it consumes no byte on the drop path"` (the seam returns an error and no bytes; the draw's only other stdin read is the injected theme resolver's)
```

Task 4.1, **Acceptance Criteria** — add:

```
- [ ] The appearance probe is the only stdin read the draw performs, and it runs after any input drop and before the alternate-screen entry — so no byte it consumes can have arrived after a screen was painted.
```

Task 4.1, **Edge Cases** — add:

```
- The appearance probe is the one read the draw performs, and it reads the pane's stdin: under an adaptive pair it writes the background-colour query and then reads until a terminator or `appearanceDetectTimeout`, discarding whatever else was queued. A keystroke typed inside that window on a redraw is therefore swallowed rather than inherited by the waiter — the single narrowing of task 4.2's inherited-bytes guarantee, bounded by that timeout and reachable only on a redraw under an adaptive pair. A constant nomination and `NO_COLOR` read nothing at all. The direction is the safe one and needs no guard of its own: the probe runs before the alternate-screen entry, so it can only reach input that arrived before a screen was painted, which is what the confirmation's own drop rule already requires of every byte.
```

**Resolution**: Pending
**Notes**:

---

### 4. The picker's pending-set task is thirty-six edits larger than it reads

**Severity**: Minor
**Plan Reference**: Phase 6, task lazy-resume-on-attach-6-6 (The picker resolves pending state from one read)
**Category**: Scope and Granularity
**Move**: settled
**Change Type**: add-to-task

**Problem**:
`applySessions` has thirty-nine call sites in `internal/tui` — three in `model.go` and thirty-six across the suites. The task changes its signature and names none of them, so an executor sizing the work from the Do step meets thirty-six compile failures it did not budget for, in a task that already covers a new seam, a new `Deps` field, a `Build` option, `cmd/open.go` wiring, four fetch sites and two message types. The plan handles exactly this situation carefully everywhere else — task 2.2 enumerates every eleven-field fixture and every `CaptureStructure` call site, task 1.3 names both non-test callers of `Set`, task 1.4 names each suite that calls the lookup, task 3.3 names both callers of `renderHeaderWithBadge` — so the omission reads as an oversight rather than a decision, and it is the one place a reader would under-estimate the task.

**Proposal**:
Name the churn, as the plan's own convention does. Nothing about the design changes: the thirty-six test calls pass a nil set and assert unchanged, and a suite left at the old arity failing to compile is the tripwire rather than a reason to keep a second entry point into the model's session state.

**Current**:
```
- Extend `applySessions` to take the set alongside the sessions and store it on the model beside `m.sessions`, replacing it wholesale on every load exactly as `m.derivedDirs` is cleared, so nothing survives the list it described.
```

**Proposed Text**:
```
- Extend `applySessions` to take the set alongside the sessions and store it on the model beside `m.sessions`, replacing it wholesale on every load exactly as `m.derivedDirs` is cleared, so nothing survives the list it described.
- Carry the signature through every call site: the two production ones in `internal/tui/model.go` (the `SessionsMsg` arm and the `previewSessionsRefreshedMsg` arm) and the thirty-six `applySessions(…)` calls across the `internal/tui` suites, which pass a nil set and assert exactly as they do today. A suite left at the old arity stops compiling, which is the intended tripwire rather than a reason to keep a second way into the model's session state.
```

**Resolution**: Pending
**Notes**:

---

### 5. A key pressed as a resize settles is dropped, and the plan says it cannot be

**Severity**: Minor
**Plan Reference**: Phase 4, task lazy-resume-on-attach-4-6 (One redraw once the size has settled)
**Category**: Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
The restructured loop selects over three sources, so a read request is outstanding whenever the loop is waiting — including at the instant the settle timer fires. If a byte arrives in that instant the reader goroutine takes it off the tty queue and offers it on an unbuffered channel the loop, having taken the settle branch, never receives; the exec then replaces the process image and the byte is gone. The plan states the opposite twice — "an outstanding read has consumed nothing" in the Do step and "so no byte is consumed by a read the loop never dispatched" as a criterion — so an executor writing a test for that criterion is testing a claim the design does not deliver, and may conclude the design is wrong rather than the sentence.

**Proposal**:
Fix the claim to the property the structure actually holds — no read-ahead, so every key the loop dispatches leaves the rest of a burst queued for the next process image — and record the one-scheduling-gap window as an accepted edge case. Closing it would mean either reading ahead, which loses the inherited-bytes guarantee outright, or arming a second timer on a wait path that is forbidden one; and it lands in the same place as the appearance probe's window, which the redraw path already carries.

**Current**:

**Do**:
```
- Restructure the read loop into a `select` over three sources, with a **request-driven** one-byte reader goroutine: the loop sends on a request channel, the goroutine performs exactly one `Read` and sends the byte back on an unbuffered channel. Only one read is ever outstanding, and an outstanding read has consumed nothing, so task 4.2's inherited-bytes guarantee survives the restructure.
```

**Acceptance Criteria**:
```
- [ ] At most one read is outstanding at any moment, so no byte is consumed by a read the loop never dispatched.
```

**Proposed Text**:

**Do** — replace that bullet with:

```
- Restructure the read loop into a `select` over three sources, with a **request-driven** one-byte reader goroutine: the loop sends on a request channel, the goroutine performs exactly one `Read` and sends the byte back on an unbuffered channel. Only one read is ever outstanding, so the loop never reads ahead of the byte it is waiting on and task 4.2's inherited-bytes guarantee holds for every key the loop dispatches.
```

**Acceptance Criteria** — replace that criterion with:

```
- [ ] At most one read is outstanding at any moment, so the loop never reads ahead: a burst delivered after the byte the loop dispatched is still queued for the next process image.
```

**Edge Cases** — add:

```
- A byte arriving in the instant between the settle timer firing and the hand-off exec is lost: the outstanding read has already taken it off the tty queue, and the loop — having selected the settle branch — never receives it before the process image is replaced. The window is one scheduling gap wide and sits on the redraw path, where the inherited-bytes guarantee is already bounded by the draw's appearance probe. Nothing is built on a byte surviving it, and closing it would mean either reading ahead, which loses the guarantee outright, or arming a second timer on a wait path that is forbidden one.
```

**Resolution**: Pending
**Notes**:

---

### 6. An acceptance criterion the implementer has to guess at

**Severity**: Minor
**Plan Reference**: Phase 6, task lazy-resume-on-attach-6-5 (The pending dot packs beside the attached one)
**Category**: Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
"A nil pending set marks nothing and panics on no row" is not a readable pass/fail statement — an implementer has to reconstruct what it means from the Edge Cases three screens further down ("a nil map must render a plain row rather than panic"). A criterion that has to be decoded is a criterion a reviewer signs off on a different reading of.

**Proposal**:
Say it plainly, in the terms the Edge Case already uses.

**Current**:
```
- [ ] A nil pending set marks nothing and panics on no row.
```

**Proposed Text**:
```
- [ ] A nil pending set marks no row and never panics: every row renders exactly as it does under an empty set.
```

**Resolution**: Pending
**Notes**:

---

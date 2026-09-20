# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. A waiter that dies closes the pane the recovery step exists to save

**Severity**: Critical
**Plan Reference**: Phase 4, tasks `lazy-resume-on-attach-4-1` (The panel-drawing process) and `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
When a waiting pane's process stops without the user having answered it — a `pkill portal`, a crash, a reclaim, a hand `respawn-pane`, or a draw that could not launch the waiter — the parked shell is meant to run a recovery step that takes the pane off the panel's screen, clears its marker and drops it to the user's shell, so the pane stays alive with its transcript above it. As the plan composes that step it is handed flags it does not accept: the helper builds the tail's argv from the same whole payload it builds the draw's from, so the tail is launched carrying `--command`, `--hook-key` and the size flags, while the tail registers `--pane` and `--pane-key` alone. Its flag parse fails, it exits non-zero, the parked shell exits with it, and tmux closes the pane. On an install where 43 of 44 live sessions hold a single pane, the session closes with the pane and the next capture drops it from the saved set with its whole transcript — the exact loss the recovery step was added to prevent. Nothing in the plan catches it: the tail's own coverage drives its body through injected seams, which never reach a flag parser, and neither end-to-end suite kills a waiter.

**Proposal**:
Fix it where the plan already decides what an argv carries — the shared composer — rather than by widening the tail's flag set. The plan's own rule is that a flag is registered only on the command that honours it, so a stray one fails the parse rather than being silently ignored (task 5.4 states this for `--drop-input` and relies on it); the tail addresses a pane and reads a marker, so widening it to accept the user's command would also put arbitrary user text into a `ps`-visible argv for a process that must not read it. So `resumeChainArgv` emits the flag set the named subcommand registers, and one criterion in each of the two tasks pins it.

**Current**:

Task 4.1 — **Do**, first bullet:

```
- Add `cmd/state_resume_chain.go` holding what the three chain commands share: the flag-name constants (`command`, `report`, `hook-key`, `pane`, `pane-key`, `width`, `height`), `type resumeChainPayload struct { Command, Report, HookKey, Pane, PaneKey string; Width, Height int }`, `resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string` composing one chain command's argv (omitting `--report` when empty and the size flags when non-positive), and `resumeChainExe() (string, error)` over `os.Executable` treating an empty path as an error.
```

Task 4.1 — **Acceptance Criteria**, third criterion:

```
- [ ] The process execs `<os.Executable()> state resume-wait` carrying `--command`, `--hook-key`, `--pane`, `--pane-key`, the `--report` it was given and `--width`/`--height` set to the size it drew at; `--report` is absent from the argv when the report is empty.
```

Task 4.5 — **Acceptance Criteria**, sixth criterion:

```
- [ ] A lazy registration execs `/bin/sh -c "<draw argv>; <recover argv>"`, with `;` and not `&&`, and with every interpolated value — the command above all — single-quoted, so a command holding spaces, quotes, `$`, backticks or a newline reaches the draw's flag parser as one token.
```

Task 4.5 — **Tests**, ninth entry:

```
- `"it separates the draw and the tail with a semicolon"`
```

**Proposed Text**:

Task 4.1 — **Do**, first bullet:

```
- Add `cmd/state_resume_chain.go` holding what the three chain commands share: the flag-name constants (`command`, `report`, `hook-key`, `pane`, `pane-key`, `width`, `height`), `type resumeChainPayload struct { Command, Report, HookKey, Pane, PaneKey string; Width, Height int }`, `resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string` composing one chain command's argv, and `resumeChainExe() (string, error)` over `os.Executable` treating an empty path as an error. `resumeChainArgv` emits only the flags the named subcommand registers — `resume-draw` and `resume-wait` take the whole payload (omitting `--report` when empty and the size flags when non-positive), `resume-recover` takes `--pane` and `--pane-key` alone, because it addresses a pane and reads a marker and nothing else. A flag a subcommand does not register fails its parse, and on the tail's path a failed parse closes the pane the tail exists to keep open.
```

Task 4.1 — **Acceptance Criteria**, third criterion, with a fourth added after it:

```
- [ ] The process execs `<os.Executable()> state resume-wait` carrying `--command`, `--hook-key`, `--pane`, `--pane-key`, the `--report` it was given and `--width`/`--height` set to the size it drew at; `--report` is absent from the argv when the report is empty.
- [ ] `resumeChainArgv` for `resume-recover` carries `--pane` and `--pane-key` and nothing else, for every payload — including one holding a command, a report and a size — so the argv the tail is launched with parses against the two flags task 4.4 registers on it.
```

Task 4.5 — **Acceptance Criteria**, sixth criterion, with one added after it:

```
- [ ] A lazy registration execs `/bin/sh -c "<draw argv>; <recover argv>"`, with `;` and not `&&`, and with every interpolated value — the command above all — single-quoted, so a command holding spaces, quotes, `$`, backticks or a newline reaches the draw's flag parser as one token.
- [ ] The recover half of that chain carries `--pane` and `--pane-key` alone, so the tail the parked shell runs after the waiter has gone starts and recovers the pane rather than failing its flag parse and taking the pane down with it.
```

Task 4.5 — **Tests**, ninth entry, with one added after it:

```
- `"it separates the draw and the tail with a semicolon"`
- `"it composes the tail with the pane flags alone"`
```

**Resolution**: Fixed
**Notes**: Applied verbatim. The shared composer now emits only the flags the named subcommand registers, with the recovery step taking the pane and pane-key alone; one criterion in each of tasks 4-1 and 4-5 pins it, plus the composer test.

---

### 2. A restored pane dies when its hooks file or its preferences file cannot be resolved

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The step that decides whether a pane waits runs in every restored pane, before anything is drawn or exec'd. On a machine where the hooks path or the preferences path cannot be resolved, both loaders answer with no store at all, and the plan directs the decision step to call straight through them — so the pane's only process dies before it reaches any of its tails. The pane closes, and with it the session and its saved transcript, for a condition that today costs nothing: the helper already treats a hooks store it could not build as "no hook" and gives the pane a bare shell so it stays usable. The task's own acceptance already promises that a prefs failure leaves the pane unharmed, and the step it directs cannot deliver that promise. Both sibling tasks get this right and say so — the waiter's store seam reports the zero result for a store that could not be resolved, and doctor's resume-mode line renders the shipped default for a store it was never handed — which is what makes this one the outlier rather than a deliberate call.

**Proposal**:
Carry both existing guards into the decision step, in the words the two sibling tasks already use: a hook store that is absent is the miss it is today, and a prefs store that is absent takes the shipped default without the accessor being called at all. Neither is a new rule — both are the helper's and doctor's current behaviour, stated where the new code path would otherwise drop them.

**Current**:

Task 4.5 — **Do**, first bullet:

```
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()` (error discarded; the returned value is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

Task 4.5 — **Acceptance Criteria**, eighth criterion:

```
- [ ] A prefs read that fails resolves the install default to lazy (the shipped default) and the pane waits; nothing about the failure fails the pane.
```

Task 4.5 — **Tests**, fourteenth entry:

```
- `"it resolves the shipped default when the prefs read fails"`
```

**Proposed Text**:

Task 4.5 — **Do**, first bullet:

```
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim, and keeping the helper's existing absent-store guard, so a `cfg.HookStore` the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when that loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

Task 4.5 — **Acceptance Criteria**, eighth criterion, with one added after it:

```
- [ ] A prefs read that fails resolves the install default to lazy (the shipped default) and the pane waits; nothing about the failure fails the pane.
- [ ] A helper run whose hook store could not be built resolves to no registration — today's miss record, no marker, no wait, a bare shell — and one whose prefs store could not be resolved takes the shipped default; neither ends the helper, and the pane restores on one of the three tails either way.
```

Task 4.5 — **Tests**, fourteenth entry, with two added after it:

```
- `"it resolves the shipped default when the prefs read fails"`
- `"it resolves the shipped default when the prefs store could not be built"`
- `"it treats a hook store that could not be built as no registration"`
```

**Resolution**: Fixed
**Notes**: Applied verbatim. The decision step keeps the helper's existing absent-store guard and takes the shipped default without calling the accessor when the preferences loader answers with no store; one criterion and two tests added.

---

### 3. The README goes on telling users their resume hooks fire by themselves

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Task Template Compliance (Do completeness)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The README's `hook` section tells the reader that a registered command "re-executes automatically when a session is attached after a reboot", that a renamed session "still re-runs its command after the next reboot", and — under its own **When hooks fire** heading — that resume hooks "run … when Portal recreates a pane from saved state". Under the mode this feature ships on by default none of that happens: the pane comes back holding a panel and the command runs when the user presses Enter. No task in the plan corrects any of those sentences or documents the panel, its two keys, or that `eager` is the mode that restores the old behaviour. The plan applies the opposite rule everywhere else — doc edits ride with the task that falsifies them, stated in four phases' planner's calls and carried out in six CLAUDE.md edits and four README edits — and the one page a user actually reads to learn what a resume hook does is the place it is missed. The reader who upgrades meets panels and finds the documentation still describing the behaviour they no longer have.

**Proposal**:
The edit rides with the task that falsifies it, as every other doc edit in this plan does. Task 4.5 is where the helper stops firing the hook unconditionally, so it carries the README edit alongside the CLAUDE.md edit it already carries. The wording is left to the executor within the constraint the rest of the feature holds to — state the command, name no particular tool — and the flag itself is already documented by task 1.6, so this is the behaviour sentence and nothing more.

**Current**:

Task 4.5 — **Do**, sixth bullet:

```
- Edit the "Resume hooks" paragraph of CLAUDE.md where it states the helper execs `sh -c '<HOOK>; exec $SHELL'` or a bare `$SHELL`: it now resolves the registration's mode first and, when that resolves lazy, parks a shell running the draw followed by the chain's tail instead.
```

Task 4.5 — **Acceptance Criteria**, final criterion:

```
- [ ] No bootstrap step, step ordering, eager signal pass or global hook is touched by this task.
```

**Proposed Text**:

Task 4.5 — **Do**, sixth bullet, with one added after it:

```
- Edit the "Resume hooks" paragraph of CLAUDE.md where it states the helper execs `sh -c '<HOOK>; exec $SHELL'` or a bare `$SHELL`: it now resolves the registration's mode first and, when that resolves lazy, parks a shell running the draw followed by the chain's tail instead.
- Edit the README's `xctl hook` section, which still tells the reader a registered command re-executes automatically after a reboot, that a renamed session still re-runs its command, and — under its **When hooks fire** heading — that resume hooks run when Portal recreates a pane. Under the shipped default the pane comes back holding the resume panel showing that command, with `⏎ resume` and `d discard`, and the command runs when the user answers; `eager` is the mode that keeps the fire-on-restore behaviour those sentences describe. One edit to the section's opening paragraph and one to the **When hooks fire** paragraph, wording the executor's, naming no particular tool.
```

Task 4.5 — **Acceptance Criteria**, final criterion, with one added after it:

```
- [ ] No bootstrap step, step ordering, eager signal pass or global hook is touched by this task.
- [ ] The README's `xctl hook` section no longer states that a registered command re-executes by itself after a reboot: its opening paragraph and its **When hooks fire** paragraph both describe the panel a lazy registration comes back holding and name `eager` as the mode that keeps today's behaviour.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 4-5 gains the README edit — the hook section's opening paragraph and its When-hooks-fire paragraph — plus the criterion that holds it.

---

### 4. On a narrow pane the two screens degrade differently, and one of them cuts the command mid-word

**Severity**: Important
**Plan Reference**: Phase 3, tasks `lazy-resume-on-attach-3-1` (The command block and the report row both screens share) and `lazy-resume-on-attach-3-4` (The discard confirmation)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
Below the size the card needs, both screens are meant to drop the frame and stack the same parts plainly — the phase states it, and the confirmation's own task says it "degrades with the pane exactly as the waiting panel does". They do not. The shared command and report blocks are pinned to the card's 52-column content width with no way to ask for another, so the waiting panel's stack cannot do what its own step directs (command rows "wrapped to the pane's width"), and the confirmation's stack takes the opposite route and reuses the card's 52-column rows. Whichever way an implementer resolves that, one of the two screens hands the canvas rows wider than the pane, and the canvas cuts them at the pane's edge with no `…` — so on a narrow restored pane the user sees a command chopped mid-word on one screen and wrapped and marked on the other, for the same registration. The wrap-and-mark rule is the whole reason the shared block exists, and the pane sizes where it is dropped are exactly the ones a restored geometry produces and a user is least able to widen.

**Proposal**:
The ladder already hands the stack builder the pane's width and pins a criterion that it does; the only thing missing is a width the shared helpers will accept. Give both helpers that parameter — card call sites pass `resumeCardContentWidth`, stack call sites pass the pane's width — and build the confirmation's degraded spec at that same width so its command and report rows match the panel's. The consequence line stays on the destructive builder's own wrap, which the byte-identical kill/delete golden protects and which the kill modal already shows at that width. Task 3.3 needs no edit: its stack step already names the pane's width and its card step passes the pinned one.

**Current**:

Task 3.1 — **Do**, fourth and fifth bullets:

```
- Add `resumeCommandRows(command string, tok theme.Token, bold bool, th theme.Theme, colourless bool) []string`: each line from `resumeCommandLines` rendered through `headerStyle(tok, th, colourless)` (with `.Bold(true)` when `bold`) and padded to `resumeCardContentWidth` with `headerPadRight`, so the widest row of the card is always the pinned width.
- Add `resumeReportRow(report string, th theme.Theme, colourless bool) (string, bool)`: `("", false)` for an empty report; otherwise one row — sanitised, `ansi.Truncate(report, resumeCardContentWidth, "…")`, rendered in `accent.attention` and padded to the pinned width. Never wrapped.
```

Task 3.1 — **Acceptance Criteria**, first criterion:

```
- [ ] Every row `resumeCommandRows` returns has `lipgloss.Width == resumeCardContentWidth`, for a one-character command, a command that wraps to exactly three lines, and a command ten times longer than three lines.
```

Task 3.1 — **Tests**, eighth entry:

```
- `"it renders a report as one truncated row"`
```

Task 3.4 — **Do**, fourth bullet:

```
- Build the plain stack by flattening `destructiveConfirmCompartments` for the same spec at the pane's width, so the degraded screen carries the title, the command, the consequence line, the report when present and the key hints without the frame, and cannot drift from the card.
```

Task 3.4 — **Tests**, final entry:

```
- `"it builds the plain stack from the same compartments as the card"` (stripped stack rows are the stripped card's content rows, in order)
```

**Proposed Text**:

Task 3.1 — **Do**, fourth and fifth bullets:

```
- Add `resumeCommandRows(command string, width int, tok theme.Token, bold bool, th theme.Theme, colourless bool) []string`: each line from `resumeCommandLines(command, width)` rendered through `headerStyle(tok, th, colourless)` (with `.Bold(true)` when `bold`) and padded to `width` with `headerPadRight`. A card call passes `resumeCardContentWidth`, so the widest row of the card is always the pinned width; a degraded stack passes the pane's width, so the rows it stacks are wrapped and `…`-marked at the width they will actually be shown at rather than cut mid-word by the canvas.
- Add `resumeReportRow(report string, width int, th theme.Theme, colourless bool) (string, bool)`: `("", false)` for an empty report; otherwise one row — sanitised, `ansi.Truncate(report, width, "…")`, rendered in `accent.attention` and padded to `width`. Never wrapped. The width is the caller's for the same reason the command block's is.
```

Task 3.1 — **Acceptance Criteria**, first criterion, with one added after it:

```
- [ ] Every row `resumeCommandRows` returns has `lipgloss.Width == resumeCardContentWidth` when it is called at that width, for a one-character command, a command that wraps to exactly three lines, and a command ten times longer than three lines.
- [ ] Called at a width narrower than the card's — the width a degraded stack passes — both helpers return rows at exactly that width, wrapped and `…`-marked to it, so neither ever hands the canvas a row it has to cut.
```

Task 3.1 — **Tests**, eighth entry, with one added after it:

```
- `"it renders a report as one truncated row"`
- `"it renders both blocks at the width its caller chose"` (table: the card's pinned width, a narrower pane's)
```

Task 3.4 — **Do**, fourth bullet:

```
- Build the plain stack by flattening `destructiveConfirmCompartments` for a spec built at the pane's width — the same title, consequence, confirm key and label, with `targetRows` and `reportRows` rebuilt through the shared helpers at that width — so the degraded screen carries the title, the command, the consequence line, the report when present and the key hints without the frame, cannot drift from the card, and wraps the command exactly as the waiting panel's stack does. The consequence keeps the builder's own wrap, as the kill modal's does.
```

Task 3.4 — **Tests**, final entry:

```
- `"it builds the plain stack from the same compartments as the card"` (at a pane width equal to the card's, the stripped stack rows are the stripped card's content rows, in order)
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Both shared helpers take the width they render at, defaulting to the card's for every framed call; the confirmation's degraded stack rebuilds its rows at the pane's width, and one criterion and one test cover the narrow case.

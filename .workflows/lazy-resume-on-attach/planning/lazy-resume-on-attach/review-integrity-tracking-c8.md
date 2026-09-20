# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The setting that decides whether every pane waits is read by a route no test can watch

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The install-wide `resume_mode` is what decides, for every restored pane on the machine, whether it comes back holding a panel or running its command — and the helper reads it by calling `loadPrefsStoreNoMigrate()` directly inside `resolveResumeDecision`. That call is invisible to a test: it resolves a path out of the environment and returns a concrete `*prefs.Store`, so nothing can observe that it happened, how often, or what it answered.

The task then asks for exactly that observation. Its acceptance criterion reads "Exactly one `LookupOnResume` and one `LoadResumeMode` call happen per helper run, whichever tail it ends on — asserted with counting seams on all three tails", and the test bullet names the seams the suite injects: `HookStore`, `Client` and `ExecShell`. None of the three counts either call. The lookup half is salvageable — the task has `resolveResumeDecision` emit the existing `hook lookup` DEBUG record, so one record in the sink is one lookup — but there is no observable of any kind for the prefs read, because `internal/prefs` is a leaf that must not log.

So the implementer arrives at a criterion that cannot be met with what the task gives them. They either invent a seam the plan never specified — choosing its shape, where it defaults, and which of the two degradations move behind it — or they write the task off as done with one of its own criteria quietly unmet. The two criteria beside it ("a prefs read that fails resolves the install default to lazy", "one whose prefs store could not be resolved takes the shipped default") land in the same place: without a seam the second is reachable only by unsetting `HOME` and `XDG_CONFIG_HOME` under a `TestMain` that poisons the `PORTAL_*` paths package-wide, which is a fixture nobody should have to build to prove a one-line fallback.

**Proposal**:
Put the loader behind a seam, which is what the plan's own conventions already decide. Task 4.1 puts this identical non-migrating prefs read behind a `ResolveTheme` seam on `resumeDrawConfig` rather than calling it inline; `hydrateConfig` already carries its external work as nil-tolerant func fields (`OpenFIFO`, `HandleTimeout`, `HandleFileMissing`, `ExecShell`); and `cmd`'s rule is that a test Executing a real command body injects every seam it depends on. A `LoadPrefsStore func() (*prefs.Store, error)` field, defaulted to `loadPrefsStoreNoMigrate` when nil, is the minimal edit that keeps every degradation where the task already put it (inside `resolveResumeDecision`) while making all three criteria directly drivable. The counting criterion is then restated to name the two things a suite can actually see: the single `hook lookup` record, and one call on the seam.

**Current**:
```markdown
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)`, emitting the same `hook lookup` hit/miss/error DEBUG records and the same lookup-failure WARN `execShellOrHookAndExit` emits today — factored into one unexported helper both sites call rather than moved out of one into the other, because `execShellOrHookAndExit` goes on emitting them verbatim on its nil-`Decision` path, where the eight existing direct callers pin them — keeping the helper's existing absent-store guard so a hook store the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when that loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

and, in the same task's `Do`:

```markdown
- Add `cmd/state_hydrate_lazy_test.go` covering all three tails against injected `HookStore`, `Client` and `ExecShell` seams plus a `logtest.Sink`; re-run the existing hydrate suites and `internal/restore`'s `TestExitClosesRestoredPane_*` / `TestNoParkedShWrapperPostRestore` unchanged.
```

and, in the same task's `Acceptance Criteria`:

```markdown
- [ ] Exactly one `LookupOnResume` and one `LoadResumeMode` call happen per helper run, whichever tail it ends on — asserted with counting seams on all three tails.
```

**Proposed Text**:
```markdown
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `LoadPrefsStore func() (*prefs.Store, error)` on `hydrateConfig` beside its other func fields, nil-tolerant in the same way: a caller that leaves it unset gets `loadPrefsStoreNoMigrate`, so the production wiring is the non-migrating route and a suite can both count the read and choose what it answers. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)`, emitting the same `hook lookup` hit/miss/error DEBUG records and the same lookup-failure WARN `execShellOrHookAndExit` emits today — factored into one unexported helper both sites call rather than moved out of one into the other, because `execShellOrHookAndExit` goes on emitting them verbatim on its nil-`Decision` path, where the eight existing direct callers pin them — keeping the helper's existing absent-store guard so a hook store the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through that seam plus `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when the loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

and, in the same task's `Do`:

```markdown
- Add `cmd/state_hydrate_lazy_test.go` covering all three tails against injected `HookStore`, `Client`, `LoadPrefsStore` and `ExecShell` seams plus a `logtest.Sink`; re-run the existing hydrate suites and `internal/restore`'s `TestExitClosesRestoredPane_*` / `TestNoParkedShWrapperPostRestore` unchanged.
```

and, in the same task's `Acceptance Criteria`:

```markdown
- [ ] Exactly one hook-store lookup and one prefs load happen per helper run, whichever tail it ends on — the lookup read off the single `hook lookup` record in the sink, the prefs load off a counting `LoadPrefsStore` seam, on all three tails.
```

**Resolution**: Fixed — task 4-5 gains `LoadPrefsStore func() (*prefs.Store, error)` on `hydrateConfig`, nil-defaulting to `loadPrefsStoreNoMigrate`; the decision reads the install default through that seam; the test-file bullet lists it beside the other injected seams; and the counting criterion is restated to the single `hook lookup` record plus one call on the seam. Tick body re-synced and byte-verified.
**Notes**: Verified before applying — `hydrateConfig` already carries `ExecShell`, `OpenFIFO`, `HandleFileMissing` and `HandleTimeout` as nil-tolerant func fields (`cmd/state_hydrate.go:40-43`), and task 4.1 already puts this same non-migrating prefs read behind a `ResolveTheme` seam, so the shape is the plan's own rather than a new convention.

---

### 2. Phase 1's first storage task hands the implementer a package that does not compile

**Severity**: Minor
**Plan Reference**: Phase 1, task `lazy-resume-on-attach-1-2` (hooks.json accepts the object form and preserves what it did not write)
**Category**: Task Self-Containment / Scope and Granularity
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 1.2 changes `Snapshot` from `map[string]map[string]string` to `map[string]map[string]Registration`, then lists what has to move with it: `Set`, `classifySet`, `removedValue`, the `hooksweep` snapshot literals — and closes by saying the remaining readers "still compile and pass untouched — they read keys and lengths only". Two of them do not. `LookupOnResume` returns the inner map's value as a command string (`cmd, ok := events[EventOnResume.String()]; ... return cmd, true, nil`) and `List` ranges it into `Hook{Command: command}`. Both stop compiling the moment the value type changes, and neither appears in either list.

That leaves the implementer with a broken package and a question the task does not answer: task 1.4 exists specifically to give those two functions a new shape, so is 1.4 being pulled forward here, or are these two meant to be adapted minimally and left alone? Getting it wrong in either direction costs a task boundary — a 1.2 that grows 1.4's result struct, or a 1.4 that arrives to find its work half done.

**Proposal**:
Name the two readers in the same bullet that names the other call sites, and say what they do here: read `.Command` off the value, signatures unchanged, the mode they can now see left for the task that reports it. The answer is settled by task 1.4's own scope — it owns the `OnResume` result struct and the `Hook.Resume` field — so 1.2's job is the minimum that compiles.

**Current**:
```markdown
- Change `Snapshot` to `map[string]map[string]Registration`, and carry the change through the store: `Set` keeps its current `command string` parameter here and stores `Registration{Command: command}`; `classifySet` compares the registration that will be written against the stored one on both command and mode, so rewriting a mode-carrying entry with a bare command is a `modify` rather than a `set-noop`; `removedValue` renders each event's `Command`.
```

**Proposed Text**:
```markdown
- Change `Snapshot` to `map[string]map[string]Registration`, and carry the change through the store: `Set` keeps its current `command string` parameter here and stores `Registration{Command: command}`; `classifySet` compares the registration that will be written against the stored one on both command and mode, so rewriting a mode-carrying entry with a bare command is a `modify` rather than a `set-noop`; `removedValue` renders each event's `Command`; and `LookupOnResume` and `List`, the two readers that take the inner map's value rather than its keys, read `.Command` off it. Both keep their current signatures here — reporting the mode they can now see is the next task's work, and nothing in this one should pull it forward.
```

**Resolution**: Fixed — task 1-2's Snapshot bullet now names `LookupOnResume` and `List` as the two readers that take the inner map's value, says they read `.Command` off it, and pins both to their current signatures with the mode left to the next task. Tick body re-synced and byte-verified.
**Notes**: Verified in the tree before applying: `LookupOnResume` at `internal/hooks/lookup.go:14` does `cmd, ok := events[EventOnResume.String()]` and `List` at `internal/hooks/store.go:227` ranges the inner map into `Hook{Command: command}` — both break on the value-type change exactly as the finding states.

---

### 3. The tmux client's inventory never learns about the two pane-option methods this feature adds

**Severity**: Minor
**Plan Reference**: Phase 2, task `lazy-resume-on-attach-2-1` (The pending marker: one name, set, read and clear); Phase 4, task `lazy-resume-on-attach-4-4` (The chain's tail)
**Category**: Task Template Compliance (plan's own doc-edit convention)
**Move**: settled
**Change Type**: update-task

**Problem**:
The plan's stated convention, repeated in four phases, is that a CLAUDE.md sentence a task falsifies is corrected by that task. Task 2.1 adds `tmux.UnsetPaneOption` to the client and edits only the `state` row. The `tmux` row says the client's pane-option surface is `SetPaneOption` in two separate places — the options enumeration ("SetServerOption, SetSessionOption, SetPaneOption — the pane-scoped `set-option -p -t <pane> <name> <value>` writer…, GetServerOption, TryGetServerOption, UnsetServerOption, ShowAllServerOptions") and the list of methods that take an already-composed target ("`SetPaneOption`, `RespawnPane`, `CapturePane`, `NewWindow`, `SplitWindow`, `ResolveHookKey`"). Both stay wrong from the end of phase 2 until task 4.4 two phases later, and 4.4 corrects only the first of them — so the target-declaring list ends the feature naming neither `UnsetPaneOption` nor `ReadPaneOption`, which is the list a contributor reads to know which methods the composition guard is watching.

**Proposal**:
Each task corrects both places for the method it adds: 2.1 for `UnsetPaneOption`, 4.4 for `ReadPaneOption`, with 4.4's bullet no longer doing 2.1's half of the options clause. That split follows from the convention the plan already states and from which task introduces which method.

**Current**:
Task `lazy-resume-on-attach-2-1`, last `Do` bullet:

```markdown
- Edit the `state` row of CLAUDE.md's package table so the marker-helpers clause names `@portal-resume-pending` as the pane-scoped pending marker with its set/unset helpers and its presence rule, beside `@portal-restoring`.
```

Task `lazy-resume-on-attach-4-4`, last `Do` bullet:

```markdown
- Extend the options clause of CLAUDE.md's `tmux` package row to name the pane-option pair the marker is written and read through — `UnsetPaneOption` alongside `SetPaneOption`, and `ReadPaneOption` as the single-pane read: a `show-options -p -t <target>` existence probe naming no option followed by the `display-message -F` format read, taking that shape for the same reason `ResolveHookKey` does, since the format read alone answers a target no live pane answers to with exit 0 and an empty string.
```

**Proposed Text**:
Task `lazy-resume-on-attach-2-1`, last `Do` bullet, plus one bullet after it:

```markdown
- Edit the `state` row of CLAUDE.md's package table so the marker-helpers clause names `@portal-resume-pending` as the pane-scoped pending marker with its set/unset helpers and its presence rule, beside `@portal-restoring`.
- Edit the `tmux` row of CLAUDE.md's package table in the two places `UnsetPaneOption` falsifies: the options clause, where `SetPaneOption` is followed by `UnsetPaneOption` — the pane-scoped `set-option -pu -t <pane> <name>` remover the pending marker is cleared through — and the list of methods handed an already-composed target, which gains `UnsetPaneOption` beside `SetPaneOption`.
```

Task `lazy-resume-on-attach-4-4`, last `Do` bullet:

```markdown
- Extend CLAUDE.md's `tmux` package row in the two places `ReadPaneOption` falsifies: the options clause, where it joins `SetPaneOption` and `UnsetPaneOption` as the single-pane read — a `show-options -p -t <target>` existence probe naming no option followed by the `display-message -F` format read, taking that shape for the same reason `ResolveHookKey` does, since the format read alone answers a target no live pane answers to with exit 0 and an empty string — and the list of methods handed an already-composed target, which gains `ReadPaneOption`.
```

**Resolution**: Fixed — task 2-1 gains a `tmux`-row doc bullet covering both places `UnsetPaneOption` falsifies (the options clause and the composed-target method list), and task 4-4's bullet is restated to do the same two for `ReadPaneOption` alone rather than carrying 2-1's half. Both tick bodies re-synced and byte-verified.
**Notes**: Verified in CLAUDE.md before applying: the `tmux` row names `SetPaneOption` in the options enumeration and again in the list of methods that declare `Target`, and nothing in the plan reached the second one.

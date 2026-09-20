# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. A pane can be marked pending and then have its chain composed from a value nobody kept

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The helper resolves the pane id and the binary's own path in the step that writes the pending marker, and then the step that actually composes the chain uses both again — but nothing carries them between the two. The task's own edge case states that an executable that cannot be resolved must be refused *before* the marker is written, "because a marked pane whose chain could not be composed would fire its hook with the marker still set and freeze its saved scrollback for life". With no carrier, the implementer's only route is to resolve again on the exec path, which is exactly the state the edge case forbids: a second failure leaves the pane marked, fires the hook anyway, and that pane's saved transcript stands still for the rest of its life with no sweep that reaches a pane option and no waiter left to report it. The alternative — inventing a carrier — is a design decision the plan otherwise makes for the implementer everywhere else in this chain.

**Proposal**:
Carry both values on the decision the mark step already mutates. `resumeDecision` gains `Exe` and `Pane`; `markPendingThenUnsetSkeletonMarker` records them on the decision at the moment it resolves them, before the marker is written; `execResumeChainAndExit` reads them off the decision and resolves nothing. This is determined by the task's own rule that the refusal is taken once and before the write — the decision struct is already a pointer precisely so a value set inside a by-value handler is seen by the caller, so it is the carrier the task already built. One acceptance criterion pins it so a re-resolution on the exec path fails a test rather than shipping.

**Current**:
```markdown
**Do** (bullets 1, 2, 3 and 4):
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller). Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()` (error discarded; the returned value is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
- Add `markPendingThenUnsetSkeletonMarker(cfg hydrateConfig)` and call it in place of `unsetSkeletonMarkerOrLog` at all three sites (the replay path in `runHydrate`, `handleHydrateTimeout`, `handleHydrateFileMissing`). When the decision says wait it first resolves `$TMUX_PANE` and `resumeChainExe()` and calls `state.SetResumePendingMarker`; any of the three failing emits exactly one `hydrateLogger.Warn("set resume pending marker failed", "pane_key", …, "error", …)` and sets `Decision.Wait = false`. It then calls `unsetSkeletonMarkerOrLog(cfg)` unchanged.
- Branch in `execShellOrHookAndExit` on a **nil-tolerant** `cfg.Decision`. A nil decision is today's behaviour unchanged: the function performs its own `LookupOnResume`, emits its own `hook lookup` DEBUG records and its own lookup-failure WARN, and execs the hook or the bare shell exactly as it does now — so the eight direct callers in `cmd/state_hydrate_exec_log_test.go` and `cmd/hooks_read_lock_test.go` compile and pass unmodified. A non-nil decision skips that read: when `cfg.Decision.Wait` it composes and execs the chain, otherwise it takes today's path reading the command off `cfg.Decision.Lookup`.
- Add `execResumeChainAndExit(cfg hydrateConfig)` composing `shellWords(resumeChainArgv(exe, "resume-draw", payload)) + "; " + shellWords(resumeChainArgv(exe, "resume-recover", payload))`, where `shellWords` (in `cmd/state_resume_chain.go`) quotes every argv element through `shellquote.Single` and joins with a space; emit the existing `exec` INFO and `cfg.ExecShell("/bin/sh", []string{"sh", "-c", chained})`. The payload carries the command from the decision's lookup, the hook key, the pane id and the pane key from `state.PaneKeyFromFIFOPath(cfg.FIFO)`; the report is empty on a first draw.

**Acceptance Criteria** (the marker-refusal criterion):
- [ ] A `SetResumePendingMarker` that fails, an absent `$TMUX_PANE` and an unresolvable executable each fire the hook exactly as an eager registration does, and each emits exactly one WARN carrying `pane_key` and `error`.
```

**Proposed Text**:
```markdown
**Do** (bullets 1, 2, 3 and 4):
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()` (error discarded; the returned value is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`. `Exe` and `Pane` are left empty here — they are resolved by the mark step, which is where the refusal has to be taken.
- Add `markPendingThenUnsetSkeletonMarker(cfg hydrateConfig)` and call it in place of `unsetSkeletonMarkerOrLog` at all three sites (the replay path in `runHydrate`, `handleHydrateTimeout`, `handleHydrateFileMissing`). When the decision says wait it first resolves `$TMUX_PANE` and `resumeChainExe()` and calls `state.SetResumePendingMarker`; any of the three failing emits exactly one `hydrateLogger.Warn("set resume pending marker failed", "pane_key", …, "error", …)` and sets `Decision.Wait = false`. On the path where all three succeed it records the resolved values on the decision as `Pane` and `Exe`, before the marker is written, so the chain is composed from the values the refusal was taken over and nothing downstream resolves either again. It then calls `unsetSkeletonMarkerOrLog(cfg)` unchanged.
- Branch in `execShellOrHookAndExit` on a **nil-tolerant** `cfg.Decision`. A nil decision is today's behaviour unchanged: the function performs its own `LookupOnResume`, emits its own `hook lookup` DEBUG records and its own lookup-failure WARN, and execs the hook or the bare shell exactly as it does now — so the eight direct callers in `cmd/state_hydrate_exec_log_test.go` and `cmd/hooks_read_lock_test.go` compile and pass unmodified. A non-nil decision skips that read: when `cfg.Decision.Wait` it composes and execs the chain, otherwise it takes today's path reading the command off `cfg.Decision.Lookup`.
- Add `execResumeChainAndExit(cfg hydrateConfig)` composing `shellWords(resumeChainArgv(cfg.Decision.Exe, "resume-draw", payload)) + "; " + shellWords(resumeChainArgv(cfg.Decision.Exe, "resume-recover", payload))`, where `shellWords` (in `cmd/state_resume_chain.go`) quotes every argv element through `shellquote.Single` and joins with a space; emit the existing `exec` INFO and `cfg.ExecShell("/bin/sh", []string{"sh", "-c", chained})`. The payload carries the command from the decision's lookup, the hook key, `cfg.Decision.Pane` as the pane id and the pane key from `state.PaneKeyFromFIFOPath(cfg.FIFO)`; the report is empty on a first draw. This function resolves nothing of its own — it is only ever reached on a decision whose `Exe` and `Pane` are already filled.

**Acceptance Criteria** (the marker-refusal criterion):
- [ ] A `SetResumePendingMarker` that fails, an absent `$TMUX_PANE` and an unresolvable executable each fire the hook exactly as an eager registration does, and each emits exactly one WARN carrying `pane_key` and `error`.
- [ ] The chain is composed from the executable path and the pane id the mark step resolved: `os.Executable` and `$TMUX_PANE` are each read exactly once per helper run, and the argv the chain carries names those values — so no second resolution can fail after the marker has been written.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. The decision struct gains the executable path and the pane id, the mark step records them before the marker is written, the exec step reads them off the decision, and one criterion pins a single read of each per helper run.

---

### 2. The process that paints every waiting pane has no stated way of measuring the pane

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-1` (The panel-drawing process)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The draw declares six injected seams and names the production binding for only two of them — the colourless flag and the theme resolver. The one that matters is `Size`: how the drawing process learns the pane's dimensions is left for the implementer to choose, and the candidates are not equivalent. A tty ioctl on the pane's own stdin costs nothing; asking tmux (`display-message -p '#{pane_width}'`) costs one tmux call per draw, which on the boot this feature is built for is forty-one tmux calls before the first panel appears, plus one more on every resize of every waiting pane. The plan settles this elsewhere — the resize task names `term.GetSize` as "the same read the draw takes" — but a reader executing task 4.1 on its own never sees that sentence, and every sibling task in this chain (4.2, 4.3, 4.4, 4.6, 5.4, 5.5) states its production wiring in full. `ExecSelf` is the same gap with a sharper edge: it must replace the process image rather than spawn, or the pane ends up holding two Portal processes and the resident-cost argument the whole design rests on is gone.

**Proposal**:
State the draw's production wiring in the same breath the seams are declared, taking the bindings the plan has already determined elsewhere: `os.Stdout` for the writer, `term.GetSize` over stdin's fd for the size (the read task 4.6 names), and `defaultExecShell` for the hand-off — whose `syscall.Exec` → WARN → `log.Close(1)` → `osExit(1)` shape is exactly the failure behaviour task 4.1's own acceptance criterion already describes, and which tasks 4.3 and 5.5 name as the termination they take.

**Current**:
```markdown
- Add `cmd/state_resume_draw.go` with `resumeDrawConfig` (a `resumeChainPayload` plus `Stdout io.Writer`, `Logger *slog.Logger`, `Colourless bool`, `Size func() (int, int, error)`, `ResolveTheme func(colourless bool) theme.Theme`, `ExecSelf func(prog string, args []string)`), `runResumeDraw(cfg) error`, the package-level seam `resumeDrawRunFunc = runResumeDraw`, and the hidden `stateResumeDrawCmd` registering on `stateCmd` with `--command` required and the rest optional.
```

**Proposed Text**:
```markdown
- Add `cmd/state_resume_draw.go` with `resumeDrawConfig` (a `resumeChainPayload` plus `Stdout io.Writer`, `Logger *slog.Logger`, `Colourless bool`, `Size func() (int, int, error)`, `ResolveTheme func(colourless bool) theme.Theme`, `ExecSelf func(prog string, args []string)`), `runResumeDraw(cfg) error`, the package-level seam `resumeDrawRunFunc = runResumeDraw`, and the hidden `stateResumeDrawCmd` registering on `stateCmd` with `--command` required and the rest optional.
- Production wiring for the rest of those seams: `Stdout` is `os.Stdout`; `Size` is `term.GetSize` over stdin's fd — the pane's own tty, so the draw makes no tmux call at all and a boot's worth of panes costs none (task 4.6's settle redraw takes the same read); `ExecSelf` is the existing `defaultExecShell`, whose `syscall.Exec` replaces the process image so the drawing process is gone by the time the pane is waiting, and whose return path is already the WARN plus non-zero exit this task's acceptance criterion describes.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 4-1 now states the production binding for every seam it declares — standard output, the terminal size read over the pane's own input, and the existing shell-exec helper that replaces the process image.

---

### 3. The waiter hands the pane over to a binary path it was never given

**Severity**: Minor
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-2` (The waiting process: two keys, everything else swallowed)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: add-to-task

**Problem**:
Every key the waiter acts on hands the pane to a fresh `resume-draw` composed as `resumeChainArgv(exe, …)`, and `exe` is never sourced — the waiter is a fresh process image that was handed flags, not the path of the binary that launched it. The implementer has to decide both where it comes from and what happens when it cannot be resolved, and the second half has a user-visible fork: swallowing the key leaves a pane whose advertised keys silently do nothing forever, while ending the wait drops the pane to the chain's tail, which recovers it to a usable shell. The same gap sits on the settle redraw in task 4.6 and on all three hand-offs in task 5.3, which inherit the dispatch shape this task declares.

**Proposal**:
Resolve it through the `resumeChainExe()` the chain already owns, at the moment of the hand-off, and treat a failure as the read-error condition the task already defines — return it, ending the wait, which reaches the chain's tail with the marker still set and the pane recovered to a usable shell. That is the plan's own stated fallback for every other way the wait can end badly ("the pending marker is still set, so the chain's tail recovers the pane to a usable shell, which is the designed fallback rather than a loss"), so nothing new is decided here.

**Current**:
```markdown
- Dispatch through two named package functions, `resumeAnswerEnter(cfg) error` and `resumeAnswerDiscard(cfg) error`, both of which here restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload unchanged (`resumeChainArgv(exe, "resume-draw", cfg.resumeChainPayload)`), each preceded by the existing `exec` INFO. Task 4.3 re-points `resumeAnswerEnter`; Phase 5 re-points `resumeAnswerDiscard`.
```

**Proposed Text**:
```markdown
- Dispatch through two named package functions, `resumeAnswerEnter(cfg) error` and `resumeAnswerDiscard(cfg) error`, both of which here resolve the binary through `resumeChainExe()`, restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload unchanged (`resumeChainArgv(exe, "resume-draw", cfg.resumeChainPayload)`), each preceded by the existing `exec` INFO. A `resumeChainExe()` that fails is returned as the wait's ending condition, exactly as a read error is — the pending marker is still set, so the chain's tail recovers the pane to a usable shell rather than leaving a pane whose keys silently do nothing. Every later hand-off in the chain resolves the same way. Task 4.3 re-points `resumeAnswerEnter`; Phase 5 re-points `resumeAnswerDiscard`.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Both hand-offs resolve the binary through the chain's own helper, and a failure returns as the wait's ending condition so the tail recovers the pane rather than leaving its keys inert.

---

### 4. The feature's one new module dependency is never named, and the two candidates are not interchangeable

**Severity**: Minor
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-5` (The theme the panel draws in)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The appearance probe is where raw terminal I/O enters Portal — `term.IsTerminal`, `term.MakeRaw`, `term.Restore` — and tasks 4.1, 4.2 and 4.6 all inherit it by reference ("the same package Phase 3's appearance probe binds"). Portal imports no such package today. There are two live candidates in the Go ecosystem and their signatures are incompatible: `golang.org/x/term` takes an `int` fd and is not in this module's graph at all, while `github.com/charmbracelet/x/term` takes a `uintptr` and is already present as an indirect requirement. The plan writes `term.IsTerminal(os.Stdin.Fd())`, which only compiles against the second — so the answer is settled, but it is settled by a detail the reader has to reverse-engineer from an argument type, on the one edit in this feature that moves `go.mod`.

**Proposal**:
Name the package where it first appears. `github.com/charmbracelet/x/term` is the one the plan's own call signature selects, it is already in the module graph as an indirect requirement of the Bubble Tea stack, and it carries `IsTerminal`, `MakeRaw`, `Restore` and the `GetSize` task 4.6 takes — so the whole chain binds one package and `go.mod` gains one promotion rather than a new module.

**Current**:
```markdown
- Add the probe behind a struct of seams — `out io.Writer`, a reader exposing `Read` plus `SetReadDeadline`, `isTerminal func() bool`, `makeRaw func() (restore func(), err error)`, and `timeout time.Duration` defaulted from `appearanceDetectTimeout` — with the production constructor binding `os.Stdout`, `os.Stdin`, `term.IsTerminal(os.Stdin.Fd())` and `term.MakeRaw`/`term.Restore`.
```

**Proposed Text**:
```markdown
- Add the probe behind a struct of seams — `out io.Writer`, a reader exposing `Read` plus `SetReadDeadline`, `isTerminal func() bool`, `makeRaw func() (restore func(), err error)`, and `timeout time.Duration` defaulted from `appearanceDetectTimeout` — with the production constructor binding `os.Stdout`, `os.Stdin`, `term.IsTerminal(os.Stdin.Fd())` and `term.MakeRaw`/`term.Restore`, over `github.com/charmbracelet/x/term`. That package is already in the module graph as an indirect requirement of the Bubble Tea stack and carries the `uintptr`-fd shape these calls are written against, so this edit promotes it to a direct requirement in `go.mod` rather than adding a module; it is also where the chain's later terminal calls come from — the waiter's raw-mode entry and the draw's size read alike — so one package serves all of them.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 3-5 names the terminal package the plan's own call signature selects, recording that it is already in the module graph indirectly and that the promotion serves the whole chain.

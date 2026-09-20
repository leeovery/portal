# Phase 4: The waiting pane — hand-off, hold, and resume — 7 tasks

## lazy-resume-on-attach-4-1

### Task 4.1: The panel-drawing process

**Problem**: Phase 3 produced the panel as a value — `tui.RenderResumePanel` takes a command, a report and a size and returns a string, and `tui.ResolvePaneTheme` answers which palette it paints in — and nothing in Portal puts either into a pane. A restored pane whose resume is lazy needs a process that enters the pane's alternate screen, paints the panel across it, and then gets out of the way: drawing touches the theme and the rendering path, and those pages stay resident for that process's life, so a process that drew and then blocked would carry all of it for as long as the pane waits. Nothing in `cmd` today reads a pane's size, enters the alternate screen, or resolves a theme outside the picker's Bubble Tea program.

**Solution**: `portal state resume-draw` — a hidden `state` subcommand that resolves the theme through the non-migrating prefs route, measures the pane, writes the alternate-screen entry followed by the rendered panel, and then replaces its own process image with `portal state resume-wait`, carrying forward everything a later screen needs so nothing downstream has to re-read the store.

**Outcome**: One invocation paints the specified panel into a pane at the pane's real size and in the install's theme, and the process that drew it is gone by the time the pane is waiting.

**Do**:
- Add `cmd/state_resume_chain.go` holding what the three chain commands share: the flag-name constants (`command`, `report`, `hook-key`, `pane`, `pane-key`, `width`, `height`), `type resumeChainPayload struct { Command, Report, HookKey, Pane, PaneKey string; Width, Height int }`, `resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string` composing one chain command's argv, and `resumeChainExe() (string, error)` over `os.Executable` treating an empty path as an error. `resumeChainArgv` emits only the flags the named subcommand registers — `resume-draw` and `resume-wait` take the whole payload (omitting `--report` when empty and the size flags when non-positive), `resume-recover` takes `--pane` and `--pane-key` alone, because it addresses a pane and reads a marker and nothing else. A flag a subcommand does not register fails its parse, and on the tail's path a failed parse closes the pane the tail exists to keep open.
- Add `hydrateAltScreenEnter = "\x1b[?1049h\x1b[?25l"` to the const block beside `hydrateResetPreamble` in `cmd/state_hydrate.go` — the enter half of the one owned pair; the existing preamble stays the leave half and is not duplicated.
- Add `cmd/state_resume_draw.go` with `resumeDrawConfig` (a `resumeChainPayload` plus `Stdout io.Writer`, `Logger *slog.Logger`, `Colourless bool`, `Size func() (int, int, error)`, `ResolveTheme func(colourless bool) theme.Theme`, `ExecSelf func(prog string, args []string)`), `runResumeDraw(cfg) error`, the package-level seam `resumeDrawRunFunc = runResumeDraw`, and the hidden `stateResumeDrawCmd` registering on `stateCmd` with `--command` required and the rest optional.
- Production wiring for the rest of those seams: `Stdout` is `os.Stdout`; `Size` is `term.GetSize` over stdin's fd — the pane's own tty, so the draw makes no tmux call at all and a boot's worth of panes costs none (the settled-size redraw takes the same read); `ExecSelf` is the existing `defaultExecShell`, whose `syscall.Exec` replaces the process image so the drawing process is gone by the time the pane is waiting, and whose return path is already the WARN plus non-zero exit this task's acceptance criterion describes.
- Body order, which is load-bearing: read the size (an error or a non-positive value is passed through to the renderer unchanged, which resolves it to its own bounded fallback); resolve the theme; write `hydrateAltScreenEnter`, then `"\x1b[H"`, then `tui.RenderResumePanel(tui.ResumeScreen{Command, Report, Width, Height, Theme, Colourless})`; emit the existing `exec` INFO (`target`, `args`) as the statement immediately before the hand-off; `ExecSelf` the `resume-wait` argv carrying the same payload plus the width and height just drawn at. The leave sequence is never written by this process.
- Production theme resolution: `noColorEnabled()` for `Colourless`; `loadPrefsStoreNoMigrate()` → `LoadThemeKeys()` → `themeResolution(keys, newThemeLoader())` → `tui.ResolvePaneTheme(resolution.Nomination, colourless)`. Any failure along that route — path resolution, the read, the resolution — degrades to `theme.AdaptivePair` over `LoadBuiltin(theme.DefaultLightSlug)` / `LoadBuiltin(theme.DefaultDarkSlug)` rather than aborting the draw; `themesDirPath()` failing already degrades to `""`, which the loader answers from the embedded built-ins.
- Extend `internal/log`'s `ResolveProcessRole` `state` arm so `resume-draw`, `resume-wait` and `resume-recover` join `hydrate` and `signal-hydrate` in returning `roleHydrate`, and add the three rows to `internal/log/process_role_test.go`'s table.
- Add one sentence to CLAUDE.md's "Resume hooks" section naming the three hidden chain subcommands (`portal state resume-draw` / `resume-wait` / `resume-recover`), stating that all three resolve to the existing `hydrate` process role and emit under the existing `hydrate` component.

**Acceptance Criteria**:
- [ ] `portal state resume-draw --command "<cmd>"` writes, in order and to its configured stdout: the alternate-screen entry, a cursor-home sequence, and exactly the bytes `tui.RenderResumePanel` returns for the same command, report, size, theme and colourless value.
- [ ] The rendered bytes are byte-identical to calling `tui.RenderResumePanel` directly with the same `ResumeScreen`, so the pane draws the production renderer and no second layout exists.
- [ ] The process execs `<os.Executable()> state resume-wait` carrying `--command`, `--hook-key`, `--pane`, `--pane-key`, the `--report` it was given and `--width`/`--height` set to the size it drew at; `--report` is absent from the argv when the report is empty.
- [ ] `resumeChainArgv` for `resume-recover` carries `--pane` and `--pane-key` and nothing else, for every payload — including one holding a command, a report and a size — so the argv the tail is launched with parses against the two flags task 4.4 registers on it.
- [ ] A size read that fails, and one returning a zero or negative dimension, still paints — the value reaches the renderer unchanged and the renderer's bounded fallback applies; nothing panics and nothing writes an empty screen.
- [ ] Under `NO_COLOR` the theme resolver is called with `colourless` true, no appearance query is written, and the painted bytes carry no SGR background parameter.
- [ ] A prefs store that cannot be resolved, a `LoadThemeKeys` that errors, and a `themeResolution` that errors each paint from the shipped light/dark pair rather than failing the command.
- [ ] The theme read is `loadPrefsStoreNoMigrate` + `LoadThemeKeys` — no call reaches `loadPrefsStore`, so no draw dispatches the one-shot `appearance` translation or writes `prefs.json`.
- [ ] An `ExecSelf` that returns (the exec failed) leaves the command exiting non-zero after one WARN, with the alternate screen still entered and the panel still painted.
- [ ] The appearance probe is the only stdin read the draw performs, and it runs after any input drop and before the alternate-screen entry — so no byte it consumes can have arrived after a screen was painted.
- [ ] `log.ResolveProcessRole` answers `hydrate` for `state resume-draw`, `state resume-wait` and `state resume-recover`, and the closed role space gains no member.

**Tests**:
- `"it writes the alternate-screen entry before the panel"`
- `"it paints the production renderer byte-identically"` (table: with and without a report)
- `"it execs the waiter carrying the payload and the size it drew at"`
- `"it omits the report flag when there is nothing to report"`
- `"it paints at the renderer's fallback when the size read fails"` (table: error, `0x0`, negative)
- `"it writes no background and runs no detection under NO_COLOR"`
- `"it paints from the shipped pair when the prefs route fails"` (table: path error, read error, resolution error)
- `"it never takes the migrating prefs route"` (a prefs file holding `appearance` is byte-unchanged after a draw)
- `"it exits non-zero when the hand-off exec fails"`
- `"it resolves the hydrate process role for every chain subcommand"` (`internal/log`)

**Edge Cases**:
- A failed or zero size read falls back to the bounded render rather than painting nothing: the drawing process reads the pane's size from a terminal that may not be one, and a pane that drew nothing would read as an ordinary restored pane with a dead keyboard.
- The alternate-screen entry is written before the paint and never left by this process — the leave belongs to whatever answers the panel or to the chain's tail, so a draw that exited after leaving would reveal the transcript with no process holding the pane.
- The enter and leave sequences are the one pair `cmd` already owns rather than a second spelling: the leave is the existing `hydrateResetPreamble` and the enter is declared beside it. A second `?1049h` written to a pane already on the alternate screen does not nest, so a redraw re-writing the entry is safe.
- `NO_COLOR` paints no canvas and writes no query — the colourless flag is resolved once here and handed to both the theme resolver and the renderer, so the two cannot disagree.
- A prefs read that fails degrades to the shipped pair rather than blocking the draw: an unreadable preferences file resolves to the shipped default by the ordinary route, and a pane that could not be painted is worse than one painted in the wrong half of a pair.
- The theme read is the non-migrating one so every pane drawing at boot never races the one-shot `appearance` translation — forty-one processes taking the migrating route concurrently is a write storm over one file for a value none of them is setting.
- A themes directory that will not resolve still paints from the embedded built-ins: `themesDirPath()` answers `""` on failure and the loader reaches the built-ins with no path at all.
- The process role resolves to the existing hydrate role so the closed role space gains no member — the mapping is argv matching, so the three names can be added before the commands that answer to them exist.
- The exec target carries everything a redraw needs so no screen re-reads the store to decide whether to draw: a redraw that re-read the store after a discard would find nothing, paint nothing, and leave a pane that looks restored, swallows every key and is frozen for the rest of its life.
- The appearance probe is the one read the draw performs, and it reads the pane's stdin: under an adaptive pair it writes the background-colour query and then reads until a terminator or the detect timeout, discarding whatever else was queued. A keystroke typed inside that window on a redraw is therefore swallowed rather than inherited by the waiter — the single narrowing of the wait task's inherited-bytes guarantee, bounded by that timeout and reachable only on a redraw under an adaptive pair. A constant nomination and `NO_COLOR` read nothing at all. The direction is the safe one and needs no guard of its own: the probe runs before the alternate-screen entry, so it can only reach input that arrived before a screen was painted, which is what the confirmation's own drop rule already requires of every byte.
- An exec that fails exits non-zero and leaves the chain's tail to recover the pane — the tail is reached because the helper composes the chain with `;` rather than `&&`.

**Context**:
> The resident cost is the runtime floor, not the binary. So no separate binary is warranted: the waiter is the same Portal binary entered on a path that does almost nothing. The process that draws must hand off to a fresh one before waiting — drawing touches the theme and the rendering path, and those pages stay resident for that process's life. The helper already ends in exactly that kind of handover, so this is the shape the code is already built around, not a new one.
>
> Every screen the pane shows takes that same handover. A wait is not one draw: `d` puts up the confirmation, Escape brings the card back, and an answer that cannot be carried out redraws the card with its report row. Each is a fresh draw that hands back to a fresh wait, so the process holding a pane between screens carries the wait and nothing else.
>
> The theme resolves as it does everywhere else in Portal: a named theme paints from the first frame with no gate at all, and a light/dark pair runs the same detect-or-timeout appearance gate the picker runs, in the process that draws. That process hands off before it waits, so the gate is paid once per draw and nothing of it stays resident while the pane waits.
>
> **Corrigendum 2026-09-19**: an unreadable `prefs.json` resolves to the shipped default like every other unreadable case — a panel is answerable in a keystroke and an unwanted resume is not undoable. This task applies the same direction to the theme keys on the same file: a read that fails paints from the shipped pair rather than failing the pane.
>
> The install default and the theme setting are read through the **non-migrating** prefs route, the one `portal doctor` already uses. The migrating route performs the one-shot `appearance` translation and writes.
>
> The waiting pane is three hidden `state` subcommands rather than one — the draw, the wait and the chain's tail are three process images in one chain, each independently testable. All three map to the existing hydrate process role and emit under the existing hydrate log component, so the closed role space and the closed component vocabulary gain no member and nothing here is a log-taxonomy change.
>
> This task's call: the chain's processes are addressed by both the pane id (`--pane`, for the marker writes) and the positional pane key (`--pane-key`, for the log records), so every hydrate-catalog record the chain emits names its pane exactly as the helper's existing records do and the closed attr vocabulary gains no member.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.2, §5.2, §3.1, Corrigendum 2026-09-19 (unreadable preferences file)

## lazy-resume-on-attach-4-2

### Task 4.2: The waiting process: two keys, everything else swallowed

**Problem**: The draw hands off to a process that does not exist. That process is the pane's only process, so anything that kills it takes the pane with it — and the keys most likely to be aimed at it are exactly the ones that kill a foreground process: Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit. The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot *answer* the panel, because nothing but Enter and `d` means anything to it. Nothing in `cmd` reads raw keystrokes today — the picker's key handling is Bubble Tea's, and this process must not be a Bubble Tea program because it has to leave the painted alternate screen behind while it execs itself away.

**Solution**: `portal state resume-wait` — put the pane's tty in raw mode, read one byte at a time, and dispatch exactly two of them; every other byte is discarded without a trace. Both acting keys hand the pane over to a fresh process image rather than doing work in place, so the process that holds a pane between screens carries the wait and nothing else.

**Outcome**: A pane holds the panel indefinitely under a process that resolves no theme, renders nothing and starts no timer; Ctrl-C, Ctrl-D, Ctrl-Z and Escape change nothing; Enter and `d` each produce exactly one hand-off; and the pane's teardown ends the waiter.

**Do**:
- Add `cmd/state_resume_wait.go` with `resumeWaitConfig` (a `resumeChainPayload` plus `Stdout io.Writer`, `In io.Reader`, `Logger *slog.Logger`, `IsTerminal func() bool`, `MakeRaw func() (restore func(), err error)`, `ExecSelf func(prog string, args []string)`), `runResumeWait(cfg) error`, the package-level seam `resumeWaitRunFunc = runResumeWait`, and the hidden `stateResumeWaitCmd` on `stateCmd`. Production wiring binds `os.Stdin`/`os.Stdout`, `term.IsTerminal`, and `term.MakeRaw`/`term.Restore` over the stdin fd — the same package Phase 3's appearance probe binds.
- Guard before the loop: a stdin that is not a terminal, or a `MakeRaw` that fails, returns an error without reading — there is nothing to wait on and spinning on a non-tty would burn a core for the life of the pane. `defer` the restore so it runs on every path out.
- Read loop: `In.Read` into a **one-byte** buffer per iteration, never buffering ahead. `\r` and `\n` dispatch as Enter; `d` dispatches as discard; every other byte — `0x03`, `0x04`, `0x1a`, `0x1b` and the bytes of any escape sequence that follows it, `D`, printable text, anything — is discarded and the loop continues. A read returning an error or EOF ends the wait by returning that condition.
- Dispatch through two named package functions, `resumeAnswerEnter(cfg) error` and `resumeAnswerDiscard(cfg) error`, both of which here resolve the binary through `resumeChainExe()`, restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload unchanged (`resumeChainArgv(exe, "resume-draw", cfg.resumeChainPayload)`), each preceded by the existing `exec` INFO. A `resumeChainExe()` that fails is returned as the wait's ending condition, exactly as a read error is — the pending marker is still set, so the chain's tail recovers the pane to a usable shell rather than leaving a pane whose keys silently do nothing. Every later hand-off in the chain resolves the same way. Task 4.3 re-points `resumeAnswerEnter`; Phase 5 re-points `resumeAnswerDiscard`.
- Install no handler for SIGHUP, SIGTERM or SIGINT, and arm no timer at all: the default disposition must continue to end the process when tmux tears the pane down, and a reported reason must stand until a key is pressed. The guard below covers the signal half, which is the half a source scan can decide — it reads every `signal.Notify` call on the wait path and fails on any signal but `syscall.SIGWINCH`, so task 4.6's resize seam leaves it green while a declined hangup fails it. Task 4.6 arms the one timer the wait path ever holds, a settle window for the resize redraw; that it neither ends the wait nor expires a report is a behavioural property and is asserted there, not by this guard.
- Cover in `cmd/state_resume_wait_test.go` driving `In` from an `os.Pipe` (or a scripted reader) and recording `ExecSelf`, with a table over every swallowed byte asserting no exec, no write and no return.

**Acceptance Criteria**:
- [ ] `\r` and `\n` each produce exactly one hand-off; `d` produces exactly one hand-off; each is preceded by one `exec` INFO carrying `target` and `args`.
- [ ] `0x03` (Ctrl-C), `0x04` (Ctrl-D), `0x1a` (Ctrl-Z), `0x1b` (Escape), `D`, `y`, `q` and a run of ordinary printable text each produce no hand-off, no write to stdout, and leave the loop running.
- [ ] A three-byte escape sequence (`\x1b[A`) produces no hand-off — each of its bytes is discarded on its own, so no sequence assembles into an action.
- [ ] A burst delivering `dy` in one write produces exactly one hand-off (the `d`) and the `y` is discarded, because nothing on this screen acts on `y`.
- [ ] The terminal restore runs on every path out: an answered key, a read error, an EOF, and a `MakeRaw` that succeeded followed by any later failure.
- [ ] A stdin that is not a terminal, and a `MakeRaw` that fails, each return an error without reading a byte, without writing, and without exec'ing.
- [ ] The restore runs **before** the hand-off exec, so the next process image inherits a cooked tty.
- [ ] The command registers no handler for SIGHUP, SIGTERM or SIGINT — asserted by a source guard over the wait path that reads every `signal.Notify` call there and fails on any signal but `syscall.SIGWINCH`, so task 4.6's resize seam leaves it green.
- [ ] The wait path arms no timer: a waiter launched with a non-empty `--report` is still reading, still holding that report and still dispatching both keys however long it is left alone with no input.
- [ ] The wait path constructs no theme, calls no renderer and writes nothing to stdout while waiting.
- [ ] The dispatch is size-independent: the same two keys act whatever `--width`/`--height` say, including values below the card's size and non-positive ones.

**Tests**:
- `"it hands the pane over on Enter"` (table: `\r`, `\n`)
- `"it hands the pane over on d"`
- `"it swallows every other key"` (table: `\x03`, `\x04`, `\x1a`, `\x1b`, `D`, `y`, `q`, `a`, ` `)
- `"it swallows the bytes of an escape sequence without assembling an action"`
- `"it acts on the first acting key in a burst and swallows the rest"`
- `"it restores the terminal on every exit path"` (table: answered, read error, EOF)
- `"it restores the terminal before the hand-off exec"`
- `"it refuses to wait on a stdin that is not a terminal"`
- `"it refuses to wait when raw mode cannot be entered"`
- `"it ends the wait on a read error rather than spinning"`
- `"it writes nothing and resolves no theme while waiting"`
- `"it acts on both keys at a size below the card's"` (table over sizes)
- `"it installs no hangup, terminate or interrupt handler"` (source guard over the wait path's signal.Notify calls)
- `"it holds its report and goes on waiting with no input at all"`

**Edge Cases**:
- Ctrl-C, Ctrl-D and Ctrl-Z arrive as bytes under raw mode and are swallowed like any other key: raw mode clears ISIG and ICANON, so none of the three reaches the process as a signal or an EOF condition — they are `0x03`, `0x04` and `0x1a` in the read buffer and nothing more.
- Escape is inert and an escape sequence's bytes never act: Escape is live only inside the confirmation `d` opens, so on this screen its byte is discarded along with whatever follows it. There is nowhere to back out to, and binding the reflex key to an irreversible deletion would make this the one place in Portal where it destroys something.
- Bytes left unread after an answer are inherited by whatever the handover execs: the loop reads one byte at a time and never reads ahead, so a byte still in the tty's input queue when the exec happens is still there for the next process image. Only the confirmation drops input in flight, which is Phase 5's rule and not this screen's.
- The terminal mode is restored on every exit path, including the pane's teardown — where tmux destroys the pty with the process and there is nothing left to restore.
- The hangup is not declined, so a killed session, a closed window or a server shutdown ends the waiter. The refusal covers what a person at the keyboard can send and stops there: an unbounded refusal would outlive the destruction of its own pane, leaving a Portal process per culled session with nothing to attach to.
- A read error or a stdin that is not a terminal ends the wait rather than spinning — the pending marker is still set, so the chain's tail recovers the pane to a usable shell, which is the designed fallback rather than a loss.
- The wait path resolves no theme and renders nothing so a pane between screens carries the wait alone: that split is the whole reason the draw hands off, and a waiter that touched the rendering path would put its pages back.
- The waiter arms no timer of its own that ends the wait or expires a report, so a reported reason stands until a key is pressed — a report the user can miss leaves them believing the thing they asked for happened. The resize settle window task 4.6 adds is the one timer the wait path arms, and all it decides is whether to hand over to a redraw; it never ends the wait and never clears the report, which rides the redraw forward.
- Both keys act at every pane size including one below the card's: the waiter never consults the size to decide what a key means, which is what makes "Enter and `d` act at every size" structural rather than a second rule.
- In this phase both keys hand over to a fresh draw of the waiting panel. Enter's destination is task 4.3's and `d`'s is Phase 5's; nothing reaches this command until the helper composes the chain (task 4.5), so neither is a live wrong behaviour in the meantime.

**Context**:
> The waiter is the pane's only process, so anything that kills it takes the pane with it — Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit. The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot *answer* the panel, because nothing but Enter and `d` means anything to it.
>
> That refusal covers what a person at the keyboard can send, and stops there. When tmux tears the pane down — the user kills the session, closes the window, or the server shuts down — the waiter exits. It does not decline the hangup, and a closed terminal ends it.
>
> A process in a pane reads the keys sent to it and nothing else. The scoping the design needs is a property of processes, not something tmux has to provide — `key-table` is a session option, not a pane option, so arming one waiting pane's keys would arm and lock every pane in its session.
>
> Escape on the waiting panel does nothing at all. There is nowhere to back out to, so it is inert. Everywhere else in Portal, Escape means *back out* — it reverses, it never acts. A key the user's hands press without consulting them can then never be the key that loses work.
>
> The drawing process cannot be a Bubble Tea program — it must leave the alternate screen painted while it execs the waiter away — which is why the key handling here is a raw read rather than a program loop.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.3, §6.3, §4.2, §5.2

## lazy-resume-on-attach-4-3

### Task 4.3: Enter resumes the pane

**Problem**: The waiter recognises Enter and hands the pane straight back to a redraw — nothing resumes, and the feature has no answer yet. Enter has to do three things in one fixed order, and the order is the whole of the protection. The pane must leave the panel's screen **before** its marker is cleared: clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus a picture of the card — the whole failure, in the space between two steps, on the path every resume takes. And nothing may run while the marker stands: handing the pane over with the marker still set freezes that pane's saved scrollback for the rest of the pane's life, with no sweep that reaches a pane option and no waiter left to report it.

**Solution**: Re-point `resumeAnswerEnter` at the ordered sequence — leave the panel's screen, clear the pending marker, then read the store again and exec what it holds now — with a clear that fails holding the answer: the panel is drawn again carrying the reason on its report row, and neither the hook nor the shell runs.

**Outcome**: Enter reveals the transcript that was underneath the panel and starts the registered command over it; an entry that went away while the pane waited drops it to a plain shell; and a freeze that will not lift leaves the pane waiting with the reason on the card and the key live.

**Do**:
- Move the hook-exec composition out of `cmd/state_hydrate.go` into `cmd/state_resume_chain.go` as `hookExecArgs(command, shell string) (prog string, args []string)` returning `"/bin/sh"` and `{"sh", "-c", command + "; exec " + shell}`, and have `execShellOrHookAndExit` call it — one declaration of the shape, with the existing hydrate suites as the tripwire.
- Add two seams to `resumeWaitConfig`: `ClearMarker func() error` (production: `state.UnsetResumePendingMarker(tmux.DefaultClient(), tmux.PaneIDTarget(cfg.Pane))`) and `LookupResume func(hookKey string) (hooks.OnResume, error)` (production: `loadHookStore()` then `LookupOnResume(hookKey, hooks.ViaHydrate)`, a nil store reporting the zero result and no error).
- Replace `resumeAnswerEnter`'s body with, in this order and no other: write `hydrateResetPreamble` to stdout; call `ClearMarker`; on a non-nil error restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload with `Report` set to the error's own text, returning without touching the store; otherwise call `LookupResume`, restore the terminal, emit the existing `exec` INFO (`target`, `args`, `hook_present`) and `ExecSelf` either `hookExecArgs(result.Command, resolveShell())` when the result is found with a non-empty command, or `resolveShell()` alone when it is not.
- Keep the store's existing degradation exactly as the helper has it: a lookup error emits the existing `hook lookup` DEBUG plus the existing WARN and falls through to a bare shell, and a miss emits the `hook lookup` miss DEBUG and does the same — the marker is already cleared on both, so the pane is no longer waiting whatever the read returned.
- Cover in `cmd/state_resume_enter_test.go` over the `ClearMarker`, `LookupResume` and `ExecSelf` seams plus a `logtest.Sink`, asserting call **order** (a recorder shared by the stdout writer and the clear seam) as well as outcomes.

**Acceptance Criteria**:
- [ ] The leave sequence is written to stdout before `ClearMarker` is called, and `LookupResume` is not called until `ClearMarker` has returned nil — asserted on a shared order recorder, not on three independent call counts.
- [ ] A `ClearMarker` that returns an error produces exactly one `resume-draw` exec carrying the same command, hook key, pane and pane key plus `--report` holding the error's text; `LookupResume` is never called and no shell or hook exec happens.
- [ ] After a failed clear the redrawn panel is the waiting panel with both key hints live — the redraw is a fresh draw of the same screen, not a decision about whether to draw.
- [ ] A found registration execs `/bin/sh -c "<command>; exec <$SHELL>"`, the same shape the helper already uses for a hook; the `exec` INFO carries `hook_present` true.
- [ ] A miss, an empty command and a lookup error each exec `$SHELL` alone with `hook_present` false, and the marker is cleared in every one of those cases.
- [ ] The command executed is the one `LookupResume` returned at the moment of the answer, not the `--command` the process was launched with — proved with a seam returning a different command from the one in the payload.
- [ ] A `ClearMarker` that returns nil for an already-absent marker is indistinguishable from one that cleared a set marker: the answer proceeds either way.
- [ ] The terminal is restored before every exec on this path, including the redraw after a failed clear.
- [ ] An exec that returns (the exec failed) terminates non-zero through the existing `defaultExecShell` shape — one WARN, `log.Close(1)`, `osExit(1)` — rather than returning to the read loop.
- [ ] `$SHELL` unset still resolves `/bin/sh` through the existing `resolveShell`, and the hydrate helper's own exec suites pass unchanged after the composition is extracted.

**Tests**:
- `"it leaves the panel's screen before it clears the marker"`
- `"it clears the marker before it reads the store"`
- `"it redraws the panel with the reason when the clear fails"`
- `"it runs neither the hook nor the shell when the clear fails"`
- `"it keeps both key hints live on the redraw after a failed clear"`
- `"it runs the command the store holds at the moment of the answer"`
- `"it drops the pane to a plain shell when the entry has gone"`
- `"it drops the pane to a plain shell when the store is unreadable"` (marker still cleared)
- `"it treats an already-absent marker as cleared"`
- `"it runs the hook in the shape the helper already uses"` (argv equality with `execShellOrHookAndExit`'s for the same command)
- `"it restores the terminal before every exec"` (table over the four outcomes)
- `"it terminates non-zero when the exec fails"`

**Edge Cases**:
- The order is load-bearing — the pane leaves the panel's screen, then the marker clears, then anything runs. Nothing is at risk in between: the freeze is still in force and the pane is showing its own transcript, so a tick landing there captures what is really in the pane.
- A clear that fails runs neither the hook nor the shell and redraws the panel with both hints live: handing the pane over with the marker still set would freeze that pane's saved scrollback for the rest of the pane's life — the pane goes on being used and every reboot restores the transcript it held when it paused — and nothing reports that state or reclaims it.
- The reason stands on the card until the next key rather than timing out; the waiter starts no timer, so the report's lifetime is the screen's.
- The store is read at the moment of the answer and never carried from the draw. A pane can wait for days, and in that time the entry can be removed by `portal hook rm --pane-key`, rewritten by a re-registration, or hand-edited. The `--command` in the payload is what the panel *displays* — which is why a post-discard redraw can name a registration that is already gone — and what runs is what the store holds when Enter is pressed.
- An entry removed while the pane waited drops it to a plain shell with the marker cleared, exactly as an unregistered pane does — a path the helper already has.
- An unreadable store drops it to a plain shell on the existing degradation with the marker cleared either way: the existing behaviour is that a lookup failure gives a bare shell so the pane stays usable, and that is unchanged.
- A marker already absent reads as cleared rather than as a failure — `set-option -pu` on a pane that never carried the option exits 0, measured against tmux 3.7c, so nothing here reads before it clears.
- The command runs in the shape the hydrate helper already uses for a hook so the pane still closes on the first `exit`: `sh -c '<HOOK>; exec $SHELL'` replaces the chain's child slot, and the chain's tail finds no marker and adds no second shell.
- An exec that fails terminates the way the helper's exec failure already does rather than leaving a live pane with no process — the just-emitted exec marker must not stand as a phantom handoff.

**Context**:
> Enter hands the pane over in place; nothing is wiped and nothing is re-laid. The replayed transcript is already in the pane's primary buffer, underneath the alternate screen the panel is drawn on, so leaving the alternate screen reveals it and the resume command starts over it. The pending marker is cleared before the hook runs.
>
> The command is read at the moment the user answers, not carried from when the panel was drawn. Acting on a value read days earlier would resume something the user had already deregistered — the one case where the two readings differ, and the one where the stale reading is plainly wrong. Re-reading costs a single file read at a moment already doing far more.
>
> The marker is cleared on both answer paths only once the pane has left the panel's screen and is showing its own transcript again. Clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus the card — the whole failure, in the space between two steps, on the path every resume takes.
>
> A freeze that cannot be lifted holds the answer. The pane leaves the panel's screen first, as every answer does; if the marker cannot then be cleared, the panel is drawn again carrying the reason on its report row, and the key can be pressed again from it. This is the shape a discard that cannot be written already takes: what the screen claims and what the pane holds never disagree.
>
> The specification calls the report row's content "the reason" and states no fixed wording for it, unlike every fixed string on either screen. This task's call is that the row carries the error as it was reported — Portal authors no sentence for it — which matches how `hook rm` already surfaces a tmux failure in tmux's own words, and the renderer already sanitises and truncates whatever string it is handed.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §6.1, §7.2, §5.3, §4.3

## lazy-resume-on-attach-4-4

### Task 4.4: The chain's tail: a waiter that dies leaves a usable pane

**Problem**: A waiter can stop being the pane's process without having handed the pane over — a `pkill portal`, a reclaim, a read error, or a draw that could not exec the waiter at all. The pane's only process is then gone and tmux closes the pane; on an install where 43 of 44 live sessions hold a single pane, the session closes with it and the next capture drops it from the saved set with its whole transcript. The recovery cannot be unconditional, though: a pane that *was* answered has already exec'd into the user's shell, and a recovery step that ran anyway would hand it a second one — `exit` would have to be pressed twice to close a restored pane, a regression against behaviour the repo already has a test for.

**Solution**: `portal state resume-recover` — the tail of the chain the helper parks, which reads the pending marker to tell the two apart. A pane no longer carrying one was answered, so the step does nothing at all and the parked shell exits with it. A pane still carrying one (or one whose marker could not be read) is recovered: off the panel's screen, marker cleared, a WARN if that clear did not land, and the user's shell exec'd whether or not it did.

**Outcome**: A killed, crashed or reclaimed waiter leaves the pane alive with its transcript above it, the session intact, the marker cleared so capture resumes, and the registration untouched — while an answered pane still closes on the first `exit`.

**Do**:
- Add `ReadPaneOption(target Target, option string) (string, error)` to `internal/tmux/tmux.go` beside `SetPaneOption` / `UnsetPaneOption`, taking the same two-read shape `ResolveHookKey` already takes: a `show-options -p -t <target>` existence probe naming **no** option, whose non-zero exit is returned as the error before anything else runs, then `display-message -p -t <target> -F "#{<option>}"` for the value. A single format read cannot carry this method's contract — measured against tmux 3.7c on 2026-09-20, `display-message -p -t <target> -F '#{@opt}'` answers a target no live pane answers to with exit 0 and an empty string, for a gone pane id, a bogus `%999` and a gone `=session:w.p` alike, which is byte-for-byte what an unset option on a live pane reads as. The probe naming no option exits 0 for a live pane whether or not it carries options and exits 1 `no such pane` otherwise; naming the option collapses the discrimination (`invalid option`, exit 1, for an unset option on a live pane), which is why it names none. The target is spent as a string at each argv and the failure is wrapped in the shape its siblings use. Pin both argvs and their order in `internal/tmux/tmux_test.go` through `commandertest.Scripted`, and extend `internal/tmux/pane_option_realtmux_test.go` with a set → read → unset → read round trip and a read against a target no live pane answers to.
- Add `cmd/state_resume_recover.go` with `resumeRecoverConfig` (`Pane`, `PaneKey string`, `Stdout io.Writer`, `Logger *slog.Logger`, `ReadMarker func() (string, error)`, `ClearMarker func() error`, `ExecShell func(prog string, args []string)`), `runResumeRecover(cfg) error`, the `resumeRecoverRunFunc` seam, and the hidden `stateResumeRecoverCmd` on `stateCmd` taking `--pane` and `--pane-key`. Production binds `tmux.DefaultClient()` with `tmux.PaneIDTarget(cfg.Pane)` and `state.ResumePendingOption`, and `defaultExecShell`.
- Body: call `ReadMarker`; when it returns no error and `state.ResumePendingSet(value)` is false, return nil having written nothing, cleared nothing and exec'd nothing. Otherwise — the marker is set, or the read itself failed — write `hydrateResetPreamble`, call `ClearMarker`, emit `hydrateLogger.Warn("unset resume pending marker failed", "pane_key", cfg.PaneKey, "error", err)` when it fails, then emit the existing `exec` INFO (`target`, `args`, `hook_present` false) and `ExecShell(resolveShell(), …)` either way.
- Read nothing else: the tail opens no hooks store, no prefs, no theme and no renderer — pin it with a source assertion over `cmd/state_resume_recover.go` alongside the behavioural tests.
- Cover in `cmd/state_resume_recover_test.go` over the three seams plus a `logtest.Sink`, asserting the do-nothing path writes zero bytes and the recovery path's exact order.
- Extend the options clause of CLAUDE.md's `tmux` package row to name the pane-option pair the marker is written and read through — `UnsetPaneOption` alongside `SetPaneOption`, and `ReadPaneOption` as the single-pane read: a `show-options -p -t <target>` existence probe naming no option followed by the `display-message -F` format read, taking that shape for the same reason `ResolveHookKey` does, since the format read alone answers a target no live pane answers to with exit 0 and an empty string.

**Acceptance Criteria**:
- [ ] A marker read returning an empty value with no error produces: no write to stdout, no `ClearMarker` call, no exec, and a nil return.
- [ ] A marker read returning `1` produces, in order: the leave sequence on stdout, one `ClearMarker` call, and one `$SHELL` exec.
- [ ] A marker read that returns an error is treated as still pending — the pane is recovered exactly as a set marker is.
- [ ] A `ClearMarker` that fails still execs the shell, and emits exactly one WARN carrying `pane_key` and `error` under the `hydrate` component; a clear that succeeds emits no WARN.
- [ ] The WARN's message is distinct from the helper's failed-mark WARN, so a grep separates "came back eager because a write failed" from "is wrongly frozen".
- [ ] The leave sequence is written before `ClearMarker` is called — the pane is showing its own transcript before its protection is dropped.
- [ ] `tmux.ReadPaneOption` composes `show-options -p -t <target>` naming no option followed by `display-message -p -t <target> -F "#{<option>}"`, in that order, takes `tmux.Target`, reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to — on the probe's exit status, because the format read alone answers exit 0 and empty there; `internal/tmux/target_composition_guard_test.go` passes with it in place.
- [ ] The command reads no `hooks.json`, no `prefs.json` and no theme, and calls no renderer.
- [ ] The command's log records are the WARN above and the existing `exec` INFO, and nothing else — no new event and no new attr key.

**Tests**:
- `"it does nothing for a pane that was already answered"`
- `"it recovers a pane whose marker is still set"`
- `"it treats a failed marker read as still pending"`
- `"it execs the shell even when the clear failed"`
- `"it records a failed clear as a WARN naming the pane and the error"`
- `"it records nothing when the clear succeeded"`
- `"it leaves the panel's screen before it drops the protection"`
- `"it reads no store and draws nothing"` (source assertion + seam call counts)
- `"it composes the existence probe and then the format read"` (scripted commander, both argvs asserted in order)
- `"it reads back a set and an unset pane option"` (real tmux)
- `"it fails a read against a target no live pane answers to"` (real tmux)

**Edge Cases**:
- A pane whose answer already ran is given no second shell so `exit` still closes a restored pane on the first press: the answered pane exec'd its own shell into the chain's child slot, and a tail that handed it another would put a second shell above it. It reads the pending marker to tell the two apart — a pane still marked was never answered and is recovered; a pane no longer marked was, and the chain simply ends.
- The pane leaves the panel's screen before its protection is dropped, because a tick landing while the card is still up writes that pane's saved transcript as history-minus-its-last-screenful plus the card, and a `pkill portal` puts every waiting pane on the install through that window at once.
- The shell runs whether or not the clear landed because a closed pane is the worse failure: a clear that did not land leaves the pane looking entirely normal while its saved transcript stands still, which is bad — but a pane that closed takes its session and its whole transcript with it.
- A clear that failed is recorded as the WARN that makes a wrongly-frozen pane findable, under the existing hydrate catalog and attr keys. The rule that holds an answer until the marker clears is unavailable here by definition — there is no waiter left to hold it — so the record is the only thing that makes the state findable.
- A marker read that itself fails is treated as still pending rather than as answered: a live pane carrying an extra shell is the lesser failure against a pane that closes under the user.
- A read against a target no live pane answers to fails on the probe's exit status and never on the format read — measured against tmux 3.7c on 2026-09-20, `display-message -p -t <gone pane> -F '#{@opt}'` exits 0 with an empty string, exactly what an unset option on a live pane reads as, so a single format read would report a pane that does not exist as one that was answered. `internal/tmuxtest`'s `ReadPaneToken` already documents that trap and `ResolveHookKey` already takes the probe-then-read shape against it; this method is the third reader to need the same discrimination and takes the same shape rather than a fourth.
- The tail is reached when the draw could not exec the waiter and not only when the waiter died — the helper composes the chain with `;`, so a non-zero draw still falls through to it.
- Nothing in the tail reads the store or draws anything: it exists to make a pane usable, and a tail that consulted a registration could resurrect a decision the user has already had taken away from them.
- A hand `respawn-pane -k` is outside what the tail recovers, and nothing here attempts it. It kills the pane's command, which is the parked shell the tail runs in, so the tail dies with the waiter and the pending marker stays set on a pane that is now an ordinary shell — its saved scrollback frozen at the moment it paused, while the picker dot and the pending count go on claiming a decision is waiting there. The marker is destroyed only with its pane and there is no address by which a sweep could reach it, and the operation is the user destroying the process that held that pane's state — the same class as a hand edit of the store.

**Context**:
> A waiter that exits without having handed the pane over drops the pane to a plain shell. It runs as the tail of a chain that takes the pane off the panel's screen, clears the pending marker, and then execs the user's shell — the shape the hydrate helper already uses for a hook. Those two steps keep the order every answer takes. A killed, crashed or reclaimed waiter therefore leaves the pane alive with its transcript above it, the session intact, the marker cleared so capture resumes, and the registration untouched, so the next reboot offers the panel afresh. Without it the pane closes — and on an install where 43 of 44 sessions hold a single pane, the session closes with it and the next capture drops it from the saved set with its whole transcript. The cost is a resident shell parent per waiting pane, a megabyte or so on top of the floor.
>
> The chain hands the pane over whether or not the clear succeeded, and records a clear that failed. A closed pane is the failure the fallback exists to prevent, so the shell runs either way. A clear that did not land leaves the second way a pane can be wrongly frozen, and the worse of the two: the pane looks entirely normal while its saved transcript stands still, and the picker dot and the pending count both go on claiming a decision is waiting there.
>
> **Corrigendum 2026-09-19**: a pane that has been answered is handed no second shell. The chain's recovery step must do nothing at all for a pane that was answered normally, or the shell an answered pane exec'd into would fall through into a second one and the user would have to type `exit` twice to close a restored pane. It reads the pending marker to tell the two apart, and a marker read that itself fails is treated as still pending.
>
> The specification calls the failed-clear record "the same WARN a marker that could not be written already gets". This task's call is the same **treatment** rather than the same message string: both are one WARN under the `hydrate` component carrying `pane_key` and `error`, and they are worded apart because they name materially different states — one pane came back eager, the other is wrongly frozen — which the specification itself distinguishes as "the worse of the two". The pair mirrors the existing `unset skeleton marker failed` naming.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.3, §7.2, §7.3, §8.2, Corrigendum 2026-09-19 (the chain's recovery step)

## lazy-resume-on-attach-4-5

### Task 4.5: The helper decides, marks and hands the pane to the panel

**Problem**: The whole chain exists and nothing reaches it — the hydrate helper still fires every registration unconditionally the moment replay finishes, so no pane has ever waited. The decision has to land in one particular place. The pending marker must be written **before** the mid-restore marker is cleared, because a gap where neither is set is a one-tick window in which the saver truncates the pane's saved transcript and writes the card over the end of it. And the helper has three tails, not one: the path where scrollback replayed, the tail it takes when the hydrate signal never arrives, and the tail it takes when the saved scrollback file is missing. All three clear the mid-restore marker inside their own handler and all three fire the hook today, so deciding the mode on some of them and not others would make whether a pane waits depend on whether its replay happened — which is not a distinction the user made.

**Solution**: Resolve the mode once, at the top of the helper — one store read and one prefs read — and carry the decision to whichever tail runs. Every clear of the mid-restore marker is preceded by the pending mark when the decision is to wait, and a mark that cannot be written downgrades the decision to eager with one WARN. The helper's exec then either fires as it does today or parks a shell running the draw followed by the chain's tail.

**Outcome**: A restored pane whose registration resolves lazy comes back holding the panel with its marker already set; a pane with no registration and one that resolves eager come back exactly as they do today; and a pane that could not be marked lands the user where eager would have put them, with the fall-through recorded.

**Do**:
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim — keeping the helper's existing absent-store guard so a hook store the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when that loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
- Add `markPendingThenUnsetSkeletonMarker(cfg hydrateConfig)` and call it in place of `unsetSkeletonMarkerOrLog` at all three sites (the replay path in `runHydrate`, `handleHydrateTimeout`, `handleHydrateFileMissing`). When the decision says wait it first resolves `$TMUX_PANE` and `resumeChainExe()` and calls `state.SetResumePendingMarker`; any of the three failing emits exactly one `hydrateLogger.Warn("set resume pending marker failed", "pane_key", …, "error", …)` and sets `Decision.Wait = false`. On the path where all three succeed it records the resolved values on the decision as `Pane` and `Exe`, before the marker is written, so the chain is composed from the values the refusal was taken over and nothing downstream resolves either again. It then calls `unsetSkeletonMarkerOrLog(cfg)` unchanged.
- Branch in `execShellOrHookAndExit` on a **nil-tolerant** `cfg.Decision`. A nil decision is today's behaviour unchanged: the function performs its own `LookupOnResume`, emits its own `hook lookup` DEBUG records and its own lookup-failure WARN, and execs the hook or the bare shell exactly as it does now — so the eight direct callers in `cmd/state_hydrate_exec_log_test.go` and `cmd/hooks_read_lock_test.go` compile and pass unmodified. A non-nil decision skips that read: when `cfg.Decision.Wait` it composes and execs the chain, otherwise it takes today's path reading the command off `cfg.Decision.Lookup`.
- Add `execResumeChainAndExit(cfg hydrateConfig)` composing `shellWords(resumeChainArgv(cfg.Decision.Exe, "resume-draw", payload)) + "; " + shellWords(resumeChainArgv(cfg.Decision.Exe, "resume-recover", payload))`, where `shellWords` (in `cmd/state_resume_chain.go`) quotes every argv element through `shellquote.Single` and joins with a space; emit the existing `exec` INFO and `cfg.ExecShell("/bin/sh", []string{"sh", "-c", chained})`. The payload carries the command from the decision's lookup, the hook key, `cfg.Decision.Pane` as the pane id and the pane key from `state.PaneKeyFromFIFOPath(cfg.FIFO)`; the report is empty on a first draw. This function resolves nothing of its own — it is only ever reached on a decision whose `Exe` and `Pane` are already filled.
- Add `cmd/state_hydrate_lazy_test.go` covering all three tails against injected `HookStore`, `Client` and `ExecShell` seams plus a `logtest.Sink`; re-run the existing hydrate suites and `internal/restore`'s `TestExitClosesRestoredPane_*` / `TestNoParkedShWrapperPostRestore` unchanged.
- Edit the "Resume hooks" paragraph of CLAUDE.md where it states the helper execs `sh -c '<HOOK>; exec $SHELL'` or a bare `$SHELL`: it now resolves the registration's mode first and, when that resolves lazy, parks a shell running the draw followed by the chain's tail instead.
- Edit the README everywhere it tells the reader a registered command runs by itself on restore. In the `xctl hook` section: the opening paragraph ("re-executes automatically when a session is attached after a reboot"), the rename paragraph ("A renamed session still re-runs its command after the next reboot" — the hook still survives the rename, but what comes back is the panel), and the **When hooks fire** paragraph ("resume hooks run only when Portal recreates a pane from saved state"). In the **Automatic Server Bootstrap & Restoration** section: the sentence ending "and resume hooks run on the recreated panes", and the "Pair restoration with resume hooks to re-run pane commands such as dev servers and editors after a reboot" line that reads the same way. Under the shipped default the pane comes back holding the resume panel showing that command, with `⏎ resume` and `d discard`, and the command runs when the user answers; `eager` is the mode that keeps the fire-on-restore behaviour those sentences describe. Wording the executor's, naming no particular tool.

**Acceptance Criteria**:
- [ ] A pane with no registration, and one whose mode resolves eager, produce byte-identical behaviour to today on all three tails: no `set-option -p`, no pending marker, the same skeleton-marker clear, the same `exec` argv and the same log records — covering both routes to eager, a registration pinned `eager` under the shipped lazy install and a registration naming no mode under an install whose `resume_mode` is `eager`.
- [ ] A registration pinned `lazy` under an install whose `resume_mode` is `eager` still waits, so the override beats the install in both directions where the decision is taken and not only inside the resolution function.
- [ ] `execShellOrHookAndExit` called with a nil `Decision` is byte-identical to today: its own lookup, the same `hook lookup` hit/miss/error DEBUG records, the same lookup-failure WARN and the same exec — the eight existing direct callers pass with no edit.
- [ ] `TestNoParkedShWrapperPostRestore`, `TestExitClosesRestoredPane_NoHook` and `TestExitClosesRestoredPane_WithHook` pass unmodified — the parked shell exists only on the lazy path.
- [ ] On each of the three tails in turn, a lazy registration produces the pending `set-option -p` **before** the skeleton marker's unset, asserted on a shared call-order recorder.
- [ ] A lazy registration execs `/bin/sh -c "<draw argv>; <recover argv>"`, with `;` and not `&&`, and with every interpolated value — the command above all — single-quoted, so a command holding spaces, quotes, `$`, backticks or a newline reaches the draw's flag parser as one token.
- [ ] The recover half of that chain carries `--pane` and `--pane-key` alone, so the tail the parked shell runs after the waiter has gone starts and recovers the pane rather than failing its flag parse and taking the pane down with it.
- [ ] A `SetResumePendingMarker` that fails, an absent `$TMUX_PANE` and an unresolvable executable each fire the hook exactly as an eager registration does, and each emits exactly one WARN carrying `pane_key` and `error`.
- [ ] A prefs read that fails resolves the install default to lazy (the shipped default) and the pane waits; nothing about the failure fails the pane.
- [ ] A helper run whose hook store could not be built resolves to no registration — today's miss record, no marker, no wait, a bare shell — and one whose prefs store could not be resolved takes the shipped default; neither ends the helper, and the pane restores on one of the three tails either way.
- [ ] An unreadable store is today's bare shell and never a wait: the lookup error path emits its existing DEBUG and WARN, `Wait` is false, and no marker is written.
- [ ] Exactly one `LookupOnResume` and one `LoadResumeMode` call happen per helper run, whichever tail it ends on — asserted with counting seams on all three tails.
- [ ] The chain is composed from the executable path and the pane id the mark step resolved: each is read exactly once per helper run, and the argv the chain carries names those values — so no second resolution can fail after the marker has been written.
- [ ] A registration whose stored command is empty is not a registration: no marker, no wait, and the pane falls through to a plain shell.
- [ ] Scrollback replay, the FIFO open/read/unlink, the settle sleep, the reset preamble and postamble, and the `scrollback replayed` / `signal timeout` / `scrollback missing` records are all unchanged.
- [ ] No bootstrap step, step ordering, eager signal pass or global hook is touched by this task.
- [ ] No README sentence still states that a registered command re-executes by itself after a reboot: the `xctl hook` section's opening, rename and **When hooks fire** paragraphs, and the **Automatic Server Bootstrap & Restoration** section's two resume-hook sentences, all describe the panel a lazy registration comes back holding and name `eager` as the mode that keeps today's behaviour.

**Tests**:
- `"it restores a pane with no registration exactly as today"` (table over the three tails)
- `"it falls back to its own lookup when no decision was resolved"` (nil `Decision`; records and exec unchanged)
- `"it restores an eager registration exactly as today"` (table over the three tails)
- `"it restores a registration naming no mode eagerly under an eager install"` (table over the three tails)
- `"it waits for a registration pinned lazy under an eager install"`
- `"it marks the pane pending before it clears the mid-restore marker"` (table over the three tails)
- `"it parks the draw and the tail in one shell for a lazy registration"`
- `"it quotes the command into the chain"` (table: spaces, single quotes, `$(…)`, backtick, newline)
- `"it separates the draw and the tail with a semicolon"`
- `"it composes the tail with the pane flags alone"`
- `"it fires the hook eagerly when the marker cannot be written"`
- `"it fires the hook eagerly when TMUX_PANE is absent"`
- `"it fires the hook eagerly when the executable cannot be resolved"`
- `"it records one WARN naming the pane and the error that refused the marker"` (table over the three refusals)
- `"it resolves the shipped default when the prefs read fails"`
- `"it resolves the shipped default when the prefs store could not be built"`
- `"it treats a hook store that could not be built as no registration"`
- `"it gives an unreadable store a bare shell and never a wait"`
- `"it reads the store once and the prefs once per pane"` (table over the three tails)
- `"it treats an empty stored command as no registration"`

**Edge Cases**:
- A nil `Decision` is today's behaviour rather than a panic. `execShellOrHookAndExit` is reached directly by eight existing suites that build a `hydrateConfig` without one, and resolving the decision is `runHydrate`'s job — a call that arrives without one performs the lookup itself, which is exactly what the function does today. Making the field's absence mean "nobody decided" is what keeps the new branch beside the existing behaviour rather than inside it.
- A pane with no registration and one that resolves eager are byte-identical to today, marker and all — the eager path is the whole of today's behaviour and the new branch sits beside it rather than inside it, which is what keeps the existing no-parked-shell and first-`exit`-closes-the-pane guarantees intact.
- The mode is resolved and the marker set before the mid-restore marker is cleared on every tail the helper ends on — the replay, the signal timeout and the missing scrollback file alike. Deciding it after the clear would open the very window this rule closes: the saver truncates the pane's saved transcript and writes the card over the end of it, the whole failure in the space between two steps.
- A pane that cannot be marked fires the hook as an eager registration does and records one WARN naming the pane and the error that refused it: a wait with no marker on the pane is the one state the design refuses, and landing the user where eager would have put them is the degradation this feature already accepts.
- An absent `$TMUX_PANE` is a pane that cannot be marked — there is no target to write the option against, so it takes the same fall-through and the same record as a failed write.
- An executable that cannot be resolved is treated the same way, and for the same reason: the refusal has to be taken **before** the marker is written, because a marked pane whose chain could not be composed would fire its hook with the marker still set and freeze its saved scrollback for life. A PATH lookup is deliberately not used as a fallback — it can find a different build than the one restoring.
- A prefs read that fails resolves to the shipped default rather than failing the pane, so an install whose `prefs.json` is unreadable meets panels rather than processes: a panel can be answered in a keystroke, while a resume the user did not want cannot be taken back.
- An unreadable store is today's bare shell and never a wait — there is no command to put on a panel, and the existing degradation keeps the pane usable.
- One store read and one prefs read per pane, carried to whichever tail runs: the reads happen once at the top, so no tail pays for them twice and no tail skips them.
- The command reaches the chain without being laundered through an unquoted shell word: it is arbitrary user-authored text crossing one shell boundary, and naive concatenation there corrupts the command — which is exactly what `internal/shellquote` exists to declare once.
- Scrollback replay, the FIFO handling, the eager signal pass and every bootstrap step are untouched: nothing in the restore path branches on whether a pane has an unfired hook, and the insertion is contained to the one process that was already the last thing to run in a restored pane.

**Context**:
> **Corrigendum 2026-09-19**: the mode is resolved and the marker written once, ahead of whichever clear runs, so whether a pane waits does not depend on whether its replay happened. The helper's two degraded tails — the hydrate signal never arriving, and a missing saved scrollback file — clear the mid-restore marker inside their own handlers and fire the hook today, and both must take the lazy branch as the replay path does.
>
> The pending marker is set before the mid-restore marker is cleared, and therefore before the panel is painted, so the pane is never unprotected.
>
> Only a pane that is going to wait is marked. The helper resolves the pane's mode before it clears the mid-restore marker, so a pane with no registration — and one whose registration resolves eager — is never marked and goes on being captured exactly as it is today. That condition is also what holds the unreachability of the inverse failure: the marker only ever lands on a pane whose sole process is the waiter, which dies with the pane.
>
> A pane that cannot be marked does not wait. If the pending marker cannot be written, the helper does not paint: it fires the hook as an eager registration does, and the pane comes back as today's restore leaves it. The fall-through is recorded — one WARN naming the pane and the error that refused the marker, one more event on the existing hydrate catalog, not a new component. This is the only degradation the feature introduces that the user cannot read off the pane in front of them.
>
> The helper execs a parked shell running the draw followed by the tail, and the draw execs the waiter in place. The parked shell is what buys the fall-through the specification already accepts the cost of, and every later screen — a resize, a report, the discard confirmation — is an exec inside that same child slot, so the chain's shape is fixed once and the tail is never re-entered while the pane is answered.
>
> The registration is read once, by the helper, and the command is carried into the chain; the draw never decides whether to draw. That is what makes the post-discard redraw possible at all.
>
> Nothing in the restore pipeline changes except the helper's own tail. No new bootstrap step, no change to step ordering, no change to the eager signal pass, and no change to the global hooks.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §7.3, §8.2, §9, §9.1, §2, §3.1, Corrigendum 2026-09-19 (the mode resolved ahead of every clear)

## lazy-resume-on-attach-4-6

### Task 4.6: One redraw once the size has settled

**Problem**: The panel is painted at a size read once, and then nothing ever re-reads it. A pane whose window is resized holds a card centred on a width that no longer exists, or — when the pane shrinks below the card — a frame clipped by a pane that can no longer hold it, with the key hints among the rows that went. Answering every size change with a fresh draw is not the fix either: a terminal dragged to a new size delivers a stream of size changes to every pane in the window, and a full waiting set answering each of them would be hundreds of process launches a second for the length of the drag. The panel holds nothing that moves, so one draw at the end of the stream is the whole of what it owes.

**Solution**: The waiter watches SIGWINCH and arms a settle window that each further change restarts. When the window elapses it reads the pane's size once and hands the pane over to a fresh draw — carrying the command and the report forward — only if that size differs from the one the panel was drawn at. Keys go on acting throughout.

**Outcome**: A drag of any length costs one handover per pane; a resize that ends where it started costs none; and the resting process after a redraw is a fresh wait rather than a draw that stayed.

**Do**:
- Add `resumeResizeSettle = 150 * time.Millisecond` to `cmd/state_resume_wait.go` and three seams on `resumeWaitConfig`: `Winch <-chan os.Signal` (production: `signal.Notify` over `syscall.SIGWINCH`), `Settle func(time.Duration) <-chan time.Time` (production: `time.After`), and `Size func() (int, int, error)` (production: the same `term.GetSize` read the draw takes).
- Restructure the read loop into a `select` over three sources, with a **request-driven** one-byte reader goroutine: the loop sends on a request channel, the goroutine performs exactly one `Read` and sends the byte back on an unbuffered channel. Only one read is ever outstanding, so the loop never reads ahead of the byte it is waiting on and task 4.2's inherited-bytes guarantee holds for every key the loop dispatches.
- On each `Winch` signal, arm `Settle(resumeResizeSettle)`, replacing whatever timer was armed; draw nothing on the signal itself.
- When the settle timer fires: call `Size`; when it errors, hand over to a redraw at the value it reported (the draw resolves a non-positive or failed size to its own bounded fallback); when it equals `cfg.Width` and `cfg.Height`, drop the window and go on waiting with no exec at all; otherwise restore the terminal, emit the `exec` INFO and `ExecSelf` `resumeChainArgv(exe, "resume-draw", cfg.resumeChainPayload)` — the payload unchanged, so the command and the report ride across.
- Keys keep priority and keep their meaning: a byte arriving while a settle window is open is dispatched exactly as task 4.2 dispatches it, and the pending redraw dies with the process image.
- Extend `cmd/state_resume_wait_test.go` (or a sibling) driving `Winch` and `Settle` as test channels, and re-run task 4.2's full swallow table against the restructured loop unchanged.

**Acceptance Criteria**:
- [ ] A burst of ten `Winch` signals delivered before the settle window elapses produces exactly one `Size` call and exactly one hand-off.
- [ ] Each signal arriving while a window is open re-arms the timer — asserted by counting `Settle` calls and proving the first timer's fire is not acted on after a re-arm.
- [ ] A settled size equal to `--width`/`--height` produces no exec, no write and no terminal restore; the loop is still reading afterwards and a subsequent key still acts.
- [ ] A settled size differing in either dimension produces exactly one `resume-draw` exec carrying the same `--command`, `--report`, `--hook-key`, `--pane` and `--pane-key` the waiter was launched with.
- [ ] A non-empty `--report` survives the redraw unchanged, so a reported reason is still on the card after a resize.
- [ ] A key delivered while a settle window is open is dispatched immediately and produces its own hand-off; no second exec follows from the pending timer.
- [ ] A `Size` that errors at settle time produces a redraw rather than ending the wait, and the argv it produces is the one the draw resolves to its bounded fallback.
- [ ] Task 4.2's swallow table, terminal-restore table and non-terminal/raw-mode refusals all pass unchanged against the restructured loop.
- [ ] At most one read is outstanding at any moment, so the loop never reads ahead: a burst delivered after the byte the loop dispatched is still queued for the next process image.
- [ ] Nothing on the redraw path resolves a theme or renders — the redraw is a handover, so the resting process after it is a fresh wait.
- [ ] Task 4.2's signal guard passes unchanged with the resize and settle seams in place: the wait path's only `signal.Notify` names `syscall.SIGWINCH`, and SIGHUP, SIGTERM and SIGINT keep their default disposition.
- [ ] The settle timer's firing neither ends the wait nor clears the report, held by the two criteria above rather than by a guard: a matching settled size leaves the loop reading with a subsequent key still acting, and a differing one carries `--report` across unchanged.

**Tests**:
- `"it draws once for a burst of size changes"`
- `"it restarts the settle window for a change arriving inside it"`
- `"it redraws nothing when the settled size matches the drawn size"`
- `"it hands over to a redraw when the settled size differs"`
- `"it carries the command and the report across a redraw"`
- `"it dispatches a key pressed while the settle window is open"`
- `"it redraws at the bounded fallback when the size read fails"`
- `"it keeps waiting after a no-op settle"`
- `"it keeps one read outstanding at a time"`
- `"it swallows every non-acting key after the restructure"` (task 4.2's table, re-run)

**Edge Cases**:
- A burst of size changes produces exactly one draw: the panel holds nothing that moves, so one draw at the end of the stream is the whole of what it owes, and the cost of a resize stays a single handover per pane.
- A change arriving inside the settle window restarts it rather than firing a second draw — otherwise a slow drag would fire one draw per settle period for its whole length.
- A settled size equal to the size the panel was drawn at redraws nothing: a window dragged out and back, or a resize of a sibling pane that leaves this one alone, costs no process launch at all.
- A key pressed while the settle window is open still acts — the wait is the pane's interaction surface and a pending redraw must never make a key wait for a timer.
- The redraw carries the command and the report forward so a reported reason survives a resize: the report stands until the next key, and a resize is not a key.
- The redraw is a handover so the resting process after it is a fresh wait rather than a draw that stayed — a resize is the same handover run backwards, and the resting state goes back to the floor.
- A size read that fails at settle time redraws at the bounded fallback rather than ending the wait: ending it would hand the pane to the chain's tail and drop the panel over a transient read failure.
- A byte arriving in the instant between the settle timer firing and the hand-off exec is lost: the outstanding read has already taken it off the tty queue, and the loop — having selected the settle branch — never receives it before the process image is replaced. The window is one scheduling gap wide and sits on the redraw path, where the inherited-bytes guarantee is already bounded by the draw's appearance probe. Nothing is built on a byte surviving it, and closing it would mean either reading ahead, which loses the guarantee outright, or arming a second timer on a wait path that is forbidden one.
- The settle duration is not stated by the specification, which says only that the redraw is taken "once the size has settled". 150 ms is this task's call: a drag delivers changes every few tens of milliseconds, so the window closes only when the drag stops, and it is short enough that a deliberate single resize redraws without a perceptible pause.

**Context**:
> A resize is the same handover run backwards: the waiter replaces itself with a fresh draw, which draws at the new width and hands back to a fresh wait. The resting state stays at the floor and the cost is paid only at the moment of the resize.
>
> The redraw is taken once the size has settled, not once per size change. A terminal dragged to a new size delivers a stream of size changes to every pane in the window, and a full waiting set answering each of them with a fresh draw would be hundreds of process launches a second for the length of the drag. The panel holds nothing that moves, so one draw at the end of the stream is the whole of what it owes.
>
> Every screen the pane shows takes that same handover — a resize, a report, and the discard confirmation alike — so the process holding a pane between screens carries the wait and nothing else, and the resting cost of a waiting set does not depend on how many of its panes the user has stopped to look at.
>
> A pane too small for the card still says what it is: below the size the card needs, the panel degrades instead of disappearing, and Enter and `d` act at every size. Which of the two forms a redraw produces is the renderer's decision from the size it is handed, not this task's.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.2, §5.2, §4.3

## lazy-resume-on-attach-4-7

### Task 4.7: A restored pane comes back holding the panel

**Problem**: Every piece of the chain has been verified against seams — a scripted reader, a recorded exec, an injected marker writer. The property the feature actually promises is one no seam can prove: that a real `portal` binary restoring a real saved session into a real tmux pane comes back holding the panel with a live waiter in it, the user's transcript intact underneath, and that Enter reveals that transcript and starts the registered command over it. Three things can only fail there — a real `sh -c` parsing the quoted chain the helper composed, a real pane's alternate screen holding the panel over a real replayed buffer, and a real process tree that must be a parked shell and a waiter rather than a stack of them. And the eager path has to go on working in the same run, because the feature ships with both modes live on one install.

**Solution**: An integration-tagged real-tmux suite that reboots a two-session fixture — a lazy subject and an eager control — through the real restore orchestrator with a built binary, then asserts the pane through Portal's own capture read and through `capture-pane`, and drives Enter and `exit` from tmux.

**Outcome**: One run proves the feature end to end: the lazy pane waits and is reported pending, the eager pane fires as it always did, Enter resumes over the recovered transcript, and the pane closes on the first `exit`.

**Do**:
- Add `internal/restore/lazy_resume_panel_integration_test.go` (`//go:build integration`, `package restore_test`, `-short` skip, `tmuxtest.SkipIfNoTmux`), with a setup helper modelled on the existing `setupExitClosesPane`: `restoretest.BuildPortalBinaryDir(t)`; `portaltest.IsolateStateForTest(t)` plus `t.Setenv("PORTAL_STATE_DIR", …)` and `state.EnsureDir()`; `PORTAL_HOOKS_FILE` and `PORTAL_PREFS_FILE` pointed into `t.TempDir()`; `portaltest.RegisterStateDirTeardownGuard(t, stateDir)`; `tmuxtest.New(t, "ptl-lazy-")`.
- Seed two sessions on that socket: a **lazy subject** whose `hooks.json` entry is the string form (so it inherits the shipped lazy default with no `prefs.json` key set at all) and an **eager control** whose entry carries `resume: eager`. Give the subject's window a second pane holding a plain shell and carrying no registration — the specification's worked example, a left pane that waits beside a right pane the user works in. Stamp each registered pane's own token with `ts.StampPaneToken`, print a distinct recognisable line into each pane before the capture, and give each hook command its own sentinel file so the two cannot be confused. Author the subject's command so it carries a space and an embedded single quote, and keep it short enough to render inside the card's wrapped rows — it is both the string the panel has to show back and the string that crosses the shell the helper composed.
- Capture with `state.CaptureStructure`, write `sessions.json` through `state.EncodeIndex`, `restoretest.RebootServer`, restore through `restoretest.NewRestoreOrchestrator` + `restoretest.RestoreWithMarker`, then `restoretest.DriveSignalHydrate` + `restoretest.WaitForSkeletonMarkersCleared`.
- Assert in ordered subtests: (a) `state.CaptureStructure`'s pending set holds the subject pane's live key and not the control's; (b) `capture-pane -p` on the subject returns the panel's title, the registered command as it is stored, and both key hints, while `capture-pane -a -p` returns the pre-reboot line underneath it; (c) the control's sentinel appears within `restoretest.PaneReactionBudget` and its `capture-pane -p` shows its transcript and no panel; (d) the subject's process tree is exactly one `sh -c` parent over one `portal state resume-wait`, with no `resume-draw` still resident and no second shell; (e) with the subject still waiting and unanswered, take a second capture by the same route the first took — `state.CaptureStructure`, the per-pane scrollback dump for every pane it did not report pending, then `state.EncodeIndex` — and assert the subject's record still names the scrollback file its pre-reboot line was filed under and that the file's bytes are unchanged; then `restoretest.RebootServer`, restore and `restoretest.DriveSignalHydrate` a second time and assert the subject comes back with the panel on `capture-pane -p`, the pre-reboot line intact under it on `capture-pane -a -p`, `@portal-resume-pending` set again, and its live pane key back in the fresh capture's pending set; (f) after `send-keys Enter`, the marker clears (poll `tmux.ReadPaneOption`), the subject's sentinel appears, and `capture-pane -p` shows the pre-reboot line with the command's output over it; (g) after `send-keys exit`, the pane is gone within the existing budget on the first press.
- Take the process-tree reading read-only and scoped: resolve the subject's `#{pane_pid}` from the fixture socket and run `ps -o pid,ppid,rss,command` over that pid and its descendants alone. The suite signals nothing and enumerates no process it did not cause to exist. Report the resting tree's combined `rss` in the test's own output whether it passes or fails, so the figure the memory case rests on is readable from a run rather than only from a failure.
- Keep it in the integration lane with `-p 1` in mind: no `t.Parallel`, the built binary via `restoretest`/`portalbintest` only, and `tmuxtest.New`'s own cleanup killing the disposable server.

**Acceptance Criteria**:
- [ ] The lazy subject's pane key is in `CaptureStructure`'s pending set and the eager control's is not, so the freeze and the wait agree on the same pane.
- [ ] `capture-pane -p` on the subject returns the panel — its title, the registered command and both key hints — and `capture-pane -a -p` returns the line the pane held before the reboot, intact underneath it.
- [ ] The command the panel shows is the stored command as stored, its space and its embedded single quote included, so the quoting that carried it across the helper's shell is proved at the pane and not only at the argv.
- [ ] The eager control fires its hook within the existing pane-reaction budget in the same run, shows no panel, and carries no pending marker.
- [ ] The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`, and no `portal state resume-draw` survives the hand-off.
- [ ] The subject's resting tree — the parked shell plus the waiter, measured once the pane is waiting and the draw is gone — carries a combined resident size below 22 MB, the ceiling the daemon was measured at, and the figure is reported by the test whether it passes or fails.
- [ ] While the subject holds the panel, a command sent to its sibling pane with `send-keys` runs in that pane and its output is readable from `capture-pane -p` on the sibling — the waiting pane takes no key that was not sent to it.
- [ ] The sibling pane carries no pending marker, is absent from the capture's pending set, shows no panel, and its scrollback file is written while the subject's is not.
- [ ] A second capture taken while the subject is still waiting leaves its scrollback file byte-unchanged and carries its previous record forward, so the file the saved state names after that capture is the one written before the pane ever paused.
- [ ] Restored a second time from that capture, the subject comes back holding the panel with the pre-reboot line intact underneath it, its pending marker set, and its key in the fresh capture's pending set — an unanswered offer returns rather than being spent, and nothing of the transcript is lost across the second reboot.
- [ ] `send-keys Enter` clears `@portal-resume-pending` on the subject within a bounded poll, produces the subject's sentinel, and leaves the pre-reboot line visible with the resumed command's output over it.
- [ ] `send-keys exit` closes the subject's pane on the first press, within the same budget the existing restored-pane suite uses.
- [ ] Every assertion about what Portal concludes goes through Portal's own reads (`state.CaptureStructure`, `tmux.ReadPaneOption`) and every assertion about what the pane shows goes through `capture-pane`; raw tmux is used only to stage the fixture and to send keys.
- [ ] The suite carries `//go:build integration`, uses `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket and a `restoretest`-built binary, spawns no `portal state daemon`, and leaves no server or subprocess behind.
- [ ] The suite signals no process and enumerates only the pid tmux reports for its own pane.

**Tests**:
- `"it reports a lazy pane as pending and an eager one as not"`
- `"it shows the panel over the pane's own transcript"`
- `"it shows the stored command on the panel through the shell the helper composed"`
- `"it fires an eager registration in the same run"`
- `"it carries one shell parent and one waiter"`
- `"it holds the pane below the resident ceiling the daemon was measured at"`
- `"it leaves the pane beside it live"` (a command sent to the sibling runs there while the subject waits)
- `"it goes on capturing the pane beside it"`
- `"it keeps the waiting pane's transcript through a capture it was never answered through"`
- `"it offers the panel again on the reboot after the one that drew it"`
- `"it clears the marker and runs the command on Enter"`
- `"it reveals the transcript that was underneath the panel"`
- `"it closes the pane on the first exit after the resume"`

**Edge Cases**:
- The assertions read the pane through Portal's own capture and through `capture-pane` rather than through the helper's internals: what matters is what Portal concludes about the pane and what a user would see in it, not what a seam was handed.
- A lazy pane is reported pending by the capture read so the freeze and the wait agree on the same pane — the marker the helper wrote is the one the saver's skip reads, and a suite that only checked the panel would miss a wait that was never protected.
- An eager registration on the same server restores exactly as today in the same run: the two modes ship live together, and a regression that only shows when both are present is exactly the one a single-mode fixture would miss.
- A pane captured while it is still waiting is the only case in which the freeze's whole purpose is observable: its saved record is the token-matched merge's output, its scrollback file is one nothing has rewritten since it paused, and the panel that comes back is drawn afresh from a registration that was never fired. Every other check of those three stops at an in-memory index, a computed reference set, or the path a record names — none of them reaches the bytes a second restore replays, and the loss they are guarding against is silent when it happens.
- Enter reveals the transcript that was underneath the panel and runs the registered command over it — the alternate screen is the whole mechanism by which the panel hides a transcript it never touched, and nothing short of a real pane demonstrates it.
- The pane closes on the first `exit` after the resume as a restored pane does today: the parked shell's tail must find no marker and add no second shell, which is the corrigendum's whole point and is not observable without a real process tree.
- The resident size is asserted against the daemon's 22 MB ceiling rather than against the ~2 MB estimate: the estimate is a guess at what Portal's startup touches, and pinning a test to it would fail on a change that costs nothing, while the ceiling catches the failure that matters — a draw that did not hand off, or a wait path that kept the rendering pages resident, which would put a full waiting set back where the eager path was.
- Panes beside a waiting one are fully live throughout, and nothing short of a real two-pane window shows it: a waiting pane that captured the client's keyboard is exactly what ruled out the dead pane and every floating overlay, and it is invisible to every seam-driven test in the feature.
- The pane's process tree carries one shell parent and one waiter rather than a stack of them — the specification accepts exactly one resident shell parent per waiting pane, and a chain that accumulated more would reinstate the resident cost the work exists to remove.
- Integration lane with isolated state, a disposable socket and no daemon: the suite builds and execs a portal binary, so it belongs behind the tag; it needs no saver, so nothing here may spawn one.
- The process-tree reading is the one place this suite looks outside tmux. It must resolve pids from the fixture's own pane and signal nothing — the developer's live tmux server and their real `portal state daemon` are present during every run.

**Context**:
> A pane whose resume is lazy holds a live Portal process that draws the panel and blocks until the user answers. The helper restore already puts in each pane does not finish: it lays down the scrollback as today, then — instead of handing the pane to the hook — draws the panel and waits.
>
> Everything else about restore is unchanged. Skeleton, geometry and scrollback replay exactly as today, so the pane still reads as restored — the transcript the user left there sits in the pane's primary buffer the whole time the panel waits, and is revealed the moment the panel goes.
>
> The panel is painted into the pane's alternate screen, so it never enters the scrollback. Measured on tmux 3.7c: a pane printed two lines of real content, entered the alternate screen, and painted a card. `capture-pane -p` returned the card; `capture-pane -a -p` returned the two original lines, intact underneath.
>
> Scrollback replays exactly as it does today, for every restored pane, whether or not a resume is pending. Nothing in the restore path branches on whether a pane has an unfired hook.
>
> A second bootstrap does not disturb a waiting pane, and bootstrap's two sweeps leave it alone — the stale-marker sweep only unsets markers whose pane is no longer live, and the orphan-FIFO sweep has nothing to reclaim because the helper unlinks its FIFO as soon as the hydrate signal arrives.
>
> The repo's existing restored-pane suites (`TestExitClosesRestoredPane_*`, `TestNoParkedShWrapperPostRestore`) are the shape this one is built from, and they remain the guarantee for the eager path: the parked shell this feature introduces exists only on the lazy path.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.1, §5.1, §6.1, §9, §9.1, §1, Corrigendum 2026-09-19 (the chain's recovery step)

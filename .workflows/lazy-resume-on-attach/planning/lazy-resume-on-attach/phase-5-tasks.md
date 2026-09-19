# Phase 5: Discarding a resume from the panel — 6 tasks

## lazy-resume-on-attach-5-1

### Task 5.1: The store's discard route records what it destroyed

**Problem**: The panel's `d` answer removes a user-authored `on-resume` command that has no other copy anywhere, and `internal/hooks` has no route that records what went. `Remove` records only which entry went (`op=rm`, `hook_key`, `via`, no `value`) — the typed-command treatment, where the user named the key on a command line and knows what they typed. The discard is the stale sweep's situation instead: it is reached by a keystroke on a panel the user is walking through, so the removed command itself belongs in the log, in the recoverable form an operator copies back out of it. The surface that reaches the store also has no name — the closed `Via` vocabulary's four members (`cli`, `internal`, `hydrate`, `doctor`) name a typed command, Portal acting on its own behalf, a hydration lookup and a diagnosis, and the waiter is none of them.

**Solution**: `Store.Discard` beside `Store.Remove`, both driven by one shared removal body so the two routes cannot drift in what they delete, differing only in the op they log and whether that line carries the removed command; plus `ViaPanel` added to the closed `Via` vocabulary.

**Outcome**: One call removes a pane's `on-resume` registration and leaves behind an INFO line carrying the command it destroyed under `op=discard` / `via=panel`, greppable apart from both `rm` and `clean-stale`, with every other entry and every other event on the same key untouched.

**Do**:
- Add `ViaPanel` to `internal/hooks/via.go` after `ViaDoctor`, with the wire value `panel` in `viaNames`, and add its row to `TestViaWireValues` in `internal/hooks/via_test.go` beside the existing four and the unset-zero case.
- Factor `Store.Remove`'s body in `internal/hooks/store.go` into `removeEntry(key string, event Event, via Via, op string, carryValue bool) (bool, error)`: return `(false, nil)` for an empty `key` before `acquireMutationLock` is reached, so no lock is taken, no sidecar is created, no file is loaded and none is written; capture the `Registration` being deleted before the `delete`; emit the lock-failure and save-failure WARNs exactly as `Remove` emits them today but under the `op` it was given; on success emit `logger.Info(op, "op", op, "hook_key", key, "via", via.String())`, with `"value", removed.Command` appended when `carryValue` is set.
- Reduce `Remove` to `return s.removeEntry(key, event, via, "rm", false)` and add `Discard(key string, event Event, via Via) (bool, error)` returning `s.removeEntry(key, event, via, "discard", true)`, documented as the panel's removal route.
- Cover the new route in `internal/hooks/discard_test.go`, staging fixtures through `hookstest.StageStore(t, hookstest.Staging{…})` and reading records through a `logtest.Sink` with `logtest.AssertRecord` and `logtest.AssertWriteFailure`; keep the existing `Remove` suites untouched as the drift tripwire for the shared body.
- Edit the `hooks.json.lock` sidecar paragraph of CLAUDE.md so its list of mutation acquires — "a `hook set`, a `hook rm`, or a clean's deletion phase" — also names the panel discard.
- Edit the first paragraph of CLAUDE.md's "Resume hooks" section to name the panel discard as a removal route beside `hook rm` and a hand edit, stating that it removes the pane's `on-resume` entry and nothing else and unstamps no token. Do **not** touch the "A key the staleness rule cannot judge is retained forever" paragraph or its sentence about sanctioned removals: the discard only ever removes token-shaped keys baked from saved state, so it never reaches an old-format entry and that sentence is not stale.

**Acceptance Criteria**:
- [ ] `Discard` removes the named key's `on-resume` entry and emits exactly one INFO under the `hooks` component carrying `op=discard`, the `hook_key`, `via=panel`, and `value` holding the removed command.
- [ ] A key holding other events alongside `on-resume` keeps those events and keeps its outer key; a key holding only `on-resume` is deleted whole, exactly as `Remove` deletes it.
- [ ] The `value` attr carries the command out of an object-form entry byte-identically to the same command stored in the string form.
- [ ] A key naming no entry, and a key whose `on-resume` event is absent, each report `(false, nil)`, write no file and emit no breadcrumb.
- [ ] An empty hook key reports `(false, nil)` without acquiring the mutation lock, without creating the `hooks.json.lock` sidecar, and without loading or saving the file.
- [ ] A save that fails and a mutation lock that cannot be acquired each report `(false, err)` with the entry still on disk; the save failure's WARN carries the existing `error` plus the `error_class` `fileutil.ClassifyWriteError` returns, and the lock failure's carries `error` and no `error_class`.
- [ ] A discard's record and a removal's record are distinguishable on both the slog message and the `op` attr, so `grep op=discard`, `grep op=rm` and `grep op=clean-stale` each select exactly one removal route.
- [ ] `hooks.Via(0).String()` is still the empty string and `hooks.ViaPanel.String()` is `panel`.
- [ ] `Remove`'s existing suites pass unmodified, and `hook rm`'s exit-status contract — exit 0 iff it removed an entry — is unchanged.
- [ ] `internal/hooks`'s leaf guard passes with no new entry: the route takes no dependency through which a tmux call could be made, so no pane token can be unstamped from it.

**Tests**:
- `"it removes the on-resume entry and records the command it destroyed"`
- `"it keeps the key's other events and its key"`
- `"it deletes the key whole when on-resume was its last event"`
- `"it records the command out of an object-form entry"` (table: string form, object form carrying a mode)
- `"it removes nothing and writes nothing for a key naming no entry"` (table: absent key, key with no on-resume event)
- `"it reaches no mutation for an empty hook key"` (no sidecar created, file byte-unchanged)
- `"it reports no removal when the save fails"` (`logtest.AssertWriteFailure`)
- `"it reports no removal when the mutation lock cannot be taken"` (`hookstest.HoldHooksSidecar`, `hookstest.AssertLockWarn`)
- `"it renders the panel via as panel"`
- `"it still renders an unset via as absent"`
- `"it keeps discard and rm greppable apart"` (both records asserted on message and `op`)
- `"it leaves every other entry byte-unchanged"`

**Edge Cases**:
- The breadcrumb is emitted from the store method as every other mutation's is, rather than from the waiting program: the store is the chokepoint, so one grep reconstructs every removal whichever surface drove it, and a second emission at the call site would double-count the act.
- A key holding events beside `on-resume` keeps them and keeps its key — the discard removes a registration, not a pane's whole hook record.
- A key naming no entry removes nothing, writes nothing, and is not an error: the caller's own answer to that case is the phase's "a discard that finds nothing to remove is still a discard", which needs a clean `(false, nil)` rather than a failure to act on.
- An empty hook key reaches no mutation. `hook rm` already decides this before the store is consulted so no empty key reaches a mutation; the shared body makes that property structural rather than dependent on each caller.
- A failed save and an unavailable lock each report no removal with the entry left standing, carrying the existing `error` and `error_class` attrs — the caller reports the refusal on the confirmation and the registration survives, which is only true if the store never claims a removal it did not make.
- `discard` and `rm` stay greppable apart so the three removal routes are distinguishable in one log: the typed command, the automatic sweep and the panel are three different acts with three different recoveries.
- The unset zero `Via` still reads as absent — the vocabulary stays numbered from one, so a zero-valued `Via` never impersonates whichever surface happens to be first.
- The route makes no tmux call, so no pane token is unstamped: discarding removes a registration and never touches the pane's durable identity.

**Context**:
> The removed command is written to the log as it goes. The confirmation makes the act deliberate; the log line makes it recoverable anyway, and costs nothing.
>
> Portal already destroys these entries two ways and treats them differently. The typed removal command records only which entry went, while the automatic stale sweep deliberately records the command itself — the recoverable form an operator copies back out of the log. This path is the sweep's situation rather than the typed command's, so it takes the sweep's treatment.
>
> The emission is one INFO line under the `hooks` component carrying the hook key and the removed command as `value`, at the production default level. It adds two members to closed vocabularies, amended in the specification because a specification is the sanctioned route and a call site is not: a new `op`, `discard`, so the three removal routes stay greppable apart; and a new `via`, `panel`, because the existing four name none of them — the waiter is neither a typed command, nor Portal acting on its own behalf, nor a hydration lookup, nor a diagnosis.
>
> A confirmed discard removes the pane's resume registration, permanently, and nothing else. Discarding neither unstamps the pane's durable token nor touches any other entry. It is a third removal route alongside `portal hook rm` and a hand edit of the store, reached from where the user already is instead of by remembering a CLI verb.
>
> `resume-hooks-silently-lost` owns hook removal — the `hook rm` CLI, the rule that removing nothing always exits non-zero, and that a removal never unstamps the pane's durable token. This route adds a surface beside that CLI and contradicts none of those rules; because the key it removes is always a token baked from saved state, it also never touches the old-format entries that specification retains permanently.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §6.4, §6.2

## lazy-resume-on-attach-5-2

### Task 5.2: The confirmation is a screen the chain can draw

**Problem**: Phase 3 built `tui.RenderResumeDiscardConfirm` and nothing calls it. The chain's drawing process renders exactly one screen: `runResumeDraw` reads the pane's size, resolves the theme, writes the alternate-screen entry and paints `tui.RenderResumePanel`, then hands off to a waiter. Everything on that path is identical for the confirmation — the same size read, the same theme resolution, the same alternate-screen entry, the same hand-off — and so is the waiter's raw-mode loop, its resize settle and its restore-before-exec discipline. A fourth hidden subcommand would duplicate both, and would have to join the process-role argv table, where the closed role space is deliberately not growing for this feature.

**Solution**: A `--screen` selector carried in the chain payload and honoured at the single point the two screens differ — which renderer the draw calls — so both screens share one draw path, one wait path and one argv shape.

**Outcome**: `portal state resume-draw --screen discard` paints the discard confirmation into a pane through exactly the machinery the waiting panel is painted through, the selector rides the hand-off so the waiter knows which screen is in front of the user, and every argv the chain composed before this task is byte-unchanged.

**Do**:
- In `cmd/state_resume_chain.go`: add `screen` to the flag-name constants; add `Screen string` to `resumeChainPayload`; declare `resumeScreenPanel = ""` and `resumeScreenDiscard = "discard"`; and have `resumeChainArgv` emit `--screen` only when the payload's screen is `resumeScreenDiscard`, so a payload naming the waiting panel composes the argv it composes today.
- Add `renderResumeScreen(s tui.ResumeScreen, screen string) string` in `cmd/state_resume_draw.go`: `tui.RenderResumeDiscardConfirm(s)` for `resumeScreenDiscard`, `tui.RenderResumePanel(s)` for every other value including the empty one.
- Route `runResumeDraw`'s paint through it, leaving the body order from task 4.1 untouched — size, theme, alternate-screen entry, cursor home, paint, `exec` INFO, hand-off — and leaving the payload it hands to `resume-wait` unchanged, so the screen rides across with the command, the report, the hook key, the pane and the size.
- Register `--screen` on both `stateResumeDrawCmd` and `stateResumeWaitCmd`, populating `Screen` on the payload each builds.
- Cover in `cmd/state_resume_draw_screen_test.go` over the draw's existing seams (`Size`, `ResolveTheme`, `ExecSelf`, `Stdout`), asserting the painted bytes against the production renderers and asserting one seam-call sequence shared by both screens.

**Acceptance Criteria**:
- [ ] `--screen discard` paints bytes byte-identical to `tui.RenderResumeDiscardConfirm` called with the same `ResumeScreen`; the waiting panel's bytes are still byte-identical to `tui.RenderResumePanel`.
- [ ] An absent `--screen`, an empty one, and an unrecognised value (`"panel"`, `"DISCARD"`, `"x"`) each paint the waiting panel, so no chain argv composed before this task changes meaning.
- [ ] `resumeChainArgv` for a payload naming the waiting panel is byte-identical to the argv it composed before this task, and the discard payload's argv differs only by `--screen discard`.
- [ ] A draw of either screen produces the same seam-call sequence — one `Size`, one `ResolveTheme`, one alternate-screen entry write, one paint, one `ExecSelf` — so the theme resolution, the size read, the alternate-screen entry and the hand-off are one path.
- [ ] The `resume-wait` argv the draw execs carries `--screen discard` when the confirmation was drawn, and carries no `--screen` when the panel was.
- [ ] A non-empty `--report` renders on whichever screen is drawn, on that screen's own report row.
- [ ] At a pane size below the card's, the confirmation degrades to the plain stack exactly as the panel does — the size ladder is the renderer's and this task adds no second decision.
- [ ] `log.ResolveProcessRole` is unchanged: no subcommand name is added, so the closed role space gains no member.
- [ ] `stateResumeWaitCmd` accepts `--screen` and carries it onto its payload, so a waiter always knows which screen is in front of the user.

**Tests**:
- `"it paints the confirmation through the production renderer"`
- `"it paints the waiting panel for an absent, empty or unrecognised screen"` (table)
- `"it composes the waiting panel's argv unchanged"`
- `"it adds only the screen flag to the confirmation's argv"`
- `"it takes one path for both screens"` (seam-call sequence equality)
- `"it carries the screen onto the waiter"` (table: panel, discard)
- `"it renders the report on whichever screen is drawn"` (table: panel, discard)
- `"it degrades the confirmation with the pane"` (table over sizes, including one below the card's)
- `"it adds no process role"` (`internal/log`, existing table re-run)

**Edge Cases**:
- An absent or unrecognised screen selector draws the waiting panel, so every existing chain argv is unchanged: the panel is the chain's default screen and the flag is additive, which is what keeps task 4.5's composed chain and task 4.6's redraw correct with no edit.
- The confirmation draw is byte-identical to calling the production renderer with the same screen values — there is no second layout and no wrapper that could drift from what Phase 3 signed off.
- The theme resolution, the size read, the alternate-screen entry and the hand-off are one path serving both screens: a second subcommand would fork all four, and the two screens would diverge first in the places nobody renders in a test.
- The screen rides the exec to the waiter so the keys offered and the keys dispatched cannot disagree — a waiter that guessed which screen was painted could offer `y` against a panel or `d` against a confirmation.
- A resize while the confirmation is up redraws the confirmation and not the panel: the settle redraw hands the payload across wholesale, so the screen travels with the command and the report.
- The size ladder decides card-or-plain-stack for the confirmation exactly as for the panel, because both renderers route through the same ladder — this task chooses a renderer and nothing else.
- No fourth subcommand is added, so the closed process-role space gains no member: a selector adds no argv name at all, which is the whole reason it is a flag.
- A report carried in the payload renders on whichever screen is drawn — the confirmation's report row is the same single row the waiting panel gives one.

**Context**:
> The discard confirmation is the kill modal, retitled. Nothing structural differs, which is the point: it is the same act the picker's kill confirm performs, so it is the same object, built through the same shared destructive-confirm builder.
>
> Every screen the pane shows takes that same handover. A wait is not one draw: `d` puts up the confirmation, Escape brings the card back, and an answer that cannot be carried out redraws the card with its report row. Each is a fresh draw that hands back to a fresh wait, exactly as a resize does.
>
> The waiting pane is three hidden `state` subcommands rather than one — the draw, the wait and the chain's tail. All three map to the existing hydrate process role and emit under the existing hydrate log component, so the closed role space and the closed component vocabulary gain no member.
>
> This phase's structural call: the confirmation is a screen selector on the existing chain commands rather than a fourth hidden subcommand. The draw's theme resolution, size read, alternate-screen entry and hand-off are identical for both screens, as are the waiter's raw-mode loop, its resize settle and its restore-before-exec discipline; only the renderer and the dispatch table differ. A fourth command would duplicate both and would have to join the process-role argv table, where a selector adds no argv name at all.
>
> The dispatch table is task 5.3's; this task renders the screen and carries the selector.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.4, §4.2, §5.2

## lazy-resume-on-attach-5-3

### Task 5.3: `d` opens the confirmation and Escape backs out

**Problem**: The waiter dispatches two keys against one screen. `d` currently hands the pane back to a fresh draw of the waiting panel — task 4.2's placeholder — so the confirmation the chain can now paint is unreachable, and there is no screen on which `y` and Escape mean anything. The keys also have to be *screen-scoped* rather than global: Enter resumes on the panel, so an Enter that acted on the confirmation would confirm an irreversible deletion with the key that means "bring it back" one screen earlier, and Escape must stay inert on the panel — binding the reflex key to an irreversible deletion would make this the one place in Portal where it destroys something.

**Solution**: Make the waiter's byte dispatch a function of the screen it was launched on: the panel's table keeps Enter and `d`, and the confirmation's table offers `y` and Escape and nothing else. `d` and Escape are hand-offs to a fresh draw of the other screen, and `y` hands over to a fresh draw of the confirmation until task 5.5 gives it the removal.

**Outcome**: `d` puts the confirmation up, Escape brings the waiting panel back unchanged, Enter is swallowed on the confirmation, everything task 4.2 swallowed on the panel is swallowed on both screens, and both keys act at every pane size.

**Do**:
- In `cmd/state_resume_wait.go`, split the read loop's dispatch on `cfg.Screen`. On `resumeScreenPanel`: `\r` and `\n` → `resumeAnswerEnter`, `d` → `resumeOpenDiscardConfirm`, every other byte swallowed (task 4.2's behaviour, unchanged). On `resumeScreenDiscard`: `y` → `resumeAnswerDiscard`, `\x1b` → `resumeCancelDiscardConfirm`, every other byte — `\r`, `\n`, `d`, `Y`, `0x03`, `0x04`, `0x1a`, printable text — swallowed.
- Add `resumeOpenDiscardConfirm(cfg) error`: restore the terminal, emit the existing `exec` INFO (`target`, `args`), and `ExecSelf` `resumeChainArgv(exe, "resume-draw", p)` where `p` is `cfg.resumeChainPayload` with `Screen` set to `resumeScreenDiscard` and `Report` cleared.
- Add `resumeCancelDiscardConfirm(cfg) error`: the same shape with `Screen` set to `resumeScreenPanel` and `Report` cleared.
- Re-point `resumeAnswerDiscard` — declared by task 4.2 as the panel's `d` dispatch — to be the confirmation's `y` dispatch: restore the terminal, emit the `exec` INFO, and hand over to a fresh draw of the confirmation (`Screen` unchanged, `Report` cleared). Task 5.5 replaces this body with the removal.
- Leave task 4.6's settle redraw exactly as it is: it execs `resumeChainArgv(exe, "resume-draw", cfg.resumeChainPayload)` with the payload untouched, so a resize keeps both the screen and the report.
- Extend `cmd/state_resume_wait_test.go` (or a sibling `cmd/state_resume_screens_test.go`) to run task 4.2's full swallow table against **both** screens, and to assert each hand-off's composed argv.

**Acceptance Criteria**:
- [ ] `d` on the waiting panel produces exactly one hand-off, to a `resume-draw` argv carrying `--screen discard`, the same `--command`, `--hook-key`, `--pane`, `--pane-key` and size, and no `--report`.
- [ ] Escape on the confirmation produces exactly one hand-off, to a `resume-draw` argv carrying no `--screen` and no `--report` — the waiting panel back as it was drawn.
- [ ] `\r` and `\n` on the confirmation produce no hand-off, no write and no state change, and the loop is still reading afterwards.
- [ ] `d` on the confirmation produces no hand-off — the screen cannot reopen itself.
- [ ] Uppercase `Y` on the confirmation produces no hand-off, matching the picker's two existing destructive confirmations, which take lowercase only.
- [ ] Escape on the waiting panel is inert: no hand-off, no write, loop still reading.
- [ ] The first byte of an escape sequence (`\x1b` of `\x1b[A`) backs out of the confirmation, and its remaining bytes are then swallowed by the panel's table on the next process image; no timer is armed to disambiguate.
- [ ] `0x03`, `0x04`, `0x1a`, and a run of ordinary printable text produce no hand-off on either screen, and neither screen's loop exits on them.
- [ ] Both hand-offs clear `Report`, so `d` opens a confirmation with nothing to report and Escape returns a panel with nothing to report, whatever the waiter was launched carrying.
- [ ] The terminal is restored before every hand-off exec on both screens, so the next process image inherits a cooked tty.
- [ ] Every dispatch above behaves identically at `--width`/`--height` below the card's size and at non-positive values.
- [ ] `y` on the confirmation produces exactly one hand-off, to a fresh draw of the confirmation, and removes nothing — no store is opened on this task's paths.
- [ ] A settle redraw while the confirmation is up execs a `resume-draw` carrying `--screen discard` and the report it was launched with.

**Tests**:
- `"it opens the confirmation on d"`
- `"it backs out to the waiting panel on Escape"`
- `"it swallows Enter on the confirmation"` (table: `\r`, `\n`)
- `"it swallows d on the confirmation"`
- `"it swallows uppercase Y on the confirmation"`
- `"it leaves Escape inert on the waiting panel"`
- `"it backs out on the first byte of an escape sequence"`
- `"it swallows every non-acting key on both screens"` (task 4.2's table, run per screen)
- `"it clears the report on both hand-offs"` (table: `d` from a reported panel, Escape from a reported confirmation)
- `"it restores the terminal before every hand-off"` (table over the four acting keys)
- `"it acts on both keys at a size below the card's"` (table over sizes, per screen)
- `"it hands over to a fresh confirmation on y"` (no store opened, argv asserted)
- `"it redraws the confirmation on a settled resize"`

**Edge Cases**:
- Enter is swallowed on the confirmation, so the reflex of confirming with Enter costs one more press of the key the footer names. Enter was rejected as the confirm key because it resumes on the panel one keystroke earlier, and the same key would then mean "bring it back" and "delete it forever" on consecutive screens.
- `d` is swallowed on the confirmation so the screen cannot reopen itself — the pane never acts on a key the screen in front of the user does not offer.
- Uppercase `Y` is swallowed as it is by the picker's two existing destructive confirms: the confirm key is lowercase only, and widening it here would make this confirmation laxer than the two it is modelled on.
- Escape stays inert on the waiting panel and is live only inside the confirmation. There is nowhere to back out to on the panel, and an inert Escape reads as cleaner than a dangerous one — a key the user's hands press without consulting them can then never be the key that loses work.
- An escape sequence's first byte backs out of the confirmation, which is the harmless direction and needs no timer: the waiter is forbidden any timer of its own, so there is no disambiguation window, and an arrow key therefore cancels rather than confirms.
- A report dies at the next key press, so `d` opens a clean confirmation and Escape returns a clean panel. The specification says a report stands until the next key rather than timing out; `d` and Escape are key presses, while a resize is not — which is why task 4.6's carry-forward is unchanged.
- Ctrl-C, Ctrl-D and Ctrl-Z are swallowed on the confirmation exactly as on the panel: the waiter is still the pane's only process on both screens, so anything that killed it would take the pane with it.
- Both keys act at every pane size including below the card's, where the frame is gone and the parts stack plainly — a confirmation that could not be answered at a small size would leave the user pressing the key the footer offered a moment earlier.
- The terminal is restored before every hand-off exec, so no process image inherits a raw tty it did not set.
- `y` hands over to a fresh confirmation draw until the discard lands, so no intermediate state can remove a registration by a half-built route: until task 5.5 the confirm key is inert in the safe direction.

**Context**:
> `d` opens a second confirmation over the panel — *this is permanent* — where `y` agrees and Escape backs out to the resume panel.
>
> While the confirmation is up, `y` and Escape are the only keys that act. Enter, `d` and everything else are swallowed there exactly as they are on the waiting panel — the pane never acts on a key the screen in front of the user does not offer, and the reflex of confirming with Enter costs nothing but a second press of the key the footer names.
>
> Escape on the waiting panel does nothing at all. There is nowhere to back out to, so it is inert. Everywhere else in Portal, Escape means *back out* — it reverses, it never acts. Binding it here to an irreversible deletion would make this the one place in the product where the reflex key destroys something.
>
> The discard key is `d`, derived from the picker's existing split: `k` kills a live thing, `d` deletes a persisted record. Nothing is killed here — the session and the pane both survive — and what goes is a stored registration. The confirm key is `y`, matching Portal's two existing destructive confirmations, both of which take `y` with `esc` to cancel through one shared builder.
>
> This phase's calls, applied here: a report's lifetime ends at the next key press, stated once so the two screens do not each invent their own rule; an escape sequence's first byte backs out of the confirmation, because the waiter is forbidden any timer and cancelling is the harmless direction; and task order puts the input-drop guard (task 5.4) before the key that destroys anything (task 5.5), so the confirm key is a redraw until then.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §6.2, §6.3, §4.3, §5.4

## lazy-resume-on-attach-5-4

### Task 5.4: The confirmation refuses input that was already in flight

**Problem**: `d` and `y` are ordinary characters in ordinary text — `cd ~/dev && yarn` contains both, in order — so a line delivered to the wrong pane opens the confirmation and agrees to it from the same buffer, destroying the only copy of a user-authored command with nobody seeing either screen. Task 5.3 makes that reachable: the waiter reads one byte at a time and never reads ahead, so the rest of a burst is still sitting in the pane's tty input queue when `d` is dispatched, and that queue survives every exec the chain performs. Nothing in Portal touches a tty input queue today.

**Solution**: The hand-off that *opens* the confirmation carries a one-shot flag; the drawing process honours it by discarding the tty's input queue before it paints anything, and clears it from the payload it hands on. A drop that cannot be performed never paints the confirmation at all — it paints the waiting panel carrying the reason on its report row.

**Outcome**: A burst carrying `d` and then `y` opens the confirmation and does not confirm it; a keystroke arriving after the confirmation is on screen still confirms it; a resize redraw of a confirmation the user is already reading drops nothing; and the input queue is touched in exactly one place in the chain.

**Do**:
- Add `cmd/tty_flush_darwin.go` and `cmd/tty_flush_linux.go`, each build-tagged for its platform and declaring `flushTTYInput(fd int) error`: darwin issues `unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, freadOnly)` over a file-local `freadOnly = 0x1`; linux issues `unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)`. Comment the darwin constant in one line — it is the `<sys/fcntl.h>` value `x/sys/unix` does not export on that platform.
- Add `drop-input` to the chain's flag-name constants and `DropInput bool` to `resumeChainPayload`; `resumeChainArgv` emits `--drop-input` only when it is set. Register the flag on `stateResumeDrawCmd` only, so a stray `--drop-input` on a `resume-wait` argv fails the parse instead of being ignored.
- Add the seam `DropInputQueue func() error` to `resumeDrawConfig`, bound in production to `flushTTYInput` over stdin's fd. As `runResumeDraw`'s **first** step — before the size read and before a byte is written — call it when the payload's `DropInput` is set. On a non-nil error, switch the screen about to be drawn to `resumeScreenPanel` and set `Report` to the error's text, then continue through task 4.1's unchanged body; on success continue unchanged.
- Clear `DropInput` on the payload the draw hands to `resume-wait`, so no later screen of the chain drops again.
- Set `DropInput` true on the argv `resumeOpenDiscardConfirm` (task 5.3) execs, and nowhere else: `resumeAnswerEnter`, `resumeAnswerDiscard`, `resumeCancelDiscardConfirm`, the settle redraw and the helper's first draw all compose their argv with it unset.
- Cover the chain behaviour in `cmd/state_resume_drop_input_test.go` over the `DropInputQueue` seam plus the draw's existing seams; cover `flushTTYInput` itself in `cmd/tty_flush_test.go` (a non-tty fd must error) and in a darwin-tagged `cmd/tty_flush_pty_test.go` that opens a pty through `/dev/ptmx` plus the host's grant/unlock/name ioctls, writes a burst into the master, flushes the slave, and asserts a read of the slave with a bounded deadline returns no bytes.
- Add a source assertion in the same package pinning `flushTTYInput` to exactly one call site outside its own declarations and tests.

**Acceptance Criteria**:
- [ ] `d` on the waiting panel composes a `resume-draw` argv carrying `--screen discard --drop-input`; no other hand-off in the chain composes an argv carrying `--drop-input`.
- [ ] A draw carrying `--drop-input` calls the drop seam exactly once, before the size read and before the first byte is written to stdout.
- [ ] A draw not carrying it never calls the drop seam, so a waiting-panel draw, a resize redraw and a report redraw all leave the input queue alone.
- [ ] The `resume-wait` argv the draw execs never carries `--drop-input`, so the flag dies with the draw that honoured it.
- [ ] A drop seam returning an error paints the **waiting panel** carrying that error's text on its report row, execs a waiter on the panel screen, and paints no confirmation — the confirmation is never presented.
- [ ] A drop seam returning nil paints the confirmation exactly as task 5.2 paints it, with no report.
- [ ] Nothing on the drop path reads stdin: the seam returns an error and no bytes, and the draw performs no read of its own.
- [ ] `flushTTYInput` errors for a non-tty file descriptor.
- [ ] On a real pty: bytes written to the master before the flush are not readable from the slave after it, and bytes written after the flush are.
- [ ] `flushTTYInput` has exactly one call site in the tree outside its declarations and its own tests.
- [ ] Task 4.2's inherited-bytes guarantee is unchanged on the waiting panel: a byte left in the queue when Enter hands the pane over is still there for the next process image.

**Tests**:
- `"it opens the confirmation with the drop flag set"`
- `"it drops the input queue once before it paints"` (call-order recorder over the seam and the stdout writer)
- `"it drops nothing when the flag is unset"` (table: panel draw, resize redraw, report redraw)
- `"it clears the drop flag from the hand-off"`
- `"it paints the waiting panel with the reason when the drop fails"`
- `"it paints no confirmation when the drop fails"`
- `"it reads no byte on the drop path"`
- `"it errors for a non-tty descriptor"`
- `"it discards bytes queued before the flush and keeps bytes queued after it"` (real pty, darwin-tagged)
- `"it touches the input queue in exactly one place"` (source assertion)

**Edge Cases**:
- A burst delivering `d` then `y` in one write opens the confirmation and does not confirm it: the `y` is in the tty's input queue when the `d` is dispatched, and the queue is discarded before the confirmation is painted.
- The drop happens once as the confirmation goes up and never on the waiting panel, whose inherited-bytes behaviour is unchanged — the panel is not a screen a stray byte can destroy anything from.
- A keystroke arriving after the confirmation is on screen still confirms it: the drop is a single act at the moment the screen goes up, not a mode the waiter runs in.
- A resize redraw of the confirmation does not drop again, because a key typed at a screen the user is already looking at is theirs. This is why the open-ness is carried in the payload and cleared on every subsequent hand-off rather than re-derived from the screen selector — the screen is `discard` for a resize too.
- The drop discards the queue rather than reading it, so no byte is consumed on behalf of a later screen: a read would take one byte and hand the rest onward, which is exactly the shape that lets a burst survive.
- A drop that cannot be performed leaves the confirmation unopened and reports the reason on the waiting panel. The specification states the property absolutely — only a keystroke arriving after the confirmation is on screen can confirm it — so a screen stale input could answer is never presented. This is the phase's one derivation from a stated rule rather than a stated rule itself.
- The drop is the only place the chain touches the input queue, pinned by a source assertion rather than by review: a second flush anywhere would silently eat a keystroke the user meant.
- The flush is a platform call with no portable spelling — `TIOCFLUSH` with the read-side bit on darwin, `TCFLSH` with `TCIFLUSH` on linux — so it is two build-tagged declarations behind one name, and the chain's own coverage runs through the seam on either platform.

**Context**:
> Nor can a burst of input carry the discard through both screens. `d` and `y` are ordinary characters in ordinary text — `cd ~/dev && yarn` contains both, in order — so a line delivered to the wrong pane would otherwise open the confirmation and agree to it from the same buffer, destroying the only copy of a user-authored command with nobody seeing either screen. Input already in flight when the confirmation opened is dropped rather than read as agreement: only a keystroke arriving after the confirmation is on screen can confirm it. The keys are unchanged.
>
> The waiter is the pane's only process, so anything that kills it takes the pane with it. It must refuse to die rather than exit. The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot *answer* the panel, because nothing but Enter and `d` means anything to it.
>
> This phase's calls, applied here: the input drop belongs to the confirmation being *opened*, not to every draw of it, so the open-ness is carried in the payload and cleared on every subsequent hand-off; a drop that cannot be performed does not open the confirmation; and task order puts this guard before the key that destroys anything, so the first commit in which the confirm key can remove a registration already refuses stale input.
>
> This task's own call is where the drop runs: in the drawing process, immediately before it paints, rather than in the waiter after the exec. That is what lets a failed drop choose the waiting panel instead — a drop attempted after the paint could only report on a confirmation that was already in front of the user, which is the state the rule above forbids.
>
> The darwin-only pty coverage is a deliberate limit: the flush's positive path needs a real terminal, the release builds both platforms, and the linux arm is the same one-ioctl call `tcflush(fd, TCIFLUSH)` makes. The chain's behaviour — when the drop runs, what a failure does, that nothing else touches the queue — is covered through the seam on either platform, and task 5.6 exercises the whole path in a real tmux pane.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §4.3, §6.2, §5.4

## lazy-resume-on-attach-5-5

### Task 5.5: `y` discards the registration and drops the pane to a shell

**Problem**: The confirmation is reachable, refuses stale input, and its confirm key still only redraws it. Nothing removes anything yet, and the key that will is the one the feature cannot get wrong: what it destroys is the only copy of a user-authored command. Two orderings are load-bearing and they pull in opposite directions. The removal must happen **while the confirmation is still on screen**, because a confirmation that closed on a failed write would be indistinguishable from one the user backed out of, and they would read a discard that did nothing as a discard they cancelled. And the answer must then take the same ordered shape every resume takes — the pane leaves the panel's screen, then the marker clears, then anything runs — because clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus a picture of the card.

**Solution**: Give `resumeAnswerDiscard` the removal, run before a single byte is written, with two report-and-stay exits — a refused write redraws the confirmation, a refused marker clear redraws the waiting panel — and one exit that drops the pane to a plain shell.

**Outcome**: `y` removes that pane's registration permanently and leaves the pane on a plain shell with its replayed transcript above it, indistinguishable from a pane that never had a hook; a store that refuses the write leaves both the registration and the marker standing with the reason on the confirmation; and a freeze that will not lift brings the card back naming the command that has already gone.

**Do**:
- Add `DiscardRegistration func(hookKey string) (bool, error)` to `resumeWaitConfig`, bound in production to `loadHookStore()` followed by `store.Discard(hookKey, hooks.EventOnResume, hooks.ViaPanel)`; a store that cannot be resolved returns that error rather than reporting a removal or a miss.
- Replace `resumeAnswerDiscard`'s body (task 5.3's redraw) with, in this order and no other: call `DiscardRegistration(cfg.HookKey)` with nothing yet written to stdout; on a non-nil error restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload with `Screen` `resumeScreenDiscard`, `Report` set to the error's own text and `DropInput` unset, returning without touching the marker; otherwise — whether or not anything was removed — write `hydrateResetPreamble` to stdout and call `ClearMarker`; on a non-nil error there, restore the terminal and `ExecSelf` a fresh `resume-draw` carrying the payload with `Screen` `resumeScreenPanel`, the `Command` unchanged, `Report` set to that error's text and `DropInput` unset; otherwise restore the terminal, emit the existing `exec` INFO (`target`, `args`, `hook_present` false) and `ExecSelf(resolveShell(), …)` in the bare-shell shape `resumeAnswerEnter` already takes on a miss.
- Make no other tmux call on this path: `ClearMarker` (task 4.3's seam, `state.UnsetResumePendingMarker`) is the only write, and nothing here reads or writes a pane token or touches a session.
- Cover in `cmd/state_resume_discard_test.go` over the `DiscardRegistration`, `ClearMarker` and `ExecSelf` seams plus a `logtest.Sink`, asserting call **order** on a recorder shared with the stdout writer, and drive the retry case by feeding `y` to a waiter launched with a non-empty `--report`.

**Acceptance Criteria**:
- [ ] `y` calls `DiscardRegistration` exactly once with the waiter's `--hook-key`, and nothing is written to stdout before it returns — the confirmation is still on screen when the store answers.
- [ ] A `DiscardRegistration` error produces exactly one `resume-draw` exec carrying `--screen discard` and `--report` holding the error's text; `ClearMarker` is never called, no reset preamble is written, and no shell is exec'd.
- [ ] `y` pressed on the redrawn confirmation retries the removal — the same path runs again, so a refusal is recoverable without leaving the screen.
- [ ] A `DiscardRegistration` reporting `(false, nil)` — nothing to remove — proceeds exactly as a removal does: reset preamble, marker clear, shell.
- [ ] On the success path the order is reset preamble → `ClearMarker` → shell exec, asserted on a shared order recorder rather than on three independent call counts.
- [ ] A `ClearMarker` error produces exactly one `resume-draw` exec carrying no `--screen`, the same `--command` the waiter was launched with, and `--report` holding the error's text; no shell is exec'd and nothing else runs.
- [ ] That redraw is a fresh draw of the same screen: nothing on the path re-reads the store to decide whether to paint, so a pane whose registration is gone is never left blank and frozen.
- [ ] The shell exec'd on the success path is `resolveShell()` alone with no wrapper, so the pane closes on the first `exit` — the cleared marker leaves the chain's tail nothing to do.
- [ ] The only tmux call on the whole path is the marker unset: no pane option is set, no token is read or unstamped, and no session command is issued.
- [ ] The terminal is restored before every exec on this path, including both redraws.
- [ ] An exec that returns (the exec failed) terminates non-zero through the existing `defaultExecShell` shape — one WARN, `log.Close(1)`, `osExit(1)` — rather than returning to the read loop.
- [ ] The waiting program emits no log line of its own for the removal: the breadcrumb is the store's.

**Tests**:
- `"it removes the registration before it writes anything"`
- `"it reports a refused write on the confirmation"`
- `"it leaves the marker and the registration standing when the write is refused"`
- `"it retries the removal on a second y from the report"`
- `"it treats nothing-to-remove as a discard"`
- `"it leaves the panel's screen before it clears the marker"`
- `"it clears the marker before it runs the shell"`
- `"it redraws the waiting panel with the reason when the clear fails"`
- `"it names the removed command on that redraw"`
- `"it runs neither the shell nor anything else when the clear fails"`
- `"it drops the pane to a plain shell with no wrapper"`
- `"it makes no tmux call but the marker unset"` (seam call counts)
- `"it restores the terminal before every exec"` (table over the three outcomes)
- `"it terminates non-zero when the exec fails"`
- `"it emits no removal breadcrumb of its own"` (`logtest.Sink`, `hooks` component records come from the store seam alone)

**Edge Cases**:
- The removal runs while the confirmation is still on screen so a refused write is reported there and the confirmation never closes on a failed write: a confirmation that closed on a failure would be indistinguishable from one that was backed out of, and the user would read a discard that did nothing as a discard they cancelled.
- A refused write leaves the registration and the marker both standing, and `y` can be pressed again from the report — the one outcome ruled out is a pane that drops its panel while the registration it named survives.
- A discard that finds nothing to remove is still a discard, so the marker clears and the pane falls through to a shell: the end state the user asked for is the state the store is already in, whether the entry was removed by `portal hook rm`, replaced by a re-registration, or hand-edited away while the pane waited.
- The removal is followed by the same ordered answer every resume takes, with the pane leaving the panel's screen before its marker is cleared. Nothing is at risk in between: the freeze is still in force and the pane is showing its own transcript, so a tick landing there captures what is really in the pane.
- A clear that fails brings the waiting panel back naming the removed command with both hints live and the reason on its report row, while neither the shell nor anything else runs. Handing the pane over with the marker still set would freeze that pane's saved scrollback for the rest of the pane's life, with no sweep that reaches a pane option and no waiter left to report it.
- That redraw is a fresh draw of the same screen rather than a decision about whether to draw: one that re-read the store would find no registration, paint nothing, and leave a pane that looks restored, swallows every key and is frozen for life. Enter from there reads the store again, finds nothing, and drops the pane through to a plain shell once the marker clears — which is where the discard was going.
- The pane closes on the first `exit` because the cleared marker leaves the chain's tail nothing to do — the tail reads the marker to tell an answered pane from an abandoned one, and an answered pane is handed no second shell.
- The only tmux write on the path is the marker unset, so no token is unstamped and no session is touched: the tmux session stays live, stays saved, and restores on the next reboot as an ordinary hookless pane.
- An exec that fails terminates the way the helper's exec failure already does, rather than leaving a live pane with no process — the just-emitted exec marker must not stand as a phantom handoff.

**Context**:
> A confirmed discard removes the pane's resume registration, permanently, and nothing else. The entry is cleaned out of the store rather than suppressed for the boot, so the panel does not return on this boot, on the next attach, or after any future reboot. The pending marker is cleared and the pane falls through to a plain shell with its replayed scrollback still above it — indistinguishable from a pane that never had a hook.
>
> A discard that finds nothing to remove is still a discard. A discard that cannot be written leaves the pane waiting and says so on the panel: an unreadable store or an unavailable lock is reported in place, the registration and the marker both stand, and the key can be pressed again. The one outcome ruled out is a pane that drops its panel while the registration it named survives.
>
> The tmux session is untouched. It stays live, stays saved, and restores on the next reboot as an ordinary hookless pane — bare shell, scrollback intact, no panel.
>
> The marker is cleared on both answer paths only once the pane has left the panel's screen and is showing its own transcript again. Clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus the card — the whole failure, in the space between two steps.
>
> A freeze that cannot be lifted holds the answer. The panel that comes back after a discard names a registration that is already gone: on the discard path the entry is removed while the confirmation is still up, so a store that refuses the write can be reported there; a marker that then refuses to clear brings the card back with nothing in the store behind it. It comes back as it was drawn — the removed command under its label, both key hints live — with the reason on its report row.
>
> The removal's INFO is emitted by the store method as every other store mutation's breadcrumb already is, so the waiting program adds no log line of its own and the discard op stays one chokepoint.
>
> The report row's content is the error as it was reported — Portal authors no sentence for it, matching task 4.3's call for the resume path's report.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §6.2, §7.2, §5.4, §5.3, §6.4

## lazy-resume-on-attach-5-6

### Task 5.6: A real pane discards its resume

**Problem**: Every piece of the discard has been verified against seams — a recorded store call, an injected marker clear, a scripted reader. The properties the feature actually promises are ones no seam can prove: that the confirmation is legible in a real pane with the user's pre-reboot transcript intact underneath it, that Escape puts the waiting panel back, that `y` takes exactly one entry out of the real `hooks.json` and leaves every other byte of that file alone, that the pane is then an ordinary shell whose saved scrollback is being written again and which closes on the first `exit`, and that the next reboot restores it with no panel at all. Only a real tmux pane running a real built binary over a real store demonstrates any of them.

**Solution**: An integration-tagged real-tmux suite that reboots a two-session fixture, drives `d`, Escape, `d`, `y` from tmux, and asserts the pane, the store and the saved state after each step — then reboots once more from the post-discard capture to prove the pane comes back hookless.

**Outcome**: One run proves the discard end to end: the confirmation appears over the transcript, Escape backs out of it, `y` removes exactly one registration, the pane becomes an ordinary shell that closes on the first `exit`, and a second restore of the same fixture offers no panel.

**Do**:
- Add `internal/restore/lazy_resume_discard_integration_test.go` (`//go:build integration`, `package restore_test`, `-short` skip, `tmuxtest.SkipIfNoTmux`), reusing task 4.7's setup shape: `restoretest.BuildPortalBinaryDir(t)`; `portaltest.IsolateStateForTest(t)` plus `t.Setenv("PORTAL_STATE_DIR", …)` and `state.EnsureDir()`; `PORTAL_HOOKS_FILE` and `PORTAL_PREFS_FILE` pointed into `t.TempDir()`; `portaltest.RegisterStateDirTeardownGuard(t, stateDir)`; `tmuxtest.New(t, "ptl-discard-")`.
- Seed two single-pane sessions on that socket: the **subject**, whose `hooks.json` entry is the string form so it inherits the shipped lazy default with no `prefs.json` key set, and a **bystander** whose entry is the object form carrying `resume: eager`. Stamp each pane's token with `ts.StampPaneToken`, print a distinct recognisable line into the subject's pane before the capture, and keep the seeded `hooks.json` bytes for the later byte-comparison.
- Capture with `state.CaptureStructure`, write `sessions.json` through `state.EncodeIndex`, `restoretest.RebootServer`, restore through `restoretest.NewRestoreOrchestrator` + `restoretest.RestoreWithMarker`, then `restoretest.DriveSignalHydrate` + `restoretest.WaitForSkeletonMarkersCleared`.
- Drive and assert in ordered subtests: (a) the subject's `capture-pane -p` returns the waiting panel and `capture-pane -a -p` returns the pre-reboot line; (b) `send-keys d` → `capture-pane -p` returns `▲ Discard resume?`, the registered command and `y discard   esc cancel`, while `capture-pane -a -p` still returns the pre-reboot line; (c) `send-keys Escape` → the waiting panel is back with both hints; (d) `send-keys d` then `send-keys y` → poll `tmux.ReadPaneOption` until `@portal-resume-pending` is empty, then read `hooks.json` and assert the subject's key is absent and the bystander's entry is byte-identical to the seed; (e) the subject's `@portal-pane-id` still reads back through `tmux.ReadPaneOption`, its session is still live, a fresh `state.CaptureStructure` enumerates it with an empty pending set, and a scrollback write for that pane now lands; (f) capture and encode `sessions.json` again while the pane is alive, then `send-keys exit` and assert the pane is gone within the existing budget on the first press; (g) `restoretest.RebootServer` + restore + `DriveSignalHydrate` from that second capture → the subject's `capture-pane -p` shows its transcript and no panel, and it carries no pending marker.
- Keep it in the integration lane with `-p 1` in mind: no `t.Parallel`, the built binary via `restoretest`/`portalbintest` only, no `portal state daemon` spawned, and `tmuxtest.New`'s own cleanup killing the disposable server. The suite signals nothing and enumerates no process it did not cause to exist.

**Acceptance Criteria**:
- [ ] `capture-pane -p` on the subject returns the discard confirmation — its `▲ Discard resume?` title, the registered command and its `y discard   esc cancel` footer — while `capture-pane -a -p` still returns the pre-reboot line intact underneath it.
- [ ] `send-keys Escape` returns the waiting panel with both its key hints, in the same run and before any discard is driven.
- [ ] After `y`, the subject's key is absent from `hooks.json` and the bystander's entry is byte-identical to the bytes that were seeded, object form and `resume` attribute included.
- [ ] The subject's `@portal-pane-id` still reads back non-empty after the discard, and its session is still live and still enumerated by `state.CaptureStructure`.
- [ ] `@portal-resume-pending` reads back empty on the subject within a bounded poll, the pane is absent from `CaptureStructure`'s pending set, and a scrollback write for that pane lands where it was previously skipped.
- [ ] `send-keys exit` closes the subject's pane on the first press, within the budget the existing restored-pane suite uses.
- [ ] A second reboot restored from the post-discard capture brings the subject back showing its transcript with no panel, carrying no pending marker, and with no waiter in its pane.
- [ ] Every assertion about what Portal concludes goes through Portal's own reads (`state.CaptureStructure`, `tmux.ReadPaneOption`, the `hooks.json` bytes) and every assertion about what the pane shows goes through `capture-pane`; raw tmux is used only to stage the fixture and to send keys.
- [ ] The suite carries `//go:build integration`, uses `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket and a `restoretest`-built binary, spawns no `portal state daemon`, and leaves no server or subprocess behind.
- [ ] The suite signals no process and enumerates no process it did not cause to exist.

**Tests**:
- `"it shows the confirmation over the pane's own transcript"`
- `"it returns the waiting panel on Escape"`
- `"it removes only the subject's registration"`
- `"it leaves the pane's durable token stamped and its session live"`
- `"it clears the pending marker and resumes writing the pane's scrollback"`
- `"it closes the pane on the first exit"`
- `"it restores the pane with no panel on the next reboot"`

**Edge Cases**:
- The confirmation is read out of the real pane through `capture-pane`, with the pre-reboot transcript intact underneath it: the alternate screen is the whole mechanism by which either screen hides a transcript it never touches, and nothing short of a real pane demonstrates it.
- Escape returns the waiting panel in the same run before the discard is driven — backing out and then confirming is the sequence a user actually performs, and a confirmation reachable only once would pass a test that drove `y` immediately.
- After `y` the key is absent from `hooks.json` and every other entry is byte-unchanged: the store rewrites the whole file on every mutation, so "it removed one entry" is only true if the neighbours survive — including an object-form entry the reader models and could re-marshal into a different shape.
- The pane's durable token is still stamped and its session is still live and still enumerated into a fresh capture: discarding removes a registration and never touches the pane's identity or its session.
- The pending marker is cleared and the pane's saved scrollback resumes being written — the freeze's whole lifetime is the wait, and a marker left set would freeze the pane's saved content for the rest of its life with nothing to reclaim it.
- The pane closes on the first `exit` — the answered pane exec'd its own shell into the chain's child slot, and the chain's tail must find no marker and add no second shell.
- A second reboot of the same fixture restores that pane with no panel and no marker, as an ordinary hookless pane: the discard is disposal rather than deferral, so the offer must not come back.
- Integration lane with isolated state, a disposable socket and no daemon: the suite builds and execs a portal binary, so it belongs behind the tag; it needs no saver, so nothing here may spawn one.
- The suite enumerates and signals no process it did not cause to exist. The developer's live tmux server and their real `portal state daemon` are present during every run, and every assertion here is reachable through tmux and the filesystem alone.

**Context**:
> A confirmed discard removes the pane's resume registration, permanently, and nothing else. The pending marker is cleared and the pane falls through to a plain shell with its replayed scrollback still above it — indistinguishable from a pane that never had a hook.
>
> The tmux session is untouched. It stays live, stays saved, and restores on the next reboot as an ordinary hookless pane — bare shell, scrollback intact, no panel.
>
> Per-boot decline and re-offer-on-next-attach were both rejected: pressing the discard key is an act of disposal, not deferral — "I've decided, actually, I don't need that session" — and an offer that comes back after you have declined it is treating a decision as a hesitation.
>
> A rewrite of one registration leaves every other entry exactly as it found it. An entry the call did not name is written back carrying what it carried — an attribute the reader does not model and a `resume` value it could not make sense of alike.
>
> The panel is painted into the pane's alternate screen, so it never enters the scrollback. Measured on tmux 3.7c: a pane printed two lines of real content, entered the alternate screen, and painted a card. `capture-pane -p` returned the card; `capture-pane -a -p` returned the two original lines, intact underneath.
>
> Task 4.7 is the shape this suite is built from — same fixture, same isolation, same read discipline — and it remains the end-to-end guarantee for the resume answer; this one is the discard's.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §6.2, §5.1, §5.4, §3.2, §7.2, §9.1

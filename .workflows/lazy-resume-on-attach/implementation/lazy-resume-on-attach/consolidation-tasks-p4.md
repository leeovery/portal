# Consolidation Tasks: lazy-resume-on-attach (Phase 4)

## Task 1: One Hand-Off, So the Marker Cannot Be Left Behind
placement: phase 4
severity: duplication

**Problem**: The chain's last act before it replaces its own process image is an INFO `exec` marker that must be the statement immediately before the hand-off — the writer is unbuffered, and a marker emitted after the exec is never written at all. That rule is carried by seven hand-copied comments and enforced by nothing: every test in the phase fakes the exec, so a site that emits the marker late, or omits it, fails no test and shows no symptom. An operator reconstructing why a pane fell out of the panel finds the chain's last image unrecorded in `portal.log` with no way to tell which hand-off broke. The full shape — resolve the binary, wrap its error, compose the argv, emit the marker, exec — is authored twice (`cmd/state_resume_draw.go:56-68`, `cmd/state_resume_wait.go:226-239`), and phase 5 adds at least two more sites that would each re-derive it from a neighbour.

**Solution**: One `resumeHandOff(logger, execSelf, subcommand string, p resumeChainPayload) error` in `cmd/state_resume_chain.go`, beside `resumeChainArgv` and `resumeChainExe` — resolve, compose, emit, exec — with `runResumeDraw` and `resumeRedraw` routed through it, so the ordering rule has one declaration and phase 5's sites inherit it rather than copying it. Strip the doubled error prefix the two copies produce at the same time: `cmd/state_resume_chain.go:81` returns `errors.New("resolve portal executable: empty path")` and both call sites wrap it again, so the pane's stderr reads `resolve portal executable: resolve portal executable: empty path`; only one of the two should carry the prefix. The five bare-shell and hook hand-offs keep their `hook_present` attr, which the plan deliberately omits from the chain's own hand-offs, so they take a sibling one-liner rather than folding into the same function.

**Outcome**: The chain's forensic marker is emitted from one place, and a new hand-off cannot be written without it.

**Do**:
- Put the doubled prefix on its owner first: `resumeChainExe` (`cmd/state_resume_chain.go:89-98`) wraps `os.Executable`'s failure as `resolve portal executable: %w` and keeps its own `resolve portal executable: empty path` sentinel, so every consumer renders exactly one prefix and no caller re-wraps — including `markResumePending`'s `ResolveExe` seam (`cmd/state_hydrate.go:350-357`), which today gets a bare OS error on the non-empty branch.
- Add `resumeHandOff(logger *slog.Logger, execSelf func(prog string, args []string), subcommand string, p resumeChainPayload) error` to `cmd/state_resume_chain.go` beside `resumeChainArgv`/`resumeChainExe`: resolve, return the resolver's error unwrapped, compose with `resumeChainArgv(exe, subcommand, p)`, emit `logger.Info("exec", "target", exe, "args", strings.Join(argv, " "))` as the statement immediately before `execSelf(exe, argv)`, and carry no `hook_present` attr.
- Add the sibling beside it for the five hand-offs that do carry the attr — `execHandOff(logger *slog.Logger, exec func(prog string, args []string), prog string, args []string, hookPresent bool)`, same marker plus `"hook_present", hookPresent` — and route `execShellAndExit` (`cmd/state_hydrate.go:179-185`), `execShellOrHookAndExit`'s hit branch (`:261-264`), `execResumeChainAndExit` (`:283-285`), `resumeAnswerEnter` (`cmd/state_resume_wait.go:192-195`) and `runResumeRecover` (`cmd/state_resume_recover.go:59-62`) through it, deleting all five copies of the two-line comment.
- Route the two full-shape sites: `runResumeDraw` (`cmd/state_resume_draw.go:56-68`) ends in `return resumeHandOff(cfg.Logger, cfg.ExecSelf, resumeWaitSubcommand, next)` once it has set `next.Width`/`next.Height`; `resumeRedraw` (`cmd/state_resume_wait.go:226-239`) calls `cfg.restore()` and then hands off with `resumeDrawSubcommand`. Restoring before the resolve rather than between resolve and exec changes nothing observable — `cfg.restore` is a `sync.OnceFunc` and `runResumeWait`'s deferred call already covers the path where the hand-off returns an error.
- Close the rule structurally: add a source guard over `cmd`'s non-test sources (through `sourceguardtest`) failing any call to an `ExecSelf`/`ExecShell` seam field outside `resumeHandOff` and `execHandOff`. There are exactly seven such calls today; after the routing there are two.

**Acceptance Criteria**:
- [ ] `resumeHandOff` and `execHandOff` are the only functions in `cmd`'s production sources that call an `ExecSelf`/`ExecShell` seam, and the guard fails when a third appears
- [ ] Each emits the `exec` INFO marker as the statement immediately before the exec, and only `execHandOff` carries `hook_present`
- [ ] Per-site marker attrs are unchanged: `target` and `args` everywhere, `hook_present` on exactly the five sites carrying it today and on neither chain hand-off
- [ ] A chain hand-off that cannot resolve the binary renders `resolve portal executable:` exactly once — for the empty-path sentinel and for an `os.Executable` failure alike
- [ ] The seven hand-copied "must stay the statement immediately before the exec" comments are gone; the rule is stated once, where it is enforced
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` pass

**Tests**:
- `"it emits the exec marker before it hands the process image over"` — new, over both helpers: an exec fake that reads the `logtest.Install(t)` sink at the moment it is called and asserts the `exec` record is already captured. This is the assertion the phase has none of, and every site inherits it.
- `"it renders one resolve portal executable clause for an empty path"` — new, over `resumeChainExe` and one hand-off.
- Existing: `cmd/state_resume_draw_test.go`, `cmd/state_resume_wait_test.go`, `cmd/state_resume_recover_test.go`, `cmd/state_resume_enter_test.go` and `cmd/state_hydrate_lazy_test.go` stay green unchanged — their exec-marker and composed-argv assertions are the regression net for the routing.
- Prove the guard discriminates by mutation: add a bare `cfg.ExecShell(...)` to a production file and confirm it reddens.

## Task 2: One Home for What a Missing Registration Means
placement: phase 4
severity: duplication

**Problem**: The reboot path and the panel path each decide independently that a missing entry, an empty command or an unreadable store means a bare shell, and each emits its own pair of breadcrumbs. The two copies have **already diverged**: `cmd/state_hydrate.go:245-265` treats a found registration carrying an empty command as a hit and would run `sh -c "; exec $SHELL"`, while `cmd/state_resume_wait.go:202-215` treats the same result as a miss. It is harmless today only because `internal/hooks/lookup.go:37-40` never produces that value — which argues for one home rather than against it. Any change to either copy compiles and leaves both suites green, and the user meets it as "my command ran on reboot but not when I pressed Enter", with nothing in the logs telling the two paths apart.

**Solution**: One function over `(logger, lookup func(string) (hooks.OnResume, error), hookKey string) string` returning the command or the empty string, with both call sites routed through it, so the three-branch rule and its two breadcrumbs have a single declaration and the empty-command branch is decided once. `lookupOnResumeOrLog`'s dependence on `hydrateConfig` is what blocks reuse today; taking the store and logger as parameters unblocks it and lets phase 5's discard path reach the same rule rather than authoring a third copy. The empty-command branch is reachable only through a seam-level fake, since the production lookup cannot produce it — the coverage is defensive and should say so.

**Outcome**: A pane resumed from the panel and the same pane resumed on reboot cannot disagree about what its registration means.

**Do**:
- Declare the rule once — `resumeRegistrationOrLog(logger *slog.Logger, lookup func(string) (hooks.OnResume, error), hookKey string) hooks.OnResume` in `cmd/state_resume_chain.go`, beside the rest of the chain's shared vocabulary. Three branches, worded exactly as both copies word them today: a read error emits the `hook lookup` DEBUG with `result=error` plus the `lookup on-resume hook failed` WARN and answers with the zero value; a miss emits `result=miss` and answers with the zero value; a hit emits `result=hit` and answers with the registration.
- Decide the empty-command branch the wait path's way (`cmd/state_resume_wait.go:209`), not the hydrate path's (`cmd/state_hydrate.go:235`): a found registration carrying no command is a miss, because `hookExecArgs("", shell)` would exec `sh -c "; exec $SHELL"`. The value returned is normalised, so `Found` and a non-empty `Command` cannot disagree.
- Return the whole `hooks.OnResume` rather than the command alone: `resolveResumeDecision` (`cmd/state_hydrate.go:198-203`) resolves the stored `Mode` off it. Normalised as above, the wait path's `.Command` is the command or the empty string exactly as it needs.
- Route both call sites. `lookupOnResumeOrLog` (`cmd/state_hydrate.go:224-241`) becomes a call with a lookup closure — `cfg.HookStore == nil` returns `(hooks.OnResume{}, nil)`, so an absent store still records `result=miss`, and otherwise `cfg.HookStore.LookupOnResume(hookKey, hooks.ViaHydrate)`. Delete `resumeCommandAtAnswer` (`cmd/state_resume_wait.go:202-215`); `resumeAnswerEnter` takes `.Command` off the shared call driven by `cfg.LookupResume`.
- Check the consumer that carried the divergence: `execShellOrHookAndExit`'s `!lookup.Found` test (`cmd/state_hydrate.go:257`) now sees a normalised value, so no empty command can reach `hookExecArgs` from either branch.

**Acceptance Criteria**:
- [ ] One function declares the miss / empty-command / read-error rule, and both resume paths call it
- [ ] A registration found carrying an empty command drops the pane to a bare shell on both paths and records `result=miss`; `sh -c "; exec $SHELL"` is unreachable
- [ ] Breadcrumbs are unchanged: same messages, same `hook_key`/`result`/`error` attrs, same levels, one DEBUG per lookup and a WARN only on a read error
- [ ] A nil hook store still records `result=miss` on the hydrate path
- [ ] The hydrate path still resolves the stored `Mode` — a lazy registration still waits, an eager one still fires at hydrate
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` pass

**Tests**:
- `"it reads a registration carrying an empty command as no hook"` — new, over the shared function through a fake lookup. The production lookup cannot produce that value (`internal/hooks/lookup.go:37-40`), so the coverage is defensive and the test says so in its own words.
- `"it records a read failure as a DEBUG and a WARN and answers no hook"` and `"it records a miss as a DEBUG alone"` — new over the shared function, carrying forward the assertions the two deleted copies' suites make today rather than adding to them.
- Existing: `cmd/state_hydrate_test.go` and `cmd/state_hydrate_lazy_test.go` (lookup breadcrumbs, the lazy decision, the eager tail) and `cmd/state_resume_wait_test.go` / `cmd/state_resume_enter_test.go` (the Enter answer) stay green unchanged.
- Prove both paths really run the one declaration by mutation: drop the WARN emission and confirm the wait path (`cmd/state_resume_enter_test.go:213-219`) and the hydrate path (`cmd/state_hydrate_exec_log_test.go:105`, `cmd/state_hydrate_lazy_test.go:376-380`) both redden.

## Task 3: One Flag Reset the Next Chain Command Cannot Miss
placement: phase 4
severity: duplication

**Problem**: Cobra leaves a flag marked as set on the shared command instance between runs, so a `cmd` test that drives a chain subcommand through the package's canonical reset and then executes inherits the previous test's values and passes while checking nothing. This is demonstrated, not hypothetical: task 4-1's `--command` refusal subtest passed vacuously when run after its sibling, and was patched with a helper local to one file; tasks 4-2 and 4-4 then wrote the same helper again in their own files rather than finding the canonical one. `resetRootCmd` (`cmd/root_test.go:20-95`) already carries the hydrate command's three flags and reaches none of the three chain commands. Whoever writes phase 5's discard command — same flag set, same hazard — has no signal that a reset exists or must be called, and the defect it hides surfaces as a chain command parsing a stale payload in a restored pane.

**Solution**: Replace the hydrate-flag block and the three byte-identical local helpers (`cmd/state_resume_draw_test.go:348-353`, `cmd/state_resume_wait_test.go:488-493`, `cmd/state_resume_recover_test.go:286-291`) with one `VisitAll`-over-`stateChildCommands` loop inside `resetRootCmd`, setting each flag back to its default and clearing its changed bit. `stateChildCommands` is declared in the same package, so it is directly reachable. That covers the hydrate flags, all three chain commands, and every chain command still to land, in one edit — and a new verb is covered the day it is registered rather than the day someone remembers.

**Outcome**: A vacuous pass of this kind is unreachable for any chain command, present or future.

**Do**:
- Replace the hydrate-flag block at `cmd/root_test.go:92-97` with one loop inside `resetRootCmd`: for each command in `stateChildCommands` (`cmd/state_test.go:217-226`), `c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })`. That covers the hydrate flags it replaces, all three chain commands, and the four other state children.
- Delete the three local helpers — `cmd/state_resume_draw_test.go:346-353`, `cmd/state_resume_wait_test.go:486-493`, `cmd/state_resume_recover_test.go:284-291` — and the extra call line at each of their five call sites (`draw:377`, `:399`; `wait:514`, `:532`; `recover:309`), so `resetRootCmd()` alone is the reset. Drop the `pflag` imports those files no longer use.
- Keep the reason on the loop, once: a flag left `Changed` on the shared instance satisfies Cobra's required-flag check, so a refusal subtest passes while checking nothing.
- Change nothing else in `resetRootCmd` — the help-flag pass, the `openCmd` re-`Init` and the other per-command blocks stay exactly as they are.

**Acceptance Criteria**:
- [ ] `resetRootCmd` resets every flag of every command in `stateChildCommands` to its `DefValue` with `Changed` cleared, and the hydrate-flag block it replaces is gone
- [ ] The three local helpers are gone and no test file declares another
- [ ] A chain subcommand's required-flag refusal fails for the right reason whatever order its siblings ran in
- [ ] A state child registered later is covered with no edit to `resetRootCmd`
- [ ] `go test ./cmd -count=1` and `go test ./cmd -shuffle=on` pass

**Tests**:
- `"it clears every flag on every state child"` — new, table-driven over `stateChildCommands`: set each flag to a non-default, call `resetRootCmd`, assert `DefValue` and `Changed == false`. Driving the list rather than a fixed set of commands is what carries the cover to a verb that lands later.
- `"it refuses a resume-wait naming no command after a sibling set one"` — new: Execute the full chain argv, then `resetRootCmd()` and Execute without `--command`, asserting the refusal. This is the exact vacuous pass 4-1 hit, and it fails without the loop.
- Existing: the three chain command suites stay green with their local reset calls removed, and the rest of `cmd` is unaffected by the wider reset — a suite that depended on a flag surviving `resetRootCmd` would surface here.

## Task 4: The Frozen-Pane Capture Cycle Cannot Be Entered Halfway
placement: phase 4
severity: duplication

**Problem**: The rule that cannot be violated silently is that the set of frozen panes must drive the scrollback re-file before anything commits, and that a frozen pane is skipped by the dump. That ordering is now restated in three places and checked in none: the daemon's cycle (`cmd/state_daemon.go:244-309`), the commit-now variant (`cmd/state_commit_now.go:120-133`), and the integration fixture this phase added (`internal/restore/lazy_resume_panel_integration_test.go:375-422`). A later change to the daemon lands in one copy; the others go on running the old sequence, and the fixture keeps passing while asserting against a route production no longer takes. The loss it exists to catch — a frozen pane's transcript reclaimed or overwritten — then ships silently, and the user meets it as an empty restored pane two reboots after a pane was left waiting, with no copy of the transcript anywhere. Demonstrated in-phase: attempt 1 of task 4-7 shipped a copy missing two of the four steps and every assertion stayed green.

**Solution**: A `state`-level composite that a caller cannot commit around — a `CaptureAndRefile(client, dir, skipSet, prev, hashes, logger) (Index, PendingSet, error)` running the structure read and the re-file as one step and handing back the frozen-pane set — with the daemon, commit-now and the fixture all entering the cycle through it. That makes the omission unrepresentable rather than merely discouraged, and leaves each caller owning only the part that genuinely differs: whether it dumps. Two of the three copies are phase 2's, so the consolidation reaches back; the third copy and the demonstrated silent pass are this phase's, which is what makes it owed here. This is the capture path — the most consequential code in the tree — so the task carries its own verification against the existing daemon suites rather than resting on the fixture it repairs.

**Outcome**: The step that protects a waiting pane's transcript cannot be skipped by a caller that forgot it existed.

**Do**:
- Add the composite to `internal/state` beside `RefilePendingScrollback` (`internal/state/scrollback.go:98`): `CaptureAndRefile(c CaptureClient, dir string, skipSet map[string]struct{}, prev *Index, hm HashMap, logger *slog.Logger) (Index, map[string]struct{}, error)` — call `CaptureStructure`, return its index, pending set and error untouched on failure, otherwise run `RefilePendingScrollback(dir, &idx, pending, hm, logger)` and hand back the mutated index with the pending set the caller's dump must skip. `CaptureStructure` stays exported and unchanged — the structure-only callers across `internal/state`, `internal/restore` and `cmd`'s fixtures keep it, and only the three cycle sites move. Name the composite in CLAUDE.md's `state` row as the entry point the capture cycle is taken through.
- Route the daemon (`cmd/state_daemon.go:249-257`): one call replaces the `CaptureStructure` + `RefilePendingScrollback` pair, with `ListSkeletonMarkers`, the per-pane dump's `paneSkipsScrollback` gate and `Commit` untouched.
- Route commit-now: rename `CommitNowDeps.CaptureStructure` to `CaptureAndRefile` with the composite's signature (`cmd/state_commit_now.go:36`), default it to `state.CaptureAndRefile` (`:63-65`), delete the standalone refile at `:129`, and correct the struct doc at `:32-33`. Carry the rename through `commitNowFixture` and `cmd/deps_merge_convention_test.go:275-284`.
- Re-point `TestStateCommitNow_RefilesResumePendingScrollback` (`cmd/state_commit_now_test.go:1187-1245`) at the real `state.CaptureAndRefile` over a fake capture client emitting the waiting pane's token and pending marker — the shape `cmd/state_commit_now_test.go:326-339` already uses. Left on a faked composite it would assert a re-file the fake performed, which is the silent pass this task exists to remove.
- Route the fixture's `captureRound` (`internal/restore/lazy_resume_panel_integration_test.go:375-422`), deleting its hand-written structure-then-refile pair and re-wording its doc comment to name the composite. Its dump loop and its skip of `skipSet ∪ pending` stay with it: whether a caller dumps is the part that genuinely differs.

**Acceptance Criteria**:
- [ ] One function runs the structure read and the re-file; the daemon, commit-now and the integration fixture all enter through it and none of the three calls `RefilePendingScrollback` itself
- [ ] A failed structure read returns before anything is re-filed, and its caller still refuses to commit
- [ ] The daemon's behaviour is unchanged: same skip set, same dump skip for a frozen pane, same `tick complete` counts, same index committed
- [ ] commit-now still passes a nil skipSet and still commits with `anyScrollbackChanged=false`, and its re-file test drives production's re-file rather than a fake's
- [ ] A pending pane's scrollback is still re-filed onto its token before the commit's housekeeping pass runs, on all three routes
- [ ] `go test ./internal/state/ ./cmd/ -count=1`, `go test ./...` and `go test -tags integration -p 1 ./...` pass

**Tests**:
- `"it re-files every frozen pane before it returns"` — new, in `internal/state` over the composite: a capture carrying a pending token-stamped pane comes back with the token path on its record and the bytes moved.
- `"it returns the capture's error with nothing re-filed"` — new: a failing `CaptureClient` leaves the scrollback directory as it was.
- Existing, and this is the verification the task carries rather than resting on the fixture it repairs: `cmd/state_daemon_resume_pending_test.go` (the whole frozen-pane cycle, `:323-521`), `cmd/state_daemon_run_test.go`'s cancellation and capture-failure cases, `cmd/state_daemon_cycle_summary_test.go`, `cmd/state_commit_now_test.go`, `internal/state/scrollback_test.go`'s `TestRefilePendingScrollback`, and `internal/restore/lazy_resume_panel_integration_test.go` — all green, with no assertion weakened or deleted.
- Prove the composite is load-bearing by mutation: drop the re-file call inside it and confirm `TestCaptureAndCommit_RefilesResumePendingScrollback` and the commit-now re-file test both redden.

## Task 5: One Declaration of How a Gone Pane Is Told From an Unset Option
placement: phase 4
severity: near-miss

**Problem**: Two functions in one file each carry the same measured tmux fact: a format read answers a vanished pane with exit 0 and an empty string, indistinguishable from an unset option on a live one, so an extra option-less probe is what separates the two. `ResolveHookKey` (`internal/tmux/tmux.go:244-263`) and `ReadPaneOption` (`:350-369`) are identical two-read sequences differing only in error wording and in how the format is passed, each with its own multi-line comment restating the measurement. If either is simplified to a single read — it looks redundant, and every test driving only live panes stays green — a gone pane starts reading as a live pane carrying no option. On the registration path that turns `portal hook set` against a vanished pane from a clean failure into a write against nothing, which is the junk-key bug the probe was added to close. On the marker path a gone pane reads as "already answered" and the recovery step declines to recover it, leaving a pane with a dead keyboard. Either is noticed long after the edit, as a stray store entry or a pane that stopped responding.

**Solution**: Express `ResolveHookKey(paneID)` as `ReadPaneOption(paneID, state.PortalPaneIDOption)`, wrapping the result to keep its own error wording, so one declaration of the probe rule remains and there is one place it can be wrongly removed from. `HookKeyFormat` stays — the whole-server enumeration still composes from it. The suites in `internal/tmux/resolve_hookkey_test.go` and `internal/tmux/pane_option_test.go` need their error strings reconciled; both already pin the probe against a real gone pane, so the consolidation inherits that coverage rather than replacing it.

**Outcome**: The measurement that makes a gone pane distinguishable is stated once, where removing it fails loudly.

**Do**:
- Express `ResolveHookKey` (`internal/tmux/tmux.go:250-263`) as `c.ReadPaneOption(paneID, state.PortalPaneIDOption)`, deleting its probe, its token read and the comment restating the measurement. `ReadPaneOption` (`:359-369`) keeps the sole statement of the rule, and its comment becomes the one place the measurement is recorded.
- Hand `ReadPaneOption`'s error back as it stands rather than adding a second wrap around it. `cmd/hooks_seams_test.go:91-135` pins the gone-pane rendering byte-for-byte as one Portal clause plus tmux's own words — `no pane answers to "%999": tmux show-options -p -t %999: no such pane: %999` — and a re-wrap would render two, undoing a wording decision an earlier work unit took deliberately. A failed token read then carries `ReadPaneOption`'s wording, which still names the pane and still wraps the recoverable `*tmux.CommandError`; no test pins the `failed to resolve hook key for pane` phrasing that is lost.
- `HookKeyFormat` (`:682`) stays as it is: `paneHookRowFormat` (`:701`) composes the whole-server enumeration from it, and nothing about that read changes.
- Reconcile the suites the change reaches: `internal/tmux/resolve_hookkey_test.go:56-59` expects the token read with the format passed positionally and must become the `-F "#{…}"` form `ReadPaneOption` composes (behaviourally identical to tmux, which is why the two copies could diverge on it unnoticed); sweep the rest of that file and `internal/tmux/pane_option_test.go` for any other wording or argv expectation the delegation moves.

**Acceptance Criteria**:
- [ ] `ResolveHookKey` issues no tmux command of its own — the probe and the format read are `ReadPaneOption`'s, and there is one place the probe can be wrongly removed from
- [ ] A pane id no live pane answers to still fails on the probe, before the token read runs, returning an empty key and a recoverable `*tmux.CommandError`
- [ ] A live pane that has never been stamped still resolves to `("", nil)`
- [ ] `hook set` and `hook rm` against a gone pane still render exactly one Portal clause followed by tmux's own words, byte-for-byte as today
- [ ] The measured fact is stated in one comment, over `ReadPaneOption`; `HookKeyFormat` and the whole-server enumeration are untouched
- [ ] `go test ./internal/tmux/ ./cmd/ -count=1` and `go test ./...` pass

**Tests**:
- No new test — the consolidation inherits coverage rather than replacing it. `internal/tmux/resolve_hookkey_realtmux_test.go:26-45` and `internal/tmux/pane_option_realtmux_test.go:116` each drive a genuinely gone pane against a real server (both in the unit lane), and `internal/tmux/resolve_hookkey_test.go` / `internal/tmux/pane_option_test.go` pin the two-read ordering and the probe's option-less argv.
- `cmd/hooks_seams_test.go`'s `TestGonePaneErrorCarriesOnePortalClause` stays green with its expected string unchanged — it is the criterion for the error-wrapping decision above.
- Prove the single declaration is load-bearing by mutation: delete the probe from `ReadPaneOption` and confirm both real-tmux gone-pane subtests redden.

## Task 6: The Shell-Quoting Package Holds the Whole Rule Again
placement: phase 4
severity: near-miss

**Problem**: `internal/shellquote` is the tree's declared single home of the POSIX quoting rule, precisely because naive composition corrupts a user-authored command — but it exports only the per-word half. The half that renders a whole argv into one shell command line is now declared separately in two packages, byte-identical: `cmd/state_resume_chain.go:72-81` and `internal/spawn/recipe.go:67-76`. Both the package's own doc and its row in `CLAUDE.md` claim it holds the rule whole, which is no longer true. The next site that has to render an argv — phase 5's discard and confirm chains are the near-term candidates — finds no helper to reach for and either writes a third copy or concatenates. The user meets that as a resume command holding a space, a quote or a `$` arriving at the panel as several arguments or truncated; on the discard path, as the wrong registration destroyed.

**Solution**: Export the joiner on `internal/shellquote` — both packages already import it and it stays stdlib-only, so no new dependency edge appears — and route both call sites through it, deleting the two local copies. The package doc and the `CLAUDE.md` row describe the package as the rule's only declaration; adding the joiner is what makes that description true again rather than requiring it to be softened.

**Outcome**: Rendering an argv into a shell command line has one implementation, in the package that exists to hold it.

**Do**:
- Export the joiner on `internal/shellquote`: `Join(argv []string) string`, quoting each element through `Single` and joining with a single space — the same body both copies have. Widen the package doc from the per-word rule to both halves, so it describes what the package holds.
- Delete `shellWords` (`cmd/state_resume_chain.go:72-81`) and re-point its callers: `cmd/state_hydrate.go:279-280` and `cmd/state_hydrate_lazy_test.go:117-118`, `:271`.
- Delete `renderCommandString` (`internal/spawn/recipe.go:67-76`) and re-point its callers: `internal/spawn/configadapter.go:52`, `:84`, `internal/spawn/ghostty.go:22`, `:29` (and the comment naming it at `:19`), plus the in-package test references in `configadapter_argv_test.go`, `configadapter_script_test.go`, `configadapter_script_integration_test.go`, `ghostty_command_test.go` and `recipe_test.go`.
- Move `TestRenderCommandString`'s three cases (`internal/spawn/recipe_test.go:165-196`) to `internal/shellquote`'s own suite against the exported name rather than dropping them — the spawn argv, the spaced element that must stay one word, and the close-escape-reopen escape are the rule's own coverage and belong with it.
- Update the `shellquote` row in `CLAUDE.md` to name the joiner beside `Single`, so the map states the surface the package now has.

**Acceptance Criteria**:
- [ ] `internal/shellquote` exports the joiner and no other package declares one; `shellWords` and `renderCommandString` are gone and every former caller goes through the export
- [ ] The rendering is unchanged byte-for-byte: each element single-quoted, space-joined, an embedded quote as close-escape-reopen, an empty element as `''`
- [ ] `internal/shellquote` still depends on the standard library alone — `internal/shellquote/leaf_guard_test.go` passes in both lanes
- [ ] The hydrate chain's `sh -c '<draw>; <recover>'` command line and Ghostty's embedded payload are byte-identical to what they are today
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` pass

**Tests**:
- The three moved cases keep their subjects and names in `internal/shellquote`'s suite; `internal/spawn/recipe_test.go` loses `TestRenderCommandString` and nothing else.
- `"it renders an empty argv as the empty string"` and `"it keeps an empty element as an empty quoted word"` — new on the joiner: both follow from `Single`, and the joiner is where a caller meets them.
- Existing: `internal/spawn`'s adapter and Ghostty suites (which compare against the renderer by calling it) and `cmd/state_hydrate_lazy_test.go`'s chain-composition assertions stay green unchanged.

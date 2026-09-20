# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The chain's tail cannot tell a gone pane from an unset marker, and its own test cannot pass

**Severity**: Critical
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-4` (The chain's tail: a waiter that dies leaves a usable pane)
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The task specifies `tmux.ReadPaneOption` as a single `display-message -p -t <target> -F "#{<option>}"` and then requires it to error for a target no live pane answers to — the property the recover tail's whole decision rests on, and the property the task's own real-tmux test asserts. Measured on tmux 3.7c on this machine (disposable `-L` socket, 2026-09-20): `display-message -p -t <gone pane id> -F '#{@opt}'` exits **0** with an **empty string**, and so do a bogus `%999` and a gone `=session:w.p`. That is byte-for-byte what an unset option on a live pane reads as. So the acceptance criterion and the test `"it fails a read against a target no live pane answers to"` cannot pass as written, and the method as specified reports "this pane was answered" for a pane that does not exist — the silent answer, where the tail's stated rule is that anything it cannot establish is treated as still pending.

The repo has already paid for this lesson twice and written it down. `internal/tmuxtest/stamp.go`'s `ReadPaneToken` carries a comment explaining that it deliberately avoids `display-message -F` for exactly this reason, and CLAUDE.md records that `ResolveHookKey` takes **two** live reads — a `show-options -p -t <pane>` existence probe naming **no** option, then the `display-message` format read — because naming an option collapses the discrimination. Measured on the same run: the probe naming no option exits 0 for a live pane whether or not it carries options, exits 1 `no such pane` for a gone or bogus target, and naming the option gives `invalid option`, exit 1, for an unset option on a live pane.

**Proposal**:
Give `ReadPaneOption` the probe-then-read shape `ResolveHookKey` already takes, and say so in the task's Do, its acceptance criterion, its scripted-commander test and its CLAUDE.md edit, with the measurement recorded as an edge case. The plan's own conventions settle this: Portal already declares this exact idiom for this exact question, and the task's stated behaviour (errors for a gone target) is unreachable any other way.

**Current**:

*Do, first bullet:*
```
- Add `ReadPaneOption(target Target, option string) (string, error)` to `internal/tmux/tmux.go` beside `SetPaneOption` / `UnsetPaneOption`: `display-message -p -t <target> -F "#{<option>}"`, the target spent as a string at the argv and the failure wrapped in the shape its siblings use. Pin the argv in `internal/tmux/tmux_test.go` through `commandertest.Scripted`, and extend `internal/tmux/pane_option_realtmux_test.go` with a set → read → unset → read round trip and a read against a target no live pane answers to.
```

*Do, last bullet:*
```
- Extend the options clause of CLAUDE.md's `tmux` package row to name the pane-option pair the marker is written and read through — `UnsetPaneOption` alongside `SetPaneOption`, and `ReadPaneOption` as the single-pane format read.
```

*Acceptance Criteria, seventh bullet:*
```
- [ ] `tmux.ReadPaneOption` composes `display-message -p -t <target> -F "#{<option>}"`, takes `tmux.Target`, reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to; `internal/tmux/target_composition_guard_test.go` passes with it in place.
```

*Tests, ninth bullet:*
```
- `"it composes the pane-option format read"` (scripted commander)
```

**Proposed Text**:

*Do, first bullet:*
```
- Add `ReadPaneOption(target Target, option string) (string, error)` to `internal/tmux/tmux.go` beside `SetPaneOption` / `UnsetPaneOption`, taking the same two-read shape `ResolveHookKey` already takes: a `show-options -p -t <target>` existence probe naming **no** option, whose non-zero exit is returned as the error before anything else runs, then `display-message -p -t <target> -F "#{<option>}"` for the value. A single format read cannot carry this method's contract — measured against tmux 3.7c on 2026-09-20, `display-message -p -t <target> -F '#{@opt}'` answers a target no live pane answers to with exit 0 and an empty string, for a gone pane id, a bogus `%999` and a gone `=session:w.p` alike, which is byte-for-byte what an unset option on a live pane reads as. The probe naming no option exits 0 for a live pane whether or not it carries options and exits 1 `no such pane` otherwise; naming the option collapses the discrimination (`invalid option`, exit 1, for an unset option on a live pane), which is why it names none. The target is spent as a string at each argv and the failure is wrapped in the shape its siblings use. Pin both argvs and their order in `internal/tmux/tmux_test.go` through `commandertest.Scripted`, and extend `internal/tmux/pane_option_realtmux_test.go` with a set → read → unset → read round trip and a read against a target no live pane answers to.
```

*Do, last bullet:*
```
- Extend the options clause of CLAUDE.md's `tmux` package row to name the pane-option pair the marker is written and read through — `UnsetPaneOption` alongside `SetPaneOption`, and `ReadPaneOption` as the single-pane read: a `show-options -p -t <target>` existence probe naming no option followed by the `display-message -F` format read, taking that shape for the same reason `ResolveHookKey` does, since the format read alone answers a target no live pane answers to with exit 0 and an empty string.
```

*Acceptance Criteria, seventh bullet:*
```
- [ ] `tmux.ReadPaneOption` composes `show-options -p -t <target>` naming no option followed by `display-message -p -t <target> -F "#{<option>}"`, in that order, takes `tmux.Target`, reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to — on the probe's exit status, because the format read alone answers exit 0 and empty there; `internal/tmux/target_composition_guard_test.go` passes with it in place.
```

*Tests, ninth bullet:*
```
- `"it composes the existence probe and then the format read"` (scripted commander, both argvs asserted in order)
```

*Edge Cases, new bullet appended after "A marker read that itself fails is treated as still pending rather than as answered…":*
```
- A read against a target no live pane answers to fails on the probe's exit status and never on the format read — measured against tmux 3.7c on 2026-09-20, `display-message -p -t <gone pane> -F '#{@opt}'` exits 0 with an empty string, exactly what an unset option on a live pane reads as, so a single format read would report a pane that does not exist as one that was answered. `internal/tmuxtest`'s `ReadPaneToken` already documents that trap and `ResolveHookKey` already takes the probe-then-read shape against it; this method is the third reader to need the same discrimination and takes the same shape rather than a fourth.
```

**Resolution**: Fixed — task 4-4's Do gives `ReadPaneOption` the probe-then-read shape with the measurement stated in line, its CLAUDE.md edit bullet describes the same two reads, the acceptance criterion names both argvs in order and says the error comes off the probe's exit status, the scripted-commander test asserts both argvs in order, and a new Edge Case carries the measurement. Phase 4's task table carries the same edge case. Tick body re-synced and byte-verified.
**Notes**: Re-measured independently before applying, on a disposable `-L` socket against tmux 3.7c (2026-09-20): `display-message -p -t %0 -F '#{@nosuchopt}'` on a live pane, `-t %999`, and `-t '=gonesess:0.0'` all exit 0 with an empty string; `show-options -p -t %0` naming no option exits 0, `-t %999` exits 1, and `show-options -p -t %0 @nosuchopt` gives `invalid option` at exit 1. Every figure in the finding reproduces. The developer's own server was never touched — the probe ran on its own socket and the server was killed after.

---

### 2. The hydrate helper's existing lookup breadcrumbs are moved out from under the eight suites that pin them

**Severity**: Important
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Category**: Task Template Compliance (Do contradicts Acceptance Criteria)
**Move**: settled
**Change Type**: update-task

**Problem**:
The task's Do directs the executor to resolve the decision at the top of `runHydrate` "moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim" — a move, out of `execShellOrHookAndExit`. Its own acceptance criterion and edge case require the opposite: `execShellOrHookAndExit` called with a nil `Decision` must be "byte-identical to today: its own lookup, the same `hook lookup` hit/miss/error DEBUG records, the same lookup-failure WARN and the same exec — the eight existing direct callers pass with no edit". Those records live in `execShellOrHookAndExit` today (`cmd/state_hydrate.go`, the hit/miss/error `hook lookup` DEBUGs and the `lookup on-resume hook failed` WARN), and `cmd/state_hydrate_exec_log_test.go` and `cmd/hooks_read_lock_test.go` drive that function directly and assert on them. An executor following the Do literally removes them and breaks the suites the same task says must pass unmodified. This is the plan's largest task and the Do is what the executor works from, so the contradiction lands where it costs most.

**Proposal**:
Reword the clause to say what the acceptance criterion already decided: the emission is factored into one shared helper both sites call, not moved. The two call sites and one implementation is what keeps the nil-`Decision` path byte-identical while the decision path reads once.

**Current**:
```
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)` — moving the existing `hook lookup` DEBUG records and the existing lookup-failure WARN here verbatim — keeping the helper's existing absent-store guard so a hook store the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when that loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

**Proposed Text**:
```
- Add `type resumeDecision struct { Wait bool; Lookup hooks.OnResume; Exe, Pane string }` and `Decision *resumeDecision` on `hydrateConfig` (a pointer so a downgrade taken inside a by-value handler is seen by the caller, and so the values the mark step resolves reach the exec step). `Exe` and `Pane` are left empty at resolution — they are filled by the mark step, which is where the refusal has to be taken. Add `resolveResumeDecision(cfg hydrateConfig) *resumeDecision`, called once at the top of `runHydrate`: perform the single `LookupOnResume(cfg.HookKey, hooks.ViaHydrate)`, emitting the same `hook lookup` hit/miss/error DEBUG records and the same lookup-failure WARN `execShellOrHookAndExit` emits today — factored into one unexported helper both sites call rather than moved out of one into the other, because `execShellOrHookAndExit` goes on emitting them verbatim on its nil-`Decision` path, where the eight existing direct callers pin them — keeping the helper's existing absent-store guard so a hook store the command could not build stays the miss it is today rather than becoming the thing that ends the pane's only process — read the install default through `loadPrefsStoreNoMigrate()` + `LoadResumeMode()`, taking `resumemode.Default` without calling the accessor at all when that loader answers with no store, and discarding the accessor's own error when it does (the value beside it is already the shipped default), and set `Wait` when the lookup found a registration carrying a non-empty command and `resumemode.Resolve(lookup.Mode, install)` answers `Lazy`.
```

**Resolution**: Pending
**Notes**:

---

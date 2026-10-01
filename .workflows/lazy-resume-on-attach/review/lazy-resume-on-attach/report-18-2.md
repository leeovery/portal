TASK: A Waiting Pane Carries the Token That Protects Its Registration (tick-52e561, lazy-resume-on-attach-18-2)

ACCEPTANCE CRITERIA:
In each scenario, pane X was restored with a registration under its saved token T, and that registration resolves lazy.
- Restore's re-stamp of T failed, so X carries no `@portal-pane-id`. When the helper parks X, X carries T as its `@portal-pane-id`.
- Continuing, the daemon's stale-hook sweep runs while X waits. X's registration is still in `hooks.json` afterwards, so Enter on X's panel runs the registered command.
- Restore's re-stamp landed. Parking X leaves its `@portal-pane-id` reading T.
- On each of the helper's three tails (replay, signal timeout, missing scrollback file), the writes run token, then alternate-screen pin, then pending marker, all before the mid-restore marker is cleared.
- The helper never reads a pane's `@portal-pane-id` on any path. The token is written without a read first.
- The token write is refused: X does not wait — no pin and no pending marker are written; the registered command runs as an eager hook (`sh -c '<command>; exec $SHELL'`); exactly one `set resume pending marker failed` WARN names the pane and the token write's error.
- The pin or the marker is refused after the token write landed: X fires eagerly, no write removes T from X, and a refused marker still lifts the pin.
- No token is written when `$TMUX_PANE` is absent or the executable cannot be resolved (no pin or marker either), when the registration resolves eager, or when the pane has no registration.

STATUS: complete

SPEC CONTEXT: Spec 7.2 (and its 2026-09-30 corrigendum) states that restore's token re-stamp is best-effort, so the helper writes the pane's saved token itself, with no read first, before it pins and marks the pane. That is the value restore would have written, so no pane is stamped that would not otherwise be. Spec 8.2 lists the token, the pin and the marker as the three writes whose refusal keeps a pane from waiting. Such a pane fires its hook eagerly, under the single existing `set resume pending marker failed` WARN naming the pane and the error. A refused marker lifts the pin. CLAUDE.md's "Resume hooks" section was brought into line after the task commit.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_hydrate.go:393-419 (`markResumePending`). The token write is at cmd/state_hydrate.go:406, the pin at :409, the marker at :412 and the marker-refusal pin lift at :413-415. The refusal WARN is emitted by the unchanged caller at cmd/state_hydrate.go:372.
- Notes:
  - The write is `cfg.Client.SetPaneOption(pane, state.PortalPaneIDOption, cfg.HookKey)`. It runs only after `requireTmuxPane` and the executable both resolve, and ahead of the pin and the marker. It reads nothing first.
  - A refused token write returns the same way a refused pin does, so the existing WARN path names its error and the pane takes the eager hand-off. There is no new log event.
  - A later refusal performs no token unset. Restore's `restampPaneToken` is untouched (the task commit c4d00fed2 changes only cmd/state_hydrate.go and two test files).
  - `cmd/state_hydrate.go:406` is the only reference to `PortalPaneIDOption` in the helper's production files, so no path reads the token.
  - An empty baked key cannot reach the write. `LookupOnResume` refuses an empty key (internal/hooks/lookup.go:30-32), so `decision.Wait` is false.
  - The new doc-comment text at cmd/state_hydrate.go:384-388 is accurate against the code.

TESTS:
- Status: Adequate
- Coverage:
  - Re-stamp failed and re-stamp landed (cmd/state_hydrate_lazy_token_test.go:21-93): this runs against a real disposable tmux server. It checks that the parked pane reads T, then runs `hooksweep.Run` against that server and asserts the registration survives, then drives Enter through `runResumeWait` with the real store and asserts the hook hand-off.
  - The sweep assertion is not vacuous. `JudgeAgainstLivePanes` stands down only on zero pane rows (internal/hooksweep/sweep.go:98), not on zero tokens. With the token write removed, the single unstamped pane would make the token-shaped `SubjectSeedA` stale and the sweep would delete it.
  - Token, pin, marker and skeleton-clear ordering on all three tails, with exactly one token write and no token read (cmd/state_hydrate_lazy_test.go:755-782).
  - Refused token write:
    - The case added to `TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked` (cmd/state_hydrate_lazy_test.go:291-296) asserts no marker, no pin, the eager hook, and one WARN naming the pane and an error.
    - cmd/state_hydrate_lazy_test.go:784-801 asserts the WARN carries the token write's error text and that it is the only WARN.
  - Pin or marker refused after the token landed (cmd/state_hydrate_lazy_test.go:803-827): the token write is present, no call touches the token other than that write, and the hook fires eagerly. The pin lift on a refused marker stays covered at cmd/state_hydrate_lazy_test.go:364-417.
  - No token write when:
    - `TMUX_PANE` is absent or the executable cannot be resolved (`wantNoToken`, cmd/state_hydrate_lazy_test.go:309-317);
    - the registration resolves eager (cmd/state_hydrate_lazy_test.go:168);
    - there is no registration (cmd/state_hydrate_lazy_test.go:138).
- Notes: The tests are focused. The error-text test for the refused token write mirrors the existing pin-error test rather than duplicating a happy path. The real-tmux test follows the cmd package's existing unit-lane real-client precedent (cmd/completion_test.go:254): a disposable `tmuxtest` socket, no daemon, no built binary.

CODE QUALITY:
- Project conventions: Followed. The pane-scoped write goes through `SetPaneOption` with a typed `tmux.Target`, the option name comes from `state.PortalPaneIDOption`, and no seam is assigned directly in the tests.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None

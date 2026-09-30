# Consolidation Findings: Lazy Resume On Attach (Phase 18)

## Findings

### F1: The dump-skip rule lives in the daemon, so every other Dump restates it and one has already fallen behind
- **Class**: drift
- **Failure**: `CaptureCycle` (in `internal/state`) documents three sets that "the caller's own scrollback dump must skip". The rule that combines them (skeleton OR pending OR carried) is written only in `cmd`, as the daemon's private `paneSkipsScrollback`. Any other `Dump` has to write the rule again. This phase added the third set (`Carried`) and updated the daemon's copy. The lazy-panel integration fixture's copy still checks only `Skeleton` and `Pending`, and nothing flagged it: no compile error, no failing test. That fixture's doc comment still says it captures "the way the daemon takes one", which is no longer true. On real tmux, the fixture would capture-pane a carried session's address that the daemon never touches. That address may now belong to another pane, so the fixture would write that pane's bytes into the carried sibling's positional file. Or the session may be gone, and the fixture would fail on a target error the daemon never hits. So any carry scenario added to that suite would pass or fail on behaviour the daemon does not have. The next set added to `CaptureCycle` has the same problem: nothing points the author at every `Dump` that must learn it.
- **Evidence**:
  - `internal/state/scrollback.go:256-269`: `CaptureCycle` and its three sets.
  - `internal/state/scrollback.go:284-285`: `Carried` is filled from the capture.
  - `internal/state/commit_cycle.go:39-41`: the `Dump` field doc, which does not name the skip.
  - `cmd/state_daemon.go:340-351`: `paneSkipsScrollback`, the only production copy of the rule, with its reason comment.
  - `cmd/state_daemon.go:304`: its call site.
  - `internal/restore/lazy_resume_panel_integration_test.go:480-485`: the "the way the daemon takes one" claim.
  - `internal/restore/lazy_resume_panel_integration_test.go:497-502`: the inline skip that leaves out `Carried`.
- **Proposed shape**:
  - Declare the rule once, as a method on the type that owns the sets: `func (c CaptureCycle) SkipsScrollback(paneKey string) bool` in `internal/state/scrollback.go`, beside `CaptureCycle`. Move the reason comment from `cmd/state_daemon.go:340-343` with it.
  - Delete `paneSkipsScrollback`. Change `cmd/state_daemon.go:304` to call `capture.SkipsScrollback(paneKey)`.
  - Replace the fixture's two inline checks (`lazy_resume_panel_integration_test.go:497-502`) with the same method call. Its "the way the daemon takes one" claim then holds again.
  - Optionally, the `Dump` field doc can name the method as the skip a dump takes.
  - Pure refactor for production: the daemon's behaviour is unchanged. The fixture gains the carried skip, and none of its current scenarios produces a carry.
- **Bank**: reviewer (task 18-1): move the dump-skip rule onto `CaptureCycle` itself as a method in `internal/state`, so it is declared once. Confirmed: the fixture still restates it without `Carried`.

### F2: CLAUDE.md describes the capture cycle and the waiting pane's writes as they were before this phase
- **Class**: drift
- **Failure**: CLAUDE.md is the working guide contributors and agents write new committers and hydrate changes from. After this phase, it contradicts the code in two places.
  1. **The `state` row.**
     - It says `captureAndRefile` returns "a `CaptureCycle{Index, Pending, Skeleton}` whose two sets the caller's own scrollback dump must skip".
     - It says the saver's skip is "mid-restore **or** pending".
     - It says `commit-now` "discards both sets".
     - It never mentions the carry, or that a carry landing on the name of a session the capture reached fails the capture.

     A new `Dump` written from this row would skip only `Skeleton` and `Pending`. It would capture a carried session's address, which may by then answer to a waiting pane: the panel gets written over that pane's transcript, and nobody finds out until the transcript is missing at the next reboot. A reader chasing a `tick failed` WARN caused by a carry name collision would also find nothing here explaining it.
  2. **The Resume hooks lazy paragraph.**
     - It says a lazy pane is "first pinned ... and then marked".
     - It lists the causes that downgrade a pane to eager as "an absent `$TMUX_PANE`, an unresolvable executable, a refused alternate-screen pin or a failed marker write".

     The mark step now writes the pane's token first, and a refused token write is a fourth downgrade cause. Someone reading the list as complete would not recognise an eager pane whose WARN names a token-write error. Someone reading the neighbouring rule ("the firing path must never read the live pane token") next to a list of writes that has no token write in it could take the write for a violation and remove it. The sweep would then reap a waiting pane's registration whenever restore's re-stamp failed.
- **Evidence**:
  - `CLAUDE.md:61` (the `state` row): the sentences "returning a `CaptureCycle{Index, Pending, Skeleton}` whose two sets the caller's own scrollback dump must skip", "The saver's per-pane scrollback skip is mid-restore **or** pending — `CaptureCycle.Skeleton` ... or `CaptureCycle.Pending` ...", and "`state commit-now` reads `sessions.json` under the lock, discards both sets and dumps nothing".
  - `CLAUDE.md:190`: "has the pane first pinned to a pane-level `alternate-screen on` ... and then marked `@portal-resume-pending`", and "A pane that cannot be marked does not wait: an absent `$TMUX_PANE`, an unresolvable executable, a refused alternate-screen pin or a failed marker write each downgrade the decision to eager".
  - Against the code:
    - `internal/state/scrollback.go:256-269`
    - `internal/state/capture.go:44-49` and `:146-151`
    - `internal/state/capture.go:197-201` (the refusal)
    - `cmd/state_daemon.go:344-351`
    - `cmd/state_hydrate.go:384-388` and `:406-408`
- **Proposed shape**:
  - **`state` row.** Say the cycle returns three sets: `Skeleton`, `Pending` and `Carried`, the last being the pane keys of a session carried forward from the previous index. If F1 lands, say the dump skips what `CaptureCycle.SkipsScrollback` answers, instead of listing the sets. Add one sentence on the carry: a live waiting pane whose session missed the capture has its previous session carried whole under its previous name, with no token put on a second record, and a carry onto a reached session's name fails the capture, so the callers' existing failure routes retry. Change "discards both sets" to "discards the sets".
  - **Lazy paragraph.** Put the token write first in the ordered writes: the baked key is written as the pane's `@portal-pane-id`, with no read first. Add a refused token write to the list of downgrade causes.
- **Bank**: reviewer (task 18-1): bring the CLAUDE.md state row in line with the carry. Confirmed, and widened to cover task 18-2's token write, which falsifies the same file's lazy paragraph.

## Comment Corrections

- `internal/state/capture.go:64-65`: the comment only restates the struct's three fields; `CaptureCycle.Carried` already documents what the carried keys are.
  OLD: `// structureCapture is one structural capture: the index, the pending set` / `// CaptureStructure returns, and the pane keys of every carried session.`
  NEW: (delete)

## Spec Defects

### S1: The carry commits a whole previous session, which the specification's no-resurrection guard forbids
- **Claim**: §7.2, "A waiting pane keeps its place in the saved set throughout": "the structural capture still enumerates the pane into `sessions.json` and merges its *previous* record back into the fresh index — guarded so a stale marker cannot resurrect a pane whose session, window or pane is gone". §7.2, "The token match costs nothing": "the match is taken only against panes found in the same enumeration, which is what preserves the existing guard against resurrecting a pane whose session, window or pane is genuinely gone". §7.3 states the saver's skip as two conditions, mid-restore or waiting.
- **Observed**: The merge only reaches sessions the capture reached. A waiting pane whose session missed the capture (renamed or killed mid-capture, or failing its environment read) is kept by carrying the previous index's whole session forward under its previous name (`internal/state/capture.go:44-49`, `:146-151`, `:175-248`). That has four consequences:
  - A session killed after the pane enumeration is committed for that cycle, and dropped on the next (`internal/state/capture_carry_test.go:234-251`).
  - The carried session is the previous topology, not the live one. A sibling that has since closed is committed, and a sibling that has moved to another session is committed a second time, without its token, beside its live record (`capture_carry_test.go:279-318`).
  - Every pane of a carried session is kept off the scrollback dump. That is a third skip condition beside the two §7.3 names (`cmd/state_daemon.go:344-351`).
  - A carry that would land on a reached session's name refuses the whole commit. The daemon logs `tick failed` and re-touches `save.requested`; `commit-now` exits non-zero (`internal/state/capture.go:197-201`).

  The guard still holds for the waiting pane itself, because the same enumeration lists it. It does not hold for the rest of the session carried with it.
- **Read**: Spec stale. The approved remediation for a waiting pane's transcript being lost when its session misses a capture chose this carry knowingly: one cycle, for sessions holding a live waiting pane. §7.2 should record the missed-session carry, its scope, and the refused commit on a name collision. §7.3's statement of the skip should admit the carried-session skip.

### S2: The specification assumes every waiting pane is already stamped, and lists only two writes that can downgrade a pane
- **Claim**: §7.2: "A pane can only be waiting if it carries a registration, and registering one is what mints and stamps its token, while a mid-restore pane carries the token restore re-stamps from its saved record, so every pane this rule reaches is already stamped — no pane is stamped that would not otherwise be." §8.2: "**A pane that cannot be marked does not wait.** If the alternate-screen pin (§5.1) or the pending marker cannot be written, the helper does not paint", and the recorded WARN names "the error that refused the pin or the marker".
- **Observed**: After a reboot, the only thing that puts the token on a waiting pane is restore's re-stamp, and that re-stamp is best-effort: it logs `set pane token failed` and carries on (`internal/restore/session.go:157-168`). The registration was stamped in an earlier server lifetime. So a parked pane could carry no token. Its registration would then look stale to the hook sweep, which could delete it before Enter runs it. This phase makes the hydrate helper write the baked key as the pane's `@portal-pane-id`, with no read first, before the pin and the marker (`cmd/state_hydrate.go:384-388`, `:406-408`). A refused token write downgrades the pane to eager under the same `set resume pending marker failed` WARN (`cmd/state_hydrate_lazy_test.go`, "the token write fails" case and `TestHydrateLazy_NamesTheTokenWritesErrorWhenItIsRefused`).
- **Read**: Spec stale. The premise "every pane this rule reaches is already stamped" is false wherever restore's re-stamp fails. The landed code closes that by stamping on the waiting path, writing the same value restore would have written. §7.2 should say the helper writes the pane's saved token before it waits. §8.2's list of writes whose refusal keeps a pane from waiting, and the WARN that names the refused write, should include the token write.

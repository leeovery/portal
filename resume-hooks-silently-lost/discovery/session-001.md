## Discovery Session 001

Date: 2026-08-22
Work unit: resume-hooks-silently-lost

### Description (as of session)

Per-pane resume hooks go missing without any error — registration accepts a pane that
doesn't resolve and writes a degenerate key, and valid entries are orphaned when a pane's
window/pane coordinates change — so panes come back from a reboot with no resume.

### Seed

- seeds/2026-08-16-hook-set-persists-degenerate-pane-key.md (inbox:bug)
- seeds/2026-08-21-resume-hooks-vanish-when-pane-moves.md (inbox:bug)
- seeds/2026-06-29-some-sessions-skip-resume-on-reboot.md (inbox:bug)

### Imports

(none)

### Map State at Start

(n/a — single-topic work)

### Exploration

The work came in from the inbox as two bug captures selected together, with a third folded
in during shaping.

The first capture (2026-08-16) records that `portal hook set --on-resume` exits 0 and
persists an entry even when `$TMUX_PANE` names a pane that does not exist. The hook key
collapses to the literal `:.` — empty session component, empty window and pane indices —
and lands in `hooks.json` under that key. Reproduced against Portal 0.11.0 with
`TMUX_PANE=%165 portal hook set --on-resume "noop"` on a dead pane. The capture notes this
is recurrent rather than a one-off: `portal.log` carries `hooks: rm op=rm hook_key=:.
via=cli` lines across both 0.10.5 and 0.11.0 (2026-08-11, 2026-08-14 twice, 2026-08-15
twice), each followed within seconds or minutes by a daemon `hooks: clean-stale
op=clean-stale entries=1` — meaning the recurring single-entry stale sweeps in the log are
this junk key being reaped, not genuine staleness, which makes the clean-stale signal
misleading when reading the log for real problems. The consequence named is that a failed
registration is indistinguishable from a successful one at the call site: the Claude Code
SessionStart hook at `~/.claude/hooks/portal-resume-hook.sh` shells out to `portal hooks
set` and records the exit code, so it logs success while the pane it meant to protect has no
hook at all. A 2026-08-16 audit found six of forty-two panes running Claude with no hook
registered. The capture flags the `rm` path as worth checking too, since removal evidently
accepts the degenerate key, and names the surfaces: the hook-key derivation primitives in
`internal/tmux` (`ResolveHookKey`, `HookKeyFormat`), the `hook set` body in `cmd/hooks.go`,
and the `internal/hooks` store.

The second capture (2026-08-21) records registered hooks disappearing from `hooks.json` with
no user action and no error, surfacing only after a reboot when a restored pane comes back
to a bare shell. Two live instances: `agentic-workflows-refactor-wt` (pane `OCCvED:1.1`),
hook registered 2026-08-16T22:17:55 under key `OCCvED:1.2` and pruned by the daemon at
22:22:16 roughly four minutes later; and `dex` window 2 (pane `d5H0DK:2.1`), registered
2026-08-20T22:48:07 under key `d5H0DK:1.2` and pruned on 2026-08-21. In both the hook was
registered correctly with the right command and session id, and the pane is still alive —
what changed is the pane's position. The key bakes `window.pane` coordinates at registration
and nothing re-keys the entry when the pane moves, so the key stops addressing its own pane
and the daemon's stale sweep removes it, correctly by its own rule. The coordinates move
under ordinary use: tmux derives `pane_index` from layout position so closing a sibling
renumbers the survivor, this install runs `renumber-windows on` so closing a window
renumbers later windows, and `break-pane` / `move-pane` change both halves. Reproduced in an
isolated `-L` socket: one unchanged pane went `t:1.2` → `t:2.1` → `t:1.1` across a
break-pane and a window kill — three keys for one pane. Impact is silent loss of session
continuity across reboots, proportional to how much the user rearranges panes; scrollback
still restores so the pane looks healthy, and `portal doctor` reports green throughout,
including `stale hooks: no stale hooks`. Named surfaces: `internal/tmux/tmux.go`
(`HookKeyFormat`, `HookKey`, `ListAllPaneHookKeys`), `internal/hooks/store.go` (`StaleKeys`,
`CleanStale`), `internal/state/capture.go` (`captureFormat`), `internal/restore/session.go`,
`cmd/hooks.go`. The capture explicitly distinguishes itself from the known subagent
SessionEnd clobber and from the degenerate `:.` issue, calling those separate failure modes
on the registration side.

The user's call was to treat the two as one fix rather than two bugfixes. The reading behind
that: both are the hook key failing to be a durable identity for a pane — one at the moment
of writing it, one over the pane's lifetime — and both fail silently, so a pane ends up
unprotected with nothing reporting it.

A third inbox capture was then folded in as a seed: "Some sessions don't resume their work
after a reboot" (2026-06-29), which records that after a crash-recovery reboot with 32
Claude sessions, roughly 28 came back fully resumed and around 4 did not — the tmux sessions
restored but the on-resume behaviour never happened for those panes. No shared
characteristic was identified; the user's hunch at the time was that the non-resuming ones
were older sessions, and the framings offered were a gap in hook firing for those panes or
an issue in the restore/resume logic itself. That capture was parked from the
`restore-host-terminal-windows` discovery with no reproduction attempted and no root cause
assumed. It reads as a symptom-level sighting of the same failure: an older pane has had
more opportunity to be rearranged, so its key drifts and the sweep reaps it, which matches
both the minority-subset shape and the age correlation. Folding it in means one
investigation closes all three rather than the same ground being covered twice. The residual
risk was named at the time of folding: if the four panes turn out to have failed for a
different reason — legacy sessions missing markers the restore path expects, which that
capture also floats — then an unrelated symptom is being carried through the fix, and the
investigation should say so rather than force the fit.

Shape signals were consistent throughout: all three items are present-broken with observed
instances (pruned keys at recorded timestamps, `:.` entries recurring across two releases,
6/42 and 4/32 pane audits), none proposes new behaviour, and there is a real root cause to
establish plus a key-identity decision behind the fix — so bugfix rather than quick-fix, and
one work unit rather than three.

### Edits

(none)

### Topics Identified

(none)

### Conclusion

Routed to investigation.

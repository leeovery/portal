# Portal command for cleaning up ghost tmux clients

When working from iPad over Blink + Mosh, opening and closing Blink windows leaves behind a persistent chain: mosh-server stays alive (Mosh is UDP, no clean disconnect signal), its child shell stays alive, the tmux client running inside that shell stays registered with the tmux server. Result: every "open Blink → close Blink" cycle leaves another ghost tmux client attached to the same tmux server. Over a few days these accumulate — observed 21 attached clients across 17 sessions on one occasion, with a single tmux server pinned at ~94% CPU because tmux's event loop is single-threaded and has to fan out redraws to every attached client. The iPad becomes nearly unusable for typing or scrolling under that load.

Portal already owns tmux session lifecycle, so this feels like the natural place for a non-destructive cleanup affordance. The core primitive is `tmux detach -a` — detaches every client except the one running the command. Zero data loss: tmux sessions, windows, panes, scrollback all survive. Only the redundant client connections get unregistered. Running this from the live iPad session instantly reclaims responsiveness.

A possible shape: `portal cleanup` (or `portal detach-others`, or similar) that wraps `tmux detach -a`. Could optionally go further and SIGTERM mosh-server processes that no longer have a live tmux client process beneath them — those are provably orphaned at that point. Mapping mosh-server → child shell → tmux client process is straightforward via process tree.

Two related thoughts that came up but don't necessarily belong in the same command:

- Root-cause prevention: ensure every place Portal does a tmux attach uses the `-d` flag, so attaching from a new Blink window forcibly detaches any prior client of that session. With `-d` everywhere, ghosts can't multiply per session even when mosh-servers pile up.
- A `MOSH_SERVER_NETWORK_TMOUT` cap was considered as a system-wide reaper, but rejected because any short timeout breaks the legitimate "iPad asleep overnight, reconnect in the morning" workflow, and any long timeout doesn't actually solve the accumulation (each new Blink connection still spawns a new mosh-server regardless of timeout).

Trigger pattern is on-demand rather than scheduled — user runs it when iPad responsiveness starts degrading, or proactively before a long working session.

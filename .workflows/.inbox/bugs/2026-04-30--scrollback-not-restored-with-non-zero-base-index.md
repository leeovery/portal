# Scrollback hydration silently fails when tmux uses non-zero base-index / pane-base-index

On a tmux setup that sets `base-index 1` and `pane-base-index 1` in `tmux.conf` (so windows and panes start numbering at 1 instead of the tmux default 0), Portal's restore step recreates sessions and panes at the right cwd and layout, but the saved scrollback is never replayed into the new pane. The pane comes up with only a fresh prompt; the session looks restored but feels empty.

The save side is fine — `~/.config/portal/state/sessions.json` lists the session and points at a per-pane scrollback file, and the binary scrollback file on disk contains the expected ANSI-coloured terminal history (verified by stripping escape codes and reading the tail). The break is purely on restore.

Portal v0.3.0 logs make the cause clear. After `tmux kill-server` and a reattach, `~/.config/portal/state/portal.log` contains:

```
WARN | restore | session "-dotfiles-HM9Zhw": pane 0 predicted=-dotfiles-HM9Zhw__0.0 live=-dotfiles-HM9Zhw__1.1
WARN | hydrate | timeout waiting for signal on --hook-key=-dotfiles-HM9Zhw:1.1 --fifo=/Users/lee/.config/portal/state/hydrate--dotfiles-HM9Zhw__1.1.fifo
```

Restore predicts the pane id as `__0.0` (window 0, pane 0) but tmux actually places it at `__1.1` because of the user's base-index / pane-base-index settings. The mismatch is logged but treated as non-fatal, and restore continues. The follow-up "hydrate" step (the bit that replays scrollback into the live pane) waits for a tmux hook signal via a FIFO whose hook-key is built from the predicted pane id. That hook never fires for a pane id tmux doesn't know about, the FIFO wait hits its timeout, hydration is abandoned, and no scrollback ends up in the pane.

Repro: tmux.conf with `set -g base-index 1` and `setw -g pane-base-index 1`, run a session that produces some scrollback, let Portal save, then `tmux kill-server` and reattach. Layout/cwd come back; scrollback does not. Removing the two base-index settings — or matching Portal's predicted indices — makes the symptom go away, which confirms the diagnosis.

Two reasonable fix directions: either read the `base-index` and `pane-base-index` server options when computing the predicted pane key on restore, or stop predicting altogether and look up the actual session/window/pane indices after `new-session` / `split-window` before arming the hydrate hook. The second is more robust against any other indexing-affecting options users might set.

Impact: anyone with a non-default tmux base-index loses scrollback persistence entirely. Save daemon reports success ("Sessions captured", "Panes captured") so the failure isn't visible until you actually try to restore — making it easy to assume Portal is working when it's silently dropping the most user-facing part of restore.

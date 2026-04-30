# Investigation: Scrollback Not Restored With Non-Zero Base Index

## Symptoms

### Problem Description

**Expected behavior:**
After `tmux kill-server` and reattach, Portal restores sessions/windows/panes with their saved scrollback replayed into each pane (matching the experience users get with default tmux indexing).

**Actual behavior:**
With tmux configured `set -g base-index 1` and `setw -g pane-base-index 1`, Portal restores skeleton (sessions/windows/panes), cwd, and layout, but the saved scrollback never appears in the pane. The pane comes up with only a fresh shell prompt — the session looks restored but feels empty.

### Manifestation

`~/.config/portal/state/portal.log` contains, for each pane after restart:

```
WARN | restore | session "-dotfiles-HM9Zhw": pane 0 predicted=-dotfiles-HM9Zhw__0.0 live=-dotfiles-HM9Zhw__1.1
WARN | hydrate | timeout waiting for signal on --hook-key=-dotfiles-HM9Zhw:1.1 --fifo=/Users/lee/.config/portal/state/hydrate--dotfiles-HM9Zhw__1.1.fifo
```

The `predicted` pane key uses default-index coordinates (window 0, pane 0); the `live` pane key reports actual coordinates (window 1, pane 1). Restore is non-fatal on this mismatch. The hydrate helper waits on a FIFO armed against the predicted key; the tmux hook keyed off the predicted key never fires for a pane id tmux doesn't know about, so the FIFO wait hits its timeout and hydration is abandoned silently.

Save side is fine — `~/.config/portal/state/sessions.json` records sessions and per-pane scrollback files; binary scrollback files on disk contain expected ANSI-coloured terminal history.

### Reproduction Steps

1. tmux.conf includes `set -g base-index 1` and `setw -g pane-base-index 1`.
2. Open a Portal-managed session, generate scrollback (run a few commands, view a long file, etc.).
3. Allow Portal save daemon to capture (logs "Sessions captured", "Panes captured").
4. `tmux kill-server`.
5. Reattach via Portal.

**Reproducibility:** Always, when both `base-index` and `pane-base-index` are non-zero. Removing those two settings makes scrollback restore work.

### Environment

- **Affected environments:** Any user with tmux base-index / pane-base-index set to non-zero (a common preference).
- **Platform:** Reported on Portal v0.3.0; not platform-specific.
- **User conditions:** Triggered purely by tmux config; no Portal-side toggle.

### Impact

- **Severity:** High — silent loss of the most user-facing restore feature for a common tmux configuration.
- **Scope:** Anyone using non-default base-index settings.
- **Business impact:** Save daemon reports success, so users may believe Portal is working while scrollback is being silently dropped on every restart.

### References

- Inbox bug report: `.workflows/.inbox/.archived/bugs/2026-04-30--scrollback-not-restored-with-non-zero-base-index.md`
- Portal log excerpt above.

---

## Analysis

_To be filled in during code analysis._

### Initial Hypotheses

Restore predicts pane keys assuming default tmux indexing (windows/panes start at 0). When users set `base-index 1` / `pane-base-index 1`, the actual indices shift, the predicted key diverges from the live key, and the hydrate FIFO/hook handshake — keyed off the predicted id — never closes.

### Code Trace

_Pending._

### Root Cause

_Pending synthesis._

---

## Fix Direction

_To be filled in after analysis and findings review._

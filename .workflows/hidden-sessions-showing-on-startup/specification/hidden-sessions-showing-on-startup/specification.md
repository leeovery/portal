# Specification: Hidden Sessions Showing On Startup

## Specification

## Problem Statement

### Symptoms

After fresh tmux/Portal startup, two sessions appear in the Portal session
list that should be hidden by default:

1. **`0`** — an unnamed bootstrap session created by Portal's own
   `StartServer` (a side-effect of `tmux new-session -d`).
2. **`_portal-saver`** — Portal's internal saver session that hosts
   `portal state daemon`.

Both surface in:

- The TUI session picker (`internal/tui`).
- `portal list` CLI output (`cmd/list.go`).

### Reproduction

1. Start tmux fresh (no existing server) or kill the tmux server.
2. Run `portal` (or `portal open` / `x`).
3. Open the TUI session picker (or run `portal list`).
4. Both `0` and `_portal-saver` are visible.

Reproducibility: always.

### Scope

- Severity: low (cosmetic / UX clutter, no data loss).
- Affected: all Portal users on all platforms.
- Confirmed independent of `tmux-resurrect` / `tmux-continuum` —
  Portal itself is the source of both unwanted sessions.

### Design Intent

The `built-in-session-resurrection` specification mandates that Portal
filters sessions whose names begin with `_` from:

- The TUI session picker.
- `sessions.json` capture.
- Any future internal-only sessions.

The capture-side filter shipped. The TUI / `portal list` filter did
not. The bootstrap session was never given an underscore-prefixed name,
so even with the spec-mandated filter in place it would still leak.

---

## Working Notes

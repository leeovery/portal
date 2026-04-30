# Investigation: Hidden Sessions Showing On Startup

## Symptoms

### Problem Description

**Expected behavior:**
After tmux/Portal startup, sessions whose names start with an underscore
(e.g. `_portal-saver`) should be hidden by default in the Portal session list.
The user recalls this convention being decided.

**Actual behavior:**
Two unwanted sessions are visible in the Portal session list at startup:

1. A session named `0` — suspected to be from the tmux-resurrect plugin.
2. `_portal-saver` — Portal's own internal saver session that hosts
   `portal state daemon`.

### Manifestation

- Session picker / session list shows the `_portal-saver` row.
- Session picker / session list shows a `0` row.
- User expectation: both should be hidden by default.

### Reproduction Steps

1. Start tmux fresh (no existing server) or kill the tmux server.
2. Run `portal` (or `portal open` / `x`) — this triggers bootstrap, which
   creates `_portal-saver` (step 4) and runs restore (step 5).
3. Open the TUI session picker.
4. Observe: both `0` and `_portal-saver` appear in the list.

**Reproducibility:** Always (per user report).

### Environment

- **Affected environments:** Local (only environment for this CLI).
- **Browser/platform:** macOS (per CLAUDE.md notes — config migration from
  macOS path).
- **User conditions:** Fresh tmux start; possibly with tmux-resurrect plugin
  installed (suspected source of the `0` session).

### Impact

- **Severity:** Low (cosmetic / UX clutter; no data loss).
- **Scope:** All Portal users with internal sessions or stray
  resurrect-plugin sessions.
- **Business impact:** Confusing UX — internal infrastructure leaks into the
  user-facing list.

### References

- `internal/tmux` — `BootstrapPortalSaver`, `EnsurePortalSaverVersion`,
  `ListSessions`.
- `internal/tui` — session listing page.
- `cmd/open.go` — TUI launch path.

---

## Analysis

### Initial Hypotheses

1. Underscore-prefix hiding was discussed but never actually implemented in
   the session listing code.
2. It was implemented but bypassed somewhere (e.g. the loading page or
   bootstrap path is showing a raw list).
3. The `0` session is created by the tmux-resurrect plugin running in the
   user's `~/.tmux.conf`, not by Portal itself.

To validate, trace:
- `tmux.ListSessions` for any name-based filtering.
- `internal/tui` session list rendering for filtering.
- `cmd/open.go` for any pre-filtering before passing to TUI.
- `BootstrapPortalSaver` to confirm the saver session naming.
- Search the codebase for any `strings.HasPrefix(..., "_")` or similar
  hidden-by-prefix logic.

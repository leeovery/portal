# Investigation: Cold-Boot Restore Lands on the Projects Page, Not Sessions

## Symptoms

### Problem Description

**Expected behavior:**
On a cold start (no tmux server yet) through the TUI picker, after the concurrent bootstrap restores every saved session, the picker should open on the **Sessions** page — matching the warm path, which lands on Sessions.

**Actual behavior:**
On the cold concurrent-bootstrap path, the picker opens on the **Projects** page instead of Sessions despite N sessions being restored correctly. The loading screen reports `Restoring sessions N/N` accurately, but the user must press `x` to reach the restored sessions.

### Manifestation

- Loading screen shows `✓ Restoring sessions 12/12 · ✓ Replaying scrollback · ✓ Running resume commands` (accurate).
- Picker opens on **Projects** (e.g. 10 projects), footer shows `x sessions`.
- Pressing `x` reveals the **Sessions** page with all 12 restored sessions (correct names, scrollback intact).
- So restore itself is fully correct — only the **initial page selection** is wrong.

### Reproduction Steps

1. Cold container, no tmux server, `sessions.json` + scrollback present (demo harness: `demo/`, sandboxed Linux container with a baked restore seed of 12 sessions).
2. `portal open` (the TUI picker) → loading screen shows `Restoring sessions 12/12`.
3. Picker opens on **Projects** (10 projects), footer `x sessions`.
4. Press `x` → **Sessions** page lists all 12 restored sessions.

**Reproducibility:** Repeatable in the demo harness (cold path). Warm path (server already running) lands on Sessions as expected — defect is specific to the cold concurrent-bootstrap landing decision.

### Environment

- **Affected environments:** Cold start (no tmux server) via the TUI picker, in the `demo/` sandboxed Linux container.
- **Browser/platform:** Linux container (demo harness `demo/portal-cold.tape`).
- **User conditions:** Saved `sessions.json` + scrollback present; tmux server not yet running so the concurrent cold-path bootstrap fires.

### Impact

- **Severity:** Low (UX). Not a data/correctness issue — sessions and scrollback restore fine.
- **Scope:** Anyone who cold-boots (reboot/fresh container) into the picker with restorable sessions.
- **Business impact:** Mildly surprising; costs an extra keypress (`x`) after a reboot to reach the just-resurrected sessions.

### References

- Seed: `seeds/2026-06-25-cold-boot-restore-lands-on-projects.md` (inbox:bug)
- Discovery: `discovery/session-001.md`
- Observed while building the cold-boot resurrection demo (`demo/portal-cold.tape`) for spectrum-tui-design, 2026-06-25.

---

## Analysis

### Initial Hypotheses

(Seed hypothesis — to verify, not asserted) The Loading → page transition chooses Sessions-vs-Projects from a session count captured *before* the restored sessions are visible to `ListSessions` — an ordering/race between restore completion on the `BootstrapCompleteMsg` path and the "no sessions yet → fall back to Projects" landing rule. Suggested to compare the cold-path landing decision in `internal/tui/model.go` (the `BootstrapCompleteMsg` handler / first non-loading page selection) against the warm path, which sees sessions at init and lands on Sessions.

Not personally reproduced by the user — observed by an agent running portal + tmux in the sandboxed container.

### Code Trace

(pending)

### Root Cause

(pending)

### Contributing Factors

(pending)

### Why It Wasn't Caught

(pending)

### Blast Radius

(pending)

---

## Fix Direction

(pending — populated after analysis and findings review)

---

## Notes

(pending)

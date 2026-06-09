# Specification: Status Recent Warnings Pipe Format

## Specification

## Problem & Scope

### Bug

`portal state status` always reports **`Recent warnings: 0 (last: none)`** regardless of what WARN/ERROR lines `portal.log` actually contains. The failure is silent — no error, no crash. The "Recent warnings" section is permanently empty in production, masking real daemon/bootstrap/restore warnings.

### Root cause

The status log reader (`internal/state/status.go`) parses `portal.log` assuming a **legacy pipe-delimited** field layout (`timestamp | level | component | message`, separator `" | "`, 4 fields). The observability layer changed the *writer* (`internal/log`) to **slog text** format:

```
<RFC3339Nano> <LEVEL> <component>: <msg> <attrs k=v…> pid=… version=… process_role=…
```

There are no `" | "` separators in this format, so `logEntryQualifies`'s `strings.SplitN(line, " | ", 4)` yields a single-element slice (`len < 4`) for **every** line and returns `false` unconditionally. `RecentWarnings` is therefore always `0` and `LastWarning` always `""`.

The contract between the writer (`internal/log`) and reader (`internal/state/status.go`) was broken on the writer side with no corresponding reader update, and no test exercised the reader against real writer output.

### Blast radius

**Affected — the two warning-derived fields only:**
- `portal state status` "Recent warnings" line → always `0 (last: none)`.
- The health/exit-code policy: `isUnhealthy`'s `RecentWarnings > 0` branch (`cmd/state_status.go`) can never fire, so a daemon actively logging WARN/ERROR still reports healthy (exit 0).

**Not affected (confirmed):**
- All other status fields — daemon running/PID/version, last save, sessions/panes counts, state size — read `daemon.pid` / `daemon.version` / `sessions.json` / the state-dir tree, **not** `portal.log`.
- No other production code reads `portal.log` (`scanRecentWarnings` is the only non-test caller of `PortalLog(`).

### Out of scope

- **The writer/handler is correct** and is not changed. Only the reader drifted; only the reader is fixed.
- No change to the recent-warnings window (last hour), the level filter (WARN/ERROR), the last-wins semantics, the malformed-line swallow-and-skip contract, or `CollectStatus`'s best-effort no-error-propagation behaviour.

### Severity & release class

Medium — a diagnostic/observability regression (the command lies by omission), not data-loss or a crash. Suitable for a **regular release**, not a hotfix.

---

## Working Notes

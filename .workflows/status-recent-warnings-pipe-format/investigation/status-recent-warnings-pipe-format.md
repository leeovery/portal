# Investigation: Status "Recent Warnings" Reads Zero (Pipe-Format Mismatch)

## Symptoms

### Problem Description

**Expected behavior:**
`portal state status` reports the actual recent warnings the daemon (and other
portal processes) logged in its "Recent warnings" section.

**Actual behavior:**
`portal state status` reports zero recent warnings regardless of what was
logged, because its log scanner still parses the **legacy pipe-delimited**
log format (`logFieldSeparator = " | "`) while `internal/log` now writes
slog **text** format (`<RFC3339Nano> <LEVEL> <component>: <msg> <attrs>`).
The formats no longer agree, so the scan silently matches nothing.

### Manifestation

- No error, no crash — silent. The "Recent warnings" section is always empty
  in production, masking real daemon/bootstrap/restore warnings.

### Reproduction Steps

1. Run portal so the daemon logs at least one WARN-level line to `portal.log`.
2. Run `portal state status`.
3. Observe the "Recent warnings" section reports zero / nothing.

**Reproducibility:** Always (deterministic format mismatch).

### Environment

- **Affected environments:** All (local + any install running the
  post-observability-layer build).
- **Platform:** n/a (format parsing, platform-independent).

### Impact

- **Severity:** Medium — a diagnostic/observability regression, not a
  data-loss or crash bug. The status command lies by omission.
- **Scope:** Anyone relying on `portal state status` to surface recent warnings.

### References

- Seed: `seeds/2026-06-02-status-recent-warnings-pipe-format.md` (inbox:bug,
  surfaced from review of `portal-observability-layer`).
- Discovery: `discovery/session-001.md`.

---

## Analysis

### Initial Hypotheses

(Seed claim, to be verified during code analysis:)
- `internal/state/status.go` (`scanRecentWarnings` / `logEntryQualifies`,
  `logFieldSeparator = " | "`) still parses the legacy pipe-delimited format.
- `internal/log`'s handler now writes slog text format.
- The two no longer agree → the warning scan matches nothing.
- Tests (`cmd/state_status_test.go`, `internal/state/status_test.go`) still
  seed pipe-format lines, so they pass against the stale parser.

Open questions deferred from discovery to this phase:
- How deep does the mismatch go — is **only** the warnings scanner affected,
  or do other `portal state status` fields read the log too?
- Do the status tests still seed the old pipe format (and thus give false
  green)?

### Code Trace

(to be populated during Step 5)

### Root Cause

(to be populated during Step 6)

---

## Fix Direction

(to be populated during Steps 6–8)

---

## Notes

User directive carried from discovery: investigate first; if the root cause is
not easily identifiable, stop the work; if it is, proceed to the fix.

# Investigation: File-Browser Alias Creation Emits No Audit Breadcrumb

## Symptoms

### Problem Description

**Expected behavior:**
Saving an alias from the shared file-browser "save alias for highlighted dir"
flow (the `a`-key, `handleAliasSave` in `internal/ui/browser.go`, wired into
`cmd/open.go`) should emit an `aliases: set` audit breadcrumb in `portal.log`,
exactly like every other production alias-mutation site.

**Actual behavior:**
Aliases created this way leave **no** `aliases: set` breadcrumb in `portal.log`.
The flow uses the un-audited two-step `store.Set(...)` + `store.Save()` rather
than the audited `SetAndSave` seam. The `AliasSaver` interface this flow depends
on exposes only `Load` / `Set` / `Save` — so the audited path is structurally
unreachable from this caller.

### Manifestation

- Missing observability: no `aliases: set` log line for file-browser-created
  aliases.
- Defeats the State-mutation audit-trail spec guarantee of "a single place per
  file where the breadcrumb can't be forgotten."

### Reproduction Steps

1. Launch the file browser (`portal open` → TUI file browser, or `cmd/open.go`
   path).
2. Highlight a directory and press `a` to save an alias for it.
3. Inspect `portal.log` — observe no `aliases: set` breadcrumb is emitted.

**Reproducibility:** Always (structural — the audited seam is not wired in).

### Environment

- **Affected environments:** All (local — this is a CLI/TUI tool).
- **Platform:** n/a
- **User conditions:** Any alias created via the file-browser `a`-key flow.

### Impact

- **Severity:** Low (observability gap, not a functional/data defect — the alias
  is still correctly written; only the audit log line is missing).
- **Scope:** Every alias created via the file-browser flow.
- **Business impact:** Incomplete audit trail; weakens the observability spec's
  chokepoint guarantee.

### References

- Seed: `seeds/2026-06-02-file-browser-alias-breadcrumb.md` (inbox:bug)
- Source: review of `portal-observability-layer/portal-observability-layer`
- Precedent: Task 3-5 instrumented `cmd/alias.go` + `internal/tui/model.go` via
  the audited `SetAndSave(name, path, "cli")` seam; the file-browser site was
  outside that task's literal "Do" list.

---

## Analysis

### Initial Hypotheses

The file-browser alias-save flow (`handleAliasSave`) is a third production
alias-mutation site still using the un-audited `Set` + `Save` two-step. Its
`AliasSaver` interface omits the audited `SetAndSave` method, so the breadcrumb
cannot be emitted. Suspected fix: extend `AliasSaver` to expose `SetAndSave`,
thread `handleAliasSave` onto it, and update the file-browser mock accordingly.

### Code Trace

_(to be completed during Code Analysis)_

### Root Cause

_(to be completed)_

---

## Fix Direction

_(to be completed during findings review)_

---

## Notes

Seed flagged this as a clear, narrowly-scoped bugfix: working code missing the
audit breadcrumb every other alias-mutation site emits. No new behaviour to
design.

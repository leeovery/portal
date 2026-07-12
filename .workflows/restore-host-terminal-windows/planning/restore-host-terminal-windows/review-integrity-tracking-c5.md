---
status: in-progress
created: 2026-07-12
cycle: 5
phase: Plan Integrity Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Integrity

## Findings

### 1. Shared `quoteJoin`/`goneVerb` helpers are unexported but called across package boundaries (compile break)

**Severity**: Important
**Plan Reference**: Task 3.4 (definition + first use), rippling to Task 3.6, Task 6.6, Task 6.7
**Category**: Task Self-Containment / Dependencies and Ordering (cross-phase seam)
**Change Type**: update-task

**Details**:
The last cycle centralised the two message helpers in a single home (`internal/spawn/message.go`, Task 3.4) so the CLI and picker render byte-identical gone/failed copy. The home is correct — `internal/spawn` is imported by both `cmd` and `internal/tui`, with no import cycle — but the helpers are authored **unexported** (`func quoteJoin`, `func goneVerb`) and then **called unqualified** from packages other than `spawn`:

- Task 3.4 (`internal/spawn/message.go` definition) but the call site `quoteJoin(gone)` / `goneVerb(len(gone))` is inside `runSpawn`, which lives in `cmd/spawn.go` (package `cmd`).
- Task 3.6: `quoteJoin(failed)` — also in `cmd/spawn.go` (package `cmd`).
- Task 6.6: `quoteJoin(failedNames)` — in the picker (package `tui`).
- Task 6.7: `quoteJoin(msg.Gone)` / `goneVerb(len(msg.Gone))` — in the picker (package `tui`).

An unexported identifier in package `spawn` is unreachable from `cmd` and `tui`; every one of these call sites is a cross-package reference to an unexported function and will not compile (`undefined: quoteJoin`). The defect manifests immediately in Task 3.4 itself (its own consumer is package `cmd`), not only downstream.

Every sibling shared `internal/spawn` helper in this plan already uses the correct exported-and-qualified pattern — `spawn.PreflightMissing` (Task 3.4), `spawn.ParseSpawnAckFlag` / `spawn.SpawnMarkerName` (Task 3.3), `spawn.NewServerOptionAckChannel`, etc. `quoteJoin`/`goneVerb` are the lone exception. The fix is to export them (`QuoteJoin`, `GoneVerb`) and qualify all four call sites (`spawn.QuoteJoin(...)` / `spawn.GoneVerb(...)`), matching `PreflightMissing`. The `internal/spawn/message.go` home and single-ownership rule are unchanged; only the identifier visibility and the call-site qualification change. (Task 6.7's "do NOT re-declare a second `func goneVerb`/`quoteJoin` in `internal/spawn`" note should likewise refer to `spawn.QuoteJoin`/`spawn.GoneVerb`.)

The core shared substring — `'<name>' is/are gone — nothing opened` / `... others left open` — stays the lockstep contract; only the CLI-vs-TUI surface wrappers (`spawn:` prefix + `fmt.Errorf` on the CLI; the `⚠` glyph + section-header render in the picker) differ, exactly as intended.

**Current** (Task 3.4, `cmd/spawn.go` `runSpawn` gone-session branch — the definition bullets and their call site):

```
    - **Define the two shared `internal/spawn` message helpers here** (this task is their first consumer; Task 3.6's leave-what-opened message and the Phase-6 picker in Tasks 6.6/6.7 reuse them verbatim, so they live in `internal/spawn` — a new `internal/spawn/message.go` — for CLI/picker lockstep):
      - `func quoteJoin(names []string) string` — single-quote each name and join with `, `: renders `'s2'` for one name and `'s2', 's4'` for several.
      - `func goneVerb(n int) string { if n == 1 { return "is" }; return "are" }` — the count-aware verb (`"is"` for one / `"are"` for several).
    - `return fmt.Errorf("spawn: %s %s gone — nothing opened", quoteJoin(gone), goneVerb(len(gone)))` — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. Both helpers are defined **once, here (this task)** and reused verbatim by Tasks 3.6/6.6/6.7 (which must **not** re-declare them), so the CLI and picker stay in lockstep. A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.
```

**Proposed** (Task 3.4 — export the helpers and qualify the call site):

```
    - **Define the two shared `internal/spawn` message helpers here** (this task is their first consumer; Task 3.6's leave-what-opened message and the Phase-6 picker in Tasks 6.6/6.7 reuse them verbatim, so they live in `internal/spawn` — a new `internal/spawn/message.go` — for CLI/picker lockstep). They MUST be **exported** (capitalised), because their consumers live in other packages — `runSpawn` here and in Task 3.6 is package `cmd` (`cmd/spawn.go`), and the Phase-6 picker in Tasks 6.6/6.7 is package `tui` (`internal/tui`). An unexported `quoteJoin`/`goneVerb` in `internal/spawn` is unreachable from `cmd`/`tui` and will not compile. Follow the identical exported-cross-package pattern already used by `spawn.PreflightMissing` (this task) and `spawn.ParseSpawnAckFlag`/`spawn.SpawnMarkerName` (Tasks 3.1/3.3):
      - `func QuoteJoin(names []string) string` — single-quote each name and join with `, `: renders `'s2'` for one name and `'s2', 's4'` for several.
      - `func GoneVerb(n int) string { if n == 1 { return "is" }; return "are" }` — the count-aware verb (`"is"` for one / `"are"` for several).
    - `return fmt.Errorf("spawn: %s %s gone — nothing opened", spawn.QuoteJoin(gone), spawn.GoneVerb(len(gone)))` — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. Both helpers are defined **once, here (this task)** as exported `internal/spawn` functions and reused verbatim (called as `spawn.QuoteJoin` / `spawn.GoneVerb`) by Tasks 3.6/6.6/6.7 (which must **not** re-declare them), so the CLI and picker stay in lockstep. A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.
```

**Ripple call-site corrections** (mechanical; part of this same fix — apply verbatim so no residual unexported-call compile break survives):

- **Task 3.6**, `cmd/spawn.go` failed-window branch:
  - Current: `` - `return fmt.Errorf("spawn: failed to open window(s) for %s — others left open", quoteJoin(failed))` `` — a plain, non-`UsageError`, non-silenced error → exit 1 on stderr. …
  - Proposed: `` - `return fmt.Errorf("spawn: failed to open window(s) for %s — others left open", spawn.QuoteJoin(failed))` `` — a plain, non-`UsageError`, non-silenced error → exit 1 on stderr. …

- **Task 6.6**, `internal/tui/model.go` failed-flash branch:
  - Current: `` - Else: `m.setFlash(fmt.Sprintf("⚠ %s failed to open — others left open", quoteJoin(failedNames)))` naming every failed window … ``
  - Proposed: `` - Else: `m.setFlash(fmt.Sprintf("⚠ %s failed to open — others left open", spawn.QuoteJoin(failedNames)))` naming every failed window … ``

- **Task 6.7**, `internal/tui/section_header.go` abort banner + its reuse note:
  - Current: `… + `fmt.Sprintf("%s %s gone — nothing opened", quoteJoin(msg.Gone), goneVerb(len(msg.Gone)))` … **Reuse the existing shared `internal/spawn` message helpers `quoteJoin` and `goneVerb` — both defined in Task 3.4 (`internal/spawn/message.go`); do NOT re-declare them here (a second `func goneVerb`/`quoteJoin` in `internal/spawn` is a duplicate-declaration compile error).** `goneVerb(n)` returns `"is"` for one …`
  - Proposed: `… + `fmt.Sprintf("%s %s gone — nothing opened", spawn.QuoteJoin(msg.Gone), spawn.GoneVerb(len(msg.Gone)))` … **Reuse the existing shared exported `internal/spawn` message helpers `spawn.QuoteJoin` and `spawn.GoneVerb` — both defined in Task 3.4 (`internal/spawn/message.go`); do NOT re-declare them (a second `func GoneVerb`/`QuoteJoin` in `internal/spawn` is a duplicate-declaration compile error).** `spawn.GoneVerb(n)` returns `"is"` for one …`

**Resolution**: Pending
**Notes**:

---

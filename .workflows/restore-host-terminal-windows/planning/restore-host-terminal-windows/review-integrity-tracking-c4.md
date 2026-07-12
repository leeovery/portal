---
status: complete
created: 2026-07-12
cycle: 4
phase: Plan Integrity Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Integrity

## Findings

### 1. `quoteJoin`/`goneVerb` shared helpers have no owning task — creation is unassigned for `quoteJoin` and contradictory for `goneVerb` (3-4 references-as-existing vs 6-7 "Add")

**Severity**: Important
**Plan Reference**: Tasks `restore-host-terminal-windows-3-4` (Pre-flight `has-session` gate) and `restore-host-terminal-windows-6-7` (Pre-flight abort UI)
**Category**: Task Self-Containment / Dependencies and Ordering
**Change Type**: update-task

**Details**:
The two shared `internal/spawn` message helpers — `quoteJoin(names []string) string` (renders `'s2'` / `'s2', 's4'`) and `goneVerb(n int) string` (returns `"is"` / `"are"`) — are new (neither exists in the codebase). They are consumed across four tasks in the natural execution order:

- `quoteJoin`: Task 3-4 (first use), 3-6, 6-6, 6-7.
- `goneVerb`: Task 3-4 (first use), 6-7.

The shared-location intent, the both-call-sites-use-it requirement, and the singular/plural (`is`/`are`) copy are all internally consistent. What is **not** coherent is which task *creates* the helpers:

1. **`quoteJoin` has no explicit creation instruction anywhere.** Every reference (3-4, 3-6, 6-6, 6-7) *uses* it and describes its behaviour, but no task's "Do" section instructs defining `func quoteJoin` in a specific file / owns it. An implementer of Task 3-4 (the first consumer) must infer they need to author it.

2. **`goneVerb` is created in the wrong task relative to its first use.** Task 3-4 (Phase 3, executes first) both *uses* `goneVerb` and describes it as "the shared `internal/spawn` helper co-located with `quoteJoin`" — i.e. as already existing. Task 6-7 (Phase 6, three phases later) explicitly instructs "**Add** the tiny count-aware verb helper `func goneVerb(n int) string { … }`." Because Task 3-4 must define `goneVerb` for its own error message to compile, an implementer who then follows Task 6-7's literal "Add" instruction declares `goneVerb` a second time in package `internal/spawn` → a hard duplicate-declaration compile error. Conversely, an implementer who treats 6-7 as the sole creation site leaves Task 3-4 (three phases earlier) referencing an undefined function.

The clean, coherent fix is to assign creation of **both** helpers to Task 3-4 (their first consumer, in a shared `internal/spawn/message.go`) with full signatures, and to change Task 6-7 from "Add … `func goneVerb`" to an explicit **reuse** of the Task-3-4 helpers (do not re-declare). This keeps the CLI (`cmd/spawn.go`) and the picker (`internal/tui`) producing byte-identical `'<session>' is/are gone — nothing opened` cores from one source of truth, with no ordering trap or duplicate declaration.

**Current**:

Task `restore-host-terminal-windows-3-4`, "Do" section, the pre-flight error-message sub-bullet:

```
    - `return fmt.Errorf("spawn: %s %s gone — nothing opened", quoteJoin(gone), goneVerb(len(gone)))` where `quoteJoin` renders `'s2'` for one and `'s2', 's4'` for several and `goneVerb` returns `"is"` for one / `"are"` for several — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. (`goneVerb` is the shared `internal/spawn` helper co-located with `quoteJoin`, so the CLI and picker stay in lockstep.) A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.
```

Task `restore-host-terminal-windows-6-7`, "Do" section, the abort-banner sub-bullet:

```
  - The abort banner: set a transient abort state (reuse the `flashText`/`flashKind` mechanism with a red/warning kind, or a dedicated `abortBannerText` field) rendered at the section-header row via `func renderPreflightAbortHeader(message string, width int, mode theme.Mode, colourless bool) string` in `section_header.go` — a red `⚠` (`theme.MV.StateRed` — the existing error token) + `fmt.Sprintf("%s %s gone — nothing opened", quoteJoin(msg.Gone), goneVerb(len(msg.Gone)))`, right-anchored dim `esc dismiss` (`theme.MV.TextDetail`), through `renderSectionHeaderRow`. Add the tiny count-aware verb helper `func goneVerb(n int) string { if n == 1 { return "is" }; return "are" }` (co-located with `quoteJoin` in the shared `internal/spawn` helper so the picker and CLI stay in lockstep), so a single gone session renders `⚠ 'fab-flowx-explore' is gone — nothing opened` — **byte-matching the delivered design copy `⚠ '<session>' is gone — nothing opened` and Task 6.7's own Outcome/Acceptance Criterion** — while several gone sessions render `⚠ 's2', 's4' are gone — nothing opened` (grammatical plural, preserving the plan's plural-safety). It sits above the multi-select banner in the section-header precedence (the transient flash/abort claimant, per the notice-band precedence).
```

**Proposed**:

Task `restore-host-terminal-windows-3-4`, "Do" section, replace the pre-flight error-message sub-bullet with:

```
    - **Define the two shared `internal/spawn` message helpers here** (this task is their first consumer; Task 3.6's leave-what-opened message and the Phase-6 picker in Tasks 6.6/6.7 reuse them verbatim, so they live in `internal/spawn` — a new `internal/spawn/message.go` — for CLI/picker lockstep):
      - `func quoteJoin(names []string) string` — single-quote each name and join with `, `: renders `'s2'` for one name and `'s2', 's4'` for several.
      - `func goneVerb(n int) string { if n == 1 { return "is" }; return "are" }` — the count-aware verb (`"is"` for one / `"are"` for several).
    - `return fmt.Errorf("spawn: %s %s gone — nothing opened", quoteJoin(gone), goneVerb(len(gone)))` — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. Both helpers are defined **once, here (this task)** and reused verbatim by Tasks 3.6/6.6/6.7 (which must **not** re-declare them), so the CLI and picker stay in lockstep. A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.
```

Task `restore-host-terminal-windows-6-7`, "Do" section, replace the abort-banner sub-bullet with:

```
  - The abort banner: set a transient abort state (reuse the `flashText`/`flashKind` mechanism with a red/warning kind, or a dedicated `abortBannerText` field) rendered at the section-header row via `func renderPreflightAbortHeader(message string, width int, mode theme.Mode, colourless bool) string` in `section_header.go` — a red `⚠` (`theme.MV.StateRed` — the existing error token) + `fmt.Sprintf("%s %s gone — nothing opened", quoteJoin(msg.Gone), goneVerb(len(msg.Gone)))`, right-anchored dim `esc dismiss` (`theme.MV.TextDetail`), through `renderSectionHeaderRow`. **Reuse the existing shared `internal/spawn` message helpers `quoteJoin` and `goneVerb` — both defined in Task 3.4 (`internal/spawn/message.go`); do NOT re-declare them here (a second `func goneVerb`/`quoteJoin` in `internal/spawn` is a duplicate-declaration compile error).** `goneVerb(n)` returns `"is"` for one / `"are"` for several, so a single gone session renders `⚠ 'fab-flowx-explore' is gone — nothing opened` — **byte-matching the delivered design copy `⚠ '<session>' is gone — nothing opened` and Task 6.7's own Outcome/Acceptance Criterion** — while several gone sessions render `⚠ 's2', 's4' are gone — nothing opened` (grammatical plural, preserving the plan's plural-safety). It sits above the multi-select banner in the section-header precedence (the transient flash/abort claimant, per the notice-band precedence).
```

**Resolution**: Fixed
**Notes**:

---

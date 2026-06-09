---
status: in-progress
created: 2026-06-09
cycle: 1
phase: Gap Analysis
topic: kill-rename-prefix-collision
---

# Review Tracking: kill-rename-prefix-collision - Gap Analysis

## Findings

### 1. Two inline `"="+session` sites in `saver_pane_pid.go` are unaddressed — in or out of scope?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Required Behaviour & The Fix > 1. Introduce the `exactTarget` session-level primitive; § Migration Scope & Out of Scope > Sites to migrate / Explicitly out of scope; § Testing Requirements & Acceptance Criteria > Acceptance criteria

**Details**:
The spec's stated intent is a *uniform end-state* — "no inline `"="+name` for a session name left anywhere in `tmux.go`" (§ 1) and "removes the exact drift surface that allowed the bug" / "as if it was never there" (§ Chosen approach). It names exactly three behaviour-neutral migration sites: `HasSession`, `HasSessionProbe`, `SwitchClient`.

However, the `internal/tmux` package contains **two additional inline `"="+sessionName` session-target strings** that the spec never mentions:
- `internal/tmux/saver_pane_pid.go:49` — `saverPanePID`: `list-panes -t "="+sessionName -F #{pane_pid}`
- `internal/tmux/saver_pane_pid.go:84` — `SaverPaneID`: `list-panes -t "="+sessionName -F #{pane_id}`

These are genuine **session targets** carrying the inline prefix — the exact drift surface the fix is meant to eliminate, just in a sibling file rather than `tmux.go`. They are not in the three-site migration list, and they are not in the "Explicitly out of scope" list either. (The out-of-scope list does name `ListPanesInSession` and "the other `list-panes -t session` reads" — but characterises them as **bare** / "left bare", which is factually true of `ListPanesInSession` but **not** of these two saver sites, which already carry `=`.)

This forces the implementer to make a scope decision the spec should make for them:
- **Option A** — migrate these two onto `exactTarget` too (consistent with the "uniform, as if it was never there" intent; behaviour-neutral, identical argv).
- **Option B** — deliberately leave them, and add them to the out-of-scope list with a one-line rationale (e.g. fixed internal `_portal-saver` name, no collision exposure — mirroring the quickstart rationale).

Either is defensible, but the spec currently leaves it undefined, so two implementers could reach different end-states and the "no inline strings remain" acceptance check would be interpreted differently.

**Proposed Addition**:
{leave blank until discussed — resolution is to pick Option A or B and state it explicitly in § Migration Scope, then reconcile the acceptance criterion (see finding 2)}

**Resolution**: Pending
**Notes**:

---

### 2. Acceptance criterion scopes the "no inline strings" guarantee to `tmux.go` only — verify this is deliberate

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Testing Requirements & Acceptance Criteria > Acceptance criteria (3rd bullet)

**Details**:
The acceptance criterion reads: "`exactTarget` exists in `internal/tmux` as the canonical session-level exact-match target builder; **no inline `"="+name` session-target strings remain in `tmux.go`**." The verifiable guarantee is bounded to the single file `tmux.go`, whereas the surrounding prose frames the goal at the package/codebase level ("as if it was never there", "the two canonical ways to build an exact-match `-t` target").

This file-scoped wording is what lets the unaddressed `saver_pane_pid.go` sites (finding 1) slip through a literal pass: an implementer who satisfies this criterion exactly would still leave inline session-target strings in the package and could legitimately mark the work done. If that is the intent (saver sites deliberately excluded), the criterion is correct but the § 1 prose ("anywhere in `tmux.go`" vs the broader narrative) and the out-of-scope list should be reconciled so the boundary is unambiguous. If the intent is the full uniform package end-state, the criterion should say `internal/tmux` (the package) rather than `tmux.go` (the file).

This is the testability/planning-readiness half of finding 1: an acceptance criterion an implementer can satisfy while leaving the stated drift surface partially open is an ambiguous gate.

**Current**:
- `exactTarget` exists in `internal/tmux` as the canonical session-level exact-match target builder; no inline `"="+name` session-target strings remain in `tmux.go`.

**Proposed Addition**:
{leave blank until discussed — wording will follow the Option A/B decision from finding 1}

**Resolution**: Pending
**Notes**:

---

### 3. Cited test line numbers point at assertion lines, not the function declarations (minor)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Problem & Root Cause > Why it wasn't caught

**Details**:
The spec cites "`TestKillSession` (`tmux_test.go:737`)" and "`TestRenameSession` (`tmux_test.go:953`)". In the current tree the `func TestKillSession` declaration is at line 723 (the cited 737 is the `wantArgs := "kill-session -t my-session"` assertion line) and `func TestRenameSession` is at line 939 (953 is its `wantArgs` assertion line). The referenced functions and the exact asserted argv strings are correct and easily findable, so this does not block implementation — but the line numbers attached to the function names are slightly off and will drift further as the file changes. Optional: cite by function name only, or note that the line is the assertion line. Flagging only because line-precise references invite over-trust.

**Proposed Addition**:
{leave blank until discussed — likely just drop or relabel the line numbers}

**Resolution**: Pending
**Notes**:

---

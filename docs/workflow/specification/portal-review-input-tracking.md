---
status: in-progress
created: 2026-02-13
phase: Input Review
topic: Portal
---

# Review Tracking: Portal - Input Review

## Findings

### 1. Shell completions not documented in `portal init` output

**Source**: git-root-and-completions discussion (Q4), x-xctl-split discussion (summary table: "Shell integration: `portal init <shell>` (completions + functions)")
**Category**: Enhancement to existing topic
**Affects**: CLI Interface → Shell Functions, portal — Direct Commands

**Details**:
The git-root-and-completions discussion concluded that shell completions (`bash`, `zsh`, `fish`) should be generated via Cobra's built-in methods. The x-xctl-split discussion then folded this into `portal init <shell>` — the init script emits both shell functions AND tab completions.

The spec currently shows `portal init` emitting only the two shell functions (`x` and `xctl`). An implementer would miss generating Cobra completions as part of the init output.

**Proposed Addition**: (pending discussion)

**Resolution**: Pending
**Notes**:

---

### 2. `portal open` command description doesn't reflect exec flags

**Source**: session-launch-command discussion (CLI syntax), x-xctl-split discussion (routing table: `x` routes to `portal open`)
**Category**: Enhancement to existing topic
**Affects**: CLI Interface → portal — Direct Commands

**Details**:
The portal commands table describes `portal open [query]` without mentioning `-e`/`--exec` flags. Since `x` passes through all args to `portal open`, the exec flags are implicitly supported but not documented on `portal open` itself.

Minor — the `x` section covers the flags comprehensively and the `portal open` description says "used via `x` shell function". But an implementer working on the `portal open` command's flag registration might miss the exec flags if they only look at the portal commands table.

**Proposed Addition**: (pending discussion)

**Resolution**: Pending
**Notes**:

---
checksum: 8af28045b8037682adc299fcaef7498e
generated: 2026-01-22T19:26:11Z
discussion_files:
  - cx-design.md
  - zellij-multi-directory.md
---

# Discussion Consolidation Analysis

## Recommended Groupings

### zw (Zellij Workspaces)
- **cx-design**: Original comprehensive design for the tool (then named "CX") covering TUI, session management, project naming, configuration, Zellij integration, and distribution. Contains foundational decisions that still apply.
- **zellij-multi-directory**: Pivotal refinement that shifts from project-centric to workspace-centric model. Renames tool to ZW, removes session→project mapping, introduces user-chosen session names, and adds inside-Zellij utility mode.

**Coupling**: These discuss the same tool at different points in its design evolution. The second discussion explicitly references the first, identifies what "carries forward" and what "changes". They are inseparable - the specification must reconcile both to capture the final design.

## Independent Discussions

(none)

## Analysis Notes

The zellij-multi-directory discussion provides a clear reconciliation guide:

**Carries forward from cx-design:**
- Go + Bubbletea TUI
- Flat config format
- Homebrew distribution via `leeovery/tools` tap
- File browser for new project discovery
- Keyboard shortcuts (N, K, Enter, etc.)
- `cx clean` subcommand (now `zw clean`)

**Superseded by zellij-multi-directory:**
- sessions.json mapping file → removed
- Automatic Claude execution → removed
- Session naming `{project}-{NN}` → user-chosen free-form
- TUI organized by projects → organized by sessions
- "cd before attach" → not needed (Zellij restores)
- Name "CX" → renamed to "ZW"

**User context**: The user confirmed that zellij-multi-directory overrides/changes behavior from the original, but valuable information from cx-design still needs inclusion. The specification skill should help reconcile what's current vs. superseded.

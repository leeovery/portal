---
checksum: ba0e5851f4a5873466fd3c810fd7766b
generated: 2026-02-12T12:10:00Z
discussion_files:
  - cx-design.md
  - zellij-multi-directory.md
  - fzf-output-mode.md
  - git-root-and-completions.md
  - zellij-to-tmux-migration.md
  - x-xctl-split.md
---

# Discussion Consolidation Analysis

## Recommended Groupings

### Portal
- **cx-design**: Original tool design — TUI, session management, project memory, distribution
- **zellij-multi-directory**: Model pivot from project-centric to workspace-centric; renamed CX → ZW
- **fzf-output-mode**: Added `list` and `attach` subcommands for scripting/fzf piping
- **git-root-and-completions**: Git root resolution for directory inputs; shell completions subcommand
- **zellij-to-tmux-migration**: Migrated multiplexer from Zellij to tmux; renamed ZW → mux; simplified (dropped layouts, exited sessions, restricted utility mode)
- **x-xctl-split**: Renamed mux → Portal; single binary + shell integration (zoxide pattern); split CLI into `x` (launcher) and `xctl` (control plane); added zoxide resolution and TTY-aware output

**Coupling**: All six discussions describe the same tool's evolution. Each builds directly on the previous. Data structures, TUI design, CLI surface, session model, and distribution are shared across all.

## Independent Discussions

(none)

## Analysis Notes

All discussions consolidated into the Portal specification.

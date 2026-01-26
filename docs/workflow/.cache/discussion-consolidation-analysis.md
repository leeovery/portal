---
checksum: b5db48fcbf683830fe06065ce9e1b618
generated: 2026-01-26T16:05:00Z
discussion_files:
  - cx-design.md
  - fzf-output-mode.md
  - git-root-and-completions.md
  - zellij-multi-directory.md
---

# Discussion Consolidation Analysis

## Recommended Groupings

### zw
- **cx-design**: Original CX design - TUI, session management, data storage, Zellij integration, configuration, CLI, distribution. Foundation for the tool.
- **zellij-multi-directory**: Pivots from project-centric to workspace-centric model. Renames CX → ZW. Supersedes key assumptions from cx-design but retains TUI, distribution, file browser design.
- **fzf-output-mode**: Adds `zw list` and `zw attach <name>` commands for fzf/scripting integration. Feature addition to existing ZW spec.
- **git-root-and-completions**: Adds git root auto-resolution for directory selection and `zw completion` subcommand. Two minor amendments to the existing ZW spec.

**Coupling**: All four discussions are about the same tool (ZW, formerly CX). cx-design and zellij-multi-directory are tightly coupled - the latter explicitly pivots and supersedes parts of the former. fzf-output-mode and git-root-and-completions are feature additions that reference the ZW specification directly and propose CLI/behavior amendments.

## Independent Discussions

(none)

## Analysis Notes

An existing "zw" specification (concluded) already exists. All four discussions represent the source material and subsequent refinements:

1. **cx-design** - foundational design decisions
2. **zellij-multi-directory** - model pivot (session = workspace, not project)
3. **fzf-output-mode** - CLI additions for scripting
4. **git-root-and-completions** - git root detection + shell completions

The zw specification was previously built from discussions 1-3. Discussion 4 is new and needs to be incorporated. The specification should be updated to include git root resolution in directory handling and `zw completion` in the CLI commands table.

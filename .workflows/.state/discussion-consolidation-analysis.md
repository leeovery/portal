---
checksum: 4a5f2e9c9721227fd9eff82db2e7014d
generated: 2026-02-13T10:15:00Z
discussion_files:
  - cx-design.md
  - zellij-multi-directory.md
  - fzf-output-mode.md
  - git-root-and-completions.md
  - zellij-to-tmux-migration.md
  - x-xctl-split.md
  - session-launch-command.md
---

# Discussion Consolidation Analysis

## Recommended Groupings

### Portal
- **cx-design**: Original tool design — TUI architecture, keyboard shortcuts, project management, session naming, data storage, distribution. Foundation that all subsequent discussions build on.
- **zellij-multi-directory**: Model pivot from project-centric to workspace-centric. Renamed tool to ZW. Redefined TUI around sessions instead of projects.
- **fzf-output-mode**: Added `list` and `attach` subcommands for scripting/fzf pipelines. Extends the CLI surface.
- **git-root-and-completions**: Git root auto-resolution for directory selection and `completion` subcommand. Additive features to the core tool.
- **zellij-to-tmux-migration**: Platform switch from Zellij to tmux. Renamed to `mux`. Dropped layouts, exited sessions, restricted utility mode. Simplified the design.
- **x-xctl-split**: CLI restructure — single binary `portal` with shell integration (`x`/`xctl`). Added zoxide resolution, alias management, TTY-aware output. Final naming and architecture.
- **session-launch-command**: Generalised exec mechanism (`-e`/`--`) for running commands in new sessions. Makes tool command-agnostic (no hardcoded claude references).

**Coupling**: All seven discussions define a single evolving tool — Portal. Each discussion either pivots the model, switches the platform, restructures the CLI, or adds features. They share the same data model (projects, sessions, aliases), TUI architecture, session lifecycle, and distribution pipeline. Inseparable for specification purposes.

## Independent Discussions

(none)

## Analysis Notes

All seven concluded discussions trace the evolution of one tool through multiple design pivots: CX → ZW → mux → Portal. The existing `portal` specification (concluded) already incorporates six of these discussions. The seventh (`session-launch-command`) adds the exec mechanism and was concluded after the portal spec.

No naming conflicts — the anchored name `portal` is the natural grouping for all discussions.

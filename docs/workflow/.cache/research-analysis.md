---
checksum: 8b8fdafef66613f3efb476658ca13608
generated: 2026-01-26T12:42:00
research_files:
  - cc-tool-plan.md
  - tmux-session-managers-analysis.md
---

# Research Analysis Cache

## Topics

### 1. Session Types & Zellij Integration
- **Source**: cc-tool-plan.md (lines 28-48)
- **Summary**: Defines two session types - Zellij (persistent, crash-resistant) and Raw (ephemeral). Sessions tracked via Zellij queries.
- **Key questions**: Is the Zellij-first approach correct? Should raw mode be equally supported?

### 2. Project Registry Design
- **Source**: cc-tool-plan.md (lines 32-48, 120-138)
- **Summary**: Projects stored in projects.json with path, name, alias, timestamps, usage count.
- **Key questions**: Is alias needed? Should use_count drive sorting/display?

### 3. CLI Command Structure
- **Source**: cc-tool-plan.md (lines 54-86)
- **Summary**: Unified `cx` command with subcommands (list, kill, clean, projects, config). Modifiers like --raw, -c, -r.
- **Key questions**: Is the command hierarchy right? Any missing commands?

### 4. Unified Interactive View
- **Source**: cc-tool-plan.md (lines 168-216)
- **Summary**: Single TUI showing projects + sessions. Projects with active sessions show indicators. Sub-picker for multiple sessions.
- **Key questions**: Is the sub-picker UX right? What about the [.] and [n] shortcuts?

### 5. Session → Project Mapping
- **Source**: cc-tool-plan.md (lines 140-160)
- **Summary**: Three options proposed: encode in name, sessions.json mapping, query Zellij CWD. Recommends sessions.json.
- **Key questions**: Confirm sessions.json approach? How to handle orphaned mappings?

### 6. Session Naming Convention
- **Source**: cc-tool-plan.md (lines 248-260)
- **Summary**: Format `{project}-{NN}`, numbers increment even after sessions deleted to avoid reuse confusion.
- **Key questions**: Is this format right? What about very long project names?

### 7. Configuration Design
- **Source**: cc-tool-plan.md (lines 90-116)
- **Summary**: YAML config at ~/.config/cx/config.yaml with defaults, session naming, UI prefs, Claude args.
- **Key questions**: Are these the right config options? Any missing?

### 8. Open Question: Zellij Layout Location
- **Source**: cc-tool-plan.md (lines 435-441)
- **Summary**: Where should custom Claude layout live? Options: bundled, user-maintained, setup command.
- **Key questions**: Confirm user-maintains-own approach?

### 9. Open Question: Auto-registration
- **Source**: cc-tool-plan.md (lines 452-458)
- **Summary**: Should projects auto-register when sessions start?
- **Key questions**: Confirm yes with manual forget?

### 10. Open Question: Missing Zellij
- **Source**: cc-tool-plan.md (lines 459-466)
- **Summary**: What if Zellij not installed? Options: error, warn+fallback, configurable.
- **Key questions**: Confirm warn+fallback approach?

### 11. Open Question: Continue/Resume Flags
- **Source**: cc-tool-plan.md (lines 468-475)
- **Summary**: Should -c and -r work with existing sessions or only new Claude instances?
- **Key questions**: Confirm new-process-only behavior?

### 12. Technical Architecture
- **Source**: cc-tool-plan.md (lines 272-356)
- **Summary**: Go project structure with cmd/, internal/ layout. Cobra CLI, Bubbletea TUI, Lipgloss styling.
- **Key questions**: Is dependency list complete? Any concerns with architecture?

### 13. Distribution Strategy
- **Source**: cc-tool-plan.md (lines 387-428)
- **Summary**: Homebrew tap at leeovery/tools, formula with Zellij dependency, manual release process.
- **Key questions**: Should Zellij be a hard homebrew dependency? GoReleaser for automation?

### 14. Implementation Phasing
- **Source**: cc-tool-plan.md (lines 477-504)
- **Summary**: Four phases - Core MVP, Session Management, Polish, Distribution.
- **Key questions**: Is the phasing right? Anything moved between phases?

### 15. Git root detection for project registration
- **Source**: tmux-session-managers-analysis.md (lines 134, 164)
- **Summary**: sesh's `root` command detects git repo root for a directory, ensuring sessions start at project root regardless of where the user is in the tree. Low effort via `git rev-parse --show-toplevel`.
- **Key questions**: Should ZW always resolve to git root, or offer it as a suggestion? Where in the flow does this apply (file browser, `zw .`, `zw <path>`)?

### 16. Shell completions via Cobra
- **Source**: tmux-session-managers-analysis.md (lines 118, 136, 160)
- **Summary**: sesh provides built-in shell completion generation for bash/zsh/fish/powershell. Cobra provides this largely for free. Standard CLI hygiene.
- **Key questions**: Just needs a `zw completion <shell>` subcommand wired up. Minimal design decisions required.

### 17. Startup commands per project
- **Source**: tmux-session-managers-analysis.md (lines 106-113, 132, 162)
- **Summary**: sesh allows defining a `startup_command` per session that runs on creation (e.g., open nvim). Could be added to ZW's projects.json.
- **Key questions**: Does this overlap with Zellij layouts (which can define commands per pane)? Is projects.json the right place?

### 18. Quick session toggle (`last` command)
- **Source**: tmux-session-managers-analysis.md (lines 115, 133, 161)
- **Summary**: sesh's `last` command switches to the second-most recently used session. Common workflow pattern.
- **Key questions**: Does Zellij track session access order? Would ZW need its own recency tracking?

### 19. zoxide integration for directory discovery
- **Source**: tmux-session-managers-analysis.md (lines 35, 50-51, 163)
- **Summary**: Both tmux-session-wizard and sesh use zoxide as a directory source. Previously reviewed and decided not to proceed.
- **Key questions**: Already decided against — documented for completeness only.

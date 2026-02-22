---
topic: portal
status: planning
format: tick
work_type: greenfield
ext_id:
specification: ../../specification/portal/specification.md
spec_commit: 5766a886dfaefb8ee771bc97fb31a55f9af5665a
created: 2026-02-22
updated: 2026-02-22
external_dependencies: []
task_list_gate_mode: gated
author_gate_mode: gated
finding_gate_mode: gated
planning:
  phase: 1
  task: ~
---

# Plan: Portal

### Phase 1: Walking Skeleton -- Session List and Attach
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Establish the Go project with Cobra CLI, tmux integration layer, and Bubble Tea TUI that lists live tmux sessions and attaches to a selected one. One complete end-to-end flow proving the architecture.

**Why this order**: Walking skeleton. Threads through every architectural layer -- CLI parsing (Cobra), tmux command execution, TUI rendering (Bubble Tea), and process handoff (exec). Proves the integration works before building further features.

**Acceptance**:
- [ ] Go module initialised with Cobra, Bubble Tea dependencies
- [ ] `portal open` launches a full-screen TUI listing running tmux sessions (name, window count, attached indicator)
- [ ] Selecting a session with Enter attaches via `exec tmux attach-session -t <name>`
- [ ] Arrow keys and j/k navigate the session list
- [ ] q and Esc quit the TUI cleanly
- [ ] Empty state ("No active sessions") displays when no tmux server or no sessions exist
- [ ] tmux list-sessions parsing handles no-server case gracefully (non-zero exit treated as zero sessions)

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-1-1 | Go Module and Cobra Root Command | none | pending | |
| portal-1-2 | tmux Session Listing and Parsing | no tmux server running, empty session list | pending | |
| portal-1-3 | Session List TUI Model | single session in list, long session names | pending | |
| portal-1-4 | Keyboard Navigation | single-item list navigation | pending | |
| portal-1-5 | Quit Handling | none | pending | |
| portal-1-6 | Attach on Enter | none | pending | |
| portal-1-7 | Empty State Display | tmux server disappears between list and display | pending | |

### Phase 2: New Session from Directory -- Project Memory and Session Creation
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Add the ability to create new tmux sessions from a directory, with git root resolution, auto-generated session naming (nanoid suffix), project memory persistence (projects.json), and the project picker TUI view.

**Why this order**: Session creation is the second core flow after attach. It introduces project storage, git root resolution, and session naming -- foundational primitives that all subsequent features depend on.

**Acceptance**:
- [ ] Selecting "[n] new in project..." from main TUI opens the project picker
- [ ] Project picker lists remembered projects sorted by last_used, with "browse for directory..." always at bottom
- [ ] Selecting a project creates a new tmux session with auto-generated name ({project}-{nanoid}) and attaches
- [ ] Session name sanitisation replaces `.` and `:` with `-`; collision with existing tmux session regenerates nanoid
- [ ] Git root resolution applied: `git -C <dir> rev-parse --show-toplevel`; non-git directories used as-is
- [ ] projects.json created/updated at ~/.config/portal/ with path, name, last_used fields
- [ ] Stale project cleanup runs automatically when project picker is displayed
- [ ] Empty state ("No saved projects yet.") displays when no projects remembered, with browse option still visible

### Phase 3: File Browser and CLI Quick-Start
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Add the interactive directory browser for navigating to and selecting new project directories, and implement the non-interactive CLI quick-start paths (`portal open .`, `portal open <path>`) that create sessions without the TUI.

**Why this order**: The file browser completes the "new session" story started in Phase 2 (browse for directory... option). CLI quick-start paths reuse session creation logic from Phase 2 but add the direct entry points that skip the TUI.

**Acceptance**:
- [ ] File browser shows directories only, starting from current working directory
- [ ] Enter or right-arrow descends into highlighted directory; Backspace or left-arrow goes to parent
- [ ] Space or Enter on "." entry selects current directory for session creation
- [ ] Typing narrows directory listing by fuzzy match at current level; Backspace removes filter char then reverts to navigation when empty
- [ ] Esc clears filter if active, otherwise cancels browser
- [ ] Hidden directories (starting with `.`) hidden by default; `.` key toggles visibility
- [ ] Selected directory is automatically added to projects.json
- [ ] `portal open .` creates session in cwd (git root resolved) without launching TUI
- [ ] `portal open <path>` creates session at resolved path without launching TUI
- [ ] Non-existent directory prints "Directory not found: {path}" and exits with code 1

### Phase 4: Query Resolution, Aliases, and Shell Integration
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Implement the alias system (storage, CRUD commands), zoxide integration, the full query resolution chain (path detection -> alias -> zoxide -> TUI fallback), and the `portal init` command that emits shell functions and tab completions.

**Why this order**: Query resolution and aliases build on the session creation and project memory infrastructure from Phases 2-3. Shell integration (`portal init`) is the user-facing entry point that ties the CLI commands into shell functions (`x`, `xctl`). These are needed before the remaining management features.

**Acceptance**:
- [ ] `portal init zsh` emits `x()` and `xctl()` shell functions plus tab completions
- [ ] `portal init zsh --cmd p` emits `p()` and `pctl()` with completions wired to custom names
- [ ] `portal init bash` and `portal init fish` emit correct shell-specific output
- [ ] `portal alias set <name> <path>` stores alias with path normalisation (~ expansion, relative to absolute)
- [ ] `portal alias rm <name>` removes alias; `portal alias list` lists all aliases
- [ ] Alias uniqueness enforced -- setting existing alias overwrites
- [ ] Alias storage at ~/.config/portal/aliases in flat key=value format
- [ ] `portal open <query>` resolves via: path detection (contains `/`, starts with `.` or `~`) -> alias match -> zoxide query -> TUI fallback with query as pre-filled filter
- [ ] Zoxide is optional soft dependency -- skipped silently if not installed
- [ ] File browser `a` shortcut prompts for alias name and saves to aliases file (directory git-root-resolved, no session started)

### Phase 5: Inside-tmux Mode, Session Management, and Filter Mode
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Implement inside-tmux detection with switch-client behaviour, TUI session management actions (kill with confirmation, rename with inline input), filter mode for fuzzy-searching the session list, and the xctl management commands (`list`, `attach`, `kill`, `clean`).

**Why this order**: These features layer on top of the working TUI and CLI from prior phases. Inside-tmux mode changes underlying tmux commands but reuses all existing TUI and session logic. Management commands and filter mode are enhancements that don't alter the core flow.

**Acceptance**:
- [ ] `TMUX` env var detected: session selection uses `tmux switch-client -t <name>` instead of `exec tmux attach-session`
- [ ] Inside tmux: current session excluded from session list; header shows current session name
- [ ] Inside tmux: new session created detached (`-d`) then `switch-client` applied
- [ ] Inside tmux: CLI commands (`portal open .`, `portal open <path>`, `portal attach`) use switch-client
- [ ] `K` on selected session prompts "Kill session '<name>'? (y/n)"; confirmed kill removes session and refreshes list
- [ ] `R` on selected session shows inline text input pre-filled with current name; Enter confirms rename via `tmux rename-session`, Esc cancels
- [ ] `/` activates filter mode with input at bottom; fuzzy matches session names; `[n] new in project...` always visible
- [ ] Filter mode: Backspace on empty filter exits mode; Esc clears filter and exits mode
- [ ] `portal list` outputs full details on TTY, names-only when piped; `--short` and `--long` override
- [ ] `portal list` with no sessions outputs nothing and exits 0
- [ ] `portal attach <name>` attaches (or switch-client inside tmux); "No session found: {name}" with exit 1 if not found
- [ ] `portal kill <name>` kills session; "No session found: {name}" with exit 1 if not found
- [ ] `portal clean` removes stale projects non-interactively, printing each removal

### Phase 6: Command Execution, Project Editing, and Distribution
status: approved
ext_id:
approved_at: 2026-02-22

**Goal**: Implement `-e`/`--` command execution for new sessions, project picker edit mode (rename and alias management), and the complete build/release pipeline (GoReleaser, GitHub Actions, Homebrew tap formula).

**Why this order**: Command execution and project editing are the final feature capabilities defined in the spec. Distribution packaging is last because it wraps the completed binary. All user-facing features must be complete before the release pipeline is meaningful.

**Acceptance**:
- [ ] `portal open -e <cmd> [destination]` resolves destination, creates session with command via `$SHELL -ic '<cmd>; exec $SHELL'` tmux shell-command
- [ ] `portal open [destination] -- <cmd> [args...]` equivalent behaviour using double-dash syntax
- [ ] `-e` and `--` are mutually exclusive; providing both produces error message and exit code 2
- [ ] When command pending: TUI shows project picker only (no session list), banner displays "Command: <cmd>"
- [ ] Command execution works for both outside-tmux (exec) and inside-tmux (detached + switch-client) flows
- [ ] Project picker `e` key opens inline edit: rename project name and manage aliases; Tab cycles fields, Enter confirms, Esc cancels
- [ ] Project picker `x` key removes selected project with confirmation prompt
- [ ] `portal version` outputs version information
- [ ] GoReleaser configuration for macOS (arm64, amd64) and Linux (arm64, amd64) builds
- [ ] GitHub Actions workflow triggers GoReleaser on version tag push
- [ ] Homebrew formula in `leeovery/homebrew-tools` auto-updated by GoReleaser
- [ ] tmux runtime dependency check at startup: missing tmux displays "Portal requires tmux. Install with: brew install tmux" and exits

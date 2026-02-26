---
topic: portal
status: concluded
format: tick
work_type: greenfield
ext_id: tick-77cbba
specification: ../../specification/portal/specification.md
spec_commit: 5766a886dfaefb8ee771bc97fb31a55f9af5665a
created: 2026-02-22
updated: 2026-02-22
external_dependencies: []
task_list_gate_mode: auto
author_gate_mode: auto
finding_gate_mode: auto
review_cycle: 3
planning:
  phase: ~
  task: ~
---

# Plan: Portal

### Phase 1: Walking Skeleton -- Session List and Attach
status: approved
ext_id: tick-57d5a4
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
| portal-1-1 | Go Module and Cobra Root Command | none | authored | tick-fc2921 |
| portal-1-2 | tmux Session Listing and Parsing | no tmux server running, empty session list | authored | tick-442b28 |
| portal-1-3 | Session List TUI Model | single session in list, long session names | authored | tick-afcc27 |
| portal-1-4 | Keyboard Navigation | single-item list navigation | authored | tick-27a6f2 |
| portal-1-5 | Quit Handling | none | authored | tick-0f5439 |
| portal-1-6 | Attach on Enter | none | authored | tick-9776d9 |
| portal-1-7 | Empty State Display | tmux server disappears between list and display | authored | tick-e70af4 |

### Phase 2: New Session from Directory -- Project Memory and Session Creation
status: approved
ext_id: tick-a8f83d
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
- [ ] `/` activates filter mode in project picker; fuzzy-matches against project names; browse option always visible

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-2-1 | Git Root Resolution | non-git directory, git not installed, symlinked paths | authored | tick-2bb2d0 |
| portal-2-2 | Session Name Generation | project name with dots and colons, collision with existing tmux session, empty project name | authored | tick-4ebf36 |
| portal-2-3 | Project Store (projects.json) | file does not exist yet, malformed JSON, config directory does not exist | authored | tick-248cf4 |
| portal-2-4 | Stale Project Cleanup | all projects stale, permission denied on stat, empty project list | authored | tick-275824 |
| portal-2-5 | Session Creation from Project | tmux server not running (first session), directory removed between selection and creation | authored | tick-f924e3 |
| portal-2-6 | Project Picker TUI View | no saved projects (empty state), single project, long project names | authored | tick-4c54e1 |
| portal-2-7 | Main TUI New in Project Integration | no sessions and no projects (both empty states), returning from project picker to session list | authored | tick-444a76 |

### Phase 3: File Browser and CLI Quick-Start
status: approved
ext_id: tick-f6e9a1
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

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-3-1 | Directory Listing Model | empty directory, permission denied on readdir, symlinked directories | authored | tick-f26ed6 |
| portal-3-2 | File Browser TUI -- Core Navigation | root directory (no parent), single subdirectory, deeply nested path | authored | tick-62c4f1 |
| portal-3-3 | File Browser Inline Filter | filter matches nothing, filter then navigate to child resets filter, all characters deleted | authored | tick-cebbcb |
| portal-3-4 | File Browser Hidden Directory Toggle and Selection | directory has only hidden subdirectories, selected directory removed between browse and select | authored | tick-feea91 |
| portal-3-5 | File Browser Integration with Project Picker | returning from browser without selection, browse from empty project list | authored | tick-9d6404 |
| portal-3-6 | CLI Quick-Start Path Resolution | non-existent directory, path is a file not a directory, relative path resolution, tilde expansion | authored | tick-33fb7f |
| portal-3-7 | CLI Quick-Start Session Creation | cwd is inside git repo subdirectory, project already in projects.json | authored | tick-afb187 |

### Phase 4: Query Resolution, Aliases, and Shell Integration
status: approved
ext_id: tick-0c342f
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

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-4-1 | Alias Store | file does not exist yet, empty file, duplicate keys in file, config directory does not exist | authored | tick-5533b4 |
| portal-4-2 | Alias Set Command with Path Normalisation | tilde expansion, relative path, overwrite existing alias | authored | tick-a2e7bd |
| portal-4-3 | Alias Remove and List Commands | remove non-existent alias, list with no aliases | authored | tick-26cdeb |
| portal-4-4 | Zoxide Query Integration | zoxide not installed, zoxide returns no match | authored | tick-b3b992 |
| portal-4-5 | Query Resolution Chain | query matches both alias and zoxide, resolved directory does not exist, path-like query | authored | tick-fe7c90 |
| portal-4-6 | Shell Init for Zsh | none | authored | tick-83fe89 |
| portal-4-7 | Shell Init Custom Command Names | cmd name conflicts with shell builtins | authored | tick-457428 |
| portal-4-8 | Shell Init for Bash and Fish | unsupported shell name passed to init | authored | tick-19ad25 |
| portal-4-9 | File Browser Alias Shortcut | empty alias name entered, alias name already exists | authored | tick-b92526 |

### Phase 5: Inside-tmux Mode, Session Management, and Filter Mode
status: approved
ext_id: tick-d5a1e0
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

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-5-1 | Inside-tmux Detection and Switch-Client | TMUX env var set but empty, TMUX var points to dead session | authored | tick-e1b2c3 |
| portal-5-2 | Inside-tmux Session List Exclusion and Header | only one session running (current), current session name very long | authored | tick-f4d5e6 |
| portal-5-3 | Inside-tmux New Session Creation | session name collision during detached creation | authored | tick-a7b8c9 |
| portal-5-4 | Inside-tmux CLI Command Routing | TMUX set but session killed before switch | authored | tick-d1e2f3 |
| portal-5-5 | Kill Session with Confirmation | killing last session, killing current session (inside tmux) | authored | tick-b4c5d6 |
| portal-5-6 | Rename Session with Inline Input | invalid characters in name, collision with existing session | authored | tick-e7f8a9 |
| portal-5-7 | Filter Mode Activation and Fuzzy Matching | no sessions match filter, single character filter | authored | tick-c1d2e3 |
| portal-5-8 | Filter Mode Exit Behaviour | rapid Backspace presses, filter active with no matches | authored | tick-f4a5b6 |
| portal-5-9 | List Command with TTY-Aware Output | no sessions running, piped to another command | authored | tick-d7e8f9 |
| portal-5-10 | Attach Command | partial name match, ambiguous name, session killed between lookup and attach | authored | tick-a1b2c4 |
| portal-5-11 | Kill Command | killing current session inside tmux, session name not found | authored | tick-e3f4d5 |
| portal-5-12 | Clean Command | no stale projects, all projects stale, permission errors | authored | tick-b6c7a8 |

### Phase 6: Command Execution, Project Editing, and Distribution
status: approved
ext_id: tick-a6b7c9
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

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-6-1 | Command Flag Parsing (-e and --) | -e with empty string, -- with no arguments, both -e and -- provided | authored | tick-d9e0f1 |
| portal-6-2 | Command-Aware Session Creation | command containing single quotes, $SHELL not set, special shell characters | authored | tick-c2d3e4 |
| portal-6-3 | Command-Pending TUI Mode | no saved projects, query pre-filled as filter, long command in banner | authored | tick-f5a6b7 |
| portal-6-4 | Project Picker Edit Mode | no aliases, multiple aliases, alias collision, empty project name | authored | tick-e8f9a0 |
| portal-6-5 | Project Picker Remove with Confirmation | removing last project, rapid key presses, project has aliases | authored | tick-b1c2d3 |
| portal-6-6 | tmux Runtime Dependency Check | tmux not executable, broken symlink | authored | tick-a4b5c6 |
| portal-6-7 | Version Command | version not set at build time | authored | tick-d7e8f0 |
| portal-6-8 | GoReleaser Configuration | none | authored | tick-f1a2b3 |
| portal-6-9 | GitHub Actions Release Workflow | tag without v prefix, workflow permissions | authored | tick-c4d5e6 |

### Phase 7: Analysis (Cycle 1)
status: approved
ext_id: tick-98bd31

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-7-1 | Wire all dependencies into openTUI and adopt functional options for tui.Model | none | authored | tick-feb4e0 |
| portal-7-2 | Extract shared session-creation pipeline from SessionCreator and QuickStart | none | authored | tick-7943e4 |
| portal-7-3 | Fix GoReleaser config to use brews instead of homebrew_casks | none | authored | tick-4aedcc |
| portal-7-4 | Extract fuzzyMatch into shared internal package | none | authored | tick-ae743d |
| portal-7-5 | Extract config file path helper to eliminate duplication | none | authored | tick-218aad |
| portal-7-6 | Pass parsedCommand as parameter instead of package-level variable | none | authored | tick-4a035c |

### Phase 8: Analysis (Cycle 2)
status: approved
ext_id: tick-3a2937

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-8-1 | Extract generic FuzzyFilter function | none | authored | tick-312e8c |
| portal-8-2 | Deduplicate ProjectStore interface | none | authored | tick-7f0e35 |
| portal-8-3 | Remove redundant quickStartResult mirror type | none | authored | tick-200812 |

### Phase 9: Analysis (Cycle 3)
status: approved
ext_id: tick-0ed051

**Goal**: Address findings from Analysis (Cycle 3).

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-9-1 | Apply initial filter to session list on query resolution fallback | none | authored | tick-1bd4ee |

### Phase 10: Analysis (Cycle 4)
status: approved
ext_id: tick-c85ba3

**Goal**: Address findings from Analysis (Cycle 4).

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| portal-10-1 | Align GoReleaser config and release workflow with tick reference pattern | none | authored | tick-eef443 |

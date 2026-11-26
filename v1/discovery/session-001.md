# Discovery Session 001

Date: 2026-11-25
Work unit: v1

Written by the migration: the research below fed several topics, so it is kept here, on the unit's discovery log, until the cut and the merges place it where it bears.

## Research: cc-tool-plan

## CX - Claude eXecute

### Vision

A Go CLI tool for managing Claude Code sessions with first-class Zellij support. Designed for developers who want persistent, resumable Claude sessions across terminal restarts, crashes, and remote access scenarios.

**Primary goals:**
- Zellij-first workflow (sessions survive crashes/restarts)
- Quick project switching with directory memory
- Remote session attachment (SSH/Mosh from phone → pick session → resume)
- Clean, intuitive TUI using bubbletea
- Homebrew installable

#### Naming

The binary is called `cx` (Claude eXecute). Short, memorable, easy to type.

```bash
brew install leeovery/tools/cx
```

---

### Core Concepts

#### Session Types

1. **Zellij Session** (default): Claude runs inside a Zellij session with a custom layout. Survives terminal crashes, can be attached from anywhere.

2. **Raw Session**: Claude runs directly in current terminal. Lost if terminal closes. Useful for quick one-off questions.

#### Project Registry

The tool maintains a registry of known projects:
- Path to project directory
- Last-used timestamp
- Optional alias/nickname
- Session count (how many times used)

When starting a new session, you can pick from recent projects, and the tool will `cd` into that directory before launching Claude.

#### Session Tracking

For Zellij sessions, the tool queries Zellij directly for session state. Each session is associated with:
- Project name (derived from directory or explicit)
- Working directory
- Status (running, attached, exited)
- Creation time

---

### CLI Interface

#### Primary Command (Unified View)

The main command `cx` (or `claude-sessions`) shows a unified interactive view:

```bash
cx                      # Show unified picker: active sessions + recent projects
cx .                    # Quick start: new session in current directory
cx <project>            # Fuzzy match → attach existing or start new
cx <path>               # If path exists, cd there and start new session

# Modifiers (apply to new sessions)
cx --raw                # Skip Zellij, run Claude directly (one-off)
cx --continue, -c       # Pass --continue to Claude
cx --resume, -r         # Pass --resume to Claude
```

**The unified view abstracts away Zellij session naming.** Users think in terms of projects/directories, not session names like "myapp-01". The tool handles the mapping internally.

#### Management Commands

```bash
cx list                 # Non-interactive list of all sessions
cx list --all           # Include dead/exited sessions
cx kill                 # Interactive picker to kill sessions
cx kill <pattern>       # Kill sessions matching pattern
cx kill --all           # Kill all active sessions
cx clean                # Delete all exited/dead Zellij sessions
cx projects             # List registered projects
cx projects add [path]  # Register directory (default: current)
cx projects forget      # Interactive picker to remove
cx config               # Show/edit configuration
cx help                 # Help
```

---

### Configuration

Location: `~/.config/cx/config.yaml`

```yaml
# Default behavior
defaults:
  use_zellij: true              # false = raw Claude by default
  zellij_layout: "claude"       # Layout name for Zellij sessions
  project_dir: "~/Code"         # Default directory to scan for projects

# Session naming
session:
  format: "{project}-{num}"     # e.g., "myapp-01"
  num_padding: 2                # Zero-pad session numbers

# UI preferences
ui:
  show_timestamps: true         # Show last-used times in project picker
  max_recent: 10                # Max projects to show in quick picker
  theme: "default"              # Future: color themes

# Claude options
claude:
  default_args: []              # Args always passed to Claude
  # Example: ["--model", "opus"]
```

---

### Data Storage

Location: `~/.config/cx/`

#### projects.json

```json
{
  "projects": [
    {
      "path": "/Users/lee/Code/myapp",
      "name": "myapp",
      "alias": null,
      "last_used": "2024-01-15T10:30:00Z",
      "use_count": 42
    }
  ]
}
```

#### Sessions

**No persistent session storage.** Sessions are queried live from Zellij. The tool parses `zellij list-sessions` output to get current state.

For session → project mapping, we can either:
1. Encode project path in session name (ugly)
2. Store a lightweight session registry that maps session names to project paths
3. Query the session's CWD from Zellij (if possible)

**Recommendation:** Option 2 - maintain `sessions.json` with active session mappings, cleaned up when sessions are deleted.

```json
{
  "sessions": {
    "myapp-01": {
      "project": "/Users/lee/Code/myapp",
      "created": "2024-01-15T10:30:00Z"
    }
  }
}
```

---

### Feature Details

#### 1. Unified Interactive View

When running `cx` with no arguments, show a single unified view combining active sessions and recent projects:

```
┌─ Claude Sessions ────────────────────────────────┐
│                                                  │
│  > myapp          ~/Code/myapp       ● 2 active  │
│    dotfiles       ~/.dotfiles          3h ago   │
│    api-server     ~/Code/api         ○ 1 active  │
│    website        ~/Code/website       1w ago   │
│                                                  │
│  ─────────────────────────────────────────────   │
│    [.] current directory                         │
│    [n] new directory...                          │
│                                                  │
│  [enter] select  [/] filter  [?] help            │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Indicators:**
- `● N active` = has running sessions (● = attached to one)
- `○ N active` = has running sessions (none attached)
- `3h ago` = no active sessions, shows last used time

**Behavior when selecting a project:**

If project has **no active sessions**:
→ cd to directory → start new session

If project has **one active session**:
→ cd to directory → attach to it

If project has **multiple active sessions**:
→ Show sub-picker to choose which to attach, or start new:

```
┌─ myapp ──────────────────────────────────────────┐
│                                                  │
│  > Attach: session 1          started 2h ago    │
│    Attach: session 2          started 30m ago   │
│    ─────────────────────────────────────────     │
│    Start new session                             │
│                                                  │
│  [enter] select  [esc] back                      │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Note:** Session names like "myapp-01" are internal implementation. Users see "session 1", "session 2" etc. The abstraction keeps things simple.

#### 2. Non-Interactive List

`cx list` for quick terminal output (no TUI):

```
myapp (~/Code/myapp)
  ● session 1    running  attached   2h ago
  ○ session 2    running             30m ago

dotfiles (~/.dotfiles)
  ○ session 1    running             3h ago

api-server (~/Code/api)
  ◌ session 1    exited              1w ago

3 active, 1 exited
```

#### 3. Remote Attach Flow

When SSHing from phone and running `cx`:

1. Same unified view appears
2. Select project with active session
3. Tool looks up project directory from registry
4. `cd` into project directory
5. Attach to Zellij session

This ensures the shell is in the right directory when you detach or the session ends. The user doesn't need to know `cx attach` exists - just `cx` handles everything.

#### 4. Session Naming

Format: `{project}-{NN}`

- Project derived from directory basename (lowercase, strip leading dot)
- Number increments per project (01, 02, 03...)
- Numbers consider both active AND exited sessions to avoid confusion

Example: Starting sessions in `~/Code/MyApp`:
- First: `myapp-01`
- Second (while 01 running): `myapp-02`
- After killing 01 and 02: `myapp-03` (not 01)

#### 5. Cleanup

`cx clean` deletes all exited Zellij sessions and removes them from `sessions.json`.

`cx kill` offers:
- Interactive picker for selective killing
- Pattern matching (`cx kill myapp` kills all myapp-* sessions)
- `--all` flag for bulk kill

---

### Technical Architecture

#### Project Structure

```
cx/
├── cmd/
│   └── cx/
│       └── main.go           # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go           # Cobra root command
│   │   ├── start.go          # Default/start command
│   │   ├── list.go           # List sessions
│   │   ├── attach.go         # Attach to session
│   │   ├── kill.go           # Kill sessions
│   │   ├── clean.go          # Clean dead sessions
│   │   ├── projects.go       # Project management
│   │   └── config.go         # Config management
│   ├── config/
│   │   └── config.go         # Config loading/saving
│   ├── projects/
│   │   └── registry.go       # Project registry operations
│   ├── sessions/
│   │   ├── zellij.go         # Zellij interaction
│   │   ├── tracker.go        # Session tracking
│   │   └── claude.go         # Claude process spawning
│   ├── ui/
│   │   ├── picker.go         # Project/session picker
│   │   ├── list.go           # Session list view
│   │   └── styles.go         # Lipgloss styles
│   └── util/
│       └── paths.go          # Path utilities
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

#### Dependencies

```go
require (
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/charmbracelet/bubbletea v0.25.0  // TUI framework
    github.com/charmbracelet/lipgloss v0.9.0    // Styling
    github.com/charmbracelet/bubbles v0.18.0    // UI components
    github.com/sahilm/fuzzy v0.1.0              // Fuzzy matching
    gopkg.in/yaml.v3 v3.0.1                     // Config parsing
)
```

#### Zellij Integration

```go
// sessions/zellij.go

type ZellijSession struct {
    Name     string
    Status   SessionStatus  // Running, Exited
    Attached bool
}

func ListSessions() ([]ZellijSession, error) {
    // Run: zellij list-sessions
    // Parse output (strip ANSI codes)
    // Return structured data
}

func AttachSession(name string) error {
    // Exec: zellij attach <name>
}

func CreateSession(name, layout string) error {
    // Exec: zellij attach -c <name> options --default-layout <layout>
}

func KillSession(name string) error {
    // Run: zellij kill-session <name>
}

func DeleteSession(name string) error {
    // Run: zellij delete-session <name>
}
```

#### Claude Spawning

```go
// sessions/claude.go

type ClaudeOptions struct {
    Continue bool
    Resume   bool
    Print    bool
    Args     []string
}

func StartClaude(opts ClaudeOptions) error {
    args := []string{}
    if opts.Continue {
        args = append(args, "--continue")
    }
    if opts.Resume {
        args = append(args, "--resume")
    }
    // ... etc

    // Exec replaces current process with claude
    return syscall.Exec(claudePath, args, os.Environ())
}
```

---

### Homebrew Distribution

#### Tap Setup

Repository: `github.com/leeovery/homebrew-tools`

Formula: `Formula/cx.rb`

```ruby
class Cx < Formula
  desc "Claude eXecute - Claude Code session manager with Zellij support"
  homepage "https://github.com/leeovery/cx"
  url "https://github.com/leeovery/cx/releases/download/v1.0.0/cx-1.0.0.tar.gz"
  sha256 "..."
  license "MIT"

  depends_on "zellij"

  def install
    bin.install "cx"
  end

  test do
    assert_match "cx version", shell_output("#{bin}/cx --version")
  end
end
```

#### Installation

```bash
brew tap leeovery/tools
brew install cx
```

#### Release Process

1. Tag release in cx repo
2. Build binaries (darwin-arm64, darwin-amd64)
3. Create GitHub release with binaries
4. Update formula with new URL and sha256

---

### Open Questions

#### 1. Zellij layout location

Where should the custom Claude layout live?
- Bundled with the tool (installed to `~/.config/zellij/layouts/`)
- User maintains their own (tool just references by name)
- Tool has a `cx setup` command that installs the layout

**Recommendation:** User maintains layout, tool references by name from config. Keeps concerns separated.

#### 2. Session directory tracking

How do we know which directory a session belongs to?
- Store in `sessions.json` when session is created
- Parse from session name (encode path somehow)
- Query Zellij for CWD (may not be possible/reliable)

**Recommendation:** Store in `sessions.json`. Simple, reliable.

#### 3. Auto-registration of projects

Should we auto-register projects when starting sessions?
- Yes: Less friction, projects remembered automatically
- No: User explicitly controls what's in the registry

**Recommendation:** Yes, auto-register. User can `forget` if needed.

#### 4. What if Zellij isn't installed?

- Error and require Zellij?
- Fall back to raw mode with a warning?
- Make it configurable?

**Recommendation:** Warn and fall back to raw mode. Zellij is the default but shouldn't be a hard requirement.

#### 5. Continue/Resume behavior

Should `-c` and `-r` work with existing sessions?
- `cx -c myapp` → attach myapp-01 and pass --continue?
- Or: `-c` and `-r` only apply when starting new Claude instances?

**Recommendation:** These flags only affect new Claude processes. For existing sessions, you're attaching to an already-running Claude.

---

### Implementation Phases

#### Phase 1: Core MVP
- Basic project picker (bubbletea)
- Zellij session creation/attachment
- Project registry (add, list, forget)
- Session listing
- Config file support
- `--raw` flag for non-Zellij mode

#### Phase 2: Session Management
- Interactive attach picker with cd-before-attach
- Kill command with picker and patterns
- Clean command for dead sessions
- Session → project mapping in sessions.json

#### Phase 3: Polish
- Fuzzy matching everywhere
- Better error messages
- `--continue` and `--resume` flags
- Help system
- Shell completions (bash, zsh, fish)

#### Phase 4: Distribution
- GitHub repo setup
- Build automation (Makefile/GoReleaser)
- Homebrew formula
- README and docs

---

### Example Workflows

#### Daily workflow

```bash
# Morning: Start working on myapp
$ cx
# [unified picker shows myapp with no active sessions]
# Select myapp → cd ~/Code/myapp → starts new session

# ... work, close laptop, go home ...

# Evening: SSH from phone
$ ssh mac
$ cx
# [same unified picker shows myapp with ● 1 active]
# Select myapp → cd ~/Code/myapp → attaches to session

# Check progress, detach
# Ctrl+o, d
```

#### Quick question (no project)

```bash
$ cx --raw
# Starts claude directly in current dir, no Zellij
```

#### Resume last conversation

```bash
$ cx -r
# Opens unified picker, then runs claude --resume in selected project
```

#### Multiple sessions on same project

```bash
$ cx myapp       # Creates session 1
$ cx myapp       # Shows sub-picker: attach session 1, or start new
                 # Select "start new" → creates session 2
$ cx list
# Shows both sessions under myapp
```

#### Cleanup

```bash
$ cx list --all
# Shows 3 active, 5 exited

$ cx clean
# Deleted 5 exited sessions

$ cx list
# Shows 3 active, 0 exited
```

---

### Notes for Implementation

1. **Use `syscall.Exec` for Claude/Zellij**: Don't spawn as child process - replace the current process so signals work correctly.

2. **Handle ANSI codes**: Zellij outputs colored text. Strip codes when parsing.

3. **Atomic file writes**: Use write-to-temp + rename for `projects.json` and `sessions.json` to prevent corruption.

4. **XDG compliance**: Use `~/.config/cx/` for config, could also support `$XDG_CONFIG_HOME`.

5. **Path expansion**: Handle `~` in paths, resolve symlinks consistently.

6. **Testing**: Mock Zellij commands for unit tests. Integration tests can use actual Zellij if available.

## Research: tmux-session-managers-analysis

---
topic: zw
status: concluded
type: research
date: 2026-01-26
---

## Comparative Analysis: Tmux Session Managers vs ZW

Analysis of three tmux-based session management tools compared against ZW (Zellij Workspaces).

### Projects Analyzed

| Project | Language | Stars | Approach |
|---------|----------|-------|----------|
| [tmux-session-wizard](https://github.com/27medkamal/tmux-session-wizard) | Shell (85.9%) | ~247 | tmux plugin, fzf popup |
| [tmux-sessionx](https://github.com/omerxx/tmux-sessionx) | Shell (88.5%) | ~1.2k | tmux plugin, fzf popup with preview |
| [sesh](https://github.com/joshmedeski/sesh) | Go (99.8%) | ~1.6k | Standalone CLI, delegates UI to fzf/gum |

---

### 1. tmux-session-wizard

#### What It Does

A tmux plugin that binds to `prefix + T` and pops up an fzf picker showing existing sessions + recently visited directories (via zoxide). Select one and it either switches to the session or creates a new one named after the directory.

#### Architecture

Minimal shell scripts across `bin/t` (main entry point), `src/helpers.sh` (utility functions), and a tmux plugin wrapper. The `bin/t` script combines two data sources into fzf: active tmux sessions (excluding current) and zoxide query results. Selection handling is straightforward — if the result contains `:` it's a session:window reference, otherwise it's a path that gets promoted in zoxide and turned into a new session. No custom TUI, no config file, no project registry.

#### What It Does Well

- **Extreme simplicity**: The entire tool is a handful of shell functions. No install complexity, no config files, no learning curve.
- **zoxide integration**: Leverages an existing tool the user probably already has to provide "project memory" without maintaining its own registry. Directories you `cd` to frequently already appear in zoxide's database.
- **Works outside tmux too**: The `t` command can be run from bare shell, not just inside tmux.
- **Session naming modes**: `directory` (basename), `full-path`, `short-path` (compresses intermediate dirs to initials, e.g. `/u/l/bin`) — gives users control over how sessions are named from directories. Implemented via a `session_name()` function with a `_normalize()` helper that replaces spaces, dots, and colons with hyphens.

#### What ZW Does Better

- **Built-in TUI**: ZW's Bubble Tea interface is purpose-built for the use case, including sections, empty states, and mobile optimization. Session-wizard is just a raw fzf list.
- **Exited/resurrectable sessions**: ZW shows exited sessions as a separate section. Session-wizard only shows live sessions.
- **Session metadata**: ZW shows tab count, attached indicator. Session-wizard shows nothing beyond the name.
- **Layout selection**: ZW lets users pick a Zellij layout when creating a new session. Session-wizard always creates a bare session.
- **Utility mode**: ZW adapts its UI when running inside the multiplexer. Session-wizard has no such concept.
- **Kill/rename operations**: ZW provides session management beyond just switching. Session-wizard only switches.

#### What We Could Learn

- **zoxide as a project source**: Rather than maintaining our own `projects.json` from scratch, we could optionally pull directory suggestions from zoxide. This would give users a "warm start" without needing to manually register projects. Our manual registry is still valuable (aliases, custom names), but zoxide could supplement the file browser as a discovery mechanism.
- **No-config philosophy taken further**: Session-wizard proves you can get a lot done with zero configuration. ZW's config is already minimal, but this reinforces the value of that approach.

---

### 2. tmux-sessionx

#### What It Does

A feature-rich tmux plugin that provides a fuzzy session manager with preview panes, multiple browse modes (sessions, windows, tree, config directories, tmuxinator projects, fzf-marks, zoxide), and inline session operations (rename, delete).

#### Architecture

Shell scripts orchestrating fzf-tmux with extensive configuration via tmux options (`set -g @sessionx-*`). The main `sessionx.sh` script has a `get_sorted_sessions()` function that retrieves sessions with most-recently-used ordering and applies configurable filter patterns. An `input()` function switches between window mode (listing all windows across sessions) and session mode. An `additional_input()` function adds custom config paths and subdirectories, deduplicating against existing sessions. The `handle_output()` function dispatches based on selection type: directory paths trigger directory-based session creation, colon-delimited strings target specific session:window pairs, and plain text maps to session names. Uses `bat` for syntax-highlighted previews.

#### What It Does Well

- **Preview pane**: Shows a file tree of the session's working directory alongside the session list. This gives context about what each session contains without switching to it.
- **Multiple browse modes**: Users can cycle through different views (sessions → windows → tree → config dirs → tmuxinator → zoxide → fzf-marks) all from within the same popup. Each mode has a `Ctrl-*` shortcut.
- **Window-level navigation**: Not just sessions - users can browse individual windows across all sessions and jump directly to a specific window.
- **Tree mode**: Hierarchical view showing sessions with their windows nested underneath.
- **Inline rename**: `Ctrl-r` renames a session without leaving the picker.
- **Inline delete**: `alt+backspace` deletes a session immediately.
- **Rich customization**: 20+ configuration options for appearance, behavior, key bindings, and integrations.

#### What ZW Does Better

- **Mobile-first design**: SessionX's dense multi-mode interface would be unusable on a phone. ZW's clean sections and minimal chrome are designed for small screens.
- **Purpose-built TUI**: SessionX is constrained by what fzf can do. ZW's Bubble Tea interface allows custom layouts, sections, prompts, and flows that fzf can't express.
- **Session creation flow**: ZW has a deliberate naming prompt + layout selection. SessionX just creates a session from whatever text you typed into fzf - no naming prompt, no layout choice.
- **Project memory with metadata**: ZW's `projects.json` stores names, aliases, and timestamps. SessionX relies on external tools (zoxide, fzf-marks, tmuxinator) for all project memory.
- **Simpler mental model**: SessionX has 7+ modes with different key bindings in each. ZW has two clear states: main picker and utility mode.
- **File browser**: ZW's interactive directory browser is more intuitive than SessionX's `Ctrl-e` expand-PWD approach.

#### What We Could Learn

- **Preview pane concept**: The idea of showing context about a session (file tree, last command, etc.) without entering it is valuable. ZW already shows tab count, but we could consider an optional detail view or preview for sessions.
- **Window-level awareness**: SessionX lets users jump to specific windows, not just sessions. ZW's equivalent would be awareness of Zellij tabs. We already query tab names - we could potentially allow navigating to a specific tab.
- **Tree/hierarchical view**: Showing sessions with their tabs nested underneath could be useful for users with many sessions. Worth considering as a future enhancement.
- **Integration ecosystem**: SessionX integrates with tmuxinator, fzf-marks, and zoxide simultaneously. While ZW shouldn't bloat itself with integrations, the pattern of "aggregate multiple session sources" is worth noting. sesh does this more cleanly (see below).

---

### 3. sesh

#### What It Does

A standalone Go CLI that aggregates session sources (tmux sessions, configured projects, tmuxinator layouts, zoxide directories) into a unified list. It delegates the UI entirely to external tools (fzf, gum, television) - sesh itself has no TUI. Users wire it together via tmux key bindings.

#### Architecture

Clean Go codebase with ~20 focused packages: `tmux/` (session operations), `zoxide/` (directory integration), `lister/` (aggregates sources), `connector/` (session creation/attachment orchestration), `configurator/` (TOML parsing), `git/` (repo/worktree detection), `cloner/` (git clone support), `namer/` (session name generation), `previewer/` (directory preview), `startup/` (post-creation commands), `tmuxinator/` (template import), and more. The `Connector` interface abstracts session creation with a `Connect(name, opts)` method, coordinating tmux, zoxide, tmuxinator, and startup modules. TOML configuration at `$XDG_CONFIG_HOME/sesh/sesh.toml`. The CLI provides `list`, `connect`, `last`, `root`, `preview`, and `completion` commands. The UI is "bring your own" — typically fzf in a tmux popup.

#### What It Does Well

- **Aggregated session sources with filtering**: `sesh list -t` (tmux only), `sesh list -z` (zoxide only), `sesh list -c` (configured only). Users can compose exactly the view they want.
- **TOML configuration with session definitions**: Users can define sessions with paths, startup commands, preview commands, and multi-window layouts in a structured config file:
  ```toml
  [[session]]
  name = "Project"
  path = "~/projects/example"
  startup_command = "nvim"
  windows = ["git", "build"]
  ```
- **Startup commands**: Run a command automatically when a session is created (e.g., open nvim, start a dev server). This is per-session configurable.
- **`sesh last`**: Quick toggle to the previous session - fixes tmux's detach-on-destroy behavior.
- **`sesh root`**: Detects git repo root for a directory, useful for creating sessions at project root regardless of where you are in the tree.
- **Preview command**: `sesh preview {}` generates a preview for any session, integrable with fzf's `--preview` flag.
- **Shell completions**: Built-in completion generation for bash/zsh/fish/powershell.
- **Platform integrations**: Raycast extension (macOS), Ulauncher extensions (Linux).

#### What ZW Does Better

- **Built-in TUI**: sesh has no UI of its own - users must wire up fzf/gum themselves. ZW provides a complete, polished experience out of the box.
- **Mobile-optimized**: sesh's "compose it yourself" approach means the UI quality depends entirely on the user's fzf configuration. ZW guarantees a good mobile experience.
- **Exited session handling**: ZW treats exited/resurrectable sessions as first-class citizens with a dedicated section. sesh only shows live tmux sessions.
- **Layout selection**: ZW's interactive layout picker is more discoverable than sesh's config-file approach.
- **File browser**: ZW has interactive directory browsing. sesh relies on zoxide or pre-configured paths.
- **Lower complexity**: sesh requires users to understand fzf, tmux key binding syntax, and TOML configuration. ZW requires running `zw`.

#### What We Could Learn

- **Startup commands**: The ability to run a command when creating a new session is genuinely useful. A developer might always want `nvim` to open, or a dev server to start. ZW could consider this as a per-project option in `projects.json`.
- **`last` command (quick toggle)**: Instantly switching to the previous session is a common workflow. ZW could offer `zw last` or a key binding to toggle between the two most recent sessions.
- **`root` command (git-aware paths)**: Starting sessions at the git root rather than the current subdirectory is smart. ZW's file browser or project registration could auto-detect git roots.
- **Sort order control**: sesh lets users control the ordering of session types in the list. ZW could consider letting users choose whether sessions or projects appear first.
- **Shell completions**: sesh generates shell completions for its CLI commands. ZW should do the same - it's a standard Go CLI feature (Cobra supports this out of the box).
- **Composable CLI design**: sesh's `list` + `connect` + `preview` commands make it scriptable. ZW already has `list` and `attach` which serve similar purposes, but `preview` is interesting.
- **Multi-window session definitions**: sesh allows defining sessions with multiple named windows and startup scripts. This is more structured than just selecting a layout file. However, this overlaps heavily with Zellij's own layout system (.kdl files), so ZW should lean on Zellij's layouts rather than duplicating this.

---

### Summary: What ZW Gets Right

These are areas where ZW's design is equal to or stronger than all three tools:

1. **Purpose-built TUI** - None of the three build their own TUI. They all delegate to fzf. ZW's Bubble Tea interface allows a tailored experience that fzf can't match, particularly for mobile.
2. **Mobile-first design** - None of the three consider small screens. ZW is explicitly designed for phone SSH.
3. **Exited session management** - Only ZW treats resurrectable sessions as a distinct, visible concept.
4. **Layout selection** - Only ZW presents an interactive layout picker during session creation.
5. **Utility mode** - Only ZW adapts its behavior when running inside the multiplexer with a dedicated mode.
6. **Session metadata display** - Tab count and attached indicators in the picker are unique to ZW.
7. **Structured project memory** - ZW's `projects.json` with aliases, names, and timestamps is richer than zoxide's implicit frequency tracking.

### Summary: Ideas Worth Adopting

Ranked by value-to-effort:

| Idea | Source | Effort | Value | Notes |
|------|--------|--------|-------|-------|
| Shell completions | sesh | Low | Medium | Cobra gives this for free |
| `zw last` toggle | sesh | Low | Medium | Quick switch to previous session |
| Startup commands per project | sesh | Medium | High | `"startup_command": "nvim"` in projects.json |
| zoxide as optional directory source | wizard, sesh | Medium | Medium | Supplement file browser, not replace projects.json |
| Git root detection | sesh | Low | Low | Auto-suggest git root when registering a project |
| Tab-level navigation | sessionx | High | Low | Jump to specific Zellij tab from picker |

### Summary: Ideas Explicitly Not Worth Adopting

| Idea | Why Not |
|------|---------|
| Delegating UI to fzf | ZW's whole value is the purpose-built TUI |
| Multiple browse modes (7+ modes like sessionx) | Complexity antithetical to ZW's mobile-first simplicity |
| tmuxinator/fzf-marks integration | Zellij ecosystem, not tmux ecosystem |
| Headless/composable-only CLI (like sesh) | ZW's TUI *is* the product; scripting is secondary |
| Preview pane in picker | Screen real estate too precious on mobile |
| Extensive configuration options (20+ like sessionx) | ZW is opinionated-by-design |

## Discussion

---
topic: cx-design
status: concluded
work_type: greenfield
date: 2026-01-22
---

## Discussion: CX Tool Design

### Context

CX (Claude eXecute) is a Go CLI for managing Claude Code sessions with Zellij support.

**The actual problem**: When SSH/Mosh-ing to a machine (e.g., from phone to Mac), it's tedious to navigate to the right project directory and attach to/start the right Zellij session. Want a single command that handles everything.

**Core workflow**:
1. SSH/Mosh to machine (outside CX scope)
2. Run `cx`
3. See unified view of sessions/projects
4. Pick one → CX handles cd + session management

#### References

- [CX Tool Plan](../../discovery/session-001.md#research-cc-tool-plan) - Initial exploration (contains hallucinated details - validate each decision)

### Questions

- [x] What is CX and what problem does it solve?
- [x] What are the core capabilities?
- [x] UI interaction model - keyboard shortcuts, navigation?
- [x] Directory discovery - how to find new projects?
- [x] Session management - naming, multiple per project?
- [x] Data storage - what needs persisting?
- [x] Zellij integration details?
- [x] Configuration needs?
- [x] CLI interface - subcommands or just `cx`?
- [x] Distribution approach?

---

*Each question gets its own section below. Check off as concluded.*

---

### What is CX and what problem does it solve?

#### Decision

CX is a **multi-purpose startup command** for Claude Code sessions with Zellij. Single entry point that handles:
- Listing active sessions and known projects
- Attaching to existing sessions
- Starting new sessions
- Directory navigation

**Not in scope**: Remote access itself (handled by SSH/Mosh separately).

---

### What are the core capabilities?

#### Decision

From unified TUI, user can:

1. **Attach to existing session** - select running session, CX does cd + attach
2. **Start new session for known project** - select project with no active session, CX does cd + start
3. **Start new session in current directory** - quick option always available
4. **Start new session in new directory** - fuzzy search through configured path (e.g., ~/Code)
5. **Multiple sessions per project** - supported, can start new even if one running

**Keyboard shortcuts while navigating list**:
- `A` - Attach to existing session
- `N` - New session (even if one already running)
- `C` - Start with `claude --continue`
- `R` - Start with `claude --resume`
- `K` - Kill session (with confirmation)
- Arrow keys / typing for navigation + fuzzy filter

**Housekeeping**:
- Remove dead projects from list (directory no longer exists)

---

### UI interaction model

#### Context

Need a clean, keyboard-driven TUI that handles both session management and project selection without being confusing.

#### Options Considered

**Flat list** - all items together, sessions and projects mixed
- Pro: Simple
- Con: Unclear what action Enter performs on each item type

**Grouped** - sections for "Active Sessions" and "Recent Projects"
- Pro: Clear separation
- Con: Implies active sessions are only for attaching, but user wants to start new sessions from active projects too

**Flat with drill-down** - projects only in main list, drill into sub-picker for sessions
- Pro: Clean main view, focused sub-view for session management
- Con: Extra navigation level

#### Decision

**Flat list with drill-down** - two-level navigation.

##### Main List

Shows projects only (flat, sorted by recency):
```
> myapp          ● 2 active
  dotfiles       ● 1 active
  api-server
  website
  ──────────────
  [.] current directory
  [/] find new...
```

**Keyboard shortcuts at main list level**:
- Arrow keys / typing → navigate + fuzzy filter
- `N` → cd to project + start new session
- `C` → cd to project + start with `claude --continue`
- `R` → cd to project + start with `claude --resume`
- `K` → kill all sessions for project (with confirmation)
- `Enter` → drill into sub-picker (if sessions exist) OR prompt to start (if no sessions)

##### Sub-picker (when drilling into project with sessions)

Focus is on managing existing sessions:
```
myapp
──────────────
> session 1    2h ago (attached)
  session 2    30m ago
  ──────────────
  Start new session for myapp

[esc] back
```

**Keyboard shortcuts in sub-picker**:
- `Enter` on session → attach to it
- `K` on session → kill that session (with confirmation)
- `Enter` on "Start new session" → starts new
- `Esc` → back to main list

Note: N/C/R not shown in sub-picker - if user wants those flags, they select "Start new session" which could then prompt for mode, or they back out to main list and use shortcuts there.

##### No sessions scenario

When pressing `Enter` on a project with no active sessions:
- Shows prompt: "Start new session for api-server?"
- Options: Normal / Continue / Resume
- Keeps behavior consistent and explicit

#### Nice to Have

**Attached device info**: Show which device is attached to a session (e.g., "attached - Lee's iPhone"). Zellij shows attached state but not the connecting device. Would need to map TTY to SSH connection via `who` or similar. Defer to post-MVP.

---

### Directory discovery

#### Context

When user wants to start a session in a new project (not already in the project list), how do they find it without manually cd-ing first?

#### Options Considered

**Configured search path with flat fuzzy search**
- Configure root(s) like `~/Code`, scan 2-3 levels deep
- `[/]` opens fuzzy finder over all found dirs
- Pro: Simple, fast
- Con: Might miss deeply nested projects, requires upfront scanning, config complexity

**Interactive file browser**
- `[/]` opens browser starting at ~ (or configured root)
- Shows directories only, navigate with arrows
- Enter to descend, Esc to go up
- N/C/R to select current dir and start session
- Typing filters current level
- Pro: Handles any depth, no scanning needed, matches mental model
- Con: Slightly more to build

**Hybrid** - flat search with fallback to browser
- More complex, unclear benefit

#### Decision

**Interactive file browser** (Option B).

When user selects `[/] find new...`:
- Opens browser at ~ (could make starting point configurable later)
- Shows directories only (not files)
- Arrow keys to navigate list
- `Enter` to descend into highlighted directory
- `Esc` to go up one level (or exit browser if at root)
- Typing filters directories at current level
- `N`/`C`/`R` to select current directory and start session with respective mode

Once a directory is selected and session started, it gets added to the project list automatically.

---

### Session management

#### Context

Need to handle naming for both projects and sessions, especially when directory names alone aren't descriptive (e.g., "api" could be any project's api folder).

#### Decision

##### Project naming

- **Directory path is the unique key** - two projects can't have the same path
- **Custom display name** - stored separately from path
- **On first session in new directory**: prompt "Project name?" with default = directory basename
- User can accept default or customize (e.g., "api" → "project-a-api")
- Name cached in project registry, used for all future sessions in that directory
- **Rename mechanism**: keyboard shortcut in main list (not `R` - that's resume; decide binding later)

##### Session naming

- Format: `{project-name}-{NN}` (e.g., `project-a-api-01`)
- Uses custom project name, not raw directory basename
- Numbers increment, never reused (even after sessions die) to avoid confusion
- Two-digit padding (01, 02... 99) - sufficient for practical use

##### Display in UI

- **Main list**: shows custom project name
- **Sub-picker**: shows "session 1", "session 2" (friendly) - project context already clear from header
- Internal Zellij session name (`project-a-api-01`) is an implementation detail, not shown to user

---

### Data storage

#### Decision

Location: `~/.config/cx/`

**Config** (separate file): `config.yaml`
- User preferences, defaults
- See Configuration section

**Data** (JSON files):

`projects.json`:
```json
{
  "projects": [
    {
      "path": "/Users/lee/Code/project-a/api",
      "name": "project-a-api",
      "alias": "api",
      "last_used": "2026-01-22T10:30:00Z",
      "next_session_num": 3
    }
  ]
}
```

Note: `alias` is optional, must be unique across all projects.

`sessions.json`:
```json
{
  "sessions": {
    "project-a-api-01": {
      "project_path": "/Users/lee/Code/project-a/api",
      "created": "2026-01-22T10:30:00Z"
    }
  }
}
```

**Why sessions.json**: Zellij tracks session state (running/exited/attached) but not which directory it belongs to. We need the mapping to cd correctly before attaching.

**Cleanup**: When sessions are killed/deleted, remove from sessions.json. Periodic cleanup of orphaned entries (Zellij session gone but still in our file).

---

### Zellij integration

#### Context

Zellij is what makes CX valuable - without it, we're just cd-ing and running Claude. The persistent sessions, crash survival, and multi-device attach all come from Zellij.

#### Decision

**Zellij is a hard dependency.** If not installed, CX errors with a helpful message ("CX requires Zellij. Install with: brew install zellij").

No raw/fallback mode - it would undermine the tool's purpose.

#### Layout

Current understanding: Zellij layouts define pane structure. We need a layout that:
- Opens single pane
- Immediately runs Claude

**Options** (need research):
1. CX installs layout to `~/.config/zellij/layouts/cx.kdl` on first run
2. CX passes inline layout or layout file path to Zellij at session creation
3. User manages layout separately, CX references by name from config

**Leaning toward**: Option 1 or 2 - CX manages the layout so user doesn't have to configure Zellij manually. Reduces friction.

#### Session operations

**Creating**: `zellij attach -c <session-name>` (creates if doesn't exist)
- Need to verify: how to specify layout? `--layout` flag?

**Attaching**: `zellij attach <session-name>`

**Listing**: `zellij list-sessions` - parse output for session names + status

**Killing**: `zellij kill-session <session-name>`

**Deleting** (exited sessions): `zellij delete-session <session-name>`

#### Research needed

- [ ] Exact syntax for creating session with layout
- [ ] Can we pass layout inline or must it be a file?
- [ ] Best location for CX-managed layout file
- [ ] What info does `zellij list-sessions` output include? (attached status, etc.)

---

### Configuration

#### Context

Need to decide what's configurable vs opinionated. YAGNI applies - don't add config options we don't need.

#### Decision

**Minimal configuration.** Be opinionated on defaults.

**Format**: Flat key=value file (like Ghostty), not YAML/JSON.
- Simpler to parse and edit
- Easier future updates, fewer breaking changes
- Overrides defaults only - if not set, use default

Location: `~/.config/cx/config`

```
# Example config (all optional, these are defaults)
claude-args=
```

**Config options** (MVP):
- `claude-args` - Default args passed to Claude (e.g., `--dangerously-skip-permissions`). Empty by default.

**Not configurable** (opinionated):
- File browser start path → uses current directory
- Timestamp display format → pick something sensible
- Session naming format → `{project}-{NN}`
- Themes → none for now

**Future**: Add config options only when real need emerges.

---

### CLI interface

#### Context

Research doc had many subcommands (`cx list`, `cx kill`, `cx projects`, etc.). But TUI handles most operations - do we need them?

#### Decision

**Minimalist approach.** Main command handles most cases, positional arg for shortcuts.

##### Main command

- `cx` → full TUI picker
- `cx .` → current directory context
  - If project known: shows options (new session / attach to existing)
  - If not known: prompts to create project
- `cx <project>` → fuzzy match on name, alias, or path
  - Single match: shows options for that project
  - Multiple matches: "Your input was ambiguous, did you mean one of these?" → picker to select

##### Subcommands (minimal)

- `cx clean` / `cx prune` → remove dead/exited sessions (non-interactive)
- `cx version` → version info
- `cx help` → usage help

**Not needed**:
- `cx list` → `cx` by itself shows the list
- `cx kill` → use TUI with `K` shortcut

##### Aliases

Projects can have an optional alias for quick access:
- Stored in project registry alongside name and path
- Must be **unique across all projects** - no duplicates allowed
- `cx api` matches alias "api" directly

**Fuzzy matching vs exact alias**:
- Alias match is exact and takes priority
- Fuzzy match can be ambiguous (multiple results) - that's OK, user picks from list
- Example: projects "api-v1" and "api-v2", user types `cx api` → shows both, user picks

---

### Distribution

#### Decision

**Homebrew** as primary distribution for macOS (and Linux where Homebrew is available).

##### Homebrew tap

- Repository: `github.com/leeovery/homebrew-tools`
- Formula declares `depends_on "zellij"` - installed automatically
- Installation: `brew tap leeovery/tools && brew install cx`

##### Platforms

- macOS (arm64, amd64)
- Linux (arm64, amd64)

##### Build automation: GoReleaser

Modern standard for Go CLI distribution. Handles:
- Cross-compilation for all platforms
- Creates GitHub releases with versioned archives
- Auto-generates/updates Homebrew formula in tap repo
- Integrates with GitHub Actions

**Release workflow**:
1. Run release script (generates tag with AI commit message)
2. Push tag
3. GitHub Action triggers, runs GoReleaser
4. GoReleaser builds binaries, creates GitHub Release
5. GoReleaser pushes updated formula to `leeovery/homebrew-tools`

##### Linux without Homebrew

GoReleaser creates `.tar.gz` archives. Options:
- Manual download from GitHub Releases
- Simple install script that curls release and extracts to `/usr/local/bin`

##### Files needed

- `.goreleaser.yaml` - GoReleaser config
- `.github/workflows/release.yml` - GitHub Action for releases
- `release.sh` - Tag/release script (user will provide)

---

### Summary

#### What is CX

A Go CLI for managing Claude Code sessions with Zellij. Single entry point (`cx`) that handles listing projects/sessions, attaching to existing sessions, and starting new ones - all with automatic directory navigation.

#### Key decisions

1. **Zellij is required** - no fallback mode, it's the core value
2. **Two-level TUI** - flat project list, drill into sub-picker for sessions
3. **Keyboard-driven** - N/C/R to start sessions, K to kill, Enter to drill/select
4. **Custom project names** - prompted on first use, stored with path as key
5. **Aliases** - optional, unique, for quick `cx <alias>` access
6. **Minimal CLI** - `cx`, `cx .`, `cx <project>`, `cx clean`
7. **Flat config** - key=value format, minimal options
8. **GoReleaser + Homebrew** - modern Go distribution

#### Research deferred to implementation

- Zellij layout mechanics (creating sessions with layout, layout file location)

#### Nice-to-haves deferred

- Attached device info (show which device connected to session)

#### Next steps

Ready for specification and planning phases.

## Research: cc-tool-plan, from discovery session 001

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

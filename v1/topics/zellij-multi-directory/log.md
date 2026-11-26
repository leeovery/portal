## Discussion

---
topic: zellij-multi-directory
status: concluded
work_type: greenfield
date: 2026-01-22
---

## Discussion: Zellij Multi-Directory Sessions

### Context

CX's current design assumes a 1:1 relationship between Zellij sessions and project directories. However, Zellij sessions can have **multiple panes spread across multiple directories/projects**.

This challenges the core model and could dramatically change the tool's architecture.

#### References

- [CX Design Discussion](../cx-design/log.md#discussion) - Concluded design assuming simpler model
- [CX Tool Plan](../../discovery/session-001.md#research-cc-tool-plan) - Initial research

#### Current Model Assumptions (from cx-design.md)

1. **sessions.json** maps each session to exactly one project path
2. **cd before attach** - CX changes to project dir before attaching
3. **Project as anchor** - TUI organizes around projects, sessions as children
4. **Session naming** - `{project-name}-{NN}` implies project ownership

### Questions

- [ ] How do users actually use Zellij multi-pane sessions?
- [ ] Does the session → project mapping need to change?
- [ ] What should "cd before attach" mean for multi-directory sessions?
- [ ] How does this affect the TUI model?
- [ ] Is the naming convention still valid?

---

*Each question gets its own section below. Check off as concluded.*

---

### How do users actually use Zellij multi-pane sessions?

#### The Problem

Zellij allows multiple panes per session, each potentially in a different directory. Need to understand how this is actually used in practice to determine if CX's model needs fundamental changes.

#### Journey

**Confirmed Zellij behavior:**
- No interactive picker - `zellij attach` without args just lists sessions, requires you to pass a name/prefix
- Sessions restore fully on reattach - not "cd-ing", the shell process never died
- You can pass any unambiguous prefix to attach

**Key realization:**
All three multi-pane scenarios (monorepo, separate projects, mixed) are valid use cases. This means session ≠ project. Session = workspace.

**Existential question raised:** Is CX even needed?

User already has aliases wrapping Zellij (`cc` function). The original "cd to project before attach" value prop disappears since Zellij restores everything.

#### Options Considered

**CX as "Claude executor"** (original vision)
- Tightly coupled to Claude Code
- Manages project → session mapping
- Auto-starts Claude in sessions
- *Problem*: Model assumes 1:1 session-project, which doesn't hold

**CX as "workspace manager"** (emerging vision)
- Zellij wrapper with better UX
- Interactive picker (Zellij lacks this)
- Workspace = session, may span multiple directories
- Claude is incidental, not core
- *Problem*: Might be a thin wrapper over `zellij attach`

**Just use aliases** (status quo)
- Already works
- No maintenance burden
- *Problem*: Limited discoverability, no rich UI

#### Decision

*Pending - need to explore what value CX adds over aliases...*

---

### What defines a workspace?

#### The Problem

If CX is a workspace manager (not project manager), how do workspaces get created and defined? This is the crux of whether CX is worth building.

#### Journey

**What user actually wants:**
1. One command (`cx`) → interactive picker
2. Picker shows existing Zellij sessions (queried live)
3. Picker shows remembered projects (for starting new sessions)
4. Select session → attach
5. Select project → cd + start new Zellij session

**Key simplification:** No session → project mapping needed. They're separate concerns:
- Sessions = live data from `zellij ls`
- Projects = CX's memory of "directories I've started sessions in before"

CX doesn't track which project a session belongs to. Select session → attach. Select project → new session.

**New session flow:**
1. Select `[n] new session...`
2. Project picker (remembered dirs + browse option)
3. Name prompt: "Workspace name? [default: dir-name]"
4. cd to dir, `zellij attach -c <name>`

#### Decision

*Pending - exploring what info we can show per session...*

---

### Can we query session details from outside Zellij?

#### The Problem

Zellij's built-in session manager shows rich info (tabs, panes, commands). Can CX show similar detail from *outside* a running session?

#### Research Findings

**Yes - via CLI actions with `--session` flag:**

```bash
# Get tab names for a specific session
zellij --session my-session action query-tab-names

# Dump full layout (KDL format)
zellij --session my-session action dump-layout

# List connected clients
zellij --session my-session action list-clients
```

This works from *outside* Zellij, targeting any running session by name.

**For exited (resurrectable) sessions:**

Layout stored in cache:
```
~/.cache/zellij/<version>/session_info/<session-name>/session-layout.kdl
```

KDL file contains tabs, panes, cwds, commands:
```kdl
layout {
    tab name="my-tab" cwd="/some/path" {
        pane command="claude" cwd="/project/dir"
        pane command="htop"
    }
}
```

**Parsing considerations:**
- `query-tab-names` returns simple text (easy)
- `dump-layout` returns KDL (need parser - Go has `github.com/sblinch/kdl-go`)
- JSON output requested but not yet implemented (Issue #4212)

#### Decision

CX can show tab names per session:
```
>  cx-03          ● attached
   └─ 2 tabs: "Claude Code", "Tests"
```

Implementation: call `zellij --session <name> action query-tab-names` for each session.

For more detail (pane cwds, commands), parse `dump-layout` KDL output.

---

### What should the TUI look like?

#### The Problem

Need a mobile-friendly, keyboard-driven TUI that works at bare shell (before Zellij is running).

#### Journey

**User's actual workflow:**
1. SSH/Mosh to machine → drops into `~` (bare shell)
2. Run `cx`
3. Full-screen picker appears (optimized for small screens)
4. Arrow to selection, Enter
5. Attached to session (or new one started)

**Zellij's built-in session manager** exists but:
- Only works *inside* an existing Zellij session
- Too information-dense for mobile screens
- Can't start from bare shell

#### Options Considered

**Minimal info per session:**
```
SESSIONS
  cx-03       ● attached
  api-work
```

**With tab names:**
```
SESSIONS
  cx-03       ● attached
  └─ 2 tabs: "Code", "Tests"
  api-work
  └─ 1 tab: "Main"
```

**With pane details (like Zellij's manager):**
- Too dense for mobile
- Overkill for just picking a session

#### Decision

**Minimal with optional expansion.** Default view shows session names + attached status. Could add tab names as one-liner if useful, but keep it scannable.

```
┌─────────────────────────────────────┐
│                                     │
│           SESSIONS                  │
│                                     │
│    >  cx-03          ● attached     │
│       api-work       2 tabs         │
│       client-proj                   │
│                                     │
│           EXITED                    │
│                                     │
│       old-feature    (resurrect)    │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

---

### Summary (so far)

#### Model Shift

From **project-centric** (original cx-design.md) to **workspace-centric**:

| Original Model | New Model |
|----------------|-----------|
| Session belongs to project | Session = workspace (may span dirs) |
| sessions.json maps session→project | No mapping needed |
| cd before attach | Not needed (Zellij restores) |
| TUI organized by projects | TUI organized by sessions |
| Naming: `{project}-{NN}` | User chooses workspace name |

#### CX's Value Proposition

1. **Interactive picker at bare shell** - what Zellij's session-manager does, but *before* you're inside Zellij
2. **Mobile-friendly** - cleaner, less dense than Zellij's built-in
3. **Project memory** - quick start new sessions in remembered directories
4. **One command** - `cx` does everything vs. `zellij ls` + `zellij attach <name>`

#### Open Questions

- [x] Is this enough value over aliases + `zellij ls | fzf`?
- [x] Session naming convention - still `{name}-{NN}` or free-form?
- [x] Should CX manage layouts for new sessions?
- [x] Can CX detect it's running inside a Zellij session?
- [x] What operations should work from inside vs outside Zellij?

---

### Is CX worth building?

#### Decision

**Yes.** Reasons:
- Personal project, fun to build in Go
- Could expand for other uses later
- Streamlines workflow, reduces cognitive load
- Not trying to sell it - scratching own itch

---

### Session naming convention

#### The Problem

Original design: `{project}-{NN}`. But if workspaces aren't tied to projects, how should naming work?

#### Decision

**Free-form with smart defaults:**

1. **Default** = directory basename (e.g., starting in `~/Code/myapp` suggests "myapp")
2. **Always prompt** before starting, even for previously saved projects
3. **Rename later** supported - workspaces evolve (start as "project-a", becomes "comparison-testing")

**Why always prompt:** User might start session in Project B directory but want to call it "testing-workflow" or something contextual.

**Example flow:**
```
Selected: ~/Code/myapp

Workspace name: [myapp] _
  (Enter to accept, or type a custom name)

Layout: [default] ▾
  • default (single pane)
  • claude (single pane + Claude)
  • split (two panes)
```

---

### Layouts for new sessions

#### The Problem

When starting a new session, what layout should be used?

#### Decision

**Default + optional saved layouts:**

1. **Default** = single pane in the chosen directory (just wraps shell in Zellij)
2. **Saved layouts** = user can pick from predefined layouts
3. **Layout picker** shown at session start (can be skipped with Enter for default)

**Example layouts:**
- `default` - single pane
- `claude` - single pane, runs `claude` command
- `split` - two horizontal panes
- `dev` - custom layout for specific workflow

**Where layouts live:** `~/.config/cx/layouts/` (KDL files, same format as Zellij)

**Future nice-to-have:** "Save current layout" from inside a session.

---

### Running CX from inside Zellij

#### The Problem

If user is already inside a Zellij session and runs `cx`, what should happen? Can CX detect this?

#### Research Needed

Zellij sets environment variables inside sessions:
- `ZELLIJ` = `0` (when inside a session)
- `ZELLIJ_SESSION_NAME` = current session name

So CX *can* detect it's running inside Zellij.

#### Options

**If inside Zellij, CX could:**

1. **Refuse** - "Already in a Zellij session. Use Zellij's session manager instead."
2. **Limited mode** - Show sessions but warn about nesting
3. **Smart mode** - Offer different actions:
   - Rename current session
   - Switch to different session (via Zellij's internal mechanism?)
   - Kill other sessions
   - Just show info (read-only)

#### Decision

**Utility mode** when running inside Zellij:
- Detect via `ZELLIJ` env var
- **Block nesting** - don't allow attaching to another session from inside one
- **Allow safe operations:**
  - Rename current session
  - View other sessions (read-only)
  - Kill other sessions
  - Show current session info

---

### File browser for new projects

#### Decision

**Unchanged from original design:**
- `[/]` or arrow to "new directory" option opens interactive file browser
- Navigate directories, select one to start session there
- Selected directory gets added to remembered projects

---

### Configuration & Storage

#### Decision

**Location:** `~/.config/cx/`

Contents:
- `config` - flat key=value config (as in original design)
- `projects.json` - remembered project directories
- `layouts/` - custom layout files (KDL format)

---

### Error Handling: Zellij not installed

#### Decision

**Not a concern** - Zellij will be a Homebrew dependency. Install scripts will ensure it's present.

If somehow missing, simple error: "CX requires Zellij. Install with: brew install zellij"

---

### CLI Subcommands

#### Decision

**Minimal, as originally designed:**
- `cx` - main TUI picker
- `cx clean` / `cx prune` - remove dead/exited sessions (non-interactive)
- `cx version` - version info
- `cx help` - usage

Most operations happen through the TUI.

---

### Distribution

#### Decision

**Unchanged from original design:**
- Homebrew tap (`leeovery/tools`)
- GoReleaser for builds
- Zellij as brew dependency

---

### Final Summary

#### The Pivot

This discussion identified a fundamental model change for CX:

**Before:** Project-centric tool that maps sessions to single project directories, auto-runs Claude, manages session→project relationships.

**After:** Workspace-centric Zellij wrapper that provides a mobile-friendly session picker, remembers project directories for quick new session creation, and lets workspaces evolve freely across multiple directories.

#### Key Decisions

| Area | Decision |
|------|----------|
| **Core model** | Session = workspace (may span multiple directories) |
| **Session→project mapping** | None needed - separate concerns |
| **Naming** | Free-form, defaults to directory basename, always prompt |
| **Layouts** | Default single pane + optional saved layouts in `~/.config/cx/layouts/` |
| **Inside Zellij** | Utility mode (rename, info, kill) - block nesting |
| **TUI** | Minimal, mobile-friendly, sessions + exited + new option |
| **Session info** | Query via `zellij --session <name> action query-tab-names` |
| **File browser** | Unchanged from original design |
| **Storage** | `~/.config/cx/` |
| **Distribution** | Homebrew + GoReleaser (unchanged) |

#### What Carries Forward from Original Design

- Go + Bubbletea TUI
- Flat config format
- Homebrew distribution via `leeovery/tools` tap
- File browser for new project discovery
- Keyboard shortcuts (N, K, Enter, etc.)
- `cx clean` subcommand

#### What Changes from Original Design

- No `sessions.json` mapping file
- No automatic Claude execution
- Session names are user-chosen, not `{project}-{NN}`
- TUI organized by sessions (not projects with session badges)
- No "cd before attach" (Zellij handles it)
- Utility mode when running inside Zellij

#### Tool Rename

**Old name:** CX (Claude eXecute) - no longer fits since it's not Claude-specific

**New name:** **ZW (Zellij Workspaces)**
- Short, easy to type
- `z` and `w` are close on keyboard
- Clearly communicates purpose: manage Zellij workspaces
- Command: `zw`

**Implications:**
- Config location: `~/.config/zw/`
- Homebrew formula: `zw`
- Repository: consider renaming from `cx` to `zw`

#### Next Steps

1. **Update specification** - Revise based on this discussion's outcomes
2. **Reconcile with cx-design.md** - Mark superseded sections
3. **Rename repository** - `cx` → `zw`
4. **Implementation planning** - New plan reflecting the simpler model

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

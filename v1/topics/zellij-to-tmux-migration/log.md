## Discussion

---
topic: zellij-to-tmux-migration
status: concluded
work_type: greenfield
date: 2026-02-10
---

## Discussion: Migrating from Zellij to tmux

### Context

ZW (Zellij Workspaces) has a concluded specification built around Zellij as the terminal multiplexer. The user has decided to switch to tmux. The core UX design, project memory, TUI architecture, CLI structure, and distribution approach are all multiplexer-agnostic and carry forward. The Zellij-specific integration layer needs reworking.

This discussion identifies what changes, what stays, and resolves the tmux-specific design decisions before revising the specification.

#### References

- [ZW Specification](../specification/zw.md) - Current Zellij-based spec (concluded)
- [cx-design discussion](../cx-design/log.md#discussion) - Original design discussion
- [zellij-multi-directory discussion](../zellij-multi-directory/log.md#discussion) - Model pivot to workspace-centric
- [fzf-output-mode discussion](../fzf-output-mode/log.md#discussion) - `zw list` and `zw attach`
- [git-root-and-completions discussion](../git-root-and-completions/log.md#discussion) - Git root resolution, shell completions
- [tmux-session-managers-analysis](../../discovery/session-001.md#research-tmux-session-managers-analysis) - Comparative analysis of tmux session managers

### Questions

- [x] What are the tmux equivalents for all Zellij session operations?
- [x] What happens to exited/resurrectable sessions (Zellij-native feature)?
- [x] How should the layout system work with tmux?
- [x] Should the tool be renamed (ZW = "Zellij Workspaces")?
- [x] How does utility mode work with tmux?
- [x] What session metadata can we display from outside tmux?
- [x] How does process handoff (exec) work with tmux?
- [x] What changes for the runtime dependency?

---

*Each question gets its own section below. Check off as concluded.*

---

### What are the tmux equivalents for all Zellij session operations?

#### Context

The current spec references Zellij CLI commands throughout. Need verified tmux 3.6a equivalents.

#### Decision

Verified against `man tmux` on the target system (tmux 3.6a):

| Operation | Zellij | tmux (verified) |
|---|---|---|
| Create or attach | `zellij attach -c <name>` | `tmux new-session -A -s <name>` (alias: `new`) |
| Create w/ start dir | cd + create | `tmux new-session -A -s <name> -c <dir>` |
| Attach to existing | `zellij attach <name>` | `tmux attach-session -t <name>` (alias: `attach`) |
| List sessions | `zellij list-sessions` | `tmux list-sessions` (alias: `ls`) |
| Kill session | `zellij kill-session <name>` | `tmux kill-session -t <name>` |
| Delete exited session | `zellij delete-session <name>` | N/A — tmux sessions don't persist after exit |
| Check session exists | N/A | `tmux has-session -t <name>` (alias: `has`) — exit 0/1 |
| Query tab/window names | `zellij --session <name> action query-tab-names` | `tmux list-windows -t <name>` (alias: `lsw`) |
| Rename session | `zellij action rename-session <new>` (inside only) | `tmux rename-session -t <name> <new>` (alias: `rename`) — works from outside |

**Key differences from Zellij:**
- tmux `new-session -A` combines create-or-attach in one command
- tmux supports `-c <dir>` to set the working directory at creation — no need to `cd` first
- tmux `rename-session` works from outside the session (Zellij could only rename from inside)
- tmux `has-session` provides a clean existence check (useful for argument resolution)
- tmux `list-sessions` output is structured, no ANSI codes to strip

**Directory change for new sessions**: The spec's current model (cd to dir, then create) simplifies to just passing `-c <resolved-dir>` on `new-session`. No directory change needed in ZW's process. Git root resolution still applies — resolve first, then pass to `-c`.

---

### What happens to exited/resurrectable sessions?

#### Context

The Zellij spec had an "EXITED" section in the TUI showing dead-but-resurrectable sessions. Zellij natively persists session state after exit and allows individual session resurrection. tmux doesn't — sessions are alive or gone.

#### Journey

Initially explored whether tmux-resurrect (installed on target system with tmux-continuum auto-saving every 10 min) could fill this gap. Research found:

- Resurrect stores snapshots at `~/.local/share/tmux/resurrect/` as tab-delimited text files
- The `last` symlink points to the most recent save
- Files contain session names, window info, working directories, running commands
- Detection is possible via directory existence or `tmux list-keys | grep resurrect`

However, resurrect's restore is **all-or-nothing** — it restores the entire saved state, not individual sessions. This is fundamentally different from Zellij's model where exited sessions are individually addressable objects.

**Key realisation**: Resurrect is disaster recovery (machine crash, tmux server dies), not a session management workflow. Users don't interact with dead sessions — they either have running sessions or they don't. The "exited sessions" concept was a Zellij-specific feature that doesn't map to tmux's model.

#### Decision

**Drop the EXITED section entirely.**

- TUI shows only running sessions + new session option
- No resurrect integration — it's outside ZW's scope
- `zw clean` simplifies to only cleaning stale projects (directories that no longer exist on disk) — no "delete exited sessions" operation
- `zellij delete-session` has no tmux equivalent and is removed from the command mapping

---

### How should the layout system work with tmux?

#### Context

The Zellij spec included a layout picker during new session creation — users could choose from `.kdl` layout files. tmux has no equivalent single-file layout format.

#### Options Considered

**A. Drop layout selection entirely** — always create single-window sessions. Users split/arrange after attaching. Simplest path.

**B. tmux built-in layout strings** — `even-horizontal`, `main-vertical`, etc. Limited to pane arrangements, no commands per pane.

**C. Shell scripts as layouts** — small scripts that create windows/panes. How tmuxinator/smug work conceptually. Flexible but adds complexity.

**D. Structured layout config** — YAML/TOML files defining windows and panes. Essentially reimplementing tmuxinator inside ZW.

#### Decision

**A — Drop layouts completely.** Not "defer to later" — not needed.

The core use case is SSH from phone → pick session → get working. Fastest path to a session is the most valuable thing. Layout selection was already optional in the Zellij spec (skipped when no custom layouts existed).

This simplifies the new session flow significantly. When starting a session in a saved project with no other prompts needed, session creation is immediate — no layout picker step.

**Spec impact:**
- Remove Layout Discovery section
- Remove layout picker from new session flow
- Remove `--layout` flag from session creation command
- New session flow for saved projects becomes: select project → session created immediately

---

### Should the tool be renamed?

#### Context

"ZW" stands for "Zellij Workspaces" — no longer fits after switching to tmux.

#### Options Considered

**tmux-flavoured (2 chars):**
- `tx` — "tmux execute/extend". Clear tmux connection, no known CLI conflicts.
- `tw` — "tmux workspaces". Direct successor to "zw". Keys on different rows.
- `tm` — "tmux manager". Could be confused with Time Machine on macOS.

**z-prefix (familiar territory):**
- `zx` — Already appeared in project history. Adjacent bottom-row keys, fast. Google has an npm `zx` tool but it's a Node package — no real conflict.
- `zz` — Same key twice, fastest possible. No CLI conflicts. But no semantic meaning, and `zz` is a vim motion.

**Other:**
- `mux` — 3 chars but immediately clear. "I'm a multiplexer tool." Distinctive, no conflicts.
- `sx` — "session execute". Short but anonymous.

#### Journey

User wanted something that acknowledges tmux without hiding it. Considered keyboard ergonomics — already has `cx` for Claude Code, `c` for Composer. The `z` prefix had history from the Zellij era but no longer carried meaning.

`tx` was appealing for being 2 chars with clear tmux association. But `mux` stood out — instantly communicable, self-documenting, and can be aliased to anything shorter (e.g., `alias x=mux`) per user preference.

#### Decision

**Rename to `mux`.**

- Command: `mux`
- Config location: `~/.config/mux/`
- Homebrew formula: `mux`
- Repository: rename from `zw` to `mux`
- 3 chars is fine — user will alias to a single char if needed

**Spec impact:** Global find-replace of `zw` → `mux` throughout. All CLI commands become `mux`, `mux .`, `mux list`, `mux attach`, etc.

---

### How does utility mode work with tmux?

#### Context

The Zellij spec had a restricted "utility mode" when running inside Zellij (detected via `ZELLIJ` env var). Attaching and creating sessions were blocked to prevent nesting. Only rename, view, and kill were allowed.

#### Journey

tmux sets the `TMUX` env var inside sessions — same detection mechanism.

But tmux has `switch-client` (verified, alias: `switchc`):
```
tmux switch-client -t <session-name>
```

This switches the current client to a different session **without nesting**. It also supports `-l` for switching to the last session.

Furthermore, new sessions can be created detached (`new-session -d -s <name> -c <dir>`) and then switched to via `switch-client`. This means there's no technical reason to block *any* operation from inside tmux.

#### Decision

**Utility mode becomes a full session picker that switches instead of attaching.** No restrictions needed.

**Detection**: Check `TMUX` env var. If set, mux is running inside tmux.

**Behaviour when inside tmux:**
- `Enter` on a session → `tmux switch-client -t <name>` (instead of `exec tmux attach`)
- `[n] new in project...` → creates session detached (`new-session -d`), then `switch-client` to it
- Kill, rename — all work as normal (rename even works from outside now)
- Current session excluded from list (you're already in it)
- Header shows current session name for context

**Behaviour when outside tmux:**
- `Enter` on a session → `exec tmux attach-session -t <name>`
- `[n] new in project...` → `exec tmux new-session -A -s <name> -c <dir>`

**CLI commands inside tmux:**
- `mux .`, `mux <path>`, `mux <alias>` → create detached + switch-client (instead of blocking)
- `mux attach <name>` → switch-client (instead of blocking)

This is strictly better than the Zellij spec — the full TUI works from everywhere, just with different underlying operations.

**Spec impact:**
- Remove "Utility Mode" as a separate concept with restricted operations
- Replace with: "inside tmux" detection that changes attach → switch-client
- Remove the separate utility mode TUI mockup
- The TUI is the same everywhere, just the action differs

---

### What session metadata can we display from outside tmux?

#### Context

The Zellij spec showed tab count and attached indicator per session. Need to verify what tmux exposes.

#### Decision

tmux's `-F` format strings provide clean, structured access to all metadata. Verified on tmux 3.6a:

**`tmux list-sessions -F`** — available per session:
- `#{session_name}` — session name
- `#{session_windows}` — window count
- `#{session_attached}` — number of attached clients (0 = detached, 1+ = attached)
- `#{session_created}` — creation timestamp (unix epoch)

**`tmux list-windows -t <name> -F`** — available per window:
- `#{window_index}` — window number
- `#{window_name}` — window name
- `#{window_panes}` — pane count
- `#{window_active}` — 1 if active window

**`tmux list-clients -F`** — available per client:
- `#{client_session}` — which session the client is in
- `#{client_name}` — client tty

This is actually cleaner than Zellij — no ANSI escape stripping needed, no output format guessing. The `-F` flag gives us exact control over the output format with pipe-delimited fields.

**TUI display per session:**
- Session name
- Window count (e.g., `2 windows`) — equivalent to Zellij's tab count
- Attached indicator (`● attached`) when `session_attached > 0`

Identical to what the Zellij spec showed, just sourced differently.

---

### How does process handoff (exec) work with tmux?

#### Context

The Zellij spec used `exec` to replace mux's process with Zellij when launching a session. This avoided terminal state management — mux's job is done once the user selects.

#### Decision

**Same approach, works identically with tmux.**

From outside tmux (bare shell):
- `exec tmux new-session -A -s <name> -c <dir>` — replaces mux's process with tmux
- `exec tmux attach-session -t <name>` — same

From inside tmux:
- No `exec` needed — `switch-client` is a tmux command, not a process replacement
- For new sessions: `tmux new-session -d -s <name> -c <dir>` then `tmux switch-client -t <name>`
- mux exits normally after issuing the tmux commands

No design changes from the Zellij spec — the `exec` pattern carries forward for the outside-tmux case.

---

### What changes for the runtime dependency?

#### Decision

Straightforward swap:

- **Runtime dependency**: tmux (was Zellij)
- **Homebrew formula**: `depends_on "tmux"` (was `depends_on "zellij"`)
- **Missing dependency message**: "mux requires tmux. Install with: brew install tmux"
- **Detection**: Check if `tmux` is in `$PATH` at startup

tmux is more widely available than Zellij — likely already installed on most target systems. The Homebrew dependency ensures it regardless.

No design changes needed — same pattern, different binary name.

---

### Summary

#### What Changes

| Area | Zellij Spec | tmux Spec |
|---|---|---|
| **Name** | ZW (Zellij Workspaces) | mux |
| **Config location** | `~/.config/zw/` | `~/.config/mux/` |
| **Session operations** | `zellij attach -c`, `zellij list-sessions`, etc. | `tmux new-session -A -s`, `tmux list-sessions`, etc. |
| **Directory for new sessions** | cd to dir, then create | `new-session -c <dir>` — no cd needed |
| **Exited sessions** | EXITED section in TUI, resurrect, delete | Dropped entirely — tmux sessions are alive or gone |
| **Layouts** | KDL layout picker during session creation | Dropped entirely — not needed |
| **Inside-multiplexer mode** | Restricted "utility mode" — blocked attach/create | Full TUI, uses `switch-client` instead of attach |
| **Session info** | Query tab names, strip ANSI codes | `-F` format strings — clean, structured output |
| **Env var detection** | `ZELLIJ` | `TMUX` |
| **Rename from outside** | Not supported (inside only) | Supported via `rename-session -t` |
| **Runtime dependency** | Zellij | tmux |

#### What Stays Unchanged

- Go + Bubble Tea TUI
- Project memory (`projects.json` with aliases, names, timestamps)
- File browser for directory discovery
- Git root resolution
- CLI structure (`mux`, `mux .`, `mux <path>`, `mux <alias>`, `mux list`, `mux attach`, `mux clean`, `mux completion`)
- Session naming (project name + nanoid suffix)
- Filter mode, keyboard shortcuts
- Flat config format
- Argument resolution logic (path detection → alias lookup → fallback)
- Distribution (GoReleaser + Homebrew tap)
- `exec` process handoff (outside tmux)
- Mobile-first design philosophy

#### Net Effect

The tmux migration **simplifies** the tool:
- No exited sessions section
- No layout picker
- No restricted utility mode — full TUI everywhere
- No ANSI code stripping
- No cd before session creation
- Rename works from outside

#### Next Steps

1. Revise specification — update `docs/workflow/specification/zw.md` → new file for `mux`
2. Rename repository — `zw` → `mux`

## Research: tmux-session-managers-analysis, from discovery session 001

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

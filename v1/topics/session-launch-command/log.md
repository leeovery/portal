## Discussion

---
topic: session-launch-command
status: concluded
work_type: greenfield
date: 2026-02-12
---

## Discussion: Session Launch with Command Execution

### Context

When launching a new tmux session via `x <project>`, the user wants the ability to also execute a command inside that session after it's created. Current `cx` hardcodes running `claude` (or `claude --continue`, `claude --resume` depending on variant). The new `portal`/`x` design should generalise this — any command can be passed through, and the whole thing should be aliasable.

#### References

- [mux specification](../specification/mux.md)
- [x-xctl-split discussion](../x-xctl-split/log.md#discussion)
- [cx-design discussion](../cx-design/log.md#discussion)

### Questions

- [x] What CLI syntax for passing the command through?
- [x] Should the command replace the shell or run then drop to shell?
- [x] How does this interact with the TUI flow?
- [x] Should projects.json support default commands per project?
- [x] How does this relate to the existing cx variants (cx, cx c, cx r)?

---

### What CLI syntax for passing the command through?

#### Options Considered

**A) Double-dash separator**: `x myproject -- claude --resume`
- Standard POSIX convention — everything after `--` is not parsed as flags
- Handles compound commands with their own flags cleanly, no quoting
- Slightly more verbose

**B) Explicit flag**: `x myproject --exec "claude --resume"` / `x -e "claude --resume" myproject`
- Self-documenting
- Aliases naturally: `alias xc='x -e claude'` → `xc myproject` works
- Needs quoting for compound commands

**C) Positional after known args**: `x myproject claude --resume`
- Shortest but ambiguous — is `claude` a project or a command?
- Fragile, rejected immediately

#### Journey

Started with double-dash as the obvious POSIX choice. It handles arbitrary compound commands cleanly without quoting. But when exploring aliasing, a problem surfaced: with `--`, the project must come *before* the separator, so `alias xc='x -- claude'` then `xc myproject` expands to `x -- claude myproject` — `myproject` becomes an arg to claude, not to x.

Shell functions solve this (`xc() { x "${1:-.}" -- claude; }`) but that's a step up from simple aliases. The `--exec`/`-e` flag aliases naturally because the flag and its value stay together, with the project as a trailing positional.

Realised the two aren't mutually exclusive — both just express "run this command after session creation." Both normalise to the same internal representation: a `[]string` of command + args.

#### Decision

**Support both `-e` and `--`.** They're mutually exclusive (error if both provided). Both resolve to the same internal exec command slice early in CLI parsing. Single downstream code path.

- `-e` / `--exec` for simple commands and clean aliasing: `x -e claude myproject`
- `--` for compound commands with flags, no quoting: `x myproject -- claude --resume --model opus`

---

### Should the command replace the shell or run then drop to shell?

#### Options Considered

**A) Run then drop to shell** — command runs, when it exits you land in zsh at the project dir. Session stays alive.

**B) Replace shell (exec)** — command exits, session dies.

**C) User decides via flag** — default A, add `--replace` for B.

#### Journey

Current `cx` uses A: `zsh -ic '$cmd; exec zsh'`. If claude crashes or you ctrl+c, you don't lose the session. Option C adds surface area for a niche case — if someone really wants exec-and-die they can pass `exec claude` as the command itself.

#### Decision

**A — run then drop to shell.** No flag needed. Users wanting exec behaviour can literally pass `exec` as part of the command: `x myproject -- exec claude`.

---

### How does this interact with the TUI flow?

#### Context

When no project is specified, `x` opens the TUI picker. What happens when a command is specified but no project?

#### Options Considered

**A) TUI opens, command applies after selection** — the command is "sticky" through the selection flow. Pick project → session created → command runs.

**B) Error — require project when command specified.**

#### Journey

B would kill the alias use case. `alias xc='x -e claude'` then just `xc` to get picker + claude would fail. The command is an instruction for what to do *after* session creation, orthogonal to *how* the project was chosen (argument, alias, zoxide, TUI).

Confirmed this means `xc` (→ `x -e claude`) works consistently:
- `xc` → TUI → pick project → claude
- `xc myproject` → direct → claude
- `xc .` → current dir → claude

#### Decision

**A — command is orthogonal to project resolution.** TUI opens normally, command applies after selection. Consistent behaviour regardless of how the project was resolved.

---

### Should projects.json support default commands per project?

#### Journey

The aliasing approach already solves "I always want claude with this project" at the shell level. Adding per-project exec config in projects.json means more config surface, precedence rules (CLI vs config — which wins?), and potentially surprising behaviour if you forget a project has a default.

Also corrected a misunderstanding: aliases in the portal design are NOT properties of projects. They live in a separate flat map as independent routing shortcuts — decided in the x-xctl-split discussion.

#### Decision

**Skip — YAGNI.** Use the tool first, feel the friction, add config later if a real pattern emerges. Shell aliases/functions cover the known use cases without adding config surface prematurely.

---

### How does this relate to the existing cx variants (cx, cx c, cx r)?

#### Context

Current `cx` has hardcoded claude-specific subcommands:
- `cx` / `cx n` → new session + `claude`
- `cx c` → new session + `claude --continue`
- `cx r` → new session + `claude --resume`

#### Decision

**cx is fully replaced by portal/x. No claude-specific functionality in x.**

Portal is a command-agnostic session manager. It doesn't know or care about claude. The generalised exec mechanism (`-e` / `--`) makes the old hardcoded variants unnecessary at the tool level.

The user creates whatever personal shortcuts they want in their shell config:
```bash
alias xc='x -e claude'
alias xcc='x -e "claude --continue"'
alias xcr='x -e "claude --resume"'
```

Clean separation of concerns: x manages sessions, shell aliases encode personal workflows.

---

### Summary

#### Key Insights

1. Exec is orthogonal to project resolution — the command applies regardless of how the project was chosen (direct, alias, zoxide, TUI)
2. `-e` and `--` aren't competing syntaxes — they're complementary. `-e` for aliases, `--` for compound commands. Same internal code path.
3. Portal should be command-agnostic. Claude-specific shortcuts belong in user shell config, not baked into the tool.
4. "Run then drop to shell" is the safe default. `exec` as a command prefix is the escape hatch — no flag needed.

#### Current State

- All five questions resolved
- No open items

#### Next Steps

- [ ] Update mux specification to include exec functionality
- [ ] Remove claude-specific references from spec if any remain

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

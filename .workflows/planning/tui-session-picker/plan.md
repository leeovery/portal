---
topic: tui-session-picker
status: planning
format: tick
work_type: feature
ext_id: tick-dd35bb
specification: ../specification/tui-session-picker/specification.md
spec_commit: ed0a774efcfa406e84785f233fcc45d27d516848
created: 2026-02-28
updated: 2026-02-28
external_dependencies: []
task_list_gate_mode: auto
author_gate_mode: auto
finding_gate_mode: auto
review_cycle: 3
planning:
  phase: 3
  task: ~
---

# Plan: TUI Session Picker

### Phase 1: Sessions Page with bubbles/list
status: approved
approved_at: 2026-02-28
ext_id: tick-f20382

**Goal**: Replace the hand-rolled session list with a `bubbles/list`-based Sessions page, establishing the page architecture, modal overlay system, custom item delegate pattern, and all session-level actions (attach, kill, rename, filter, n-key). This phase also sets up the page-switching skeleton so Phase 2 can plug in the Projects page.

**Why this order**: The Sessions page is the default landing page and the most exercised path. Building it first proves the `bubbles/list` integration pattern, establishes the reusable modal overlay approach, and creates the page-switching infrastructure. All subsequent phases build on these patterns. It also removes the largest chunk of hand-rolled code (viewState enum, manual cursor, manual filter, inline rendering), de-risking the architectural replacement.

**Acceptance**:
- [ ] Sessions page renders via `bubbles/list` with custom `ItemDelegate` showing session name, window count, and attached badge
- [ ] `Enter` attaches to the selected session (sets `Selected()` and quits)
- [ ] `k` triggers kill confirmation modal overlay; `y` kills and refreshes list; `n`/`Esc` dismisses
- [ ] `r` triggers rename modal overlay with `textinput` pre-populated; `Enter` renames and refreshes; `Esc` dismisses
- [ ] Inside-tmux mode excludes current session from items and displays it in list title (`Sessions (current: {name})`)
- [ ] `/` activates `bubbles/list` built-in filtering
- [ ] Initial filter applied via `SetFilterText()` and `SetFilterState(list.FilterApplied)` on init
- [ ] `n` creates session in current working directory and exits TUI
- [ ] `Esc` progressive back: dismiss modal -> clear filter -> exit TUI
- [ ] `q` quits the TUI
- [ ] `p` key triggers page switch (Projects page can be a stub/empty-state page for now)
- [ ] Modal overlay renders styled content over list output via `lipgloss.Place()`
- [ ] Help bar displays context-appropriate keybindings
- [ ] Old `viewState` enum, hand-rolled session rendering, and manual cursor/filter logic are removed from the model

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| tui-session-picker-1-1 | Session List Item and Custom ItemDelegate | singular window pluralization, long session names, attached vs detached display | authored | tick-5d021f |
| tui-session-picker-1-2 | Sessions Page with bubbles/list Core | empty session list shows list empty state, SessionsMsg error triggers quit, inside-tmux with only current session | authored | tick-c64e34 |
| tui-session-picker-1-3 | Modal Overlay System and Kill Confirmation | kill last remaining session shows empty state, kill error triggers refresh | authored | tick-b29c05 |
| tui-session-picker-1-4 | Rename Modal with TextInput | empty rename input rejected, rename to same name, rename error | authored | tick-34ba3d |
| tui-session-picker-1-5 | N-Key New Session in CWD | session creation error, no session creator configured | authored | tick-01c27d |
| tui-session-picker-1-6 | Built-in Filtering and Initial Filter | initial filter with no matches, empty initial filter is no-op | authored | tick-b00429 |
| tui-session-picker-1-7 | Page-Switching Skeleton and Help Bar | switching to empty stub page, switching back preserves list state | authored | tick-fe1e6f |
| tui-session-picker-1-8 | Esc Progressive Back Behavior | Esc during rename modal vs kill modal, Esc with filter active but no modal | authored | tick-f7693a |
| tui-session-picker-1-9 | Remove Old Hand-Rolled Session Code | none | authored | tick-f08098 |

### Phase 2: Projects Page with bubbles/list
status: approved
approved_at: 2026-02-28
ext_id: tick-364f1a

**Goal**: Replace `ProjectPickerModel` with a `bubbles/list`-based Projects page including all project actions (new session on enter, edit, delete), custom item delegate, file browser integration, and complete two-way page navigation. Delete the old `ProjectPickerModel` and its tests.

**Why this order**: Depends on Phase 1's page-switching infrastructure, modal overlay system, and `bubbles/list` delegate pattern. This phase completes the second page of the two-page architecture, reusing the patterns Phase 1 established.

**Acceptance**:
- [ ] Projects page renders via `bubbles/list` with custom `ItemDelegate` showing project name and path
- [ ] `Enter` creates a new session in the selected project's directory and attaches (exits TUI)
- [ ] `e` triggers project edit modal overlay with name field, alias list, and full edit controls; `Enter` saves; `Esc` cancels
- [ ] `d` triggers delete confirmation modal overlay; `y` removes project and refreshes list; `n`/`Esc` dismisses
- [ ] `b` opens file browser sub-view; directory selection creates session and exits TUI; cancel returns to Projects page
- [ ] `s` navigates to Sessions page; `x` toggles between pages
- [ ] `n` creates session in current working directory from Projects page
- [ ] `Esc` progressive back: dismiss modal -> clear filter -> exit from browser -> exit TUI
- [ ] Help bar displays context-appropriate keybindings
- [ ] `ProjectPickerModel` (`internal/ui/projectpicker.go`) and `projectpicker_test.go` are deleted
- [ ] Empty projects page displays `bubbles/list` built-in empty message ("No saved projects")
- [ ] Independent filter state per page (switching pages does not carry filter text)

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| tui-session-picker-2-1 | Project List Item and Custom ItemDelegate | long project paths, projects with identical names | authored | tick-df51d2 |
| tui-session-picker-2-2 | Projects Page with bubbles/list Core | empty project list shows built-in empty message, project load error, session creation error | authored | tick-9184b3 |
| tui-session-picker-2-3 | Delete Confirmation Modal for Projects | delete last remaining project shows empty state, delete while filter active | authored | tick-5509f1 |
| tui-session-picker-2-4 | Project Edit Modal | empty name rejected, alias collision, alias removal, no editor configured | authored | tick-fd7b3f |
| tui-session-picker-2-5 | File Browser Integration from Projects Page | browser cancel returns to Projects page, Esc in browser returns to Projects page | authored | tick-f14aa6 |
| tui-session-picker-2-6 | Two-Way Page Navigation and Independent Filters | switching pages does not carry filter text, navigating to empty page shows empty message | authored | tick-9aebe2 |
| tui-session-picker-2-7 | Remove Old ProjectPickerModel | none | authored | tick-de5cb8 |

### Phase 3: Command-Pending Mode and Launch Defaults
status: approved
approved_at: 2026-02-28
ext_id: tick-68f174

**Goal**: Implement command-pending mode (TUI locked to Projects page with restricted keybindings and pending command display) and default page selection logic on launch. Wire up the `cmd/open.go` integration points.

**Why this order**: Both pages must be fully functional before command-pending mode can lock to one page and restrict its keybindings. Default page logic requires both pages to exist so it can choose between them based on session/project state.

**Acceptance**:
- [ ] `portal open -e cmd` launches TUI in command-pending mode locked to Projects page
- [ ] Command-pending mode: `s` and `x` keybindings are not registered (not shown in help bar)
- [ ] Command-pending mode: status line below title displays `Select project to run: {command}`
- [ ] Command-pending mode: `Enter` creates session with pending command
- [ ] Command-pending mode: `b` opens file browser; directory selection creates session with pending command
- [ ] Command-pending mode: `n` creates session in cwd with pending command
- [ ] Command-pending mode: `q` exits; `Esc` clears filter first if active, second `Esc` exits
- [ ] Command-pending mode help bar shows restricted keybindings
- [ ] Default page on launch: sessions exist -> Sessions page; no sessions -> Projects page; both empty -> Projects page
- [ ] Empty pages remain reachable via `p`/`s` navigation regardless of content state
- [ ] Initial filter (`--filter` flag) applied to whichever page is the default
- [ ] `cmd/open.go` wiring updated to pass command and filter to the new model API

#### Tasks
| ID | Name | Edge Cases | Status | Ext ID |
|----|------|------------|--------|--------|
| tui-session-picker-3-1 | Default Page Selection on Launch | both pages empty defaults to Projects, sessions exist but all filtered by inside-tmux | authored | tick-2f0ec0 |
| tui-session-picker-3-2 | Command-Pending Mode Core | pressing s/x does nothing, page-switch keys absent from help bar | authored | tick-310db8 |
| tui-session-picker-3-3 | Command-Pending Status Line and Help Bar | long command text, multi-word commands | authored | tick-f8d97a |
| tui-session-picker-3-4 | Command-Pending Enter Creates Session with Command | session creation error, empty project list | authored | tick-e8fd08 |
| tui-session-picker-3-5 | Command-Pending Browse and N-Key with Command | browser cancel returns to locked Projects page, n-key in cwd with command | authored | tick-c5bbbb |
| tui-session-picker-3-6 | Command-Pending Esc and Quit Behavior | Esc with filter active needs two presses, Esc with modal active dismisses modal first | authored | tick-5c4639 |
| tui-session-picker-3-7 | Initial Filter Applied to Default Page | initial filter with no matches, empty initial filter is no-op, filter applied to Projects when no sessions | authored | tick-bd640d |
| tui-session-picker-3-8 | Wire cmd/open.go to New Model API | no command no filter passthrough, command with filter combined | authored | tick-dc682b |

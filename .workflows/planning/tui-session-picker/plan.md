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
task_list_gate_mode: gated
author_gate_mode: auto
finding_gate_mode: gated
planning:
  phase: 1
  task: 4
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
| tui-session-picker-1-4 | Rename Modal with TextInput | empty rename input rejected, rename to same name, rename error | pending | |
| tui-session-picker-1-5 | N-Key New Session in CWD | session creation error, no session creator configured | pending | |
| tui-session-picker-1-6 | Built-in Filtering and Initial Filter | initial filter with no matches, empty initial filter is no-op | pending | |
| tui-session-picker-1-7 | Page-Switching Skeleton and Help Bar | switching to empty stub page, switching back preserves list state | pending | |
| tui-session-picker-1-8 | Esc Progressive Back Behavior | Esc during rename modal vs kill modal, Esc with filter active but no modal | pending | |
| tui-session-picker-1-9 | Remove Old Hand-Rolled Session Code | none | pending | |

### Phase 2: Projects Page with bubbles/list
status: approved
approved_at: 2026-02-28
ext_id:

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

### Phase 3: Command-Pending Mode and Launch Defaults
status: approved
approved_at: 2026-02-28
ext_id:

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

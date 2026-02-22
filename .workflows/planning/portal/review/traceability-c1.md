---
status: in-progress
created: 2026-02-22
cycle: 1
phase: Traceability Review
topic: Portal
---

# Review Tracking: Portal - Traceability

## Findings

### 1. Project Picker Filter Mode Not Covered

**Type**: Missing from plan
**Spec Reference**: Project Picker Interaction -- keyboard shortcuts table (/ key: "Enter filter mode (fuzzy-matches against project name)")
**Plan Reference**: N/A -- no task exists
**Change Type**: add-to-task

**Details**:
The specification defines a `/` keyboard shortcut in the project picker that activates filter mode for fuzzy-matching against project names. This is explicitly listed in the Project Picker Interaction keyboard shortcuts table. No task in the plan implements this. Phase 5 filter mode tasks (portal-5-7, portal-5-8) are scoped exclusively to the session list. The project picker (portal-2-6) mentions navigation but not filter mode.

**Current**:
From portal-2-6 (tick-4c54e1) acceptance criteria:
```
- [ ] Displays projects sorted by last_used
- [ ] browse option always visible at bottom
- [ ] Arrow keys and j/k navigate list
- [ ] Enter on project returns selection
- [ ] Enter on browse returns browse action
- [ ] Esc returns to session list
- [ ] Empty state with browse still selectable
```

**Proposed**:
Updated portal-2-6 (tick-4c54e1) acceptance criteria:
```
- [ ] Displays projects sorted by last_used
- [ ] browse option always visible at bottom
- [ ] Arrow keys and j/k navigate list
- [ ] Enter on project returns selection
- [ ] Enter on browse returns browse action
- [ ] Esc returns to session list
- [ ] Empty state with browse still selectable
- [ ] / activates filter mode; typing fuzzy-matches against project names
- [ ] Filter mode: [n]/browse option always visible regardless of filter
- [ ] Filter mode: Backspace removes last char; on empty filter exits filter mode
- [ ] Filter mode: Esc clears filter and exits filter mode
```

Updated portal-2-6 (tick-4c54e1) tests -- add:
```
- slash activates filter mode in project picker
- typing narrows project list by fuzzy match
- browse option always visible during filter
- backspace on empty filter exits filter mode
- esc clears filter and exits filter mode
```

**Resolution**: Pending
**Notes**:

---

### 2. TUI Fallback with Pre-Filled Session List Filter Not Covered

**Type**: Missing from plan
**Spec Reference**: Query Resolution -- "No match: Fall back to the TUI with the query pre-filled as the filter text. If a command is pending (-e/--), this opens the project picker; otherwise, the main session picker."
**Plan Reference**: portal-4-5 (tick-fe7c90) -- Query Resolution Chain
**Change Type**: update-task

**Details**:
When `x <query>` fails to resolve via path, alias, or zoxide, the spec requires falling back to the TUI with the query pre-filled as filter text. Task portal-6-3 covers the command-pending case (project picker with pre-filled filter), but the normal case (no command pending, session list with pre-filled filter) is not covered. Task portal-4-5 mentions "TUI fallback" in a single line but does not describe pre-filling the filter in the session list.

**Current**:
From portal-4-5 (tick-fe7c90) full description:
```
QueryResolver: path detection -> alias -> zoxide -> TUI fallback. Directory validation after resolution.
```

**Proposed**:
Updated portal-4-5 (tick-fe7c90) full description:
```
**Problem**: Portal needs a unified query resolution chain for `portal open <query>`. The argument must be resolved through a priority chain: path detection, alias lookup, zoxide query, then TUI fallback. After resolution, the directory must be validated.

**Solution**: Implement QueryResolver that applies the resolution chain in order. Path detection uses a heuristic: arguments containing `/` or starting with `.` or `~` are treated as paths. Non-path arguments check alias store, then zoxide, then fall back to TUI with the query pre-filled as filter text.

**Outcome**: `portal open <query>` resolves through the full chain. Resolved directories are validated. Unresolved queries fall back to the TUI with the query pre-filled as filter text in the session list (or project picker if command pending).

**Do**:
- Create internal/resolver/query.go with QueryResolver struct
- IsPathArgument(arg): true if contains `/` or starts with `.` or `~`
- Resolve(query): path detection -> alias lookup -> zoxide query -> TUI fallback
- After alias or zoxide resolution, validate directory exists on disk
- If directory not found after resolution, print "Directory not found: {path}" and exit 1
- On TUI fallback: return a FallbackResult containing the query string for pre-filling the filter
- Wire into portal open: when FallbackResult returned, launch TUI with query as initial filter text in session list

**Acceptance Criteria**:
- [ ] Path-like arguments (containing `/`, starting with `.` or `~`) resolved directly as paths
- [ ] Non-path arguments checked against alias store first
- [ ] Alias miss falls through to zoxide query
- [ ] Zoxide miss (or not installed) falls through to TUI fallback
- [ ] TUI fallback pre-fills the query as filter text in the session list
- [ ] Resolved directory validated; "Directory not found: {path}" and exit 1 if missing
- [ ] Zoxide skipped silently when not installed

**Tests**:
- path-like argument resolved directly (contains /)
- path-like argument resolved directly (starts with .)
- path-like argument resolved directly (starts with ~)
- non-path argument resolved via alias
- alias miss falls through to zoxide
- zoxide miss falls through to TUI fallback
- TUI fallback includes query string for filter pre-fill
- resolved directory validated for existence
- non-existent resolved directory prints error and exits 1
- zoxide not installed skipped silently
- query matches alias; alias path used (not zoxide)

**Edge Cases**:
- Query matches both alias and zoxide: alias wins (alias is checked first)
- Resolved directory does not exist: error message and exit 1
- Path-like query (e.g., ./mydir): treated as path, not sent through alias/zoxide chain

**Spec Reference**: .workflows/specification/portal/specification.md -- Query Resolution section
```

**Resolution**: Pending
**Notes**:

---

### 3. Phase 3 Tasks Have Incomplete Descriptions -- Insufficient Implementation Detail

**Type**: Incomplete coverage
**Spec Reference**: File Browser section, CLI Interface section (x . and x <path>)
**Plan Reference**: Phase 3 tasks portal-3-1 through portal-3-7 (tick-f26ed6, tick-62c4f1, tick-cebbcb, tick-feea91, tick-9d6404, tick-33fb7f, tick-afb187)
**Change Type**: update-task

**Details**:
All seven Phase 3 tasks have severely truncated descriptions that omit the required task template fields (Problem, Solution, Outcome). Several also lack Acceptance Criteria and have minimal Do sections. An implementer would need to go back to the specification to understand what to build. The spec has extensive detail on file browser behavior (directory-only listing, navigation keys, filter behavior, hidden directory toggle, selection via Space/Enter on dot entry, starting directory as cwd) and CLI quick-start paths (path resolution, tilde expansion, validation, git root resolution, project registration, exec handoff) that is not reflected in these task descriptions.

**Current**:
portal-3-1 (tick-f26ed6):
```
ListDirectories(path, showHidden) returns []DirEntry (directories only). Handles empty dirs, permission denied, symlinks.

**Do**:
- Create internal/browser/listing.go
- Read directory entries, filter to directories only
- Option to include/exclude hidden (. prefix)
- Sort alphabetically
- Return empty slice on permission denied

**Tests**:
- returns only directories
- excludes hidden when showHidden=false
- includes hidden when showHidden=true
- returns empty for empty directory
- handles permission denied gracefully
```

portal-3-2 (tick-62c4f1):
```
Bubble Tea FileBrowserModel for directory navigation. Enter/right descends, Backspace/left ascends. Shows . entry for current dir.

**Do**:
- Create internal/ui/browser.go
- Init with starting path (cwd)
- Render: current path, dir listing with cursor, . entry at top
- Enter/right on dir: descend into it
- Backspace/left: go to parent (no-op at root)
- Up/Down/j/k: navigate listing

**Tests**:
- displays current path
- enter descends into directory
- backspace goes to parent
- no-op at root directory
- cursor navigation works
```

portal-3-3 (tick-cebbcb):
```
Typing narrows listing by fuzzy match. Backspace: remove char, or navigate to parent if filter empty. Esc: clear filter, or cancel browser if no filter.

**Do**:
- Add filter state to browser model
- Typing adds to filter string, narrows visible entries
- Backspace on non-empty filter removes char
- Backspace on empty filter goes to parent
- Esc clears filter if active, cancels browser if not
- Filter resets when entering a subdirectory

**Tests**:
- typing filters directory listing
- backspace removes filter char
- backspace on empty filter goes to parent
- esc clears active filter
- esc cancels browser when no filter
- filter resets on directory change
```

portal-3-4 (tick-feea91):
```
. key toggles hidden dir visibility. Space/Enter on . entry selects current directory for session creation.

**Do**:
- . key toggles showHidden flag (only when not filtering)
- Space on . entry: select current dir, emit selection message
- Enter on . entry: same as Space
- Validate directory exists before selection
- Selection triggers auto-add to projects.json (via integration)

**Tests**:
- dot key toggles hidden visibility
- space on dot entry emits selection
- enter on dot entry emits selection
- selection message contains current path
- dot key ignored when filtering
```

portal-3-5 (tick-9d6404):
```
Wire browse for directory option to file browser. Selection flows through git root, project registration, session creation.

**Do**:
- Project picker: browse option switches to file browser view
- Browser selection: resolve git root, register project, create session
- Browser cancel (Esc): return to project picker
- Works from empty project list (browse is only option)

**Tests**:
- browse option opens file browser
- selection creates session with resolved path
- cancel returns to project picker
- browse works from empty project list
```

portal-3-6 (tick-33fb7f):
```
portal open . and portal open <path> resolve directory without TUI. Validate existence, expand tilde, resolve relative paths.

**Do**:
- ResolvePath(arg): expand ~, resolve relative to absolute, validate
- IsPathArgument(arg): true if contains / or starts with . or ~
- Wire into portal open: if path arg, resolve and skip TUI
- Non-existent: print "Directory not found: {path}", exit 1
- Is file not dir: print error, exit 1

**Tests**:
- resolves relative path to absolute
- expands tilde to home directory
- returns error for non-existent path
- returns error when path is a file
- portal open . resolves cwd
```

portal-3-7 (tick-afb187):
```
Wire resolved path through git root, project registration, session naming, tmux creation, exec handoff.

**Do**:
- QuickStart(path): resolve git root -> register/update project -> generate name -> tmux new-session -> exec
- Reuse Phase 2 primitives (GitResolver, ProjectStore, SessionCreator)
- Update last_used if project already exists
- exec handoff same as TUI attach path

**Tests**:
- creates session with git-root-resolved directory
- registers new project in store
- updates last_used for existing project
- exec replaces process with tmux
```

**Proposed**:
portal-3-1 (tick-f26ed6):
```
**Problem**: The file browser needs a directory listing function that returns only directories, with optional hidden directory filtering. This is the data layer for the file browser TUI.

**Solution**: Implement ListDirectories(path, showHidden) returning a sorted slice of directory entries. Files are excluded. Hidden directories (starting with `.`) are excluded unless showHidden is true.

**Outcome**: Tested function returning directory-only entries, handling empty dirs, permission denied, and symlinks gracefully.

**Do**:
- Create internal/browser/listing.go with DirEntry struct (Name string, IsSymlink bool)
- ListDirectories(path string, showHidden bool) ([]DirEntry, error)
- Read directory entries via os.ReadDir, filter to directories only (files not displayed per spec)
- Exclude entries starting with `.` when showHidden is false
- Sort entries alphabetically by name
- Return empty slice (not error) on permission denied
- Follow symlinks to check if target is a directory

**Acceptance Criteria**:
- [ ] Returns only directory entries, no files
- [ ] Hidden directories excluded when showHidden is false
- [ ] Hidden directories included when showHidden is true
- [ ] Entries sorted alphabetically
- [ ] Returns empty slice on permission denied
- [ ] Symlinked directories included; symlinked files excluded
- [ ] Returns empty slice for empty directory

**Tests**:
- returns only directories
- excludes hidden when showHidden=false
- includes hidden when showHidden=true
- returns empty for empty directory
- handles permission denied gracefully
- sorts entries alphabetically
- includes symlinked directories
- excludes symlinked files

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Content: shows directories only)
```

portal-3-2 (tick-62c4f1):
```
**Problem**: Portal needs an interactive file browser TUI for navigating the filesystem to select a directory for new session creation. The browser must support descending into directories, ascending to parent, and showing a `.` entry to select the current directory.

**Solution**: Implement a Bubble Tea FileBrowserModel that starts at the current working directory, renders the directory listing with cursor navigation, and supports Enter/right-arrow to descend and Backspace/left-arrow to ascend.

**Outcome**: Interactive file browser TUI with directory navigation. Enter/right descends, Backspace/left ascends, `.` entry at top represents current directory. Navigation stops at root.

**Do**:
- Create internal/ui/browser.go with FileBrowserModel struct
- Init with starting path set to current working directory (per spec: "Starting directory: Current working directory")
- Render: current path header, `.` entry at top (current dir indicator), then directory listing with cursor
- Enter or right-arrow on a directory: descend into it, refresh listing
- Backspace or left-arrow: go to parent directory; no-op at filesystem root
- Up/Down and j/k: navigate the listing
- Cursor resets to 0 when entering a new directory

**Acceptance Criteria**:
- [ ] Browser starts at current working directory
- [ ] Current path displayed as header
- [ ] `.` entry shown at top of listing
- [ ] Enter on directory descends into it
- [ ] Right-arrow on directory descends into it
- [ ] Backspace goes to parent directory
- [ ] Left-arrow goes to parent directory
- [ ] Navigation is no-op at filesystem root
- [ ] Up/Down and j/k navigate listing
- [ ] Cursor resets when entering subdirectory

**Tests**:
- displays current path
- shows dot entry at top
- enter descends into directory
- right arrow descends into directory
- backspace goes to parent
- left arrow goes to parent
- no-op at root directory
- cursor navigation works
- cursor resets on directory change
- starts at cwd

**Edge Cases**:
- Root directory: Backspace/left-arrow are no-ops
- Single subdirectory: cursor stays at 0
- Deeply nested path: header shows full path

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Behavior) section
```

portal-3-3 (tick-cebbcb):
```
**Problem**: The file browser needs inline filtering so users can quickly narrow the directory listing by typing. The spec defines dual-purpose Backspace (filter char removal or parent navigation) and dual-purpose Esc (clear filter or cancel browser).

**Solution**: Add filter state to the browser model. Typing narrows visible entries by fuzzy match at the current directory level. Backspace removes the last filter character; when filter is empty, Backspace reverts to navigation (go to parent). Esc clears active filter; if no filter is active, Esc cancels the browser.

**Outcome**: Typing fuzzy-filters directory listing. Backspace and Esc have context-sensitive behavior based on filter state.

**Do**:
- Add filterText string to FileBrowserModel
- Rune keypresses: append to filterText, recompute visible entries by fuzzy match
- Backspace: if filterText non-empty, remove last char and re-filter; if filterText empty, go to parent directory
- Esc: if filterText non-empty, clear filterText and show full listing; if filterText empty, cancel browser (emit cancel message)
- Filter resets (filterText = "") when entering a subdirectory via Enter/right-arrow
- Cursor resets to 0 when filter changes

**Acceptance Criteria**:
- [ ] Typing narrows directory listing by fuzzy match
- [ ] Backspace removes last filter character when filter active
- [ ] Backspace goes to parent when filter is empty
- [ ] Esc clears filter when filter is active
- [ ] Esc cancels browser when no filter is active
- [ ] Filter resets when entering a subdirectory
- [ ] Cursor resets when filter changes

**Tests**:
- typing filters directory listing
- backspace removes filter char
- backspace on empty filter goes to parent
- esc clears active filter
- esc cancels browser when no filter
- filter resets on directory change
- cursor resets on filter change
- filter matches nothing shows empty listing

**Edge Cases**:
- Filter matches nothing: empty listing shown, no directories selectable
- Filter then navigate to child: filter resets
- All characters deleted via Backspace: filter empty, full listing restored

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Filtering) section
```

portal-3-4 (tick-feea91):
```
**Problem**: The file browser needs two additional features: toggling hidden directory visibility and selecting the current directory for session creation. Hidden directories (`.` prefix) are hidden by default and toggled with the `.` key. The current directory is selected via Space or Enter on the `.` entry.

**Solution**: Add a showHidden toggle activated by the `.` key (only outside filter mode to avoid conflict with typing). Space or Enter on the `.` entry at the top of the listing selects the current directory, emitting a selection message. The toggle is session-scoped (resets on next browser open).

**Outcome**: `.` key toggles hidden directory visibility. Space/Enter on `.` entry selects current directory for session creation.

**Do**:
- `.` key toggles showHidden flag and refreshes listing (only when not in filter mode, to avoid conflict with typing `.` as filter text)
- Space on `.` entry: emit selection message with current directory path
- Enter on `.` entry: same behavior as Space
- Validate directory still exists before emitting selection
- Selection message carries the path for downstream processing (git root resolution, project registration, session creation)
- Toggle applies only to current browser session; resets on next open

**Acceptance Criteria**:
- [ ] `.` key toggles hidden directory visibility
- [ ] Hidden directories shown after toggle
- [ ] Hidden directories hidden again after second toggle
- [ ] Space on `.` entry emits directory selection
- [ ] Enter on `.` entry emits directory selection
- [ ] Selection message contains current directory path
- [ ] `.` key ignored when filter is active
- [ ] Toggle resets on next browser open
- [ ] Selected directory auto-added to projects.json (via integration)

**Tests**:
- dot key toggles hidden visibility
- space on dot entry emits selection
- enter on dot entry emits selection
- selection message contains current path
- dot key ignored when filtering
- directory has only hidden subdirectories and toggle reveals them
- selected directory removed between browse and select produces error

**Edge Cases**:
- Directory has only hidden subdirectories: listing appears empty until `.` toggle
- Selected directory removed between browse and select: error at selection time

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Hidden directories, Select current directory) section
```

portal-3-5 (tick-9d6404):
```
**Problem**: The file browser must be wired into the project picker flow. When the user selects "browse for directory..." in the project picker, the file browser opens. A directory selection flows through git root resolution, project registration, and session creation. Cancelling returns to the project picker.

**Solution**: Connect the project picker's browse option to the file browser. On selection, resolve git root, register the project in projects.json, create the tmux session, and exec/switch. On cancel (Esc with no filter), return to the project picker view.

**Outcome**: End-to-end flow from project picker browse option through file browser to session creation. Cancel returns to project picker.

**Do**:
- In project picker: Enter on browse option transitions to file browser view
- File browser selection: resolve directory to git root, upsert project in store, create session (reuse Phase 2 SessionCreator)
- File browser cancel (Esc with no filter): transition back to project picker view
- Works from empty project list (browse is the only selectable option)

**Acceptance Criteria**:
- [ ] Browse option in project picker opens file browser
- [ ] Selected directory resolved to git root before session creation
- [ ] Selected directory registered in projects.json
- [ ] Session created with auto-generated name and resolved directory
- [ ] Cancel in browser returns to project picker
- [ ] Browse works from empty project list (only browse option available)

**Tests**:
- browse option opens file browser
- selection creates session with git-root-resolved path
- selection registers project in store
- cancel returns to project picker
- browse works from empty project list

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Access, Behavior), Project Memory (How Directories are Added)
```

portal-3-6 (tick-33fb7f):
```
**Problem**: Portal needs non-interactive quick-start paths (`portal open .` and `portal open <path>`) that resolve a directory and create a session without launching the TUI. Paths must be validated, expanded, and resolved.

**Solution**: Implement ResolvePath for tilde expansion and relative-to-absolute conversion. Implement IsPathArgument to detect path-like arguments. Wire into `portal open`: if the argument is a path, resolve it and skip the TUI. Validate the resolved path exists and is a directory.

**Outcome**: `portal open .` resolves cwd, `portal open <path>` resolves the given path. Non-existent paths print error and exit 1. No TUI is launched.

**Do**:
- Create internal/resolver/path.go with ResolvePath(arg string) (string, error)
- Expand `~` to user home directory
- Resolve relative paths to absolute using filepath.Abs
- Validate path exists (os.Stat); if not, return error with "Directory not found: {path}"
- Validate path is a directory (not a file); if file, return error
- IsPathArgument(arg string) bool: true if arg contains `/` or starts with `.` or `~`
- Wire into cmd/open.go: check IsPathArgument; if true, call ResolvePath and skip TUI, proceed to session creation

**Acceptance Criteria**:
- [ ] `portal open .` resolves current working directory
- [ ] `portal open <relative-path>` resolves to absolute path
- [ ] `portal open ~/path` expands tilde to home directory
- [ ] Non-existent path prints "Directory not found: {path}" and exits with code 1
- [ ] Path that is a file (not directory) prints error and exits with code 1
- [ ] TUI is not launched for path arguments

**Tests**:
- resolves relative path to absolute
- expands tilde to home directory
- returns error for non-existent path
- returns error when path is a file
- portal open . resolves cwd
- IsPathArgument true for paths with /
- IsPathArgument true for paths starting with .
- IsPathArgument true for paths starting with ~
- IsPathArgument false for plain words

**Spec Reference**: .workflows/specification/portal/specification.md -- x (The Launcher, Quick-start shortcuts), Query Resolution (Path detection heuristic, Path validation)
```

portal-3-7 (tick-afb187):
```
**Problem**: After path resolution, Portal must orchestrate the full session creation flow: git root resolution, project registration, session name generation, tmux session creation, and process handoff. This wires the CLI quick-start paths through the existing Phase 2 primitives.

**Solution**: Implement a QuickStart function that takes a resolved path and executes the full creation pipeline. Reuse GitResolver, ProjectStore, and SessionCreator from Phase 2. Update last_used if the project already exists in the store.

**Outcome**: `portal open .` and `portal open <path>` create a tmux session in the resolved directory with git root resolution, project registration, and exec handoff.

**Do**:
- Implement QuickStart(path string) in cmd/open.go or internal/session/
- Steps: resolve git root -> derive project name from basename -> upsert project (add or update last_used) -> generate session name -> tmux new-session -A -s <name> -c <dir> -> exec handoff
- Reuse Phase 2 primitives: GitResolver, ProjectStore.Upsert, SessionCreator
- Update last_used timestamp if project already exists in projects.json
- Exec handoff replaces Portal process with tmux (same as TUI attach path)

**Acceptance Criteria**:
- [ ] Resolved path goes through git root resolution
- [ ] New project registered in projects.json with name from directory basename
- [ ] Existing project's last_used timestamp updated
- [ ] Session name auto-generated ({project}-{nanoid})
- [ ] tmux session created with resolved directory via -c flag
- [ ] Portal process replaced by tmux via exec

**Tests**:
- creates session with git-root-resolved directory
- registers new project in store
- updates last_used for existing project
- exec replaces process with tmux
- session name follows {project}-{nanoid} format
- project name derived from directory basename after git root resolution

**Edge Cases**:
- cwd is inside git repo subdirectory: resolved to git root, project name from root basename
- Project already in projects.json: last_used updated, no duplicate entry

**Spec Reference**: .workflows/specification/portal/specification.md -- Directory Change for New Sessions, Git Root Resolution, How Directories are Added, Session Naming, Process Handoff
```

**Resolution**: Pending
**Notes**: All seven Phase 3 tasks lack the required Problem, Solution, Outcome fields and have insufficient detail for implementation without referencing the specification. The proposed content adds these fields and pulls forward the relevant spec details.

---

### 4. Phase 4 Tasks Have Incomplete Descriptions -- Insufficient Implementation Detail

**Type**: Incomplete coverage
**Spec Reference**: Alias Storage, CLI Interface (xctl alias commands), Query Resolution, Shell Functions (portal init), File Browser (alias shortcut)
**Plan Reference**: Phase 4 tasks portal-4-1 through portal-4-9 (tick-5533b4, tick-a2e7bd, tick-26cdeb, tick-b3b992, tick-fe7c90, tick-83fe89, tick-457428, tick-19ad25, tick-b92526)
**Change Type**: update-task

**Details**:
All nine Phase 4 tasks have severely truncated descriptions -- most are one or two lines with no structured fields. They lack Problem, Solution, Outcome, Do, Acceptance Criteria, and Tests sections. An implementer would need to go back to the specification for alias storage format, path normalization rules, shell init output format, zoxide integration details, and query resolution chain logic. The spec has extensive detail on all of these that is not reflected in the task descriptions.

**Current**:
portal-4-1 (tick-5533b4):
```
AliasStore type managing ~/.config/portal/aliases flat key=value file. Load/Save/Get/Set/Delete/List operations.

**Do**: Create internal/alias/store.go. Handle missing file/dir, empty file, duplicate keys (last wins).
```

portal-4-2 (tick-a2e7bd):
```
portal alias set <name> <path> command. NormalisePath expands tilde and resolves relative to absolute.
```

portal-4-3 (tick-26cdeb):
```
portal alias rm <name> (exit 1 if not found) and portal alias list (sorted, empty output when none).
```

portal-4-4 (tick-b3b992):
```
ZoxideResolver wrapping zoxide query. Graceful skip when not installed. ErrZoxideNotInstalled and ErrNoMatch sentinel errors.
```

portal-4-5 (tick-fe7c90):
```
QueryResolver: path detection -> alias -> zoxide -> TUI fallback. Directory validation after resolution.
```

portal-4-6 (tick-83fe89):
```
portal init zsh emits x()/xctl() functions + Cobra completions + compdef wiring.
```

portal-4-7 (tick-457428):
```
--cmd flag customises function names. portal init zsh --cmd p emits p()/pctl().
```

portal-4-8 (tick-19ad25):
```
portal init bash and portal init fish emit shell-specific functions and completions. Unsupported shell returns exit 2.
```

portal-4-9 (tick-b92526):
```
a key in file browser prompts for alias name, git-root-resolves directory, saves alias. No session started.
```

**Proposed**:
portal-4-1 (tick-5533b4):
```
**Problem**: Portal needs persistent alias storage in a flat key=value file at ~/.config/portal/aliases. Aliases map short names to directory paths for quick navigation.

**Solution**: Implement AliasStore with Load/Save/Get/Set/Delete/List operations managing the aliases file. Handle missing file, missing config directory, empty file, and duplicate keys gracefully.

**Outcome**: Tested AliasStore that reads and writes ~/.config/portal/aliases in flat key=value format with full CRUD operations.

**Do**:
- Create internal/alias/store.go with AliasStore struct
- File format: one alias per line, `name=path` (e.g., `m2api=/Users/lee/Code/mac2/api`)
- Load(): read file, parse lines, handle missing file (return empty), handle empty file (return empty), handle duplicate keys (last wins)
- Save(): create config directory if needed, write all aliases to file
- Get(name string) (string, bool): return path for alias name
- Set(name, path string): add or overwrite alias (aliases must be unique per spec)
- Delete(name string) bool: remove alias, return whether it existed
- List() []Alias: return all aliases sorted by name

**Acceptance Criteria**:
- [ ] Load returns empty map when file does not exist
- [ ] Load returns empty map for empty file
- [ ] Load handles duplicate keys (last value wins)
- [ ] Save creates ~/.config/portal/ directory if needed
- [ ] Set adds new alias
- [ ] Set overwrites existing alias (uniqueness enforced)
- [ ] Delete removes alias and returns true; returns false if not found
- [ ] List returns all aliases sorted by name
- [ ] File format is flat key=value, one per line

**Tests**:
- loads empty map when file does not exist
- loads aliases from valid file
- handles duplicate keys (last wins)
- creates config directory on save
- set adds new alias
- set overwrites existing alias
- delete removes existing alias
- delete returns false for non-existent alias
- list returns sorted aliases
- handles empty file

**Context**:
> Per spec: "Aliases are stored separately from projects in ~/.config/portal/aliases, using a flat key-value format." Aliases must be unique -- each name maps to exactly one path.

**Spec Reference**: .workflows/specification/portal/specification.md -- Alias Storage section
```

portal-4-2 (tick-a2e7bd):
```
**Problem**: Portal needs a `portal alias set <name> <path>` CLI command that stores an alias with path normalization. Paths must have tilde expanded and relative paths resolved to absolute before storage.

**Solution**: Implement the Cobra command for `portal alias set`. Normalize the path (expand `~`, resolve relative to absolute) before storing via AliasStore. The aliases file always contains absolute paths per spec.

**Outcome**: `portal alias set m2api ~/Code/mac2/api` stores the alias with the fully resolved absolute path.

**Do**:
- Create cmd/alias.go with `portal alias` parent command and `portal alias set` subcommand
- Accept exactly two positional arguments: name and path
- NormalisePath(path string) string: expand `~` to os.UserHomeDir(), resolve relative paths to absolute via filepath.Abs()
- Call AliasStore.Set(name, normalisedPath)
- Save the aliases file after setting
- If overwriting an existing alias, do so silently (spec: setting existing alias overwrites)

**Acceptance Criteria**:
- [ ] `portal alias set <name> <path>` stores alias
- [ ] Tilde expanded to home directory before storage
- [ ] Relative paths resolved to absolute before storage
- [ ] Aliases file always contains absolute paths
- [ ] Setting existing alias overwrites silently
- [ ] Exit 0 on success

**Tests**:
- sets new alias with absolute path
- expands tilde in path
- resolves relative path to absolute
- overwrites existing alias silently
- aliases file contains absolute path after set

**Spec Reference**: .workflows/specification/portal/specification.md -- Alias Storage (Path normalization), xctl alias commands
```

portal-4-3 (tick-26cdeb):
```
**Problem**: Portal needs `portal alias rm <name>` and `portal alias list` CLI commands for alias management.

**Solution**: Implement Cobra subcommands for alias removal and listing. Remove prints error and exits 1 if alias not found. List outputs all aliases sorted; empty output when none.

**Outcome**: Complete alias management CLI: set, remove, and list operations.

**Do**:
- Add `portal alias rm` subcommand: accept one positional argument (name)
- If alias not found, print error to stderr and exit 1
- If alias found, delete and save; exit 0
- Add `portal alias list` subcommand: no arguments
- List all aliases in format `name=path`, sorted by name
- Empty output (nothing printed) when no aliases exist; exit 0

**Acceptance Criteria**:
- [ ] `portal alias rm <name>` removes alias and exits 0
- [ ] `portal alias rm <name>` prints error and exits 1 when alias not found
- [ ] `portal alias list` outputs all aliases sorted by name
- [ ] `portal alias list` produces empty output when no aliases
- [ ] Both commands exit 0 on success

**Tests**:
- rm removes existing alias
- rm prints error for non-existent alias
- rm exits 1 for non-existent alias
- list outputs aliases sorted by name
- list produces empty output when no aliases
- list exits 0

**Spec Reference**: .workflows/specification/portal/specification.md -- xctl alias commands, Alias Storage
```

portal-4-4 (tick-b3b992):
```
**Problem**: Portal needs zoxide integration as an optional step in query resolution. Zoxide provides frecency-based directory matching but may not be installed.

**Solution**: Implement ZoxideResolver that wraps `zoxide query <terms>`. Return the best match on success. Use sentinel errors for "not installed" and "no match" conditions. Skip silently when zoxide is not installed.

**Outcome**: Tested zoxide integration that resolves queries via frecency, with graceful degradation when zoxide is absent.

**Do**:
- Create internal/resolver/zoxide.go with ZoxideResolver struct
- Query(terms string) (string, error): run `zoxide query <terms>`, return trimmed stdout on exit 0
- Use exec.LookPath("zoxide") to check availability before running
- If LookPath fails: return "", ErrZoxideNotInstalled (caller skips silently)
- If zoxide exits non-zero (no match): return "", ErrNoMatch
- Define sentinel errors: ErrZoxideNotInstalled, ErrNoMatch

**Acceptance Criteria**:
- [ ] Returns best zoxide match on success
- [ ] Returns ErrZoxideNotInstalled when zoxide not on PATH
- [ ] Returns ErrNoMatch when zoxide finds no match
- [ ] Caller can skip silently on ErrZoxideNotInstalled
- [ ] Output trimmed of whitespace

**Tests**:
- returns best match from zoxide query
- returns ErrZoxideNotInstalled when zoxide not installed
- returns ErrNoMatch when zoxide returns no match
- trims whitespace from zoxide output
- handles multi-word query terms

**Spec Reference**: .workflows/specification/portal/specification.md -- Query Resolution (Zoxide query step), Dependencies (Zoxide is optional soft dependency)
```

portal-4-6 (tick-83fe89):
```
**Problem**: Portal needs a `portal init zsh` command that outputs shell integration: the `x()` and `xctl()` shell functions plus tab completions wired to the function names.

**Solution**: Implement `portal init` Cobra command with zsh subcommand/argument. Emit shell functions that route `x` to `portal open` and `xctl` to `portal`. Emit Cobra-generated completions. Wire completions to function names via `compdef`.

**Outcome**: `eval "$(portal init zsh)"` sets up shell functions and tab completions for zsh.

**Do**:
- Create cmd/init.go with `portal init` command accepting shell name as positional argument
- For zsh: emit `function x() { portal open "$@" }` and `function xctl() { portal "$@" }`
- Emit Cobra-generated zsh completions via rootCmd.GenZshCompletion()
- Wire completions to function names: `compdef x=portal` and `compdef xctl=portal` (or equivalent)
- All output goes to stdout for eval consumption
- The init script is the single source of shell integration per spec

**Acceptance Criteria**:
- [ ] `portal init zsh` emits x() function routing to portal open
- [ ] `portal init zsh` emits xctl() function routing to portal
- [ ] Tab completions emitted for zsh
- [ ] Completions wired to x and xctl names (not just portal binary)
- [ ] Output is valid zsh that can be eval'd

**Tests**:
- init zsh outputs x function definition
- init zsh outputs xctl function definition
- init zsh outputs completion setup
- completions wired to x and xctl names
- output is valid shell syntax

**Context**:
> Per spec: "portal init also emits shell tab completions for the portal binary (generated by Cobra). The init script is the single source of shell integration -- functions, aliases, and completions are all emitted together." Completions must work for the shell function names: "xctl<TAB> completes subcommands (e.g., compdef xctl=portal for zsh)."

**Spec Reference**: .workflows/specification/portal/specification.md -- Shell Functions, CLI Interface (portal init)
```

portal-4-7 (tick-457428):
```
**Problem**: The `portal init` command needs a `--cmd` flag that customizes the shell function names. `portal init zsh --cmd p` should emit `p()` and `pctl()` instead of `x()` and `xctl()`.

**Solution**: Add a `--cmd` string flag to `portal init`. When provided, use the custom name as the launcher function and append `ctl` for the control plane function. Wire completions to the custom names.

**Outcome**: `eval "$(portal init zsh --cmd p)"` creates `p()` and `pctl()` with completions wired to those names.

**Do**:
- Add --cmd string flag to portal init command (default: "x")
- Launcher function name: value of --cmd (e.g., "p")
- Control plane function name: value of --cmd + "ctl" (e.g., "pctl")
- Emit functions with custom names
- Wire completions to custom names (e.g., `compdef p=portal` and `compdef pctl=portal`)

**Acceptance Criteria**:
- [ ] --cmd p emits p() and pctl() functions
- [ ] Completions wired to custom names
- [ ] Default (no --cmd) emits x() and xctl()
- [ ] Custom names work for all supported shells

**Tests**:
- cmd flag changes function names
- default without cmd flag uses x and xctl
- completions wired to custom names
- ctl suffix appended to cmd name

**Edge Cases**:
- cmd name conflicts with shell builtins: not Portal's concern; user's choice

**Spec Reference**: .workflows/specification/portal/specification.md -- Architecture (Command names configurable via --cmd), Shell Functions
```

portal-4-8 (tick-19ad25):
```
**Problem**: Portal needs `portal init bash` and `portal init fish` commands that emit shell-specific functions and completions. Unsupported shell names should produce an error.

**Solution**: Extend portal init to support bash and fish alongside zsh. Each shell gets appropriate syntax for function definitions and completions. Unsupported shell names produce an error with exit code 2.

**Outcome**: portal init works for bash, zsh, and fish. Unsupported shells produce a clear error.

**Do**:
- Implement bash output: bash function syntax, Cobra bash completions
- Implement fish output: fish function syntax, Cobra fish completions
- Validate shell argument: must be "bash", "zsh", or "fish"
- Unsupported shell: print error message to stderr, exit 2 (invalid usage per spec exit codes)
- No powershell support per spec (Portal wraps tmux, which doesn't run on Windows)

**Acceptance Criteria**:
- [ ] `portal init bash` emits valid bash functions and completions
- [ ] `portal init fish` emits valid fish functions and completions
- [ ] Unsupported shell name prints error and exits with code 2
- [ ] bash/zsh/fish are the only supported shells
- [ ] --cmd flag works for all three shells

**Tests**:
- init bash outputs valid bash syntax
- init fish outputs valid fish syntax
- unsupported shell returns exit code 2
- unsupported shell prints error message
- cmd flag works with bash
- cmd flag works with fish

**Spec Reference**: .workflows/specification/portal/specification.md -- CLI Interface (portal init, Supported shells)
```

portal-4-9 (tick-b92526):
```
**Problem**: The file browser needs an `a` shortcut that prompts for an alias name and saves the highlighted directory as an alias. The directory is git-root-resolved before saving. No session is started.

**Solution**: Handle the `a` key in the file browser. Prompt for an alias name via inline text input. On confirmation, resolve the highlighted directory to git root, then save to the aliases file via AliasStore. No session creation occurs.

**Outcome**: Pressing `a` in the file browser creates an alias for the highlighted directory without starting a session.

**Do**:
- Handle `a` key in file browser: enter alias prompt mode
- Render inline text input for alias name
- On Enter: validate non-empty name, resolve highlighted directory to git root, call AliasStore.Set(name, resolvedDir), save
- On Esc: cancel, return to browser
- If alias name already exists, overwrite (per spec: "setting an alias that already exists overwrites it")
- No session is started after alias creation

**Acceptance Criteria**:
- [ ] `a` key prompts for alias name
- [ ] Enter confirms and saves alias
- [ ] Directory git-root-resolved before saving
- [ ] Alias stored in ~/.config/portal/aliases
- [ ] No session started after alias creation
- [ ] Esc cancels alias prompt
- [ ] Existing alias name overwrites silently

**Tests**:
- a key enters alias prompt
- enter saves alias with git-root-resolved path
- esc cancels alias prompt
- empty alias name not saved
- existing alias name overwrites
- no session started after alias creation

**Context**:
> Per spec: "Add alias: a on a highlighted directory prompts for an alias name. Saves to ~/.config/portal/aliases (directory is resolved to git root first). No session is started."

**Spec Reference**: .workflows/specification/portal/specification.md -- File Browser (Add alias) section
```

**Resolution**: Pending
**Notes**: All nine Phase 4 tasks lack the required task template fields and have insufficient detail for implementation. The proposed content adds the full structure and pulls forward spec details. Note: portal-4-5 is addressed separately in Finding 2 with additional TUI fallback detail.

---

### 5. Homebrew Formula tmux Dependency in GoReleaser

**Type**: Incomplete coverage
**Spec Reference**: Distribution -- "The Homebrew formula declares tmux as a dependency, ensuring it's installed automatically."
**Plan Reference**: Phase 6, portal-6-8 (tick-f1a2b3) -- GoReleaser Configuration
**Change Type**: add-to-task

**Details**:
Task portal-6-8 mentions "Add tmux as a Homebrew dependency in the formula template" in the Do section and "tmux declared as Homebrew dependency" in acceptance criteria. However, the spec also mentions the release script that generates version tags (step 1 of the release process). The plan does not cover this release script. Looking more carefully at the spec: "1. Run release script (generates version tag)" -- this is a process step, not a feature to build. The release process is manual (push tag). No finding here actually -- the GoReleaser and GH Actions tasks cover the automated parts. The manual step of creating a tag is standard practice and doesn't need a task. Withdrawing this finding.

Actually, let me re-examine. The spec's release process:
1. Run release script (generates version tag)
2. Push tag to GitHub
3. GitHub Actions workflow triggers GoReleaser
4. GoReleaser builds binaries and creates GitHub Release
5. GoReleaser auto-updates the Homebrew formula

Step 1 mentions a "release script" but this is a convenience script, not a core feature. The plan doesn't create this script, but it's minor tooling. Not a significant finding.

Let me look for other findings instead.

**Spec: `x <query>` existing session matching** -- Wait, re-reading the spec's `x` launcher table: `x <query>` resolves via "alias -> zoxide -> TUI fallback". But this is about resolving to a directory for a new session. What about attaching to an existing session by query? The spec doesn't define `x <query>` as attaching to an existing session -- that's what `xctl attach <name>` is for. `x <query>` always resolves to a directory and creates a new session. This is consistent with the plan.

No additional finding needed for the Homebrew item. Let me remove finding 5 and renumber.

**Resolution**: Withdrawn
**Notes**: Not a significant gap.

---

### 5. Session Sorting Not Explicitly Stated in Plan

**Type**: Incomplete coverage
**Spec Reference**: TUI Design -- Sorting: "Sessions: Displayed in the order returned by `tmux list-sessions`. No additional sorting applied." and "Projects: Sorted by `last_used` timestamp, most recent first."
**Plan Reference**: Phase 1, portal-1-3 (tick-afcc27) -- Session List TUI Model
**Change Type**: add-to-task

**Details**:
The spec explicitly states sessions should be displayed in the order returned by tmux (no additional sorting). Task portal-1-3 does not mention sorting behavior at all. While the absence of sorting logic implicitly preserves tmux order, this spec decision should be explicitly documented in the task to prevent an implementer from adding their own sort. The project sorting (by last_used) is correctly captured in portal-2-6.

**Current**:
From portal-1-3 (tick-afcc27) acceptance criteria:
```
- [ ] portal open launches full-screen alternate-screen TUI
- [ ] Sessions appear with name, window count, attached indicator
- [ ] Window count uses correct pluralisation (1 window vs N windows)
- [ ] Attached sessions show indicator; detached do not
- [ ] First session highlighted by default (cursor at 0)
- [ ] All tests pass
```

**Proposed**:
Updated portal-1-3 (tick-afcc27) acceptance criteria:
```
- [ ] portal open launches full-screen alternate-screen TUI
- [ ] Sessions appear with name, window count, attached indicator
- [ ] Sessions displayed in tmux list-sessions order (no additional sorting)
- [ ] Window count uses correct pluralisation (1 window vs N windows)
- [ ] Attached sessions show indicator; detached do not
- [ ] First session highlighted by default (cursor at 0)
- [ ] All tests pass
```

**Resolution**: Pending
**Notes**: Minor but worth making explicit to prevent implementer from adding unnecessary sorting.

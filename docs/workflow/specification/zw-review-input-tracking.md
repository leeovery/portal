---
status: complete
created: 2026-01-26
phase: Input Review
topic: ZW
---

# Review Tracking: ZW - Input Review

## Findings

### 1. Project name field lifecycle

**Source**: cx-design.md (line 218: "On first session in new directory: prompt 'Project name?'") and zellij-multi-directory.md (simplified model removed explicit project naming)
**Category**: Gap/Ambiguity
**Affects**: Project Memory section, projects.json Structure

**Details**:
The spec's `projects.json` has a `name` field described as "defaults to directory basename, can be customized" — but there's no described mechanism for when/how it gets set or changed. The New Session Flow only shows workspace name and layout prompts. An implementer wouldn't know: Is the user prompted for a project name on first use? Does it silently default? How do you change it later?

**Proposed Addition**:
(pending discussion)

**Resolution**: Approved (expanded scope — also revised Session Naming to auto-generated)
**Notes**: Added Project Naming and Project Management subsections. Rewrote Session Naming to auto-generated {project-name}-{nanoid}. Updated New Session Flow and CLI command descriptions.

---

### 2. File browser shows directories only

**Source**: cx-design.md (line 195: "Shows directories only (not files)")
**Category**: Enhancement to existing topic
**Affects**: File Browser section

**Details**:
The cx-design discussion explicitly states the file browser shows directories only, not files. The spec's File Browser section describes navigation behavior but doesn't explicitly state this filtering. An implementer might show both files and directories.

**Proposed Addition**:
(pending discussion)

**Resolution**: Approved
**Notes**: Added "directories only" to File Browser Behavior list.

---

### 3. GoReleaser Homebrew formula auto-update

**Source**: cx-design.md (lines 430-441: "Auto-generates/updates Homebrew formula in tap repo" and 5-step release workflow)
**Category**: Enhancement to existing topic
**Affects**: Distribution > Build & Release

**Details**:
The cx-design discussion explicitly describes GoReleaser auto-updating the Homebrew formula in the tap repo as part of the release workflow. The spec only says "A release script tags a version, which triggers a GitHub Actions workflow to perform the build and publish the release." Without mentioning the formula auto-update, an implementer might not configure GoReleaser's brew section.

**Proposed Addition**:
(pending discussion)

**Resolution**: Approved
**Notes**: Replaced Build & Release with detailed 5-step release process.

---

### 4. `zw attach` inside Zellij should be blocked

**Source**: zellij-multi-directory.md (line 376: "Block nesting - don't allow attaching to another session from inside one")
**Category**: Gap/Ambiguity
**Affects**: Running Inside Zellij > Utility Mode, CLI Interface

**Details**:
The spec says utility mode blocks "Attaching to another session (prevents nesting)" for TUI interactions. But the `zw attach <name>` CLI command also needs to respect this — if invoked from inside a Zellij session, it should refuse. The spec doesn't explicitly connect `zw attach` to the inside-Zellij detection.

**Proposed Addition**:
(pending discussion)

**Resolution**: Approved
**Notes**: Clarified that nesting block applies to both TUI and `zw attach` CLI command.

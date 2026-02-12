---
topic: x-xctl-split
status: in-progress
date: 2026-02-11
---

# Discussion: Rename & Binary Split (mux → x / xctl)

## Context

Current tool is specced as `mux` — single binary with subcommands (`mux`, `mux .`, `mux <path>`, `mux list`, `mux attach`, `mux clean`, etc.). Not yet implemented.

Proposal: rename to Portal and restructure the CLI surface. Separate interactive launcher UX from scripting/management commands. The tool manages tmux sessions/workspaces — `x` is the "portal" into them.

Source material from external AI conversation proposed two separate binaries. Discussion evolved into a single-binary + shell-integration approach.

### References

- [Current mux spec](../specification/mux.md)
- [tmux-session-managers-analysis](../research/tmux-session-managers-analysis.md)
- [zellij-to-tmux-migration discussion](zellij-to-tmux-migration.md)

## Questions

- [x] Architecture: two binaries or single binary + shell integration?
- [x] Naming: project name, binary name, shell commands?
- [x] What behaviour belongs in `x` (interactive launcher)?
- [x] Query resolution: aliases + zoxide + fallback
- [ ] What behaviour belongs in `xctl` (control plane)?
- [ ] Output conventions for xctl (machine-friendly, --json, exit codes)
- [ ] How does this affect the existing mux spec?

---

*Each question above gets its own section below. Check off as concluded.*

---

## Architecture: two binaries or single binary + shell integration?

### Context

Source material proposed two separate binaries (`x` and `xctl`). But distributing, installing, and versioning two binaries adds friction. And user wanted configurable command names.

### Options Considered

**Option A: Two separate binaries**
- Pros: clean separation at OS level, no shell integration needed
- Cons: two binaries to distribute/install/update, names not configurable without user-maintained aliases

**Option B: Single binary + shell integration (zoxide pattern)**
- Pros: one binary to ship, configurable names via `--cmd`, completions emitted by same `init`, proven pattern (zoxide, starship)
- Cons: requires `eval "$(portal init zsh)"` in shell rc — but this is already standard practice

### Journey

Initially discussed as two binaries per source material. User raised: "do we need two binaries?" — noted the alternative of a single binary with a subcommand (`portal ctl`). Then connected this to zoxide's pattern: one binary (`zoxide`), shell integration creates the UX aliases (`z`, `zi`), configurable via `--cmd`.

Key insight: the `x` vs `xctl` conceptual split still exists — it's just `portal` vs `portal ctl` under the hood. Shell functions are the ergonomic layer.

### Decision

**Single binary `portal` with shell integration.** User adds one line to `.zshrc`:
```
eval "$(portal init zsh)"
```
This emits shell functions/aliases for `x` and `xctl`. Configurable via `--cmd`:
```
eval "$(portal init zsh --cmd p)"  # creates p() and pctl()
```
Whether the init emits functions or aliases is an implementation detail — whatever works. User doesn't define anything manually.

Confidence: High. Matches proven patterns, solves distribution, enables configurability.

---

## Naming: project name, binary name, shell commands?

### Decision

- **Project/engine name**: Portal
- **Binary**: `portal`
- **Default shell commands**: `x` (launcher), `xctl` (control plane)
- **Configurable**: `--cmd` flag on `portal init` changes both (e.g., `--cmd p` → `p` + `pctl`)

The `ctl` suffix convention is well-understood (kubectl, systemctl, sysctl). Immediately signals "control/management tool."

Confidence: High.

---

## What behaviour belongs in `x` (interactive launcher)?

### Decision

`x` is the portal into workspaces. It does one thing: get you into a tmux session.

| Input | Behaviour |
|-------|-----------|
| `x` | Opens TUI picker |
| `x .` | New session in cwd, attach |
| `x <path>` | New session at resolved path, attach |
| `x <query>` | Resolve via alias → zoxide → TUI fallback, attach |

No subcommands. No verbs. Just a destination (or no args for TUI). This is the muscle-memory daily driver.

Confidence: High.

---

## Query resolution: aliases + zoxide + fallback

### Context

Current mux spec uses explicit project aliases. Source material suggested zoxide integration. User wanted both — aliases for deterministic shortcuts (warp-drive style), zoxide for the long tail.

### Options Considered

**Option A: Aliases only** (current mux spec)
- Pros: deterministic, simple
- Cons: must register everything, no discovery

**Option B: Zoxide only**
- Pros: zero config, leverages existing data
- Cons: ambiguous for common dir names (`api`, `app`), no custom shortcuts

**Option C: Aliases + zoxide + fallback**
- Pros: deterministic shortcuts where needed, fuzzy matching everywhere else, graceful degradation
- Cons: slightly more complex resolution — but the order is clear

### Journey

User cited warp-drive as the mental model for aliases — short muscle-memory shortcuts like `m2api` → `~/Code/mac2/api`, `aa` → `~/Code/aerobid/api`. These solve the problem zoxide can't: when multiple directories share the same name (`api` appears in 10 projects).

For everything else, zoxide already has rich frecency data from normal shell usage. Building Portal's own frecency tracker would be over-engineering — cold-start problem, limited data from Portal usage alone.

### Decision

**Resolution order for `x <query>`:**
1. **Existing path** (absolute, relative, `~`) → use directly
2. **Alias match** → resolve to configured path
3. **Zoxide query** (`zoxide query <terms>`) → best frecency match
4. **No match** → fall back to TUI with query pre-filled as filter

Zoxide is an optional soft dependency. If not installed, step 3 is skipped. Aliases and TUI fallback still work.

**Alias management via `xctl`:**
```
xctl alias set m2api ~/Code/mac2/api
xctl alias rm m2api
xctl alias list
```

Storage: flat key-value in a config file. Either a dedicated aliases file or a section in the main config — keep it simple, don't over-engineer.

Confidence: High.

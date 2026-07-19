package spawn

// ExecutableResolver resolves the picker's own binary path. It is the seam over
// os.Executable so command composition is unit-testable without depending on the
// real running binary; production callers pass os.Executable directly.
type ExecutableResolver func() (string, error)

// composeOpenArgv is the pure builder for the env-self-sufficient `open` argv a
// spawned window runs — the SAME open grammar a human types, one pinned target
// plus the hidden --ack receipt. It returns a real argv (never shell syntax),
// each fragment load-bearing:
//
//   - /usr/bin/env … : prefixes a minimal explicit env in front of the exec.
//   - -u TMUX -u TMUX_PANE : the explicit strip. A picker triggered from INSIDE
//     tmux must not leak TMUX/TMUX_PANE into the spawned window, or its `portal
//     open` would take the switch-client path instead of a clean out-of-tmux
//     exec-attach — the spawned N−1 MUST run out of tmux.
//   - PATH=<path> : the ONLY injected var — no whole-env snapshot. PATH is the
//     sole var the spawned open needs to find tmux in a bare host env.
//   - exePath : the picker's own absolute binary (not a bare "portal" PATH
//     lookup) so the version-gated warm-command latch stays satisfied and each
//     spawned open takes the abridged fast-path.
//   - open + the pinned target : one discrete flag/value pair per Surface Kind —
//     SurfaceAttach → `--session <name>` (attach an existing session),
//     SurfaceMint → `--path <literal-dir>` (mint a fresh session at a directory).
//     The target value is a discrete argv element, so a name/dir with a space
//     never needs shell quoting. The mint dir is the REDUCED literal dir resolved
//     at burst time (Surface.Value for a SurfaceMint) — never an alias key or
//     zoxide query, which could re-resolve differently inside the window.
//   - --ack <batch>:<token> : TWO discrete argv elements (never a joined
//     "--ack=<value>" and never quoted). The colon-joined value is unambiguous
//     because option-safe ids are colon-free (see FormatSpawnAckFlag).
//   - -- <command…> : the OPTIONAL mint-only command passthrough (spec § Command
//     passthrough rides mint windows only). When command is non-empty AND the
//     surface is a mint, the literal "--" terminator plus each command element is
//     appended AFTER --ack, so the window runs the command as its new session's
//     initial process. Each command element stays ONE argv element — carried
//     BYTE-IDENTICALLY, never joined, split, or quoted — so a single
//     `-e "npm run dev"` string (command = ["npm run dev"]) reaches the window as
//     one unit, identical to the trigger's local mint. An ATTACH surface never
//     carries the command (an existing session has no safe command-injection
//     channel), and an empty command appends nothing (no dangling bare "--").
//
// Minting happens in the WINDOW at exec time (via `open --path`), never in the
// parent: the burster only spawns this argv, so a window that never comes up
// never mints and leaves no orphan session.
func composeOpenArgv(exePath, path string, surface Surface, batch, token string, command []string) []string {
	targetFlag := "--session"
	if surface.Kind == SurfaceMint {
		targetFlag = "--path"
	}
	argv := []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"PATH=" + path,
		exePath, "open", targetFlag, surface.Value,
		"--ack", FormatSpawnAckFlag(batch, token),
	}
	// Mint-only command passthrough: append "-- <command…>" verbatim after --ack,
	// only for a mint surface carrying a non-empty command. Attach windows and the
	// empty-command case append nothing (no bare "--").
	if surface.Kind == SurfaceMint && len(command) > 0 {
		argv = append(argv, "--")
		argv = append(argv, command...)
	}
	return argv
}

// AttachSurfaces maps a list of existing session names to all-attach Surface
// specs (one SurfaceAttach per name, in order). It is the convergence point that
// lets the picker's all-attach multi-select burst (internal/tui — its SOLE
// consumer, which only ever attaches to already-selected sessions) feed the
// generalized surface-spec Burster without a forked, name-only builder.
func AttachSurfaces(names []string) []Surface {
	surfaces := make([]Surface, len(names))
	for i, name := range names {
		surfaces[i] = Surface{Kind: SurfaceAttach, Value: name}
	}
	return surfaces
}

package spawn

// ExecutableResolver resolves the running binary's own path; production callers
// pass os.Executable.
type ExecutableResolver func() (string, error)

// WindowEnv resolves the "KEY=VALUE" assignments every window's portal is
// started with: whatever the opener resolved its sessions, state and config
// from. A window's shell starts from the terminal's own environment, which
// need not match the opener's, so whatever is not carried can resolve elsewhere.
type WindowEnv func() ([]string, error)

// windowStripped are the variables no window, nor the adapter asking for one,
// may inherit. The ZMX_SESSION strip is load-bearing: a window inheriting it
// would switch the session its opener runs in rather than attach in a
// terminal of its own. TMUX and TMUX_PANE go too, so nothing in the window
// takes itself to be inside a tmux.
var windowStripped = []string{"ZMX_SESSION", "TMUX", "TMUX_PANE"}

// composeOpenArgv unsets windowStripped and sets env before running the
// window's portal. exePath is the running binary rather than a PATH lookup, so
// each window runs the same version of Portal as its opener.
func composeOpenArgv(exePath string, env []string, surface Surface, batch, token string, command []string) []string {
	targetFlag := "--session"
	if surface.Kind == SurfaceMint {
		targetFlag = "--path"
	}
	argv := []string{"/usr/bin/env"}
	for _, name := range windowStripped {
		argv = append(argv, "-u", name)
	}
	argv = append(argv, env...)
	argv = append(argv, exePath, "open", targetFlag, surface.Value, "--ack", FormatSpawnAckFlag(batch, token))
	if surface.Kind == SurfaceMint && len(command) > 0 {
		argv = append(argv, "--")
		argv = append(argv, command...)
	}
	return argv
}

func AttachSurfaces(names []string) []Surface {
	surfaces := make([]Surface, len(names))
	for i, name := range names {
		surfaces[i] = Surface{Kind: SurfaceAttach, Value: name}
	}
	return surfaces
}

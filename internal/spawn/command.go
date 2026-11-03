package spawn

// ExecutableResolver resolves the running binary's own path; production callers
// pass os.Executable.
type ExecutableResolver func() (string, error)

// The ZMX_SESSION strip is load-bearing: a window inheriting it would switch
// the session its opener runs in rather than attach in a terminal of its own.
// TMUX and TMUX_PANE go too, so nothing in the window takes itself to be
// inside a tmux. exePath is the running binary rather than a PATH lookup, so
// each window runs the same version of Portal as its opener.
func composeOpenArgv(exePath, path string, surface Surface, batch, token string, command []string) []string {
	targetFlag := "--session"
	if surface.Kind == SurfaceMint {
		targetFlag = "--path"
	}
	argv := []string{
		"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE",
		"PATH=" + path,
		exePath, "open", targetFlag, surface.Value,
		"--ack", FormatSpawnAckFlag(batch, token),
	}
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

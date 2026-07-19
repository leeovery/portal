package spawn

// SurfaceKind distinguishes the two outcomes a resolved open target reduces to:
// attaching to an existing session, or minting a fresh session at a directory.
type SurfaceKind int

const (
	// SurfaceAttach names an existing session to attach to; Surface.Value is the
	// session NAME. It is the iota zero value, so a zero Surface is an attach.
	SurfaceAttach SurfaceKind = iota
	// SurfaceMint names a directory to mint a fresh session at; Surface.Value is
	// the literal DIR.
	SurfaceMint
)

// String renders a SurfaceKind for readable test output and log lines.
func (k SurfaceKind) String() string {
	switch k {
	case SurfaceAttach:
		return "attach"
	case SurfaceMint:
		return "mint"
	default:
		return "unknown"
	}
}

// Surface is one resolved open target, reduced to what the burst opens: either an
// attach to a named session or a mint at a literal directory. It is the output of
// the read-only resolve engine (cmd.resolveOpenSurfaces) that the multi-target
// burst consumes.
//
// Per spec § Burst exec-argv & mint responsibility, a mint target is reduced to a
// literal existing directory at resolve time — Surface.Value for a SurfaceMint is
// that resolved absolute dir, never the alias key / zoxide query / -p input. An
// alias or zoxide query never travels to a spawned window (it could re-resolve
// differently mid-burst); only the literal dir does, so `--path <dir>` cannot
// diverge and resolution never re-runs inside the window.
type Surface struct {
	Kind  SurfaceKind
	Value string
}

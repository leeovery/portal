package xdg

import (
	"path/filepath"
	"strings"
)

// Lookup resolves an environment variable name to its value, reporting the
// empty string for one that is not set. It is the seam that lets a single
// precedence rule serve both the process environment and an exec.Cmd-style env
// slice a test is about to hand a subprocess.
type Lookup func(name string) string

// EnvSlice returns a Lookup over an exec.Cmd-style "NAME=value" slice. A later
// entry wins, matching exec.Cmd's own dedupe, so the value it reports is the
// one a subprocess launched with the slice would see.
func EnvSlice(env []string) Lookup {
	return func(name string) string {
		prefix := name + "="
		value := ""
		for _, entry := range env {
			if after, ok := strings.CutPrefix(entry, prefix); ok {
				value = after
			}
		}
		return value
	}
}

// ConfigFileID identifies one of Portal's config files: the environment
// variable that overrides its location, the filename it takes under the config
// base, and the log component that speaks for it. The three are declared
// together so that renaming any of them is one edit for every reader — the
// production route and the test seeders that must resolve the same file the
// binary under test reads.
type ConfigFileID struct {
	EnvVar   string
	Filename string
	// LogComponent is empty for a file outside the closed log-component
	// vocabulary. Its one-shot migration still runs; every emission about it is
	// suppressed.
	LogComponent string
}

// Portal's config files. Each is named here and nowhere else.
var (
	ProjectsFile  = ConfigFileID{EnvVar: "PORTAL_PROJECTS_FILE", Filename: "projects.json", LogComponent: "projects"}
	AliasesFile   = ConfigFileID{EnvVar: "PORTAL_ALIASES_FILE", Filename: "aliases", LogComponent: "aliases"}
	HooksFile     = ConfigFileID{EnvVar: "PORTAL_HOOKS_FILE", Filename: "hooks.json", LogComponent: "hooks"}
	PrefsFile     = ConfigFileID{EnvVar: "PORTAL_PREFS_FILE", Filename: "prefs.json"}
	TerminalsFile = ConfigFileID{EnvVar: "PORTAL_TERMINALS_FILE", Filename: "terminals.json"}
)

// ConfigFile is a resolved config-file location.
type ConfigFile struct {
	// Path is where the file lives under the resolved precedence.
	Path string
	// Overridden reports that Path came from the per-file environment variable
	// rather than from the config base. Callers gate on it what only makes sense
	// for the default location — the one-shot Application Support migration
	// among them: a file the user pointed at explicitly is never moved.
	Overridden bool
}

// ConfigFilePath declares Portal's config-file precedence, once and for every
// reader of it: the file's own environment variable, made Absolute, else
// <config base>/portal/<filename>, where the config base is $XDG_CONFIG_HOME
// and then $HOME/.config. The lookup supplies every environment layer, so a
// caller resolving against a subprocess's env slice resolves by the same rule
// as the subprocess itself — a layer added here reaches both by construction.
//
// It resolves a path and nothing more: it creates nothing, stats nothing and
// migrates nothing. Only the home fallback reads outside the lookup, through
// os.UserHomeDir.
func ConfigFilePath(lookup Lookup, id ConfigFileID) (ConfigFile, error) {
	switch path, err := fromEnv(lookup, id.EnvVar); {
	case err != nil:
		return ConfigFile{}, err
	case path != "":
		return ConfigFile{Path: path, Overridden: true}, nil
	}

	portal, err := PortalDir(lookup)
	if err != nil {
		return ConfigFile{}, err
	}

	return ConfigFile{Path: filepath.Join(portal, id.Filename)}, nil
}

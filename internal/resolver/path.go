package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IsPathArgument(arg string) bool {
	if arg == "" {
		return false
	}
	if IsSearchSigil(arg) {
		return false
	}
	return strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'
}

// IsSearchSigil reports whether arg carries the search form: a leading slash and
// no further slash, so `/port` and a bare `/` are search forms while every other
// path shape — `/Users/leeovery/Code/portal`, `/tmp/`, `./port`, `~/port` — is
// not. It is a pure test of the argument's shape and never touches the
// filesystem, so the same command line reads the same way on every machine.
func IsSearchSigil(arg string) bool {
	return strings.HasPrefix(arg, "/") && strings.Count(arg, "/") == 1
}

// SearchTerm returns the text a search-form argument carries after its leading
// slash, empty for a bare slash.
func SearchTerm(arg string) string {
	return strings.TrimPrefix(arg, "/")
}

// ResolvePath expands a tilde and returns the absolute path, erroring unless it
// exists and is a directory.
func ResolvePath(arg string) (string, error) {
	expanded := ExpandTilde(arg)

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("Directory not found: %s", abs) //nolint:staticcheck // user-facing message per spec
	}

	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}

	return abs, nil
}

// NormalisePath expands a tilde and resolves to absolute. Unlike ResolvePath it
// does not check that the path exists.
func NormalisePath(path string) string {
	expanded := ExpandTilde(path)

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return expanded
	}

	return abs
}

func ExpandTilde(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// AbbreviateHome is the inverse of ExpandTilde: a path under home is rewritten
// to its `~/…` form and every other path is returned unchanged. It never touches
// the filesystem or the environment, so a path that does not exist abbreviates
// exactly as one that does. The home is cleaned first, so one given with a
// trailing separator abbreviates as the bare directory does. An empty home
// abbreviates nothing, and a root home only itself, as a shell's prompt does.
func AbbreviateHome(home, path string) string {
	if home == "" {
		return path
	}
	home = filepath.Clean(home)
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	return path
}

// ProcessHome is the process's home directory, or empty where it cannot be
// resolved, which AbbreviateHome reads as abbreviating nothing.
func ProcessHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

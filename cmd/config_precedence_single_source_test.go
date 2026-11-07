package cmd

import (
	"go/ast"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// TestConfigPrecedenceIsSingleSourced guards what the parity test can only
// sample: the parity test drives the env layers it knows about, so a layer
// added to one route alone would escape it until someone thought to add a case.
// This guard removes the "someone thought to" — both routes reach the
// precedence only through xdg.ConfigFilePath and read no environment variable
// of their own, so a layer added to that one declaration reaches both by
// construction, and a layer added outside it fails here.
func TestConfigPrecedenceIsSingleSourced(t *testing.T) {
	routes := []struct {
		file string
		fn   string
	}{
		{file: "config.go", fn: "configFilePath"},
		{file: "../internal/hookstest/hooks.go", fn: "ResolveHooksFilePathFromEnv"},
	}

	// Every way a route could resolve an environment layer behind the shared
	// rule's back. lookup(name) is not among them: that is the rule's own seam,
	// handed the whole environment rather than one variable's precedence.
	ownEnvReads := []string{"Getenv", "LookupEnv", "Environ", "CutPrefix", "ConfigBaseFrom"}

	for _, route := range routes {
		t.Run(route.fn+" resolves through the shared declaration", func(t *testing.T) {
			calls := callsIn(t, route.file, route.fn)

			if !slices.Contains(calls, "ConfigFilePath") {
				t.Errorf("%s in %s calls %v, none of them xdg.ConfigFilePath — the precedence must be read, never restated", route.fn, route.file, calls)
			}
			for _, forbidden := range ownEnvReads {
				if slices.Contains(calls, forbidden) {
					t.Errorf("%s in %s calls %s — an env layer read outside xdg.ConfigFilePath reaches one route only", route.fn, route.file, forbidden)
				}
			}
		})
	}
}

// callsIn returns the names of every call made by the named function in file.
func callsIn(t *testing.T, file, fn string) []string {
	t.Helper()
	source := sourceguardtest.PackageSource(t, filepath.Dir(file), filepath.Base(file))

	var (
		calls []string
		found bool
	)
	sourceguardtest.ForEachFuncCall(source.File, func(funcName string, call *ast.CallExpr) bool {
		if funcName != fn {
			return true
		}
		found = true
		calls = append(calls, sourceguardtest.CalleeName(call))
		return true
	})
	if !found {
		t.Fatalf("%s declares no function %s — this guard has stopped looking at the route it names", file, fn)
	}
	return calls
}

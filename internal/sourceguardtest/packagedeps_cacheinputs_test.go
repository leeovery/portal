package sourceguardtest

import (
	"slices"
	"testing"
)

func TestPackageDeps_CacheInputs(t *testing.T) {
	t.Run("it reads the judged package's directory before resolving its dependencies", func(t *testing.T) {
		calls := recordCalls(t, []dep{{Path: nanoidPkg}})

		PackageDeps(t, nanoidPkg, InDir(nanoidPkgDir))

		want := []string{"read " + nanoidPkgDir, "list"}
		if !slices.Equal(*calls, want) {
			t.Errorf("PackageDeps made the calls %v, want %v — without the read, the judged package's sources are no part of the test binary's cache inputs", *calls, want)
		}
	})

	t.Run("it reads its own working directory when no directory is named", func(t *testing.T) {
		calls := recordCalls(t, []dep{{Path: nanoidPkg}})

		PackageDeps(t, nanoidPkg)

		want := []string{"read .", "list"}
		if !slices.Equal(*calls, want) {
			t.Errorf("PackageDeps made the calls %v, want %v", *calls, want)
		}
	})

	t.Run("it resolves the same dependency set as before the read", func(t *testing.T) {
		// The fixture's module root holds no .go file of its own, so the read
		// taken there yields nothing: the resolution must be unmoved by it.
		dir := stageTaggedFixtureModule(t)

		deps := PackageDeps(t, fixtureLanePkg, InDir(dir))

		if !slices.Contains(deps, fixtureLanePkg) {
			t.Errorf("PackageDeps resolved %v, want the set to hold %s — a directory read yielding nothing has displaced the resolution", deps, fixtureLanePkg)
		}
	})
}

// recordCalls swaps both the directory read and the enumeration for the
// duration of the test, appending a marker per call to the slice it returns, so
// that the order the two are made in is observable.
func recordCalls(t *testing.T, deps []dep) *[]string {
	t.Helper()

	var calls []string
	priorRead, priorList := readPackageSources, listDeps
	readPackageSources = func(dir string) { calls = append(calls, "read "+dir) }
	listDeps = func(depsConfig, string) ([]dep, error) {
		calls = append(calls, "list")
		return deps, nil
	}
	t.Cleanup(func() { readPackageSources, listDeps = priorRead, priorList })
	return &calls
}

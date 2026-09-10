package sourceguardtest

import (
	"slices"
	"testing"
)

const portalModule = "github.com/leeovery/portal"

func TestPackageDeps_CacheInputs(t *testing.T) {
	closure := []dep{
		{Path: nanoidPkg, Module: portalModule, Dir: "/tree/internal/nanoid"},
		{Path: logPkg, Module: portalModule, Dir: "/tree/internal/log"},
		{Path: "fmt", Module: "", Dir: "/goroot/src/fmt"},
		{Path: "golang.org/x/sys/unix", Module: "golang.org/x/sys", Dir: "/modcache/golang.org/x/sys/unix"},
	}

	t.Run("it reads the directory of every dependency in the judged package's module once the set is resolved", func(t *testing.T) {
		calls := recordCalls(t, closure)

		PackageDeps(t, nanoidPkg)

		want := []string{"list", "read /tree/internal/nanoid", "read /tree/internal/log"}
		if !slices.Equal(*calls, want) {
			t.Errorf("PackageDeps made the calls %v, want %v — a dependency the judging binary never read is no part of its cache inputs, and an import added there is served a cached pass", *calls, want)
		}
	})

	t.Run("it reads the judged closure rather than the directory the enumeration was anchored at", func(t *testing.T) {
		calls := recordCalls(t, closure)

		PackageDeps(t, nanoidPkg, InDir("/tree"))

		want := []string{"list", "read /tree/internal/nanoid", "read /tree/internal/log"}
		if !slices.Equal(*calls, want) {
			t.Errorf("PackageDeps made the calls %v, want %v — the anchor holds the judged sources only by coincidence", *calls, want)
		}
	})

	t.Run("it reads nothing when the judged package has no row of its own", func(t *testing.T) {
		calls := recordCalls(t, closure[1:])

		PackageDeps(t, nanoidPkg)

		if want := []string{"list"}; !slices.Equal(*calls, want) {
			t.Errorf("PackageDeps made the calls %v, want %v — with no module to compare against, no directory is known to be the tree's own", *calls, want)
		}
	})

	t.Run("it resolves the same dependency set as before the reads", func(t *testing.T) {
		dir := stageTaggedFixtureModule(t)

		deps := PackageDeps(t, fixtureLanePkg, InDir(dir))

		if !slices.Contains(deps, fixtureLanePkg) {
			t.Errorf("PackageDeps resolved %v, want the set to hold %s — the directory reads have displaced the resolution", deps, fixtureLanePkg)
		}
	})
}

// recordCalls swaps both the directory read and the enumeration for the
// duration of the test, appending a marker per call to the slice it returns, so
// that which directories are read, and when, is observable.
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

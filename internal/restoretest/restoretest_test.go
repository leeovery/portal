//go:build integration

// Package restoretest tests — verify the small pure helpers that the
// integration round-trip tests across cmd/bootstrap and internal/restore
// share. The build tag matches the package itself so this file is only
// compiled under `go test -tags=integration`.

package restoretest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestProjectRoot asserts ProjectRoot walks up from the test's runtime CWD
// until it finds the directory containing the repository's go.mod. The
// integration test packages (cmd/bootstrap, internal/restore, cmd) all
// rely on this to compile the portal CLI from the repo root.
func TestProjectRoot(t *testing.T) {
	root, err := restoretest.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected go.mod under %s: %v", root, err)
	}
	// Sanity: the located module should be the portal module. We read
	// the first line of go.mod and assert the module path matches; a
	// false positive (e.g. a stray go.mod in a parent dir) would
	// otherwise pass the os.Stat check above.
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	want := "module github.com/leeovery/portal"
	if !contains(string(data), want) {
		t.Errorf("go.mod at %s does not declare %q; got:\n%s", root, want, data)
	}
}

// TestSortedKeySet covers the ordering guarantee of SortedKeySet across
// the cases the round-trip diagnostics actually exercise: empty, single
// key, already-sorted, reverse-sorted, and unsorted input.
func TestSortedKeySet(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]struct{}
		want []string
	}{
		{
			name: "empty",
			in:   map[string]struct{}{},
			want: []string{},
		},
		{
			name: "single",
			in:   map[string]struct{}{"alpha": {}},
			want: []string{"alpha"},
		},
		{
			name: "already sorted",
			in:   map[string]struct{}{"alpha": {}, "beta": {}, "gamma": {}},
			want: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "reverse",
			in:   map[string]struct{}{"gamma": {}, "beta": {}, "alpha": {}},
			want: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "unsorted",
			in:   map[string]struct{}{"beta": {}, "gamma": {}, "alpha": {}},
			want: []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restoretest.SortedKeySet(tt.in)
			if len(got) == 0 && len(tt.want) == 0 {
				return // both empty — reflect.DeepEqual treats nil != []string{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SortedKeySet(%v) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
}

// contains is a stdlib-free substring check so the test does not pull in
// strings just for one assertion.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

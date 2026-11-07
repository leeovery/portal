package cmd

import (
	"os"
	"testing"
)

func TestPinToolchainCaches(t *testing.T) {
	t.Run("it runs no go when both caches arrive pinned", func(t *testing.T) {
		t.Setenv("GOMODCACHE", "/pinned/mod")
		t.Setenv("GOCACHE", "/pinned/build")
		t.Setenv("PATH", t.TempDir())

		if err := pinToolchainCaches(); err != nil {
			t.Fatalf("pinToolchainCaches = %v, want nil with no go on PATH to run", err)
		}
		if got := os.Getenv("GOMODCACHE"); got != "/pinned/mod" {
			t.Errorf("GOMODCACHE = %q, want the inherited pin", got)
		}
		if got := os.Getenv("GOCACHE"); got != "/pinned/build" {
			t.Errorf("GOCACHE = %q, want the inherited pin", got)
		}
	})

	t.Run("it asks the toolchain when either cache is unpinned", func(t *testing.T) {
		for _, unpinned := range []string{"GOMODCACHE", "GOCACHE"} {
			t.Run(unpinned, func(t *testing.T) {
				t.Setenv("GOMODCACHE", "/pinned/mod")
				t.Setenv("GOCACHE", "/pinned/build")
				t.Setenv(unpinned, "relative/cache")
				t.Setenv("PATH", t.TempDir())

				if err := pinToolchainCaches(); err == nil {
					t.Fatal("pinToolchainCaches = nil with no go on PATH, want the go env failure that shows it asked")
				}
			})
		}
	})
}

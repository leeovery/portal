package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/project"
)

// unreadableProjectStore stages a projects.json the store cannot read — inside
// a directory with no permissions, so Load fails with a permission error rather
// than reporting the file simply absent.
func unreadableProjectStore(t *testing.T) *project.Store {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "noread")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return project.NewStore(filepath.Join(dir, "projects.json"))
}

// The projects check stands down on a read it could not take, in the words of
// its declared const.
func TestStaleProjectsCheckStandDownCopy(t *testing.T) {
	t.Run("it words the read failure as the user reads it", func(t *testing.T) {
		if projectStoreReadStandDownPhrase != "could not read projects.json" {
			t.Errorf("projectStoreReadStandDownPhrase = %q, want the line the report prints", projectStoreReadStandDownPhrase)
		}
	})

	t.Run("it renders the projects read-failure line from its declared const", func(t *testing.T) {
		cases := []struct {
			name  string
			store *project.Store
		}{
			{name: "no store to read", store: nil},
			{name: "store read failed", store: unreadableProjectStore(t)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := checkStaleProjects(tc.store)
				if got.status != checkNotEvaluable {
					t.Errorf("status = %v, want not evaluable", got.status)
				}
				if got.detail != projectStoreReadStandDownPhrase {
					t.Errorf("detail = %q, want the declared phrase %q", got.detail, projectStoreReadStandDownPhrase)
				}
			})
		}
	})
}

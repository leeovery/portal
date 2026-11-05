package migrate_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/migrate"
)

func TestReadLegacy_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		body, want string
	}{
		"corrupt JSON":                {body: `{"version": 1, "saved_at": "2026-11-04T22:15:03Z", "sessions": [`, want: "decode"},
		"sessions of the wrong shape": {body: `{"version": 1, "saved_at": "2026-11-04T22:15:03Z", "sessions": {"a": 1}}`, want: "decode"},
		"no version":                  {body: `{"saved_at": "2026-11-04T22:15:03Z", "sessions": []}`, want: "has no version"},
		"a newer version":             {body: `{"version": 2, "saved_at": "2026-11-04T22:15:03Z", "sessions": []}`, want: "is version 2, not 1"},
		"no saved_at":                 {body: `{"version": 1, "sessions": []}`, want: "has no saved_at"},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "sessions.json"), tc.body)

			_, err := migrate.ReadLegacy(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ReadLegacy = %v, want an error saying %q", err, tc.want)
			}
		})
	}

	t.Run("it refuses a state dir holding no sessions.json, as a missing file", func(t *testing.T) {
		if _, err := migrate.ReadLegacy(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadLegacy = %v, want an error matching fs.ErrNotExist", err)
		}
	})

	t.Run("it reads an empty index", func(t *testing.T) {
		plan, err := migrate.NewPlan(readLegacy(t, legacyDir(t, nil, nil)), nil, naming(t))
		if err != nil || len(plan.Sessions) != 0 {
			t.Errorf("NewPlan = %+v, %v; want no sessions", plan, err)
		}
	})
}

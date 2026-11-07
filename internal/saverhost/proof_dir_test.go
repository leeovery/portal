package saverhost_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saverhost"
)

// The saver's zmx directory is proved as the zmx-dir claim compares it: any
// spelling of the directory, never another directory.
func TestProve_ZmxDirComparedCanonically(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "zx")
	other := filepath.Join(t.TempDir(), "zx")
	for _, d := range []string{dir, other} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	session := marks[0]

	// proveWith proves a daemon whose environment carries env beside the
	// saver session's name, by marks naming dir.
	proveWith := func(t *testing.T, canonical map[string]func(string) string, env ...string) error {
		t.Helper()
		d := daemon(nil)
		d.env = append([]string{"PATH=/usr/bin", session}, env...)
		p := newProcs(map[int]*proc{daemonPID: d})
		cfg := config(backendtest.New(), p, t.TempDir())
		cfg.Marks = []string{session, "ZMX_DIR=" + dir}
		cfg.Canonical = canonical
		return hostOf(t, cfg).Prove(daemonPID)
	}
	zmxDir := map[string]func(string) string{"ZMX_DIR": zmx.CanonicalDir}

	for name, env := range map[string][]string{
		"the same spelling":              {"ZMX_DIR=" + dir},
		"a trailing slash":               {"ZMX_DIR=" + dir + "/"},
		"a symlinked path":               {"ZMX_DIR=" + link},
		"every setting, each a spelling": {"ZMX_DIR=" + link, "ZMX_DIR=" + dir + "/"},
		"a dotted spelling":              {"ZMX_DIR=" + filepath.Dir(dir) + "/./zx"},
	} {
		t.Run("it proves "+name+" of the directory", func(t *testing.T) {
			if err := proveWith(t, zmxDir, env...); err != nil {
				t.Errorf("Prove with %q = %v, want proved", env, err)
			}
		})
	}

	// A relative path is read against the daemon's working directory, not
	// this one: from here each of these resolves to dir.
	if err := os.Symlink(dir, filepath.Join(filepath.Dir(dir), "here")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(dir))
	for name, env := range map[string][]string{
		"a different directory":                     {"ZMX_DIR=" + other},
		"no ZMX_DIR":                                nil,
		"an empty ZMX_DIR":                          {"ZMX_DIR="},
		"a relative path":                           {"ZMX_DIR=zx"},
		"a relative link to an absolute target":     {"ZMX_DIR=here"},
		"a second setting naming another directory": {"ZMX_DIR=" + link, "ZMX_DIR=" + other},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if err := proveWith(t, zmxDir, env...); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
				t.Errorf("Prove with %q = %v, want ErrNotSaverDaemon", env, err)
			}
		})
	}

	t.Run("a variable Canonical doesn't key is compared exactly", func(t *testing.T) {
		if err := proveWith(t, nil, "ZMX_DIR="+dir+"/"); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Errorf("Prove of a trailing slash with no canonical spelling = %v, want ErrNotSaverDaemon", err)
		}
		if err := proveWith(t, zmxDir, "ZMX_DIR="+dir, session+"/"); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Errorf("Prove of a session name with a trailing slash = %v, want ErrNotSaverDaemon", err)
		}
	})
}

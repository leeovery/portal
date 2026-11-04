package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
)

func TestIsAtomicTemp(t *testing.T) {
	t.Run("it matches every temp file AtomicWrite makes", func(t *testing.T) {
		var made []string
		fileutil.SetSyncsForTest(t, func(f *os.File) error {
			made = append(made, filepath.Base(f.Name()))
			return nil
		}, func(string) error { return nil })
		target := filepath.Join(t.TempDir(), "sessions.json")

		for i := range 32 {
			if err := fileutil.AtomicWrite(target, []byte{byte(i)}); err != nil {
				t.Fatal(err)
			}
		}

		if len(made) != 32 {
			t.Fatalf("AtomicWrite made %d temp files, want 32", len(made))
		}
		for _, name := range made {
			if !fileutil.IsAtomicTemp(name) {
				t.Errorf("IsAtomicTemp(%q) = false for a temp file AtomicWrite made", name)
			}
		}
	})

	t.Run("it matches no other name", func(t *testing.T) {
		for _, name := range []string{
			"",
			".atomic-.tmp",
			".atomic-12a.tmp",
			".atomic--1.tmp",
			".atomic-1",
			".atomic-1.tmp.bak",
			"x.atomic-1.tmp",
			"atomic-1.tmp",
			".atomic-1.TMP",
			"api-0a1b2c3d4e5f-20261104T101500Z-4001.bin",
		} {
			if fileutil.IsAtomicTemp(name) {
				t.Errorf("IsAtomicTemp(%q) = true, want false", name)
			}
		}
	})
}

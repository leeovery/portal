package state_test

import (
	"errors"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var apiCreated = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func scrollbackFile(t *testing.T, name string, created time.Time, pid int) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(name, created, pid)
	if err != nil {
		t.Fatalf("SessionScrollbackFile(%s): %v", name, err)
	}
	return file
}

func TestSessionScrollbackFile(t *testing.T) {
	t.Run("it is the session's stem and creation under scrollback/, relative as the index records it", func(t *testing.T) {
		if got, want := scrollbackFile(t, "api-x7Kd9a", apiCreated, 4242), "scrollback/"+apiStem+"-20261104T100000Z-4242.bin"; got != want {
			t.Errorf("SessionScrollbackFile = %q, want %q", got, want)
		}
	})

	t.Run("it names the creation in UTC, to the nanosecond where it has one", func(t *testing.T) {
		at := apiCreated.Add(500 * time.Millisecond).In(time.FixedZone("BST", 3600))
		if got, want := scrollbackFile(t, "api-x7Kd9a", at, 4242), "scrollback/"+apiStem+"-20261104T100000.5Z-4242.bin"; got != want {
			t.Errorf("SessionScrollbackFile = %q, want %q", got, want)
		}
	})

	t.Run("each creation of a name has a file of its own", func(t *testing.T) {
		first := scrollbackFile(t, "api-x7Kd9a", apiCreated, 4242)
		for what, other := range map[string]string{
			"another time":         scrollbackFile(t, "api-x7Kd9a", apiCreated.Add(time.Second), 4242),
			"another pid":          scrollbackFile(t, "api-x7Kd9a", apiCreated, 4243),
			"a name of other case": scrollbackFile(t, "API-x7Kd9a", apiCreated, 4242),
		} {
			if strings.EqualFold(first, other) {
				t.Errorf("a creation at %s names %q, folding to the first's %q", what, other, first)
			}
		}
		if again := scrollbackFile(t, "api-x7Kd9a", apiCreated, 4242); again != first {
			t.Errorf("the same creation names %q, then %q", first, again)
		}
	})

	t.Run("a zero creation is refused", func(t *testing.T) {
		if _, err := state.SessionScrollbackFile("api-x7Kd9a", time.Time{}, 4242); !errors.Is(err, state.ErrNoCreation) {
			t.Errorf("SessionScrollbackFile = %v, want ErrNoCreation", err)
		}
	})

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" is refused", func(t *testing.T) {
			if _, err := state.SessionScrollbackFile(name, apiCreated, 4242); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("SessionScrollbackFile = %v, want ErrUnsafeName", err)
			}
		})
	}
}

func TestIsSessionScrollbackFile(t *testing.T) {
	for what, base := range map[string]string{
		"a creation's file":                    path.Base(scrollbackFile(t, "api-x7Kd9a", apiCreated, 4242)),
		"a creation's file to the nanosecond":  path.Base(scrollbackFile(t, "api-x7Kd9a", apiCreated.Add(500*time.Millisecond), 4242)),
		"a name holding the stem's separators": path.Base(scrollbackFile(t, "a-b-20261104T100000Z-1", apiCreated, 0)),
	} {
		t.Run("it recognises "+what, func(t *testing.T) {
			if !state.IsSessionScrollbackFile(base) {
				t.Errorf("IsSessionScrollbackFile(%q) = false, want true", base)
			}
		})
	}

	for what, base := range map[string]string{
		"a legacy file named by the name alone": "api-x7Kd9a.bin",
		"a stem whose hash is not its name's":   "api-x7Kd9a-000000000000-20261104T100000Z-4242.bin",
		"a stem with no creation":               apiStem + "-.bin",
		"a stem with a bad creation":            apiStem + "-soon-4242.bin",
		"a stem with no pid":                    apiStem + "-20261104T100000Z.bin",
		"a file not ending .bin":                apiStem + "-20261104T100000Z-4242.txt",
		"a creation's file kept aside":          apiStem + "-20261104T100000Z-4242.bin.old",
		"an atomic write's temp file":           ".atomic-123.tmp",
		"no name at all":                        "",
	} {
		t.Run("it refuses "+what, func(t *testing.T) {
			if state.IsSessionScrollbackFile(base) {
				t.Errorf("IsSessionScrollbackFile(%q) = true, want false", base)
			}
		})
	}
}

func TestScrollbackPath(t *testing.T) {
	file := scrollbackFile(t, "api-x7Kd9a", apiCreated, 4242)

	t.Run("it is the file the entry names, in the state directory", func(t *testing.T) {
		path, err := state.ScrollbackPath("/s", sessionindex.Entry{Name: "api-x7Kd9a", CreatedAt: apiCreated, PID: 4242, ScrollbackFile: file})
		if want := filepath.Join("/s", filepath.FromSlash(file)); err != nil || path != want {
			t.Errorf("ScrollbackPath = %q, %v; want %q", path, err, want)
		}
	})

	t.Run("an entry that adopted a later creation keeps the file an earlier one named", func(t *testing.T) {
		adopted := sessionindex.Entry{Name: "api-x7Kd9a", CreatedAt: apiCreated.Add(48 * time.Hour), PID: 9, ScrollbackFile: file}
		if _, err := state.ScrollbackPath("/s", adopted); err != nil {
			t.Errorf("ScrollbackPath = %v, want the earlier creation's file", err)
		}
	})

	for what, foreign := range map[string]string{
		"none":                           "",
		"a file named by the name alone": "scrollback/api-x7Kd9a.bin",
		"another session's file":         scrollbackFile(t, "api-x7Kd9a-2", apiCreated, 4242),
		"a file of other case":           scrollbackFile(t, "API-x7Kd9a", apiCreated, 4242),
		"a stem with no creation":        "scrollback/" + apiStem + "-.bin",
		"a stem with a bad creation":     "scrollback/" + apiStem + "-soon-4242.bin",
		"a stem with no pid":             "scrollback/" + apiStem + "-20261104T100000Z.bin",
		"a file not ending .bin":         "scrollback/" + apiStem + "-20261104T100000Z-4242.txt",
		"a file below scrollback/":       "scrollback/sub/" + apiStem + "-20261104T100000Z-4242.bin",
		"a file outside scrollback/":     apiStem + "-20261104T100000Z-4242.bin",
		"a climb out of the directory":   "scrollback/../sessions.json",
		"a climb dressed as its file":    "scrollback/../scrollback/" + apiStem + "-20261104T100000Z-4242.bin",
		"an absolute path":               "/etc/" + apiStem + "-20261104T100000Z-4242.bin",
	} {
		t.Run("an entry naming "+what+" is refused", func(t *testing.T) {
			_, err := state.ScrollbackPath("/s", sessionindex.Entry{Name: "api-x7Kd9a", ScrollbackFile: foreign})
			if !errors.Is(err, state.ErrForeignScrollback) {
				t.Errorf("ScrollbackPath(%q) = %v, want ErrForeignScrollback", foreign, err)
			}
		})
	}

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" is refused", func(t *testing.T) {
			if _, err := state.ScrollbackPath("/s", sessionindex.Entry{Name: name, ScrollbackFile: file}); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("ScrollbackPath = %v, want ErrUnsafeName", err)
			}
		})
	}
}

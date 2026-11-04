package state_test

import (
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

func TestSessionScrollback(t *testing.T) {
	t.Run("it is scrollback/<name>.bin, relative as the index records it", func(t *testing.T) {
		file, err := state.SessionScrollbackFile("api-x7Kd9a")
		if err != nil || file != "scrollback/api-x7Kd9a.bin" {
			t.Errorf("SessionScrollbackFile = %q, %v; want scrollback/api-x7Kd9a.bin", file, err)
		}

		path, err := state.SessionScrollbackPath("/s", "api-x7Kd9a")
		if want := filepath.Join("/s", "scrollback", "api-x7Kd9a.bin"); err != nil || path != want {
			t.Errorf("SessionScrollbackPath = %q, %v; want %q", path, err, want)
		}
	})

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" is refused", func(t *testing.T) {
			if _, err := state.SessionScrollbackFile(name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("SessionScrollbackFile = %v, want ErrUnsafeName", err)
			}
			if _, err := state.SessionScrollbackPath("/s", name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("SessionScrollbackPath = %v, want ErrUnsafeName", err)
			}
		})
	}
}

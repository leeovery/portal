package backend_test

import (
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

func TestSession(t *testing.T) {
	t.Run("it is attached only while a client is attached", func(t *testing.T) {
		for clients, want := range map[int]bool{0: false, 1: true, 3: true} {
			if got := (backend.Session{Clients: clients}).Attached(); got != want {
				t.Errorf("Attached() with %d clients = %v, want %v", clients, got, want)
			}
		}
	})

	t.Run("it is Portal's own when its name starts with an underscore", func(t *testing.T) {
		for name, want := range map[string]bool{
			"_portal-saver":   true,
			"_":               true,
			"dotfiles-PpaTER": false,
			"a_b":             false,
			"":                false,
		} {
			if got := (backend.Session{Name: name}).PortalOwned(); got != want {
				t.Errorf("PortalOwned() for %q = %v, want %v", name, got, want)
			}
		}
	})
}

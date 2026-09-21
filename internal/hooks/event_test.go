package hooks_test

import (
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// The Event value is the second-level key of hooks.json, so it is an on-disk
// contract: a changed value orphans every entry already persisted under the old
// one, and the hydrate lookup would find nothing to fire.
func TestEventWireValue(t *testing.T) {
	if got := hooks.EventOnResume.String(); got != "on-resume" {
		t.Errorf("EventOnResume = %q, want %q", got, "on-resume")
	}
}

func TestEventRoundTrip(t *testing.T) {
	t.Run("it persists and reads back an entry through the Event constant", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{})

		if err := store.Set(hookstest.SubjectSeedA, hooks.EventOnResume, hooks.Registration{Command: "echo hi"}, hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("unexpected error on load: %v", err)
		}
		if got := h[hookstest.SubjectSeedA]["on-resume"].Command; got != "echo hi" {
			t.Errorf("persisted on-resume command = %q, want %q", got, "echo hi")
		}

		got, err := store.LookupOnResume(hookstest.SubjectSeedA, hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error on lookup: %v", err)
		}
		if want := (hooks.OnResume{Command: "echo hi", Found: true}); got != want {
			t.Errorf("lookup = %+v, want %+v", got, want)
		}

		removed, err := store.Remove(hookstest.SubjectSeedA, hooks.EventOnResume, hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if !removed {
			t.Error("Remove reported no removal, want the entry Set wrote")
		}
	})
}

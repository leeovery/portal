package sessionindex_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestLookup(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Dir: "/a"}, {Name: "b"}}}

	e, ok := idx.Lookup("a")
	if !ok || e.Dir != "/a" {
		t.Fatalf("Lookup(a) = %+v, %v; want the a entry", e, ok)
	}
	e.Dir = "/moved"
	if idx.Sessions[0].Dir != "/moved" {
		t.Errorf("an edit through Lookup did not reach the index")
	}
	if _, ok := idx.Lookup("missing"); ok {
		t.Errorf("Lookup(missing) found an entry")
	}
}

func TestLabel(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}, {Name: "b"}}}
	for name, want := range map[string]string{"a": "Alpha", "b": "b", "missing": "missing"} {
		if got := idx.Label(name); got != want {
			t.Errorf("Label(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestSetLabel(t *testing.T) {
	t.Run("it sets a trimmed label", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Dir: "/a"}}}
		if err := idx.SetLabel("a", "  my work \n"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if want := (sessionindex.Entry{Name: "a", Label: "my work", Dir: "/a"}); idx.Sessions[0] != want {
			t.Errorf("entry = %+v, want %+v", idx.Sessions[0], want)
		}
	})

	t.Run("it creates a bare entry for a session with none", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.SetLabel("a", "Alpha"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0] != (sessionindex.Entry{Name: "a", Label: "Alpha"}) {
			t.Errorf("sessions = %+v, want one bare entry labelled Alpha", idx.Sessions)
		}
	})

	for _, reset := range []string{"", "   ", "a", " a "} {
		t.Run("the label "+`"`+reset+`"`+" resets to the default", func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}}}
			if err := idx.SetLabel("a", reset); err != nil {
				t.Fatalf("SetLabel: %v", err)
			}
			if idx.Sessions[0].Label != "" {
				t.Errorf("label = %q, want cleared", idx.Sessions[0].Label)
			}
			if got := idx.Label("a"); got != "a" {
				t.Errorf("Label = %q, want the name", got)
			}
		})
	}

	for name, label := range map[string]string{
		"a newline": "two\nlines",
		"a tab":     "tab\there",
		"an escape": "red\x1b[31m",
		"a DEL":     "del\x7f",
	} {
		t.Run("a label holding "+name+" is refused and changes nothing", func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}}}
			if err := idx.SetLabel("a", label); !errors.Is(err, sessionindex.ErrLabelControlChar) {
				t.Errorf("SetLabel(%q) = %v, want ErrLabelControlChar", label, err)
			}
			if err := idx.SetLabel("b", label); !errors.Is(err, sessionindex.ErrLabelControlChar) {
				t.Errorf("SetLabel on a new session = %v, want ErrLabelControlChar", err)
			}
			if len(idx.Sessions) != 1 || idx.Sessions[0].Label != "Alpha" {
				t.Errorf("sessions = %+v, want untouched", idx.Sessions)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	if !idx.Remove("b") {
		t.Errorf("Remove(b) = false, want true")
	}
	if len(idx.Sessions) != 2 || idx.Sessions[0].Name != "a" || idx.Sessions[1].Name != "c" {
		t.Errorf("sessions = %+v, want a and c", idx.Sessions)
	}
	if idx.Remove("b") {
		t.Errorf("Remove of an absent entry = true, want false")
	}
}

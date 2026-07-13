package spawn

import "testing"

func TestQuoteJoin(t *testing.T) {
	t.Run("it single-quotes a single name", func(t *testing.T) {
		if got := QuoteJoin([]string{"s2"}); got != "'s2'" {
			t.Errorf("QuoteJoin([s2]) = %q, want %q", got, "'s2'")
		}
	})

	t.Run("it single-quotes and comma-joins several names", func(t *testing.T) {
		if got := QuoteJoin([]string{"s2", "s4"}); got != "'s2', 's4'" {
			t.Errorf("QuoteJoin([s2 s4]) = %q, want %q", got, "'s2', 's4'")
		}
	})

	t.Run("it renders the empty string for no names", func(t *testing.T) {
		if got := QuoteJoin(nil); got != "" {
			t.Errorf("QuoteJoin(nil) = %q, want empty", got)
		}
	})
}

func TestGoneVerb(t *testing.T) {
	t.Run("it is 'is' for a single name", func(t *testing.T) {
		if got := GoneVerb(1); got != "is" {
			t.Errorf("GoneVerb(1) = %q, want %q", got, "is")
		}
	})

	t.Run("it is 'are' for several names", func(t *testing.T) {
		if got := GoneVerb(2); got != "are" {
			t.Errorf("GoneVerb(2) = %q, want %q", got, "are")
		}
	})

	t.Run("it is 'are' for zero (plural default)", func(t *testing.T) {
		if got := GoneVerb(0); got != "are" {
			t.Errorf("GoneVerb(0) = %q, want %q", got, "are")
		}
	})
}

func TestGoneMessage(t *testing.T) {
	t.Run("it renders the singular gone body for one name", func(t *testing.T) {
		const want = "'s2' is gone — nothing opened"
		if got := GoneMessage([]string{"s2"}); got != want {
			t.Errorf("GoneMessage([s2]) = %q, want %q", got, want)
		}
	})

	t.Run("it renders the plural gone body for several names", func(t *testing.T) {
		const want = "'s2', 's4' are gone — nothing opened"
		if got := GoneMessage([]string{"s2", "s4"}); got != want {
			t.Errorf("GoneMessage([s2 s4]) = %q, want %q", got, want)
		}
	})
}

func TestUnsupportedNoopMessage(t *testing.T) {
	t.Run("it renders the honest no-host-local body for a NULL identity", func(t *testing.T) {
		const want = "no host-local terminal — nothing opened"
		if got := UnsupportedNoopMessage(Identity{}); got != want {
			t.Errorf("UnsupportedNoopMessage(NULL) = %q, want %q", got, want)
		}
	})

	t.Run("it names the terminal and bundle id for a recognised identity", func(t *testing.T) {
		const want = "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"
		id := Identity{Name: "Apple Terminal", BundleID: "com.apple.Terminal"}
		if got := UnsupportedNoopMessage(id); got != want {
			t.Errorf("UnsupportedNoopMessage(id) = %q, want %q", got, want)
		}
	})
}

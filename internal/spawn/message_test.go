package spawn

import (
	"strings"
	"testing"
)

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

func TestPartialFailureMessage(t *testing.T) {
	t.Run("it renders the leave-what-opened body for one failed name when others opened", func(t *testing.T) {
		const want = "'s2' failed to open — others left open"
		if got := PartialFailureMessage([]string{"s2"}, true); got != want {
			t.Errorf("PartialFailureMessage([s2], true) = %q, want %q", got, want)
		}
	})

	t.Run("it names every failed window in list order for several names when others opened", func(t *testing.T) {
		const want = "'s2', 's3' failed to open — others left open"
		if got := PartialFailureMessage([]string{"s2", "s3"}, true); got != want {
			t.Errorf("PartialFailureMessage([s2 s3], true) = %q, want %q", got, want)
		}
	})

	t.Run("it renders the nothing-opened body for one failed name on total failure", func(t *testing.T) {
		const want = "'s2' failed to open — nothing opened"
		if got := PartialFailureMessage([]string{"s2"}, false); got != want {
			t.Errorf("PartialFailureMessage([s2], false) = %q, want %q", got, want)
		}
	})

	t.Run("it names every failed window in list order for several names on total failure", func(t *testing.T) {
		const want = "'s2', 's3' failed to open — nothing opened"
		if got := PartialFailureMessage([]string{"s2", "s3"}, false); got != want {
			t.Errorf("PartialFailureMessage([s2 s3], false) = %q, want %q", got, want)
		}
	})

	t.Run("it carries no spawn prefix and no glyph for both variants", func(t *testing.T) {
		for _, othersOpened := range []bool{true, false} {
			got := PartialFailureMessage([]string{"s2"}, othersOpened)
			if strings.HasPrefix(got, "spawn:") {
				t.Errorf("PartialFailureMessage body = %q, want no \"spawn:\" prefix", got)
			}
			if strings.ContainsRune(got, '⚠') {
				t.Errorf("PartialFailureMessage body = %q, want no ⚠ glyph", got)
			}
		}
	})
}

func TestUnsupportedNoopMessage(t *testing.T) {
	t.Run("it renders the plain remote-connection body for a NULL identity", func(t *testing.T) {
		const want = "can't open new windows over a remote connection — nothing opened"
		if got := UnsupportedNoopMessage(Identity{}); got != want {
			t.Errorf("UnsupportedNoopMessage(NULL) = %q, want %q", got, want)
		}
	})

	t.Run("it names the terminal and bundle id for a recognised identity", func(t *testing.T) {
		const want = "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"
		id := Identity{Name: "Apple Terminal", BundleID: "com.apple.Terminal"}
		if got := UnsupportedNoopMessage(id); got != want {
			t.Errorf("UnsupportedNoopMessage(id) = %q, want %q", got, want)
		}
	})
}

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

package warning_test

import (
	"bytes"
	"testing"

	"github.com/leeovery/portal/internal/warning"
)

func TestWriteLines(t *testing.T) {
	t.Run("zero warnings writes nothing", func(t *testing.T) {
		var buf bytes.Buffer
		warning.WriteLines(&buf, nil)
		if buf.Len() != 0 {
			t.Errorf("WriteLines on nil wrote %q; want empty", buf.String())
		}

		warning.WriteLines(&buf, []warning.Warning{})
		if buf.Len() != 0 {
			t.Errorf("WriteLines on empty slice wrote %q; want empty", buf.String())
		}
	})

	t.Run("single warning with multiple lines writes one Fprintln per line", func(t *testing.T) {
		var buf bytes.Buffer
		warning.WriteLines(&buf, []warning.Warning{
			{Lines: []string{"line one", "line two", "line three"}},
		})

		want := "line one\nline two\nline three\n"
		if buf.String() != want {
			t.Errorf("WriteLines wrote %q; want %q", buf.String(), want)
		}
	})

	t.Run("multiple warnings concatenate in order", func(t *testing.T) {
		var buf bytes.Buffer
		warning.WriteLines(&buf, []warning.Warning{
			{Lines: []string{"first warn line 1", "first warn line 2"}},
			{Lines: []string{"second warn line 1"}},
			{Lines: []string{"third warn line 1", "third warn line 2"}},
		})

		want := "first warn line 1\nfirst warn line 2\n" +
			"second warn line 1\n" +
			"third warn line 1\nthird warn line 2\n"
		if buf.String() != want {
			t.Errorf("WriteLines wrote %q; want %q", buf.String(), want)
		}
	})

	t.Run("warning with empty lines slice writes nothing for that warning", func(t *testing.T) {
		var buf bytes.Buffer
		warning.WriteLines(&buf, []warning.Warning{
			{Lines: []string{"before"}},
			{Lines: nil},
			{Lines: []string{"after"}},
		})

		want := "before\nafter\n"
		if buf.String() != want {
			t.Errorf("WriteLines wrote %q; want %q", buf.String(), want)
		}
	})
}

package bgreply

import (
	"slices"
	"testing"
)

const (
	darkReply   = "\x1b]11;rgb:0b0b/0c0c/1414\a"
	lightReply  = "\x1b]11;rgb:e1e1/e2e2/e7e7\a"
	stDark      = "\x1b]11;rgb:0b0b/0c0c/1414\x1b\\"
	c1Light     = "\x1b]11;rgb:e1e1/e2e2/e7e7\x9c"
	truncated   = "\x1b]11;rgb:0b0b/0c0c/14"
	darkColour  = "rgb:0b0b/0c0c/1414"
	lightColour = "rgb:e1e1/e2e2/e7e7"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		rest     string
		payloads []string
		head     string
	}{
		{"nothing", "", "", nil, ""},
		{"a key", "\r", "\r", nil, ""},
		{"a BEL-terminated reply", darkReply, "", []string{darkColour}, ""},
		{"an ESC \\-terminated reply", stDark, "", []string{darkColour}, ""},
		{"a C1 ST-terminated reply", c1Light, "", []string{lightColour}, ""},
		{"Enter on either side", "\r" + darkReply + "\r", "\r\r", []string{darkColour}, ""},
		{"two replies and a key between", lightReply + "d" + stDark, "d", []string{lightColour, darkColour}, ""},
		{"two replies back to back", darkReply + c1Light, "", []string{darkColour, lightColour}, ""},
		{"a reply cut off in its payload", "d" + truncated, "d", nil, truncated},
		{"a reply cut off at its closing escape", "\x1b]11;rgb:0b\x1b", "", nil, "\x1b]11;rgb:0b\x1b"},
		{"a reply, then a second cut off", darkReply + "\r" + truncated, "\r", []string{darkColour}, truncated},
		{"an introducer cut off two bytes in", "q\x1b]", "q", nil, "\x1b]"},
		{"an introducer cut off four bytes in", "q\x1b]11", "q", nil, "\x1b]11"},
		{"an escape alone at the end", "q\x1b", "q\x1b", nil, ""},
		{"a reply an escape broke off, then an arrow", "\x1b]11;rgb\x1b[A", "\x1b[A", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest, payloads, head := Split([]byte(tc.in))

			if string(rest) != tc.rest || !slices.Equal(payloads, tc.payloads) || string(head) != tc.head {
				t.Errorf("Split(%q) = %q, %q, %q; want %q, %q, %q", tc.in, rest, payloads, head, tc.rest, tc.payloads, tc.head)
			}
		})
	}

	t.Run("a head shares no memory with the input", func(t *testing.T) {
		in := []byte("d" + truncated)

		_, _, head := Split(in)
		in[len(in)-1] = 'x'

		if string(head) != truncated {
			t.Errorf("head = %q after its input changed, want %q", head, truncated)
		}
	})
}

func TestTyped(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		typed bool
	}{
		{"nothing", "", false},
		{"a whole reply", darkReply, false},
		{"two replies", darkReply + c1Light, false},
		{"a reply still arriving", truncated, false},
		{"an introducer's start", "\x1b]1", false},
		{"a reply at its closing escape", "\x1b]11;rgb:0b\x1b", false},
		{"an escape, which may open a reply", "\x1b", false},
		{"a reply, then an escape", darkReply + "\x1b", false},
		{"a key", "\r", true},
		{"an arrow", "\x1b[A", true},
		{"a reply and a key", darkReply + "\r", true},
		{"an escape before a reply", "\x1b" + darkReply, true},
		{"an escape before a reply still arriving", "\x1b" + truncated, true},
		{"a key before a reply still arriving", "x" + truncated, true},
		{"a reply's rest with no head", "rgb:0b0b/0c0c/1414\a", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Typed([]byte(tc.in)); got != tc.typed {
				t.Errorf("Typed(%q) = %v, want %v", tc.in, got, tc.typed)
			}
		})
	}
}

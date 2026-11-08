package bgreply

import (
	"slices"
	"strconv"
	"strings"
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

func TestStream(t *testing.T) {
	t.Run("one piece", func(t *testing.T) {
		cases := []struct {
			name       string
			in         string
			replies    int
			typed      bool
			unfinished bool
		}{
			{"nothing", "", 0, false, false},
			{"a whole reply", darkReply, 1, false, false},
			{"two replies", darkReply + c1Light, 2, false, false},
			{"a reply still arriving", truncated, 0, false, true},
			{"an introducer's start", "\x1b]1", 0, false, true},
			{"a reply at its closing escape", "\x1b]11;rgb:0b\x1b", 0, false, true},
			{"an escape, which may open a reply", "\x1b", 0, false, true},
			{"a reply, then an escape", darkReply + "\x1b", 1, false, true},
			{"a key", "\r", 0, true, false},
			{"an arrow", "\x1b[A", 0, true, false},
			{"a reply and a key", darkReply + "\r", 1, true, false},
			{"an escape before a reply", "\x1b" + darkReply, 1, true, false},
			{"an escape before a reply still arriving", "\x1b" + truncated, 0, true, true},
			{"a key before a reply still arriving", "x" + truncated, 0, true, true},
			{"a key, then an escape", "x\x1b", 0, true, true},
			{"a reply's rest with no head", "rgb:0b0b/0c0c/1414\a", 0, true, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var s Stream
				replies, typed := s.Take([]byte(tc.in))

				if replies != tc.replies || typed != tc.typed || s.Unfinished() != tc.unfinished {
					t.Errorf("Take(%q) = %d, %v, unfinished %v; want %d, %v, unfinished %v",
						tc.in, replies, typed, s.Unfinished(), tc.replies, tc.typed, tc.unfinished)
				}
			})
		}
	})

	t.Run("a reply split across pieces is finished by the piece that ends it, and is typed by neither", func(t *testing.T) {
		for _, cut := range []int{1, 2, 4, 5, 12, len(stDark) - 1} {
			t.Run(strconv.Itoa(cut), func(t *testing.T) {
				var s Stream
				headReplies, headTyped := s.Take([]byte(stDark[:cut]))
				tailReplies, tailTyped := s.Take([]byte(stDark[cut:]))

				if headReplies != 0 || headTyped || tailReplies != 1 || tailTyped || s.Unfinished() {
					t.Errorf("Take(%q), Take(%q) = (%d, %v), (%d, %v), unfinished %v; want (0, false), (1, false), unfinished false",
						stDark[:cut], stDark[cut:], headReplies, headTyped, tailReplies, tailTyped, s.Unfinished())
				}
			})
		}
	})

	t.Run("an escape that opens no reply is typed with the piece after it", func(t *testing.T) {
		var s Stream
		s.Take([]byte("\x1b"))

		if replies, typed := s.Take([]byte("[A")); replies != 0 || !typed || s.Unfinished() {
			t.Errorf("Take(%q) after an escape = %d, %v, unfinished %v; want 0, true, unfinished false", "[A", replies, typed, s.Unfinished())
		}
	})

	t.Run("an escape alone as the input falls quiet was the Escape key, and opens no reply", func(t *testing.T) {
		var s Stream
		s.Take([]byte("\x1b"))
		s.Quiet()

		if s.Unfinished() {
			t.Fatal("an escape the input fell quiet after is still unfinished")
		}
		if replies, typed := s.Take([]byte("]11;" + darkColour + "\a")); replies != 0 || !typed {
			t.Errorf("Take(a reply's rest) after the quiet = %d, %v; want 0, true", replies, typed)
		}
	})

	t.Run("a reply cut off as the input falls quiet is kept for its rest", func(t *testing.T) {
		var s Stream
		s.Take([]byte(truncated))
		s.Quiet()

		if replies, typed := s.Take([]byte("14\a")); replies != 1 || typed {
			t.Errorf("Take(the reply's rest) after the quiet = %d, %v; want 1, false", replies, typed)
		}
	})

	t.Run("a head longer than any reply's is typed, and kept no longer", func(t *testing.T) {
		var s Stream
		s.Take([]byte(Introducer))

		replies, typed := s.Take([]byte(strings.Repeat("0", headKept)))

		if replies != 0 || !typed || s.Unfinished() {
			t.Errorf("a head of %d bytes = %d, %v, unfinished %v; want 0, true, unfinished false", len(Introducer)+headKept, replies, typed, s.Unfinished())
		}
	})

	t.Run("a head as long as a reply's is kept", func(t *testing.T) {
		var s Stream
		s.Take([]byte(Introducer + strings.Repeat("0", headKept-len(Introducer))))

		if replies, typed := s.Take([]byte("\a")); replies != 1 || typed {
			t.Errorf("a reply of %d bytes = %d, %v; want 1, false", headKept+1, replies, typed)
		}
	})
}

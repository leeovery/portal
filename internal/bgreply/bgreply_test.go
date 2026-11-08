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
		cut      bool
	}{
		{"nothing", "", "", nil, false},
		{"a key", "\r", "\r", nil, false},
		{"a BEL-terminated reply", darkReply, "", []string{darkColour}, false},
		{"an ESC \\-terminated reply", stDark, "", []string{darkColour}, false},
		{"a C1 ST-terminated reply", c1Light, "", []string{lightColour}, false},
		{"Enter on either side", "\r" + darkReply + "\r", "\r\r", []string{darkColour}, false},
		{"two replies and a key between", lightReply + "d" + stDark, "d", []string{lightColour, darkColour}, false},
		{"two replies back to back", darkReply + c1Light, "", []string{darkColour, lightColour}, false},
		{"a reply cut off in its payload", "d" + truncated, "d", nil, true},
		{"a reply cut off at its closing escape", "\x1b]11;rgb:0b\x1b", "", nil, true},
		{"a reply, then a second cut off", darkReply + "\r" + truncated, "\r", []string{darkColour}, true},
		{"an introducer cut off two bytes in", "q\x1b]", "q", nil, true},
		{"an introducer cut off four bytes in", "q\x1b]11", "q", nil, true},
		{"an escape alone at the end", "q\x1b", "q\x1b", nil, false},
		{"a reply an escape broke off, then an arrow", "\x1b]11;rgb\x1b[A", "\x1b[A", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest, payloads, cut := Split([]byte(tc.in))

			if string(rest) != tc.rest || !slices.Equal(payloads, tc.payloads) || cut != tc.cut {
				t.Errorf("Split(%q) = %q, %q, %v; want %q, %q, %v", tc.in, rest, payloads, cut, tc.rest, tc.payloads, tc.cut)
			}
		})
	}
}

func TestReadsAsOne(t *testing.T) {
	cases := []struct {
		name           string
		in             string
		whole, partial bool
	}{
		{"a whole reply", darkReply, true, false},
		{"a whole ESC \\-terminated reply", stDark, true, false},
		{"a whole C1 ST-terminated reply", c1Light, true, false},
		{"an escape", "\x1b", false, true},
		{"an introducer's start", "\x1b]1", false, true},
		{"a reply still arriving", truncated, false, true},
		{"a reply at its closing escape", "\x1b]11;rgb:0b\x1b", false, true},
		{"a key", "\r", false, false},
		{"an arrow", "\x1b[A", false, false},
		{"a reply and a key", darkReply + "\r", false, false},
		{"two replies", darkReply + darkReply, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			whole, partial := ReadsAsOne([]byte(tc.in))

			if whole != tc.whole || partial != tc.partial {
				t.Errorf("ReadsAsOne(%q) = %v, %v; want %v, %v", tc.in, whole, partial, tc.whole, tc.partial)
			}
		})
	}
}

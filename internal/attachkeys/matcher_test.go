package attachkeys_test

import (
	"encoding/json"
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
)

// richKeys binds every shape of key, so each encoding has a binding to find.
const richKeys = `{
	"detach": ["alt+d", "ctrl+o", "ctrl+alt+x", "alt+f1", "ctrl+f5", "ctrl+shift+f12", "alt+/"],
	"detach_to_shell": "alt+shift+d",
	"kill": ["alt+q", "ctrl+]"]
}`

func bindingsFor(t testing.TB, raw string) attachkeys.Bindings {
	t.Helper()
	b, warnings := attachkeys.Parse(json.RawMessage(raw))
	if len(warnings) != 0 {
		t.Fatalf("Parse(%s) warnings = %v; want none", raw, warnings)
	}
	return b
}

// scanned is what one Scan returns; action zero means nothing was found.
type scanned struct {
	forward string
	action  attachkeys.Action
}

func scan(m *attachkeys.Matcher, read string) scanned {
	forward, action, found := m.Scan([]byte(read))
	if found != (action != 0) {
		return scanned{forward: string(forward), action: -1}
	}
	return scanned{forward: string(forward), action: action}
}

// passes is a read forwarded whole, with nothing found.
func passes(read string) scanned {
	return scanned{forward: read}
}

func found(action attachkeys.Action) scanned {
	return scanned{action: action}
}

func assertScans(t *testing.T, m *attachkeys.Matcher, reads []string, want []scanned) {
	t.Helper()
	for i, read := range reads {
		if got := scan(m, read); got != want[i] {
			t.Errorf("read %d %q: Scan = (forward %q, action %d); want (forward %q, action %d)",
				i, read, got.forward, got.action, want[i].forward, want[i].action)
		}
	}
}

type scanCase struct {
	name string
	read string
	want scanned
}

func runScanCases(t *testing.T, raw string, cases []scanCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertScans(t, attachkeys.NewMatcher(bindingsFor(t, raw)), []string{tc.read}, []scanned{tc.want})
		})
	}
}

type readsCase struct {
	name  string
	reads []string
	want  []scanned
}

func runReadsCases(t *testing.T, cases []readsCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertScans(t, attachkeys.NewMatcher(bindingsFor(t, richKeys)), tc.reads, tc.want)
		})
	}
}

const (
	detach = attachkeys.Detach
	toSh   = attachkeys.DetachToShell
	kill   = attachkeys.Kill
)

func TestScan_EachBindingInEachEncoding(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"alt+d legacy, a read of its own", "\x1bd", found(detach)},
		{"alt+d CSI-u", "\x1b[100;3u", found(detach)},
		{"alt+d CSI-u press event", "\x1b[100;3:1u", found(detach)},
		{"alt+d CSI-u base-layout alternate", "\x1b[100::100;3u", found(detach)},
		{"alt+d CSI-u alternates and event", "\x1b[100::100;3:1u", found(detach)},
		{"alt+d CSI-u text field", "\x1b[100;3;100u", found(detach)},
		{"alt+d CSI-u caps lock", "\x1b[100;67u", found(detach)},
		{"alt+d CSI-u num lock", "\x1b[100;131u", found(detach)},
		{"alt+d CSI-u both locks and event", "\x1b[100;195:1u", found(detach)},
		{"alt+d modifyOtherKeys", "\x1b[27;3;100~", found(detach)},
		{"alt+d modifyOtherKeys caps lock", "\x1b[27;67;100~", found(detach)},

		{"ctrl+o legacy", "\x0f", found(detach)},
		{"ctrl+o CSI-u", "\x1b[111;5u", found(detach)},
		{"ctrl+o CSI-u press event", "\x1b[111;5:1u", found(detach)},
		{"ctrl+o CSI-u caps lock", "\x1b[111;69u", found(detach)},
		{"ctrl+o modifyOtherKeys", "\x1b[27;5;111~", found(detach)},

		{"ctrl+alt+x legacy, a read of its own", "\x1b\x18", found(detach)},
		{"ctrl+alt+x CSI-u", "\x1b[120;7u", found(detach)},
		{"ctrl+alt+x modifyOtherKeys", "\x1b[27;7;120~", found(detach)},

		{"alt+shift+d legacy, a read of its own", "\x1bD", found(toSh)},
		{"alt+shift+d CSI-u", "\x1b[100;4u", found(toSh)},
		{"alt+shift+d CSI-u shifted alternate", "\x1b[100:68;4u", found(toSh)},
		{"alt+shift+d CSI-u caps lock", "\x1b[100;68:1u", found(toSh)},
		{"alt+shift+d by its uppercase code, as fixterms and iTerm2 send it", "\x1b[68;4u", found(toSh)},
		{"alt+shift+d modifyOtherKeys, by its uppercase code", "\x1b[27;4;68~", found(toSh)},

		{"alt+q legacy, a read of its own", "\x1bq", found(kill)},
		{"alt+q CSI-u", "\x1b[113;3u", found(kill)},
		{"ctrl+] legacy", "\x1d", found(kill)},
		{"ctrl+] CSI-u", "\x1b[93;5u", found(kill)},

		{"alt+/ legacy, a read of its own", "\x1b/", found(detach)},
		{"alt+/ CSI-u", "\x1b[47;3u", found(detach)},
	})
}

func TestScan_FunctionKeysWithModifiers(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"alt+f1 legacy", "\x1b[1;3P", found(detach)},
		{"alt+f1 with a press event", "\x1b[1;3:1P", found(detach)},
		{"alt+f1 caps lock", "\x1b[1;67P", found(detach)},
		{"ctrl+f5 legacy", "\x1b[15;5~", found(detach)},
		{"ctrl+f5 with a press event", "\x1b[15;5:1~", found(detach)},
		{"ctrl+shift+f12 legacy", "\x1b[24;6~", found(detach)},
		{"a function key amid typing", "ab\x1b[15;5~cd", scanned{forward: "ab", action: detach}},
		{"alt+f1 release swallowed", "\x1b[1;3:3P", scanned{}},
		{"ctrl+f5 repeat swallowed", "\x1b[15;5:2~", scanned{}},
		{"ctrl+f1 is not alt+f1", "\x1b[1;5P", passes("\x1b[1;5P")},
		{"unmodified f1", "\x1bOP", passes("\x1bOP")},
		{"a cursor position report shares alt+f1's numbers", "\x1b[1;3R", passes("\x1b[1;3R")},
		{"f6 is not f5", "\x1b[17;5~", passes("\x1b[17;5~")},
		{"a function key takes no alternates", "\x1b[15:1;5~", passes("\x1b[15:1;5~")},
	})
}

func TestScan_ModifiersMatchExactly(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"ctrl+alt+d is not alt+d", "\x1b[100;7u", passes("\x1b[100;7u")},
		{"super+alt+d is not alt+d", "\x1b[100;11u", passes("\x1b[100;11u")},
		{"an uppercase code without shift is not the letter", "\x1b[68;3u", passes("\x1b[68;3u")},
		{"an unbound alt key", "\x1b[97;3u", passes("\x1b[97;3u")},
		{"an unbound alt key, legacy", "\x1ba", passes("\x1ba")},
		{"an unknown event type", "\x1b[100;3:4u", passes("\x1b[100;3:4u")},
		{"a zero modifier field", "\x1b[100;0u", passes("\x1b[100;0u")},
		{"an empty modifier field", "\x1b[100;u", passes("\x1b[100;u")},
		{"a private marker", "\x1b[?100;3u", passes("\x1b[?100;3u")},
		{"an intermediate byte", "\x1b[100;3 u", passes("\x1b[100;3 u")},
		{"too many fields", "\x1b[100;3;100;1u", passes("\x1b[100;3;100;1u")},
		{"too many alternates", "\x1b[100:1:2:3;3u", passes("\x1b[100:1:2:3;3u")},
		{"a code too long to be one", "\x1b[100000000;3u", passes("\x1b[100000000;3u")},
		{"the kitty flags reply", "\x1b[?15u", passes("\x1b[?15u")},
		{"modifyOtherKeys with the wrong modifier", "\x1b[27;5;100~", passes("\x1b[27;5;100~")},
		{"modifyOtherKeys with too many fields", "\x1b[27;3;100;1~", passes("\x1b[27;3;100;1~")},
		{"modifyOtherKeys with no code", "\x1b[27;3~", passes("\x1b[27;3~")},
	})
}

func TestScan_RepeatsAndReleasesAreSwallowed(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"a release", "\x1b[100;3:3u", scanned{}},
		{"a repeat", "\x1b[100;3:2u", scanned{}},
		{"a release with lock bits", "\x1b[111;69:3u", scanned{}},
		{"a release amid typing, the typing kept", "ab\x1b[100;3:3ucd", passes("abcd")},
		{"two releases", "\x1b[100;3:3u-\x1b[111;5:3u-", passes("--")},
		{"a release before a press", "x\x1b[100;3:3uy\x1b[113;3uz", scanned{forward: "xy", action: kill}},
		{"an unbound key's release passes", "\x1b[97;3:3u", passes("\x1b[97;3:3u")},
	})
}

func TestScan_ForwardsWhatPrecedesTheKeyAndDropsTheRest(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"typing then a ctrl key", "hello\x0fworld", scanned{forward: "hello", action: detach}},
		{"a line then a ctrl key", "ls\r\x0frm -rf /", scanned{forward: "ls\r", action: detach}},
		{"typing then a CSI-u key", "hello\x1b[100;3uworld", scanned{forward: "hello", action: detach}},
		{"the first key wins", "a\x1b[113;3ub\x0f", scanned{forward: "a", action: kill}},
		{"sequences before a CSI-u key", "\x1b[A\x1b[100;3u\x1b[B", scanned{forward: "\x1b[A", action: detach}},
	})
}

// A legacy alt key and an escape followed by a key send the same bytes; only
// a read holding nothing else is the alt key.
func TestScan_LegacyAltKeysOnlyAsAReadOfTheirOwn(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"a lone alt+d", "\x1bd", found(detach)},
		{"escape, then q and a register, typed together", "\x1bqa", passes("\x1bqa")},
		{"escape, then dd, typed together", "\x1bdd", passes("\x1bdd")},
		{"typing, then escape and q", "ihello\x1bq", passes("ihello\x1bq")},
		{"typing around escape and d", "hello\x1bdworld", passes("hello\x1bdworld")},
		{"escape, then ctrl+alt+x's bytes, then more", "\x1b\x18x", passes("\x1b\x18x")},
		{"an escape, then alt+d, is two keys", "\x1b\x1bd", passes("\x1b\x1bd")},
		{"a ctrl key after escape and d still matches", "\x1bd\x0f", scanned{forward: "\x1bd", action: detach}},
	})
}

func TestScan_OrdinaryInputPassesUnchanged(t *testing.T) {
	for _, read := range []string{
		"hello world\r",
		"\x1b",
		"abc\x1b",
		"\x7f\t",
		"héllo ✓",
		"\x1b[A\x1bOA\x1b[1;3D",
		"\x1b:wq\r",
		"\x1b[<0;10;5M\x1b[M #!",
		"\x1b[I\x1b[O",
		"",
	} {
		t.Run(read, func(t *testing.T) {
			assertScans(t, attachkeys.NewMatcher(bindingsFor(t, richKeys)), []string{read}, []scanned{passes(read)})
		})
	}
}

func TestScan_NoLegacyFormInsideAnotherSequence(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"a ctrl byte inside a string", "\x1b]52;c;\x0f\x07", passes("\x1b]52;c;\x0f\x07")},
		{"a ctrl byte inside an ST-ended string", "\x1bP\x0f\x1b\\", passes("\x1bP\x0f\x1b\\")},
		{"a ctrl byte inside a CSI sequence", "\x1b[1;\x0f5A", passes("\x1b[1;\x0f5A")},
		{"the byte an SS3 sequence carries", "\x1bO\x0f", passes("\x1bO\x0f")},
		{"a ctrl key after BEL ends a string", "\x1b]0;t\x07\x0f", scanned{forward: "\x1b]0;t\x07", action: detach}},
		{"a ctrl key after ST ends a string", "\x1b]0;t\x1b\\\x0f", scanned{forward: "\x1b]0;t\x1b\\", action: detach}},
		{"a ctrl key after an SS3 sequence", "\x1bOA\x0f", scanned{forward: "\x1bOA", action: detach}},
		{"an ESC ends an open string and starts its own", "\x1b]0;t\x1b[100;3u", scanned{forward: "\x1b]0;t", action: detach}},
		{"an ESC ends an open CSI sequence and starts its own", "\x1b[1;\x1b[100;3u", scanned{forward: "\x1b[1;", action: detach}},
	})
}

func TestScan_NothingInsideABracketedPaste(t *testing.T) {
	runScanCases(t, richKeys, []scanCase{
		{"keys inside a paste", "\x1b[200~\x1bd\x0fx\x1b[100;3u\x1b[201~", passes("\x1b[200~\x1bd\x0fx\x1b[100;3u\x1b[201~")},
		{"a key after the paste ends", "\x1b[200~text\x1b[201~\x0f", scanned{forward: "\x1b[200~text\x1b[201~", action: detach}},
		{"an ESC inside a paste restarts the end marker", "\x1b[200~\x1b\x1b[201~\x0f", scanned{forward: "\x1b[200~\x1b\x1b[201~", action: detach}},
		{"a near miss leaves the paste open", "\x1b[200~\x1b[202~\x0f", passes("\x1b[200~\x1b[202~\x0f")},
	})
}

func TestScan_APasteSpansReads(t *testing.T) {
	runReadsCases(t, []readsCase{
		{
			"a paste over several reads",
			[]string{"\x1b[200~abc", "\x1bd", "def\x1b[201~", "\x1bd"},
			[]scanned{passes("\x1b[200~abc"), passes("\x1bd"), passes("def\x1b[201~"), found(detach)},
		},
		{
			"both markers split across reads",
			[]string{"\x1b[20", "0~\x0f", "x\x1b[2", "01~\x0f"},
			[]scanned{passes("\x1b[20"), passes("0~\x0f"), passes("x\x1b[2"), {forward: "01~", action: detach}},
		},
		{
			"a start marker over three reads",
			[]string{"\x1b", "[2", "00~\x0f", "\x1b[201~", "\x0f"},
			[]scanned{passes("\x1b"), passes("[2"), passes("00~\x0f"), passes("\x1b[201~"), found(detach)},
		},
		{
			"a start marker begun and not finished",
			[]string{"\x1b[20", "x\x0f"},
			[]scanned{passes("\x1b[20"), {forward: "x", action: detach}},
		},
		{
			"the input a match drops is never read, so opens no paste",
			[]string{"\x0f\x1b[200~", "\x0f"},
			[]scanned{found(detach), found(detach)},
		},
	})
}

// Each read is parsed afresh: whatever one leaves open never reaches the next.
func TestScan_NoStateCrossesReads(t *testing.T) {
	runReadsCases(t, []readsCase{
		{"an ESC ending a read, then a ctrl key", []string{"abc\x1b", "\x0f"}, []scanned{passes("abc\x1b"), found(detach)}},
		{"an ESC ending a read, then a lone alt key", []string{"\x1b", "\x1bd"}, []scanned{passes("\x1b"), found(detach)}},
		{"a stray APC opener", []string{"\x1b_", "\x0f"}, []scanned{passes("\x1b_"), found(detach)}},
		{"a stray OSC opener", []string{"\x1b]0;title", "\x0f"}, []scanned{passes("\x1b]0;title"), found(detach)}},
		{"a stray DCS opener", []string{"\x1bP", "\x1b[100;3u"}, []scanned{passes("\x1bP"), found(detach)}},
		{"a stray SS3 opener", []string{"\x1bO", "\x0f"}, []scanned{passes("\x1bO"), found(detach)}},
		{"an open CSI sequence", []string{"\x1b[1;", "\x0f"}, []scanned{passes("\x1b[1;"), found(detach)}},
		{"a key after a split sequence's tail", []string{"\x1b[1;", "3D\x0f"}, []scanned{passes("\x1b[1;"), {forward: "3D", action: detach}}},
	})
}

func TestScan_SplitKeysAreMissed(t *testing.T) {
	runReadsCases(t, []readsCase{
		{"a legacy key", []string{"\x1b", "d"}, []scanned{passes("\x1b"), passes("d")}},
		{"a CSI-u key", []string{"\x1b[100;", "3u"}, []scanned{passes("\x1b[100;"), passes("3u")}},
		{"a release is not swallowed", []string{"\x1b[100;3:", "3u"}, []scanned{passes("\x1b[100;3:"), passes("3u")}},
	})
}

// A read can begin with the tail of a terminal's reply to a query. It is read
// as typing, so nothing it could hold is bindable.
func TestScan_SplitReplyTailsFindNothing(t *testing.T) {
	runReadsCases(t, []readsCase{
		{"an OSC reply ended by BEL", []string{"\x1b]11;rgb:00", "00/0000/0000\x07"}, []scanned{passes("\x1b]11;rgb:00"), passes("00/0000/0000\x07")}},
		{"an OSC reply ended by ST", []string{"\x1b]11;rgb:00", "00/0000/0000\x1b\\"}, []scanned{passes("\x1b]11;rgb:00"), passes("00/0000/0000\x1b\\")}},
		{"an ST read on its own", []string{"\x1b]11;rgb:0/0/0", "\x1b\\"}, []scanned{passes("\x1b]11;rgb:0/0/0"), passes("\x1b\\")}},
		{"an ST split in two", []string{"\x1b]11;rgb:0/0/0\x1b", "\\"}, []scanned{passes("\x1b]11;rgb:0/0/0\x1b"), passes("\\")}},
		{"a DCS reply", []string{"\x1bP>|kitty", "(0.40)\x1b\\"}, []scanned{passes("\x1bP>|kitty"), passes("(0.40)\x1b\\")}},
		{"an APC reply", []string{"\x1b_Gi=1", ";OK\x1b\\"}, []scanned{passes("\x1b_Gi=1"), passes(";OK\x1b\\")}},
		{"a cursor position report", []string{"\x1b[1;", "3R"}, []scanned{passes("\x1b[1;"), passes("3R")}},
		{"a reply's head read on its own", []string{"\x1b]", "11;rgb:0/0/0\x07"}, []scanned{passes("\x1b]"), passes("11;rgb:0/0/0\x07")}},
	})
}

func TestScan_DisabledActionMatchesNothing(t *testing.T) {
	runScanCases(t, `{"kill":[]}`, []scanCase{
		{"its legacy form", "\x1bq", passes("\x1bq")},
		{"its CSI-u form", "\x1b[113;3u", passes("\x1b[113;3u")},
		{"its release", "\x1b[113;3:3u", passes("\x1b[113;3:3u")},
		{"another action still matches", "\x1bd", found(detach)},
	})
}

func TestScan_NoBindingsForwardsEverything(t *testing.T) {
	m := attachkeys.NewMatcher(attachkeys.Bindings{})
	reads := []string{"\x1bd", "\x0f", "\x1b[100;3u", "\x1b[100;3:3u", "\x1b[200~", "x"}
	want := make([]scanned, len(reads))
	for i, read := range reads {
		want[i] = passes(read)
	}
	assertScans(t, m, reads, want)
}

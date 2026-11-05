package capture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// demoDir is where the README's demo tapes live, demo/ at the repository's
// root.
const demoDir = "../../demo"

// defaultTypingSpeed is how long vhs takes over each key it types where the
// tape sets no TypingSpeed.
const defaultTypingSpeed = 50 * time.Millisecond

// tape is a demo tape as vhs plays it, read for what reaches the program it
// records: its Output and settings; what's typed to the shell before its first
// Wait+Screen, which starts the program; and from that wait on, which is the
// tape's zero, each key at its moment, the screens it waits for, and the spans
// it records.
type tape struct {
	name     string
	output   string
	settings map[string]string
	// prelude is what's typed to the shell before the first Wait+Screen, an
	// Enter as the carriage return it sends; preludeHidden is whether all of it
	// was typed hidden, with nothing shown before the program drew.
	prelude       string
	preludeHidden bool
	keys          []tapeKey
	waits         []screenWait
	shown         []span
	// ends is the tape's last moment, from its zero.
	ends time.Duration
}

// tapeKey is a key a tape presses once its program runs: as Bubble Tea decodes
// it, when, and whether the tape is hidden as it's pressed.
type tapeKey struct {
	at     time.Duration
	key    tea.KeyPressMsg
	hidden bool
}

// screenWait is a Wait+Screen once the program runs: what it waits to see, at
// the moment it's reached.
type screenWait struct {
	at time.Duration
	re *regexp.Regexp
}

// span is a stretch the tape records, between a Show and the Hide after it,
// and how many keys the tape had pressed by its end.
type span struct {
	from, to   time.Duration
	keysBefore int
}

// lastShown is the tape's last recorded span: a still's art is its last frame.
func (tp tape) lastShown() (span, bool) {
	if len(tp.shown) == 0 {
		return span{}, false
	}
	return tp.shown[len(tp.shown)-1], true
}

// tail are the keys the tape presses once it's done recording.
func (tp tape) tail() []tapeKey {
	last, ok := tp.lastShown()
	if !ok {
		return tp.keys
	}
	return tp.keys[last.keysBefore:]
}

// recorded are the keys the tape presses before it's done recording, hidden or
// shown.
func (tp tape) recorded() []tapeKey {
	return tp.keys[:len(tp.keys)-len(tp.tail())]
}

// readTape reads and parses demo/<name>.tape.
func readTape(t *testing.T, name string) tape {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(demoDir, name+".tape"))
	if err != nil {
		t.Fatalf("read the %s tape: %v", name, err)
	}
	tp, err := parseTape(name, string(src))
	if err != nil {
		t.Fatalf("demo/%s.tape: %v", name, err)
	}
	return tp
}

// demoTapeNames are the names of every tape in demo/, sorted.
func demoTapeNames(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(demoDir, "*.tape"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("demo/ holds no tapes")
	}
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = strings.TrimSuffix(filepath.Base(path), ".tape")
	}
	slices.Sort(names)
	return names
}

// tapeSettings are the settings a tape may make: vhs's others, such as
// PlaybackSpeed or LoopOffset, would play the tape otherwise than its keys are
// timed, so a tape that makes one is refused.
var tapeSettings = []string{
	"Shell", "FontFamily", "FontSize", "LetterSpacing", "LineHeight", "Width", "Height",
	"Padding", "CursorBlink", "Framerate", "Theme", "WaitTimeout", "TypingSpeed",
}

// namedKeys are the keys a tape names, as a terminal sends each.
var namedKeys = map[string]string{
	"Enter":     "\r",
	"Space":     " ",
	"Escape":    "\x1b",
	"Tab":       "\t",
	"Backspace": "\x7f",
	"Up":        "\x1b[A",
	"Down":      "\x1b[B",
	"Right":     "\x1b[C",
	"Left":      "\x1b[D",
}

// shiftTab is shift+tab as a terminal sends it.
const shiftTab = "\x1b[Z"

// ctrlKey is a Ctrl+<letter> command, and the letter.
var ctrlKey = regexp.MustCompile(`^Ctrl\+([A-Za-z])$`)

// ctrlSent is what a terminal sends for Ctrl and the letter: the control
// character numbered by the letter's place in the alphabet.
func ctrlSent(letter string) string {
	return string(rune(strings.ToLower(letter)[0] - 'a' + 1))
}

// tapeParser reads a tape a line at a time, keeping the clock and what's
// hidden as vhs does.
type tapeParser struct {
	tp      tape
	speed   time.Duration
	hidden  bool
	started bool
	acted   bool
	clock   time.Duration
	showing time.Duration
}

// parseTape parses a tape in the subset of vhs's language the demos use:
// comments; Output; Set; Hide and Show; Type; Sleep; Enter, Space, Escape,
// Tab, Backspace and the arrows, each with an optional speed and count;
// Shift+Tab; Ctrl+<letter>; and Wait+Screen. Anything else is an error, as is a
// setting made after the tape has acted, but its typing speed.
func parseTape(name, src string) (tape, error) {
	p := &tapeParser{
		tp:      tape{name: name, settings: map[string]string{}, preludeHidden: true},
		speed:   defaultTypingSpeed,
		showing: -1,
	}
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := p.command(line); err != nil {
			return tape{}, fmt.Errorf("line %d, %q: %w", i+1, line, err)
		}
	}
	if !p.started {
		return tape{}, errors.New("no Wait+Screen: nothing says when the program has drawn")
	}
	p.closeSpan()
	p.tp.ends = p.clock
	return p.tp, nil
}

// command reads one command, and the duration after its name where it takes
// one: a key's typing speed, or a wait's timeout.
func (p *tapeParser) command(line string) error {
	word, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	word, speedArg, timed := strings.Cut(word, "@")
	speed := p.speed
	if timed {
		d, err := parseTapeDuration(speedArg)
		if err != nil {
			return err
		}
		speed = d
	}
	if timed && !takesSpeed(word) {
		return fmt.Errorf("%s takes no speed", word)
	}
	switch word {
	case "Output":
		return p.output(rest)
	case "Set":
		return p.set(rest)
	case "Hide", "Show":
		return p.toggle(word == "Hide", rest)
	case "Type":
		text, err := unquoted(rest)
		if err != nil {
			return err
		}
		return p.typeText(text, speed)
	case "Sleep":
		return p.sleep(rest)
	case "Wait+Screen":
		return p.wait(rest)
	case "Shift+Tab":
		return p.single(word, rest, shiftTab)
	}
	if m := ctrlKey.FindStringSubmatch(word); m != nil {
		return p.single(word, rest, ctrlSent(m[1]))
	}
	if sent, ok := namedKeys[word]; ok {
		return p.named(sent, rest, speed)
	}
	return fmt.Errorf("%s is no command the demo tapes use", word)
}

// takesSpeed reports whether the command takes a speed after its name.
func takesSpeed(word string) bool {
	_, named := namedKeys[word]
	return named || word == "Type" || word == "Wait+Screen"
}

func (p *tapeParser) output(rest string) error {
	path, err := unquoted(rest)
	if err != nil {
		return err
	}
	if p.acted {
		return errors.New("Output comes before the tape acts")
	}
	p.tp.output = path
	return nil
}

// set makes a setting, refusing any vhs would play otherwise than timed, and
// any but the typing speed once the tape has acted, as vhs ignores those.
func (p *tapeParser) set(rest string) error {
	key, value, _ := strings.Cut(rest, " ")
	value = strings.TrimSpace(value)
	switch {
	case !slices.Contains(tapeSettings, key):
		return fmt.Errorf("the setting %s is none the demo tapes make", key)
	case value == "":
		return fmt.Errorf("the setting %s has no value", key)
	case p.acted && key != "TypingSpeed":
		return fmt.Errorf("the setting %s comes after the tape has acted, so vhs ignores it", key)
	case key == "TypingSpeed":
		d, err := parseTapeDuration(value)
		if err != nil {
			return err
		}
		p.speed = d
	}
	p.tp.settings[key] = value
	return nil
}

// toggle hides or shows the tape, refusing what's already so.
func (p *tapeParser) toggle(hide bool, rest string) error {
	if rest != "" {
		return errors.New("Hide and Show take nothing")
	}
	if hide == p.hidden {
		return errors.New("the tape is already so")
	}
	p.acted = true
	p.hidden = hide
	switch {
	case !p.started:
		p.tp.preludeHidden = p.tp.preludeHidden && hide
	case hide:
		p.closeSpan()
	default:
		p.showing = p.clock
	}
	return nil
}

// closeSpan ends the span the tape is recording, once its program runs.
func (p *tapeParser) closeSpan() {
	if p.showing < 0 {
		return
	}
	p.tp.shown = append(p.tp.shown, span{from: p.showing, to: p.clock, keysBefore: len(p.tp.keys)})
	p.showing = -1
}

// typeText types each character in turn, a key at a time.
func (p *tapeParser) typeText(text string, speed time.Duration) error {
	if text == "" {
		return errors.New("Type types nothing")
	}
	for _, r := range text {
		if err := p.press(string(r), speed); err != nil {
			return err
		}
	}
	return nil
}

// named presses a named key, as many times as its count says, once where it
// gives none.
func (p *tapeParser) named(sent, rest string, speed time.Duration) error {
	count := 1
	if rest != "" {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 1 {
			return fmt.Errorf("the count %q is no count of presses", rest)
		}
		count = n
	}
	for range count {
		if err := p.press(sent, speed); err != nil {
			return err
		}
	}
	return nil
}

// single presses a chord, which takes neither a speed nor a count.
func (p *tapeParser) single(word, rest, sent string) error {
	if rest != "" {
		return fmt.Errorf("%s takes nothing", word)
	}
	return p.press(sent, p.speed)
}

// press sends what a key sends, then waits the speed, as vhs does: to the shell
// before the program runs, and to the program after.
func (p *tapeParser) press(sent string, speed time.Duration) error {
	p.acted = true
	if !p.started {
		p.tp.prelude += sent
		p.tp.preludeHidden = p.tp.preludeHidden && p.hidden
		return nil
	}
	key, err := decodedKey(sent)
	if err != nil {
		return err
	}
	p.tp.keys = append(p.tp.keys, tapeKey{at: p.clock, key: key, hidden: p.hidden})
	p.clock += speed
	return nil
}

func (p *tapeParser) sleep(rest string) error {
	d, err := parseTapeDuration(rest)
	if err != nil {
		return err
	}
	p.acted = true
	if p.started {
		p.clock += d
	}
	return nil
}

// wait reads a Wait+Screen: the first starts the tape's clock, the program
// having drawn.
func (p *tapeParser) wait(rest string) error {
	if len(rest) < 2 || !strings.HasPrefix(rest, "/") || !strings.HasSuffix(rest, "/") {
		return errors.New("Wait+Screen takes a /regular expression/")
	}
	re, err := regexp.Compile(rest[1 : len(rest)-1])
	if err != nil {
		return err
	}
	p.acted = true
	if !p.started {
		p.started = true
		if !p.hidden {
			p.tp.preludeHidden = false
			p.showing = 0
		}
	}
	p.tp.waits = append(p.tp.waits, screenWait{at: p.clock, re: re})
	return nil
}

// unquoted is a double-quoted string's text: vhs reads no escapes in it.
func unquoted(s string) (string, error) {
	if len(s) < 2 || !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) || strings.Count(s, `"`) != 2 {
		return "", fmt.Errorf("%s is no double-quoted string", s)
	}
	return s[1 : len(s)-1], nil
}

// parseTapeDuration reads a duration as a tape gives one, its unit given.
func parseTapeDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%q is no duration, such as 500ms or 2s", s)
	}
	return d, nil
}

// decodedKey is the key Bubble Tea decodes from what a terminal sends: exactly
// one key press, from all of it.
func decodedKey(sent string) (tea.KeyPressMsg, error) {
	var decoder uv.EventDecoder
	n, event := decoder.Decode([]byte(sent))
	key, ok := event.(uv.KeyPressEvent)
	if !ok || n != len(sent) {
		return tea.KeyPressMsg{}, fmt.Errorf("%q decodes to %#v, no single key press", sent, event)
	}
	return tea.KeyPressMsg(key), nil
}

func TestTheTapeParserReadsWhatTheTapesUse(t *testing.T) {
	src := `# A comment, and a blank line after it.

Output "demo/out/sample/"
Set Shell "bash"
Set Theme { "name": "x", "background": "#000000" }
Set TypingSpeed 100ms
Hide
Type "TZ=UTC demo/out/capturetool --scenario x"
Enter
Wait+Screen@10s /P O R T A L/
Show
Sleep 1s
Type "s/"
Type@200ms "a b"
Set TypingSpeed 10ms
Down 2
Up@300ms
Enter@50ms 2
Space
Escape
Tab
Backspace
Left
Right
Shift+Tab
Wait+Screen /Sessions/
Hide
Ctrl+C
`
	tp, err := parseTape("sample", src)
	if err != nil {
		t.Fatal(err)
	}
	if tp.output != "demo/out/sample/" || tp.settings["Shell"] != `"bash"` || tp.settings["Theme"] != `{ "name": "x", "background": "#000000" }` {
		t.Errorf("read Output %q and settings %q", tp.output, tp.settings)
	}
	if tp.prelude != "TZ=UTC demo/out/capturetool --scenario x\r" || !tp.preludeHidden {
		t.Errorf("the prelude reads %q, hidden %t, want the command and its Enter, hidden", tp.prelude, tp.preludeHidden)
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	want := []tapeKey{
		{at: ms(1000), key: keyRune('s')},
		{at: ms(1100), key: keyRune('/')},
		{at: ms(1200), key: keyRune('a')},
		{at: ms(1400), key: keySpace},
		{at: ms(1600), key: keyRune('b')},
		{at: ms(1800), key: keyDown},
		{at: ms(1810), key: keyDown},
		{at: ms(1820), key: keyUp},
		{at: ms(2120), key: keyEnter()},
		{at: ms(2170), key: keyEnter()},
		{at: ms(2220), key: keySpace},
		{at: ms(2230), key: keyEsc},
		{at: ms(2240), key: tea.KeyPressMsg{Code: tea.KeyTab}},
		{at: ms(2250), key: tea.KeyPressMsg{Code: tea.KeyBackspace}},
		{at: ms(2260), key: tea.KeyPressMsg{Code: tea.KeyLeft}},
		{at: ms(2270), key: tea.KeyPressMsg{Code: tea.KeyRight}},
		{at: ms(2280), key: keyShiftTab},
		{at: ms(2290), key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, hidden: true},
	}
	if !slices.Equal(tp.keys, want) {
		t.Errorf("read the keys\n%v\nwant\n%v", tp.keys, want)
	}
	if len(tp.waits) != 2 || tp.waits[0].at != 0 || tp.waits[0].re.String() != "P O R T A L" || tp.waits[1].at != ms(2290) || tp.waits[1].re.String() != "Sessions" {
		t.Errorf("read the waits %v, want P O R T A L at the zero and Sessions at 2.29s", tp.waits)
	}
	if want := []span{{from: 0, to: ms(2290), keysBefore: len(want) - 1}}; !slices.Equal(tp.shown, want) {
		t.Errorf("read the shown spans %v, want %v", tp.shown, want)
	}
	if tp.ends != ms(2300) {
		t.Errorf("the tape ends at %s, want 2.3s", tp.ends)
	}
	if tail := tp.tail(); len(tail) != 1 || !tail[0].hidden {
		t.Errorf("the tail is %v, want the Ctrl+C alone", tail)
	}
}

func TestTheTapeParserRefusesWhatItDoesntKnow(t *testing.T) {
	const start = "Hide\nType \"x\"\nEnter\nWait+Screen /x/\n"
	tests := map[string]string{
		"an unknown command":                   start + "Screenshot \"x.png\"\n",
		"a key it doesn't name":                start + "PageDown\n",
		"Alt chords":                           start + "Alt+x\n",
		"Ctrl with no letter":                  start + "Ctrl+1\n",
		"a setting that retimes a tape":        "Set PlaybackSpeed 2\n" + start,
		"a setting after the tape acts":        start + "Set FontSize 17\n",
		"a setting with no value":              "Set FontSize\n" + start,
		"Output after the tape acts":           start + "Output \"demo/out/x/\"\n",
		"an unquoted Output":                   "Output demo/out/x/\n" + start,
		"an unquoted Type":                     start + "Type x\n",
		"a single-quoted Type":                 start + "Type 'x'\n",
		"an empty Type":                        start + "Type \"\"\n",
		"a Sleep with no unit":                 start + "Sleep 2\n",
		"a negative Sleep":                     start + "Sleep -1s\n",
		"a speed with no unit":                 start + "Down@50 2\n",
		"a speed on a command that takes none": start + "Sleep@1s 1s\n",
		"a count that isn't one":               start + "Down 0\n",
		"a count on a chord":                   start + "Shift+Tab 2\n",
		"a wait with no slashes":               start + "Wait+Screen x\n",
		"a wait's bad expression":              start + "Wait+Screen /(/\n",
		"Hide when hidden":                     "Hide\nHide\n" + start,
		"Show when shown":                      start + "Show\nShow\n",
		"Hide with an argument":                start + "Hide now\n",
		"no Wait+Screen":                       "Hide\nType \"x\"\nEnter\n",
		"Wait with no screen":                  start + "Wait /x/\n",
	}
	if _, err := parseTape("refused", start); err != nil {
		t.Fatalf("the tape each refusal adds to doesn't parse: %v", err)
	}
	for name, src := range tests {
		if _, err := parseTape("refused", src); err == nil {
			t.Errorf("%s: parsed, want it refused", name)
		}
	}
}

// Each key a tape types takes its typing speed, and the clock counts from the
// first Wait+Screen: what's typed before it goes to the shell and takes no
// time on the program's clock.
func TestTypedKeysAreTimedByTypingSpeed(t *testing.T) {
	tests := []struct {
		name, src string
		// ms are when each key lands, in milliseconds from the zero.
		ms []time.Duration
	}{
		{name: "the default speed", src: `Type "abc"`, ms: []time.Duration{0, 50, 100}},
		{name: "the speed set", src: "Set TypingSpeed 80ms\nHide\nType \"x\"\nEnter\nWait+Screen /x/\nType \"abc\"", ms: []time.Duration{0, 80, 160}},
		{name: "the command's own speed", src: `Type@300ms "ab"` + "\nType \"c\"", ms: []time.Duration{0, 300, 600}},
		{name: "a count of presses", src: "Down@120ms 3\nUp", ms: []time.Duration{0, 120, 240, 360}},
		{name: "a sleep between", src: "Type \"a\"\nSleep 450ms\nType \"b\"", ms: []time.Duration{0, 500}},
	}
	for _, tt := range tests {
		src := tt.src
		if !strings.Contains(src, "Wait+Screen") {
			src = "Hide\nType \"capturetool\"\nEnter\nWait+Screen /x/\n" + src
		}
		tp, err := parseTape(tt.name, src)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		var at, want []time.Duration
		for _, k := range tp.keys {
			at = append(at, k.at)
		}
		for _, ms := range tt.ms {
			want = append(want, ms*time.Millisecond)
		}
		if !slices.Equal(at, want) {
			t.Errorf("%s: the keys land at %v, want %v", tt.name, at, want)
		}
	}
}

// A tape's keys reach the program as Bubble Tea decodes what the terminal
// sends: a space carries its text, a letter its own, an uppercase one shift,
// and the named keys and chords none.
func TestTapeKeysAreAsBubbleTeaDecodesThem(t *testing.T) {
	tests := map[string]tea.KeyPressMsg{
		"Space":     {Code: tea.KeySpace, Text: " "},
		"Enter":     {Code: tea.KeyEnter},
		"Escape":    {Code: tea.KeyEscape},
		"Tab":       {Code: tea.KeyTab},
		"Backspace": {Code: tea.KeyBackspace},
		"Up":        {Code: tea.KeyUp},
		"Down":      {Code: tea.KeyDown},
		"Left":      {Code: tea.KeyLeft},
		"Right":     {Code: tea.KeyRight},
		"Shift+Tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"Ctrl+C":    {Code: 'c', Mod: tea.ModCtrl},
		`Type "s"`:  {Code: 's', Text: "s"},
		`Type " "`:  {Code: tea.KeySpace, Text: " "},
		`Type "/"`:  {Code: '/', Text: "/"},
		`Type "S"`:  {Code: 's', ShiftedCode: 'S', Text: "S", Mod: tea.ModShift},
	}
	for command, want := range tests {
		tp, err := parseTape("keys", "Hide\nType \"x\"\nEnter\nWait+Screen /x/\n"+command)
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if len(tp.keys) != 1 || tp.keys[0].key != want {
			t.Errorf("%s presses %#v, want %#v", command, tp.keys, want)
		}
	}
}

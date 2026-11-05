package capture

import (
	"go/token"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// player plays a model as a Bubble Tea program would, on a synctest bubble's
// virtual clock, so a scenario's timers, cues and keys land at their moments in
// no time at all: each command runs in a goroutine of its own and posts what it
// returns, and the player delivers what's posted as the clock reaches it, in
// the order the commands were run, so a play is the same every time. Posting
// never blocks, and the play's end waits out on the clock every command still
// running, the timers the model set among them, so none outlives the bubble.
type player struct {
	t     *testing.T
	start time.Time
	size  Size
	model tea.Model
	// runs numbers the commands run, and quit is whether the model has quit,
	// after which nothing more is delivered.
	runs int
	quit bool
	// running are the commands run that haven't yet returned.
	running sync.WaitGroup

	mu     sync.Mutex
	outbox []post
	// posted wakes the player when a command posts.
	posted chan struct{}
}

// post is a message a command returned, and where it falls in the order the
// commands were run: its command's number, then its place in each batch or
// sequence it came from.
type post struct {
	order []int
	msg   tea.Msg
}

// playScenario plays the scenario on a terminal of the given size, its clock
// starting now. It must be called inside synctest.Test, as the scenario reads
// the bubble's clock.
func playScenario(t *testing.T, s Scenario, size Size) *player {
	t.Helper()
	return playing(t, s.Model(), size)
}

// playing plays m on a terminal of the given size as a program starts one: the
// terminal's size first, then the model's Init.
func playing(t *testing.T, m tea.Model, size Size) *player {
	t.Helper()
	p := &player{t: t, start: time.Now(), size: size, model: m, posted: make(chan struct{}, 1)}
	t.Cleanup(p.finish)
	p.deliver(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})
	p.run(p.model.Init())
	p.settle()
	return p
}

// until plays on till at into the play, delivering everything due by then.
func (p *player) until(at time.Duration) {
	timer := time.NewTimer(time.Until(p.start.Add(at)))
	defer timer.Stop()
	for {
		p.settle()
		select {
		case <-p.posted:
		case <-timer.C:
			p.settle()
			return
		}
	}
}

// press plays on till at, then presses each key in turn.
func (p *player) press(at time.Duration, keys ...tea.KeyPressMsg) {
	p.until(at)
	for _, key := range keys {
		p.deliver(key)
		p.settle()
	}
}

// screen is the model's screen as text, a line per row of the terminal, as a
// golden frame's text keeps it.
func (p *player) screen() string {
	rows := strings.Split(p.model.View().Content, "\n")
	rows = rows[:min(len(rows), p.size.Height)]
	for len(rows) < p.size.Height {
		rows = append(rows, "")
	}
	return Plain(strings.Join(rows, "\n") + "\n")
}

// rows are the model's screen's rows as drawn, escape codes and all.
func (p *player) rows() []string {
	return strings.Split(p.model.View().Content, "\n")
}

// scenarioModel is the scenario's model as the play has left it.
func (p *player) scenarioModel() ScenarioModel {
	p.t.Helper()
	m, ok := p.model.(ScenarioModel)
	if !ok {
		p.t.Fatalf("the player plays %T, want a scenario", p.model)
	}
	return m
}

// settle delivers everything posted by now, and what that leads to at once,
// until every command is waiting on the clock.
func (p *player) settle() {
	for {
		synctest.Wait()
		due := p.take()
		if len(due) == 0 {
			return
		}
		for _, d := range due {
			p.deliver(d.msg)
		}
	}
}

// take empties the outbox, in the order its messages' commands were run.
func (p *player) take() []post {
	p.mu.Lock()
	defer p.mu.Unlock()
	due := p.outbox
	p.outbox = nil
	slices.SortStableFunc(due, func(a, b post) int { return slices.Compare(a.order, b.order) })
	return due
}

// deliver hands the model msg, as a program does, and runs the command it
// answers with: but a quit, which ends the play, and the program's own
// requests of the terminal, which no terminal answers here.
func (p *player) deliver(msg tea.Msg) {
	switch {
	case p.quit || isTerminalRequest(msg):
		return
	case isQuit(msg):
		p.quit = true
		return
	}
	next, cmd := p.model.Update(msg)
	p.model = next
	p.run(cmd)
}

// run runs cmd in a goroutine of its own, numbered after every command before.
func (p *player) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	p.runs++
	order := []int{p.runs}
	p.running.Go(func() { p.exec(order, cmd) })
}

// finish ends the play: nothing more is delivered, and every command still
// running is waited for, the clock moving on to each timer it waits on, so the
// bubble ends with none blocked.
func (p *player) finish() {
	p.quit = true
	p.running.Wait()
}

// exec runs cmd, and posts what it returns, at its place in the order.
func (p *player) exec(order []int, cmd tea.Cmd) {
	p.expand(order, cmd())
}

// expand posts msg, or where it's commands, runs them as a program does: a
// batch's at once, waiting for them all, and a sequence's one at a time, in
// order. Bubble Tea's sequence is unexported, so it's told by its shape, a
// slice of commands.
func (p *player) expand(order []int, msg tea.Msg) {
	cmds, inTurn := commandsIn(msg)
	switch {
	case cmds == nil:
		p.post(order, msg)
	case inTurn:
		for i, cmd := range cmds {
			if cmd != nil {
				p.exec(placed(order, i), cmd)
			}
		}
	default:
		var all sync.WaitGroup
		for i, cmd := range cmds {
			if cmd != nil {
				all.Go(func() { p.exec(placed(order, i), cmd) })
			}
		}
		all.Wait()
	}
}

// post puts msg in the outbox at its place in the order, and wakes the player.
func (p *player) post(order []int, msg tea.Msg) {
	if msg == nil {
		return
	}
	p.mu.Lock()
	p.outbox = append(p.outbox, post{order: order, msg: msg})
	p.mu.Unlock()
	select {
	case p.posted <- struct{}{}:
	default:
	}
}

// placed is the place of a batch's or sequence's i-th command, within the
// place of the command that returned it.
func placed(order []int, i int) []int {
	return append(slices.Clone(order), i)
}

// commandsIn are the commands msg carries, and whether they run in turn: a
// batch's, at once; a sequence's, or any other slice of commands, in turn; none
// for any other message.
func commandsIn(msg tea.Msg) (cmds []tea.Cmd, inTurn bool) {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch, false
	}
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds = make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

// isQuit reports whether msg is the program's quit.
func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}

// teaPackage is Bubble Tea's import path, as its types carry it.
var teaPackage = reflect.TypeFor[tea.QuitMsg]().PkgPath()

// isTerminalRequest reports whether msg is one of Bubble Tea's unexported
// messages, the requests a program makes of its terminal, such as for its
// background colour.
func isTerminalRequest(msg tea.Msg) bool {
	t := reflect.TypeOf(msg)
	return t != nil && t.PkgPath() == teaPackage && !token.IsExported(t.Name())
}

// clockRecorder is a model that notes when each message it's handed arrives,
// for the player's own tests: its Init is what it was given, and its answer to
// a message what answers makes of it then, as a tick's timer starts as it's
// made.
type clockRecorder struct {
	start   time.Time
	init    tea.Cmd
	answers map[string]func() tea.Cmd
	arrived *[]arrival
}

// arrival is a message a clockRecorder was handed, and how long into the play.
type arrival struct {
	msg string
	at  time.Duration
}

func (c clockRecorder) Init() tea.Cmd { return c.init }

func (c clockRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	name, ok := msg.(string)
	if !ok {
		return c, nil
	}
	*c.arrived = append(*c.arrived, arrival{msg: name, at: time.Since(c.start)})
	if answer, ok := c.answers[name]; ok {
		return c, answer()
	}
	return c, nil
}

func (c clockRecorder) View() tea.View { return tea.NewView("") }

// says is a command answering msg at once.
func says(msg string) tea.Cmd {
	return func() tea.Msg { return msg }
}

// saysAfter is a command answering msg once d has passed from now.
func saysAfter(d time.Duration, msg string) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

func TestThePlayerFiresTicksAtTheirVirtualTimes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var arrived []arrival
		model := clockRecorder{
			start: time.Now(),
			init: tea.Batch(
				saysAfter(time.Second, "tick"),
				// Both timers start now: the sequence holds the sooner till the
				// later has landed.
				tea.Sequence(saysAfter(2*time.Second, "later"), saysAfter(500*time.Millisecond, "sooner")),
				says("at once"),
			),
			answers: map[string]func() tea.Cmd{
				"tick":    func() tea.Cmd { return saysAfter(250*time.Millisecond, "tock") },
				"sooner":  func() tea.Cmd { return tea.Quit },
				"at once": func() tea.Cmd { return saysAfter(3*time.Second, "after the quit") },
			},
			arrived: &arrived,
		}

		p := playing(t, model, harnessSize)
		p.until(1100 * time.Millisecond)
		if got := slices.Clone(arrived); !slices.Equal(got, []arrival{{"at once", 0}, {"tick", time.Second}}) {
			t.Errorf("by 1.1s, %v arrived, want what's answered at once at 0, then the tick at 1s", got)
		}
		p.until(4 * time.Second)
		want := []arrival{
			{"at once", 0}, {"tick", time.Second}, {"tock", 1250 * time.Millisecond},
			{"later", 2 * time.Second}, {"sooner", 2 * time.Second},
		}
		if !slices.Equal(arrived, want) {
			t.Errorf("by 4s, %v arrived, want %v", arrived, want)
		}
		if !p.quit {
			t.Error("the model quit, but the player played on, want the quit recorded")
		}
	})
}

// Commands that answer at the same moment are delivered in the order they
// were run, however their goroutines are scheduled.
func TestThePlayerDeliversWhatsDueAtOnceInTheOrderItWasRun(t *testing.T) {
	for range 20 {
		synctest.Test(t, func(t *testing.T) {
			var arrived []arrival
			model := clockRecorder{
				start:   time.Now(),
				init:    tea.Batch(says("a"), saysAfter(time.Second, "b"), says("c"), saysAfter(time.Second, "d")),
				arrived: &arrived,
			}
			playing(t, model, harnessSize).until(time.Second)
			var order []string
			for _, a := range arrived {
				order = append(order, a.msg)
			}
			if !slices.Equal(order, []string{"a", "c", "b", "d"}) {
				t.Errorf("delivered %v, want a and c at once, then b and d, each in the order run", order)
			}
		})
	}
}

func TestThePlayerDropsTheProgramsRequestsOfItsTerminal(t *testing.T) {
	for _, tt := range []struct {
		msg  tea.Msg
		want bool
	}{
		{msg: tea.RequestBackgroundColor(), want: true},
		{msg: tea.WindowSizeMsg{}, want: false},
		{msg: tea.QuitMsg{}, want: false},
		{msg: "a message of the model's own", want: false},
		{msg: nil, want: false},
	} {
		if got := isTerminalRequest(tt.msg); got != tt.want {
			t.Errorf("isTerminalRequest(%T) = %t, want %t", tt.msg, got, tt.want)
		}
	}
}

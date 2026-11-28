package zmx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/proctable"
)

// errNoProgram is a session whose Info names no process it runs.
var errNoProgram = errors.New("the session names no process it runs")

// shells are the kernel's names for the shells the walk follows into their
// foreground job. A login shell's name, unlike its argv[0], has no dash.
var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true,
	"ksh": true, "mksh": true, "tcsh": true, "csh": true,
}

// WorkDir reads the session as Session does, then the directory of the
// process its work runs in, as workDir finds it: Claude under Portal's sh -c
// wrappers, which draw no prompt and report no directory, a shell at its
// prompt, or the program a shell runs in the foreground. When that is the
// directory the session records, under another spelling, as a symlinked path
// a shell reported is, the recorded spelling stands (PreferRecorded). Where
// no directory can be read (no process named, the process table unreadable,
// every process on the way gone, or off darwin), it is the directory the
// session records. The session's answer and the comparison are bounded by
// the probe ceiling; the process table's listing and the directory reads are
// the kernel's, which take no context.
func (b *Backend) WorkDir(ctx context.Context, name string) (string, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	i, err := b.askWithin(ctx, name)
	if err != nil {
		return "", fmt.Errorf("read the work directory of session %q: %w", name, err)
	}
	hostname, _ := b.hostname()
	recorded := sessionOf(name, i, hostname).Dir
	found, err := b.processes.workDir(int(i.pid))
	if err != nil || found == "" {
		return recorded, nil
	}
	return preferRecorded(ctx, b.stat, recorded, found), nil
}

// PreferRecorded is found, the directory a session's work was found in, as
// recorded spells it when recorded, the directory the session records, is
// the same directory on disk, however each spells it: through links, or in
// another case on a filesystem that folds it. Equal spellings are taken as
// they are; any other comparison reads both from disk, and one that hasn't
// answered when ctx ends, as on a hung network mount, or that ctx has
// already ended, gives found.
func PreferRecorded(ctx context.Context, recorded, found string) string {
	return preferRecorded(ctx, os.Stat, recorded, found)
}

func preferRecorded(ctx context.Context, stat func(string) (fs.FileInfo, error), recorded, found string) string {
	if recorded == "" || recorded == found || ctx.Err() != nil {
		return found
	}
	// Buffered, so a stat that answers after ctx has ended leaves nothing
	// waiting on it.
	same := make(chan bool, 1)
	go func() {
		recordedInfo, err := stat(recorded)
		if err != nil {
			same <- false
			return
		}
		foundInfo, err := stat(found)
		same <- err == nil && os.SameFile(recordedInfo, foundInfo)
	}()
	select {
	case ok := <-same:
		if ok {
			return recorded
		}
	case <-ctx.Done():
	}
	return found
}

// workDir walks, in one listing of the table, from pid, the session's
// program, down each shell's foreground job, and reads the directory of the
// process it stops at: a shell with no child on the way to the foreground
// group of the program's terminal, or several, as a pipeline has, or any
// process that is not a shell. A child on the way is one in that group, or
// the ancestor of one, as a shell nested in another's job is; so a job in a
// group of its own, stopped or in the background, a zombie or a process
// exiting is not followed. When the directory of the process it stops at
// can't be read, it is the deepest one on the way that can. A listing holds
// no cycle, but the walk takes at most a step per process all the same.
func (t processTable) workDir(pid int) (string, error) {
	if pid <= 0 {
		return "", errNoProgram
	}
	procs, err := t.list()
	if err != nil {
		return "", err
	}
	byPID := make(map[int]proctable.Process, len(procs))
	children := make(map[int][]proctable.Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p)
	}
	onTheWay := towardsForeground(procs, byPID, byPID[pid])
	walked := []int{pid}
	for range len(procs) {
		next, ok := foregroundChild(byPID[pid], children[pid], onTheWay)
		if !ok {
			break
		}
		pid = next.PID
		walked = append(walked, pid)
	}
	var dir string
	for _, pid := range slices.Backward(walked) {
		if dir, err = t.cwd(pid); err == nil && dir != "" {
			return dir, nil
		}
	}
	return "", err
}

// towardsForeground is every process on the way to the foreground group of
// program's terminal: each live process in that group, and each of its
// ancestors. A program on no terminal has no foreground, so none.
func towardsForeground(procs []proctable.Process, byPID map[int]proctable.Process, program proctable.Process) map[int]bool {
	onTheWay := map[int]bool{}
	if !program.Terminal {
		return onTheWay
	}
	for _, p := range procs {
		if p.Group != program.ForegroundGroup || p.Gone {
			continue
		}
		for at := p.PID; at > 0 && !onTheWay[at]; at = byPID[at].PPID {
			onTheWay[at] = true
		}
	}
	return onTheWay
}

// foregroundChild is the child of p the walk moves to: when p is a shell, its
// one child on the way to the foreground that is neither a zombie nor
// exiting.
func foregroundChild(p proctable.Process, children []proctable.Process, onTheWay map[int]bool) (proctable.Process, bool) {
	if !shells[p.Command] {
		return proctable.Process{}, false
	}
	var found []proctable.Process
	for _, c := range children {
		if onTheWay[c.PID] && !c.Gone {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		return proctable.Process{}, false
	}
	return found[0], true
}

// ReportDir sends the session, within the probe ceiling, one Output frame
// carrying the OSC 7 a shell sends for dir on this machine, as zmx print
// would, on a connection of its own, and waits for the daemon to broadcast it
// back: by then the daemon's terminal has taken it, so it is in the state
// every client attaching later is sent, zmx records it as the session's cwd,
// and every client attached has been sent it. It always sends: zmx's cwd
// record, which Info reports, is set from the directory a session was made
// in as well as from OSC 7, while what an attach is sent comes from its
// terminal alone, which only OSC 7 sets, so a recorded cwd that already reads
// as dir says nothing of what is replayed. An Info read comes first all the
// same: it refuses a daemon of another protocol before the frame goes out,
// so no daemon of another framing can read the frame as input to the
// session.
//
// zmx sends a SIGWINCH to the session's program, that process alone and not
// its group, as it takes the frame, as for any output while a client is
// connected, this one counting: an sh wrapper ignores it, and an interactive
// shell redraws its prompt. The frame also marks the session as having
// output, so one that has printed nothing yet is sent a snapshot at the next
// attach, and its foreground group a SIGWINCH, which is harmless.
func (b *Backend) ReportDir(ctx context.Context, name, dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("report the directory of session %q: %q is not absolute", name, dir)
	}
	ctx, cancel := b.bound(ctx, b.limits.probe, "; the report may still land")
	defer cancel()
	if _, err := b.askWithin(ctx, name); err != nil {
		return fmt.Errorf("report the directory of session %q: %w", name, err)
	}
	c, s, err := b.open(ctx, name)
	if err != nil {
		return fmt.Errorf("report the directory of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	hostname, _ := b.hostname()
	report := osc7(hostname, dir)
	if err := c.send(tagOutput, report); err != nil {
		return fmt.Errorf("report the directory of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	if err := c.awaitEcho(report); err != nil {
		return fmt.Errorf("report the directory of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	return nil
}

// awaitEcho reads the session's output until the Output sent comes back:
// the daemon broadcasts it, to its sender too, once it has fed it to the
// session's terminal.
func (c *conn) awaitEcho(sent []byte) error {
	for {
		p, err := c.await(tagOutput)
		if err != nil || bytes.Equal(p, sent) {
			return err
		}
	}
}

// osc7 is the OSC 7 a shell on the machine hostname sends to say it works in
// dir: file://<hostname><dir>, dir percent-encoded but for the bytes zmx's
// toOsc7Cwd leaves as they are, so zmx's parseOsc7Cwd, and localDir, read dir
// back exactly. Every control byte is encoded, so none ends the sequence
// early.
func osc7(hostname, dir string) []byte {
	var b strings.Builder
	b.WriteString("\x1b]7;file://")
	b.WriteString(hostname)
	for i := range len(dir) {
		if c := dir[i]; pathByte(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	b.WriteString("\x1b\\")
	return []byte(b.String())
}

// pathByte reports whether zmx's toOsc7Cwd leaves c unencoded in a path, as
// its isPathChar does: RFC 3986's unreserved characters, /, : and @.
func pathByte(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~/:@", c) >= 0
}

// localDir reads a session's cwd as zmx reports it, as a directory on this
// machine, or "" when it names none here. zmx keeps the cwd as
// file://<host><percent-encoded path>, as a raw OSC 7 the shell sent (which
// may be kitty-shell-cwd://), or as a plain path. OSC 7 crosses SSH, so a
// cwd on another host names no directory here.
func localDir(cwd, hostname string) string {
	if strings.HasPrefix(cwd, "/") {
		return cwd
	}
	u, err := url.Parse(cwd)
	if err != nil || (u.Scheme != "file" && u.Scheme != "kitty-shell-cwd") || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	if !isLocalHost(u.Hostname(), hostname) {
		return ""
	}
	return u.Path
}

// isLocalHost is zmx's rule: no host, localhost, this machine's name, or the
// same first DNS label, since gethostname may give the short name where OSC 7
// carries the full one, or the reverse.
func isLocalHost(host, hostname string) bool {
	if host == "" || strings.EqualFold(host, "localhost") || strings.EqualFold(host, hostname) {
		return true
	}
	label, _, _ := strings.Cut(hostname, ".")
	hostLabel, _, _ := strings.Cut(host, ".")
	return label != "" && strings.EqualFold(hostLabel, label)
}

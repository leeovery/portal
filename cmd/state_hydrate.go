package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/vtcapture"
	"github.com/spf13/cobra"
)

// The reset sequences make the cursor visible, leave the alternate screen and
// reset SGR; the postamble adds CRLF so the shell prompt lands at column 0. The
// entry is their pair: a second one written to a session already on the
// alternate screen does not nest, so a redraw may re-write it.
//
// The reset saves the cursor before it leaves. zmx's terminal, told to leave
// an alternate screen it is not on, restores the main screen's saved cursor,
// home when none was saved, and the prompt would land among the rows above.
// Each screen keeps a saved cursor of its own, so a reset written on the
// alternate screen still leaves it for the place it entered it from.
const (
	hydrateAltScreenEnter = "\x1b[?1049h\x1b[?25l"
	hydrateResetPreamble  = "\x1b[?25h\x1b7\x1b[?1049l\x1b[0m"
	hydrateResetPostamble = hydrateResetPreamble + "\r\n"
	hydrateSettleSleep    = 100 * time.Millisecond
	// hydrateStateWindow is how much of a transcript's end the replay holds
	// back to find the screen state zmx closed its capture with: many times
	// what that state takes.
	hydrateStateWindow = 16 << 10
	// hydrateSessionBound bounds the read of the session's creation for its
	// claim, so the hand-off waits on the backend no longer.
	hydrateSessionBound = 2 * time.Second
)

// hydrateFlagFile names the session's saved transcript in the helper's argv.
// Its session is named by --session, as the resume chain's subcommands name it.
const hydrateFlagFile = "file"

type hydrateConfig struct {
	// Session is the zmx session restore created, as it baked it into the
	// helper's argv: the key of the session's resume hook, its flags and its
	// index entry.
	Session string
	// Current is the session the helper runs in, as zmx names it in the
	// environment: Session itself, or the helper acts for no session.
	Current string
	// File is the session's saved transcript, empty when it saved none.
	// OpenTranscript opens it; unset, it is os.Open.
	File           string
	OpenTranscript func(path string) (io.ReadCloser, error)

	Stdout    io.Writer
	Logger    *slog.Logger
	HookStore *hooks.Store
	ExecShell func(prog string, args []string)

	// LoadPrefsStore and ResolveExe are nil-tolerant: unset takes the
	// non-migrating prefs route and the running binary's own path.
	LoadPrefsStore func() (*prefs.Store, error)
	ResolveExe     func() (string, error)

	// SetPending raises a session's resume-pending flag and ClearHydrating
	// lowers its hydrating flag; unset, they are state.ResumePending's Set and
	// state.Hydrating's Clear.
	SetPending     func(dir, name string) error
	ClearHydrating func(dir, name string) (bool, error)

	// Settle lets the session's terminal take in the replay before anything
	// after it runs; unset, it sleeps hydrateSettleSleep. SessionBound bounds
	// the read of the session's creation; zero is hydrateSessionBound.
	Settle       func()
	SessionBound time.Duration

	// DisableTTYSignals turns the session's kill keys into bytes before a
	// waiting session's chain is parked; the chain hands them back on whatever
	// path gives the session to a hook or a shell.
	DisableTTYSignals func() error
}

// resumeDecision carries one session's resolved resume mode from the top of
// the helper to its hand-off.
type resumeDecision struct {
	Wait   bool
	Lookup hooks.OnResume
}

// parkedChain is what the mark step resolved for a session it raised the
// resume-pending flag of: the executable its chain is composed from, and the
// flag's path, which the chain's shell tests and removes.
type parkedChain struct {
	Exe  string
	Flag string
}

func hydrateLoggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return hydrateLogger
	}
	return logger
}

// runHydrate restores one session's screen and hands the session on. Its
// steps keep this order, which the session's saved transcript depends on: the
// saver captures a session only once its hydrating flag is down, so that flag
// stays raised until the replay has settled, a waiting session has raised its
// resume-pending flag in its place, and the session's creation is recorded,
// its restore finished, while the claim rule still reads the session as
// restoring.
//
// A transcript it couldn't replay leaves the restore unfinished, so the saved
// one is kept: the session says so and where it is, its entry stays owed and
// its hydrating flag raised, so the saver neither captures it over that
// transcript nor drops the session if it ends, and the next restore replays
// it.
//
// ExecShell replaces the process image, so in production it never returns.
func runHydrate(ctx context.Context, cfg hydrateConfig) {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	if err := sessionNameRefusal(cfg.Session, cfg.Current); err != nil {
		cfg.Logger.Warn("not run in the session it restores, so no hook and no panel", "session", cfg.Session, "error", err)
		handOffToHookOrShell(cfg.Logger, cfg.ExecShell, "")
		return
	}
	decision := resolveResumeDecision(cfg)
	replayed := replayTranscript(cfg) == nil
	if !replayed {
		keepTranscript(cfg)
	}
	parked := markIfWaiting(cfg, decision)
	recordCreation(ctx, cfg, replayed)
	if replayed {
		lowerHydrating(cfg)
	}
	execShellOrHookAndExit(cfg, decision.Lookup, parked)
}

// replayFailedNotice is the line a session whose saved scrollback couldn't be
// replayed shows above its prompt, naming the file kept.
const replayFailedNotice = "Portal couldn't replay this session's saved scrollback; it's kept at %s"

// keepTranscript tells the session its saved transcript couldn't be replayed
// and is kept, and where.
func keepTranscript(cfg hydrateConfig) {
	_, _ = fmt.Fprintf(cfg.Stdout, replayFailedNotice+"\r\n", cfg.File)
	cfg.Logger.Info("scrollback kept for the next restore", "session", cfg.Session, "path", cfg.File)
}

// sessionNameRefusal says why the helper must act for no session: zmx names
// none in the environment, or names another. The session's flags, hook, index
// entry and log records are all keyed by its exact name, so a name zmx doesn't
// confirm reaches none of them.
func sessionNameRefusal(session, current string) error {
	switch {
	case current == "":
		return errors.New("ZMX_SESSION is unset")
	case current != session:
		return fmt.Errorf("ZMX_SESSION is %q", current)
	}
	return nil
}

// replayTranscript writes the session's saved transcript to its terminal
// between the reset sequences, then lets the terminal take it in. Only its
// rows are replayed, without the modes its capture opened with and the screen
// state it closed with: the program that set those modes is gone, so the
// shell must not inherit them, and the cursor ends where the last row does,
// so the prompt lands below it whatever the terminal's height. Only a
// transcript with something in it is replayed: one that saved none, an empty
// one and one that can't be read write nothing at all. One failing part way is
// cut and closed as a whole one is, so the shell inherits nothing its bytes
// set; its cause is recorded. A capture zmx opened with modes but closed in a
// form this build doesn't read is replayed whole, its cursor placement with
// it, so that is recorded too: zmx changing its closing state would otherwise
// bring the prompt back over the rows unseen.
//
// It returns why a transcript it could not replay whole wasn't: nil for one
// replayed whole, for no transcript, and for one gone, which leaves nothing
// to keep.
func replayTranscript(cfg hydrateConfig) error {
	if cfg.File == "" {
		return nil
	}
	open := cfg.OpenTranscript
	if open == nil {
		open = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	}
	file, err := open(cfg.File)
	if err != nil {
		logTranscriptUnreadable(cfg, err)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = file.Close() }()
	start := time.Now()
	out := &replayWriter{w: cfg.Stdout}
	closeUnread, err := copyTranscriptContent(out, file)
	took := time.Since(start)
	if closeUnread {
		cfg.Logger.Warn("scrollback closing state not recognised, so replayed whole", "session", cfg.Session, "path", cfg.File)
	}
	if out.started {
		_, _ = io.WriteString(cfg.Stdout, hydrateResetPostamble)
		settleReplay(cfg)
	}
	switch {
	case err == nil:
		cfg.Logger.Info("scrollback replayed", "bytes", out.n, "took", took)
	case out.started:
		cfg.Logger.Warn("scrollback replay cut short", "session", cfg.Session, "path", cfg.File, "bytes", out.n, "error", err)
	default:
		logTranscriptUnreadable(cfg, err)
	}
	return err
}

func settleReplay(cfg hydrateConfig) {
	if cfg.Settle != nil {
		cfg.Settle()
		return
	}
	time.Sleep(hydrateSettleSleep)
}

// copyTranscriptContent copies the transcript's rows to w: it reads the head
// first and cuts the modes its capture opened with from it, then holds back
// the end the closing state sits in until the read is done, and cuts that
// state from it. The end of a read failing part way is cut too: no row places
// the cursor, so the cut takes a closing state the read reached and nothing
// else. closeUnread reports a whole capture led by modes, so one zmx took,
// whose end holds no closing state the cut recognises.
func copyTranscriptContent(w io.Writer, r io.Reader) (closeUnread bool, err error) {
	head, err := readTranscriptHead(r)
	modes, rows := vtcapture.Modes(head)
	held := &heldEnd{w: w, end: rows}
	if err == nil {
		_, err = io.Copy(held, r)
	}
	end := vtcapture.TrimClose(held.end)
	closeUnread = err == nil && len(modes) > 0 && len(end) == len(held.end)
	if _, writeErr := w.Write(end); err == nil {
		err = writeErr
	}
	return closeUnread, err
}

// readTranscriptHead reads the first hydrateStateWindow bytes of r, which
// hold the modes its capture opened with, or the whole of a shorter one.
func readTranscriptHead(r io.Reader) ([]byte, error) {
	head := make([]byte, hydrateStateWindow)
	n, err := io.ReadFull(r, head)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = nil
	}
	return head[:n], err
}

// heldEnd passes what is written to it on to w, but for the last
// hydrateStateWindow bytes, which it holds.
type heldEnd struct {
	w   io.Writer
	end []byte
}

func (h *heldEnd) Write(p []byte) (int, error) {
	h.end = append(h.end, p...)
	if over := len(h.end) - hydrateStateWindow; over > 0 {
		if _, err := h.w.Write(h.end[:over]); err != nil {
			return 0, err
		}
		h.end = h.end[over:]
	}
	return len(p), nil
}

// replayWriter writes the reset preamble ahead of a transcript's first byte,
// so a transcript with nothing to replay writes nothing, and counts the bytes
// replayed.
type replayWriter struct {
	w       io.Writer
	n       int64
	started bool
}

func (r *replayWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !r.started {
		r.started = true
		_, _ = io.WriteString(r.w, hydrateResetPreamble)
	}
	n, err := r.w.Write(p)
	r.n += int64(n)
	return n, err
}

// logTranscriptUnreadable records why no transcript was replayed: the file
// was gone, refused, or failed before a byte of it was read.
func logTranscriptUnreadable(cfg hydrateConfig, cause error) {
	switch {
	case errors.Is(cause, fs.ErrNotExist):
		cfg.Logger.Warn("scrollback file not found", "session", cfg.Session, "path", cfg.File)
	case errors.Is(cause, fs.ErrPermission):
		cfg.Logger.Warn("scrollback file unreadable (permission denied)", "session", cfg.Session, "path", cfg.File)
	default:
		cfg.Logger.Warn("scrollback file I/O error", "session", cfg.Session, "path", cfg.File, "error", cause)
	}
	cfg.Logger.Info("scrollback missing", "path", cfg.File)
}

// resolveResumeDecision reads the session's registration and the install-wide
// default once, at the top of the helper, so the hand-off decides by what the
// helper read before it raised anything.
func resolveResumeDecision(cfg hydrateConfig) resumeDecision {
	lookup := lookupOnResumeOrLog(cfg)
	wait := lookup.Found && resumemode.Resolve(lookup.Mode, installResumeMode(cfg)) == resumemode.Lazy
	return resumeDecision{Wait: wait, Lookup: lookup}
}

// A prefs store that cannot be built, and a read that fails, both take the
// shipped default: a panel is answered in a keystroke, while a resume the user
// did not want cannot be taken back.
func installResumeMode(cfg hydrateConfig) resumemode.Mode {
	load := cfg.LoadPrefsStore
	if load == nil {
		load = loadPrefsStoreNoMigrate
	}
	store, err := load()
	if err != nil {
		return installResumeModeOf(nil)
	}
	return installResumeModeOf(store)
}

// The read error is discarded: LoadResumeMode answers resumemode.Default beside it.
func installResumeModeOf(store *prefs.Store) resumemode.Mode {
	if store == nil {
		return resumemode.Default
	}
	mode, _ := store.LoadResumeMode()
	return mode
}

// An absent store reads as an unregistered key, which the shared rule records
// and answers as any other miss.
func lookupOnResumeOrLog(cfg hydrateConfig) hooks.OnResume {
	return resumeRegistrationOrLog(cfg.Logger, func(hookKey string) (hooks.OnResume, error) {
		if cfg.HookStore == nil {
			return hooks.OnResume{}, nil
		}
		return cfg.HookStore.LookupOnResume(hookKey, hooks.ViaHydrate)
	}, cfg.Session)
}

// markIfWaiting raises the resume-pending flag of a session that is going to
// wait, while its hydrating flag still keeps the saver off it. A session whose
// flag can't be raised does not wait: it returns nil, as does one that was
// never going to.
func markIfWaiting(cfg hydrateConfig, decision resumeDecision) *parkedChain {
	if !decision.Wait {
		return nil
	}
	parked, err := markResumePending(cfg)
	if err != nil {
		cfg.Logger.Warn("set resume pending flag failed", resumeLogAttrs(cfg.Session, "error", err)...)
		return nil
	}
	return &parked
}

// The executable is resolved before the flag is raised, so a refusal is taken
// while the session is still an eager one: a flagged session whose chain could
// not be composed would fire its hook and never be captured again. A name no
// file can carry refuses here, before anything is written.
func markResumePending(cfg hydrateConfig) (parkedChain, error) {
	resolveExe := cfg.ResolveExe
	if resolveExe == nil {
		resolveExe = resumeChainExe
	}
	exe, err := resolveExe()
	if err != nil {
		return parkedChain{}, err
	}
	setPending := cfg.SetPending
	if setPending == nil {
		setPending = state.ResumePending.Set
	}
	flag, lowerErr, err := raiseResumePending(cfg.Session, setPending)
	if lowerErr != nil {
		warnResumePendingClearFailed(cfg.Logger, cfg.Session, lowerErr)
	}
	if err != nil {
		return parkedChain{}, err
	}
	return parkedChain{Exe: exe, Flag: flag}, nil
}

// recordCreation records the creation the session's backend reports as its
// index entry's, through the claim every claimer makes, and, the replay
// having finished, finishes the session's restore: its entry is owed none any
// more. It runs while the session is still flagged hydrating, so the rule
// adopts whatever entry restore saved and never replaces it: a restore that
// died between creating the session and recording it leaves the entry its
// saved creation, which the saver would replace once the flag is down. An
// entry already recording this creation and owed nothing is left as it is. A
// failure is the helper's to record, never to wait on: the session is handed
// on regardless, its restore unfinished until a claim finds it live once the
// flag is down.
//
// A session whose flag is proved down, as when the helper is run by hand,
// claims nothing: the rule would judge its entry as any claim's, and could
// replace it.
func recordCreation(ctx context.Context, cfg hydrateConfig, replayed bool) {
	if !stillHydrating(cfg.Session) {
		cfg.Logger.Debug("session creation not claimed: its hydrating flag is down", "session", cfg.Session)
		return
	}
	if err := claimCreation(ctx, cfg, replayed); err != nil {
		cfg.Logger.Warn("record session creation failed", "session", cfg.Session, "error", err)
	}
}

// stillHydrating reports whether the session's hydrating flag is raised, a
// flag that can't be looked for counting as raised, as the claim rule counts
// it.
func stillHydrating(session string) bool {
	dir, err := state.Dir()
	return err != nil || state.Hydrating.Raised(dir, session)
}

// claimCreation claims the session's entry for the creation its backend
// reports, finishing its restore in the same write when it replayed.
func claimCreation(ctx context.Context, cfg hydrateConfig, replayed bool) error {
	b, err := sessionBackend()
	if err != nil {
		return err
	}
	bound := cfg.SessionBound
	if bound == 0 {
		bound = hydrateSessionBound
	}
	readCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	live, err := liveSession(readCtx, b, cfg.Session, "its creation is not recorded")
	if err != nil {
		return err
	}
	var finish func(*sessionindex.Index) error
	if replayed {
		finish = func(idx *sessionindex.Index) error {
			idx.Hydrated(live.Name, live.Created, live.PID)
			return nil
		}
	}
	return claimSession("internal", b.ValidateName, live, finish, nil)
}

// lowerHydrating ends the session's restore: from here the saver captures it,
// and a claim judges it, as any other session. A flag left raised keeps the
// saver from capturing the session at all, so its failure is recorded.
func lowerHydrating(cfg hydrateConfig) {
	lower := cfg.ClearHydrating
	if lower == nil {
		lower = state.Hydrating.Clear
	}
	dir, err := state.Dir()
	if err == nil {
		_, err = lower(dir, cfg.Session)
	}
	if err != nil {
		cfg.Logger.Warn("clear hydrating flag failed", resumeLogAttrs(cfg.Session, "error", err)...)
	}
}

// Only a session the mark step returned parks; any other runs the
// registration's command, whatever mode it resolved to.
func execShellOrHookAndExit(cfg hydrateConfig, registration hooks.OnResume, parked *parkedChain) {
	if parked != nil {
		execResumeChainAndExit(cfg, registration.Command, *parked)
		return
	}
	handOffToHookOrShell(cfg.Logger, cfg.ExecShell, registration.Command)
}

// execResumeChainAndExit parks the session on the draw followed by the
// chain's tail, so a waiter that ends without answering leaves a session its
// user can still type into. Both halves are composed from what the mark step
// resolved, which resolves nothing further: a resolution failing after the
// flag is raised would leave the session flagged for life.
//
// A session whose signal generation could not be turned off still parks: the
// chain's trap turns a kill key that lands into the recovery tail.
func execResumeChainAndExit(cfg hydrateConfig, command string, parked parkedChain) {
	payload := resumeChainPayload{Command: command, Session: cfg.Session}
	if err := cfg.DisableTTYSignals(); err != nil {
		cfg.Logger.Warn("disable terminal signals failed", payload.logAttrs("error", err)...)
	}
	args := []string{"sh", "-c", parkedResumeChain(parked, payload)}
	execHandOff(cfg.Logger, cfg.ExecShell, "/bin/sh", args, true)
}

// parkedChainTrap keeps the parked shell alive through a group interrupt or
// quit so its recovery tail still runs, and through a SIGTERM so the session
// outlasts a shutdown's SIGTERM until its daemon ends it. It must stay a
// caught trap: an ignored signal stays ignored across exec, and the hook and
// the user's shell would inherit it. SIGHUP stays uncaught, so a kill still
// ends the session.
const parkedChainTrap = "trap : INT QUIT TERM; "

// parkedChainBackstop leaves the session at a login shell when the tail could
// not start at all. It keys on the shell's could-not-run statuses, not on any
// failure: a tail that recovered the session exec'd the user's shell, whose
// exit status arrives here, and a second shell after it would make the session
// take two exits to close. The executable check tells a tail that never started
// from a shell that exited 126 or 127 itself. The binary can leave the baked
// path while an answered session's shell runs, so the tail's own gate comes
// first: a flag proved lowered ends the chain, and one that can't be looked
// for counts as still raised. Past the gate it takes the tail's own steps in
// the tail's order, since no Portal binary is left to take them.
func parkedChainBackstop(parked parkedChain) string {
	answeredGate := `if ` + flagProvedLowered(parked.Flag) + `; then exit $s; fi`
	reset := `printf '%s' ` + shellquote.Single(hydrateResetPreamble)
	clearFlag := shellquote.Join([]string{"rm", "-f", "--", parked.Flag}) + ` 2>/dev/null`
	return `; s=$?; case $s in 126|127) if [ ! -x ` + shellquote.Single(parked.Exe) + ` ]; then ` +
		answeredGate + `; ` + reset + `; ` + clearFlag +
		`; stty sane 2>/dev/null; ` + backstopLoginShell + `; fi;; esac; exit $s`
}

// backstopLoginShell execs the session's login shell, found as resolveShell
// finds it: a $SHELL holding a slash must be a file it can run, and a bare one
// must be on PATH. command -v won't do for a path, since dash answers for any
// path that exists, a directory or a file it can't run among them, and the
// exec would then fail and end the session. One that can't be found is
// /bin/sh, exported as $SHELL as the Go hand-off has it.
const backstopLoginShell = `case $SHELL in */*) [ -f "$SHELL" ] && [ -x "$SHELL" ];; ` +
	`*) command -v "$SHELL" >/dev/null 2>&1;; esac || export SHELL=` + fallbackShell +
	`; exec "$SHELL" ` + loginShellFlag

// flagProvedLowered is a shell test that succeeds only when the flag at path
// is proved lowered, as the chain's Go steps read it too (flagLowered):
// nothing stands at path, and its directory could be searched for it, or,
// that directory gone too, the state directory above it could. Anything the
// test can't see through, an unsearchable directory among them, leaves the
// flag raised.
func flagProvedLowered(path string) string {
	dir := filepath.Dir(path)
	return `{ [ ! -e ` + shellquote.Single(path) + ` ] && { ` + searchable(dir) +
		` || { [ ! -e ` + shellquote.Single(dir) + ` ] && ` + searchable(filepath.Dir(dir)) + `; }; }; }`
}

// flagStillRaised negates flagProvedLowered. POSIX allows one ! per pipeline.
func flagStillRaised(path string) string { return `! ` + flagProvedLowered(path) }

// searchable is a shell test that succeeds for a directory whose entries can
// be looked up.
func searchable(dir string) string {
	return `{ [ -d ` + shellquote.Single(dir) + ` ] && [ -x ` + shellquote.Single(dir) + ` ]; }`
}

// sigtermStatus is the status the shell reports for a child a SIGTERM ended.
const sigtermStatus = 128 + int(syscall.SIGTERM)

// parkedChainDraw starts the draw again when the draw, or the waiter it became,
// was ended by a SIGTERM while the session still waits: a catch is not
// inherited across fork or exec and a Go image cannot install one before its
// runtime has started, so each hand-off opens a moment only the parked shell
// can close. The flag's gate keeps an answered session, whose own shell now
// runs as that process, off the panel.
func parkedChainDraw(parked parkedChain, payload resumeChainPayload) string {
	return `while ` + shellquote.Join(resumeChainArgv(parked.Exe, resumeDrawSubcommand, payload)) +
		`; [ $? -eq ` + strconv.Itoa(sigtermStatus) + ` ] && ` + flagStillRaised(parked.Flag) + `; do :; done`
}

func parkedResumeChain(parked parkedChain, payload resumeChainPayload) string {
	return parkedChainTrap +
		parkedChainDraw(parked, payload) + "; " +
		shellquote.Join(resumeChainArgv(parked.Exe, resumeRecoverSubcommand, payload)) +
		parkedChainBackstop(parked)
}

// syscall.Exec returns only on failure, and that path must end through
// log.Close(1) + osExit(1), or the just-emitted exec marker stands as a phantom
// handoff.
func defaultExecShell(prog string, args []string) {
	err := syscall.Exec(prog, args, os.Environ())
	hydrateLogger.Warn("exec handoff failed", "target", prog, "args", strings.Join(args, " "), "error", err)
	log.Close(1)
	osExit(1)
}

var hydrateRunFunc = runHydrate

// stateHydrateCmd is what restore creates each session running: it replays
// the session's saved transcript, then hands the session to its resume hook,
// its resume panel or its shell.
var stateHydrateCmd = &cobra.Command{
	Use:    "hydrate",
	Short:  "Replay a restored session's saved scrollback, then hand it on (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		session, _ := cmd.Flags().GetString(resumeFlagSession)
		file, _ := cmd.Flags().GetString(hydrateFlagFile)

		// A nil store degrades the hook lookup to a bare shell, which beats
		// failing the per-session helper closed.
		store, _ := loadHookStore()

		hydrateRunFunc(cmd.Context(), hydrateConfig{
			Session:           session,
			Current:           currentName(),
			File:              file,
			Stdout:            cmd.OutOrStdout(),
			Logger:            hydrateLogger,
			HookStore:         store,
			ExecShell:         defaultExecShell,
			DisableTTYSignals: clearStdinSignals,
		})
		return nil
	},
}

func init() {
	stateHydrateCmd.Flags().String(resumeFlagSession, "", "The restored zmx session's name, which keys its hook, its flags and its index entry")
	// Optional: a session that saved no transcript is restored with none.
	stateHydrateCmd.Flags().String(hydrateFlagFile, "", "Absolute path to the session's saved scrollback")
	_ = stateHydrateCmd.MarkFlagRequired(resumeFlagSession)

	stateCmd.AddCommand(stateHydrateCmd)
}

// Package alias provides persistence for path aliases in a flat key=value file.
package alias

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// logger is the aliases-component logger, bound once at package init. Every
// aliases-file mutation that flows through the audited combined methods
// (SetAndSave / DeleteAndSave) emits a single breadcrumb under this component so
// `grep "aliases:" portal.log` reconstructs the change history. importing
// internal/log introduces no cycle — internal/log depends only on the standard
// library.
//
// Message-shape: the op verb is BOTH the slog message (preserving the
// `aliases: <verb>` catalog shape and grep idiom) AND a required "op" attr drawn
// from the closed value space (set / modify / rm / set-noop), so JSON output and
// `grep op=set` filtering both work — see the hooks store for the full rationale.
var logger = log.For("aliases")

// Alias represents a single name-to-path mapping.
type Alias struct {
	Name string
	Path string
}

// Store manages persistence of alias data to a flat key=value file.
type Store struct {
	path    string
	aliases map[string]string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{
		path:    path,
		aliases: make(map[string]string),
	}
}

// Load reads aliases from the flat key=value file.
// Returns an empty map when the file is missing or empty.
// Duplicate keys are resolved with last-wins semantics.
func (s *Store) Load() (map[string]string, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.aliases = make(map[string]string)
			return s.aliases, nil
		}
		return nil, fmt.Errorf("failed to open aliases file: %w", err)
	}
	defer func() { _ = f.Close() }()

	aliases := make(map[string]string)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		name, path, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		aliases[name] = path
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read aliases file: %w", err)
	}

	s.aliases = aliases
	return s.aliases, nil
}

// Save writes all aliases to the file in sorted key=value format.
// Creates the parent directory if it does not exist.
//
// The two failure phases are sentinel-wrapped from fileutil's write-phase
// sentinels so the audited combined methods (SetAndSave / DeleteAndSave) can map
// error_class via fileutil.ClassifyWriteError without re-implementing the token
// table — the sentinel strings ARE the closed error_class tokens, so the mapping
// is 1:1 and cannot drift:
//   - os.MkdirAll failure -> ErrWriteTempCreate -> "write-failed-temp-create"
//     (the directory is the write's creation prerequisite; the closed space has
//     no write-failed-mkdir, mirroring fileutil.AtomicWrite's own mapping).
//   - os.WriteFile failure -> ErrWriteWrite -> "write-failed-write".
//
// [needs-info, resolved-in-comment] error_class phase mapping = option (a): map
// manually because Save uses os.WriteFile, NOT fileutil.AtomicWrite. Option (b)
// — migrating Save to fileutil.AtomicWrite for atomicity plus the unified
// AtomicWrite phase sentinels (temp-create / write / fsync / rename) — is a
// deferred future improvement: it changes Save's on-disk behaviour (temp-file +
// rename instead of in-place truncating write), which is out of scope for the
// observability work.
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: failed to create config directory: %w", fileutil.ErrWriteTempCreate, err)
	}

	sorted := s.List()

	var b strings.Builder
	for _, a := range sorted {
		fmt.Fprintf(&b, "%s=%s\n", a.Name, a.Path)
	}

	if err := os.WriteFile(s.path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("%w: failed to write aliases file: %w", fileutil.ErrWriteWrite, err)
	}

	return nil
}

// Get returns the path for the given alias name and whether it was found.
func (s *Store) Get(name string) (string, bool) {
	path, ok := s.aliases[name]
	return path, ok
}

// Set adds or overwrites an alias. Each name maps to exactly one path.
func (s *Store) Set(name, path string) {
	s.aliases[name] = path
}

// Delete removes the alias with the given name.
// Returns true if the alias existed, false otherwise.
func (s *Store) Delete(name string) bool {
	_, ok := s.aliases[name]
	if ok {
		delete(s.aliases, name)
	}
	return ok
}

// SetAndSave is the audited mutation path for adding or updating an alias: it
// classifies the op from the pre-mutation map, performs the in-memory Set, runs
// Save, and emits exactly one breadcrumb under the aliases component.
//
// [needs-info, resolved-in-comment] EMISSION POINT = option (a): a single
// COMBINED store-seam method that does the in-memory op AND Save AND emits. This
// is the only shape that cleanly satisfies the set-noop skip-Save requirement —
// a separate audited Save() could not selectively skip persistence for a no-op.
//
// via records the mutation origin (cli for user-facing `portal alias set` and
// the TUI alias editor; the closed value space is cli / internal / migrate).
//
// The op is classified from the pre-mutation map:
//   - name absent                       -> INFO "set"
//   - name present, path == existing     -> DEBUG "set-noop"; Save is SKIPPED so
//     the file is not touched, and SetAndSave returns nil without mutating.
//   - name present, path != existing     -> INFO "modify"
//
// On a persist failure the breadcrumb is WARN carrying the wrapped error and its
// error_class (from Save's sentinel-wrapping), and the error is returned.
func (s *Store) SetAndSave(name, path, via string) error {
	existing, present := s.aliases[name]
	if present && existing == path {
		// The value already matches: emit a DEBUG no-op breadcrumb and return
		// without touching the file (no Save).
		logger.Debug("set-noop", "op", "set-noop", "alias", name, "via", via)
		return nil
	}

	op := "set"
	if present {
		op = "modify"
	}

	s.Set(name, path)

	if err := s.Save(); err != nil {
		logger.Warn(op, "op", op, "alias", name, "value", path, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "op", op, "alias", name, "value", path, "via", via)
	return nil
}

// DeleteAndSave is the audited mutation path for removing an alias: it deletes
// the in-memory entry, and — only if the entry existed — runs Save and emits one
// breadcrumb under the aliases component.
//
// An absent-key delete returns (false, nil) WITHOUT Save and WITHOUT emitting:
// the CLI rejects an absent alias before any persist would occur, so there is no
// mutation to audit (preserving the pre-instrumentation "alias not found" path,
// which never wrote the file). The "rm" breadcrumb carries NO value attr per the
// closed-attr rule (value is only set for set / modify).
//
// On a persist failure (existed entry) the breadcrumb is WARN carrying the
// wrapped error and its error_class, and (true, err) is returned.
func (s *Store) DeleteAndSave(name, via string) (existed bool, err error) {
	existed = s.Delete(name)
	if !existed {
		return false, nil
	}

	if err := s.Save(); err != nil {
		logger.Warn("rm", "op", "rm", "alias", name, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return true, err
	}

	logger.Info("rm", "op", "rm", "alias", name, "via", via)
	return true, nil
}

// Keys returns all alias names sorted. It exposes the finite alias-key
// namespace for glob enumeration (resolver.AliasLookup.Keys) without leaking the
// []Alias name-to-path shape that List returns.
func (s *Store) Keys() []string {
	keys := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

// List returns all aliases sorted by name.
func (s *Store) List() []Alias {
	result := make([]Alias, 0, len(s.aliases))
	for name, path := range s.aliases {
		result = append(result, Alias{Name: name, Path: path})
	}

	slices.SortFunc(result, func(a, b Alias) int {
		return strings.Compare(a.Name, b.Name)
	})

	return result
}

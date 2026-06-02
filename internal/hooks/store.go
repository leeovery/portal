// Package hooks provides persistence for hook registrations keyed by structural keys.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/storelog"
)

// logger is the hooks-component logger, bound once at package init. Every
// hooks.json mutation that flows through Set/Remove emits a single breadcrumb
// under this component so `grep "hooks:" portal.log` reconstructs the change
// history. importing internal/log introduces no cycle — internal/log depends
// only on the standard library.
//
// Message-shape: the op verb is BOTH the slog message (preserving the
// `hooks: <verb>` catalog shape and grep idiom) AND a required "op" attr drawn
// from the closed value space (set / modify / rm / clean-stale / set-noop). The
// spec's "State-mutation audit trail" lists op under Required attrs and the
// closed attr-key vocabulary, so JSON output and `grep op=set` filtering both
// need op as a real structured attr — not only the message string.
var logger = log.For("hooks")

// Hook represents a single hook entry for list output.
type Hook struct {
	Key     string
	Event   string
	Command string
}

// hooksFile is the on-disk JSON structure: map[structural_key]map[event]command.
type hooksFile = map[string]map[string]string

// Store manages persistence of hook data to a JSON file.
type Store struct {
	path string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads hooks from the JSON file.
// Returns an empty map when the file is missing or contains malformed JSON.
func (s *Store) Load() (hooksFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hooksFile{}, nil
		}
		return nil, err
	}

	var h hooksFile
	if err := json.Unmarshal(data, &h); err != nil {
		return hooksFile{}, nil
	}

	return h, nil
}

// Save writes hooks to the JSON file using atomic write (temp file + rename).
// Creates the parent directory if it does not exist.
func (s *Store) Save(h hooksFile) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data)
}

// SaveAudited persists h via Save and emits one audit breadcrumb under the
// hooks component, keeping audit emission at the store seam (the chokepoint
// that can't be forgotten) rather than scattered at callers.
//
// It is the persistence path for bulk rewrites that have no single per-file
// key — e.g. the migrate-rename N-key rewrite, which is treated as a batch
// op=modify. A per-key breadcrumb would be wrong there (there is no single
// affected key), so the breadcrumb carries entries=N instead of hook_key.
//
//   - On success: INFO with the given op, entries=N, and via.
//   - On failure: WARN with op, entries=N, via, the wrapped error and its
//     write-failed-* error_class; the error is returned.
//
// h is the same map type Store.Load returns, so cmd-layer callers can pass the
// value they loaded and rewrote without referencing the unexported alias.
func (s *Store) SaveAudited(h hooksFile, op string, entries int, via string) error {
	if err := s.Save(h); err != nil {
		logger.Warn(op, "op", op, "entries", entries, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "op", op, "entries", entries, "via", via)
	return nil
}

// Set adds or overwrites a hook for the given key and event.
// Creates the inner map for the key if it does not exist.
//
// via records the mutation origin for the audit breadcrumb and is drawn from
// the closed value space cli / internal / migrate (cli for user-facing
// `portal hooks set`; internal for code-driven mutations).
//
// Set emits one audit breadcrumb under the hooks component, classified from the
// pre-write Load:
//   - new key/event              -> INFO "set"
//   - existing key/event, value differs -> INFO "modify"
//   - existing key/event, value matches -> DEBUG "set-noop"; Save is SKIPPED so
//     the file is not touched.
//
// On a persist failure the breadcrumb is WARN carrying the wrapped error and its
// error_class.
func (s *Store) Set(key, event, command, via string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	op := classifySet(h, key, event, command)
	if op == "set-noop" {
		// The value already matches: emit a DEBUG no-op breadcrumb and return
		// without touching the file (no Save).
		logger.Debug("set-noop", "op", "set-noop", "hook_key", key, "via", via)
		return nil
	}

	if h[key] == nil {
		h[key] = make(map[string]string)
	}
	h[key][event] = command

	if err := s.Save(h); err != nil {
		logger.Warn(op, "op", op, "hook_key", key, "value", command, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "op", op, "hook_key", key, "value", command, "via", via)
	return nil
}

// classifySet returns the op verb for a Set against the loaded state h:
// "set" for a brand-new key or event, "modify" when the entry exists with a
// different value, and "set-noop" when the entry exists with the same value.
func classifySet(h hooksFile, key, event, command string) string {
	events, ok := h[key]
	if !ok {
		return "set"
	}
	existing, ok := events[event]
	if !ok {
		return "set"
	}
	if existing == command {
		return "set-noop"
	}
	return "modify"
}

// Remove deletes a hook for the given key and event.
// Removes the outer key if the inner map becomes empty.
// No-op (no error) if the key or event does not exist.
//
// via records the mutation origin for the audit breadcrumb (cli / internal /
// migrate). Remove always rewrites the file via Save — even an absent-key remove
// re-persists the (unchanged) state — so it always emits one breadcrumb under
// the hooks component: INFO "rm" on success (no value attr), WARN "rm" with the
// wrapped error and its error_class on a persist failure.
func (s *Store) Remove(key, event, via string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	if events, ok := h[key]; ok {
		delete(events, event)
		if len(events) == 0 {
			delete(h, key)
		}
	}

	if err := s.Save(h); err != nil {
		logger.Warn("rm", "op", "rm", "hook_key", key, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info("rm", "op", "rm", "hook_key", key, "via", via)
	return nil
}

// List returns a flat slice of Hook structs sorted by key then event type.
func (s *Store) List() ([]Hook, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	var list []Hook
	for key, events := range h {
		for event, command := range events {
			list = append(list, Hook{
				Key:     key,
				Event:   event,
				Command: command,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Key != list[j].Key {
			return list[i].Key < list[j].Key
		}
		return list[i].Event < list[j].Event
	})

	return list, nil
}

// CleanStale removes hook entries for keys not present in liveKeys.
// Returns the removed keys. The file is only saved if at least one entry
// was removed.
//
// CleanStale is a batch mutation and follows the batch-summary breadcrumb
// shape: one DEBUG per removed key, then exactly one INFO summary (op
// clean-stale, entries=N, via=internal, took=<elapsed>) on a successful
// whole-batch Save, or one WARN carrying the wrapped error and its
// write-failed-* error_class on a Save failure. via is always "internal"
// because CleanStale is only ever invoked by code-driven cleanup, never a
// user-facing command.
//
// [needs-info, resolved-in-comment] The spec's batch contract mentions a
// per-entry WARN with error_class=unexpected on a mid-loop failure. That has
// no reachable site here: the kept/removed partition is computed entirely in
// memory and persistence is a SINGLE batched Save of the kept map. There is no
// point at which one entry can fail while the batch continues — the only
// failure mode is the whole-batch Save below, which is write-failed-* (not
// unexpected). We do NOT fabricate a synthetic per-entry failure path.
func (s *Store) CleanStale(liveKeys []string) ([]string, error) {
	start := time.Now()

	h, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load hooks: %w", err)
	}

	live := make(map[string]struct{}, len(liveKeys))
	for _, k := range liveKeys {
		live[k] = struct{}{}
	}

	kept := make(hooksFile)
	var removed []string

	for key, events := range h {
		if _, ok := live[key]; ok {
			kept[key] = events
		} else {
			removed = append(removed, key)
		}
	}

	// Zero-removal case: a clean that removes nothing is an idempotent no-op.
	// Preserve the existing skip (no Save) and emit NO summary — the
	// batch-summary INFO is reserved for batches that did work; an idempotent
	// no-op must not clutter the INFO baseline.
	if len(removed) == 0 {
		return removed, nil
	}

	for _, key := range removed {
		logger.Debug("clean-stale", "op", "clean-stale", "hook_key", key, "via", "internal")
	}

	if err := s.Save(kept); err != nil {
		// Whole-batch persist failure: the shared helper emits the WARN with the
		// write-failed-* error_class (from the AtomicWrite phase space, NOT
		// "unexpected"); we wrap-and-return.
		storelog.EmitCleanStaleSummary(logger, len(removed), start, err)
		return nil, fmt.Errorf("failed to save after cleaning stale hooks: %w", err)
	}

	// entries_failed is omitted: there is no per-entry failure path (see the
	// [needs-info] note above), so M is always 0 and the attr stays absent.
	storelog.EmitCleanStaleSummary(logger, len(removed), start, nil)

	return removed, nil
}

// Get returns the event map for a specific key, or an empty map if the key
// has no hooks.
func (s *Store) Get(key string) (map[string]string, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	events, ok := h[key]
	if !ok {
		return map[string]string{}, nil
	}

	return events, nil
}

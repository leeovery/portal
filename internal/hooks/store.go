// Package hooks provides persistence for hook registrations keyed by structural keys.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// logger is the hooks-component logger, bound once at package init. Every
// hooks.json mutation that flows through Set/Remove emits a single breadcrumb
// under this component so `grep "hooks:" portal.log` reconstructs the change
// history. importing internal/log introduces no cycle — internal/log depends
// only on the standard library.
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
		logger.Debug("set-noop", "hook_key", key, "via", via)
		return nil
	}

	if h[key] == nil {
		h[key] = make(map[string]string)
	}
	h[key][event] = command

	if err := s.Save(h); err != nil {
		logger.Warn(op, "hook_key", key, "value", command, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "hook_key", key, "value", command, "via", via)
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
		logger.Warn("rm", "hook_key", key, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info("rm", "hook_key", key, "via", via)
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
func (s *Store) CleanStale(liveKeys []string) ([]string, error) {
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

	if len(removed) > 0 {
		if err := s.Save(kept); err != nil {
			return nil, fmt.Errorf("failed to save after cleaning stale hooks: %w", err)
		}
	}

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

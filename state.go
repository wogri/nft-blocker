package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type GroupState struct {
	Blocked      bool       `yaml:"blocked"`
	BlockedUntil *time.Time `yaml:"blocked_until,omitempty"`
}

type StateData struct {
	Groups   map[string]*GroupState `yaml:"groups"`
	BlockAll bool                   `yaml:"block_all"`
}

type State struct {
	mu       sync.RWMutex
	data     StateData
	filePath string
}

func NewState(filePath string) *State {
	return &State{
		filePath: filePath,
		data: StateData{
			Groups: make(map[string]*GroupState),
		},
	}
}

func (s *State) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file yet, start fresh
		}
		return fmt.Errorf("reading state %s: %w", s.filePath, err)
	}
	var data StateData
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing state %s: %w", s.filePath, err)
	}
	if data.Groups == nil {
		data.Groups = make(map[string]*GroupState)
	}
	s.data = data
	return nil
}

func (s *State) Save() error {
	s.mu.RLock()
	raw, err := yaml.Marshal(&s.data)
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, raw, 0600); err != nil {
		return fmt.Errorf("writing state tmp: %w", err)
	}
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}

func (s *State) SetGroupBlocked(name string, blocked bool, until *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Groups[name] = &GroupState{
		Blocked:      blocked,
		BlockedUntil: until,
	}
}

func (s *State) SetBlockAll(blocked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.BlockAll = blocked
}

func (s *State) GetGroupState(name string) (blocked bool, until *time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	gs, ok := s.data.Groups[name]
	if !ok {
		return false, nil
	}
	return gs.Blocked, gs.BlockedUntil
}

func (s *State) IsBlockAll() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.BlockAll
}

func (s *State) Snapshot() StateData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Deep copy groups
	groups := make(map[string]*GroupState, len(s.data.Groups))
	for k, v := range s.data.Groups {
		cp := *v
		groups[k] = &cp
	}
	return StateData{
		Groups:   groups,
		BlockAll: s.data.BlockAll,
	}
}

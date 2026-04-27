package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestState_NewState(t *testing.T) {
	s := NewState("test.yaml")
	if s.IsBlockAll() {
		t.Error("new state should not have block-all active")
	}
	blocked, until := s.GetGroupState("nonexistent")
	if blocked || until != nil {
		t.Error("nonexistent group should not be blocked")
	}
}

func TestState_SetGroupBlocked(t *testing.T) {
	s := NewState("")

	s.SetGroupBlocked("kids", true, nil)
	blocked, until := s.GetGroupState("kids")
	if !blocked {
		t.Error("kids should be blocked")
	}
	if until != nil {
		t.Error("until should be nil for forever block")
	}

	now := time.Now().Add(1 * time.Hour)
	s.SetGroupBlocked("kids", true, &now)
	blocked, until = s.GetGroupState("kids")
	if !blocked {
		t.Error("kids should still be blocked")
	}
	if until == nil || !until.Equal(now) {
		t.Errorf("until = %v, want %v", until, now)
	}

	s.SetGroupBlocked("kids", false, nil)
	blocked, _ = s.GetGroupState("kids")
	if blocked {
		t.Error("kids should be unblocked")
	}
}

func TestState_SetBlockAll(t *testing.T) {
	s := NewState("")

	s.SetBlockAll(true, nil)
	if !s.IsBlockAll() {
		t.Error("block-all should be active")
	}
	ba := s.GetBlockAllState()
	if !ba.Blocked || ba.BlockedUntil != nil {
		t.Error("block-all state mismatch")
	}

	now := time.Now().Add(30 * time.Minute)
	s.SetBlockAll(true, &now)
	ba = s.GetBlockAllState()
	if ba.BlockedUntil == nil || !ba.BlockedUntil.Equal(now) {
		t.Errorf("block-all until = %v, want %v", ba.BlockedUntil, now)
	}

	s.SetBlockAll(false, nil)
	if s.IsBlockAll() {
		t.Error("block-all should be inactive")
	}
}

func TestState_Snapshot(t *testing.T) {
	s := NewState("")

	until := time.Now().Add(1 * time.Hour)
	s.SetGroupBlocked("kids", true, &until)
	s.SetBlockAll(true, nil)

	snap := s.Snapshot()

	// Verify snapshot is independent (deep copy)
	s.SetGroupBlocked("kids", false, nil)
	s.SetBlockAll(false, nil)

	if !snap.Groups["kids"].Blocked {
		t.Error("snapshot should still show kids as blocked")
	}
	if !snap.BlockAll.Blocked {
		t.Error("snapshot should still show block-all as active")
	}
}

func TestState_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	// Save state
	s := NewState(path)
	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	s.SetGroupBlocked("kids", true, &until)
	s.SetBlockAll(true, nil)
	if err := s.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Load into new state
	s2 := NewState(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}

	blocked, loadedUntil := s2.GetGroupState("kids")
	if !blocked {
		t.Error("loaded state: kids should be blocked")
	}
	if loadedUntil == nil || !loadedUntil.Truncate(time.Second).Equal(until) {
		t.Errorf("loaded until = %v, want %v", loadedUntil, until)
	}
	if !s2.IsBlockAll() {
		t.Error("loaded state: block-all should be active")
	}
}

func TestState_LoadNonexistent(t *testing.T) {
	s := NewState("/nonexistent/state.yaml")
	if err := s.Load(); err != nil {
		t.Fatalf("loading nonexistent file should not error: %v", err)
	}
}

func TestState_LoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	os.WriteFile(path, []byte(`{{{{`), 0644)

	s := NewState(path)
	if err := s.Load(); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestState_SaveAtomicity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	s := NewState(path)
	s.SetGroupBlocked("g1", true, nil)
	if err := s.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Tmp file should not remain
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after save")
	}
}

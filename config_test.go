package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
password: "secret"
listen: ":9090"
interfaces:
  - "eth0"
  - "eth1"
state_file: "mystate.yaml"
groups:
  kids:
    display_name: "Kids"
    mac_addresses:
      - "AA:BB:CC:DD:EE:FF"
`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Password != "secret" {
		t.Errorf("password = %q, want %q", cfg.Password, "secret")
	}
	if cfg.Listen != ":9090" {
		t.Errorf("listen = %q, want %q", cfg.Listen, ":9090")
	}
	if len(cfg.Interfaces) != 2 || cfg.Interfaces[0] != "eth0" {
		t.Errorf("interfaces = %v, want [eth0 eth1]", cfg.Interfaces)
	}
	if cfg.StateFile != "mystate.yaml" {
		t.Errorf("state_file = %q, want %q", cfg.StateFile, "mystate.yaml")
	}
	if len(cfg.Groups) != 1 {
		t.Errorf("groups count = %d, want 1", len(cfg.Groups))
	}
	g := cfg.Groups["kids"]
	if g.DisplayName != "Kids" {
		t.Errorf("display_name = %q, want %q", g.DisplayName, "Kids")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
password: "pass"
groups:
  g1:
    display_name: "G1"
    mac_addresses:
      - "11:22:33:44:55:66"
`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != ":8081" {
		t.Errorf("default listen = %q, want %q", cfg.Listen, ":8081")
	}
	if len(cfg.Interfaces) != 1 || cfg.Interfaces[0] != "br_lan" {
		t.Errorf("default interfaces = %v, want [br_lan]", cfg.Interfaces)
	}
	if cfg.StateFile != "state.yaml" {
		t.Errorf("default state_file = %q, want %q", cfg.StateFile, "state.yaml")
	}
}

func TestLoadConfig_MissingPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
groups:
  g1:
    display_name: "G1"
    mac_addresses:
      - "11:22:33:44:55:66"
`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestLoadConfig_NoGroups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`password: "pass"`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for no groups")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`{{{{`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

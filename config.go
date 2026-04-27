package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Group struct {
	DisplayName  string   `yaml:"display_name"`
	MACAddresses []string `yaml:"mac_addresses"`
}

type Config struct {
	Password   string           `yaml:"password"`
	Listen     string           `yaml:"listen"`
	Interfaces []string         `yaml:"interfaces"`
	StateFile  string           `yaml:"state_file"`
	Groups     map[string]Group `yaml:"groups"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("config: password must be set")
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8081"
	}
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = []string{"br_lan"}
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "state.yaml"
	}
	if len(cfg.Groups) == 0 {
		return nil, fmt.Errorf("config: at least one group must be defined")
	}
	return &cfg, nil
}

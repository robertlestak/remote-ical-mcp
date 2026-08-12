package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type RemoteCalendar struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description"`
	URL         string `json:"url" yaml:"url"`
}

type Config struct {
	Calendars []RemoteCalendar `json:"calendars" yaml:"calendars"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	ext := filepath.Ext(path)

	if ext == ".json" {
		err = json.Unmarshal(data, cfg)
	} else {
		err = yaml.Unmarshal(data, cfg)
	}
	if err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if len(c.Calendars) == 0 {
		return fmt.Errorf("no calendars configured")
	}
	seen := make(map[string]bool, len(c.Calendars))
	for i, cal := range c.Calendars {
		if cal.Name == "" {
			return fmt.Errorf("calendar %d has no name", i)
		}
		if cal.URL == "" {
			return fmt.Errorf("calendar %q has no url", cal.Name)
		}
		if seen[cal.Name] {
			return fmt.Errorf("duplicate calendar name %q", cal.Name)
		}
		seen[cal.Name] = true
	}
	return nil
}

//go:build !restricted

package config

import (
	"encoding/json"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		cfg.Normalize()
		return cfg, Validate(cfg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	expanded := os.ExpandEnv(string(b))
	if strings.HasPrefix(strings.TrimSpace(expanded), "{") {
		cfg = Config{}
		if err := json.Unmarshal([]byte(expanded), &cfg); err != nil {
			return cfg, err
		}
	} else {
		cfg = Config{}
		dec := yaml.NewDecoder(strings.NewReader(expanded))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			return cfg, err
		}
	}
	cfg.Normalize()
	return cfg, Validate(cfg)
}

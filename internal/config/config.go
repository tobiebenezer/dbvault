//go:build restricted

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Load supports JSON and the legacy single-resource YAML subset in restricted
// builds. Full strict YAML arrays are available in the production build.
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
		cfg.Normalize()
		return cfg, Validate(cfg)
	}
	m := parseFlatYAML(expanded)
	applyLegacy(&cfg, m)
	cfg.Normalize()
	return cfg, Validate(cfg)
}

func parseFlatYAML(s string) map[string]string {
	out := map[string]string{}
	stack := []string{}
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		level := indent / 2
		if level < len(stack) {
			stack = stack[:level]
		}
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), "\"")
		if value == "" {
			if level == len(stack) {
				stack = append(stack, key)
			} else if level < len(stack) {
				stack[level] = key
			}
			continue
		}
		full := append(append([]string{}, stack...), key)
		out[strings.Join(full, ".")] = value
	}
	return out
}

func applyLegacy(c *Config, m map[string]string) {
	setS := func(k string, p *string) {
		if v, ok := m[k]; ok {
			*p = v
		}
	}
	setB := func(k string, p *bool) {
		if v, ok := m[k]; ok {
			*p = v == "true" || v == "yes"
		}
	}
	setI := func(k string, p *int) {
		if v, ok := m[k]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				*p = n
			}
		}
	}
	setI64 := func(k string, p *int64) {
		if v, ok := m[k]; ok {
			if n, err := ParseBytes(v); err == nil {
				*p = n
			}
		}
	}
	if v, ok := m["version"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			panic(fmt.Sprintf("invalid version %q", v))
		}
		c.Version = n
	}
	setS("server.data_directory", &c.Server.DataDirectory)
	setS("server.scratch_directory", &c.Server.ScratchDirectory)
	setS("server.listen", &c.Server.Listen)
	setS("catalogue.path", &c.Catalogue.Path)
	setS("source.id", &c.Source.ID)
	setS("source.name", &c.Source.Name)
	setS("source.driver", &c.Source.Driver)
	setS("source.path", &c.Source.Path)
	setB("source.enabled", &c.Source.Enabled)
	setS("repository.id", &c.Repository.ID)
	setS("repository.name", &c.Repository.Name)
	setI64("repository.chunk_bytes", &c.Repository.ChunkBytes)
	setS("repository.key_id", &c.Repository.KeyID)
	setI("repository.retention.keep_last", &c.Repository.Retention.KeepLast)
	setI("repository.retention.daily", &c.Repository.Retention.Daily)
	setI("repository.retention.weekly", &c.Repository.Retention.Weekly)
	setI("repository.retention.monthly", &c.Repository.Retention.Monthly)
	setS("repository.retention.tombstone_grace", &c.Repository.Retention.TombstoneGrace)
	setS("destination.id", &c.Destination.ID)
	setS("destination.driver", &c.Destination.Driver)
	setS("destination.path", &c.Destination.Path)
	setS("destination.profile", &c.Destination.Profile)
	setS("destination.endpoint", &c.Destination.Endpoint)
	setS("destination.region", &c.Destination.Region)
	setS("destination.bucket", &c.Destination.Bucket)
	setS("destination.prefix", &c.Destination.Prefix)
	setB("destination.use_path_style", &c.Destination.UsePathStyle)
	setS("keys.directory", &c.Keys.Directory)
	setB("schedule.enabled", &c.Schedule.Enabled)
	setI("schedule.every_seconds", &c.Schedule.EverySeconds)
	setS("schedule.cron", &c.Schedule.Cron)
	setS("schedule.timezone", &c.Schedule.Timezone)
}

package config

import (
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	RunnerSocket           string `koanf:"runner_socket"`
	AnalyzerSocket         string `koanf:"analyzer_socket"`
	DatabaseURL            string `koanf:"database_url"`
	Port                   string `koanf:"port"`
	ListenHost             string `koanf:"listen_host"`
	AdminToken             string `koanf:"admin_token"`
	StaticDir              string `koanf:"static_dir"`
	SessionTTLHours        int    `koanf:"session_ttl_hours"`
	ContestDurationSeconds int    `koanf:"contest_duration_seconds"`
}

func Load(path string) (Config, error) {
	k := koanf.New(".")

	// Missing/unreadable config.toml is non-fatal — env vars can still carry the day.
	_ = k.Load(file.Provider(path), toml.Parser())

	_ = k.Load(env.Provider("", ".", func(s string) string {
		return strings.ToLower(s)
	}), nil)

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

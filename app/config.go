package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type httpListen struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (h httpListen) Addr() string {
	host := h.Host
	if host == "" {
		host = "0.0.0.0"
	}
	if h.Port == 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", host, h.Port)
}

type serverConfig struct {
	System httpListen
	API    httpListen
}

func parseServerConfig(raw []byte) (serverConfig, error) {
	var cf struct {
		Server struct {
			HTTP map[string]httpListen `yaml:"http"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(raw, &cf); err != nil {
		return serverConfig{}, fmt.Errorf("server config: %w", err)
	}
	sys, ok := cf.Server.HTTP["system"]
	if !ok || sys.Port == 0 {
		return serverConfig{}, fmt.Errorf("server.http.system.port required")
	}
	api, ok := cf.Server.HTTP["api"]
	if !ok || api.Port == 0 {
		return serverConfig{}, fmt.Errorf("server.http.api.port required")
	}
	return serverConfig{System: sys, API: api}, nil
}

func newLogger(raw []byte) (*slog.Logger, error) {
	var cf struct {
		Logger struct {
			Level  string `yaml:"level"`
			Format string `yaml:"format"`
		} `yaml:"logger"`
	}
	if err := yaml.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("logger config: %w", err)
	}
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(cf.Logger.Level)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(cf.Logger.Format, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h), nil
}

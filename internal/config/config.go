package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from a YAML string like "30s".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// Service describes a single monitored service.
type Service struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Target         string            `yaml:"target"`
	Interval       Duration          `yaml:"interval"`
	Timeout        Duration          `yaml:"timeout"`
	ExpectedStatus int               `yaml:"expected_status"`
	Headers        map[string]string `yaml:"headers"`
}

// WebhookConfig holds alert webhook settings.
type WebhookConfig struct {
	URL      string   `yaml:"url"`
	Cooldown Duration `yaml:"cooldown"`
}

// AlertsConfig holds all alert configuration.
type AlertsConfig struct {
	Webhook WebhookConfig `yaml:"webhook"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Address string `yaml:"address"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	Path string `yaml:"path"`
}

// Config is the root application configuration.
type Config struct {
	Services []Service     `yaml:"services"`
	Alerts   AlertsConfig  `yaml:"alerts"`
	Server   ServerConfig  `yaml:"server"`
	Storage  StorageConfig `yaml:"storage"`
}

var validTypes = map[string]bool{
	"http":   true,
	"tcp":    true,
	"ping":   true,
	"docker": true,
}

// Load reads, parses, and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Unmarshal into a raw intermediate to detect YAML parse errors vs duration errors.
	type rawService struct {
		Name           string            `yaml:"name"`
		Type           string            `yaml:"type"`
		Target         string            `yaml:"target"`
		Interval       string            `yaml:"interval"`
		Timeout        string            `yaml:"timeout"`
		ExpectedStatus int               `yaml:"expected_status"`
		Headers        map[string]string `yaml:"headers"`
	}
	type rawConfig struct {
		Services []rawService  `yaml:"services"`
		Alerts   AlertsConfig  `yaml:"alerts"`
		Server   ServerConfig  `yaml:"server"`
		Storage  StorageConfig `yaml:"storage"`
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults.
	if raw.Server.Address == "" {
		raw.Server.Address = ":8080"
	}
	if raw.Storage.Path == "" {
		raw.Storage.Path = "servprobe.db"
	}

	if len(raw.Services) == 0 {
		return nil, fmt.Errorf("at least one service must be configured")
	}

	cfg := &Config{
		Alerts:  raw.Alerts,
		Server:  raw.Server,
		Storage: raw.Storage,
	}

	names := make(map[string]bool, len(raw.Services))
	for i, rs := range raw.Services {
		if rs.Name == "" {
			return nil, fmt.Errorf("service[%d]: name is required", i)
		}
		if names[rs.Name] {
			return nil, fmt.Errorf("duplicate service name %q", rs.Name)
		}
		names[rs.Name] = true

		if rs.Target == "" {
			return nil, fmt.Errorf("service %q: target is required", rs.Name)
		}
		if !validTypes[rs.Type] {
			return nil, fmt.Errorf("service %q: invalid type %q (must be http, tcp, ping, or docker)", rs.Name, rs.Type)
		}

		svc := Service{
			Name:           rs.Name,
			Type:           rs.Type,
			Target:         rs.Target,
			ExpectedStatus: rs.ExpectedStatus,
			Headers:        rs.Headers,
		}

		// Parse interval with default.
		if rs.Interval == "" {
			svc.Interval = Duration{30 * time.Second}
		} else {
			d, err := time.ParseDuration(rs.Interval)
			if err != nil {
				return nil, fmt.Errorf("service %q: invalid interval %q: %w", rs.Name, rs.Interval, err)
			}
			svc.Interval = Duration{d}
		}

		// Parse timeout with default.
		if rs.Timeout == "" {
			svc.Timeout = Duration{5 * time.Second}
		} else {
			d, err := time.ParseDuration(rs.Timeout)
			if err != nil {
				return nil, fmt.Errorf("service %q: invalid timeout %q: %w", rs.Name, rs.Timeout, err)
			}
			svc.Timeout = Duration{d}
		}

		// Default expected_status for HTTP.
		if rs.Type == "http" && svc.ExpectedStatus == 0 {
			svc.ExpectedStatus = 200
		}

		cfg.Services = append(cfg.Services, svc)
	}

	return cfg, nil
}

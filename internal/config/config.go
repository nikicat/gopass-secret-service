package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for gopass-secret-service
type Config struct {
	// StorePath is the path to the GoPass store
	StorePath string `yaml:"store_path"`

	// Prefix is the prefix for secret-service entries in gopass
	Prefix string `yaml:"prefix"`

	// DefaultCollection is the name of the default collection
	DefaultCollection string `yaml:"default_collection"`

	// LogLevel is the logging level (debug, info, warn, error)
	LogLevel string `yaml:"log_level"`

	// LogFile is the path to the log file (empty for stderr)
	LogFile string `yaml:"log_file"`

	// Replace indicates whether to replace an existing secret-service provider
	Replace bool `yaml:"replace"`

	// BusAddress is a custom D-Bus socket address to connect to instead of the session bus.
	// When set, DBUS_SESSION_BUS_ADDRESS is left unchanged (useful for private bus setups
	// where child processes like gpg-agent/pinentry still need the real session bus).
	BusAddress string `yaml:"bus_address"`

	// ConfigPath is the resolved path to the config file
	ConfigPath string `yaml:"-"`
}

// DefaultConfig returns a new Config with default values
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		StorePath:         filepath.Join(homeDir, ".local/share/gopass/stores/root"),
		Prefix:            "secret-service",
		DefaultCollection: "default",
		LogLevel:          "info",
		LogFile:           "",
		Replace:           false,
	}
}

// ResolveConfigPath resolves the config file path from: explicit value > env > default
func ResolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if envPath := os.Getenv("GOPASS_SECRET_SERVICE_CONFIG"); envPath != "" {
		return envPath
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config/gopass-secret-service/config.yaml")
}

// LoadFromFileAndEnv loads config: defaults → file → env vars. No flag parsing.
func LoadFromFileAndEnv(configPath string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.ConfigPath = configPath

	// Load config file if it exists
	if err := cfg.loadFromFile(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Apply environment variables (override config file)
	cfg.applyEnv()

	// Expand ~ in paths
	cfg.StorePath = expandPath(cfg.StorePath)
	cfg.LogFile = expandPath(cfg.LogFile)

	return cfg, nil
}

func (c *Config) loadFromFile() error {
	data, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		return err
	}
	expanded := os.ExpandEnv(string(data))
	return yaml.Unmarshal([]byte(expanded), c)
}

func (c *Config) applyEnv() {
	if v := os.Getenv("GOPASS_SECRET_SERVICE_STORE_PATH"); v != "" {
		c.StorePath = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_PREFIX"); v != "" {
		c.Prefix = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_DEFAULT_COLLECTION"); v != "" {
		c.DefaultCollection = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_LOG_FILE"); v != "" {
		c.LogFile = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_REPLACE"); v == "true" || v == "1" {
		c.Replace = true
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_BUS_ADDRESS"); v != "" {
		c.BusAddress = v
	}
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}
